package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RestartMode mirrors TS enum: "never" | "on-failure".
type RestartMode string

const (
	RestartNever     RestartMode = "never"
	RestartOnFailure RestartMode = "on-failure"
)

type Project struct {
	Path    string            `yaml:"path"`
	Cmd     string            `yaml:"cmd"`
	Restart RestartMode       `yaml:"restart,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	EnvFile string            `yaml:"env_file,omitempty"`
}

type Settings struct {
	LogBufferLines      int    `yaml:"log_buffer_lines"`
	LogDir              string `yaml:"log_dir"`
	LogRotateSizeMB     int    `yaml:"log_rotate_size_mb"`
	LogRotateKeep       int    `yaml:"log_rotate_keep"`
	ShutdownGraceMs     int    `yaml:"shutdown_grace_ms"`
	GroupStartDelayMs   int    `yaml:"group_start_delay_ms"`
	RestartBackoffMs    []int  `yaml:"restart_backoff_ms"`
	RestartMaxAttempts  int    `yaml:"restart_max_attempts"`
	RestartResetAfterMs int    `yaml:"restart_reset_after_ms"`
	PtyCols             int    `yaml:"pty_cols"`
	PtyRows             int    `yaml:"pty_rows"`
	LogFlushIntervalMs  int    `yaml:"log_flush_interval_ms"`
}

type Config struct {
	Projects map[string]Project  `yaml:"projects"`
	Groups   map[string][]string `yaml:"groups,omitempty"`
	Settings Settings            `yaml:"settings,omitempty"`
}

const defaultConfigPath = "~/.config/procs/config.yml"

// ResolvePath returns the config file path, honouring explicit > env > default.
func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return ExpandPath(explicit)
	}
	if v := os.Getenv("PROCS_CONFIG"); v != "" {
		return ExpandPath(v)
	}
	return ExpandPath(defaultConfigPath)
}

// Load reads, parses, validates, and expands a config file.
// explicit overrides env + default.
func Load(explicit string) (*Config, error) {
	path, err := ResolvePath(explicit)
	if err != nil {
		return nil, wrap("", "", "resolve config path", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, wrap(path, "", "read config file", err)
	}

	cfg, err := decode(raw)
	if err != nil {
		return nil, wrap(path, "", err.Error(), err)
	}

	applyDefaults(&cfg.Settings)

	if err := validate(cfg); err != nil {
		return nil, &Error{Path: path, Msg: err.Error(), Err: err}
	}
	if err := expandAll(cfg); err != nil {
		return nil, &Error{Path: path, Msg: err.Error(), Err: err}
	}
	return cfg, nil
}

func decode(raw []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytesReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return &cfg, nil
}

func applyDefaults(s *Settings) {
	d := DefaultSettings
	if s.LogBufferLines == 0 {
		s.LogBufferLines = d.LogBufferLines
	}
	if s.LogDir == "" {
		s.LogDir = d.LogDir
	}
	if s.LogRotateSizeMB == 0 {
		s.LogRotateSizeMB = d.LogRotateSizeMB
	}
	if s.LogRotateKeep == 0 {
		s.LogRotateKeep = d.LogRotateKeep
	}
	if s.ShutdownGraceMs == 0 {
		s.ShutdownGraceMs = d.ShutdownGraceMs
	}
	if s.GroupStartDelayMs == 0 {
		s.GroupStartDelayMs = d.GroupStartDelayMs
	}
	if len(s.RestartBackoffMs) == 0 {
		s.RestartBackoffMs = append([]int(nil), d.RestartBackoffMs...)
	}
	if s.RestartMaxAttempts == 0 {
		s.RestartMaxAttempts = d.RestartMaxAttempts
	}
	if s.RestartResetAfterMs == 0 {
		s.RestartResetAfterMs = d.RestartResetAfterMs
	}
	if s.PtyCols == 0 {
		s.PtyCols = d.PtyCols
	}
	if s.PtyRows == 0 {
		s.PtyRows = d.PtyRows
	}
	if s.LogFlushIntervalMs == 0 {
		s.LogFlushIntervalMs = d.LogFlushIntervalMs
	}
}

func validate(c *Config) error {
	var errs []error
	if len(c.Projects) == 0 {
		errs = append(errs, fmt.Errorf("projects: at least one project required"))
	}
	for id, p := range c.Projects {
		if p.Path == "" {
			errs = append(errs, fmt.Errorf("projects.%s.path: required", id))
		}
		if p.Cmd == "" {
			errs = append(errs, fmt.Errorf("projects.%s.cmd: required", id))
		}
		if p.Restart == "" {
			p.Restart = RestartNever
			c.Projects[id] = p
		}
		if p.Restart != RestartNever && p.Restart != RestartOnFailure {
			errs = append(errs, fmt.Errorf("projects.%s.restart: must be 'never' or 'on-failure', got %q", id, p.Restart))
		}
	}
	for gname, members := range c.Groups {
		if len(members) == 0 {
			errs = append(errs, fmt.Errorf("groups.%s: must have at least one member", gname))
		}
		for i, m := range members {
			if _, ok := c.Projects[m]; !ok {
				errs = append(errs, fmt.Errorf("groups.%s[%d]: unknown project %q", gname, i, m))
			}
		}
	}
	if c.Settings.LogBufferLines < 1 {
		errs = append(errs, fmt.Errorf("settings.log_buffer_lines: must be positive"))
	}
	if c.Settings.LogRotateKeep < 1 {
		errs = append(errs, fmt.Errorf("settings.log_rotate_keep: must be positive"))
	}
	if c.Settings.LogFlushIntervalMs < 1 {
		errs = append(errs, fmt.Errorf("settings.log_flush_interval_ms: must be positive"))
	}
	if c.Settings.PtyCols < 1 || c.Settings.PtyRows < 1 {
		errs = append(errs, fmt.Errorf("settings.pty_cols/pty_rows: must be positive"))
	}
	return joinErrs(errs)
}

func expandAll(c *Config) error {
	var errs []error
	for id, p := range c.Projects {
		exp, err := ExpandPath(p.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf("projects.%s.path: %w", id, err))
			continue
		}
		p.Path = exp
		if p.EnvFile != "" {
			envExp, err := ExpandPath(p.EnvFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("projects.%s.env_file: %w", id, err))
				continue
			}
			p.EnvFile = envExp
		}
		c.Projects[id] = p
	}
	if exp, err := ExpandPath(c.Settings.LogDir); err != nil {
		errs = append(errs, fmt.Errorf("settings.log_dir: %w", err))
	} else {
		c.Settings.LogDir = exp
	}
	return joinErrs(errs)
}

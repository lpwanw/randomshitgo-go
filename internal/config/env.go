package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// parseEnvFile reads a `KEY=VALUE` file (one pair per line). Blank lines and
// `#` comments are skipped. Quoted values — single or double — have the outer
// quotes stripped. Returns a parse error with the offending line number when
// the file contains malformed content, so startup surfaces a clear pointer.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		// Support optional `export ` prefix (matches bash env files).
		if strings.HasPrefix(raw, "export ") {
			raw = strings.TrimSpace(raw[len("export "):])
		}
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("line %d: missing '=': %q", lineNo, raw)
		}
		key := strings.TrimSpace(raw[:eq])
		val := strings.TrimSpace(raw[eq+1:])
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return out, nil
}

// BuildEnv returns the environment slice ("KEY=VALUE" form) to pass to the
// child process. Precedence (low → high, later wins): `base` → env_file →
// inline env. Unless the caller overrides `base`, pass `os.Environ()` so the
// child inherits `$PATH`, `$TERM`, etc.
func (p Project) BuildEnv(base []string) ([]string, error) {
	merged := make(map[string]string, len(base)+len(p.Env)+8)
	for _, kv := range base {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			merged[kv[:eq]] = kv[eq+1:]
		}
	}
	if p.EnvFile != "" {
		fileEnv, err := parseEnvFile(p.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("env_file %s: %w", p.EnvFile, err)
		}
		for k, v := range fileEnv {
			merged[k] = v
		}
	}
	for k, v := range p.Env {
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	return out, nil
}

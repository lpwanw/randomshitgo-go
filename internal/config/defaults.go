package config

// DefaultSettings mirrors src/config/defaults.ts verbatim. Zero fields in a
// loaded Settings are replaced from here.
var DefaultSettings = Settings{
	LogBufferLines:      1000,
	LogDir:              "~/.cache/procs/logs",
	LogRotateSizeMB:     10,
	LogRotateKeep:       5,
	ShutdownGraceMs:     5000,
	GroupStartDelayMs:   300,
	RestartBackoffMs:    []int{1000, 2000, 4000, 8000, 16000},
	RestartMaxAttempts:  5,
	RestartResetAfterMs: 60000,
	PtyCols:             120,
	PtyRows:             40,
	LogFlushIntervalMs:  150,
}

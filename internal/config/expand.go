package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var envVarRe = regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

// ExpandPath expands leading `~` and `$VAR` / `${VAR}` references. Strict:
// `~user` (with username) is rejected; undefined env vars error.
func ExpandPath(p string) (string, error) {
	out := p
	switch {
	case out == "~" || strings.HasPrefix(out, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		out = home + out[1:]
	case strings.HasPrefix(out, "~"):
		return "", fmt.Errorf("path %q: ~user expansion not supported; use absolute path or ~/ form", p)
	}

	var missing []string
	out = envVarRe.ReplaceAllStringFunc(out, func(m string) string {
		name := strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(m, "$"), "}"), "{")
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return ""
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("path %q: undefined env var(s): %s", p, strings.Join(missing, ", "))
	}
	return out, nil
}

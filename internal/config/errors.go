package config

import (
	"errors"
	"fmt"
	"strings"
)

// Error wraps a config failure with optional file + key path breadcrumb.
type Error struct {
	Path    string // config file path, if known
	KeyPath string // dotted path into the doc (e.g. "projects.api.cmd")
	Msg     string
	Err     error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("config")
	if e.Path != "" {
		fmt.Fprintf(&b, " (%s)", e.Path)
	}
	b.WriteString(": ")
	if e.KeyPath != "" {
		fmt.Fprintf(&b, "%s: ", e.KeyPath)
	}
	b.WriteString(e.Msg)
	if e.Err != nil && e.Err.Error() != e.Msg {
		fmt.Fprintf(&b, " (%v)", e.Err)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

func wrap(path, key, msg string, err error) error {
	return &Error{Path: path, KeyPath: key, Msg: msg, Err: err}
}

// Join multiple validation errors into one.
type multiErr struct{ errs []error }

func (m *multiErr) Error() string {
	parts := make([]string, len(m.errs))
	for i, e := range m.errs {
		parts[i] = "  • " + e.Error()
	}
	return "config validation failed:\n" + strings.Join(parts, "\n")
}
func (m *multiErr) Unwrap() []error { return m.errs }

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return &multiErr{errs: errs}
}

var _ error = (*Error)(nil)
var _ interface{ Unwrap() error } = (*Error)(nil)
var _ interface{ Unwrap() []error } = (*multiErr)(nil)
var _ = errors.Unwrap // keep errors import

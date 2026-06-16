//go:build !windows

package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
)

// ChildRec is one persisted child: its project id and OS pid. Used to detect
// processes orphaned by a daemon crash so a fresh daemon doesn't start
// duplicates.
type ChildRec struct {
	ID  string `json:"id"`
	PID int    `json:"pid"`
}

// writeChildren atomically writes the current live children to path (0600) via
// a temp file + rename.
func writeChildren(path string, recs []ChildRec) error {
	data, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".children-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ReadChildren reads the persisted child list. A missing file yields nil, nil.
func ReadChildren(path string) ([]ChildRec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var recs []ChildRec
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// AliveChildren filters recs to those whose pid is still alive (signal 0).
func AliveChildren(recs []ChildRec) []ChildRec {
	var alive []ChildRec
	for _, r := range recs {
		if r.PID <= 0 {
			continue
		}
		if err := syscall.Kill(r.PID, 0); err == nil {
			alive = append(alive, r)
		}
	}
	return alive
}
//go:build !windows

package process

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestStartPTY_ReadsOutput(t *testing.T) {
	cmd, ptmx, err := startPTY(context.Background(), t.TempDir(), "printf hi", nil, 80, 24)
	if err != nil {
		t.Fatalf("startPTY: %v", err)
	}
	defer ptmx.Close()

	// Read with a deadline to avoid hanging.
	ptmx.SetReadDeadline(time.Now().Add(2 * time.Second))
	out, _ := io.ReadAll(ptmx)
	_ = cmd.Wait()

	if string(out) != "hi" {
		t.Fatalf("expected \"hi\", got %q", string(out))
	}
}

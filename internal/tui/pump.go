package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/taynguyen/procs/internal/state"
)

// subscribeRuntime returns a Cmd that blocks on the RuntimeStore notification
// channel, then sends a RuntimeChangedMsg and re-arms itself so the subscription
// stays live for the lifetime of the program.
func subscribeRuntime(s *state.RuntimeStore) tea.Cmd {
	ch := s.Subscribe()
	return waitRuntimeChange(s, ch)
}

// waitRuntimeChange waits for a single notification on ch, then emits
// RuntimeChangedMsg and queues the next wait. Using tea.Sequence ensures the
// re-arm happens after Update processes this message, preventing stacking.
func waitRuntimeChange(s *state.RuntimeStore, ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-ch
		snap := s.Snapshot()
		return RuntimeChangedMsg{Snapshot: snap}
	}
}

// rearmRuntimeSubscribe returns the next wait command after processing a
// RuntimeChangedMsg. The caller must invoke this and include the returned Cmd.
func rearmRuntimeSubscribe(s *state.RuntimeStore) tea.Cmd {
	// Re-use the store's existing subscription channel by creating a new one.
	// We create a fresh subscription so we don't miss notifications that
	// arrived while we were processing the previous one.
	return subscribeRuntime(s)
}

// logTick returns a Cmd that fires once after interval with a LogTickMsg.
// Re-arm by calling logTick again in the Update handler.
func logTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return LogTickMsg(t)
	})
}

// toastPruneTick returns a Cmd that fires once after 1 second to trigger
// expired-toast cleanup.
func toastPruneTick() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return ToastExpiredMsg{}
	})
}

// statusRefreshTick fires every 2 seconds to trigger git+port status refresh.
func statusRefreshTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return statusRefreshTickMsg{}
	})
}

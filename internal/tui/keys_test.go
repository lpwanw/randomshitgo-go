package tui

import (
	"strings"
	"testing"
)

// TestKeyMap_FullHelpIncludesCommand ensures the `:` command key is discoverable
// via the help overlay. Users who previously relied on `q` to quit must be able
// to find the new command entry point.
func TestKeyMap_FullHelpIncludesCommand(t *testing.T) {
	km := DefaultKeyMap()
	groups := km.FullHelp()

	found := false
	for _, group := range groups {
		for _, b := range group {
			for _, k := range b.Keys() {
				if k == ":" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("FullHelp() must expose the ':' command binding")
	}

	// Quit help text should reflect the new Ctrl+C-only binding.
	help := km.Quit.Help()
	if !strings.Contains(strings.ToLower(help.Key), "ctrl+c") {
		t.Errorf("Quit help key should mention ctrl+c, got %q", help.Key)
	}
}

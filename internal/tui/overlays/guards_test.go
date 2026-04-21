package overlays

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoFullScreenPlace is the structural guard for the "popup takes the whole
// UI" regression. Overlays must render as compact boxes / bars and let the
// caller compose them via tui.overlayCenter / overlayBottomRight. Any use of
// lipgloss.Place(width, height, …) in this package would re-introduce the bug.
func TestNoFullScreenPlace(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read overlays dir: %v", err)
	}

	// Matches lipgloss.Place(..., width, ..., height, ...) — i.e. a Place call
	// that takes a width/height pair from a View method. Anchor-less Place
	// calls (e.g. lipgloss.Place(10, 3, …)) are not expected here.
	fullScreenRe := regexp.MustCompile(`lipgloss\.Place\s*\([^,)]*width[^,)]*,[^,)]*height`)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if fullScreenRe.Match(data) {
			t.Errorf("%s: overlay still calls lipgloss.Place with width×height — use compact View + tui.overlayCenter instead", path)
		}
	}
}

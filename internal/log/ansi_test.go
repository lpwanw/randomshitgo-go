package log

import "testing"

func TestStripANSI(t *testing.T) {
	in := "\x1b[31;1merror\x1b[0m: file\x1b[Knot found"
	got := StripANSI(in)
	if got != "error: filenot found" {
		t.Fatalf("got %q", got)
	}
}

func TestStripANSIPreservesPlain(t *testing.T) {
	in := "plain text with (punct)"
	if got := StripANSI(in); got != in {
		t.Fatalf("mangled plain: %q", got)
	}
}

func TestDecodeForRenderPreservesColors(t *testing.T) {
	in := []byte("\x1b[31mred\x1b[0m")
	if got := DecodeForRender(in); got != string(in) {
		t.Fatalf("colour sgr dropped: %q", got)
	}
}

func TestDecodeForRenderStripsCursorEscapes(t *testing.T) {
	in := []byte("\x1b[2Jboom\x1b[Hdone")
	got := DecodeForRender(in)
	if got != "boomdone" {
		t.Fatalf("cursor-escape leak: %q", got)
	}
}

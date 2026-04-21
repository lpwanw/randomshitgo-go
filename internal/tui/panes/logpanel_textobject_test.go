package panes

import "testing"

func TestTObj_Iw_Word(t *testing.T) {
	s, e, ok := tobjWordLike("foo BAR baz", 5, isWord, false)
	if !ok || s != 4 || e != 7 {
		t.Errorf("iw BAR: want (4,7), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_Aw_AddsTrailingSpace(t *testing.T) {
	s, e, ok := tobjWordLike("foo BAR baz", 5, isWord, true)
	if !ok || s != 4 || e != 8 {
		t.Errorf("aw BAR: want (4,8), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_Aw_LeadingWhenNoTrailing(t *testing.T) {
	s, e, ok := tobjWordLike("foo bar", 5, isWord, true)
	// cursor on "bar"; no trailing whitespace → include leading space
	if !ok || s != 3 || e != 7 {
		t.Errorf("aw trailing-eol: want (3,7), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_IW_BigWord(t *testing.T) {
	s, e, ok := tobjWordLike("foo-bar baz", 2, isWORD, false)
	if !ok || s != 0 || e != 7 {
		t.Errorf("iW foo-bar: want (0,7), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_Iquote_Inner(t *testing.T) {
	// "x=\"hello world\" rest" — find inside the quotes.
	line := `x="hello world" rest`
	s, e, ok := tobjQuote(line, 5, '"', false)
	if !ok || s != 3 || e != 14 {
		t.Errorf(`i" inner: want (3,14), got (%d,%d,%v)`, s, e, ok)
	}
}

func TestTObj_Aquote_IncludesQuotesAndTrailingSpace(t *testing.T) {
	line := `x="hello world" rest`
	s, e, ok := tobjQuote(line, 5, '"', true)
	// open=2, close=14, around=true → (2, 15) then trailing space → 16
	if !ok || s != 2 || e != 16 {
		t.Errorf(`a" outer: want (2,16), got (%d,%d,%v)`, s, e, ok)
	}
}

func TestTObj_Paren_Inner(t *testing.T) {
	line := "fn(alpha, beta)"
	s, e, ok := tobjBracket(line, 4, '(', ')', false)
	if !ok || s != 3 || e != 14 {
		t.Errorf("i( inner: want (3,14), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_Paren_InnermostWins(t *testing.T) {
	// Two nested pairs; cursor inside the inner one should pick the inner.
	line := "a(b(c)d)e"
	s, e, ok := tobjBracket(line, 4, '(', ')', false)
	if !ok || s != 4 || e != 5 {
		t.Errorf("i( innermost: want (4,5), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_Brace_Around(t *testing.T) {
	line := "x{body}y"
	s, e, ok := tobjBracket(line, 3, '{', '}', true)
	if !ok || s != 1 || e != 7 {
		t.Errorf("a{ outer: want (1,7), got (%d,%d,%v)", s, e, ok)
	}
}

func TestTObj_MissingPair(t *testing.T) {
	_, _, ok := tobjQuote("no quotes here", 3, '"', false)
	if ok {
		t.Error("missing quotes should return ok=false")
	}
	_, _, ok = tobjBracket("no brackets here", 3, '(', ')', false)
	if ok {
		t.Error("missing brackets should return ok=false")
	}
}

package panes

import (
	"strings"
	"testing"
)

func TestSQL_DetectLeadingVerb(t *testing.T) {
	if _, ok := prettifySQL("SELECT 1"); !ok {
		t.Error("SELECT should detect")
	}
	if _, ok := prettifySQL("  insert into t values (1)"); !ok {
		t.Error("lowercase should detect")
	}
	if _, ok := prettifySQL("just a log line"); ok {
		t.Error("plain line should NOT detect")
	}
}

func TestSQL_BasicSelect(t *testing.T) {
	rows, ok := prettifySQL("SELECT id, name FROM users WHERE id = 1 ORDER BY name LIMIT 5")
	if !ok {
		t.Fatal("detect failed")
	}
	want := []string{
		"SELECT id, name",
		"FROM users",
		"WHERE id = 1",
		"ORDER BY name",
		"LIMIT 5",
	}
	if !eqSlice(unblueAll(rows), want) {
		t.Errorf("want %v\n got %v", want, rows)
	}
}

func TestSQL_AndOrIndented(t *testing.T) {
	rows, _ := prettifySQL("SELECT * FROM t WHERE a=1 AND b=2 OR c=3")
	joined := strings.Join(unblueAll(rows), "\n")
	if !strings.Contains(joined, "  AND b=2") {
		t.Errorf("AND should indent 2: %v", rows)
	}
	if !strings.Contains(joined, "  OR c=3") {
		t.Errorf("OR should indent 2: %v", rows)
	}
}

func TestSQL_QuoteIsolation(t *testing.T) {
	rows, _ := prettifySQL(`SELECT * FROM t WHERE name = 'SELECT me' AND k=1`)
	joined := strings.Join(unblueAll(rows), "\n")
	// The literal 'SELECT me' must not produce an extra row.
	if strings.Count(joined, "SELECT") != 2 {
		t.Errorf("quoted SELECT should not split, got:\n%s", joined)
	}
	// The literal SELECT should stay inside the WHERE row.
	if !strings.Contains(joined, "'SELECT me'") {
		t.Errorf("quoted literal mangled: %v", rows)
	}
}

func TestSQL_ParenSubqueryInline(t *testing.T) {
	rows, _ := prettifySQL("SELECT id FROM t WHERE id IN (SELECT id FROM other WHERE x=1)")
	joined := strings.Join(unblueAll(rows), "\n")
	if strings.Contains(joined, "\n  SELECT id FROM other") {
		t.Errorf("subquery should not split: %v", rows)
	}
}

func TestSQL_Join(t *testing.T) {
	rows, _ := prettifySQL("SELECT a.id FROM a INNER JOIN b ON a.id = b.a_id")
	joined := strings.Join(unblueAll(rows), "\n")
	if !strings.Contains(joined, "  INNER JOIN b") {
		t.Errorf("INNER JOIN should indent 2: %v", rows)
	}
	if !strings.Contains(joined, "  ON a.id = b.a_id") {
		t.Errorf("ON should indent 2: %v", rows)
	}
}

func TestSQL_Insert(t *testing.T) {
	rows, _ := prettifySQL("INSERT INTO users (id, name) VALUES (1, 'x')")
	want := []string{
		"INSERT INTO users (id, name)",
		"VALUES (1, 'x')",
	}
	if !eqSlice(unblueAll(rows), want) {
		t.Errorf("INSERT: want %v, got %v", want, rows)
	}
}

func TestSQL_CasePreserved(t *testing.T) {
	rows, _ := prettifySQL("select id from users")
	joined := strings.Join(unblueAll(rows), "\n")
	if !strings.HasPrefix(joined, "select ") {
		t.Errorf("case should be preserved: %q", joined)
	}
	if strings.Contains(joined, "SELECT ") || strings.Contains(joined, "FROM ") {
		t.Errorf("should not uppercase keywords: %q", joined)
	}
}

func TestSQL_PanelToggle(t *testing.T) {
	lp := NewLogPanel(80, 20)
	lp.SetLines([]string{"SELECT 1 FROM t"})
	if len(lp.rawLines) != 1 {
		t.Fatalf("default raw=%d want 1", len(lp.rawLines))
	}
	lp.SetSQLPretty(true)
	if len(lp.rawLines) < 2 {
		t.Errorf("SQL on should expand; raw=%v", lp.rawLines)
	}
	lp.SetSQLPretty(false)
	if len(lp.rawLines) != 1 {
		t.Errorf("SQL off should restore; raw=%v", lp.rawLines)
	}
}

func TestSQL_RailsPrefix(t *testing.T) {
	rows, ok := prettifySQL(`User Load (0.8ms)  SELECT "users".* FROM "users" WHERE "users"."id" = 1`)
	if !ok {
		t.Fatal("rails prefix should split")
	}
	if len(rows) < 4 {
		t.Fatalf("want >=4 rows (prefix + SELECT + FROM + WHERE), got %d: %v", len(rows), rows)
	}
	if !strings.Contains(unblue(rows[0]), "User Load (0.8ms)") {
		t.Errorf("first row should keep prefix: %q", rows[0])
	}
	if !strings.HasPrefix(unblue(rows[1]), "SELECT") {
		t.Errorf("second row should start SELECT: %q", rows[1])
	}
}

func TestSQL_SequelPrefix(t *testing.T) {
	rows, ok := prettifySQL(`(0.001s) SELECT * FROM posts`)
	if !ok {
		t.Fatal("sequel prefix should split")
	}
	if unblue(rows[0]) != "(0.001s)" {
		t.Errorf("prefix row: want (0.001s), got %q", rows[0])
	}
}

func TestSQL_GORMPrefix(t *testing.T) {
	rows, ok := prettifySQL(`[0.352ms] [rows:1] SELECT 1`)
	if !ok {
		t.Fatal("gorm prefix should split")
	}
	if !strings.Contains(unblue(rows[0]), "[0.352ms]") {
		t.Errorf("prefix row missing timing: %q", rows[0])
	}
}

func TestSQL_TimingButNotSQL_PassesThrough(t *testing.T) {
	rows, ok := prettifySQL(`response (15ms) ok`)
	if ok {
		t.Errorf("no SQL verb trailing → should not split; got %v", rows)
	}
}

func TestSQL_TransactionBeginPrefix(t *testing.T) {
	rows, ok := prettifySQL(`TRANSACTION (0.2ms)  BEGIN`)
	if !ok {
		t.Fatal("transaction prefix should split")
	}
	if !strings.Contains(unblue(rows[0]), "TRANSACTION (0.2ms)") {
		t.Errorf("prefix row missing: %q", rows[0])
	}
	if len(rows) < 2 || !strings.HasPrefix(strings.TrimSpace(unblue(rows[1])), "BEGIN") {
		t.Errorf("BEGIN row missing: %v", rows)
	}
}

func TestSQL_RowsWrappedBlue(t *testing.T) {
	rows, ok := prettifySQL("SELECT 1 FROM t WHERE x=1")
	if !ok {
		t.Fatal("detect failed")
	}
	for i, r := range rows {
		if !strings.HasPrefix(r, sqlBlueOpen) {
			t.Errorf("row %d missing blue open: %q", i, r)
		}
		if !strings.HasSuffix(r, sqlBlueClose) {
			t.Errorf("row %d missing fg-reset close: %q", i, r)
		}
	}
}

func TestSQL_PrefixRowAlsoBlue(t *testing.T) {
	rows, _ := prettifySQL(`User Load (0.8ms)  SELECT 1`)
	if len(rows) == 0 || !strings.HasPrefix(rows[0], sqlBlueOpen) {
		t.Errorf("prefix row should be blue-wrapped: %q", rows)
	}
}

// unblue strips the blue SGR wrapper that prettifySQL now applies to every
// row, so existing assertions can keep comparing semantic content.
func unblue(s string) string {
	s = strings.TrimPrefix(s, sqlBlueOpen)
	s = strings.TrimSuffix(s, sqlBlueClose)
	return s
}

func unblueAll(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = unblue(r)
	}
	return out
}

func eqSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

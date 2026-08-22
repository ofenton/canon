package event

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTriggersBlockRawUpdateAndDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(sample("CANON-1")); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE events SET subject = 'tampered'`,
		`DELETE FROM events`,
	} {
		if _, err := s.db.Exec(q); err == nil {
			t.Errorf("%q succeeded; the log must be append-only", q)
		} else if !strings.Contains(err.Error(), "immutable") {
			t.Errorf("%q: unexpected error %v", q, err)
		} else {
			t.Logf("%-40s blocked: %v", q, err)
		}
	}
}

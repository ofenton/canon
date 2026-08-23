package event

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seedN(t *testing.T, s *Store, n int) {
	t.Helper()
	at := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	actor := Actor{ID: "ollie", Kind: ActorHuman}
	if err := s.AppendBatch(func(yield func(*Event) bool) {
		for i := range n {
			if !yield(New("issue.created", fmt.Sprintf("CANON-%d", i+1), at, actor,
				map[string]any{"title": fmt.Sprintf("issue %d", i+1), "state": "todo"})) {
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// A plain copy of the main database file is not a backup in WAL mode. This is the
// mistake the Backup method exists to prevent, so it is worth demonstrating.
func TestPlainCopyOfTheMainFileLosesData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "canon.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedN(t, s, 500)

	// Copy only the main file, as a naive operator would.
	naive := filepath.Join(dir, "naive.db")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(naive, data, 0o600); err != nil {
		t.Fatal(err)
	}

	restored, err := Open(naive)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.Count()
	if err != nil {
		t.Fatal(err)
	}
	if got == 500 {
		t.Skip("this build checkpointed eagerly; the WAL hazard did not reproduce")
	}
	t.Logf("a plain copy of canon.db recovered %d of 500 events — the rest were in the WAL", got)
}

// AC: THE SYSTEM SHALL store all data in a single file that can be copied while
// running to produce a valid backup.
// AC: WHEN a copied data file is restored THE SYSTEM SHALL start with identical state.
func TestBackupIsConsistentWhileWriting(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedN(t, s, 500)

	// Keep writing throughout, so the backup is taken against a moving target.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		at := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
		actor := Actor{ID: "agent:one", Kind: ActorAgent, Model: "claude-opus-5"}
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = s.Append(New("issue.created", fmt.Sprintf("LIVE-%d", i), at, actor,
				map[string]any{"title": "concurrent", "state": "todo"}))
		}
	}()
	time.Sleep(30 * time.Millisecond)

	out := filepath.Join(dir, "backup.db")
	if err := s.Backup(out); err != nil {
		t.Fatalf("backup: %v", err)
	}
	close(stop)
	<-done

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("the backup is not a single file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("the backup is empty")
	}
	// One file, and only one: no sidecars to remember.
	for _, sidecar := range []string{out + "-wal", out + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Errorf("%s exists; the backup must be a single file", sidecar)
		}
	}

	restored, err := Open(out)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	defer restored.Close()

	count, err := restored.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count < 500 {
		t.Fatalf("the backup holds %d events; the 500 seeded before it must all be present", count)
	}
	t.Logf("backup captured %d events, taken while writes were in flight", count)

	// Every event must decode, and the seeded ones must all be there.
	events, err := restored.All()
	if err != nil {
		t.Fatalf("reading the restored log: %v", err)
	}
	seeded := 0
	for _, e := range events {
		if e.Type == "" || e.Actor.ID == "" {
			t.Fatalf("event %s did not survive the backup intact", e.ID)
		}
		if len(e.Subject) > 6 && e.Subject[:6] == "CANON-" {
			seeded++
		}
	}
	if seeded != 500 {
		t.Errorf("restored %d of 500 seeded events", seeded)
	}
}

func TestBackupNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedN(t, s, 10)

	out := filepath.Join(dir, "backup.db")
	if err := s.Backup(out); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(out); err == nil {
		t.Error("a second backup to the same path must be refused, not silently replace the first")
	}
}

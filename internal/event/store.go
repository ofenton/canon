package event

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ErrImmutable is returned when a write would alter an event that already exists.
var ErrImmutable = errors.New("events are immutable")

// Store is an append-only event log over SQLite.
//
// SQLite is used, rather than a plain file, because the projection layer needs to
// read ranges cheaply and because a single file that can be copied while running is
// the whole backup story (R22). Nothing here depends on SQL beyond INSERT and
// SELECT, so the same log can later live in a git ref without changing this contract.
//
// The schema has no UPDATE or DELETE path. That is enforced by triggers rather than
// convention, so a future careless query cannot quietly rewrite history.
type Store struct {
	db *sql.DB

	mu   sync.Mutex // serialises id generation so ids stay monotonic per process
	last string
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      TEXT    NOT NULL UNIQUE,
    type    TEXT    NOT NULL,
    subject TEXT    NOT NULL,
    at      TEXT    NOT NULL,
    body    BLOB    NOT NULL
) STRICT;

CREATE INDEX IF NOT EXISTS events_subject ON events (subject, seq);

-- Append-only is a property of the store, not a habit of its callers.
CREATE TRIGGER IF NOT EXISTS events_no_update
BEFORE UPDATE ON events
BEGIN
    SELECT RAISE(ABORT, 'events are immutable: update is not permitted');
END;

CREATE TRIGGER IF NOT EXISTS events_no_delete
BEFORE DELETE ON events
BEGIN
    SELECT RAISE(ABORT, 'events are immutable: delete is not permitted');
END;
`

// Open opens or creates a log at path.
func Open(path string) (*Store, error) {
	// WAL keeps readers unblocked during appends, and lets the file be copied for
	// backup while the server is running.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening event log %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialising event log %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Append validates and stores one event, assigning an id and sequence if absent.
func (s *Store) Append(e *Event) error {
	return s.AppendBatch(func(yield func(*Event) bool) { yield(e) })
}

// AppendBatch stores many events in one transaction.
//
// Per-event transactions cost an fsync each, which is what makes a naive loop
// thousands of times slower than the budget allows.
func (s *Store) AppendBatch(events iter.Seq[*Event]) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning append: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO events (id, type, subject, at, body) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing append: %w", err)
	}
	defer stmt.Close()

	var appendErr error
	events(func(e *Event) bool {
		if e.ID == "" {
			e.ID = s.nextID(e.At)
		}
		if err := e.Validate(); err != nil {
			appendErr = err
			return false
		}
		body, err := e.Marshal()
		if err != nil {
			appendErr = fmt.Errorf("encoding event %s: %w", e.ID, err)
			return false
		}
		res, err := stmt.Exec(e.ID, e.Type, e.Subject, e.At.UTC().Format(timeFormat), body)
		if err != nil {
			// A duplicate id means something tried to restate an existing fact.
			appendErr = fmt.Errorf("%w: event %s already exists: %w", ErrImmutable, e.ID, err)
			return false
		}
		if seq, err := res.LastInsertId(); err == nil {
			e.Seq = seq
		}
		return true
	})
	if appendErr != nil {
		return appendErr
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing append: %w", err)
	}
	return nil
}

// All returns every event in append order.
func (s *Store) All() ([]*Event, error) {
	return s.collect(`SELECT seq, body FROM events ORDER BY seq`)
}

// Since returns events appended after seq, in order. This is the projection's read path.
func (s *Store) Since(seq int64) ([]*Event, error) {
	return s.collect(`SELECT seq, body FROM events WHERE seq > ? ORDER BY seq`, seq)
}

// Subject returns every event about one subject, in order.
func (s *Store) Subject(subject string) ([]*Event, error) {
	return s.collect(`SELECT seq, body FROM events WHERE subject = ? ORDER BY seq`, subject)
}

// Count returns the number of stored events.
func (s *Store) Count() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM events`).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting events: %w", err)
	}
	return n, nil
}

func (s *Store) collect(query string, args ...any) ([]*Event, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	defer rows.Close()

	var out []*Event
	for rows.Next() {
		var seq int64
		var body []byte
		if err := rows.Scan(&seq, &body); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		e, err := Unmarshal(body)
		if err != nil {
			return nil, fmt.Errorf("decoding event at seq %d: %w", seq, err)
		}
		e.Seq = seq
		out = append(out, e)
	}
	return out, rows.Err()
}

// idEncoding is Crockford-ish base32 without padding: case-insensitive, and it sorts
// lexically in the same order as the bytes it encodes.
var idEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// nextID builds a 26-character identifier from a millisecond timestamp and 80 random
// bits, in the ULID layout. Time-ordered ids mean append order and id order agree,
// so replay needs no separate sort, and two clones can merge by sorting on id alone —
// which is what makes a git-ref home possible later without changing this format.
func (s *Store) nextID(at time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var buf [16]byte
	ms := at.UTC().UnixMilli()
	if ms < 0 {
		ms = 0
	}
	binary.BigEndian.PutUint64(buf[:8], uint64(ms)<<16)
	if _, err := rand.Read(buf[6:]); err != nil {
		// crypto/rand does not fail in practice; if it ever does, a duplicate id
		// would corrupt the log, so refuse rather than continue with weak entropy.
		panic(fmt.Sprintf("event: reading randomness for id: %v", err))
	}
	id := idEncoding.EncodeToString(buf[:])

	// Events appended within the same millisecond must still order deterministically.
	if id <= s.last {
		id = increment(s.last)
	}
	s.last = id
	return id
}

// increment returns the next id in lexical order after prev.
func increment(prev string) string {
	b := []byte(prev)
	alphabet := "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for i := len(b) - 1; i >= 0; i-- {
		pos := indexByte(alphabet, b[i])
		if pos < len(alphabet)-1 {
			b[i] = alphabet[pos+1]
			return string(b)
		}
		b[i] = alphabet[0]
	}
	return string(b)
}

func indexByte(s string, c byte) int {
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return 0
}

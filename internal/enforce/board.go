package enforce

import (
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/projection"
)

// Board is re-exported so callers need not import projection.
type Board = projection.Board

// SaveBoard records a named query and grouping key.
//
// Boards are state, not policy: they are created and discarded as people's attention
// moves, so they live in the log rather than canon.yaml. The query inside one is
// still checked against the schema at save time, so a board cannot outlive the field
// it filters on without someone noticing.
func (e *Enforcer) SaveBoard(p Principal, name, rawQuery, groupBy string, validate func(string) error, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("a board needs a name")
	}
	if _, exists := e.view.Board(name); exists {
		return fmt.Errorf("board %q already exists", name)
	}
	if validate != nil {
		if err := validate(rawQuery); err != nil {
			return fmt.Errorf("board %q: %w", name, err)
		}
	}
	if groupBy == "" {
		groupBy = "state"
	}
	return e.append("board.created", name, at, p.Actor,
		map[string]any{"query": rawQuery, "group_by": groupBy})
}

// DeleteBoard removes a saved board.
func (e *Enforcer) DeleteBoard(p Principal, name string, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if _, ok := e.view.Board(name); !ok {
		return fmt.Errorf("unknown board %q", name)
	}
	return e.append("board.deleted", name, at, p.Actor, nil)
}

// Boards returns every saved board.
func (e *Enforcer) Boards() ([]*Board, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view.Boards(), nil
}

// BoardByName returns one saved board.
func (e *Enforcer) BoardByName(name string) (*Board, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	board, ok := e.view.Board(name)
	if !ok {
		return nil, fmt.Errorf("unknown board %q", name)
	}
	return board, nil
}

package ingest

import (
	"regexp"
	"strings"
)

// The ledger's format, as `.sdlc/bin/validate-plan.py` defines it. These expressions
// are the one place this package knows what the template looks like.
var (
	headingRe = regexp.MustCompile(`^##\s+([a-z]{2,6}-\d{3})\s*:\s*(.+?)\s*$`)
	fieldRe   = regexp.MustCompile(`^-\s+\*\*([A-Za-z ]+):\*\*\s*(.*)$`)
	reqRe     = regexp.MustCompile(`^-\s+\*\*(R\d+):\*\*\s*(.*)$`)
	criterion = regexp.MustCompile(`^\s+-\s+\[([ xX])\]\s+(.*)$`)
	titleRe   = regexp.MustCompile(`^#\s+(.+?)\s*$`)
)

// Increment is one entry in a repository's ledger.
type Increment struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Status string            `json:"status"`
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields,omitempty"`
	// Criteria are the acceptance criteria and whether each is ticked.
	Criteria []Criterion `json:"criteria,omitempty"`
	// Traces are the requirement ids this increment claims to satisfy.
	Traces []string `json:"traces,omitempty"`
	// DependsOn are increment ids in the same ledger.
	DependsOn []string `json:"depends_on,omitempty"`
	// Transitions is the status history, derived from the ledger's commit history.
	Transitions []Transition `json:"transitions,omitempty"`
}

// Criterion is one acceptance criterion.
type Criterion struct {
	Text string `json:"text"`
	Met  bool   `json:"met"`
}

// Transition is one status change, with the commit that made it.
//
// From is empty for the transition that created the increment.
type Transition struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to"`
	At     string `json:"at"`
	Commit string `json:"commit"`
}

// Requirement is one line of a product spec.
type Requirement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// parseLedger reads a ledger into increments, in the order they appear.
//
// Tolerant by design. This meets other people's markdown, and a parser that refuses
// a file because one increment is malformed would make Canon useless for exactly the
// repositories most worth reporting on. What it cannot read it leaves out, and
// conformance reporting (feat-037) is where that becomes visible.
func parseLedger(text string) []Increment {
	var out []Increment
	var current *Increment
	inCriteria := false
	field := ""

	flush := func() {
		if current != nil {
			out = append(out, *current)
			current = nil
		}
	}

	for _, line := range strings.Split(text, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			current = &Increment{ID: m[1], Title: m[2], Fields: map[string]string{}}
			inCriteria, field = false, ""
			continue
		}
		if current == nil {
			continue
		}

		// A section heading of any level ends the increment: everything after
		// "## Sequencing" is prose about the plan, not part of it.
		if strings.HasPrefix(line, "## ") {
			flush()
			continue
		}

		if m := fieldRe.FindStringSubmatch(line); m != nil {
			key, value := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
			inCriteria = key == "Acceptance Criteria"
			// A field's value may be on the following indented lines rather than
			// inline — Test Strategy and Acceptance Criteria are always written that
			// way. Reading only the inline part reported every increment in this
			// repository as having no test strategy, which is the kind of
			// false-positive storm that makes a conformance report worthless.
			field = strings.ToLower(strings.ReplaceAll(key, " ", "_"))
			switch key {
			case "Status":
				current.Status = strings.ToLower(value)
			case "Type":
				current.Type = strings.ToLower(value)
			case "Traces":
				current.Traces = idList(value)
			case "Dependencies":
				current.DependsOn = idList(value)
			default:
				if value != "" {
					current.Fields[field] = value
				}
			}
			continue
		}

		// Continuation: an indented line under the field just seen.
		if field != "" && strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			text := strings.TrimSpace(line)
			if !inCriteria {
				current.Fields[field] = strings.TrimSpace(current.Fields[field] + " " + strings.TrimPrefix(text, "- "))
			}
		} else if strings.TrimSpace(line) == "" {
			field = ""
		}

		if inCriteria {
			if m := criterion.FindStringSubmatch(line); m != nil {
				current.Criteria = append(current.Criteria, Criterion{
					Text: strings.TrimSpace(m[2]),
					Met:  m[1] != " ",
				})
			}
		}
	}
	flush()
	return out
}

// idList pulls ids out of a comma-separated field, ignoring "none" and prose.
func idList(value string) []string {
	if value == "" || strings.EqualFold(strings.TrimSpace(value), "none") {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.Trim(strings.TrimSpace(part), "`")
		if part == "" || strings.EqualFold(part, "none") {
			continue
		}
		out = append(out, part)
	}
	return out
}

// statuses reduces a ledger to id → status, which is all a transition needs.
func statuses(text string) map[string]string {
	out := map[string]string{}
	for _, inc := range parseLedger(text) {
		if inc.Status != "" {
			out[inc.ID] = inc.Status
		}
	}
	return out
}

// parseSpec reads a product spec: its title, its opening purpose, and its
// requirements.
func parseSpec(text string) (name, purpose string, reqs []Requirement) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if m := titleRe.FindStringSubmatch(line); m != nil {
			name = m[1]
			break
		}
	}

	// The purpose is the first real paragraph under the first section heading —
	// which in the template is Problem or Outcome. Taking the first paragraph of the
	// file instead would return the status block.
	var collecting bool
	var para []string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if collecting {
				break
			}
			collecting = true
			continue
		}
		if !collecting {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "**") {
			continue
		}
		para = append(para, trimmed)
	}
	purpose = strings.Join(para, " ")

	for _, line := range lines {
		if m := reqRe.FindStringSubmatch(line); m != nil {
			reqs = append(reqs, Requirement{ID: m[1], Text: strings.TrimSpace(m[2])})
		}
	}
	return name, purpose, reqs
}

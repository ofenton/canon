// Package schema loads and validates the organisation's issue schema.
//
// The whole organisation's schema lives in one canon.yaml, versioned in git and
// changed by pull request. There is no per-project override and no runtime interface
// for adding a field or a state — that absence is the product. Jira instances reach
// 90+ workflows and 800 custom fields because each individual addition is reasonable
// and nobody ever sees the aggregate. Here the aggregate is a diff someone approves.
package schema

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Version is the schema format this build understands.
const Version = 1

// Category groups states for reporting. Without a fixed set, "done" acquires sixteen
// spellings and no cross-team question can be answered.
type Category string

const (
	Open   Category = "open"
	Active Category = "active"
	Closed Category = "closed"
)

var categories = []Category{Open, Active, Closed}

// FieldType is the set of value types a field may hold. It is deliberately short.
type FieldType string

const (
	String FieldType = "string"
	Text   FieldType = "text"
	Enum   FieldType = "enum"
	Number FieldType = "number"
	Bool   FieldType = "bool"
	Date   FieldType = "date"
	User   FieldType = "user"
)

var fieldTypes = []FieldType{String, Text, Enum, Number, Bool, Date, User}

// State is one position in the workflow.
type State struct {
	Name             string   `yaml:"name"`
	Category         Category `yaml:"category"`
	RequiresEvidence bool     `yaml:"requires_evidence"`

	line int
}

// Transition is a permitted move between two states.
type Transition struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`

	line int
}

// Field is a value an issue may carry.
type Field struct {
	Name     string    `yaml:"name"`
	Type     FieldType `yaml:"type"`
	Required bool      `yaml:"required"`
	Values   []string  `yaml:"values"`

	line int
}

// IssueType names a set of fields. It is a view over fields, not a storage type:
// epics, stories and sub-tasks are parent/child relations, not separate entities.
type IssueType struct {
	Name   string   `yaml:"name"`
	Fields []string `yaml:"fields"`

	line int
}

// Schema is the organisation's entire issue schema.
type Schema struct {
	Version     int          `yaml:"version"`
	States      []State      `yaml:"states"`
	Transitions []Transition `yaml:"transitions"`
	Fields      []Field      `yaml:"fields"`
	IssueTypes  []IssueType  `yaml:"issue_types"`
	Roles       []Role       `yaml:"roles"`
	Hierarchy   Hierarchy    `yaml:"hierarchy"`

	states      map[string]State
	fields      map[string]Field
	transitions map[string]map[string]bool
	roles       map[string]Role
}

// Load reads and validates the schema at path.
//
// Every problem found is reported together rather than one per run: fixing a schema
// error at a time is exactly the friction that stops people improving it.
func Load(path string) (*Schema, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", path, err)
	}

	// Decode via a Node first so every element keeps its line number. An error that
	// cannot be located in the file is barely better than no error at all.
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", path, withContext(raw, locateYAMLError(err)))
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, fmt.Errorf("schema %s is empty", path)
	}

	var s Schema
	if err := doc.Content[0].Decode(&s); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", path, withContext(raw, locateYAMLError(err)))
	}
	if err := s.checkUnknownKeys(doc.Content[0], path); err != nil {
		return nil, err
	}
	s.recordLines(doc.Content[0])

	if err := s.validate(path); err != nil {
		return nil, err
	}
	s.index()
	return &s, nil
}

// checkUnknownKeys refuses keys the schema does not define.
//
// Silently ignoring an unrecognised key means a typo disables a rule without telling
// anyone, and it is how "sprints:" would quietly become someone's local extension.
func (s *Schema) checkUnknownKeys(root *yaml.Node, path string) error {
	known := map[string]bool{
		"version": true, "states": true, "transitions": true,
		"fields": true, "issue_types": true, "roles": true, "hierarchy": true,
	}
	var unknown []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if !known[key.Value] {
			unknown = append(unknown, fmt.Sprintf("%q at line %d", key.Value, key.Line))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("schema %s: unknown top-level key %s (valid keys: %s)",
			path, strings.Join(unknown, ", "), strings.Join(sortedKeys(known), ", "))
	}
	return nil
}

// recordLines attaches source line numbers to each element for error reporting.
func (s *Schema) recordLines(root *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		for j, item := range value.Content {
			switch key.Value {
			case "states":
				if j < len(s.States) {
					s.States[j].line = item.Line
				}
			case "transitions":
				if j < len(s.Transitions) {
					s.Transitions[j].line = item.Line
				}
			case "fields":
				if j < len(s.Fields) {
					s.Fields[j].line = item.Line
				}
			case "issue_types":
				if j < len(s.IssueTypes) {
					s.IssueTypes[j].line = item.Line
				}
			case "roles":
				if j < len(s.Roles) {
					s.Roles[j].line = item.Line
				}
			}
		}
		if key.Value == "hierarchy" {
			s.Hierarchy.line = value.Line
		}
	}
}

func (s *Schema) validate(path string) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if s.Version != Version {
		add("version %d is not supported; this build understands version %d", s.Version, Version)
	}
	if len(s.States) == 0 {
		add("at least one state is required")
	}

	seenState := map[string]int{}
	for _, st := range s.States {
		switch {
		case st.Name == "":
			add("line %d: state has no name", st.line)
		case seenState[st.Name] != 0:
			add("line %d: duplicate state %q, first defined at line %d",
				st.line, st.Name, seenState[st.Name])
		default:
			seenState[st.Name] = st.line
		}
		if !validCategory(st.Category) {
			add("line %d: state %q has category %q; valid categories are %s",
				st.line, st.Name, st.Category, join(categories))
		}
	}

	for _, tr := range s.Transitions {
		if _, ok := seenState[tr.From]; !ok {
			add("line %d: transition %s -> %s refers to undefined state %q",
				tr.line, tr.From, tr.To, tr.From)
		}
		if _, ok := seenState[tr.To]; !ok {
			add("line %d: transition %s -> %s refers to undefined state %q",
				tr.line, tr.From, tr.To, tr.To)
		}
	}

	seenField := map[string]int{}
	for _, f := range s.Fields {
		switch {
		case f.Name == "":
			add("line %d: field has no name", f.line)
		case seenField[f.Name] != 0:
			add("line %d: duplicate field %q, first defined at line %d",
				f.line, f.Name, seenField[f.Name])
		default:
			seenField[f.Name] = f.line
		}
		if !validFieldType(f.Type) {
			add("line %d: field %q has type %q; valid types are %s",
				f.line, f.Name, f.Type, join(fieldTypes))
		}
		if f.Type == Enum && len(f.Values) == 0 {
			add("line %d: enum field %q must list its values", f.line, f.Name)
		}
		if f.Type != Enum && len(f.Values) > 0 {
			add("line %d: field %q is not an enum but lists values", f.line, f.Name)
		}
	}

	seenType := map[string]int{}
	for _, it := range s.IssueTypes {
		switch {
		case it.Name == "":
			add("line %d: issue type has no name", it.line)
		case seenType[it.Name] != 0:
			add("line %d: duplicate issue type %q, first defined at line %d",
				it.line, it.Name, seenType[it.Name])
		default:
			seenType[it.Name] = it.line
		}
		for _, name := range it.Fields {
			if _, ok := seenField[name]; !ok {
				add("line %d: issue type %q references undefined field %q",
					it.line, it.Name, name)
			}
		}
	}

	// Roles reference fields and states, so index them before checking grants.
	s.index()
	s.validateRoles(add)
	s.validateHierarchy(add)

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("schema %s has %d problem(s):\n  - %s",
		path, len(problems), strings.Join(problems, "\n  - "))
}

func (s *Schema) index() {
	s.states = make(map[string]State, len(s.States))
	for _, st := range s.States {
		s.states[st.Name] = st
	}
	s.fields = make(map[string]Field, len(s.Fields))
	for _, f := range s.Fields {
		s.fields[f.Name] = f
	}
	s.roles = make(map[string]Role, len(s.Roles))
	for _, r := range s.Roles {
		s.roles[r.Name] = r
	}
	s.transitions = map[string]map[string]bool{}
	for _, tr := range s.Transitions {
		if s.transitions[tr.From] == nil {
			s.transitions[tr.From] = map[string]bool{}
		}
		s.transitions[tr.From][tr.To] = true
	}
}

// HasState reports whether name is a defined state.
func (s *Schema) HasState(name string) bool { _, ok := s.states[name]; return ok }

// HasField reports whether name is a defined field.
func (s *Schema) HasField(name string) bool { _, ok := s.fields[name]; return ok }

// Field returns the definition of a field.
func (s *Schema) Field(name string) (Field, bool) { f, ok := s.fields[name]; return f, ok }

// CanTransition reports whether from -> to is permitted.
func (s *Schema) CanTransition(from, to string) bool { return s.transitions[from][to] }

// RequiresEvidence reports whether entering a state demands evidence.
func (s *Schema) RequiresEvidence(state string) bool { return s.states[state].RequiresEvidence }

// PermittedFrom lists the states reachable from one state, sorted. Enforcement uses
// this to tell a caller what it should have done, rather than only that it was wrong.
func (s *Schema) PermittedFrom(state string) []string {
	out := make([]string, 0, len(s.transitions[state]))
	for to := range s.transitions[state] {
		out = append(out, to)
	}
	sort.Strings(out)
	return out
}

// FieldNames lists every defined field, sorted.
func (s *Schema) FieldNames() []string {
	out := make([]string, 0, len(s.fields))
	for name := range s.fields {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// withContext appends the offending region of the file to a parse error.
//
// yaml.v3 reports the line where a block began, not always the line that broke it:
// a bad indent on line 4 is reported against the sequence starting on line 2. A bare
// line number therefore sends a reader to the wrong place. Printing the region with
// line numbers makes the problem findable regardless of the parser's precision.
func withContext(raw []byte, err error) error {
	match := yamlLine.FindStringSubmatch(err.Error())
	if match == nil {
		return err
	}
	at, convErr := strconv.Atoi(match[1])
	if convErr != nil {
		return err
	}

	lines := strings.Split(string(raw), "\n")
	const window = 3
	start := max(at-1, 1)
	end := min(at+window, len(lines))

	var b strings.Builder
	fmt.Fprintf(&b, "%v\n\n  the parser stopped at line %d; the problem is at or just after it:\n", err, at)
	for i := start; i <= end; i++ {
		marker := "  "
		if i == at {
			marker = "->"
		}
		fmt.Fprintf(&b, "  %s %4d | %s\n", marker, i, lines[i-1])
	}
	return errors.New(b.String())
}

// locateYAMLError makes yaml.v3's type errors carry their line numbers up front.
func locateYAMLError(err error) error {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return errors.New(strings.Join(typeErr.Errors, "; "))
	}
	return err
}

func validCategory(c Category) bool {
	for _, known := range categories {
		if c == known {
			return true
		}
	}
	return false
}

func validFieldType(t FieldType) bool {
	for _, known := range fieldTypes {
		if t == known {
			return true
		}
	}
	return false
}

func join[T ~string](values []T) string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return strings.Join(out, ", ")
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

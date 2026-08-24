package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Hierarchy declares which issue types may nest inside which, as ordered levels.
//
// Nesting is a property of *types*, not of individual issues. An epic contains
// features, a feature contains stories, a story contains tasks and bugs. Expressing
// that as an ordering rather than as a per-issue rule buys two things: several types
// can share a level naturally, and cycles become impossible by construction, because
// a child's level is always strictly greater than its parent's.
//
// Declaring it is what makes nesting legal at all. A schema with no hierarchy has no
// answer to "may this contain that", and guessing would mean inventing policy the
// organisation has not agreed.
type Hierarchy struct {
	// Levels lists issue types from outermost to innermost.
	Levels [][]string `yaml:"levels"`
	// AllowSkipping permits a parent more than one level above the child — an epic
	// directly containing a story. Organisations genuinely differ here, so it is
	// theirs to decide rather than ours; strict is the default.
	AllowSkipping bool `yaml:"allow_skipping"`

	// levelOf maps an issue type to its zero-based depth.
	levelOf map[string]int
	line    int
}

// Declared reports whether the schema constrains nesting at all.
func (s *Schema) HierarchyDeclared() bool { return len(s.Hierarchy.Levels) > 0 }

// LevelOf returns an issue type's depth, and whether it is placed at all.
func (s *Schema) LevelOf(issueType string) (int, bool) {
	level, ok := s.Hierarchy.levelOf[issueType]
	return level, ok
}

// CanNest reports whether a child of one type may sit directly under a parent of
// another, and explains why not when it may not.
func (s *Schema) CanNest(childType, parentType string) error {
	if !s.HierarchyDeclared() {
		return fmt.Errorf("this schema declares no hierarchy, so nothing may be nested; add a hierarchy block to canon.yaml to permit it")
	}
	childLevel, ok := s.LevelOf(childType)
	if !ok {
		return fmt.Errorf("issue type %q is not placed in the hierarchy", childType)
	}
	parentLevel, ok := s.LevelOf(parentType)
	if !ok {
		return fmt.Errorf("issue type %q is not placed in the hierarchy", parentType)
	}

	switch {
	case s.Hierarchy.AllowSkipping && childLevel > parentLevel:
		return nil
	case !s.Hierarchy.AllowSkipping && childLevel == parentLevel+1:
		return nil
	}

	permitted := s.ParentTypesFor(childType)
	if len(permitted) == 0 {
		return fmt.Errorf("%s is at the top of the hierarchy and cannot have a parent", childType)
	}
	return fmt.Errorf("%s %s cannot sit under %s %s; %s %s may only sit under %s",
		article(childType), childType, article(parentType), parentType,
		article(childType), childType, strings.Join(permitted, " or "))
}

// ParentTypesFor lists the issue types that may contain one, sorted.
func (s *Schema) ParentTypesFor(childType string) []string {
	childLevel, ok := s.LevelOf(childType)
	if !ok || childLevel == 0 {
		return nil
	}
	var out []string
	if s.Hierarchy.AllowSkipping {
		for level := 0; level < childLevel; level++ {
			out = append(out, s.Hierarchy.Levels[level]...)
		}
	} else {
		out = append(out, s.Hierarchy.Levels[childLevel-1]...)
	}
	sort.Strings(out)
	return out
}

// ChildTypesFor lists the issue types one may contain, sorted.
func (s *Schema) ChildTypesFor(parentType string) []string {
	parentLevel, ok := s.LevelOf(parentType)
	if !ok {
		return nil
	}
	var out []string
	if s.Hierarchy.AllowSkipping {
		for level := parentLevel + 1; level < len(s.Hierarchy.Levels); level++ {
			out = append(out, s.Hierarchy.Levels[level]...)
		}
	} else if parentLevel+1 < len(s.Hierarchy.Levels) {
		out = append(out, s.Hierarchy.Levels[parentLevel+1]...)
	}
	sort.Strings(out)
	return out
}

// validateHierarchy checks the declaration against the issue types.
//
// Every issue type must be placed exactly once. A type left out would be silently
// unnestable — a rule nobody wrote and nobody could find — and a type in two levels
// has no defined depth.
func (s *Schema) validateHierarchy(add func(string, ...any)) {
	s.Hierarchy.levelOf = map[string]int{}
	if !s.HierarchyDeclared() {
		return
	}

	declared := map[string]bool{}
	for _, it := range s.IssueTypes {
		declared[it.Name] = true
	}

	for level, names := range s.Hierarchy.Levels {
		if len(names) == 0 {
			add("line %d: hierarchy level %d is empty", s.Hierarchy.line, level+1)
			continue
		}
		for _, name := range names {
			if !declared[name] {
				add("line %d: hierarchy names %q, which is not a defined issue type; defined types are %s",
					s.Hierarchy.line, name, strings.Join(s.issueTypeNames(), ", "))
				continue
			}
			if _, dup := s.Hierarchy.levelOf[name]; dup {
				add("line %d: issue type %q appears in more than one hierarchy level",
					s.Hierarchy.line, name)
				continue
			}
			s.Hierarchy.levelOf[name] = level
		}
	}

	var missing []string
	for _, it := range s.IssueTypes {
		if _, placed := s.Hierarchy.levelOf[it.Name]; !placed {
			missing = append(missing, it.Name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		add("line %d: every issue type must appear in the hierarchy; %s not placed",
			s.Hierarchy.line, strings.Join(missing, ", "))
	}
}

func (s *Schema) issueTypeNames() []string {
	out := make([]string, 0, len(s.IssueTypes))
	for _, it := range s.IssueTypes {
		out = append(out, it.Name)
	}
	sort.Strings(out)
	return out
}

// article picks a or an. These messages are the product's voice, and "a epic" reads
// as carelessness in exactly the place a user is already annoyed.
func article(word string) string {
	if word == "" {
		return "a"
	}
	switch word[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an"
	}
	return "a"
}

package tracejob

import (
	"fmt"
	"strings"
)

// ProcessSelector represents a selector-like label query to select the target process
// within the container namespace.
type ProcessSelector struct {
	// label -> value, values cannot contain spaces.
	// Assumes equality match, will be changed when more operators added.
	terms map[string]string
}

// NewProcessSelector will construct a selector by parsing the query.
func NewProcessSelector(query string) (*ProcessSelector, error) {
	s := &ProcessSelector{
		terms: map[string]string{},
	}

	query = strings.TrimSpace(query)
	if len(query) == 0 {
		return s, nil
	}

	terms := strings.Split(query, ",")
	for _, t := range terms {
		seg := strings.Split(t, "=")
		if len(seg) != 2 {
			return nil, fmt.Errorf("invalid term in selector at %s", t)
		}

		label, value := strings.TrimSpace(seg[0]), strings.TrimSpace(seg[1])
		s.terms[label] = value
	}

	return s, nil
}

func (s *ProcessSelector) String() string {
	elems := []string{}
	for k, v := range s.terms {
		elems = append(elems, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(elems, ",")
}

func (s *ProcessSelector) Pid() (string, bool) {
	return s.get("pid")
}

func (s *ProcessSelector) Exe() (string, bool) {
	return s.get("exe")
}

func (s *ProcessSelector) Comm() (string, bool) {
	return s.get("comm")
}

func (s *ProcessSelector) Cmdline() (string, bool) {
	return s.get("cmdline")
}

func (s *ProcessSelector) get(label string) (string, bool) {
	value, ok := s.terms[label]
	return value, ok
}

package tracejob

import (
	"fmt"
	"strings"
)

// Selector represents a label query to select the entity to be traced.
// Most commonly this entity would resolve into a host, pod or container.
type Selector struct {
	// label -> value, values cannot contain spaces.
	// Assumes equality match, will be changed when more operators added.
	terms map[string]string
}

// NewSelector will construct a selector by parsing the query.
func NewSelector(query string) (*Selector, error) {
	s := &Selector{
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

// Get a label by name.
func (s *Selector) Get(label string) (string, bool) {
	value, ok := s.terms[label]
	return value, ok
}

// Set a label by name.
// Returns the value just set.
func (s *Selector) Set(label, value string) string {
	s.terms[label] = value
	return value
}

// String converts selector back to a query.
func (s *Selector) String() string {
	terms := []string{}
	for k, v := range s.terms {
		terms = append(terms, k+"="+v)
	}
	return strings.Join(terms, ",")
}

func (s *Selector) Node() (string, bool) {
	return s.Get("node")
}

func (s *Selector) Pod() (string, bool) {
	return s.Get("pod")
}

func (s *Selector) PodUID() (string, bool) {
	return s.Get("pod-uid")
}

func (s *Selector) Container() (string, bool) {
	return s.Get("container")
}

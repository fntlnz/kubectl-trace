package tracejob

import (
	"sort"
	"strings"
	"testing"

	. "github.com/go-check/check"
)

type SelectorSuite struct{}

func init() {
	Suite(&SelectorSuite{})
}

func Test(t *testing.T) { TestingT(t) }

// sorted will output a query like selector.String() but with terms in order.
func sorted(s *Selector) string {
	terms := sort.StringSlice{}
	for k, v := range s.terms {
		terms = append(terms, k+"="+v)
	}
	terms.Sort()
	return strings.Join(terms, ",")
}

func (s *SelectorSuite) TestNewSlice(c *C) {
	parsed, err := NewSelector("foo=bar, abc=xyz")
	c.Assert(err, IsNil)
	c.Assert(sorted(parsed), Equals, "abc=xyz,foo=bar")
}

func (s *SelectorSuite) TestNewSliceError(c *C) {
	// Would do a table test if more than 3 cases.
	_, err := NewSelector("pod=hello-world node=minikube")
	c.Assert(err, NotNil)

	_, err = NewSelector("pod=hello-world,, node=minikube")
	c.Assert(err, NotNil)
}

func (s *SelectorSuite) TestString(c *C) {
	expected := &Selector{
		terms: map[string]string{
			"foo": "bar",
			"abc": "xyz",
		},
	}

	parsed, _ := NewSelector("foo=bar, abc=xyz")
	c.Assert(parsed, DeepEquals, expected)
}

func (s *SelectorSuite) TestNode(c *C) {
	parsed, _ := NewSelector("pod=hello-world, node=minikube")
	val, ok := parsed.Node()
	c.Assert(ok, Equals, true)
	c.Assert(val, Equals, "minikube")
}

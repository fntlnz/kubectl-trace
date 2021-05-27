package tracejob

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSelector(t *testing.T) {
	expected := &ProcessSelector{
		terms: map[string]string{
			"pid":  "last",
			"comm": "foobar",
		},
	}

	parsed, _ := NewProcessSelector("comm=foobar, pid=last")
	assert.Equal(t, expected, parsed)
}

func TestNewSelectorError(t *testing.T) {
	_, err := NewProcessSelector("pid=last comm=foobar")
	assert.NotNil(t, err)
	_, err = NewProcessSelector("pid=last,, comm=foobar")
	assert.NotNil(t, err)
}

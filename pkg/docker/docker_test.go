package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseImageNameOnlyName(t *testing.T) {
	imageName := "testing"
	expected := Image{
		Name: "testing",
	}

	actual, err := ParseImageName(imageName)
	assert.Nil(t, err)
	assert.Equal(t, &expected, actual)
}

func TestParseImageNameWithNameTag(t *testing.T) {
	imageName := "testing:latest"
	expected := Image{
		Name: "testing",
		Tag:  "latest",
	}

	actual, err := ParseImageName(imageName)
	assert.Nil(t, err)
	assert.Equal(t, &expected, actual)
}

func TestParseImageNameWithRepositoryNameTag(t *testing.T) {
	imageName := "weird/testing:latest"
	expected := Image{
		Repository: "weird",
		Name:       "testing",
		Tag:        "latest",
	}

	actual, err := ParseImageName(imageName)
	assert.Nil(t, err)
	assert.Equal(t, &expected, actual)
}

func TestParseImageNameHostnameRepositoryNameTag(t *testing.T) {
	imageName := "quay.io/weird/testing:latest"
	expected := Image{
		Hostname:   "quay.io",
		Repository: "weird",
		Name:       "testing",
		Tag:        "latest",
	}

	actual, err := ParseImageName(imageName)
	assert.Nil(t, err)
	assert.Equal(t, &expected, actual)
}

func TestInvalidImageNames(t *testing.T) {
	invalid := []string{
		"https://example.com/nope/sorry:whatever",
		"http://example.com/nope/sorry:whatever",
		"some/big/long/name",
		"another:with:invalid:tags",
	}
	for _, name := range invalid {
		parsed, err := ParseImageName(name)
		assert.Nil(t, parsed)
		assert.NotNil(t, err)
	}
}

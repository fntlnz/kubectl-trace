package downloader

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTarDirectory(t *testing.T) {
	b := &bytes.Buffer{}
	err := TarDirectory(b, "testdata/test_tar_directory")
	assert.Nil(t, err)

	files := []tar.Header{}
	reader := tar.NewReader(b)
	for {
		hdr, err := reader.Next()
		if err == io.EOF {
			break
		}
		assert.Nil(t, err)

		files = append(files, tar.Header{
			Name: hdr.Name,
			Size: hdr.Size,
		})
	}

	expected := []tar.Header{
		{
			Name: "test_tar_directory/another/baz",
			Size: 12,
		},
		{
			Name: "test_tar_directory/bar",
			Size: 4,
		},
		{
			Name: "test_tar_directory/foo",
			Size: 0,
		},
	}

	assert.EqualValues(t, expected, files)
}

func TestTarDirectoryDoesNotExist(t *testing.T) {
	b := &bytes.Buffer{}
	err := TarDirectory(b, "does-not-exist")
	assert.NotNil(t, err)
}

package upload

import (
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
)

func TestUpload(t *testing.T) {
	s, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		Scheme:         "http",
		InitialObjects: []fakestorage.Object{},
	})
	assert.Nil(t, err)
	defer s.Stop()

	s.CreateBucket("upload-test-bucket")

	g, err := NewGcsUploader(GcsUploaderOptions{
		Endpoint: s.URL(),
		NoAuth:   true,
	})
	assert.Nil(t, err)

	err = g.Upload("./testdata/gcs_upload_test", "gs://upload-test-bucket/_output/traces")
	assert.Nil(t, err)

	objects, _, err := s.ListObjects("upload-test-bucket", "_output/traces", "", false)
	assert.Nil(t, err)

	files := []string{}
	for _, o := range objects {
		files = append(files, o.Name)
	}

	expected := []string{
		"_output/traces/metadata.json",
		"_output/traces/options.json",
	}
	assert.Equal(t, expected, files)
}

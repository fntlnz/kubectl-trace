package upload

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GcsUploader handles uploading metadata output to google cloud storage.
type GcsUploader struct {
	client *storage.Client
}

// GcsUploaderOptions are used to customize GcsUploader instance.
type GcsUploaderOptions struct {
	// Endpoint for GCS server if it needs to be overridden.
	Endpoint string

	// NoAuth disables authentication for easier testing.
	NoAuth bool
}

// NewGcsUploader constructs a GcsUploader.
func NewGcsUploader(opts GcsUploaderOptions) (*GcsUploader, error) {
	var err error

	clientOpts := []option.ClientOption{}
	if len(opts.Endpoint) > 0 {
		clientOpts = append(clientOpts, option.WithEndpoint(opts.Endpoint))
	}
	if opts.NoAuth {
		clientOpts = append(clientOpts, option.WithoutAuthentication())
	}

	client, err := storage.NewClient(context.Background(), clientOpts...)
	if err != nil {
		return nil, err
	}

	return &GcsUploader{
		client: client,
	}, nil
}

// Upload everything at metaDir to bucketURL.
func (g *GcsUploader) Upload(metaDir, bucketURL string) error {
	gsurl, err := url.Parse(bucketURL)
	if err != nil {
		return err
	}

	if len(gsurl.Host) == 0 {
		return fmt.Errorf("no bucket specified in output url")
	}

	bucket := g.client.Bucket(gsurl.Host)

	// GCS handles an object path of /foo/bar by displaying a directory named "/"
	// in the root of the bucket. This is not what we want.
	outDir := strings.TrimPrefix(gsurl.Path, "/")

	err = filepath.Walk(metaDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		relPath, err := filepath.Rel(metaDir, filePath)
		if err != nil {
			return err
		}

		writer := bucket.Object(path.Join(outDir, relPath)).NewWriter(context.Background())
		_, err = io.Copy(writer, file)
		if err != nil {
			return err
		}

		err = writer.Close()
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

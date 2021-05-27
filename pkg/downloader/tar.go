package downloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mholt/archiver/v3"
)

// TarDirectory writes a tar with all files rooted at path.
func TarDirectory(w io.Writer, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	t := archiver.NewTar()
	err = t.Create(w)
	if err != nil {
		return err
	}
	defer t.Close()

	baseDir := filepath.Dir(path)
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		return t.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				FileInfo:   info,
				CustomName: relPath,
			},
			ReadCloser: file,
		})
	})
}

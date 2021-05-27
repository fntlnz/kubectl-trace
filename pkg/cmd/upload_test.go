package cmd

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/uuid"
)

// This is a technique used in go stdlib (exec_test.go) and documented
// here https://npf.io/2015/06/testing-exec-command/
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestUpload(t *testing.T) {
	pid := fmt.Sprintf("pid.%s", uuid.NewUUID())
	cmd := fakeExecCommand("trace-uploader", "--pid="+pid, "--out=testdata/test_upload")

	b := &bytes.Buffer{}
	cmd.Stdout = b

	err := cmd.Start()
	assert.Nil(t, err)

	// Wait on pid file to be written
	for count := 3; count > 0; count-- {
		var stat os.FileInfo
		stat, err = os.Stat(pid)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		assert.False(t, stat.IsDir())
	}
	assert.Nil(t, err)

	err = cmd.Process.Signal(os.Interrupt)
	assert.Nil(t, err)
	os.Remove(pid)

	err = cmd.Wait()
	assert.Nil(t, err)

	// Proxy for verifying that testdata/test_upload was included in generated tarball.
	assert.Contains(t, b.String(), "this-file-was-included-in-tarball")

	hdr, err := tar.NewReader(b).Next()
	assert.Nil(t, err)
	assert.Equal(t, hdr.Name, "test_upload/metadata.json")
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// We are executing trace-uploader command but through the cmd.test binary
	// so remove cmd.test and its args so that flag parsing works correctly.
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = os.Args[i+1:]
			break
		}
	}

	err := NewUploadCommand().Execute()
	if err != nil {
		panic(err)
	}
	os.Exit(0)
}

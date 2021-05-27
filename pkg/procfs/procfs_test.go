package procfs

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestFindPidsForContainerFindsTheContainer(t *testing.T) {
	_ = setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc", 0755))
	assert.Nil(t, ProcFs.MkdirAll("/proc/1/ns", 0755))
	assert.Nil(t, symlink(ProcFs, "/pid:[0000000001]", "/proc/1/ns/pid"))

	pids, err := FindPidsForContainer("1")

	assert.Nil(t, err)
	assert.Equal(t, []string{"1"}, pids)
}

func TestFindPidsForContainerFindsTheCorrectPids(t *testing.T) {
	_ = setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc/1/ns", 0755))
	assert.Nil(t, symlink(ProcFs, "/pid:[0000000001]", "/proc/1/ns/pid"))

	assert.Nil(t, ProcFs.MkdirAll("/proc/2/ns", 0755))
	assert.Nil(t, symlink(ProcFs, "/pid:[0000000001]", "/proc/2/ns/pid"))

	assert.Nil(t, ProcFs.MkdirAll("/proc/3/ns", 0755))
	assert.Nil(t, symlink(ProcFs, "/pid:[0000000001]", "/proc/3/ns/pid"))

	assert.Nil(t, ProcFs.MkdirAll("/proc/10/ns", 0755))
	assert.Nil(t, symlink(ProcFs, "/pid:[1010101010]", "/proc/10/ns/pid"))

	pids, err := FindPidsForContainer("1")
	assert.Nil(t, err)

	sort.Strings(pids)
	assert.Equal(t, []string{"1", "2", "3"}, pids)
}

func TestGetFinalNamespacePid(t *testing.T) {
	_ = setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc/47/ns", 0755))
	data := []byte("Name:	testprogram\n NStgid:	47	1\n NSpid:	47	1\n NSpgid:	47	1\n NSsid:	47	1\n")
	assert.Nil(t, afero.WriteFile(ProcFs, "/proc/47/status", data, 0444))

	pid, err := GetFinalNamespacePid("47")
	assert.Nil(t, err)

	assert.Equal(t, "1", pid)
}

func TestGetProcComm(t *testing.T) {
	_ = setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc/42", 0755))
	data := []byte("ruby")
	assert.Nil(t, afero.WriteFile(ProcFs, "/proc/42/comm", data, 0444))

	comm, err := GetProcComm("42")
	assert.Nil(t, err)

	assert.Equal(t, "ruby", comm)
}

func TestGetProcCmdline(t *testing.T) {
	_ = setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc/42", 0755))
	data := []byte("Rails uri_path=/foo/bar request_id=1234")
	assert.Nil(t, afero.WriteFile(ProcFs, "/proc/42/cmdline", data, 0444))

	cmdline, err := GetProcCmdline("42")
	assert.Nil(t, err)

	assert.Equal(t, "Rails uri_path=/foo/bar request_id=1234", cmdline)
}

func TestGetProcExe(t *testing.T) {
	basePath := setupBasePath(t)

	assert.Nil(t, ProcFs.MkdirAll("/proc/42", 0755))
	assert.Nil(t, symlink(ProcFs, "/usr/local/bin/ruby", "/proc/42/exe"))

	exe, err := GetProcExe("42")
	assert.Nil(t, err)

	expected := path.Join(basePath, "/usr/local/bin/ruby")
	assert.Equal(t, expected, exe)
}

func setupBasePath(t *testing.T) string {
	tempDir, err := ioutil.TempDir("", "example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir) // clean up

	ProcFs = afero.NewBasePathFs(afero.NewOsFs(), tempDir)
	assert.Nil(t, ProcFs.MkdirAll("/proc", 0755))

	return tempDir
}

func symlink(fs afero.Fs, oldname string, newname string) error {
	if r, ok := fs.(afero.Linker); ok {
		return r.SymlinkIfPossible(oldname, newname)
	}

	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
}

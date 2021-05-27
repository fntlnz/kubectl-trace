package integration

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	batchv1 "k8s.io/api/batch/v1"
)

var outputDownloadedPattern = regexp.MustCompile(`downloaded (?P<tarfile>kubectl-trace-[^.]+\.tar)`)

type outputAsserter func(string, []byte)

func (k *KubectlTraceSuite) TestRunNode() {
	nodeName := k.GetTestNode()
	bpftraceProgram := `kprobe:do_sys_open { printf("%s: %s\n", comm, str(arg1)) }'`
	out := k.KubectlTraceCmd("run", "--imagename="+k.RunnerImage(), "-e", bpftraceProgram, nodeName)
	assert.Regexp(k.T(), "trace (\\w+-){4}\\w+ created", out)
}

func (k *KubectlTraceSuite) TestRunRbspyProcPid() {
	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	selectors := []string{"pid=1"}

	out := k.KubectlTraceCmd(
		"run",
		"pod/ruby",
		"--target-namespace="+TraceJobsTargetNamespace,
		"--tracer=rbspy",
		"--deadline=5",
		"--imagename="+k.RunnerImage(),
		"--process-selector="+strings.Join(selectors, ","),
		"--output="+outputDir)

	lookFor := `Writing formatted output to (.*?)profile\.speedscope\.json`
	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		assert.Regexp(k.T(), lookFor, string(contents))
	})
}

func (k *KubectlTraceSuite) TestRunRbspyPostProcess() {
	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	selectors := []string{"pid=1"}

	out := k.KubectlTraceCmd(
		"run",
		"pod/ruby",
		"--target-namespace="+TraceJobsTargetNamespace,
		"--tracer=rbspy",
		"--deadline=5",
		"--imagename="+k.RunnerImage(),
		"--process-selector="+strings.Join(selectors, ","),
		"--output="+outputDir)

	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		flamePath := path.Join(outputDir, "flamegraph.svg")
		_, err = os.Stat(flamePath)
		assert.False(k.T(), os.IsNotExist(err))
	})
}

func (k *KubectlTraceSuite) TestProcessSelectorChoosesHighestPid() {
	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	selectors := []string{"pid=last", "exe=ruby"}
	out := k.KubectlTraceCmd(
		"run",
		"pod/ruby",
		"--target-namespace="+TraceJobsTargetNamespace,
		"--tracer=fake",
		"--program=pidtrace",
		"--args=$target_pid",
		"--imagename="+k.RunnerImage(),
		"--process-selector="+strings.Join(selectors, ","),
		"--output="+outputDir)

	re := regexp.MustCompile(`nspid: (?P<pid>[0-9]+)`)
	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		matches := re.FindSubmatch(contents)
		assert.Equal(k.T(), len(matches), 2)
		pidIndex := re.SubexpIndex("pid")
		assert.Less(k.T(), pidIndex, len(matches))
		matchedPid := matches[pidIndex]
		pid, err := strconv.Atoi(string(matchedPid))
		assert.Nil(k.T(), err)
		assert.Greater(k.T(), pid, 1)
	})
}

func (k *KubectlTraceSuite) TestProcessSelectors() {
	processSelectors := []string{
		"pid=1",
		"pid=last,exe=ruby",
		"pid=last,comm=ruby",
		"pid=last,cmdline=fork-from-args",
		"pid=last,exe=ruby,cmdline=first",
	}

	lookFors := []string{
		"nspid: 1",
		"cmdline: second",
		"cmdline: second",
		"nspid: 1",
		"cmdline: first",
	}

	for i, s := range processSelectors {
		k.runProcessSelectorTest(s, lookFors[i])
	}
}

func (k *KubectlTraceSuite) runProcessSelectorTest(processSelector string, lookFor string) {
	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	out := k.KubectlTraceCmd(
		"run",
		"pod/ruby",
		"--target-namespace="+TraceJobsTargetNamespace,
		"--tracer=fake",
		"--program=pidtrace",
		"--args=$target_pid",
		"--imagename="+k.RunnerImage(),
		"--process-selector="+processSelector,
		"--output="+outputDir)

	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		assert.Regexp(k.T(), regexp.MustCompile(lookFor), string(contents))
	})
}

func (k *KubectlTraceSuite) TestDownloadOutput() {
	nodeName := k.GetTestNode()

	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	out := k.KubectlTraceCmd("run", "node/"+nodeName, "--tracer=fake", "--process-selector=pid=last", "--imagename="+k.RunnerImage(), "--output="+outputDir, "--program=output")
	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		// do nothing
	})
}

func (k *KubectlTraceSuite) TestReturnErrOnErr() {
	nodeName := k.GetTestNode()

	out := k.KubectlTraceCmd("run", "node/"+nodeName, "--tracer=fake", "--process-selector=pid=last", "--imagename="+k.RunnerImage(), "--output=stdout", "--program=failure")
	assert.Regexp(k.T(), regexp.MustCompile("trace [a-f0-9-]{36} created"), out)

	var job batchv1.Job

	for {
		jobs := k.GetJobs().Items
		assert.Equal(k.T(), 1, len(jobs))

		job = jobs[0]
		if len(job.Status.Conditions) > 0 {
			break // on the first condition
		}

		time.Sleep(1 * time.Second)
	}

	assert.Equal(k.T(), 1, len(job.Status.Conditions))
	assert.Equal(k.T(), "Failed", string(job.Status.Conditions[0].Type))
	assert.Equal(k.T(), int32(0), job.Status.Succeeded, "No jobs in the batch should have succeeded")
	assert.Greater(k.T(), job.Status.Failed, int32(1), "There should be at least one failed job")
}

func (k *KubectlTraceSuite) TestDownloadTeedOutput() {
	outputDir, err := ioutil.TempDir("", "kubectl-trace-output-download")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(outputDir) // clean up

	nodeName := k.GetTestNode()
	out := k.KubectlTraceCmd("run", "node/"+nodeName, "--tracer=fake", "--process-selector=pid=last", "--imagename="+k.RunnerImage(), "--output="+outputDir+"/", "--program=output")

	lookFor := "trace-uploader pid found at /var/run/trace-uploader"
	k.assertDownloadedOutput(out, outputDir, func(outputDir string, contents []byte) {
		assert.Regexp(k.T(), regexp.MustCompile(lookFor), string(contents))
	})
}

func (k *KubectlTraceSuite) assertDownloadedOutput(out string, outputDir string, assertOutput outputAsserter) {
	assert.Regexp(k.T(), outputDownloadedPattern, out)

	matches := outputDownloadedPattern.FindSubmatch([]byte(out))
	tarIdx := outputDownloadedPattern.SubexpIndex("tarfile")
	tarfile := string(matches[tarIdx])
	info, err := os.Stat(path.Join(outputDir, tarfile))
	assert.Nil(k.T(), err)

	// If a local output path is provided then intermediate directories might be created.
	// Make sure that the tooling that creates those directories does not create one
	// with the name of the tarball.
	assert.False(k.T(), info.IsDir())
	assert.Greater(k.T(), info.Size(), int64(0))

	tempDir, err := ioutil.TempDir("", "kubectl-trace-integration-untar")
	assert.Nil(k.T(), err)
	defer os.RemoveAll(tempDir)

	err = untar(path.Join(outputDir, tarfile), tempDir)
	assert.Nil(k.T(), err)

	outDir := path.Join(tempDir, "kubectl-trace")
	stdoutLogPath := path.Join(outDir, "stdout.log")
	_, err = os.Stat(stdoutLogPath)
	assert.False(k.T(), os.IsNotExist(err))

	contents, err := ioutil.ReadFile(stdoutLogPath)
	assert.Nil(k.T(), err)
	assertOutput(outDir, contents)
}

func untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		os.MkdirAll(filepath.Dir(path), 0766) // FIXME error check
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

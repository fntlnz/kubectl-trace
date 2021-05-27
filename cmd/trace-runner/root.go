package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"

	"github.com/iovisor/kubectl-trace/pkg/cmd"
	"github.com/spf13/pflag"
)

func main() {
	writeMetadata()

	flags := pflag.NewFlagSet("trace-runner", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewTraceRunnerCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func writeMetadata() {
	hostname, err := os.Hostname()
	check(err)

	metadata := &struct {
		Host string   `json:"pod"`
		Args []string `json:"args"`
	}{
		Host: hostname,
		Args: os.Args,
	}

	bytes, err := json.Marshal(metadata)
	check(err)

	err = os.MkdirAll(cmd.MetadataDir, 0644)
	check(err)

	err = ioutil.WriteFile(path.Join(cmd.MetadataDir, "metadata.json"), bytes, 0644)
	check(err)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

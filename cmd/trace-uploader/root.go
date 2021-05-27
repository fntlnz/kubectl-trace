package main

import (
	"os"

	"github.com/iovisor/kubectl-trace/pkg/cmd"
	"github.com/spf13/pflag"
)

func main() {
	flags := pflag.NewFlagSet("trace-uploader", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewUploadCommand()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

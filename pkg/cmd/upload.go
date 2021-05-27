package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"

	"github.com/iovisor/kubectl-trace/pkg/downloader"
	"github.com/spf13/cobra"
)

var (
	pidRequiredArgErrString = "pid is a required argument"
	outRequiredArgErrString = "out is a required argument"
)

// UploadOptions ...
type UploadOptions struct {
	pidFile string
	outDir  string
}

// NewUploadOptions provides an instance of UploadOptions with default values.
func NewUploadOptions() *UploadOptions {
	return &UploadOptions{}
}

// NewUploadCommand povides the upload command wrapping UploadOptions.
func NewUploadCommand() *cobra.Command {
	o := NewUploadOptions()

	cmd := &cobra.Command{
		Use: "trace-uploader --pid PIDFILE --out OUTDIR",
		PreRunE: func(c *cobra.Command, args []string) error {
			return o.Validate(c, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Run(); err != nil {
				fmt.Println(err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&o.pidFile, "pid", o.pidFile, "File to write uploader pid to")
	cmd.Flags().StringVar(&o.outDir, "out", o.outDir, "Directory with tracer output and metadata")
	return cmd
}

func (o *UploadOptions) Validate(c *cobra.Command, args []string) error {
	if !c.Flag("pid").Changed {
		return fmt.Errorf(pidRequiredArgErrString)
	}
	if !c.Flag("out").Changed {
		return fmt.Errorf(outRequiredArgErrString)
	}

	return nil
}

func (o *UploadOptions) Run() error {
	pid := os.Getpid()
	o.stderrf("uploader started with pid:%v\n", pid)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	err := ioutil.WriteFile(o.pidFile, []byte(strconv.Itoa(pid)), 0644)
	if err != nil {
		return err
	}
	o.stderrf("pid written to %v\n", o.pidFile)

	o.stderrf("waiting for SIGINT to start uplaod...")
	<-sigCh

	err = downloader.TarDirectory(os.Stdout, o.outDir)
	if err != nil {
		return err
	}

	return nil
}

func (o *UploadOptions) stderrf(format string, a ...interface{}) (int, error) {
	return fmt.Fprintf(os.Stderr, format, a...)
}

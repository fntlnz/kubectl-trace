package cmd

import (
	"bytes"
	"context"
	"fmt"
	"github.com/iovisor/kubectl-trace/pkg/flamegraph"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/fntlnz/mountinfo"
	"github.com/spf13/cobra"
)

type TraceRunnerOptions struct {
	podUID                  string
	containerName           string
	inPod                   bool
	programPath             string
	bpftraceBinaryPath      string
	flameGraphBinaryPath    string
	stackCollapseBinaryPath string
	flameGraph              bool
	flameGraphOutputPath    string
}

func NewTraceRunnerOptions() *TraceRunnerOptions {
	return &TraceRunnerOptions{}
}

func NewTraceRunnerCommand() *cobra.Command {
	o := NewTraceRunnerOptions()
	cmd := &cobra.Command{
		PreRunE: func(c *cobra.Command, args []string) error {
			return o.Validate(c, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				fmt.Fprintln(os.Stdout, err.Error())
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.containerName, "container", "c", o.containerName, "Specify the container")
	cmd.Flags().StringVarP(&o.podUID, "poduid", "p", o.podUID, "Specify the pod UID")
	cmd.Flags().StringVarP(&o.programPath, "program", "f", "program.bt", "Specify the bpftrace program path")
	cmd.Flags().StringVarP(&o.bpftraceBinaryPath, "bpftracebinary", "b", "/bin/bpftrace", "Specify the bpftrace binary path")
	cmd.Flags().StringVar(&o.flameGraphBinaryPath, "flamegraphbinary", "/bin/flamegraph", "Specify the flamegraph generator binary path")
	cmd.Flags().StringVar(&o.stackCollapseBinaryPath, "stackcollapsebinary", "/bin/stackcollapse-bpftrace", "Specify the stackcollapse-bpftrace binary path (used for Flame Graphs)")
	cmd.Flags().BoolVar(&o.inPod, "inpod", false, "Whether or not run this bpftrace in a pod's container process namespace")
	cmd.Flags().BoolVar(&o.flameGraph, "flamegraph", false, "When true, generate and save a Flame Graph (only works with stack, kstack and ustack)")
	cmd.Flags().StringVar(&o.flameGraphOutputPath, "flamegraph-output-path", "/tmp/flamegraph.svg", "Where to save the generated flamegraph")
	return cmd
}

func (o *TraceRunnerOptions) Validate(cmd *cobra.Command, args []string) error {
	// TODO(fntlnz): do some more meaningful validation here, for now just checking if they are there
	if o.inPod == true && (len(o.containerName) == 0 || len(o.podUID) == 0) {
		return fmt.Errorf("poduid and container must be specified when inpod=true")
	}
	return nil
}

// Complete completes the setup of the command.
func (o *TraceRunnerOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *TraceRunnerOptions) Run() error {
	programPath := o.programPath
	if o.inPod == true {
		pid, err := findPidByPodContainer(o.podUID, o.containerName)
		if err != nil {
			return err
		}
		if pid == nil {
			return fmt.Errorf("pid not found")
		}
		if len(*pid) == 0 {
			return fmt.Errorf("invalid pid found")
		}
		f, err := ioutil.ReadFile(programPath)
		if err != nil {
			return err
		}
		programPath = path.Join(os.TempDir(), "program-container.bt")
		r := strings.Replace(string(f), "$container_pid", *pid, -1)
		if err := ioutil.WriteFile(programPath, []byte(r), 0755); err != nil {
			return err
		}
	}

	fmt.Println("if your program has maps to print, send a SIGINT using Ctrl-C, if you want to interrupt the execution send SIGINT two times")
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Signal(syscall.SIGINT))

	go func() {
		killable := false
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				if !killable {
					killable = true
					fmt.Println("\nfirst SIGINT received, now if your program had maps and did not free them it should print them out")
					continue
				}
				return
			}
		}
	}()

	c := exec.CommandContext(ctx, o.bpftraceBinaryPath, programPath)

	var stackBuf io.ReadWriter
	stackBuf = os.Stdout

	if o.flameGraph {
		stackBuf = new(bytes.Buffer)
	}

	c.Stdout = stackBuf
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return err
	}
	if o.flameGraph {
		if err := generateFlameGraph(ctx,
			stackBuf,
			o.stackCollapseBinaryPath,
			o.flameGraphBinaryPath,
			o.flameGraphOutputPath,
		); err != nil {
			return err
		}
	}
	return nil
}

func generateFlameGraph(ctx context.Context,
	stackBuf io.ReadWriter,
	stackCollapseBinaryPath string,
	flameGraphBinaryPath string,
	flameGraphOutputPath string) error {
	fg := flamegraph.New(stackCollapseBinaryPath, flameGraphBinaryPath)

	fgbuf, err := fg.Generate(ctx, stackBuf)

	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(fgbuf)
	if err := ioutil.WriteFile("/dev/stdout", buf.Bytes(), 0644); err != nil {
		return err
	}
	return nil
}

func findPidByPodContainer(podUID, containerName string) (*string, error) {
	d, err := os.Open("/proc")

	if err != nil {
		return nil, err
	}

	defer d.Close()

	for {
		dirs, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, di := range dirs {
			if !di.IsDir() {
				continue
			}
			dname := di.Name()
			if dname[0] < '0' || dname[0] > '9' {
				continue
			}

			mi, err := mountinfo.GetMountInfo(path.Join("/proc", dname, "mountinfo"))
			if err != nil {
				continue
			}

			for _, m := range mi {
				root := m.Root
				if strings.Contains(root, podUID) && strings.Contains(root, containerName) {
					return &dname, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no process found for specified pod and container")
}

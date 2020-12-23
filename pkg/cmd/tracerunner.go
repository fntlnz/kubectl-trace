package cmd

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/fntlnz/mountinfo"
	"github.com/iovisor/kubectl-trace/pkg/tracejob"
	"github.com/spf13/cobra"
)

const (
	bpftrace = "bpftrace"
	bcc      = "bcc"
)

var (
	bpfTraceBinaryPath = "/usr/bin/bpftrace"
)

type TraceRunnerOptions struct {
	// The tracing system to use.
	// tracer = bpftrace | bcc | perf
	tracer string

	// Selector (label query) that identifies entity to be traced.
	// selector = label '=' value [',' labelN '=' valueN ...]
	// Currently supported labels:
	// - node
	//   Select a node by name
	// - pod
	//   Select a pod by name
	// - pod-uid
	//   Select a pod by UID
	// - container
	//   Select a container by name
	selector string

	// Where will the tracing system send output.
	// output = stdout | file:///path | URI
	output string

	// In the case of bcc the name of the bcc program to execute.
	// In the case of bpftrace the path to contents of the user provided expression or program.
	program string

	// In the case of bcc the user provided arguments to pass on to program.
	// Not used for bpftrace.
	programArgs string

	// Values populated after validation
	parsedSelector *tracejob.Selector
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

	cmd.Flags().StringVar(&o.tracer, "tracer", "bpftrace", "Tracing system to use")
	cmd.Flags().StringVar(&o.selector, "selector", "", "Selector (label query) to filter on")
	cmd.Flags().StringVar(&o.output, "output", "stdout", "Where will the tracing system send output")
	cmd.Flags().StringVar(&o.program, "program", "/programs/program.bt", "Tracer input script or executable")
	cmd.Flags().StringVar(&o.programArgs, "program-args", o.programArgs, "Arguments to pass through to executable in --program")
	return cmd
}

func (o *TraceRunnerOptions) Validate(cmd *cobra.Command, args []string) error {
	switch o.tracer {
	case bpftrace, bcc:
	default:
		return fmt.Errorf("unknown tracer %s", o.tracer)
	}

	switch o.output {
	case "stdout":
	default:
		return fmt.Errorf("unknown output %s", o.output)
	}

	parsed, err := tracejob.NewSelector(o.selector)
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	o.parsedSelector = parsed

	_, podOk := parsed.PodUID()
	_, containerOk := parsed.Container()
	if podOk != containerOk {
		return fmt.Errorf("selector must specify both podUID and container")
	}

	return nil
}

// Complete completes the setup of the command.
func (o *TraceRunnerOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *TraceRunnerOptions) Run() error {
	var err error
	var binary, args *string
	switch o.tracer {
	case bpftrace:
		binary, args, err = o.prepBpfTraceCommand()
	case bcc:
		binary, args, err = o.prepBccCommand()
	}

	if err != nil {
		return err
	}

	// Assume output is stdout until other backends are implemented.

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

	var c *exec.Cmd
	if args == nil || len(*args) == 0 {
		c = exec.CommandContext(ctx, *binary)
	} else {
		c = exec.CommandContext(ctx, *binary, *args)
	}

	c.Stdout = os.Stdout
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr
	return c.Run()
}

func (o *TraceRunnerOptions) prepBpfTraceCommand() (*string, *string, error) {
	programPath := o.program

	// Render $container_pid to actual process pid if scoped to container.
	podUID, _ := o.parsedSelector.PodUID()
	container, ok := o.parsedSelector.Container()
	if ok {
		pid, err := findPidByPodContainer(podUID, container)
		if err != nil {
			return nil, nil, err
		}
		f, err := ioutil.ReadFile(programPath)
		if err != nil {
			return nil, nil, err
		}
		programPath = path.Join(os.TempDir(), "program-container.bt")
		r := strings.Replace(string(f), "$container_pid", *pid, -1)
		if err := ioutil.WriteFile(programPath, []byte(r), 0755); err != nil {
			return nil, nil, err
		}
	}

	return &bpfTraceBinaryPath, &programPath, nil
}

func (o *TraceRunnerOptions) prepBccCommand() (*string, *string, error) {
	return nil, nil, fmt.Errorf("tracer bcc not implemented")
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

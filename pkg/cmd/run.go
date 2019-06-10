package cmd

import (
	"context"
	"fmt"
	"github.com/iovisor/kubectl-trace/pkg/cp"
	"io/ioutil"

	"github.com/iovisor/kubectl-trace/pkg/attacher"
	"github.com/iovisor/kubectl-trace/pkg/factory"
	"github.com/iovisor/kubectl-trace/pkg/meta"
	"github.com/iovisor/kubectl-trace/pkg/signals"
	"github.com/iovisor/kubectl-trace/pkg/tracejob"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var (
	// ImageNameTag represents the default tracerunner image
	ImageNameTag = "quay.io/iovisor/kubectl-trace-bpftrace:latest"
	// InitImageNameTag represents the default init container image
	InitImageNameTag = "quay.io/iovisor/kubectl-trace-init:latest"
)

var (
	runShort = `Execute a bpftrace program on resources` // Wrap with i18n.T()

	runLong = runShort

	runExamples = `
  # Count system calls using tracepoints on a specific node
  %[1]s trace run node/kubernetes-node-emt8.c.myproject.internal -e 'kprobe:do_sys_open { printf("%%s: %%s\n", comm, str(arg1)) }'

  # Execute a bpftrace program from file on a specific node
  %[1]s trace run node/kubernetes-node-emt8.c.myproject.internal -f read.bt

  # Run an bpftrace inline program on a pod container
  %[1]s trace run pod/nginx -c nginx -e "tracepoint:syscalls:sys_enter_* { @[probe] = count(); }"
  %[1]s trace run pod/nginx nginx -e "tracepoint:syscalls:sys_enter_* { @[probe] = count(); }"

  # Run a bpftrace inline program on a pod container with a custom image for the init container responsible to fetch linux headers 
  %[1]s trace run pod/nginx nginx -e "tracepoint:syscalls:sys_enter_* { @[probe] = count(); } --init-imagename=quay.io/custom-init-image-name --fetch-headers"

  # Run a bpftrace inline program on a pod container with a custom image for the bpftrace container that will run your program in the cluster
  %[1]s trace run pod/nginx nginx -e "tracepoint:syscalls:sys_enter_* { @[probe] = count(); } --imagename=quay.io/custom-bpftrace-image-name"`

	runCommand                    = "run"
	usageString                   = "(POD | TYPE/NAME)"
	requiredArgErrString          = fmt.Sprintf("%s is a required argument for the %s command", usageString, runCommand)
	containerAsArgOrFlagErrString = "specify container inline as argument or via its flag"
	bpftraceMissingErrString      = "the bpftrace program is mandatory"
	bpftraceDoubleErrString       = "specify the bpftrace program either via an external file or via a literal string, not both"
	bpftraceEmptyErrString        = "the bpftrace programm cannot be empty"
)

// RunOptions ...
type RunOptions struct {
	genericclioptions.IOStreams

	namespace         string
	explicitNamespace bool

	// Flags local to this command
	container      string
	eval           string
	program        string
	serviceAccount string
	imageName      string
	initImageName  string
	fetchHeaders   bool
	flameGraph     bool

	resourceArg string
	attach      bool
	isPod       bool
	podUID      string
	nodeName    string

	clientConfig *rest.Config
}

// NewRunOptions provides an instance of RunOptions with default values.
func NewRunOptions(streams genericclioptions.IOStreams) *RunOptions {
	return &RunOptions{
		IOStreams: streams,

		serviceAccount: "default",
		imageName:      ImageNameTag,
		initImageName:  InitImageNameTag,
	}
}

// NewRunCommand provides the run command wrapping RunOptions.
func NewRunCommand(factory factory.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRunOptions(streams)

	cmd := &cobra.Command{
		Use:          fmt.Sprintf("%s %s [-c CONTAINER] [--attach]", runCommand, usageString),
		Short:        runShort,
		Long:         runLong,                             // Wrap with templates.LongDesc()
		Example:      fmt.Sprintf(runExamples, "kubectl"), // Wrap with templates.Examples()
		SilenceUsage: true,
		PreRunE: func(c *cobra.Command, args []string) error {
			return o.Validate(c, args)
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(factory, c, args); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				fmt.Fprintln(o.ErrOut, err.Error())
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.container, "container", "c", o.container, "Specify the container")
	cmd.Flags().BoolVarP(&o.attach, "attach", "a", o.attach, "Whether or not to attach to the trace program once it is created")
	cmd.Flags().StringVarP(&o.eval, "eval", "e", o.eval, "Literal string to be evaluated as a bpftrace program")
	cmd.Flags().StringVarP(&o.program, "filename", "f", o.program, "File containing a bpftrace program")
	cmd.Flags().StringVar(&o.serviceAccount, "serviceaccount", o.serviceAccount, "Service account to use to set in the pod spec of the kubectl-trace job")
	cmd.Flags().StringVar(&o.imageName, "imagename", o.imageName, "Custom image for the tracerunner")
	cmd.Flags().StringVar(&o.initImageName, "init-imagename", o.initImageName, "Custom image for the init container responsible to fetch and prepare linux headers")
	cmd.Flags().BoolVar(&o.fetchHeaders, "fetch-headers", o.fetchHeaders, "Whether to fetch linux headers or not")
	cmd.Flags().BoolVar(&o.flameGraph, "flamegraph", o.flameGraph, "Generate and save a Flame Graph (only works with stack, kstack and ustack), must be used with attach.")

	return cmd
}

// Validate validates the arguments and flags populating RunOptions accordingly.
func (o *RunOptions) Validate(cmd *cobra.Command, args []string) error {
	containerFlagDefined := cmd.Flag("container").Changed
	switch len(args) {
	case 1:
		o.resourceArg = args[0]
		break
	// 2nd argument interpreted as container when provided
	case 2:
		o.resourceArg = args[0]
		o.container = args[1]
		if containerFlagDefined {
			return fmt.Errorf(containerAsArgOrFlagErrString)
		}
		break
	default:
		return fmt.Errorf(requiredArgErrString)
	}

	if !cmd.Flag("eval").Changed && !cmd.Flag("filename").Changed {
		return fmt.Errorf(bpftraceMissingErrString)
	}
	if cmd.Flag("eval").Changed == cmd.Flag("filename").Changed {
		return fmt.Errorf(bpftraceDoubleErrString)
	}
	if (cmd.Flag("eval").Changed && len(o.eval) == 0) || (cmd.Flag("filename").Changed && len(o.program) == 0) {
		return fmt.Errorf(bpftraceEmptyErrString)
	}

	if o.flameGraph && !o.attach {
		return fmt.Errorf("a FlameGraph can be generated only in attach mode")
	}

	return nil
}

// Complete completes the setup of the command.
func (o *RunOptions) Complete(factory factory.Factory, cmd *cobra.Command, args []string) error {
	// Prepare program
	if len(o.program) > 0 {
		b, err := ioutil.ReadFile(o.program)
		if err != nil {
			return fmt.Errorf("error opening program file")
		}
		o.program = string(b)
	} else {
		o.program = o.eval
	}

	// Prepare namespace
	var err error
	o.namespace, o.explicitNamespace, err = factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	// Look for the target object
	x := factory.
		NewBuilder().
		WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
		NamespaceParam(o.namespace).
		SingleResourceType().
		ResourceNames("nodes", o.resourceArg). // Search nodes by default
		Do()

	obj, err := x.Object()
	if err != nil {
		return err
	}

	// Check we got a pod or a node
	o.isPod = false

	var node *v1.Node

	switch v := obj.(type) {
	case *v1.Pod:
		if len(v.Spec.NodeName) == 0 {
			return fmt.Errorf("cannot attach a trace program to a pod that is not currently scheduled on a node")
		}
		o.isPod = true
		found := false
		o.podUID = string(v.UID)
		for _, c := range v.Spec.Containers {
			// default if no container provided
			if len(o.container) == 0 {
				o.container = c.Name
				found = true
				break
			}
			// check if the provided one exists
			if c.Name == o.container {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("no containers found for the provided pod/container combination")
		}

		obj, err = factory.
			NewBuilder().
			WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
			ResourceNames("nodes", v.Spec.NodeName).
			Do().Object()

		if err != nil {
			return err
		}

		if n, ok := obj.(*v1.Node); ok {
			node = n
		}

		break
	case *v1.Node:
		node = v
		break
	default:
		return fmt.Errorf("first argument must be %s", usageString)
	}

	if node == nil {
		return fmt.Errorf("could not determine on which node to run the trace program")
	}

	labels := node.GetLabels()
	val, ok := labels["kubernetes.io/hostname"]
	if !ok {
		return fmt.Errorf("label kubernetes.io/hostname not found in node")
	}
	o.nodeName = val

	// Prepare client
	o.clientConfig, err = factory.ToRESTConfig()
	if err != nil {
		return err
	}

	return nil
}

// Run executes the run command.
func (o *RunOptions) Run() error {
	juid := uuid.NewUUID()
	jobsClient, err := batchv1client.NewForConfig(o.clientConfig)
	if err != nil {
		return err
	}

	coreClient, err := corev1client.NewForConfig(o.clientConfig)
	if err != nil {
		return err
	}

	tc := &tracejob.TraceJobClient{
		JobClient:    jobsClient.Jobs(o.namespace),
		ConfigClient: coreClient.ConfigMaps(o.namespace),
	}

	tj := tracejob.TraceJob{
		Name:             fmt.Sprintf("%s%s", meta.ObjectNamePrefix, string(juid)),
		Namespace:        o.namespace,
		ServiceAccount:   o.serviceAccount,
		ID:               juid,
		Hostname:         o.nodeName,
		Program:          o.program,
		PodUID:           o.podUID,
		ContainerName:    o.container,
		IsPod:            o.isPod,
		ImageNameTag:     o.imageName,
		InitImageNameTag: o.initImageName,
		FetchHeaders:     o.fetchHeaders,
		FlameGraph:       o.flameGraph,
	}

	job, err := tc.CreateJob(tj)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.IOStreams.Out, "trace %s created\n", tj.ID)

	if o.attach {
		ctx := context.Background()
		ctx = signals.WithStandardSignals(ctx)
		a := attacher.NewAttacher(coreClient, o.clientConfig, o.IOStreams)
		a.WithContext(ctx)
		a.AttachJob(tj.ID, job.Namespace)
	}

	if o.flameGraph {
		src := cp.FileSpec{
			PodNamespace: job.Namespace,
			PodName:      "",
			File:         "/tmp/flamegraph.svg",
		}
		dest := cp.FileSpec{
			PodNamespace: job.Namespace,
			PodName:      "",
			File:         "/tmp/copiedflamegraph.svg",
		}
		if err := cp.CopyFromPod(src, dest); err != nil {
			return err
		}
	}

	return nil
}

package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/iovisor/kubectl-trace/pkg/attacher"
	"github.com/iovisor/kubectl-trace/pkg/downloader"
	"github.com/iovisor/kubectl-trace/pkg/meta"
	"github.com/iovisor/kubectl-trace/pkg/signals"
	"github.com/iovisor/kubectl-trace/pkg/tracejob"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	// ImageName represents the default tracerunner image
	ImageName = "quay.io/iovisor/kubectl-trace-runner"
	// ImageTag represents the tag to fetch for ImageName
	ImageTag = "latest"
	// InitImageName represents the default init container image
	InitImageName = "quay.io/iovisor/kubectl-trace-init"
	// InitImageTag represents the tag to fetch for InitImage
	InitImageTag = "latest"
	// DefaultDeadline is the maximum time a tracejob is allowed to run, in seconds
	DefaultDeadline = 3600
	// DefaultDeadlineGracePeriod is the maximum time to wait to print a map or histogram, in seconds
	// note that it must account for startup time, as the deadline as based on start time
	DefaultDeadlineGracePeriod = 30
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

	tracerNotFound                   = "unknown tracer %s"
	tracerNeededForSelectorErrString = "tracer must be specified when specifying selector"
	tracerNeededForOutputErrString   = "tracer must be specified when specifying output"
	containerAsSelectorErrString     = "container must be specified in selector when specifying tracer"
	resourceAsSelectorErrString      = "node/pod must be specified in selector when specifying tracer"

	pidProcessSelectorRequiredForTracer = "a pid process selector must be specified for tracer %s"
)

// RunOptions ...
type RunOptions struct {
	genericclioptions.IOStreams

	namespace         string
	explicitNamespace bool

	// Flags local to this command
	eval     string
	filename string

	// Flags for generic interface
	// See TraceRunnerOptions for definitions.
	// TODO: clean this up
	tracer          string
	targetNamespace string
	processSelector string
	program         string
	programArgs     []string
	output          string
	tracerDefined   bool
	parsedSelector  *tracejob.ProcessSelector

	googleAppSecret     string
	serviceAccount      string
	imageName           string
	initImageName       string
	fetchHeaders        bool
	deadline            int64
	deadlineGracePeriod int64

	resourceArg string
	container   string

	attach   bool
	download bool

	clientConfig *rest.Config
}

// NewRunOptions provides an instance of RunOptions with default values.
func NewRunOptions(streams genericclioptions.IOStreams) *RunOptions {
	return &RunOptions{
		IOStreams: streams,

		serviceAccount:      "default",
		imageName:           ImageName + ":" + ImageTag,
		initImageName:       InitImageName + ":" + InitImageTag,
		deadline:            int64(DefaultDeadline),
		deadlineGracePeriod: int64(DefaultDeadlineGracePeriod),
	}
}

// NewRunCommand provides the run command wrapping RunOptions.
func NewRunCommand(factory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
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

	// flags for existing usage
	cmd.Flags().StringVarP(&o.container, "container", "c", o.container, "Specify the container")
	cmd.Flags().StringVarP(&o.eval, "eval", "e", o.eval, "Literal string to be evaluated as a bpftrace program")
	cmd.Flags().StringVarP(&o.filename, "filename", "f", o.filename, "File containing a bpftrace program")

	// flags for new generic interface
	cmd.Flags().StringVar(&o.tracer, "tracer", "bpftrace", "Tracing system to use")
	cmd.Flags().StringVar(&o.targetNamespace, "target-namespace", "", "Namespace in which the target pod exists (if applicable). Defaults to the namespace argument passed to kubectl.")
	cmd.Flags().StringVar(&o.processSelector, "process-selector", "", "Process Selector (similar to a label query) to filter on")
	cmd.Flags().StringVar(&o.output, "output", "stdout", "Where to send tracing output (stdout or local path)")
	cmd.Flags().StringVar(&o.program, "program", o.program, "Program to execute")
	cmd.Flags().StringArrayVar(&o.programArgs, "args", o.programArgs, "Additional arguments to pass on to program, repeat flag for multiple arguments")

	// global flags
	cmd.Flags().StringVar(&o.googleAppSecret, "google-application-secret", o.googleAppSecret, "A secret containing a JSON formatted google service account key, for GOOGLE_APPLICATION_CREDENITALS. Used for GCS support.")
	cmd.Flags().BoolVarP(&o.attach, "attach", "a", o.attach, "Whether or not to attach to the trace program once it is created")
	cmd.Flags().StringVar(&o.serviceAccount, "serviceaccount", o.serviceAccount, "Service account to use to set in the pod spec of the kubectl-trace job")
	cmd.Flags().StringVar(&o.imageName, "imagename", o.imageName, "Custom image for the tracerunner")
	cmd.Flags().StringVar(&o.initImageName, "init-imagename", o.initImageName, "Custom image for the init container responsible to fetch and prepare linux headers")
	cmd.Flags().BoolVar(&o.fetchHeaders, "fetch-headers", o.fetchHeaders, "Whether to fetch linux headers or not")
	cmd.Flags().Int64Var(&o.deadline, "deadline", o.deadline, "Maximum time to allow trace to run in seconds")
	cmd.Flags().Int64Var(&o.deadlineGracePeriod, "deadline-grace-period", o.deadlineGracePeriod, "Maximum wait time to print maps or histograms after deadline, in seconds")

	return cmd
}

// Validate validates the arguments and flags populating RunOptions accordingly.
func (o *RunOptions) Validate(cmd *cobra.Command, args []string) error {
	// Selector can only be used in conjunction with tracer.
	o.tracerDefined = cmd.Flag("tracer").Changed
	if !o.tracerDefined && cmd.Flag("process-selector").Changed {
		return fmt.Errorf(tracerNeededForSelectorErrString)
	}
	if !o.tracerDefined && cmd.Flag("output").Changed {
		return fmt.Errorf(tracerNeededForOutputErrString)
	}

	switch o.tracer {
	case bpftrace, bcc, rbspy, fake:
	default:
		return fmt.Errorf(tracerNotFound, o.tracer)
	}

	parsed, err := tracejob.NewProcessSelector(o.processSelector)
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	o.parsedSelector = parsed

	err = validateSelectorForTracer(o.tracer, o.parsedSelector)
	if err != nil {
		return err
	}

	// All filtering must be via selector if tracer is specified.
	containerFlagDefined := cmd.Flag("container").Changed
	if o.tracerDefined && containerFlagDefined {
		return fmt.Errorf(containerAsSelectorErrString)
	}

	//if !o.tracerDefined {
	switch len(args) {
	case 1:
		o.resourceArg = args[0]
		break
	// 2nd argument interpreted as container when provided
	case 2:
		o.resourceArg = args[0]
		o.container = args[1] // NOTE: this should actually be -c, to be consistent with the rest of kubectl
		if containerFlagDefined {
			return fmt.Errorf(containerAsArgOrFlagErrString)
		}
		break
	default:
		return fmt.Errorf(requiredArgErrString)
	}

	if len(o.output) == 0 {
		return fmt.Errorf("output cannot be empty when specified")
	}

	switch {
	case o.output == "stdout":
	case o.output[0] == '/' || o.output[0] == '.':
		o.download = true
	case strings.HasPrefix(o.output, "gs://"):
	default:
		return fmt.Errorf("unknown output %s", o.output)
	}

	switch o.tracer {
	case bpftrace, bcc:
		evalDefined, filenameDefined, programDefined := cmd.Flag("eval").Changed, cmd.Flag("filename").Changed, cmd.Flag("program").Changed
		if !evalDefined && !filenameDefined && !programDefined {
			return fmt.Errorf(bpftraceMissingErrString)
		}
		if (evalDefined && filenameDefined) || (evalDefined && programDefined) || (filenameDefined && programDefined) {
			return fmt.Errorf(bpftraceDoubleErrString)
		}
		if (evalDefined && len(o.eval) == 0) || (filenameDefined && len(o.program) == 0) || (programDefined && len(o.program) == 0) {
			return fmt.Errorf(bpftraceEmptyErrString)
		}
	default:
	}

	return nil
}

// Complete completes the setup of the command.
func (o *RunOptions) Complete(factory cmdutil.Factory, cmd *cobra.Command, args []string) error {
	// Prepare program
	if len(o.program) == 0 {
		if len(o.filename) > 0 {
			b, err := ioutil.ReadFile(o.filename)
			if err != nil {
				return fmt.Errorf("error opening program file")
			}
			o.program = string(b)
		} else {
			o.program = o.eval
		}
	}

	// Prepare namespaces
	var err error
	o.namespace, o.explicitNamespace, err = factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	// If a target namespace is not specified explicitly, we want to reuse the
	// namespace as understood by kubectl.
	if o.targetNamespace == "" {
		o.targetNamespace = o.namespace
	}

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

	clientset, err := kubernetes.NewForConfig(o.clientConfig)
	if err != nil {
		return err
	}

	target, err := tracejob.ResolveTraceJobTarget(clientset, o.resourceArg, o.container, o.targetNamespace)

	if err != nil {
		return err
	}

	tc := tracejob.NewTraceJobClient(clientset, o.namespace)

	tj := tracejob.TraceJob{
		Name:           fmt.Sprintf("%s%s", meta.ObjectNamePrefix, string(juid)),
		Namespace:      o.namespace,
		ServiceAccount: o.serviceAccount,
		ID:             juid,
		Target:         *target,
		// FIXME find a way to pass remaining members of trace job target
		Tracer:              o.tracer,
		Selector:            o.processSelector,
		Output:              o.output,
		Program:             o.program,
		ProgramArgs:         o.programArgs,
		GoogleAppSecret:     o.googleAppSecret,
		ImageNameTag:        o.imageName,
		InitImageNameTag:    o.initImageName,
		FetchHeaders:        o.fetchHeaders,
		Deadline:            o.deadline,
		DeadlineGracePeriod: o.deadlineGracePeriod,
	}

	job, err := tc.CreateJob(tj)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.IOStreams.Out, "trace %s created\n", tj.ID)

	if o.download {
		if o.attach {
			go o.waitOnDownload(tj, clientset.CoreV1())
		} else {
			fmt.Fprintln(o.IOStreams.Out, "waiting for trace to be downloaded")
			o.waitOnDownload(tj, clientset.CoreV1())
		}
	}

	if o.attach {
		ctx := context.Background()
		ctx = signals.WithStandardSignals(ctx)
		a := attacher.NewAttacher(clientset.CoreV1(), o.clientConfig, o.IOStreams)
		a.WithContext(ctx)
		a.AttachJob(tj.ID, job.Namespace)
	}

	return nil
}

func (o *RunOptions) waitOnDownload(tj tracejob.TraceJob, coreClient corev1client.CoreV1Interface) {
	d := downloader.New(coreClient, o.clientConfig)
	err := d.Start(tj.ID, tj.Namespace, o.output, MetadataDir)
	if err != nil {
		fmt.Fprintf(o.IOStreams.ErrOut, "[downloader] %s\n", err.Error())
		return
	}
	fmt.Fprintf(o.IOStreams.Out, "downloaded %v\n", downloader.Filename(tj.ID))
}

func validateSelectorForTracer(tracer string, selector *tracejob.ProcessSelector) error {
	switch tracer {
	case bcc, bpftrace, fake:
	case rbspy:
		if _, ok := selector.Pid(); !ok {
			return fmt.Errorf(pidProcessSelectorRequiredForTracer, rbspy)
		}
	default:
	}

	return nil
}

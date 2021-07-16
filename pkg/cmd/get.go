package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/iovisor/kubectl-trace/pkg/meta"
	"github.com/iovisor/kubectl-trace/pkg/tracejob"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var (
	getCommand = "get"
	getShort   = `Get the running traces` // Wrap with i18n.T()
	getLong    = getShort

	getExamples = `
  # Get all traces in a namespace
  %[1]s trace get -n myns

  # Get only a specific trace
  %[1]s trace get 656ee75a-ee3c-11e8-9e7a-8c164500a77e

  # Get only a specific trace in a specific namespace
  %[1]s trace get 656ee75a-ee3c-11e8-9e7a-8c164500a77e -n myns

  # Get all traces in all namespaces
  %[1]s trace get --all-namespaces`

	argumentsErr     = fmt.Sprintf("at most one argument for %s command", getCommand)
	missingTargetErr = fmt.Sprintf("specify either a TRACE_ID or a namespace or all namespaces")
)

// GetOptions ...
type GetOptions struct {
	genericclioptions.IOStreams
	ResourceBuilderFlags *genericclioptions.ResourceBuilderFlags

	namespace string

	// Local to this command
	allNamespaces bool
	traceArg      string
	clientConfig  *rest.Config
	traceID       *types.UID
	traceName     *string
}

// NewGetOptions provides an instance of GetOptions with default values.
func NewGetOptions(streams genericclioptions.IOStreams) *GetOptions {
	rbFlags := &genericclioptions.ResourceBuilderFlags{}
	rbFlags.WithAllNamespaces(false)

	return &GetOptions{
		ResourceBuilderFlags: rbFlags,
		IOStreams:            streams,
	}
}

// NewGetCommand provides the get command wrapping GetOptions.
func NewGetCommand(factory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewGetOptions(streams)

	cmd := &cobra.Command{
		Use:          fmt.Sprintf("%s (TRACE_ID | TRACE_NAME)", getCommand),
		Short:        getShort,
		Long:         getLong,                             // wrap with templates.LongDesc()
		Example:      fmt.Sprintf(getExamples, "kubectl"), // wrap with templates.Examples()
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

	o.ResourceBuilderFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *GetOptions) Validate(cmd *cobra.Command, args []string) error {
	switch len(args) {
	case 1:
		if meta.IsObjectName(args[0]) {
			o.traceName = &args[0]
		} else {
			tid := types.UID(args[0])
			o.traceID = &tid
		}
		break
	}

	return nil
}

// Complete completes the setup of the command.
func (o *GetOptions) Complete(factory cmdutil.Factory, cmd *cobra.Command, args []string) error {
	// Prepare namespace
	var err error
	o.namespace, _, err = factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	// All namespaces, when present, overrides namespace flag
	if cmd.Flag("all-namespaces").Changed {
		o.allNamespaces = *o.ResourceBuilderFlags.AllNamespaces
		o.namespace = ""
	}

	// Prepare client
	o.clientConfig, err = factory.ToRESTConfig()
	if err != nil {
		return err
	}

	return nil
}

func (o *GetOptions) Run() error {
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

	tc.WithOutStream(o.Out)

	tf := tracejob.TraceJobFilter{
		Name: o.traceName,
		ID:   o.traceID,
	}

	jobs, err := tc.GetJob(tf)

	if err != nil {
		return err
	}

	// TODO: support other output formats via the o flag, like json, yaml. Not sure if a good idea, trace is not a resource in k8s
	jobsTablePrint(o.Out, jobs)
	return nil
}

// TODO(fntlnz): This needs better printing, perhaps we could use the humanreadable table from k8s itself
// to be consistent with the main project.
func jobsTablePrint(o io.Writer, jobs []tracejob.TraceJob) {
	format := "%s \t %s \t %s \t %s \t %s\t"
	if len(jobs) == 0 {
		fmt.Println("No resources found.")
		return
	}
	// initialize tabwriter
	w := new(tabwriter.Writer)
	// minwidth, tabwidth, padding, padchar, flags
	w.Init(o, 8, 8, 0, '\t', 0)
	defer w.Flush()

	// TODO(fntlnz): Do the status and age fields, we don't have a way to get them now, so reporting
	// them as missing.
	fmt.Fprintf(w, format, "NAMESPACE", "NODE", "NAME", "STATUS", "AGE")
	for _, j := range jobs {
		status := j.Status
		if status == "" {
			status = tracejob.TraceJobUnknown
		}
		fmt.Fprintf(w, "\n"+format, j.Namespace, j.Hostname, j.Name, status, translateTimestampSince(j.StartTime))
	}
	fmt.Fprintf(w, "\n")
}

// translateTimestampSince returns the elapsed time since timestamp in
// human-readable approximation.
func translateTimestampSince(timestamp *metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

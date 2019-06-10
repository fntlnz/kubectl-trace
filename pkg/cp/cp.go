package cp

import(
	"k8s.io/kubernetes/pkg/kubectl/cmd/exec"
)

type FileSpec struct {
	PodNamespace string
	PodName      string
	File         string
}

var (
	errFileCannotBeEmpty = errors.New("Filepath can not be empty")
)

// CopyFromPod copies a file from the original pod to the client machine.
// code initially taken the kubectl:
// 	https://github.com/kubernetes/kubernetes/blob/3d4124f2e083591c98e8a874760011781d3ee15c/pkg/kubectl/cmd/cp/cp.go#L278
func CopyFromPod(src, dest FileSpec) error {
	if len(src.File) == 0 || len(dest.File) == 0 {
		return errFileCannotBeEmpty
	}

	reader, outStream := io.Pipe()
	options := &exec.ExecOptions{
		StreamOptions: exec.StreamOptions{
			IOStreams: genericclioptions.IOStreams{
				In:     nil,
				Out:    outStream,
				ErrOut: o.Out,
			},

			Namespace: src.PodNamespace,
			PodName:   src.PodName,
		},

		// TODO: Improve error messages by first testing if 'tar' is present in the container?
		Command:  []string{"tar", "cf", "-", src.File},
		Executor: &exec.DefaultRemoteExecutor{},
	}

	go func() {
		defer outStream.Close()
		err := o.execute(options)
		cmdutil.CheckErr(err)
	}()
	
	prefix := getPrefix(src.File)
	prefix = path.Clean(prefix)
	// remove extraneous path shortcuts - these could occur if a path contained extra "../"
	// and attempted to navigate beyond "/" in a remote filesystem
	prefix = stripPathShortcuts(prefix)
	return o.untarAll(reader, dest.File, prefix)
}

package downloader

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/iovisor/kubectl-trace/pkg/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	tcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	// PidFile is where trace-uploader will write its pid
	PidFile = "/var/run/trace-uploader"
)

type Downloader struct {
	CoreV1Client tcorev1.CoreV1Interface
	Config       *restclient.Config
}

func New(client tcorev1.CoreV1Interface, config *restclient.Config) *Downloader {
	return &Downloader{
		CoreV1Client: client,
		Config:       config,
	}
}

func (d *Downloader) Start(traceJobID types.UID, namespace, downloadDir, podOutDir string) error {
	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.08,
		Jitter:   0.0,
		Steps:    30,
	}, func() (bool, error) {
		selector := fmt.Sprintf("%s=%s", meta.TraceIDLabelKey, traceJobID)
		pl, err := d.CoreV1Client.Pods(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: selector,
		})

		if err != nil {
			return false, err
		}

		if len(pl.Items) == 0 {
			// The trace job might exists but the pod might not have been scheduled so continue retrying.
			return false, nil
		}

		pod := &pl.Items[0]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("cannot attach to container in completed pod")
		}

		if len(pod.Spec.Containers) != 1 {
			return false, fmt.Errorf("pod contains more than one container")
		}

		restClient := d.CoreV1Client.RESTClient().(*restclient.RESTClient)
		containerName := pod.Spec.Containers[0].Name

		err = os.MkdirAll(downloadDir, 0755)
		if err != nil {
			return false, err
		}

		downloadFile, err := os.Create(path.Join(downloadDir, Filename(traceJobID)))
		if err != nil {
			return false, err
		}
		defer downloadFile.Close()

		req := restClient.Post().
			Resource("pods").
			Name(pod.Name).
			Namespace(namespace).
			SubResource("exec")
		req.VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   []string{"/bin/trace-uploader", "--pid", PidFile, "--out", podOutDir},
			Stdin:     false,
			Stdout:    true,
			Stderr:    false,
			TTY:       false,
		}, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(d.Config, "POST", req.URL())
		if err != nil {
			// There might be issues attaching to container if pod is initializing so continue retrying.
			return false, nil
		}

		err = exec.Stream(remotecommand.StreamOptions{
			Stdin:             nil,
			Stdout:            downloadFile,
			Stderr:            nil,
			Tty:               false,
			TerminalSizeQueue: nil,
		})
		if err != nil {
			// There might be issues attaching to container if pod is initializing so continue retrying.
			return false, nil
		}

		return true, nil
	})

	return err
}

// Filename is where downloader will put trace output.
func Filename(traceJobID types.UID) string {
	return fmt.Sprintf("%s%s.tar", meta.TracePrefix, traceJobID)
}

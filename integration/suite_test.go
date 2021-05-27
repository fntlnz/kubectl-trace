package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iovisor/kubectl-trace/pkg/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	DockerPushOutput = regexp.MustCompile("latest: digest: sha256:[0-9a-f]{64} size: [0-9]+")
)

var (
	KubectlTraceBinary = os.Getenv("TEST_KUBECTLTRACE_BINARY") // allow overriding the kubectl-trace binary used
	KubernetesBackend  = os.Getenv("TEST_KUBERNETES_BACKEND")  // allow specifying which kubernetes backend to use for tests
	ForceFreshBackend  = os.Getenv("TEST_FORCE_FRESH_BACKEND") // force a fresh kubernetes backend for tests
	TeardownBackend    = os.Getenv("TEST_TEARDOWN_BACKEND")    // force backend to be torn down after test run
	RegistryLocalPort  = os.Getenv("TEST_REGISTRY_PORT")       // override default port for backend's docker registry
)

const RegistryRemotePort = 5000
const RegistryWaitTimeout = 60

const (
	TraceJobsSystemNamespace = "kubectl-trace-system"
	TraceJobsTargetNamespace = "kubectl-trace-targets"
)

const IntegrationNamespaceLabel = "kubectl-trace-integration-ns"

var (
	ContainerDependencies = []string{
		"quay.io/iovisor/target-ruby",
		"quay.io/iovisor/kubectl-trace-init",
		"quay.io/iovisor/kubectl-trace-operator",
	}
)

type TestBackend interface {
	SetupBackend()
	TeardownBackend()
	GetBackendNode() string
	RunnerImage() string
	RegistryPort() int
}

type TestNameSpaceInfo struct {
	Namespace string
	Passed    bool
}

type KubectlTraceSuite struct {
	suite.Suite

	testBackend    TestBackend
	kubeConfigPath string
	lastTest       string
	namespaces     map[string]*TestNameSpaceInfo
}

func init() {
	if KubectlTraceBinary == "" {
		KubectlTraceBinary = "kubectl-trace"
	}

	if KubernetesBackend == "" {
		KubernetesBackend = KubernetesKindBackend
	}
}

func (k *KubectlTraceSuite) RunnerImage() string {
	return k.testBackend.RunnerImage()
}

func (k *KubectlTraceSuite) GetTestNode() string {
	return k.testBackend.GetBackendNode()
}

func (k *KubectlTraceSuite) SetupSuite() {
	path, err := os.Getwd()
	assert.Nil(k.T(), err)

	// tests are run from /path/to/kubectl-trace-shopify/integration
	k.kubeConfigPath = filepath.Join(path, "..", "_output", "kubeconfig")

	switch KubernetesBackend {
	case KubernetesKindBackend:
		k.testBackend = &kindBackend{
			suite: k,
		}
	case KubernetesMinikubeBackend:
		k.testBackend = &minikubeBackend{
			suite: k,
		}
	}

	k.testBackend.SetupBackend()

	k.cleanupPreviousRunNamespaces()
	k.namespaces = make(map[string]*TestNameSpaceInfo)

	fmt.Println("Pushing dependencies...")
	for _, image := range ContainerDependencies {
		k.tagAndPushIntegrationImage(image, "latest")
	}

	fmt.Println("Setting up targets...")
	k.setupTargets()

	fmt.Println("Setting up operator...")
	k.setupOperator()

	fmt.Printf("\x1b[1mKUBECONFIG=%s\x1b[0m\n", k.kubeConfigPath)
}

func (k *KubectlTraceSuite) setupOperator() {
	global := k.runWithoutError("kubectl", "kustomize", "../operator/config/global")
	client := k.runWithoutError("kubectl", "kustomize", "../operator/config/client")
	rbac := k.runWithoutError("kubectl", "kustomize", "../operator/config/rbac")

	deploy := k.runWithoutError("kubectl", "kustomize", "../operator/config/base")
	localDeploy := strings.ReplaceAll(deploy, "gcr.io/shopify-docker-images/cloud/production", fmt.Sprintf("localhost:%d/iovisor", RegistryRemotePort))

	all := strings.Join([]string{global, client, rbac, localDeploy}, "---\n")
	k.runWithoutErrorWithStdin(all, "kubectl", "apply", "-f", "-")
	k.runWithoutError("kubectl", "-n", TraceJobsSystemNamespace, "rollout", "restart", "deployment/kubectl-trace-controller-manager")
	k.runWithoutError("kubectl", "-n", TraceJobsSystemNamespace, "rollout", "status", "--timeout=180s", "deployment/kubectl-trace-controller-manager")
}

func (k *KubectlTraceSuite) teardownOperator() {
	global := k.runWithoutError("kubectl", "kustomize", "../operator/config/global")
	client := k.runWithoutError("kubectl", "kustomize", "../operator/config/client")
	rbac := k.runWithoutError("kubectl", "kustomize", "../operator/config/rbac")

	all := strings.Join([]string{global, client, rbac}, "---\n")
	k.runWithoutErrorWithStdin(all, "kubectl", "delete", "-f", "-")
}

func (k *KubectlTraceSuite) setupTargets() {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), TraceJobsTargetNamespace, metav1.GetOptions{})
	if statusError, isStatus := err.(*errors.StatusError); isStatus {
		if errors.IsNotFound(statusError) {
			namespaceSpec := &apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: TraceJobsTargetNamespace}}
			_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), namespaceSpec, metav1.CreateOptions{})
			assert.Nil(k.T(), err)
		} else {
			assert.FailNow(k.T(), "could not ensure namespace %s: %s", TraceJobsTargetNamespace, statusError.Error())
		}
	}

	_, err = clientset.CoreV1().Pods(TraceJobsTargetNamespace).Get(context.TODO(), "ruby", metav1.GetOptions{})
	if statusError, isStatus := err.(*errors.StatusError); isStatus {
		if errors.IsNotFound(statusError) {
			k.createRubyTarget(TraceJobsTargetNamespace, "ruby", "first", "second")
		} else {
			assert.FailNow(k.T(), "could not create Ruby target: %s", statusError.Error())
		}
	}
}

func (k *KubectlTraceSuite) teardownTargets() {
	k.deleteRubyTarget(TraceJobsTargetNamespace, "ruby")
}

func checkRegistryAvailable(registryPort int) error {
	registry := fmt.Sprintf("http://localhost:%d/v2/", registryPort)
	err := fmt.Errorf("registry %s is unavailable", registry)

	attempts := 0
	for err != nil && attempts < RegistryWaitTimeout {
		_, err = http.Get(registry)
		time.Sleep(1 * time.Second)
		attempts++
	}

	if err != nil {
		fmt.Printf("Failed waiting for registry to become available after %d seconds\n", attempts)
	}

	return err
}

func (k *KubectlTraceSuite) tagAndPushIntegrationImage(sourceName string, sourceTag string) {
	parsedImage, err := docker.ParseImageName(sourceName)
	assert.Nil(k.T(), err)

	pushTag := fmt.Sprintf("localhost:%d/%s/%s:latest", k.testBackend.RegistryPort(), parsedImage.Repository, parsedImage.Name)
	sourceImage := sourceName + ":" + sourceTag

	output := k.runWithoutError("docker", "tag", sourceImage, pushTag)
	assert.Empty(k.T(), output)

	output = k.runWithoutError("docker", "push", pushTag)
	assert.Regexp(k.T(), DockerPushOutput, output)
}

func (k *KubectlTraceSuite) BeforeTest(suiteName, testName string) {
	k.lastTest = testName
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	namespace, err := generateNamespaceName()

	k.namespaces[testName] = &TestNameSpaceInfo{Namespace: namespace}
	assert.Nil(k.T(), err)

	namespaceLabels := map[string]string{
		IntegrationNamespaceLabel: testName,
	}

	nsSpec := &apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: k.namespace(), Labels: namespaceLabels}}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), nsSpec, metav1.CreateOptions{})
	assert.Nil(k.T(), err)
}

func (k *KubectlTraceSuite) AfterTest(suiteName, testName string) {
	k.namespaces[testName].Passed = !k.T().Failed()

	if k.namespaces[testName].Passed {
		// delete the namespace if the test passed
		k.deleteNamespace(k.namespace())
	}
	k.lastTest = ""
}

func (k *KubectlTraceSuite) cleanupPreviousRunNamespaces() {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	clientset, err := kubernetes.NewForConfig(clientConfig)
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{LabelSelector: IntegrationNamespaceLabel})

	if err != nil {
		fmt.Printf("Error listing previous namespaces %v", err)
	}

	for _, ns := range namespaces.Items {
		fmt.Printf("Cleaning up test namespace from previous run %s\n", ns.Name)
		k.deleteNamespace(ns.Name)
	}
}

func (k *KubectlTraceSuite) deleteNamespace(namespace string) {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	fg := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &fg}
	err = clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, deleteOptions)
	assert.Nil(k.T(), err)
}

// Reports namespaces of any failed tests for debugging purposes
func (k *KubectlTraceSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {
	if TeardownBackend != "" {
		return
	}

	for _, v := range stats.TestStats {
		if !v.Passed {
			namespace := k.namespaces[v.TestName].Namespace
			fmt.Printf("\x1b[1m%s failed, namespace %s has been preserved for debugging\x1b[0m\n", v.TestName, namespace)
		}
	}
}

func (k *KubectlTraceSuite) TearDownSuite() {
	if TeardownBackend != "" {
		k.teardownOperator()
		k.teardownTargets()
		k.testBackend.TeardownBackend()
	}
}

func TestKubectlTraceSuite(t *testing.T) {
	suite.Run(t, &KubectlTraceSuite{})
}

func (k *KubectlTraceSuite) GetJobs() *batchv1.JobList {
	return k.GetJobsInNamespace(k.namespace())
}

func (k *KubectlTraceSuite) GetJobsInNamespace(namespace string) *batchv1.JobList {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	jobs, err := clientset.BatchV1().Jobs(namespace).List(context.TODO(), metav1.ListOptions{})
	assert.Nil(k.T(), err)

	return jobs
}

func (k *KubectlTraceSuite) namespace() string {
	if k.lastTest == "" {
		require.NotEmpty(k.T(), k.lastTest, "Programming error in test suite: lastTest not set on suite. This condition should be impossible to hit and is a bug if you see this.")
	}

	namespaceInfo := k.namespaces[k.lastTest]
	return namespaceInfo.Namespace
}

func (k *KubectlTraceSuite) KubectlTraceCmd(args ...string) string {
	args = append([]string{fmt.Sprintf("--namespace=%s", k.namespace())}, args...)
	return k.runWithoutError(KubectlTraceBinary, args...)
}

func generateNamespaceName() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(fmt.Sprintf("kubectl-trace-test-%X", buf)), nil
}

func (k *KubectlTraceSuite) runWithoutError(command string, args ...string) string {
	return k.runWithoutErrorWithStdin("", command, args...)
}

func (k *KubectlTraceSuite) runWithoutErrorWithStdin(input string, command string, args ...string) string {
	// prepare the command
	comm := exec.Command(command, args...)

	// prepare stdin unless it's empty
	if input != "" {
		stdin, err := comm.StdinPipe()
		if err != nil {
			assert.Nilf(k.T(), err, "Could not create the commmand: %s", err.Error())
		}
		go func() {
			defer stdin.Close()
			io.WriteString(stdin, input)
		}()
	}

	// prepare the environment
	comm.Env = os.Environ()
	comm.Env = append(comm.Env, fmt.Sprintf("KUBECONFIG=%s", k.kubeConfigPath)) // required to write a unique kubeconfig for the test run)

	// run it
	o, err := comm.CombinedOutput()
	combined := string(o)

	assert.Nilf(k.T(), err, "Command failed with output %s", combined)

	return combined
}

func (k *KubectlTraceSuite) createRubyTarget(namespace, name string, args ...string) error {
	image := fmt.Sprintf("localhost:%d/iovisor/target-ruby:latest", RegistryRemotePort)
	command := append([]string{"./fork-from-args"}, args...)

	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	pod := &apiv1.Pod{}
	var w watch.Interface

	podRequest := &apiv1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: apiv1.PodSpec{
			Containers: []apiv1.Container{
				{
					Name:    name,
					Image:   image,
					Command: command,
				},
			},
		},
	}

	if pod, err = clientset.CoreV1().Pods(namespace).Create(context.TODO(), podRequest, metav1.CreateOptions{}); err != nil {
		return err
	}

	status := pod.Status
	if w, err = clientset.CoreV1().Pods(namespace).Watch(context.TODO(), metav1.ListOptions{
		Watch:           true,
		ResourceVersion: pod.ResourceVersion,
		FieldSelector:   fields.Set{"metadata.name": name}.AsSelector().String(),
		LabelSelector:   labels.Everything().String(),
	}); err != nil {
		return err
	}

	func() {
		for {
			select {
			case events, ok := <-w.ResultChan():
				if !ok {
					return
				}
				pod = events.Object.(*apiv1.Pod)
				status = pod.Status
				if pod.Status.Phase != apiv1.PodPending {
					w.Stop()
				}
			case <-time.After(10 * time.Second):
				fmt.Println("timeout to wait for pod active")
				w.Stop()
			}
		}
	}()
	if status.Phase != apiv1.PodRunning {
		return fmt.Errorf("Pod is unavailable: %v", status.Phase)
	}

	return nil
}

func (k *KubectlTraceSuite) deleteRubyTarget(namespace, name string) error {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	fg := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &fg}
	if err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), name, deleteOptions); err != nil {
		return err
	}

	return nil
}

func (k *KubectlTraceSuite) deleteOperatorPod() error {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientset, err := kubernetes.NewForConfig(clientConfig)
	assert.Nil(k.T(), err)

	listOptions := metav1.ListOptions{LabelSelector: "app=kubectl-trace-operator"}
	pods, err := clientset.CoreV1().Pods(TraceJobsSystemNamespace).List(context.TODO(), listOptions)
	if err != nil {
		return err
	}

	assert.Equal(k.T(), 1, len(pods.Items))
	pod := pods.Items[0]

	fg := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &fg}
	if err = clientset.CoreV1().Pods(TraceJobsSystemNamespace).Delete(context.TODO(), pod.Name, deleteOptions); err != nil {
		return err
	}

	return nil
}

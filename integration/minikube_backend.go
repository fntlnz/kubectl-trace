package integration

import (
	"fmt"
	"github.com/iovisor/kubectl-trace/pkg/cmd"
	"github.com/iovisor/kubectl-trace/pkg/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

var (
	MinikubeStartOutput    = `Done! kubectl is now configured to use "` + MinikubeProfileName + `"`
	MinikubeDeleteOutput   = `Removed all traces of the "` + MinikubeProfileName + `" cluster`
	MinikubeRegistryOutput = "The 'registry' addon is enabled"
)

const (
	MinikubeMinimumVersion      = "v1.20.0"
	KubernetesMinikubeBackend   = "minikube"
	MinikubeProfileName         = "minikube-kubectl-trace"
	MinikubeDefaultRegistryPort = 6000
)

type minikubeBackend struct {
	suite        *KubectlTraceSuite
	runnerImage  string
	registryPort int
}

func (b *minikubeBackend) SetupBackend() {
	_, err := exec.LookPath("minikube")
	assert.Nil(b.suite.T(), err)

	fmt.Println("Setting up Minikube backend...")

	minikubeVersion := strings.TrimSpace(b.suite.runWithoutError("minikube", "version", "--short"))
	fmt.Printf("minikube is version %s, minimum is %s\n", minikubeVersion, MinikubeMinimumVersion)
	require.GreaterOrEqual(b.suite.T(), semver.Compare(minikubeVersion, MinikubeMinimumVersion), 0, fmt.Sprintf("minikube %s is too old, upgrade to %s", minikubeVersion, MinikubeMinimumVersion))

	if RegistryLocalPort == "" {
		b.registryPort = MinikubeDefaultRegistryPort
	} else {
		b.registryPort, err = strconv.Atoi(RegistryLocalPort)
		assert.Nil(b.suite.T(), err)
	}

	if ForceFreshBackend != "" {
		b.TeardownBackend()
	}

	output := b.suite.runWithoutError("minikube", "start", "--insecure-registry=localhost:"+strconv.Itoa(RegistryRemotePort), "--profile", MinikubeProfileName)
	require.Contains(b.suite.T(), output, MinikubeStartOutput)

	output = b.suite.runWithoutError("minikube", "addons", "enable", "registry", "--profile", MinikubeProfileName)
	require.Contains(b.suite.T(), output, MinikubeRegistryOutput)

	// The minikube registry uses a replica controller, so we match on pod label when waiting for it
	fmt.Printf("Waiting for minikube registry to be ready...\n")
	_ = b.suite.runWithoutError("kubectl", "wait", "pod", "-l", "actual-registry=true", "-n", "kube-system", "--for=condition=Ready", "--timeout=120s")

	minikube_ip := strings.TrimSuffix(b.suite.runWithoutError("minikube", "ip", "--profile", MinikubeProfileName), "\n")
	require.NotNil(b.suite.T(), net.ParseIP(minikube_ip))

	// Start the registry proxy container if not already started
	comm := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", "minikube-kubectl-trace-registry-proxy")
	o, err := comm.CombinedOutput()
	if err != nil || strings.TrimSuffix(string(o), "\n") != "true" {
		output := b.suite.runWithoutError("docker", "run", "-d", "--restart=always", "--network=host", "--name", "minikube-kubectl-trace-registry-proxy", "alpine/socat", "TCP-LISTEN:6000,reuseaddr,fork", "TCP:"+minikube_ip+":"+strconv.Itoa(RegistryRemotePort))
		fmt.Printf("Started registry proxy: %s\n", output)
	}

	// make sure the repository is available
	require.Nil(b.suite.T(), checkRegistryAvailable(b.registryPort))

	// parse the image of the desired image runner name
	parsedImage, err := docker.ParseImageName(cmd.ImageName)
	require.Nil(b.suite.T(), err)

	// set the runner image name with the repository port
	// TODO: extract the repository logic somewhere?
	b.runnerImage = fmt.Sprintf("localhost:%d/%s/%s:latest", RegistryRemotePort, parsedImage.Repository, parsedImage.Name)

	// tag and push the integration image
	b.suite.tagAndPushIntegrationImage(cmd.ImageName, cmd.ImageTag)
}

func (b *minikubeBackend) TeardownBackend() {
	fmt.Printf("Deleting minikube cluster...\n")
	output := b.suite.runWithoutError("minikube", "delete", "--profile", MinikubeProfileName)
	assert.Contains(b.suite.T(), output, MinikubeDeleteOutput)

	b.suite.runWithoutError("docker", "rm", "-f", "minikube-kubectl-trace-registry-proxy") // FIXME allow this to fail if container is missing, or verify it is running first
}
func (b *minikubeBackend) GetBackendNode() string {
	return MinikubeProfileName
}

func (b *minikubeBackend) RegistryPort() int {
	return b.registryPort
}

func (b *minikubeBackend) RunnerImage() string {
	return b.runnerImage
}

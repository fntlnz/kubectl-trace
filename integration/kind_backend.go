package integration

import (
	"fmt"
	"github.com/iovisor/kubectl-trace/pkg/docker"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/iovisor/kubectl-trace/pkg/cmd"
	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	KubernetesKindBackend   = "kind"
	KindClusterName         = "kubectl-trace-kind"
	KindDefaultRegistryPort = 5000
)

type kindBackend struct {
	suite        *KubectlTraceSuite
	provider     *cluster.Provider
	runnerImage  string
	name         string
	registryPort int
}

func (b *kindBackend) SetupBackend() {
	var err error
	b.name = KindClusterName

	fmt.Println("Setting up KiND backend...")
	if RegistryLocalPort == "" {
		b.registryPort = KindDefaultRegistryPort
	} else {
		b.registryPort, err = strconv.Atoi(RegistryLocalPort)
		assert.Nil(b.suite.T(), err)
	}

	registryConfig := `
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:` + strconv.Itoa(RegistryRemotePort) + `"]
    endpoint = ["http://kind-registry:` + strconv.Itoa(RegistryRemotePort) + `"]
`

	b.provider = cluster.NewProvider()

	if ForceFreshBackend != "" {
		b.TeardownBackend()
	}

	clusters, _ := b.provider.List()

	if !kindClusterExists(clusters, b.name) {
		fmt.Printf("Creating new KiND cluster\n")
		// Create the cluster
		err = b.provider.Create(
			b.name,
			cluster.CreateWithRetain(false),
			cluster.CreateWithWaitForReady(time.Duration(0)),
			cluster.CreateWithKubeconfigPath(b.suite.kubeConfigPath),
			cluster.CreateWithRawConfig([]byte(registryConfig)),

			// todo > we need a logger
			// cluster.ProviderWithLogger(logger),
			// runtime.GetDefault(logger),
		)
		assert.Nil(b.suite.T(), err)
	}

	b.provider.ExportKubeConfig(b.name, b.suite.kubeConfigPath)

	// Start the registry container if not already started
	comm := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", "kind-registry")
	o, err := comm.CombinedOutput()
	if err != nil || strings.TrimSuffix(string(o), "\n") != "true" {
		output := b.suite.runWithoutError("docker", "run", "-d", "--restart=always", "-p", fmt.Sprintf("%d:%d", b.registryPort, RegistryRemotePort), "--name", "kind-registry", "registry:2")
		fmt.Printf("Started registry: %s\n", output)
	}

	// This template is to avoid having to unmarshal the JSON, and will print "true" if there is container called "kind-registry" on the network
	networkTemplate := `{{range $_, $value  := .Containers }}{{if index $value "Name" | eq "kind-registry"}}true{{end}}{{end}}`
	output := b.suite.runWithoutError("docker", "network", "inspect", "kind", "--format", networkTemplate)
	if strings.TrimSuffix(output, "\n") != "true" {
		// Connect to docker network
		output = b.suite.runWithoutError("docker", "network", "connect", "kind", "kind-registry")
		fmt.Printf("Connected network: %s\n", output)
	}

	// make sure the repository is available
	assert.Nil(b.suite.T(), checkRegistryAvailable(b.registryPort))

	// parse the image of the desired image runner name
	parsedImage, err := docker.ParseImageName(cmd.ImageName)
	assert.Nil(b.suite.T(), err)

	// set the runner image name with the repository port
	b.runnerImage = fmt.Sprintf("localhost:%d/%s/%s:latest", RegistryRemotePort, parsedImage.Repository, parsedImage.Name)

	b.suite.tagAndPushIntegrationImage(cmd.ImageName, cmd.ImageTag)
}

func (b *kindBackend) TeardownBackend() {
	fmt.Printf("Deleting KiND cluster\n")
	kubeConfig, err := b.provider.KubeConfig(b.name, false)
	assert.Nil(b.suite.T(), err)
	err = b.provider.Delete(b.name, kubeConfig)
	assert.Nil(b.suite.T(), err)
	b.suite.runWithoutError("docker", "rm", "-f", "kind-registry")
}

func (b *kindBackend) GetBackendNode() string {
	nodes, err := b.provider.ListNodes(b.name)
	assert.Nil(b.suite.T(), err)
	assert.Equal(b.suite.T(), 1, len(nodes))
	nodeName := nodes[0].String()
	return nodeName
}

func (b *kindBackend) RegistryPort() int {
	return b.registryPort
}

func (b *kindBackend) RunnerImage() string {
	return b.runnerImage
}

func kindClusterExists(clusters []string, cluster string) bool {
	for _, n := range clusters {
		if cluster == n {
			return true
		}
	}
	return false
}

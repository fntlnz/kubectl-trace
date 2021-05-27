/*

Currently, we are doing a bunch of crazy stuff in order to build our trace job.

We have two ways of specifying the thing we actually want to trace:

- From a literal target resource ( run.go 's o.resourceArg, taken from arg[0])
  - resourceArg is overloaded, it can be either the node, or the pod which we would like to trace
    - In the future, it may be other types, eg, deployment/NAME, or statefulset/NAME, etc.
  - additionally, o.container is specified further with arg[1], and may be required for cases where resourceArg is not a Node.

Complicating this even further, we also permit for the target to be given by a Selector string

Because of this, the selector string is parsed into a struct with a mutable hash, and we end up using
the selector struct as a way to store our actual target.

To simplify this, we should separate the **desired target** from the **selected target**.

If the pod / container are specified with positional arguments, then the selector **must not**
include a selector for the pod or container, but is permitted to have a process selector.

It is also valid to have no positional arguments at all, and rely entirely on the
selector to specify the pod / container to target.

1. Resolve the Target
  - Look at any positional arguments (from CLI)
  - Look at the Selector
  - Perform validations
  - Resolve to:
    - At minimum, always a node to run the tracejob on (inferred through pod, and perhaps through deployment → pod)
    - If a pod is being targeted, it may also have a Pod target and a Container
2. Create the TraceJob
  - The fully specify target (the output from 1, above) is provided as a parameter
  - The expectation is that a TraceJob is an entity that is scheduled on a given node:
    - Other logic (1) is responsible for determining the correct node to run on
    - Only necessary selector arguments are passed to the trace-runner, as an argument within the trace job

*/

package tracejob

import (
	"context"
	"fmt"
	"strings"

	//v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	//"k8s.io/client-go/kubernetes/scheme"
)

/*
This struct will be used when creating the TraceJob
*/

type TraceJobTarget struct {
	Node        string // Used for tracejob NodeSelector
	PodUID      string // passed as argument to trace-runner
	ContainerID string // passed as argument to trace-runner
}

/*
ResolveTraceJobTarget will:

- take a kubernetes resource, possibly namespaced, and resolve it to a concrete node that the tracejob can run on
  - if the resource is a node, the scope of the trace will be the entire node
  - if the resource is a pod, or something that can be resolved to a pod:
    - resource is namespaced, the targetNamespace is required
    - if the pod has multiple containers, a container name is required, otherwise the only container will be used

clientset       - a kubernetes clientset, used for resolving the target resource
resource        - a string, indicating the kubernetes resource to be traced. Currently supported: node, pod
container       - (optional) the container to trace, required only if needed to disambiguate
targetNamespace - (optional) the target namespace of a given resource, required if the resource is namespaced

*/

func ResolveTraceJobTarget(clientset kubernetes.Interface, resource, container, targetNamespace string) (*TraceJobTarget, error) {

	target := TraceJobTarget{}

	var resourceType string
	var resourceID string

	resourceParts := strings.Split(resource, "/")

	if len(resourceParts) < 1 || len(resourceParts) > 2 {
		return nil, fmt.Errorf("Invalid resource string %s\n", resource)
	} else if len(resourceParts) == 1 {
		resourceType = "node"
		resourceID = resourceParts[0]
	} else if len(resourceParts) == 2 {
		resourceType = resourceParts[0]
		resourceID = resourceParts[1]
	}

	switch resourceType {
	case "node":
		nodeClient := clientset.CoreV1().Nodes()
		node, err := nodeClient.Get(context.TODO(), resourceID, metav1.GetOptions{})

		if err != nil {
			return nil, fmt.Errorf("Failed to locate a node for %s %v\n", resourceID, err)
		} else {
			labels := node.GetLabels()
			val, ok := labels["kubernetes.io/hostname"]
			if !ok {
				return nil, fmt.Errorf("label kubernetes.io/hostname not found in node")
			}
			target.Node = val
		}
		return &target, nil
	case "pod":
		podClient := clientset.CoreV1().Pods(targetNamespace)
		pod, err := podClient.Get(context.TODO(), resourceID, metav1.GetOptions{})

		if err != nil {
			return nil, fmt.Errorf("failed to locate a pod for %s; error was %s", resourceID, err.Error())
		} else {
			if pod.Spec.NodeName == "" {
				return nil, fmt.Errorf("cannot attach a trace program to a pod that is not currently scheduled on a node")
			}

			var targetContainer string
			target.Node = pod.Spec.NodeName
			target.PodUID = string(pod.UID)

			if len(pod.Spec.Containers) == 1 {
				targetContainer = pod.Spec.Containers[0].Name
			} else {
				// FIXME verify container is not empty
				targetContainer = container
			}

			for _, s := range pod.Status.ContainerStatuses {
				if s.Name == targetContainer {
					containerID := strings.TrimPrefix(s.ContainerID, "docker://")
					containerID = strings.TrimPrefix(containerID, "containerd://")
					target.ContainerID = containerID
					break
				}
			}

			if target.ContainerID == "" {
				return nil, fmt.Errorf("no containers found for the provided pod %s and container %s combination", pod.Name, targetContainer)
			}

		}
		return &target, nil
	default:
		return nil, fmt.Errorf("Unsupported resource type %s for %s\n", resourceType, resourceID)
	}

	return &target, nil
}

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/iovisor/kubectl-trace/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	"k8s.io/client-go/tools/clientcmd"
)

var tjScheme = runtime.NewScheme()

const waitForTraceJobSeconds = 60

func init() {
	v1alpha1.AddToScheme(tjScheme)
}

func (k *KubectlTraceSuite) TestTraceJobCreation() {
	traceJobName := k.createTraceJob("rbspy", TraceJobsTargetNamespace, "ruby", "ruby", "pid=last,exe=ruby", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeSuccess, waitForTraceJobSeconds*time.Second))

	tracejobs := k.getTraceJobs()
	require.Equal(k.T(), 1, len(tracejobs.Items))

	jobs := k.GetJobsInNamespace(k.namespace())
	require.Equal(k.T(), 1, len(jobs.Items))

	job := jobs.Items[0]
	tracejob := tracejobs.Items[0]
	assert.Equal(k.T(), job.ObjectMeta.Name, tracejob.Status.JobName)

	conditions := tracejob.Status.Conditions
	assert.Equal(k.T(), 2, len(conditions))
	assert.Equal(k.T(), v1alpha1.ConditionTypeRunning, conditions[0].Type)
	assert.Equal(k.T(), v1alpha1.ConditionTypeSuccess, conditions[1].Type)
}

func (k *KubectlTraceSuite) TestTraceJobsAreNotInvalidAfterOperatorRestart() {
	targetName := "ruby-target"
	err := k.createRubyTarget(k.namespace(), targetName)
	require.Nil(k.T(), err)

	traceJobName := k.createTraceJob("rbspy", k.namespace(), targetName, targetName, "pid=last,exe=ruby", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeSuccess, waitForTraceJobSeconds*time.Second))

	// Delete the target so that further attempts to validate the target would fail
	k.deleteRubyTarget(k.namespace(), targetName)

	// Delete the operator manager pod, which will be restarted by the deployment machinery
	k.deleteOperatorPod()

	// Finally, wait for the pod to restart and *not* mark the TraceJob as invalid
	waitForInvalid := k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeInvalid, 30*time.Second)
	assert.NotNil(k.T(), waitForInvalid)
	assert.Contains(k.T(), waitForInvalid.Error(), "gave up waiting")
}

func (k *KubectlTraceSuite) TestInvalidTraceJobTarget() {
	traceJobName := k.createTraceJob("rbspy", TraceJobsTargetNamespace, "not-a-pod", "ruby", "pid=last,exe=ruby", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeInvalid, waitForTraceJobSeconds*time.Second))

	tracejobs := k.getTraceJobs()
	require.Equal(k.T(), 1, len(tracejobs.Items))

	tracejob := tracejobs.Items[0]
	assert.Equal(k.T(), 1, len(tracejob.Status.Conditions))
	assert.Equal(k.T(), "InvalidTarget", tracejob.Status.Conditions[0].Reason)
}

func (k *KubectlTraceSuite) TestTraceJobGCCleansUpSuccessfulJobAfterTTL() {
	traceJobName := k.createTraceJob("rbspy", TraceJobsTargetNamespace, "ruby", "ruby", "pid=last,exe=ruby", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeSuccess, waitForTraceJobSeconds*time.Second))

	tracejobs := k.getTraceJobs()
	require.Equal(k.T(), 1, len(tracejobs.Items))

	k.createGC(1, 100, 100, 1)

	/*
		A sleep is used because the conditions we want to sync on is:
		- The job has Completed (synced above)
		- The TTL has expired
		- The controller has had time to refresh and notic this.
		With a refresh interval of "1", and TTL of "1",
		this should succeed within a minimum of 3 seconds in most cases, as it is
		possible that the check will execute before the TTL is expired.
		To avoid the possibility of flakiness, an extra 2 seconds of padding are added.
	*/
	time.Sleep(5 * time.Second)

	tracejobs = k.getTraceJobs()
	assert.Equal(k.T(), 0, len(tracejobs.Items))
}

func (k *KubectlTraceSuite) TestTraceJobGCCleansUpFailedJobAfterTTL() {
	traceJobName := k.createTraceJob("rbspy", TraceJobsTargetNamespace, "ruby", "ruby", "invalid", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeFailed, waitForTraceJobSeconds*time.Second))

	tracejobs := k.getTraceJobs()
	require.Equal(k.T(), 1, len(tracejobs.Items))

	k.createGC(100, 1, 100, 1)

	// See above for comment on appropriate delay
	time.Sleep(5 * time.Second)

	tracejobs = k.getTraceJobs()
	assert.Equal(k.T(), 0, len(tracejobs.Items))
}

func (k *KubectlTraceSuite) TestTraceJobGCCleansUpInvalidJobAfterTTL() {
	traceJobName := k.createTraceJob("rbspy", TraceJobsTargetNamespace, "not-a-pod", "invalid", "invalid", "stdout", 5)
	assert.Nil(k.T(), k.waitForTraceJob(traceJobName, v1alpha1.ConditionTypeInvalid, waitForTraceJobSeconds*time.Second))

	tracejobs := k.getTraceJobs()
	require.Equal(k.T(), 1, len(tracejobs.Items))

	k.createGC(100, 100, 1, 1)

	// See above for comment on appropriate delay
	time.Sleep(5 * time.Second)

	tracejobs = k.getTraceJobs()
	assert.Equal(k.T(), 0, len(tracejobs.Items))
}

func (k *KubectlTraceSuite) createTraceJob(tracer, namespace, podId, containerName, selector, output string, deadline int64) string {
	traceJob := v1alpha1.TraceJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TraceJob",
			APIVersion: "trace.iovisor.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "integration-test-",
			Namespace:    k.namespace(),
		},
		Spec: v1alpha1.TraceJobSpec{
			Tracer:           tracer,
			TargetNamespace:  namespace,
			Resource:         "pod/" + podId,
			Container:        containerName,
			Selector:         selector,
			FetchHeaders:     true,
			Output:           output,
			ImageNameTag:     fmt.Sprintf("localhost:%d/iovisor/kubectl-trace-runner:latest", RegistryRemotePort),
			InitImageNameTag: fmt.Sprintf("localhost:%d/iovisor/kubectl-trace-init:latest", RegistryRemotePort),
			Deadline:         deadline,
		},
	}

	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientConfig.APIPath = "/apis"
	clientConfig.GroupVersion = &v1alpha1.GroupVersion
	clientConfig.NegotiatedSerializer = serializer.NewCodecFactory(tjScheme)

	client, err := rest.RESTClientFor(clientConfig)
	assert.Nil(k.T(), err)

	result := v1alpha1.TraceJob{}
	err = client.Post().Resource("tracejobs").Body(&traceJob).Namespace(k.namespace()).Do(context.TODO()).Into(&result)
	assert.NotNil(k.T(), result)
	assert.Nil(k.T(), err)

	return result.GetObjectMeta().GetName()
}

func (k *KubectlTraceSuite) waitForTraceJob(traceJobName string, waitForType v1alpha1.ConditionType, duration time.Duration) error {
	until := time.Now().Add(duration)

	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientConfig.APIPath = "/apis"
	clientConfig.GroupVersion = &v1alpha1.GroupVersion
	clientConfig.NegotiatedSerializer = serializer.NewCodecFactory(tjScheme)

	client, err := rest.RESTClientFor(clientConfig)
	assert.Nil(k.T(), err)

	traceJob := v1alpha1.TraceJob{}
	for {
		err = client.Get().Namespace(k.namespace()).Resource("tracejobs").Name(traceJobName).Do(context.TODO()).Into(&traceJob)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}

		for _, c := range traceJob.Status.Conditions {
			if c.Type == waitForType {
				return nil
			}
		}

		time.Sleep(1 * time.Second)
		if time.Now().After(until) {
			break
		}
	}

	return fmt.Errorf("gave up waiting for condition %s on tracejob %s", string(waitForType), traceJobName)
}

func (k *KubectlTraceSuite) createGC(completedTTL, failedTTL, invalidTTL, frequency int64) {
	traceJobGC := v1alpha1.TraceJobGC{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TraceJobGC",
			APIVersion: "trace.iovisor.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tracejobgc-integration-test",
			Namespace: k.namespace(),
		},
		Spec: v1alpha1.TraceJobGCSpec{
			CompletedTTL: completedTTL,
			FailedTTL:    failedTTL,
			InvalidTTL:   invalidTTL,
			Frequency:    frequency,
		},
	}

	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientConfig.APIPath = "/apis"
	clientConfig.GroupVersion = &v1alpha1.GroupVersion
	clientConfig.NegotiatedSerializer = serializer.NewCodecFactory(tjScheme)

	client, err := rest.RESTClientFor(clientConfig)
	assert.Nil(k.T(), err)

	result := v1alpha1.TraceJobGC{}
	err = client.Post().Resource("tracejobgcs").Body(&traceJobGC).Namespace(k.namespace()).Do(context.TODO()).Into(&result)
	assert.NotNil(k.T(), result)
	assert.Nil(k.T(), err)
}

func (k *KubectlTraceSuite) getTraceJobs() *v1alpha1.TraceJobList {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	assert.Nil(k.T(), err)

	clientConfig.APIPath = "/apis"
	clientConfig.GroupVersion = &v1alpha1.GroupVersion
	clientConfig.NegotiatedSerializer = serializer.NewCodecFactory(tjScheme)

	client, err := rest.RESTClientFor(clientConfig)
	assert.Nil(k.T(), err)

	traceJobList := v1alpha1.TraceJobList{}
	err = client.Get().Namespace(k.namespace()).Resource("tracejobs").Do(context.TODO()).Into(&traceJobList)
	assert.Nil(k.T(), err)

	return &traceJobList
}

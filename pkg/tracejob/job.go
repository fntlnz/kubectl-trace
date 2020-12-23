package tracejob

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/iovisor/kubectl-trace/pkg/meta"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	batchv1typed "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1typed "k8s.io/client-go/kubernetes/typed/core/v1"
)

type TraceJobClient struct {
	JobClient    batchv1typed.JobInterface
	ConfigClient corev1typed.ConfigMapInterface
	outStream    io.Writer
}

// TraceJob is a container of info needed to create the job responsible for tracing.
type TraceJob struct {
	Name                string
	ID                  types.UID
	Namespace           string
	ServiceAccount      string
	Hostname            string
	Tracer              string
	Selector            string
	Output              string
	Program             string
	ImageNameTag        string
	InitImageNameTag    string
	FetchHeaders        bool
	Deadline            int64
	DeadlineGracePeriod int64
	StartTime           *metav1.Time
	Status              TraceJobStatus
}

// WithOutStream setup a file stream to output trace job operation information
func (t *TraceJobClient) WithOutStream(o io.Writer) {
	if o == nil {
		t.outStream = ioutil.Discard
	}
	t.outStream = o
}

type TraceJobFilter struct {
	Name *string
	ID   *types.UID
}

func (nf TraceJobFilter) selectorOptions() metav1.ListOptions {
	selectorOptions := metav1.ListOptions{}

	if nf.Name != nil {
		selectorOptions = metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", meta.TraceLabelKey, *nf.Name),
		}
	}

	if nf.ID != nil {
		selectorOptions = metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", meta.TraceIDLabelKey, *nf.ID),
		}
	}

	if nf.Name == nil && nf.ID == nil {
		selectorOptions = metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s", meta.TraceIDLabelKey),
		}
	}

	return selectorOptions
}

func (t *TraceJobClient) findJobsWithFilter(nf TraceJobFilter) ([]batchv1.Job, error) {

	selectorOptions := nf.selectorOptions()
	if len(selectorOptions.LabelSelector) == 0 {
		return []batchv1.Job{}, nil
	}

	jl, err := t.JobClient.List(context.Background(), selectorOptions)

	if err != nil {
		return nil, err
	}
	return jl.Items, nil
}

func (t *TraceJobClient) findConfigMapsWithFilter(nf TraceJobFilter) ([]apiv1.ConfigMap, error) {
	selectorOptions := nf.selectorOptions()
	if len(selectorOptions.LabelSelector) == 0 {
		return []apiv1.ConfigMap{}, nil
	}

	cm, err := t.ConfigClient.List(context.Background(), selectorOptions)

	if err != nil {
		return nil, err
	}
	return cm.Items, nil
}

func (t *TraceJobClient) GetJob(nf TraceJobFilter) ([]TraceJob, error) {
	jl, err := t.findJobsWithFilter(nf)
	if err != nil {
		return nil, err
	}
	tjobs := []TraceJob{}

	for _, j := range jl {
		labels := j.GetLabels()
		name, ok := labels[meta.TraceLabelKey]
		if !ok {
			name = ""
		}
		id, ok := labels[meta.TraceIDLabelKey]
		if !ok {
			id = ""
		}
		hostname, err := jobHostname(j)
		if err != nil {
			hostname = ""
		}
		tj := TraceJob{
			Name:      name,
			ID:        types.UID(id),
			Namespace: j.Namespace,
			Hostname:  hostname,
			StartTime: j.Status.StartTime,
			Status:    jobStatus(j),
		}
		tjobs = append(tjobs, tj)
	}

	return tjobs, nil
}

func (t *TraceJobClient) DeleteJobs(nf TraceJobFilter) error {
	nothingDeleted := true
	jl, err := t.findJobsWithFilter(nf)
	if err != nil {
		return err
	}

	dp := metav1.DeletePropagationForeground
	for _, j := range jl {
		err := t.JobClient.Delete(context.Background(), j.Name, metav1.DeleteOptions{
			GracePeriodSeconds: int64Ptr(0),
			PropagationPolicy:  &dp,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(t.outStream, "trace job %s deleted\n", j.Name)
		nothingDeleted = false
	}

	cl, err := t.findConfigMapsWithFilter(nf)

	if err != nil {
		return err
	}

	for _, c := range cl {
		err := t.ConfigClient.Delete(context.Background(), c.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(t.outStream, "trace configuration %s deleted\n", c.Name)
		nothingDeleted = false
	}

	if nothingDeleted {
		fmt.Fprintf(t.outStream, "error: no trace found to be deleted\n")
	}
	return nil
}

func (t *TraceJobClient) CreateJob(nj TraceJob) (*batchv1.Job, error) {

	traceCmd := []string{
		"/bin/timeout",
		"--preserve-status",
		"--signal",
		"INT",
		strconv.FormatInt(nj.Deadline, 10),
		"/bin/trace-runner",
		"--tracer=" + nj.Tracer,
		"--selector=" + nj.Selector,
		"--output=" + nj.Output,
	}

	if nj.Tracer == "bpftrace" {
		traceCmd = append(traceCmd, "--program=/programs/program.bt")
	} else {
		traceCmd = append(traceCmd, "--program="+nj.Program)
	}

	commonMeta := metav1.ObjectMeta{
		Name:      nj.Name,
		Namespace: nj.Namespace,
		Labels: map[string]string{
			meta.TraceLabelKey:   nj.Name,
			meta.TraceIDLabelKey: string(nj.ID),
		},
		Annotations: map[string]string{
			meta.TraceLabelKey:   nj.Name,
			meta.TraceIDLabelKey: string(nj.ID),
		},
	}

	cm := &apiv1.ConfigMap{
		ObjectMeta: commonMeta,
		Data: map[string]string{
			"program.bt": nj.Program,
		},
	}

	job := &batchv1.Job{
		ObjectMeta: commonMeta,
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   int64Ptr(nj.Deadline + nj.DeadlineGracePeriod),
			TTLSecondsAfterFinished: int32Ptr(5),
			Parallelism:             int32Ptr(1),
			Completions:             int32Ptr(1),
			BackoffLimit:            int32Ptr(1),
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: commonMeta,
				Spec: apiv1.PodSpec{
					HostPID:            true,
					ServiceAccountName: nj.ServiceAccount,
					Volumes: []apiv1.Volume{
						apiv1.Volume{
							Name: "program",
							VolumeSource: apiv1.VolumeSource{
								ConfigMap: &apiv1.ConfigMapVolumeSource{
									LocalObjectReference: apiv1.LocalObjectReference{
										Name: cm.Name,
									},
								},
							},
						},
						apiv1.Volume{
							Name: "usr-host",
							VolumeSource: apiv1.VolumeSource{
								HostPath: &apiv1.HostPathVolumeSource{
									Path: "/usr",
								},
							},
						},
						apiv1.Volume{
							Name: "modules-host",
							VolumeSource: apiv1.VolumeSource{
								HostPath: &apiv1.HostPathVolumeSource{
									Path: "/lib/modules",
								},
							},
						},
						apiv1.Volume{
							Name: "sys",
							VolumeSource: apiv1.VolumeSource{
								HostPath: &apiv1.HostPathVolumeSource{
									Path: "/sys",
								},
							},
						},
					},
					Containers: []apiv1.Container{
						apiv1.Container{
							Name:    nj.Name,
							Image:   nj.ImageNameTag,
							Command: traceCmd,
							TTY:     true,
							Stdin:   true,
							Resources: apiv1.ResourceRequirements{
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse("100m"),
									apiv1.ResourceMemory: resource.MustParse("100Mi"),
								},
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse("1"),
									apiv1.ResourceMemory: resource.MustParse("1G"),
								},
							},
							VolumeMounts: []apiv1.VolumeMount{
								apiv1.VolumeMount{
									Name:      "program",
									MountPath: "/programs",
									ReadOnly:  true,
								},
								apiv1.VolumeMount{
									Name:      "sys",
									MountPath: "/sys",
									ReadOnly:  true,
								},
							},
							SecurityContext: &apiv1.SecurityContext{
								Privileged: boolPtr(true),
							},
							// We want to send SIGINT prior to the pod being killed, so we can print the map
							// we will also wait for an arbitrary amount of time (10s) to give bpftrace time to
							// process and summarize the data
							Lifecycle: &apiv1.Lifecycle{
								PreStop: &apiv1.Handler{
									Exec: &apiv1.ExecAction{
										Command: []string{
											"/bin/bash",
											"-c",
											fmt.Sprintf("kill -SIGINT $(pidof bpftrace) && sleep %s", strconv.FormatInt(nj.DeadlineGracePeriod, 10)),
										},
									},
								},
							},
						},
					},
					RestartPolicy: "Never",
					Affinity: &apiv1.Affinity{
						NodeAffinity: &apiv1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
								NodeSelectorTerms: []apiv1.NodeSelectorTerm{
									apiv1.NodeSelectorTerm{
										MatchExpressions: []apiv1.NodeSelectorRequirement{
											apiv1.NodeSelectorRequirement{
												Key:      "kubernetes.io/hostname",
												Operator: apiv1.NodeSelectorOpIn,
												Values:   []string{nj.Hostname},
											},
										},
									},
								},
							},
						},
					},
					Tolerations: []apiv1.Toleration{
						apiv1.Toleration{
							Effect:   apiv1.TaintEffectNoSchedule,
							Operator: apiv1.TolerationOpExists,
						},
					},
				},
			},
		},
	}

	if nj.FetchHeaders {
		// If we are downloading headers, add the initContainer and set up mounts
		job.Spec.Template.Spec.InitContainers = []apiv1.Container{
			apiv1.Container{
				Name:  "kubectl-trace-init",
				Image: nj.InitImageNameTag,
				Resources: apiv1.ResourceRequirements{
					Requests: apiv1.ResourceList{
						apiv1.ResourceCPU:    resource.MustParse("100m"),
						apiv1.ResourceMemory: resource.MustParse("100Mi"),
					},
					Limits: apiv1.ResourceList{
						apiv1.ResourceCPU:    resource.MustParse("1"),
						apiv1.ResourceMemory: resource.MustParse("1G"),
					},
				},
				VolumeMounts: []apiv1.VolumeMount{
					apiv1.VolumeMount{
						Name:      "lsb-release",
						MountPath: "/etc/lsb-release.host",
						ReadOnly:  true,
					},
					apiv1.VolumeMount{
						Name:      "os-release",
						MountPath: "/etc/os-release.host",
						ReadOnly:  true,
					},
					apiv1.VolumeMount{
						Name:      "modules-dir",
						MountPath: "/lib/modules",
					},
					apiv1.VolumeMount{
						Name:      "modules-host",
						MountPath: "/lib/modules.host",
						ReadOnly:  true,
					},
					apiv1.VolumeMount{
						Name:      "linux-headers-generated",
						MountPath: "/usr/src/",
					},
					apiv1.VolumeMount{
						Name:      "boot-host",
						MountPath: "/boot.host",
					},
				},
			},
		}

		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes,
			apiv1.Volume{
				Name: "lsb-release",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: "/etc/lsb-release",
					},
				},
			},
			apiv1.Volume{
				Name: "os-release",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: "/etc/os-release",
					},
				},
			},
			apiv1.Volume{
				Name: "modules-dir",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: "/var/cache/linux-headers/modules_dir",
					},
				},
			},
			apiv1.Volume{
				Name: "linux-headers-generated",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: "/var/cache/linux-headers/generated",
					},
				},
			},
			apiv1.Volume{
				Name: "boot-host",
				VolumeSource: apiv1.VolumeSource{
					HostPath: &apiv1.HostPathVolumeSource{
						Path: "/boot",
					},
				},
			})

		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts,
			apiv1.VolumeMount{
				Name:      "modules-dir",
				MountPath: "/lib/modules",
				ReadOnly:  true,
			},
			apiv1.VolumeMount{
				Name:      "modules-host",
				MountPath: "/lib/modules.host",
				ReadOnly:  true,
			},
			apiv1.VolumeMount{
				Name:      "linux-headers-generated",
				MountPath: "/usr/src/",
				ReadOnly:  true,
			})

	} else {
		// If we aren't downloading headers, unconditionally used the ones linked in /lib/modules
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts,
			apiv1.VolumeMount{
				Name:      "usr-host",
				MountPath: "/usr-host",
				ReadOnly:  true,
			},
			apiv1.VolumeMount{
				Name:      "modules-host",
				MountPath: "/lib/modules",
				ReadOnly:  true,
			})
	}
	if _, err := t.ConfigClient.Create(context.Background(), cm, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	return t.JobClient.Create(context.Background(), job, metav1.CreateOptions{})
}

func int32Ptr(i int32) *int32 { return &i }
func int64Ptr(i int64) *int64 { return &i }
func boolPtr(b bool) *bool    { return &b }

func jobHostname(j batchv1.Job) (string, error) {
	aff := j.Spec.Template.Spec.Affinity
	if aff == nil {
		return "", fmt.Errorf("affinity not found for job")
	}

	nodeAff := aff.NodeAffinity

	if nodeAff == nil {
		return "", fmt.Errorf("node affinity not found for job")
	}

	requiredScheduling := nodeAff.RequiredDuringSchedulingIgnoredDuringExecution

	if requiredScheduling == nil {
		return "", fmt.Errorf("node affinity RequiredDuringSchedulingIgnoredDuringExecution not found for job")
	}
	nst := requiredScheduling.NodeSelectorTerms
	if len(nst) == 0 {
		return "", fmt.Errorf("node selector terms are empty in node affinity for job")
	}

	me := nst[0].MatchExpressions

	if len(me) == 0 {
		return "", fmt.Errorf("node selector terms match expressions are empty in node affinity for job")
	}

	for _, v := range me {
		if v.Key == "kubernetes.io/hostname" {
			if len(v.Values) == 0 {
				return "", fmt.Errorf("hostname affinity found but no values in it for job")
			}

			return v.Values[0], nil
		}
	}

	return "", fmt.Errorf("hostname not found for job")
}

// TraceJobStatus is a label for the running status of a trace job at the current time.
type TraceJobStatus string

// These are the valid status of traces.
const (
	// TraceJobRunning means the trace job has active pods.
	TraceJobRunning TraceJobStatus = "Running"
	// TraceJobCompleted means the trace job does not have any active pod and has success pods.
	TraceJobCompleted TraceJobStatus = "Completed"
	// TraceJobFailed means the trace job does not have any active or success pod and has fpods that failed.
	TraceJobFailed TraceJobStatus = "Failed"
	// TraceJobUnknown means that for some reason we do not have the information to determine the status.
	TraceJobUnknown TraceJobStatus = "Unknown"
)

func jobStatus(j batchv1.Job) TraceJobStatus {
	if j.Status.Active > 0 {
		return TraceJobRunning
	}
	if j.Status.Succeeded > 0 {
		return TraceJobCompleted
	}
	if j.Status.Failed > 0 {
		return TraceJobFailed
	}
	return TraceJobUnknown
}

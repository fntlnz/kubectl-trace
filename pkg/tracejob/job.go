package tracejob

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TraceJob is a container of info needed to create the job responsible for tracing.
type TraceJob struct {
	Name                string
	ID                  types.UID
	Namespace           string
	ServiceAccount      string
	Tracer              string
	Target              TraceJobTarget
	Selector            string
	Output              string
	Program             string
	ProgramArgs         []string
	ImageNameTag        string
	InitImageNameTag    string
	FetchHeaders        bool
	Deadline            int64
	DeadlineGracePeriod int64
	GoogleAppSecret     string
	StartTime           *metav1.Time
	Status              TraceJobStatus
}

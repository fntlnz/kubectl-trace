package tracejob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "default"
)

type jobSuite struct {
	suite.Suite
	client *TraceJobClient
}

func TestJobSuite(t *testing.T) {
	suite.Run(t, &jobSuite{})
}

func (j *jobSuite) SetupTest() {
	j.client = NewTraceJobClient(fake.NewSimpleClientset(), testNamespace)
}

func (j *jobSuite) TestCreateJob() {
	testJobName := "test-basic-create"
	tj := TraceJob{
		Name: testJobName,
	}

	job, err := j.client.CreateJob(tj)

	assert.Nil(j.T(), err)
	assert.NotNil(j.T(), job)

	joblist, err := j.client.JobClient.List(context.TODO(), metav1.ListOptions{})
	assert.Nil(j.T(), err)
	assert.NotNil(j.T(), joblist)

	assert.Len(j.T(), joblist.Items, 1)
	assert.Equal(j.T(), joblist.Items[0].Spec.Template.Spec.Containers[0].Name, testJobName)
}

func (j *jobSuite) TestCreateJobWithGoogleAppSecret() {
	testJobName := "test-create-with-google-app-secret"
	tj := TraceJob{
		Name:            testJobName,
		GoogleAppSecret: "test-gcp-secret",
	}

	job, err := j.client.CreateJob(tj)

	assert.Nil(j.T(), err)
	assert.NotNil(j.T(), job)

	joblist, err := j.client.JobClient.List(context.TODO(), metav1.ListOptions{})
	assert.Nil(j.T(), err)
	assert.NotNil(j.T(), joblist)

	assert.Len(j.T(), joblist.Items, 1)
	assert.Equal(j.T(), joblist.Items[0].Spec.Template.Spec.Containers[0].Name, testJobName)

	assert.Len(j.T(), joblist.Items[0].Spec.Template.Spec.Containers[0].Env, 1)
	assert.Equal(j.T(), joblist.Items[0].Spec.Template.Spec.Containers[0].Env[0].Name, "GOOGLE_APPLICATION_CREDENTIALS")
}

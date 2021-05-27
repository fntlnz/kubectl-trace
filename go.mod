module github.com/iovisor/kubectl-trace

go 1.15

require (
	cloud.google.com/go/storage v1.12.0
	github.com/creack/pty v1.1.11
	github.com/fntlnz/mountinfo v0.0.0-20171106231217-40cb42681fad
	github.com/fsouza/fake-gcs-server v1.21.2
	github.com/go-logr/logr v0.3.0
	github.com/kr/pretty v0.2.1 // indirect
	github.com/mholt/archiver/v3 v3.5.0
	github.com/prometheus/client_golang v1.7.1
	github.com/spf13/afero v1.3.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	golang.org/x/mod v0.3.0
	golang.org/x/net v0.0.0-20200904194848-62affa334b73
	golang.org/x/sys v0.0.0-20201201145000-ef89a241ccb3 // indirect
	google.golang.org/api v0.33.0
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/cli-runtime v0.19.0-beta.2
	k8s.io/client-go v0.19.2
	k8s.io/kubectl v0.19.0-beta.2
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/kind v0.9.0
)

module github.com/iovisor/kubectl-trace

go 1.12

replace (
	github.com/docker/docker => github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/docker/go-connections => github.com/docker/go-connections v0.3.0
	github.com/docker/go-units => github.com/docker/go-units v0.3.3
)

require (
	cloud.google.com/go v0.34.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/MakeNowJust/heredoc v0.0.0-20171113091838-e9091a26100e // indirect
	github.com/chai2010/gettext-go v0.0.0-20170215093142-bf70f2a70fb1 // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elazarl/goproxy v0.0.0-20190410145444-c548f45dcf1d // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190410145444-c548f45dcf1d // indirect
	github.com/evanphx/json-patch v4.1.0+incompatible // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fntlnz/mountinfo v0.0.0-20171106231217-40cb42681fad
	github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/go-openapi/spec v0.18.0 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/google/btree v0.0.0-20180813153112-4030bb1f1f0c // indirect
	github.com/google/go-cmp v0.2.0 // indirect
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20181110185634-c63ab54fda8f // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/json-iterator/go v1.1.6 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/russross/blackfriday v2.0.0+incompatible // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/sirupsen/logrus v1.3.0 // indirect
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3
	github.com/stretchr/testify v1.3.0 // indirect
	golang.org/x/crypto v0.0.0-20190411191339-88737f569e3a // indirect
	golang.org/x/net v0.0.0-20190404232315-eb5bcb51f2a3 // indirect
	golang.org/x/oauth2 v0.0.0-20181203162652-d668ce993890 // indirect
	golang.org/x/sys v0.0.0-20190412213103-97732733099d // indirect
	golang.org/x/time v0.0.0-20181108054448-85acf8d2951c // indirect
	google.golang.org/appengine v1.5.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.2.2 // indirect
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/cli-runtime v0.0.0-20190409023024-d644b00f3b79
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v0.3.0 // indirect
	k8s.io/kube-openapi v0.0.0-20190401085232-94e1e7b7574c // indirect
	k8s.io/kubernetes v1.14.1
	k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7 // indirect
	sigs.k8s.io/kind v0.0.0-20190405235428-578ac5843ea3
	sigs.k8s.io/kustomize v2.0.3+incompatible // indirect
)

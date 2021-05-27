SHELL=/bin/bash -o pipefail

GO ?= go
DOCKER ?= docker

COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
GIT_COMMIT := $(if $(shell git status --porcelain --untracked-files=no 2> /dev/null),${COMMIT_NO}-dirty,${COMMIT_NO})
GIT_TAG    ?= $(shell git describe 2> /dev/null)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")

GIT_ORG ?= iovisor

DOCKER_BUILD_PROGRESS ?= auto

IMAGE_NAME_OPERATOR ?= quay.io/$(GIT_ORG)/kubectl-trace-operator
IMAGE_NAME_INITCONTAINER ?= quay.io/$(GIT_ORG)/kubectl-trace-init
IMAGE_NAME_TRACERUNNER ?= quay.io/$(GIT_ORG)/kubectl-trace-runner

IMAGE_TRACERUNNER_BRANCH := $(IMAGE_NAME_TRACERUNNER):$(GIT_BRANCH_CLEAN)
IMAGE_TRACERUNNER_COMMIT := $(IMAGE_NAME_TRACERUNNER):$(GIT_COMMIT)
IMAGE_TRACERUNNER_TAG    := $(IMAGE_NAME_TRACERUNNER):$(GIT_TAG)
IMAGE_TRACERUNNER_LATEST := $(IMAGE_NAME_TRACERUNNER):latest

IMAGE_INITCONTAINER_BRANCH := $(IMAGE_NAME_INITCONTAINER):$(GIT_BRANCH_CLEAN)
IMAGE_INITCONTAINER_COMMIT := $(IMAGE_NAME_INITCONTAINER):$(GIT_COMMIT)
IMAGE_INITCONTAINER_TAG    := $(IMAGE_NAME_INITCONTAINER):$(GIT_TAG)
IMAGE_INITCONTAINER_LATEST := $(IMAGE_NAME_INITCONTAINER):latest

IMAGE_OPERATOR_BRANCH := $(IMAGE_NAME_OPERATOR):$(GIT_BRANCH_CLEAN)
IMAGE_OPERATOR_COMMIT := $(IMAGE_NAME_OPERATOR):$(GIT_COMMIT)
IMAGE_OPERATOR_TAG    := $(IMAGE_NAME_OPERATOR):$(GIT_TAG)
IMAGE_OPERATOR_LATEST := $(IMAGE_NAME_OPERATOR):latest

IMAGE_BUILD_FLAGS_EXTRA ?= # convenience to allow to specify extra build flags with env var, defaults to nil

BPFTRACEVERSION ?= "v0.11.1"
BCCVERSION ?= "v0.17.0-focal-release"

LDFLAGS := -ldflags '-X github.com/iovisor/kubectl-trace/pkg/version.buildTime=$(shell date +%s) -X github.com/iovisor/kubectl-trace/pkg/version.gitCommit=${GIT_COMMIT} -X github.com/iovisor/kubectl-trace/pkg/cmd.ImageName=${IMAGE_NAME_TRACERUNNER} -X github.com/iovisor/kubectl-trace/pkg/cmd.ImageTag=${GIT_COMMIT} -X github.com/iovisor/kubectl-trace/pkg/cmd.InitImageName=${IMAGE_NAME_INITCONTAINER} -X github.com/iovisor/kubectl-trace/pkg/cmd.InitImageTag=${GIT_COMMIT}'
TESTPACKAGES := $(shell go list ./... | grep -v github.com/iovisor/kubectl-trace/integration | grep -v github.com/iovisor/kubectl-trace/operator)
TEST_ONLY ?=

kubectl_trace ?= _output/bin/kubectl-trace
trace_runner ?= _output/bin/trace-runner
trace_uploader ?= _output/bin/trace-uploader

# ensure variables are available to child invocations of make
export

.PHONY: build
build: clean ${kubectl_trace}

${kubectl_trace}:
	CGO_ENABLED=1 $(GO) build ${LDFLAGS} -o $@ ./cmd/kubectl-trace

${trace_runner}:
	CGO_ENABLED=1 $(GO) build ${LDFLAGS} -o $@ ./cmd/trace-runner

${trace_uploader}:
	CGO_ENABLED=1 $(GO) build ${LDFLAGS} -o $@ ./cmd/trace-uploader

.PHONY: operator
operator:
	make -C operator

.PHONY: cross
cross:
	IMAGE_NAME_TRACERUNNER=$(IMAGE_NAME_TRACERUNNER) GO111MODULE=on goreleaser --snapshot --rm-dist

.PHONY: release
release:
	IMAGE_NAME_TRACERUNNER=$(IMAGE_NAME_TRACERUNNER) GO111MODULE=on goreleaser --rm-dist

.PHONY: clean
clean:
	$(RM) -R _output
	$(RM) -R dist

.PHONY: image/build-init
image/build-init:
	$(DOCKER) build \
		--progress=$(DOCKER_BUILD_PROGRESS) \
		-t $(IMAGE_INITCONTAINER_BRANCH) \
		-f ./build/Dockerfile.initcontainer \
		${IMAGE_BUILD_FLAGS_EXTRA} \
		./build
	$(DOCKER) tag "$(IMAGE_INITCONTAINER_BRANCH)" "$(IMAGE_INITCONTAINER_COMMIT)"
	$(DOCKER) tag "$(IMAGE_INITCONTAINER_BRANCH)" "$(IMAGE_INITCONTAINER_TAG)"
	$(DOCKER) tag "$(IMAGE_INITCONTAINER_BRANCH)" "$(IMAGE_INITCONTAINER_LATEST)"

.PHONY: image/build
image/build:
	DOCKER_BUILDKIT=1 $(DOCKER) build \
		--build-arg bpftraceversion=$(BPFTRACEVERSION) \
		--build-arg GIT_ORG=$(GIT_ORG) \
		--build-arg bccversion=$(BCCVERSION) \
		--progress=$(DOCKER_BUILD_PROGRESS) \
		--build-arg BUILDKIT_INLINE_CACHE=1 \
		--cache-from "$(IMAGE_TRACERUNNER_BRANCH)" \
		-t "$(IMAGE_TRACERUNNER_BRANCH)" \
		-f build/Dockerfile.tracerunner \
		${IMAGE_BUILD_FLAGS_EXTRA} \
		.
	$(DOCKER) tag "$(IMAGE_TRACERUNNER_BRANCH)" "$(IMAGE_TRACERUNNER_COMMIT)"
	$(DOCKER) tag "$(IMAGE_TRACERUNNER_BRANCH)" "$(IMAGE_TRACERUNNER_TAG)"
	$(DOCKER) tag "$(IMAGE_TRACERUNNER_BRANCH)" "$(IMAGE_TRACERUNNER_LATEST)"

.PHONY: image/build-operator
image/build-operator:
	DOCKER_BUILDKIT=1 $(DOCKER) build \
		--build-arg GIT_ORG=$(GIT_ORG) \
		--progress=$(DOCKER_BUILD_PROGRESS) \
		--build-arg BUILDKIT_INLINE_CACHE=1 \
		--cache-from "$(IMAGE_OPERATOR_BRANCH)" \
		-t "$(IMAGE_OPERATOR_BRANCH)" \
		-f build/Dockerfile.operator \
		${IMAGE_BUILD_FLAGS_EXTRA} \
		.
	$(DOCKER) tag "$(IMAGE_OPERATOR_BRANCH)" "$(IMAGE_OPERATOR_COMMIT)"
	$(DOCKER) tag "$(IMAGE_OPERATOR_BRANCH)" "$(IMAGE_OPERATOR_TAG)"
	$(DOCKER) tag "$(IMAGE_OPERATOR_BRANCH)" "$(IMAGE_OPERATOR_LATEST)"

.PHONY: image/integration-support
image/integration-support: image/ruby-target

.PHONY: image/ruby-target
image/ruby-target:
	make -C build/test/ruby image/ruby-target

.PHONY: image/push
image/push:
	$(DOCKER) push $(IMAGE_TRACERUNNER_BRANCH)
	$(DOCKER) push $(IMAGE_TRACERUNNER_COMMIT)
	$(DOCKER) push $(IMAGE_TRACERUNNER_TAG)
	$(DOCKER) push $(IMAGE_INITCONTAINER_BRANCH)
	$(DOCKER) push $(IMAGE_INITCONTAINER_COMMIT)
	$(DOCKER) push $(IMAGE_INITCONTAINER_TAG)

.PHONY: image/latest
image/latest:
	$(DOCKER) tag $(IMAGE_TRACERUNNER_COMMIT) $(IMAGE_TRACERUNNER_LATEST)
	$(DOCKER) push $(IMAGE_TRACERUNNER_LATEST)
	$(DOCKER) tag $(IMAGE_INITCONTAINER_COMMIT) $(IMAGE_INITCONTAINER_LATEST)
	$(DOCKER) push $(IMAGE_INITCONTAINER_LATEST)

.PHONY: test
test:
	$(GO) test -v -race $(TESTPACKAGES)

.PHONY: integration
integration: build image/build image/integration-support image/build-init image/build-operator
	TEST_KUBECTLTRACE_BINARY=$(shell pwd)/$(kubectl_trace) $(GO) test ${LDFLAGS} -failfast -count=1 -v ./integration/... -run TestKubectlTraceSuite $(if $(TEST_ONLY),-testify.m $(TEST_ONLY),)

.PHONY: operator/test
operator/test:
	make -C operator test

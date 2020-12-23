SHELL=/bin/bash -o pipefail

GO ?= go
DOCKER ?= docker

COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
GIT_COMMIT := $(if $(shell git status --porcelain --untracked-files=no),${COMMIT_NO}-dirty,${COMMIT_NO})
GIT_TAG    ?= $(shell git describe 2> /dev/null)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")

GIT_ORG ?= iovisor

IMAGE_NAME_INIT ?= quay.io/$(GIT_ORG)/kubectl-trace-init
IMAGE_NAME ?= quay.io/$(GIT_ORG)/kubectl-trace-runner

IMAGE_TRACERUNNER_BRANCH := $(IMAGE_NAME):$(GIT_BRANCH_CLEAN)
IMAGE_TRACERUNNER_COMMIT := $(IMAGE_NAME):$(GIT_COMMIT)
IMAGE_TRACERUNNER_TAG    := $(IMAGE_NAME):$(GIT_TAG)
IMAGE_TRACERUNNER_LATEST := $(IMAGE_NAME):latest

IMAGE_INITCONTAINER_BRANCH := $(IMAGE_NAME_INIT):$(GIT_BRANCH_CLEAN)
IMAGE_INITCONTAINER_COMMIT := $(IMAGE_NAME_INIT):$(GIT_COMMIT)
IMAGE_INITCONTAINER_TAG    := $(IMAGE_NAME_INIT):$(GIT_TAG)
IMAGE_INITCONTAINER_LATEST := $(IMAGE_NAME_INIT):latest

IMAGE_BUILD_FLAGS ?= "--no-cache"

BPFTRACEVERSION ?= "v0.11.1"

LDFLAGS := -ldflags '-X github.com/iovisor/kubectl-trace/pkg/version.buildTime=$(shell date +%s) -X github.com/iovisor/kubectl-trace/pkg/version.gitCommit=${GIT_COMMIT} -X github.com/iovisor/kubectl-trace/pkg/cmd.ImageName=${IMAGE_NAME} -X github.com/iovisor/kubectl-trace/pkg/cmd.ImageTag=${GIT_COMMIT} -X github.com/iovisor/kubectl-trace/pkg/cmd.InitImageName=${IMAGE_NAME_INIT} -X github.com/iovisor/kubectl-trace/pkg/cmd.InitImageTag=${GIT_COMMIT}'
TESTPACKAGES := $(shell go list ./... | grep -v github.com/iovisor/kubectl-trace/integration)

kubectl_trace ?= _output/bin/kubectl-trace
trace_runner ?= _output/bin/trace-runner

.PHONY: build
build: clean ${kubectl_trace}

${kubectl_trace}:
	CGO_ENABLED=1 $(GO) build ${LDFLAGS} -mod vendor -o $@ ./cmd/kubectl-trace

${trace_runner}:
	CGO_ENABLED=1 $(GO) build ${LDFLAGS} -mod vendor -o $@ ./cmd/trace-runner

.PHONY: cross
cross:
	IMAGE_NAME=$(IMAGE_NAME) GO111MODULE=on goreleaser --snapshot --rm-dist

.PHONY: release
release:
	IMAGE_NAME=$(IMAGE_NAME) GO111MODULE=on goreleaser --rm-dist

.PHONY: clean
clean:
	$(RM) -R _output
	$(RM) -R dist

.PHONY: image/build-init
image/build-init:
	$(DOCKER) build \
		$(IMAGE_BUILD_FLAGS) \
		-t $(IMAGE_INITCONTAINER_BRANCH) \
		-f ./build/Dockerfile.initcontainer ./build
	$(DOCKER) tag $(IMAGE_INITCONTAINER_BRANCH) $(IMAGE_INITCONTAINER_COMMIT)
	$(DOCKER) tag $(IMAGE_INITCONTAINER_BRANCH) $(IMAGE_INITCONTAINER_TAG)

.PHONY: image/build
image/build:
	$(DOCKER) build \
		--build-arg bpftraceversion=$(BPFTRACEVERSION) \
		--build-arg GIT_ORG=$(GIT_ORG) \
		$(IMAGE_BUILD_FLAGS) \
		-t "$(IMAGE_TRACERUNNER_BRANCH)" \
		-f build/Dockerfile.tracerunner .
	$(DOCKER) tag $(IMAGE_TRACERUNNER_BRANCH) $(IMAGE_TRACERUNNER_COMMIT)
	$(DOCKER) tag "$(IMAGE_TRACERUNNER_BRANCH)" $(IMAGE_TRACERUNNER_BRANCH)
	$(DOCKER) tag "$(IMAGE_TRACERUNNER_BRANCH)" $(IMAGE_TRACERUNNER_TAG)


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
integration:
	TEST_KUBECTLTRACE_BINARY=$(shell pwd)/$(kubectl_trace) $(GO) test ${LDFLAGS} -v ./integration/...

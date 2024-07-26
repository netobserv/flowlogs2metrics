export GOBIN=$(CURDIR)/bin
export PATH:=$(GOBIN):$(PATH)

include .bingo/Variables.mk

export GOROOT=$(shell go env GOROOT)
export GOFLAGS=-mod=vendor
export GO111MODULE=on
export CGO_ENABLED=0
export GOOS=linux

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= latest
BUILD_DATE := $(shell date +%Y-%m-%d\ %H:%M)
TAG_COMMIT := $(shell git rev-list --abbrev-commit --tags --max-count=1)
TAG := $(shell git describe --abbrev=0 --tags ${TAG_COMMIT} 2>/dev/null || true)
BUILD_SHA := $(shell git rev-parse --short HEAD)
BUILD_VERSION := $(TAG:v%=%)
ifneq ($(COMMIT), $(TAG_COMMIT))
	BUILD_VERSION := $(BUILD_VERSION)-$(BUILD_SHA)
endif
ifneq ($(shell git status --porcelain),)
	BUILD_VERSION := $(BUILD_VERSION)-dirty
endif

# Go architecture and targets images to build
GOARCH ?= amd64
MULTIARCH_TARGETS ?= amd64

# In CI, to be replaced by `netobserv`
IMAGE_ORG ?= $(USER)

# IMAGE_TAG_BASE defines the namespace and part of the image name for remote images.
IMAGE_TAG_BASE ?= quay.io/$(IMAGE_ORG)/flowlogs-pipeline

# Image URL to use all building/pushing image targets
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
IMAGE_CACHE ?= $(IMAGE_TAG_BASE)-cache:$(VERSION)
OCI_BUILD_OPTS ?=

# Image building tool (docker / podman) - docker is preferred in CI
OCI_BIN_PATH = $(shell which docker 2>/dev/null || which podman)
OCI_BIN ?= $(shell basename ${OCI_BIN_PATH} 2>/dev/null)

MIN_GO_VERSION := 1.20.0
FLP_BIN_FILE=flowlogs-pipeline
CG_BIN_FILE=confgenerator
K8S_CACHE_BIN_FILE=k8s-cache
NETFLOW_GENERATOR=nflow-generator
CMD_DIR=./cmd/
FLP_CONF_FILE ?= contrib/kubernetes/flowlogs-pipeline.conf.yaml
KIND_CLUSTER_NAME ?= kind

.DEFAULT_GOAL := help

FORCE: ;

# build a single arch target provided as argument
define build_target
	echo 'building image for arch $(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) buildx build --load --build-arg TARGETPLATFORM=linux/$(1) --build-arg TARGETARCH=$(1) --build-arg BUILDPLATFORM=linux/amd64 ${OCI_BUILD_OPTS} -t ${IMAGE}-$(1) -f contrib/docker/Dockerfile .;
	DOCKER_BUILDKIT=1 $(OCI_BIN) buildx build --load --build-arg TARGETPLATFORM=linux/$(1) --build-arg TARGETARCH=$(1) --build-arg BUILDPLATFORM=linux/amd64 ${OCI_BUILD_OPTS} -t ${IMAGE_CACHE}-$(1) -f contrib/docker/cache.Dockerfile .;
endef

# push a single arch target image
define push_target
	echo 'pushing image ${IMAGE}-$(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) push ${IMAGE}-$(1);
	DOCKER_BUILDKIT=1 $(OCI_BIN) push ${IMAGE_CACHE}-$(1);
endef

# manifest create a single arch target provided as argument
define manifest_add_target
	echo 'manifest add target $(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest add ${IMAGE} ${IMAGE}-$(1);
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest add ${IMAGE_CACHE} ${IMAGE_CACHE}-$(1);
endef

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: vendors
vendors: ## Check go vendors
	@echo "### Checking vendors"
	go mod tidy && go mod vendor

.PHONY: validate_go
validate_go:
	@current_ver=$$(go version | { read _ _ v _; echo $${v#go}; }); \
	required_ver=${MIN_GO_VERSION}; min_ver=$$(echo -e "$$current_ver\n$$required_ver" | sort -V | head -n 1); \
	if [[ $$min_ver == $$current_ver ]]; then echo -e "\n!!! golang version > $$required_ver required !!!\n"; exit 7;fi

##@ Develop

.PHONY: validate_go lint
lint: $(GOLANGCI_LINT) ## Lint the code
	$(GOLANGCI_LINT) run ./... --timeout=3m

.PHONY: build_code
build_code:
	GOARCH=${GOARCH} go build -ldflags "-X 'main.BuildVersion=$(BUILD_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'" "${CMD_DIR}${FLP_BIN_FILE}"
	GOARCH=${GOARCH} go build -ldflags "-X 'main.BuildVersion=$(BUILD_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'" "${CMD_DIR}${CG_BIN_FILE}"

.PHONY: build_k8s_cache
build_k8s_cache:
	GOARCH=${GOARCH} go build -ldflags "-X 'main.BuildVersion=$(BUILD_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'" "${CMD_DIR}${K8S_CACHE_BIN_FILE}"

.PHONY: build
build: validate_go lint build_code build_k8s_cache docs ## Build flowlogs-pipeline executables and update the docs

.PHONY: docs
docs: FORCE ## Update flowlogs-pipeline documentation
	@./hack/update-docs.sh
	@go run cmd/apitodoc/main.go > docs/api.md
	@./hack/update-enum-docs.sh
	@go run cmd/operationalmetricstodoc/main.go > docs/operational-metrics.md

.PHONY: clean
clean: ## Clean
	rm -f "${FLP_BIN_FILE}"
	go clean ./...

TEST_OPTS := -race -coverpkg=./... -covermode=atomic -coverprofile cover.out
.PHONY: tests-unit
tests-unit: validate_go ## Unit tests
	# tests may rely on non-thread safe libs such as go-ipfix => no -race flag
	go test $$(go list ./... | grep /testnorace)
	# enabling CGO is required for -race flag
	CGO_ENABLED=1 go test -p 1 $(TEST_OPTS) $$(go list ./... | grep -v /e2e | grep -v /testnorace)

.PHONY: coverage-report
coverage-report: ## Generate coverage report
	@echo "### Generating coverage report"
	go tool cover --func=./cover.out

.PHONY: coverage-report-html
coverage-report-html: ## Generate HTML coverage report
	@echo "### Generating HTML coverage report"
	go tool cover --html=./cover.out

.PHONY: tests-fast
tests-fast: TEST_OPTS=
tests-fast: tests-unit ## Fast unit tests (no race tests / coverage)

.PHONY: tests-e2e
tests-e2e: validate_go $(KIND)  ## End-to-end tests
	go test -p 1 -v -timeout 20m $$(go list ./... | grep  /e2e)

.PHONY: tests-all
tests-all: validate_go tests-unit tests-e2e ## All tests

# note: to review profile execute: go tool pprof -web /tmp/flowlogs-pipeline-cpu-profile.out (make sure graphviz is installed)
.PHONY: benchmarks
benchmarks: $(BENCHSTAT) validate_go ## Benchmark
	go test -bench=. ./cmd/flowlogs-pipeline -o=/tmp/flowlogs-pipeline.test \
	-cpuprofile /tmp/flowlogs-pipeline-cpu-profile.out \
	-run=^# -count=10 -parallel=1 -cpu=1 -benchtime=100x \
	 | tee /tmp/flowlogs-pipeline-benchmark.txt
	 $(BENCHSTAT) /tmp/flowlogs-pipeline-benchmark.txt

.PHONY: run
run: build ## Run
	./"${FLP_BIN_FILE}"

##@ Images

# note: to build and push custom image tag use: IMAGE_ORG=myuser VERSION=dev make images
.PHONY: image-build
image-build: ## Build MULTIARCH_TARGETS images
	trap 'exit' INT; \
	$(foreach target,$(MULTIARCH_TARGETS),$(call build_target,$(target)))

.PHONY: image-push
image-push: ## Push MULTIARCH_TARGETS images
	trap 'exit' INT; \
	$(foreach target,$(MULTIARCH_TARGETS),$(call push_target,$(target)))

.PHONY: manifest-build
manifest-build: ## Build MULTIARCH_TARGETS manifest
	@echo 'building manifest $(IMAGE)'
	DOCKER_BUILDKIT=1 $(OCI_BIN) rmi ${IMAGE} -f || true
	DOCKER_BUILDKIT=1 $(OCI_BIN) rmi ${IMAGE_CACHE} -f || true
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest create ${IMAGE} $(foreach target,$(MULTIARCH_TARGETS), --amend ${IMAGE}-$(target));
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest create ${IMAGE_CACHE} $(foreach target,$(MULTIARCH_TARGETS), --amend ${IMAGE_CACHE}-$(target));

.PHONY: manifest-push
manifest-push: ## Push MULTIARCH_TARGETS manifest
	@echo 'publish manifest $(IMAGE)'
ifeq (${OCI_BIN}, docker)
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE};
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE_CACHE};
else
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE} docker://${IMAGE};
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE_CACHE} docker://${IMAGE_CACHE};
endif

include .mk/development.mk
include .mk/shortcuts.mk

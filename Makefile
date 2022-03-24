# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 2.5.0-beta.6

# MIN_KUBE_VERSION is the build flag of minimum Kubernetes version.
MIN_KUBE_VERSION ?= 1.19.0

# Generate kuttl e2e tests for the following storageos/kind-node versions
# TEST_KIND_NODES is not intended to be updated manually.
# Please run 'LATEST_KIND_NODE=<latest-kind-node> make update-kind-nodes'.
TEST_KIND_NODES ?= 1.19.0,1.20.5,1.21.0,1.22.3,1.23.0

REPO ?= operator

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS=stable
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_PACKAGE_NAME ?= --package=storageosoperator
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL) $(BUNDLE_PACKAGE_NAME)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# my.domain/newop-bundle:$VERSION and my.domain/newop-catalog:$VERSION.
IMAGE_TAG_BASE ?= storageos/operator

# BUNDLE_IMAGE defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMAGE=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMAGE ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# Image URL to use all building/pushing image targets
OPERATOR_IMAGE ?= storageos/operator:test

# Image URL for manifest image.
MANIFESTS_IMAGE ?= storageos/operator-manifests:test

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif


LD_FLAGS = -X main.SupportedMinKubeVersion=$(MIN_KUBE_VERSION)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
# Refer: https://github.com/operator-framework/operator-sdk/issues/4203.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

API_MANAGER_VERSION ?= develop
API_MANAGER_IMAGE ?= storageos/api-manager:$(API_MANAGER_VERSION)
API_MANAGER_MANIFESTS_IMAGE ?= storageos/api-manager-manifests:$(API_MANAGER_VERSION)
PORTAL_MANAGER_VERSION ?= develop
PORTAL_MANAGER_IMAGE ?= storageos/portal-manager:$(PORTAL_MANAGER_VERSION)
PORTAL_MANAGER_MANIFESTS_IMAGE ?= storageos/portal-manager-manifests:$(PORTAL_MANAGER_VERSION)
EXTERNAL_PROVISIONER_IMAGE ?= storageos/csi-provisioner:v2.1.1-patched
EXTERNAL_ATTACHER_IMAGE ?= quay.io/k8scsi/csi-attacher:v3.1.0
EXTERNAL_RESIZER_IMAGE ?= quay.io/k8scsi/csi-resizer:v1.1.0
INIT_IMAGE ?= storageos/init:v2.1.2
NODE_IMAGE ?= storageos/node:v2.5.0
NODE_MANAGER_VERSION ?= develop
NODE_MANAGER_IMAGE ?= storageos/node-manager:$(NODE_MANAGER_VERSION)
NODE_MANAGER_MANIFESTS_IMAGE ?= storageos/node-manager-manifests:$(NODE_MANAGER_VERSION)
UPGRADE_GUARD_IMAGE ?= storageos/upgrade-guard:develop
NODE_DRIVER_REG_IMAGE ?= quay.io/k8scsi/csi-node-driver-registrar:v2.1.0
LIVENESS_PROBE_IMAGE ?= quay.io/k8scsi/livenessprobe:v2.2.0

# The related image environment variables. These are used in the opreator's
# configuration by converting into a ConfigMap and loading as a container's
# environment variables. See make target cofig-update.
define REL_IMAGE_CONF
RELATED_IMAGE_API_MANAGER=${API_MANAGER_IMAGE}
RELATED_IMAGE_PORTAL_MANAGER=${PORTAL_MANAGER_IMAGE}
RELATED_IMAGE_CSIV1_EXTERNAL_PROVISIONER=${EXTERNAL_PROVISIONER_IMAGE}
RELATED_IMAGE_CSIV1_EXTERNAL_ATTACHER_V3=${EXTERNAL_ATTACHER_IMAGE}
RELATED_IMAGE_CSIV1_EXTERNAL_RESIZER=${EXTERNAL_RESIZER_IMAGE}
RELATED_IMAGE_STORAGEOS_INIT=${INIT_IMAGE}
RELATED_IMAGE_STORAGEOS_NODE=${NODE_IMAGE}
RELATED_IMAGE_NODE_MANAGER=${NODE_MANAGER_IMAGE}
RELATED_IMAGE_UPGRADE_GUARD=${UPGRADE_GUARD_IMAGE}
RELATED_IMAGE_CSIV1_NODE_DRIVER_REGISTRAR=${NODE_DRIVER_REG_IMAGE}
RELATED_IMAGE_CSIV1_LIVENESS_PROBE=${LIVENESS_PROBE_IMAGE}
endef
export REL_IMAGE_CONF

_env:
	env
	
all: build

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

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } /^##@/{ printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen config-update ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	make _manifests

_manifests:
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=storageos-operator webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac/bases

generate: controller-gen mockgen ## Generate code containing DeepCopy, DeepCopyInto, DeepCopyObject method implementations and mocks.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	go generate -v ./...

api-manager:
	NAME=api-manager VERSION=$(API_MANAGER_VERSION) MANIFESTS_IMAGE=$(API_MANAGER_MANIFESTS_IMAGE) hack/pull-manifests.sh
	NAME=api-manager VERSION=$(API_MANAGER_VERSION) FROM=clusterrole-storageos-api-manager.yaml TO=api_manager_role.yaml hack/update-rbac.sh
	make config-update

portal-manager:
	NAME=portal-manager VERSION=$(PORTAL_MANAGER_VERSION) MANIFESTS_IMAGE=$(PORTAL_MANAGER_MANIFESTS_IMAGE) hack/pull-manifests.sh
	NAME=portal-manager VERSION=$(PORTAL_MANAGER_VERSION) FROM=clusterrole-storageos-portal-manager.yaml TO=portal_manager_role.yaml hack/update-rbac.sh
	make config-update

node-manager:
	NAME=node-manager VERSION=$(NODE_MANAGER_VERSION) MANIFESTS_IMAGE=$(NODE_MANAGER_MANIFESTS_IMAGE) hack/pull-manifests.sh
	NAME=node-manager VERSION=$(NODE_MANAGER_VERSION) FROM=clusterrole-storageos-node-manager.yaml TO=node_manager_role.yaml hack/update-rbac.sh
	make config-update

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: manifests generate fmt vet ## Run tests.
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

.PHONY: e2e
e2e: ## Run e2e tests.
	kubectl-kuttl test --config e2e/kuttl/operator-deployment-1.22.yaml

update-kind-nodes:
ifndef LATEST_KIND_NODE
	$(error LATEST_KIND_NODE is undefined)
endif
	LATEST_KIND_NODE=$(LATEST_KIND_NODE) ./hack/update-kind-nodes.sh

generate-tests: ## Generate kuttl e2e tests
	TEST_KIND_NODES=$(TEST_KIND_NODES) REPO=$(REPO) ./hack/generate-tests.sh

##@ Build

build: manifests generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 go build -ldflags "$(LD_FLAGS)" -o bin/manager main.go

run: generate fmt vet manifests ## Run a controller from your host.
	RELATED_IMAGE_API_MANAGER=${API_MANAGER_IMAGE} \
	RELATED_IMAGE_PORTAL_MANAGER=${PORTAL_MANAGER_IMAGE} \
	RELATED_IMAGE_CSIV1_EXTERNAL_PROVISIONER=${EXTERNAL_PROVISIONER_IMAGE} \
	RELATED_IMAGE_CSIV1_EXTERNAL_ATTACHER_V3=${EXTERNAL_ATTACHER_IMAGE} \
	RELATED_IMAGE_CSIV1_EXTERNAL_RESIZER=${EXTERNAL_RESIZER_IMAGE} \
	RELATED_IMAGE_STORAGEOS_INIT=${INIT_IMAGE} \
	RELATED_IMAGE_STORAGEOS_NODE=${NODE_IMAGE} \
	RELATED_IMAGE_NODE_MANAGER=${NODE_MANAGER_IMAGE} \
	RELATED_IMAGE_UPGRADE_GUARD=${UPGRADE_GUARD_IMAGE} \
	RELATED_IMAGE_CSIV1_NODE_DRIVER_REGISTRAR=${NODE_DRIVER_REG_IMAGE} \
	RELATED_IMAGE_CSIV1_LIVENESS_PROBE=${LIVENESS_PROBE_IMAGE} \
	POD_NAMESPACE=default \
	go run ./main.go

config-update: ## Update the operator configuration.
	@echo "$$REL_IMAGE_CONF" > config/manager/related_images_config.yaml

# Build the docker image
operator-image: ## Build docker image with the manager.
	docker build -t ${OPERATOR_IMAGE} --build-arg VERSION=$(VERSION)  .

operator-image-push: ## Push docker image with the manager.
	docker push ${OPERATOR_IMAGE}

# Build the manifests docker image.
manifests-image: manifests
	docker build -t $(MANIFESTS_IMAGE) --build-arg OPERATOR_IMAGE=$(OPERATOR_IMAGE) -f manifests.Dockerfile .

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

install-manifest: manifests kustomize ## Generate the operator install manifest.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	$(KUSTOMIZE) build config/default > storageos-operator.yaml

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

MOCKGEN = $(shell pwd)/bin/mockgen
mockgen:
	$(call go-get-tool,$(MOCKGEN),github.com/golang/mock/mockgen@latest)

OPERATOR_SDK_DOWNLOAD_URL = "https://github.com/operator-framework/operator-sdk/releases/download/v1.7.1/operator-sdk_linux_amd64"
ifeq ($(shell uname | tr '[:upper:]' '[:lower:]'), darwin)
OPERATOR_SDK_DOWNLOAD_URL = "https://github.com/operator-framework/operator-sdk/releases/download/v1.7.1/operator-sdk_darwin_amd64"
endif

OPERATOR_SDK = $(shell pwd)/bin/operator-sdk
operator-sdk:
	$(call curl-get-tool,$(OPERATOR_SDK), $(OPERATOR_SDK_DOWNLOAD_URL))

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# curl-get-tool will download any content of $2 and install it to $1.
define curl-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
echo "Downloading $(2)" ;\
curl -Lo $(1) $(2) ;\
chmod +x $(1) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: operator-sdk manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests --package=storageosoperator --apis-dir=api/v1 -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	# Rename manifest files containing colons.
	for file in bundle/manifests/* ; do if [[ $$file == *:* ]]; then mv "$$file" "$${file//:/_}"; fi; done
	$(OPERATOR_SDK) bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMAGE) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) operator-image-push OPERATOR_IMAGE=$(BUNDLE_IMAGE)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMAGES=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMAGES ?= $(BUNDLE_IMAGE)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMAGE=example.com/operator-catalog:v0.2.0).
CATALOG_IMAGE ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMAGE to an existing catalog image tag to add $BUNDLE_IMAGES to that image.
ifneq ($(origin CATALOG_BASE_IMAGE), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMAGE)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMAGE) --bundles $(BUNDLE_IMAGES) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) operator-image-push OPERATOR_IMAGE=$(CATALOG_IMAGE)

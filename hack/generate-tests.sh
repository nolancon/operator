#!/bin/bash

: "${TEST_KIND_NODES?= required}"
: "${REPO?= required}"

# This script reads a comma-delimited string TEST_KIND_NODES of storageos/kind-node versions
# for kuttl tests to be run on, and generates the relevant files for each version.

IFS=', ' read -r -a kind_nodes <<< "$TEST_KIND_NODES"


# remove existing files
rm -f ./e2e/kind/*
rm -f ./e2e/kuttl/*
rm -f ./.github/workflows/kuttl*



for kind_node in "${kind_nodes[@]}"
do
	# write kind config file for version
	major=${kind_node%.*}
	if [ ! -d "./e2e/kind" ]; then
		mkdir -p ./e2e/kind
	fi
	file=./e2e/kind/kind-config-${major}.yaml

	cat <<EOF > "${file}"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha3
nodes:
- role: control-plane
  image: storageos/kind-node:v${kind_node}
EOF
	# write kuttl config file for version (deployment)
	if [ ! -d "./e2e/kuttl" ]; then
		mkdir -p ./e2e/kuttl
	fi
	file=./e2e/kuttl/${REPO}-deployment-${major}.yaml

	# deployment tests always use 'stable' testDir
	cat <<EOF > "${file}"
apiVersion: kuttl.dev/v1beta1
kind: TestSuite
testDirs:
- ./e2e/tests/deployment/stable
kindConfig: e2e/kind/kind-config-${major}.yaml
startKIND: true
kindNodeCache: true
kindContainers:
  - storageos/operator:test
timeout: 800
EOF

	# write kuttl github action for version
	if [ ! -d "./.github/workflows" ]; then
		mkdir -p ./.github/workflows
	fi
	file=./.github/workflows/kuttl-e2e-test-${major}.yaml

	cat <<EOF > "${file}"
name: kuttl e2e test ${major}

on: [push, pull_request]

jobs:
  test:
    name: kuttl e2e test ${major}
    runs-on: ubuntu-18.04
    env:
      KUTTL: /usr/local/bin/kubectl-kuttl
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.16'
      - name: Install kuttl and kubectl
        run: |
          sudo curl -Lo \$KUTTL https://github.com/kudobuilder/kuttl/releases/download/v0.11.1/kubectl-kuttl_0.11.1_linux_x86_64
          sudo chmod +x \$KUTTL
          sudo curl -LO https://dl.k8s.io/release/\$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl
          sudo chmod +x kubectl
      - name: Build and prepare manifests
        run: make manifests
      - name: Build container
        run: make operator-image
      - name: Run kuttl ${major}
        run: sudo kubectl-kuttl test --config e2e/kuttl/${REPO}-deployment-${major}.yaml
EOF

done

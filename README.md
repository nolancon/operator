# StorageOS Operator

The StorageOS Operator deploys and configures a StorageOS cluster on Kubernetes.

[![Go Report Card](https://goreportcard.com/badge/github.com/storageos/operator)](https://goreportcard.com/report/github.com/storageos/operator)
[![e2e test](https://github.com/storageos/operator/actions/workflows/kuttl-e2e-test.yaml/badge.svg)](https://github.com/storageos/operator/actions/workflows/kuttl-e2e-test.yaml)
[![Lint and test](https://github.com/storageos/operator/actions/workflows/test.yml/badge.svg)](https://github.com/storageos/operator/actions/workflows/test.yml)
[![CodeQL](https://github.com/storageos/operator/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/storageos/operator/actions/workflows/codeql-analysis.yml)
[![Active](http://img.shields.io/badge/Status-Active-green.svg)](https://github.com/storageos/operator)
[![PR's Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat)](https://github.com/storageos/operator/pulls)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

## Setup/Development

1. Build operator container image with `make operator-image`. Publish or copy
   the container image to an existing k8s cluster to make it available for use
   within the cluster.
2. Generate install manifest file with `make install-manifest`. This will
   generate `storageos-operator.yaml` file.
3. Install the operator with `kubectl create -f storageos-operator.yaml`.

The operator can also be run from outside the cluster with `make run`. Ensure
the CRDs that the operator requires are installed in the cluster before running
it using `make install`.

### Install StorageOS cluster

1. Ensure an etcd cluster is available to be used with StorageOS.
2. Create a secret for the cluster, for example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storageos-api
  namespace: storageos
  labels:
    app: storageos
data:
  # echo -n '<secret>' | base64
  username: c3RvcmFnZW9z
  password: c3RvcmFnZW9z
```

3. Create a `StorageOSCluster` custom resource in the same namespace as the
above secret and refer the secret in the spec:

```yaml
apiVersion: storageos.com/v1
kind: StorageOSCluster
metadata:
  name: storageoscluster-sample
  namespace: storageos
spec:
  secretRefName: storageos-api
  storageClassName: storageos
  kvBackend:
    address: "<etcd-address>"
```

This will create a StorageOS cluster in `storageos` namespace with a
StorageClass `storageos` that can be used to provision StorageOS volumes.

## Testing

Run the unit tests with `make test`.

Run e2e tests with `make e2e`. e2e tests use [kuttl](https://kuttl.dev/).
Install kuttl kubectl plugin before running the e2e tests.

## Update api-manager manifests

Api-manager manifests are stored in the repo but updated manually. To update the manifests please follow the instructions below.

* Edit `API_MANAGER_VERSION` in `Makefile` or pass it to make target.
* Execute `make api-manager` command.
* Enjoy new version.

## Update portal-manager manifests

portal-manager manifests are stored in the repo but updated manually. To update the manifests please follow the instructions below.

* Edit `PORTAL_MANAGER_VERSION` in `Makefile` or pass it to make target.
* Execute `make portal-manager` command.
* Enjoy new version.

## Update node-manager manifests

node-manager manifests are stored in the repo but updated manually. To update the manifests please follow the instructions below.

* Edit `NODE_MANAGER_VERSION` in `Makefile` or pass it to make target.
* Execute `make node-manager` command.
* Enjoy new version.

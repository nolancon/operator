module github.com/storageos/operator

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/darkowlzz/operator-toolkit v0.0.0-20210721205719-05b03cd74f02
	github.com/go-logr/logr v0.3.0
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/storageos/go-api v0.0.0-20220209143821-59d68c680d51
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.15.0
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/cli-runtime v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog/v2 v2.4.0
	sigs.k8s.io/controller-runtime v0.8.3
	sigs.k8s.io/kustomize/api v0.7.1
	sigs.k8s.io/kustomize/kyaml v0.10.5
	sigs.k8s.io/yaml v1.2.0
)

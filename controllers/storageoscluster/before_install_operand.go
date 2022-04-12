package storageoscluster

import (
	"context"
	"fmt"

	"github.com/ondat/operator-toolkit/declarative"
	"github.com/ondat/operator-toolkit/declarative/kubectl"
	eventv1 "github.com/ondat/operator-toolkit/event/v1"
	"github.com/ondat/operator-toolkit/operator/v1/operand"
	storageoscomv1 "github.com/storageos/operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
)

// beforeInstallPackage contains the resource manifests for beforeInstall
// operand.
const beforeInstallPackage = "before-install"

type BeforeInstallOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient
}

var _ operand.Operand = &BeforeInstallOperand{}

func (bi *BeforeInstallOperand) Name() string                             { return bi.name }
func (bi *BeforeInstallOperand) Requires() []string                       { return bi.requires }
func (bi *BeforeInstallOperand) RequeueStrategy() operand.RequeueStrategy { return bi.requeueStrategy }
func (bi *BeforeInstallOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	return true, nil
}
func (c *BeforeInstallOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (bi *BeforeInstallOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "BeforeInstallOperand.Ensure")
	defer span.End()

	if err := deleteNodeIfNewEnvDetected(ctx, obj, bi.client); err != nil {
		return nil, err
	}

	b, err := getBeforeInstallBuilder(bi.fs, obj, bi.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Apply(ctx)
}

func (bi *BeforeInstallOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "BeforeInstallOperand.Delete")
	defer span.End()

	b, err := getBeforeInstallBuilder(bi.fs, obj, bi.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Delete(ctx)
}

func getBeforeInstallBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	return declarative.NewBuilder(beforeInstallPackage, fs,
		declarative.WithKubectlClient(kcl),
	)
}

func deleteNodeIfNewEnvDetected(ctx context.Context, obj client.Object, cl client.Client) error {
	ctx, span, _, log := instrumentation.Start(ctx, "BeforeInstallOperand.deleteNodeIfNewEnvDetected")
	defer span.End()

	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	envVars := failoverPolicyToImplement(cluster.Spec.NodeFailoverPolicy)
	if envVars == nil {
		return nil
	}

	log.Info("Check environment variables of node failover policy against those in node configmap")

	nodeConfigMap := &corev1.ConfigMap{}
	err := cl.Get(ctx, types.NamespacedName{
		Name:      "storageos-node",
		Namespace: obj.GetNamespace(),
	}, nodeConfigMap)
	if err != nil && !apierrors.IsNotFound(err) {
		span.RecordError(err)
		return err
	}

	if nodeConfigMap.Data[nodeFailoverPolicyEnvVar] == envVars[nodeFailoverPolicyEnvVar] {
		return nil
	}

	log.Info("Delete node daemonset to enforce new environment variables set by failover policy")

	err = cl.Delete(ctx, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storageos-node",
			Namespace: obj.GetNamespace(),
		},
	}, &client.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		span.RecordError(err)
		return err
	}

	return nil
}

func NewBeforeInstallOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *BeforeInstallOperand {
	return &BeforeInstallOperand{
		name:            name,
		client:          client,
		requires:        requires,
		requeueStrategy: requeueStrategy,
		fs:              fs,
		kubectlClient:   kcl,
	}
}

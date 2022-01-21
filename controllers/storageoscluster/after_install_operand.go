package storageoscluster

import (
	"context"
	"fmt"

	"github.com/darkowlzz/operator-toolkit/declarative"
	"github.com/darkowlzz/operator-toolkit/declarative/kubectl"
	"github.com/darkowlzz/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/darkowlzz/operator-toolkit/event/v1"
	"github.com/darkowlzz/operator-toolkit/operator/v1/operand"
	storageoscomv1 "github.com/storageos/operator/api/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/types"
)

// afterInstallPackage contains the resource manifests for afterInstall
// operand.
const afterInstallPackage = "after-install"

type AfterInstallOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient

	podDisruptionBudgetDeployed *bool
}

var _ operand.Operand = &AfterInstallOperand{}

func (ai *AfterInstallOperand) Name() string                             { return ai.name }
func (ai *AfterInstallOperand) Requires() []string                       { return ai.requires }
func (ai *AfterInstallOperand) RequeueStrategy() operand.RequeueStrategy { return ai.requeueStrategy }
func (ai *AfterInstallOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	return true, nil
}
func (c *AfterInstallOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (ai *AfterInstallOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	ctx, span, _, log := instrumentation.Start(ctx, "AfterInstallOperand.Ensure")
	defer span.End()

	resources := []string{}
	if _, ok := cluster.Spec.NodeManagerFeatures[upgradeGuardFeatureKey]; !ok {
		log.Info("Upgrade guard feature is disabled")
		if ai.podDisruptionBudgetDeployed == nil || *ai.podDisruptionBudgetDeployed {
			log.Info("Delete Pod Disruption Budget")
			err := ai.client.Delete(ctx, &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storageos-upgrade-guard",
					Namespace: obj.GetNamespace(),
				},
			}, &client.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				span.RecordError(err)
				return nil, err
			}
		}

		f := false
		ai.podDisruptionBudgetDeployed = &f
	} else {
		log.Info("Upgrade guard feature is enabled")
		resources = append(resources, "pod-disruption-budget.yaml")

		t := true
		ai.podDisruptionBudgetDeployed = &t
	}

	b, err := getAfterInstallBuilder(ai.fs, obj, ai.kubectlClient, resources)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Apply(ctx)
}

func (ai *AfterInstallOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	ctx, span, _, log := instrumentation.Start(ctx, "AfterInstallOperand.Delete")
	defer span.End()

	resources := []string{}
	if _, ok := cluster.Spec.NodeManagerFeatures[upgradeGuardFeatureKey]; ok {
		log.Info("Upgrade guard feature is enabled")
		resources = append(resources, "pod-disruption-budget.yaml")
	}

	b, err := getAfterInstallBuilder(ai.fs, obj, ai.kubectlClient, resources)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Delete(ctx)
}

func getAfterInstallBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient, resources []string) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	mutators := []kustomize.MutateFunc{
		kustomize.AddNamespace(cluster.GetNamespace()),
		func(k *types.Kustomization) {
			k.Resources = append(k.Resources, resources...)
		},
	}

	return declarative.NewBuilder(afterInstallPackage, fs,
		declarative.WithKustomizeMutationFunc(mutators),
		declarative.WithKubectlClient(kcl),
	)
}

func NewAfterInstallOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *AfterInstallOperand {
	return &AfterInstallOperand{
		name:            name,
		client:          client,
		requires:        requires,
		requeueStrategy: requeueStrategy,
		fs:              fs,
		kubectlClient:   kcl,
	}
}

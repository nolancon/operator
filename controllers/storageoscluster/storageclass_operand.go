package storageoscluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkowlzz/operator-toolkit/declarative"
	"github.com/darkowlzz/operator-toolkit/declarative/kubectl"
	"github.com/darkowlzz/operator-toolkit/declarative/transform"
	eventv1 "github.com/darkowlzz/operator-toolkit/event/v1"
	"github.com/darkowlzz/operator-toolkit/operator/v1/operand"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"

	storageoscomv1 "github.com/storageos/operator/api/v1"
	stransform "github.com/storageos/operator/internal/transform"
)

const (
	// storageclassPackage contains the resource manifests for storageclass
	// operand.
	storageclassPackage = "storageclass"

	// csiSecretNameKey is the StorageClass parameter key for CSI secret name.
	csiSecretNameKey = "csi.storage.k8s.io/secret-name"

	// csiSecretNamespaceKey is the StorageClass parameter key for CSI secret
	// namespace.
	csiSecretNamespaceKey = "csi.storage.k8s.io/secret-namespace"

	// storageClassParameterPath is the path to StorageClass parameters.
	storageClassParametersPath = "parameters"
)

type StorageClassOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient
}

var _ operand.Operand = &StorageClassOperand{}

func (sc *StorageClassOperand) Name() string                             { return sc.name }
func (sc *StorageClassOperand) Requires() []string                       { return sc.requires }
func (sc *StorageClassOperand) RequeueStrategy() operand.RequeueStrategy { return sc.requeueStrategy }
func (sc *StorageClassOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	return true, nil
}
func (c *StorageClassOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (sc *StorageClassOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	_, span, _, log := instrumentation.Start(ctx, "StorageClassOperand.Ensure")
	defer span.End()

	b, err := getStorageClassBuilder(sc.fs, obj, sc.kubectlClient)
	if err != nil {
		if errors.Is(err, errNoResource) {
			log.Info("no storageclass specified")
			return nil, nil
		}
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Apply(ctx)
}

func (sc *StorageClassOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "StorageClassOperand.Delete")
	defer span.End()

	b, err := getStorageClassBuilder(sc.fs, obj, sc.kubectlClient)
	if err != nil {
		if errors.Is(err, errNoResource) {
			return nil, nil
		}
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Delete(ctx)
}

func getStorageClassBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	// Skip if no StorageClass name is provided.
	if cluster.Spec.StorageClassName == "" {
		return nil, errNoResource
	}

	// StorageClass transforms.
	scTransforms := []transform.TransformFunc{}

	// Set the StorageClass name.
	nameTF := stransform.SetMetadataNameFunc(cluster.Spec.StorageClassName)

	// Set secret reference.
	secretNameTF := stransform.SetScalarNodeStringValueFunc(csiSecretNameKey, cluster.Spec.SecretRefName, storageClassParametersPath)
	secretNamespaceTF := stransform.SetScalarNodeStringValueFunc(csiSecretNamespaceKey, cluster.Namespace, storageClassParametersPath)

	scTransforms = append(scTransforms, nameTF, secretNameTF, secretNamespaceTF)

	return declarative.NewBuilder(storageclassPackage, fs,
		declarative.WithManifestTransform(transform.ManifestTransform{
			"storageclass/storageclass.yaml": scTransforms,
		}),
		declarative.WithKubectlClient(kcl),
	)
}

func NewStorageClassOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *StorageClassOperand {
	return &StorageClassOperand{
		name:            name,
		client:          client,
		requires:        requires,
		requeueStrategy: requeueStrategy,
		fs:              fs,
		kubectlClient:   kcl,
	}
}

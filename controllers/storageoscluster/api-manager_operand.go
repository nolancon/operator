package storageoscluster

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/darkowlzz/operator-toolkit/declarative"
	"github.com/darkowlzz/operator-toolkit/declarative/kubectl"
	"github.com/darkowlzz/operator-toolkit/declarative/kustomize"
	"github.com/darkowlzz/operator-toolkit/declarative/transform"
	eventv1 "github.com/darkowlzz/operator-toolkit/event/v1"
	"github.com/darkowlzz/operator-toolkit/operator/v1/operand"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/api/v1"
	"github.com/storageos/operator/internal/image"
	stransform "github.com/storageos/operator/internal/transform"
)

const (
	// apiManagerPackage contains the resource manifests for api-manager
	// operand.
	apiManagerPackage = "api-manager"

	// Kustomize image name for container image.
	kImageAPIManager = "api-manager"

	// Related image environment variable.
	apiManagerImageEnvVar = "RELATED_IMAGE_API_MANAGER"
)

type APIManagerOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient
}

var _ operand.Operand = &APIManagerOperand{}

func (c *APIManagerOperand) Name() string                             { return c.name }
func (c *APIManagerOperand) Requires() []string                       { return c.requires }
func (c *APIManagerOperand) RequeueStrategy() operand.RequeueStrategy { return c.requeueStrategy }

func (c *APIManagerOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	ctx, span, _, log := instrumentation.Start(ctx, "APIManagerOperand.ReadyCheck")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Get the deployment object and check status of the replicas. One ready
	// replica should be enough for the installation to continue.
	amDep := &appsv1.Deployment{}
	key := client.ObjectKey{Name: "storageos-api-manager", Namespace: obj.GetNamespace()}
	if err := c.client.Get(ctx, key, amDep); err != nil {
		return false, err
	}

	if amDep.Status.AvailableReplicas > 0 {
		log.Info("Found available replicas more than 0", "availableReplicas", amDep.Status.AvailableReplicas)
		return true, nil
	}

	log.Info("api-manager not ready")
	return false, nil
}

func (c *APIManagerOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (c *APIManagerOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "APIManagerOperand.Ensure")
	defer span.End()

	b, err := getAPIManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Apply(ctx)
}

func (c *APIManagerOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "APIManagerOperand.Delete")
	defer span.End()

	b, err := getAPIManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return nil, b.Delete(ctx)
}

func getAPIManagerBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	// Get image name.
	images := []kustomizetypes.Image{}

	// Get the images from the cluster spec. This overwrites the default image
	// set by the operator related images environment variables.
	if img := image.GetKustomizeImage(kImageAPIManager, cluster.Spec.Images.APIManagerContainer, os.Getenv(apiManagerImageEnvVar)); img != nil {
		images = append(images, *img)
	}

	// Create deployment transforms.
	deploymentTransforms := []transform.TransformFunc{}

	// Add secret volume transform.
	apiSecretVolTF := stransform.SetPodTemplateSecretVolumeFunc("api-secret", cluster.Spec.SecretRefName, nil)

	deploymentTransforms = append(deploymentTransforms, apiSecretVolTF)

	roleBindingTransforms := []transform.TransformFunc{}

	// Add namespace of cross-referenced role binding subject.
	// NOTE: In kustomize, when using role binding, if the subject service
	// account is not defined in the same kustomization file, kustomize doesn't
	// update the subject namespace of the service account. Set the cross
	// referenced service account namespace.
	// Refer: https://github.com/kubernetes-sigs/kustomize/issues/1377
	daemonsetSASubjectNamespaceTF := stransform.SetClusterRoleBindingSubjectNamespaceFunc("storageos-node", cluster.GetNamespace())

	roleBindingTransforms = append(roleBindingTransforms, daemonsetSASubjectNamespaceTF)

	return declarative.NewBuilder(apiManagerPackage, fs,
		declarative.WithManifestTransform(transform.ManifestTransform{
			"api-manager/deployment-storageos-api-manager.yaml":            deploymentTransforms,
			"api-manager/clusterrolebinding-storageos-key-management.yaml": roleBindingTransforms,
		}),
		declarative.WithKustomizeMutationFunc([]kustomize.MutateFunc{
			kustomize.AddNamespace(cluster.GetNamespace()),
			kustomize.AddImages(images),
		}),
		declarative.WithKubectlClient(kcl),
	)
}

func NewAPIManagerOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *APIManagerOperand {
	return &APIManagerOperand{
		name:            name,
		client:          client,
		requires:        requires,
		requeueStrategy: requeueStrategy,
		fs:              fs,
		kubectlClient:   kcl,
	}
}

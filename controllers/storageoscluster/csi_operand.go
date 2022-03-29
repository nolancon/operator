package storageoscluster

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ondat/operator-toolkit/declarative"
	"github.com/ondat/operator-toolkit/declarative/kubectl"
	"github.com/ondat/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/ondat/operator-toolkit/event/v1"
	"github.com/ondat/operator-toolkit/operator/v1/operand"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/api/v1"
	"github.com/storageos/operator/internal/image"
)

const (
	// csiPackage contains the resource manifests for csi operand.
	csiPackage = "csi"

	// Kustomize image name for container image.
	kImageCSIProvisioner = "csi-provisioner"
	kImageCSIAttacher    = "csi-attacher"
	kImageCSIResizer     = "csi-resizer"

	// Related image environment variable.
	csiProvisionerEnvVar = "RELATED_IMAGE_CSIV1_EXTERNAL_PROVISIONER"
	// TODO: Attacher env var has "V3" suffix for backwards compatibility.
	// Remove the suffix when doing a breaking change.
	csiAttacherEnvVar = "RELATED_IMAGE_CSIV1_EXTERNAL_ATTACHER_V3"
	csiResizerEnvVar  = "RELATED_IMAGE_CSIV1_EXTERNAL_RESIZER"
)

type CSIOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient
}

var _ operand.Operand = &CSIOperand{}

func (c *CSIOperand) Name() string                             { return c.name }
func (c *CSIOperand) Requires() []string                       { return c.requires }
func (c *CSIOperand) RequeueStrategy() operand.RequeueStrategy { return c.requeueStrategy }

func (c *CSIOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	ctx, span, _, log := instrumentation.Start(ctx, "CSIOperand.ReadyCheck")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Get the deployment object and check status of the replicas.
	csiDep := &appsv1.Deployment{}
	key := client.ObjectKey{Name: "storageos-csi-helper", Namespace: obj.GetNamespace()}
	if err := c.client.Get(ctx, key, csiDep); err != nil {
		return false, err
	}

	if csiDep.Status.AvailableReplicas > 0 {
		log.Info("Found available replicas more than 0", "availableReplicas", csiDep.Status.AvailableReplicas)
		return true, nil
	}

	log.Info("csi-helper not ready")
	return false, nil
}

func (c *CSIOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (c *CSIOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "CSIOperand.Ensure")
	defer span.End()

	b, err := getCSIBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Apply(ctx)
}

func (c *CSIOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "CSIOperand.Delete")
	defer span.End()

	b, err := getCSIBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Delete(ctx)
}

func getCSIBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	// Get image names.
	images := []kustomizetypes.Image{}

	// Get the images from the cluster spec. These overwrite the default images
	// set by the operator related images environment variables.
	if img := image.GetKustomizeImage(kImageCSIProvisioner, cluster.Spec.Images.CSIExternalProvisionerContainer, os.Getenv(csiProvisionerEnvVar)); img != nil {
		images = append(images, *img)
	}
	if img := image.GetKustomizeImage(kImageCSIAttacher, cluster.Spec.Images.CSIExternalAttacherContainer, os.Getenv(csiAttacherEnvVar)); img != nil {
		images = append(images, *img)
	}
	if img := image.GetKustomizeImage(kImageCSIResizer, cluster.Spec.Images.CSIExternalResizerContainer, os.Getenv(csiResizerEnvVar)); img != nil {
		images = append(images, *img)
	}

	return declarative.NewBuilder(csiPackage, fs,
		declarative.WithKustomizeMutationFunc([]kustomize.MutateFunc{
			kustomize.AddNamespace(cluster.GetNamespace()),
			kustomize.AddImages(images),
		}),
		declarative.WithKubectlClient(kcl),
	)
}

func NewCSIOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *CSIOperand {
	return &CSIOperand{
		name:            name,
		client:          client,
		requires:        requires,
		requeueStrategy: requeueStrategy,
		fs:              fs,
		kubectlClient:   kcl,
	}
}

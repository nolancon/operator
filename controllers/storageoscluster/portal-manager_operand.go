package storageoscluster

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/darkowlzz/operator-toolkit/declarative"
	"github.com/darkowlzz/operator-toolkit/declarative/kubectl"
	"github.com/darkowlzz/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/darkowlzz/operator-toolkit/event/v1"
	"github.com/darkowlzz/operator-toolkit/operator/v1/operand"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/apis/v1"
	"github.com/storageos/operator/internal/image"
)

const (
	// portalManagerPackage contains the resource manifests for portalManager operand.
	portalManagerPackage = "portal-manager"

	// Kustomize image name for container image.
	kImageKubePortalManager = "controller"

	// Related image environment variable.
	kubePortalManagerEnvVar = "RELATED_IMAGE_PORTAL_MANAGER"

	// Name of the Portal Manager deployment.
	deploymentName = "storageos-portal-manager"
)

type PortalManagerOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient

	currentStateLock chan bool
	currentState     bool
	getCurrentState  sync.Once
}

var _ operand.Operand = &PortalManagerOperand{}

func (c *PortalManagerOperand) Name() string                             { return c.name }
func (c *PortalManagerOperand) Requires() []string                       { return c.requires }
func (c *PortalManagerOperand) RequeueStrategy() operand.RequeueStrategy { return c.requeueStrategy }

func (c *PortalManagerOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	c.currentStateLock <- true
	currState := c.currentState
	<-c.currentStateLock

	// Skip check if not deployed.
	if !currState {
		return true, nil
	}

	ctx, span, _, log := instrumentation.Start(ctx, "PortalManagerOperand.ReadyCheck")
	defer span.End()

	// Get the deployment object and check status of the replicas.
	portalManagerDep := &appsv1.Deployment{}
	key := client.ObjectKey{Name: deploymentName, Namespace: obj.GetNamespace()}
	if err := c.client.Get(ctx, key, portalManagerDep); err != nil {
		return false, err
	}

	if portalManagerDep.Status.AvailableReplicas > 0 {
		log.V(4).Info("Found available replicas more than 0", "availableReplicas", portalManagerDep.Status.AvailableReplicas)
		return true, nil
	}

	log.V(4).Info("portal-manager not ready")
	return false, nil
}

func (c *PortalManagerOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (c *PortalManagerOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, log := instrumentation.Start(ctx, "PortalManagerOperand.Ensure")
	defer span.End()

	var err error
	c.getCurrentState.Do(func() {
		// Get current state of the deployment object.
		portalManagerDep := &appsv1.Deployment{}
		key := client.ObjectKey{Name: deploymentName, Namespace: obj.GetNamespace()}
		err = c.client.Get(ctx, key, portalManagerDep)
		c.currentState = err == nil

		log.V(4).Info("portal-manager state", "deployed", c.currentState)
	})
	if err != nil && !apierrors.IsNotFound(err) {
		span.RecordError(err)
		return nil, err
	}

	b, err := getPortalManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	if cluster.Spec.EnablePortalManager && !c.currentState {
		err := b.Apply(ctx)

		c.currentStateLock <- true
		c.currentState = err == nil
		<-c.currentStateLock

		return nil, err
	} else if !cluster.Spec.EnablePortalManager && c.currentState {
		c.currentStateLock <- true
		c.currentState = false
		<-c.currentStateLock

		return nil, b.Delete(ctx)
	}

	return nil, nil
}

func (c *PortalManagerOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, _ := instrumentation.Start(ctx, "PortalManagerOperand.Delete")
	defer span.End()

	b, err := getPortalManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return nil, b.Delete(ctx)
}

func getPortalManagerBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	// Get image name.
	images := []kustomizetypes.Image{}

	// Get the images from the cluster spec. These overwrite the default images
	// set by the operator related images environment variables.
	if img := image.GetKustomizeImage(kImageKubePortalManager, cluster.Spec.Images.PortalManagerContainer, os.Getenv(kubePortalManagerEnvVar)); img != nil {
		images = append(images, *img)
	}

	return declarative.NewBuilder(portalManagerPackage, fs,
		declarative.WithKustomizeMutationFunc([]kustomize.MutateFunc{
			kustomize.AddNamespace(cluster.GetNamespace()),
			kustomize.AddImages(images),
		}),
		declarative.WithKubectlClient(kcl),
	)
}

func NewPortalManagerOperand(
	name string,
	client client.Client,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *PortalManagerOperand {
	return &PortalManagerOperand{
		name:             name,
		client:           client,
		requires:         requires,
		requeueStrategy:  requeueStrategy,
		fs:               fs,
		kubectlClient:    kcl,
		currentStateLock: make(chan bool, 1),
	}
}

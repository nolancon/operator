package storageoscluster

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ondat/operator-toolkit/declarative"
	"github.com/ondat/operator-toolkit/declarative/kubectl"
	"github.com/ondat/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/ondat/operator-toolkit/event/v1"
	"github.com/ondat/operator-toolkit/operator/v1/operand"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/api/v1"
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
	pmDeploymentName = "storageos-portal-manager"
)

type PortalManagerOperand struct {
	name            string
	client          client.Client
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient

	currentStateLock  chan bool
	currentState      bool
	fetchCurrentState sync.Once
}

var _ operand.Operand = &PortalManagerOperand{}

func (c *PortalManagerOperand) Name() string                             { return c.name }
func (c *PortalManagerOperand) Requires() []string                       { return c.requires }
func (c *PortalManagerOperand) RequeueStrategy() operand.RequeueStrategy { return c.requeueStrategy }

func (c *PortalManagerOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	// Skip check if not deployed.
	if !c.getCurrentState() {
		return true, nil
	}

	ctx, span, _, log := instrumentation.Start(ctx, "PortalManagerOperand.ReadyCheck")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Get the deployment object and check status of the replicas.
	portalManagerDep := &appsv1.Deployment{}
	key := client.ObjectKey{Name: pmDeploymentName, Namespace: obj.GetNamespace()}
	if err := c.client.Get(ctx, key, portalManagerDep); err != nil {
		return false, err
	}

	if portalManagerDep.Status.AvailableReplicas > 0 {
		log.Info("Found available replicas more than 0", "availableReplicas", portalManagerDep.Status.AvailableReplicas)
		return true, nil
	}

	log.Info("portal-manager not ready")
	return false, nil
}

func (c *PortalManagerOperand) PostReady(ctx context.Context, obj client.Object) error { return nil }

func (c *PortalManagerOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, log := instrumentation.Start(ctx, "PortalManagerOperand.Ensure")
	defer span.End()

	var err error
	c.fetchCurrentState.Do(func() {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		// Get current state of the deployment object.
		portalManagerDep := &appsv1.Deployment{}
		key := client.ObjectKey{Name: pmDeploymentName, Namespace: obj.GetNamespace()}
		err = c.client.Get(ctx, key, portalManagerDep)
		c.setCurrentState(err == nil)

		log.Info("portal-manager state", "deployed", c.currentState)
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

		c.setCurrentState(err == nil)

		return nil, err
	} else if !cluster.Spec.EnablePortalManager && c.currentState {
		c.setCurrentState(false)

		return nil, b.Delete(ctx)
	}

	return nil, nil
}

func (c *PortalManagerOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	// Skip if not deployed.
	if !c.getCurrentState() {
		return nil, nil
	}

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

func (c *PortalManagerOperand) getCurrentState() bool {
	c.currentStateLock <- true
	defer func() {
		<-c.currentStateLock
	}()

	return c.currentState
}

func (c *PortalManagerOperand) setCurrentState(currentState bool) {
	c.currentStateLock <- true
	defer func() {
		<-c.currentStateLock
	}()

	c.currentState = currentState
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

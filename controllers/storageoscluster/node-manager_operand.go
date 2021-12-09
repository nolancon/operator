package storageoscluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/darkowlzz/operator-toolkit/declarative"
	"github.com/darkowlzz/operator-toolkit/declarative/kubectl"
	"github.com/darkowlzz/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/darkowlzz/operator-toolkit/event/v1"
	"github.com/darkowlzz/operator-toolkit/operator/v1/operand"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	tappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/apis/v1"
	"github.com/storageos/operator/internal/image"
	"github.com/storageos/operator/watchers"
)

const (
	// nodeManagerPackage contains the resource manifests for nodeManager operand.
	nodeManagerPackage = "node-manager"

	// Kustomize image name for container image.
	kImageKubeNodeManager = "node-manager"

	// Related image environment variable.
	kubeNodeManagerEnvVar = "RELATED_IMAGE_NODE_MANAGER"

	// Name of the Node Manager deployment.
	nmDeploymentName = "storageos-node-manager"

	// Name of StorageOS Node
	snDaemonSetName = "storageos-node"
)

type NodeManagerOperand struct {
	name            string
	client          client.Client
	appsGetter      tappv1.AppsV1Interface
	requires        []string
	requeueStrategy operand.RequeueStrategy
	fs              filesys.FileSystem
	kubectlClient   kubectl.KubectlClient

	watchLock      chan bool
	watcher        *watchers.DaemonSetWatcher
	watchCloseChan chan bool

	currentStateLock  chan bool
	currentState      bool
	fetchCurrentState sync.Once
}

var _ operand.Operand = &NodeManagerOperand{}

func (c *NodeManagerOperand) Name() string                             { return c.name }
func (c *NodeManagerOperand) Requires() []string                       { return c.requires }
func (c *NodeManagerOperand) RequeueStrategy() operand.RequeueStrategy { return c.requeueStrategy }

func (c *NodeManagerOperand) ReadyCheck(ctx context.Context, obj client.Object) (bool, error) {
	// Skip check if not deployed.
	if !c.getCurrentState() {
		return true, nil
	}

	ctx, span, _, log := instrumentation.Start(ctx, "NodeManagerOperand.ReadyCheck")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Get the deployment object and check status of the replicas.
	nodeManagerDep := &appsv1.Deployment{}
	key := client.ObjectKey{Name: nmDeploymentName, Namespace: obj.GetNamespace()}
	if err := c.client.Get(ctx, key, nodeManagerDep); err != nil {
		return false, err
	}

	if nodeManagerDep.Status.AvailableReplicas == *nodeManagerDep.Spec.Replicas {
		log.V(4).Info("Found available replicas more than 0", "availableReplicas", nodeManagerDep.Status.AvailableReplicas)
		return true, nil
	}

	log.V(4).Info("node-manager not ready")
	return false, nil
}

func (c *NodeManagerOperand) PostReady(ctx context.Context, obj client.Object) error {
	// Skip if not deployed.
	if !c.getCurrentState() {
		return nil
	}

	ctx, span, _, log := instrumentation.Start(ctx, "NodeManagerOperand.PostReady")
	defer span.End()

	c.watchLock <- true
	defer func() {
		<-c.watchLock
	}()

	if c.watcher != nil {
		log.Info("watcher is already running")
		return nil
	}

	c.watcher = watchers.NewDaemonSetWatcher(c.appsGetter.DaemonSets(obj.GetNamespace()), "storageos node")
	if err := c.watcher.Setup(ctx, true, "app.kubernetes.io/component=control-plane"); err != nil {
		log.Error(err, "unable to set daemonset watcher")
		return err
	}

	if err := c.updateReplicas(ctx, obj, log); err != nil {
		return err
	}

	c.watcher.Start(ctx, func(watchChan <-chan watch.Event) error {
		log.Info("watcher has started")
		for {
			select {
			case event, ok := <-watchChan:
				if !ok {
					log.Info("watcher has closed")
					return errors.New("watcher has closed")
				} else if event.Type != watch.Modified {
					continue
				}

				log.Info("daemonset has changed")

				err := func() (err error) {
					ctx, cancel := context.WithTimeout(ctx, time.Minute)
					defer cancel()

					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-ticker.C:
							if err = c.updateReplicas(ctx, obj, log); err != nil {
								log.Error(err, "unable to update replicas")
								continue
							}

							log.Info("successfully updated replicas")
							cancel()
						case <-ctx.Done():
							return
						}
					}
				}()
				if err != nil {
					return err
				}
			case <-c.watchCloseChan:
				log.Info("resource has deleted")

				c.watchLock <- true
				defer func() {
					<-c.watchLock
				}()
				c.watcher = nil

				return nil
			}
		}
	})

	return nil
}

func (c *NodeManagerOperand) Ensure(ctx context.Context, obj client.Object, ownerRef metav1.OwnerReference) (eventv1.ReconcilerEvent, error) {
	ctx, span, _, log := instrumentation.Start(ctx, "NodeManagerOperand.Ensure")
	defer span.End()

	var err error
	c.fetchCurrentState.Do(func() {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		// Get current state of the deployment object.
		nodeManagerDep := &appsv1.Deployment{}
		key := client.ObjectKey{Name: nmDeploymentName, Namespace: obj.GetNamespace()}
		err = c.client.Get(ctx, key, nodeManagerDep)

		c.setCurrentState(err == nil)

		log.V(4).Info("node-manager state", "deployed", c.currentState)
	})
	if err != nil && !apierrors.IsNotFound(err) {
		span.RecordError(err)
		return nil, err
	}

	b, err := getNodeManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	currState := c.getCurrentState()
	if len(cluster.Spec.NodeManagerFeatures) > 0 && !currState {
		err := b.Apply(ctx)

		if err := c.updateReplicas(ctx, obj, log); err != nil {
			return nil, err
		}

		c.setCurrentState(err == nil)

		return nil, err
	} else if len(cluster.Spec.NodeManagerFeatures) == 0 && currState {
		c.setCurrentState(false)

		if err = b.Delete(ctx); err == nil {
			c.watchCloseChan <- true
		}

		return nil, err
	}

	return nil, nil
}

func (c *NodeManagerOperand) Delete(ctx context.Context, obj client.Object) (eventv1.ReconcilerEvent, error) {
	// Skip if not deployed.
	if !c.getCurrentState() {
		return nil, nil
	}

	ctx, span, _, _ := instrumentation.Start(ctx, "NodeManagerOperand.Delete")
	defer span.End()

	b, err := getNodeManagerBuilder(c.fs, obj, c.kubectlClient)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if err = b.Delete(ctx); err == nil {
		c.watchCloseChan <- true
	}

	return nil, err
}

func (c *NodeManagerOperand) updateReplicas(ctx context.Context, obj client.Object, log logr.Logger) error {
	ds := &appsv1.DaemonSet{}
	objKey := types.NamespacedName{Namespace: obj.GetNamespace(), Name: snDaemonSetName}
	if err := c.client.Get(ctx, objKey, ds); err != nil {
		return err
	}

	log.Info("set replicas to", "replicas", ds.Status.DesiredNumberScheduled)
	payload := []patchUInt32Value{{
		Op:    "replace",
		Path:  "/spec/replicas",
		Value: uint32(ds.Status.DesiredNumberScheduled),
	}}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = c.appsGetter.Deployments(obj.GetNamespace()).
		Patch(ctx, nmDeploymentName, types.JSONPatchType, payloadBytes, metav1.PatchOptions{})

	return err
}

func getNodeManagerBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient) (*declarative.Builder, error) {
	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	// Get image name.
	images := []kustomizetypes.Image{}

	// Get the images from the cluster spec. These overwrite the default images
	// set by the operator related images environment variables.
	if img := image.GetKustomizeImage(kImageKubeNodeManager, cluster.Spec.Images.NodeManagerContainer, os.Getenv(kubeNodeManagerEnvVar)); img != nil {
		images = append(images, *img)
	}

	return declarative.NewBuilder(nodeManagerPackage, fs,
		declarative.WithKustomizeMutationFunc([]kustomize.MutateFunc{
			kustomize.AddNamespace(cluster.GetNamespace()),
			kustomize.AddImages(images),
		}),
		declarative.WithKubectlClient(kcl),
	)
}

func (c *NodeManagerOperand) getCurrentState() bool {
	c.currentStateLock <- true
	defer func() {
		<-c.currentStateLock
	}()

	return c.currentState
}

func (c *NodeManagerOperand) setCurrentState(currentState bool) {
	c.currentStateLock <- true
	defer func() {
		<-c.currentStateLock
	}()

	c.currentState = currentState
}

func NewNodeManagerOperand(
	name string,
	client client.Client,
	appsGetter tappv1.AppsV1Interface,
	requires []string,
	requeueStrategy operand.RequeueStrategy,
	fs filesys.FileSystem,
	kcl kubectl.KubectlClient,
) *NodeManagerOperand {
	return &NodeManagerOperand{
		name:             name,
		client:           client,
		appsGetter:       appsGetter,
		requires:         requires,
		requeueStrategy:  requeueStrategy,
		fs:               fs,
		kubectlClient:    kcl,
		watchLock:        make(chan bool, 1),
		watchCloseChan:   make(chan bool),
		currentStateLock: make(chan bool, 1),
	}
}

//  patchStringValue specifies a patch operation for a uint32.
type patchUInt32Value struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value uint32 `json:"value"`
}

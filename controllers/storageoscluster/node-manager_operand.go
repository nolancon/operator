package storageoscluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/ondat/operator-toolkit/declarative"
	"github.com/ondat/operator-toolkit/declarative/kubectl"
	"github.com/ondat/operator-toolkit/declarative/kustomize"
	eventv1 "github.com/ondat/operator-toolkit/event/v1"
	"github.com/ondat/operator-toolkit/operator/v1/operand"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	tappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/resid"
	kustomizetypes "sigs.k8s.io/kustomize/api/types"

	storageoscomv1 "github.com/storageos/operator/api/v1"
	"github.com/storageos/operator/internal/image"
	"github.com/storageos/operator/watchers"
)

const (
	// nodeManagerPackage contains the resource manifests for nodeManager operand.
	nodeManagerPackage = "node-manager"

	// Kustomize image name for container image.
	kImageKubeNodeManager = "controller"

	// Related images environment variables.
	kubeNodeManagerEnvVar  = "RELATED_IMAGE_NODE_MANAGER"
	kubeUpgradeGuardEnvVar = "RELATED_IMAGE_UPGRADE_GUARD"

	// Name of the Node Manager deployment.
	nmDeploymentName = "storageos-node-manager"

	// Name of StorageOS Node
	snDaemonSetName = "storageos-node"

	// Node manager features.
	upgradeGuardFeatureKey = "upgradeGuard"

	upgradeGuardReadinessProbeInterval = time.Second
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
		log.Info("node-manager fetch error")
		return false, err
	}

	if nodeManagerDep.Status.AvailableReplicas > 0 {
		log.Info("Found available replicas more than 0", "availableReplicas", nodeManagerDep.Status.Replicas)
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

	if err := c.updateNodeManagerReplicas(ctx, obj, log); err != nil {
		return err
	}

	closeWatchWaiter := make(chan bool)

	watchErrChan := c.watcher.Start(ctx, func(watchChan <-chan watch.Event) error {
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
							if err = c.updateNodeManagerReplicas(ctx, obj, log); err != nil {
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
				c.watcher = nil
				<-c.watchLock

				close(closeWatchWaiter)

				return nil
			}
		}
	})

	go func() {
		select {
		case <-closeWatchWaiter:
			return
		case err := <-watchErrChan:
			log.Error(err, "node watcher encountered error")
			// I didn't find any nice way to shut down operator from here.
			// Anyway a non working watcher is a panic situation :)
			panic(err)
		}
	}()

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
		key := client.ObjectKey{Name: nmDeploymentName, Namespace: obj.GetNamespace()}
		err = c.client.Get(ctx, key, &appsv1.Deployment{})

		c.setCurrentState(err == nil)

		log.Info("node-manager state", "deployed", c.currentState)
	})
	if err != nil && !apierrors.IsNotFound(err) {
		span.RecordError(err)
		return nil, err
	}

	b, err := c.getNodeManagerBuilder(c.fs, obj, c.kubectlClient, log)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	cluster, ok := obj.(*storageoscomv1.StorageOSCluster)
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to StorageOSCluster", obj)
	}

	currState := c.getCurrentState()
	if len(cluster.Spec.NodeManagerFeatures) > 0 {
		err := b.Apply(ctx)

		if !currState {
			c.setCurrentState(err == nil)
		}

		return nil, err
	} else if currState {
		if err = b.Delete(ctx); err == nil {
			c.setCurrentState(false)

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

	ctx, span, _, log := instrumentation.Start(ctx, "NodeManagerOperand.Delete")
	defer span.End()

	b, err := c.getNodeManagerBuilder(c.fs, obj, c.kubectlClient, log)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if err = b.Delete(ctx); err == nil {
		c.watchCloseChan <- true
	}

	return nil, err
}

func (c *NodeManagerOperand) updateNodeManagerReplicas(ctx context.Context, obj client.Object, log logr.Logger) error {
	replicas, err := c.getNodeReplicas(ctx, obj, log)
	if err != nil {
		return err
	}

	return c.setNodeManagerReplicas(ctx, obj, log, replicas)
}

func (c *NodeManagerOperand) getNodeReplicas(ctx context.Context, obj client.Object, log logr.Logger) (uint32, error) {
	ds := &appsv1.DaemonSet{}
	objKey := types.NamespacedName{Namespace: obj.GetNamespace(), Name: snDaemonSetName}
	if err := c.client.Get(ctx, objKey, ds); err != nil {
		return 0, err
	}

	return uint32(ds.Status.DesiredNumberScheduled), nil
}

func (c *NodeManagerOperand) setNodeManagerReplicas(ctx context.Context, obj client.Object, log logr.Logger, replicas uint32) error {
	log.Info("set replicas to", "replicas", replicas)
	payload := []patchUInt32Value{{
		Op:    "replace",
		Path:  "/spec/replicas",
		Value: replicas,
	}}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = c.appsGetter.Deployments(obj.GetNamespace()).
		Patch(ctx, nmDeploymentName, types.JSONPatchType, payloadBytes, metav1.PatchOptions{})

	return err
}

func (c *NodeManagerOperand) getNodeManagerBuilder(fs filesys.FileSystem, obj client.Object, kcl kubectl.KubectlClient, log logr.Logger) (*declarative.Builder, error) {
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

	replicas, err := c.getNodeReplicas(context.Background(), obj, log)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	mutators := []kustomize.MutateFunc{
		kustomize.AddNamespace(cluster.GetNamespace()),
		kustomize.AddImages(images),
		func(k *kustomizetypes.Kustomization) {
			k.Replicas = []kustomizetypes.Replica{
				{
					Name:  nmDeploymentName,
					Count: int64(replicas),
				},
			}
		},
	}

	nextIndex := 2 // Needs to update if number of containers changes in deployment.

	// Append upgrade guard as sidecar.
	if config, ok := cluster.Spec.NodeManagerFeatures[upgradeGuardFeatureKey]; ok {
		mutator := getSideCarContainerMutator(nextIndex, cluster.Spec.Images.UpgradeGuardContainer, os.Getenv(kubeUpgradeGuardEnvVar), "upgrade-guard", config, upgradeGuardReadinessProbeInterval)
		if mutator != nil {
			// nextIndex += 1
			mutators = append(mutators, mutator)
		}
	}

	return declarative.NewBuilder(nodeManagerPackage, fs,
		declarative.WithKustomizeMutationFunc(mutators),
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

//  patchUInt32Value specifies a patch operation for a uint32.
type patchUInt32Value struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value uint32 `json:"value"`
}

func getSideCarContainerMutator(nextIndex int, image, defaultImage string, containerName string, config string, readinessProbeInterval time.Duration) func(*kustomizetypes.Kustomization) {
	if image == "" {
		image = defaultImage
	}
	if image == "" {
		return nil
	}

	return func(k *kustomizetypes.Kustomization) {
		k.Patches = append(k.Patches, generateSideCarContainerPatch(nextIndex, containerName, image, config, readinessProbeInterval))
	}
}

// generateSideCarContainerPatch generates a sidecar container patch.
func generateSideCarContainerPatch(nextIndex int, name string, image string, config string, readinessProbeInterval time.Duration) kustomizetypes.Patch {
	// Convert sidecar context to environment variables.
	// We don't have any other usecase at the moment.
	envs := ""
	if config != "" {
		for _, item := range strings.Split(config, ",") {
			keyValue := strings.Split(item, "=")
			value := ""
			if len(keyValue) > 1 {
				value = keyValue[1]
			}
			envs += fmt.Sprintf(`{
				"name": "%s",
				"value": "%s"
			},`, strings.TrimSpace(keyValue[0]), strings.TrimSpace(value))
		}
	}

	return kustomizetypes.Patch{
		Patch: fmt.Sprintf(`[{
			"op": "add",
			"path": "/spec/template/spec/containers/%d",
			"value": {
				"name": "%s",
				"image": "%s",
				"env": [
					%s
					{
						"name": "NODE_NAME",
						"valueFrom": {
							"fieldRef": {
								"apiVersion": "v1",
								"fieldPath": "spec.nodeName"
							}
						}
					}
				],
				"livenessProbe": {
					"httpGet": {
					  "path": "/healthz",
					  "port": 8081
					},
					"initialDelaySeconds": 15,
					"periodSeconds": 20
				  },
				  "readinessProbe": {
					"httpGet": {
					  "path": "/readyz",
					  "port": 8081
					},
					"initialDelaySeconds": 5,
					"periodSeconds": %d
				  }
			}
		}]`, nextIndex, name, image, envs, int(readinessProbeInterval.Seconds())),
		Target: &kustomizetypes.Selector{
			Gvk:  resid.FromKind("Deployment"),
			Name: nmDeploymentName,
		},
	}
}

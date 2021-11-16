package controllers

import (
	"context"
	"fmt"

	compositev1 "github.com/darkowlzz/operator-toolkit/controller/composite/v1"
	"github.com/darkowlzz/operator-toolkit/declarative/loader"
	"github.com/darkowlzz/operator-toolkit/operator/v1/executor"
	tkpredicate "github.com/darkowlzz/operator-toolkit/predicate"
	"github.com/darkowlzz/operator-toolkit/telemetry"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	storageoscomv1 "github.com/storageos/operator/apis/v1"
	"github.com/storageos/operator/controllers/storageoscluster"
)

const instrumentationName = "github.com/storageos/operator/controllers"

var instrumentation *telemetry.Instrumentation

func init() {
	// Setup package instrumentation.
	instrumentation = telemetry.NewInstrumentationWithProviders(
		instrumentationName, nil, nil,
		ctrl.Log.WithName("controllers").WithName("StorageOSCluster"),
	)
}

// StorageOSClusterReconciler reconciles a StorageOSCluster object
type StorageOSClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	compositev1.CompositeReconciler
}

func NewStorageOSClusterReconciler(mgr ctrl.Manager) *StorageOSClusterReconciler {
	return &StorageOSClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
}

// +kubebuilder:rbac:groups=storageos.com,resources=storageosclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storageos.com,resources=storageosclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=storageos.com,resources=storageosclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets;daemonsets;deployments;replicasets,verbs=*
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events;namespaces;serviceaccounts;secrets;services;services/status;services/finalizers;persistentvolumeclaims;persistentvolumeclaims/status;persistentvolumes;configmaps;configmaps/status;replicationcontrollers;pods/binding;pods/status;endpoints;endpoints/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;create;patch;delete;bind
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses;volumeattachments;volumeattachments/status;csinodeinfos;csinodes;csistoragecapacities;csidrivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;create;patch;delete
// +kubebuilder:rbac:groups=csi.storage.k8s.io,resources=csidrivers;csistoragecapacities,verbs=create;delete;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=privileged,verbs=get;create;update;delete;use
// +kubebuilder:rbac:groups=api.storageos.com,resources=volumes;nodes,verbs=create;patch;get;watch;delete;list
// +kubebuilder:rbac:groups=api.storageos.com,resources=volumes/status;nodes/status,verbs=get;update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *StorageOSClusterReconciler) SetupWithManager(mgr ctrl.Manager, kubeVersion, channel string) error {
	_, span, _, log := instrumentation.Start(context.Background(), "StorageosCluster.SetupWithManager")
	defer span.End()

	// Load manifests in an in-memory filesystem.
	fs, err := loader.NewLoadedManifestFileSystem(loader.DefaultChannelDir, channel)
	if err != nil {
		return fmt.Errorf("failed to create loaded ManifestFileSystem: %w", err)
	}

	// TODO: Expose the executor strategy option via SetupWithManager.
	cc, err := storageoscluster.NewStorageOSClusterController(mgr, kubeVersion, fs, executor.Parallel)
	if err != nil {
		return err
	}

	// Initialize the reconciler.
	err = r.CompositeReconciler.Init(mgr, cc, &storageoscomv1.StorageOSCluster{},
		compositev1.WithName("storageoscluster-controller"),
		compositev1.WithCleanupStrategy(compositev1.FinalizerCleanup),
		compositev1.WithInitCondition(compositev1.DefaultInitCondition),
		compositev1.WithInstrumentation(nil, nil, log),
	)
	if err != nil {
		return fmt.Errorf("failed to create new CompositeReconciler: %w", err)
	}

	// Use the GenerationChangedPredicate to ignore the status update events
	// but capture the events due to labels, annotations and finalizers change.
	return ctrl.NewControllerManagedBy(mgr).
		For(&storageoscomv1.StorageOSCluster{}).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
			tkpredicate.FinalizerChangedPredicate{},
		)).
		Complete(r)
}

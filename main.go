package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"

	"github.com/ondat/operator-toolkit/declarative/loader"
	"github.com/ondat/operator-toolkit/telemetry/export"
	"github.com/ondat/operator-toolkit/webhook/cert"
	"go.uber.org/zap/zapcore"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configstorageoscomv1 "github.com/storageos/operator/api/config.storageos.com/v1"
	storageoscomv1 "github.com/storageos/operator/api/v1"
	"github.com/storageos/operator/controllers"
	whctrlr "github.com/storageos/operator/controllers/webhook"
	"github.com/storageos/operator/internal/version"
	"github.com/storageos/operator/watchers"
	// +kubebuilder:scaffold:imports
)

// podNamespace is the operator's pod namespace environment variable.
const podNamespace = "POD_NAMESPACE"

var (
	renewDeadline       = 5 * time.Second
	leaseDuration       = 7 * time.Second
	leaderRetryDuration = time.Second
	scheme              = runtime.NewScheme()
	setupLog            = ctrl.Log.WithName("setup")
)

var SupportedMinKubeVersion string

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(storageoscomv1.AddToScheme(scheme))
	utilruntime.Must(configstorageoscomv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=*
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;list;watch;create;update;patch;delete

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")

	var opts zap.Options
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Configure logger.
	f := func(ec *zapcore.EncoderConfig) {
		ec.TimeKey = "timestamp"
		ec.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	}
	encoderOpts := func(o *zap.Options) {
		o.EncoderConfigOptions = append(o.EncoderConfigOptions, f)
	}

	zapLogger := zap.New(zap.UseFlagOptions(&opts), zap.StacktraceLevel(zapcore.PanicLevel), encoderOpts)
	ctrl.SetLogger(zapLogger)
	klog.SetLogger(zapLogger)

	// Setup telemetry.
	telemetryShutdown, err := export.InstallJaegerExporter("storageos-operator")
	if err != nil {
		setupLog.Error(err, "unable to setup telemetry exporter")
		os.Exit(1)
	}
	defer telemetryShutdown()

	currentNS := os.Getenv(podNamespace)
	if len(currentNS) == 0 {
		setupLog.Error(errors.New("current namespace not found"), "failed to get current namespace")
		os.Exit(1)
	}

	// Load controller manager configuration and create manager options from
	// it.
	ctrlConfig := configstorageoscomv1.OperatorConfig{}
	options := ctrl.Options{
		Scheme:                  scheme,
		LeaderElectionID:        "storageos-operator-leader",
		LeaderElectionNamespace: currentNS,
		RenewDeadline:           &renewDeadline,
		LeaseDuration:           &leaseDuration,
		RetryPeriod:             &leaderRetryDuration,
	}
	if configFile != "" {
		var err error
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&ctrlConfig))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}

	defaultRestConfig := ctrl.GetConfigOrDie()
	defaultKubeClient := kubernetes.NewForConfigOrDie(defaultRestConfig)

	restConfig := ctrl.GetConfigOrDie()
	restConfig.Timeout = time.Minute
	timeoutKubeClient := kubernetes.NewForConfigOrDie(restConfig)

	// Get Kubernetes version.
	kubeVersion, err := timeoutKubeClient.Discovery().ServerVersion()
	if err != nil {
		setupLog.Error(err, "unable to get Kubernetes version")
		os.Exit(1)
	}

	// Validate Kubernetes version
	if SupportedMinKubeVersion != "" && !version.IsSupported(kubeVersion.String(), SupportedMinKubeVersion) {
		setupLog.Error(errors.New("unsupported Kubernetes version"), "current version of Kubernetes is lower than required minimum version", "supported", SupportedMinKubeVersion, "current", kubeVersion.String())
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(defaultRestConfig, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create an uncached client to be used in the certificate manager
	// and ConfigMap watcher.
	// NOTE: Cached client from manager can't be used here because the cache is
	// uninitialized at this point.
	cli, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		setupLog.Error(err, "failed to create raw client")
		os.Exit(1)
	}

	// Configure the certificate manager.
	certOpts := cert.Options{
		CertRefreshInterval: ctrlConfig.WebhookCertRefreshInterval.Duration,
		Service: &admissionregistrationv1.ServiceReference{
			Name:      ctrlConfig.WebhookServiceName,
			Namespace: currentNS,
		},
		Client:                      cli,
		SecretRef:                   &types.NamespacedName{Name: ctrlConfig.WebhookSecretRef, Namespace: currentNS},
		ValidatingWebhookConfigRefs: []types.NamespacedName{{Name: ctrlConfig.ValidatingWebhookConfigRef}},
	}
	// Create certificate manager without manager to start the provisioning
	// immediately.
	// NOTE: Certificate Manager implements nonLeaderElectionRunnable interface
	// but since the webhook server is also a nonLeaderElectionRunnable, they
	// start at the same time, resulting in a race condition where sometimes
	// the certificates aren't available when the webhook server starts. By
	// passing nil instead of the manager, the certificate manager is not
	// managed by the controller manager. It starts immediately, in a blocking
	// fashion, ensuring that the cert is created before the webhook server
	// starts.
	if err := cert.NewManager(nil, certOpts); err != nil {
		setupLog.Error(err, "unable to provision certificate")
		os.Exit(1)
	}
	// Set channel based on K8s version. If that channel does not exist, use default channelDir 'stable'.
	cleanKubeVersion := version.CleanupVersion(kubeVersion.String())
	channel := version.MajorMinor(cleanKubeVersion)
	_, err = os.Stat(filepath.Join(loader.DefaultChannelDir, channel))
	if err != nil {
		channel = loader.DefaultChannelName
	}

	if err = controllers.NewStorageOSClusterReconciler(mgr).
		SetupWithManager(mgr, defaultKubeClient.AppsV1(), cleanKubeVersion, channel); err != nil {
		setupLog.Error(err, "unable to create controller",
			"controller", "StorageOSCluster")
		os.Exit(1)
	}

	// Create and set up admission webhook controller.
	clusterWh, err := whctrlr.NewStorageOSClusterWebhook(mgr.GetClient(), mgr.GetScheme())
	if err != nil {
		setupLog.Error(err, "unable to create admission webhook controller",
			"controller", clusterWh.CtrlName)
		os.Exit(1)
	}
	if err := clusterWh.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup webhook controller with manager",
			"controller", clusterWh.CtrlName)
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())

	// Watch storageos node config map and restart on change.
	setupLog.Info("starting storageos node configmap watcher")
	nodeConfigWatcher := watchers.NewConfigMapWatcher(defaultKubeClient.CoreV1().ConfigMaps(currentNS), "config change")
	if err := nodeConfigWatcher.Setup(ctx, true, "app.kubernetes.io/component=control-plane"); err != nil {
		setupLog.Error(err, "unable to set storageos node configmap watcher")
		os.Exit(1)
	}
	dsClient := defaultKubeClient.AppsV1().DaemonSets(currentNS)

	nodeConfigWatchErrChan := nodeConfigWatcher.Start(ctx, func(nodeConfigWatchChan <-chan watch.Event) error {
		for {
			event, ok := <-nodeConfigWatchChan
			if !ok {
				setupLog.Info("storageos node configmap watcher has closed")
				return errors.New("storageos node configmap watcher has closed")
			}

			if event.Type == watch.Modified {
				setupLog.Info("storageos node config map has updated, delete node daemonset to enforce new environment variables")
				err := dsClient.Delete(ctx, "storageos-node", metav1.DeleteOptions{})
				if err != nil {
					return errors.New("failed to delete storageos node daemonset")
				}
			}
		}
	})
	go func() {
		err := <-nodeConfigWatchErrChan
		setupLog.Error(err, "storageos node config map watcher encountered error")
	}()

	// Watch storageos operator config map and restart on change.
	setupLog.Info("starting storageos operator config map watcher")
	watcher := watchers.NewConfigMapWatcher(defaultKubeClient.CoreV1().ConfigMaps(currentNS), "config change")
	if err := watcher.Setup(ctx, true, "app.kubernetes.io/component=operator"); err != nil {
		setupLog.Error(err, "unable to set storageos operator config map watcher")
		os.Exit(1)
	}
	watchErrChan := watcher.Start(ctx, func(watchChan <-chan watch.Event) error {
		for {
			event, ok := <-watchChan
			if !ok {
				setupLog.Info("storageos operator configmap watcher has closed")
				return errors.New("storageos operator configmap watcher has closed")
			}

			if event.Type == watch.Modified {
				// If this happens we want to restart the operator, so cancel the root context
				setupLog.Info("storageos operator config map has updated")
				cancel()
				break
			}
		}

		return nil
	})
	go func() {
		err := <-watchErrChan
		setupLog.Error(err, "storageos operator config map watcher encountered error")
		cancel()
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

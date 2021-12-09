package watchers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	tcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ConfigMapWatcher watches StorageOS related config maps.
type ConfigMapWatcher struct {
	name    string
	client  tcorev1.ConfigMapInterface
	listOpt metav1.ListOptions
	log     logr.Logger
}

// setup configures config map watcher with init values.
func (w *ConfigMapWatcher) Setup(ctx context.Context, fromNow bool, labelSelector string) error {
	w.listOpt = metav1.ListOptions{}

	if labelSelector != "" {
		w.listOpt.LabelSelector = labelSelector
	}

	var configMaps *corev1.ConfigMapList
	if fromNow {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		var err error
		if configMaps, err = w.client.List(ctx, w.listOpt); err != nil {
			return err
		}
	}

	w.listOpt.Watch = true
	if configMaps != nil {
		w.listOpt.ResourceVersion = configMaps.ResourceVersion
	}

	return nil
}

// Start wathing config maps.
func (w *ConfigMapWatcher) Start(ctx context.Context, watchConsumer WatchConsumer) {
	start(ctx, w.name, watchChange(ctx, w.client, w.listOpt, watchConsumer))
}

// NewConfigMapWatcher consructs a new config map watcher.
func NewConfigMapWatcher(client tcorev1.ConfigMapInterface, name string) *ConfigMapWatcher {
	return &ConfigMapWatcher{
		name:   name,
		client: client,
		log:    ctrl.Log.WithName(name + " config map watcher"),
	}
}

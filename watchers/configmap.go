package watchers

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	tcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var watcherLog = ctrl.Log.WithName("config map watcher")

// ConfigMapWatcher watches StorageOS related config maps.
type ConfigMapWatcher struct {
	client  tcorev1.ConfigMapInterface
	listOpt metav1.ListOptions
}

// setup configures config map watcher with init values.
func (w *ConfigMapWatcher) Setup(ctx context.Context) error {
	w.listOpt = metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=operator",
	}

	configMaps, err := w.client.List(context.Background(), w.listOpt)
	if err != nil {
		return err
	}

	w.listOpt.Watch = true
	w.listOpt.ResourceVersion = configMaps.ResourceVersion

	return nil
}

// Start wathing config maps until first change.
func (w *ConfigMapWatcher) Start(ctx context.Context) {
	for waitPeriod := 0; ; waitPeriod = (waitPeriod + 1) * 2 {
		if waitPeriod != 0 {
			watcherLog.Info("wait before start watching", "period", waitPeriod)
			time.Sleep(time.Second * time.Duration(waitPeriod))
		}

		err := w.watchChange(ctx)
		if err != nil {
			watcherLog.Error(err, "unable to start config map watcher")
			// Increase wait period of reconnect.
			continue
		}

		// Something has changed.
		break
	}
}

// watchChange consumes watch events and returns on change.
func (w *ConfigMapWatcher) watchChange(ctx context.Context) error {
	watcher, err := w.client.Watch(ctx, w.listOpt)
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		event, ok := <-watcher.ResultChan()
		if !ok {
			watcherLog.Info("watcher has closed")
			break
		}

		if event.Type == watch.Modified {
			watcherLog.Info("config map has updated")
			break
		}
	}

	return nil
}

// NewConfigMapWatcher consructs a new config map watcher.
func NewConfigMapWatcher(client tcorev1.ConfigMapInterface) *ConfigMapWatcher {
	return &ConfigMapWatcher{
		client: client,
	}
}

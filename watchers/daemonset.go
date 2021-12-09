package watchers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	tappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var dsWatcherLog = ctrl.Log.WithName("daemonset watcher")

// DaemonSetWatcher watches StorageOS related daemonsets.
type DaemonSetWatcher struct {
	name    string
	client  tappv1.DaemonSetInterface
	listOpt metav1.ListOptions
	log     logr.Logger
}

// setup configures daemonset watcher with init values.
func (w *DaemonSetWatcher) Setup(ctx context.Context, fromNow bool, labelSelector string) error {
	w.listOpt = metav1.ListOptions{}

	if labelSelector != "" {
		w.listOpt.LabelSelector = labelSelector
	}

	var daemonSets *appv1.DaemonSetList
	if fromNow {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		var err error
		if daemonSets, err = w.client.List(ctx, w.listOpt); err != nil {
			return err
		}
	}

	w.listOpt.Watch = true
	if daemonSets != nil {
		w.listOpt.ResourceVersion = daemonSets.ResourceVersion
	}

	return nil
}

// Start wathing daemonsets.
func (w *DaemonSetWatcher) Start(ctx context.Context, watchConsumer WatchConsumer) {
	start(ctx, w.name, watchChange(ctx, w.client, w.listOpt, watchConsumer))
}

// NewDaemonSetWatcher consructs a new daemonset watcher.
func NewDaemonSetWatcher(client tappv1.DaemonSetInterface, name string) *DaemonSetWatcher {
	return &DaemonSetWatcher{
		name:   name,
		client: client,
		log:    ctrl.Log.WithName(name + " daemonset watcher"),
	}
}

package watchers

import (
	"context"
	"math"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
)

const maxWaitPeriod = 120

// WatchConsumer consumes watch events.
type WatchConsumer func(<-chan watch.Event) error

// watchFunc manages object watching.
type watchFunc func(context.Context, time.Duration) error

// start wathing given resources.
func start(ctx context.Context, name string, watchFunc watchFunc) {
	go func() {
		log := ctrl.Log.WithName(name + " watcher")

		for waitPeriod := 0; ; waitPeriod = int(math.Min(float64((waitPeriod+1)*2), float64(maxWaitPeriod))) {
			waitDuration := time.Second * time.Duration(waitPeriod)
			if waitPeriod != 0 {
				log.Info("wait before start", "period", waitPeriod)
				time.Sleep(waitDuration)
			}

			err := watchFunc(ctx, waitDuration)
			if err != nil {
				log.Error(err, "there was an error during watch")
				// Increase wait period of reconnect.
				continue
			}

			// Watch should be stop.
			log.Info("watch has ended")
			break
		}
	}()
}

type watchClient interface {
	Watch(context.Context, metav1.ListOptions) (watch.Interface, error)
}

// watchChange return a func which does watch and calls consumer.
func watchChange(ctx context.Context, watchClient watchClient, listOpt metav1.ListOptions, consume WatchConsumer) watchFunc {
	return func(ctx context.Context, waitDuration time.Duration) error {
		watcher, err := watchClient.Watch(ctx, listOpt)
		if err != nil {
			return err
		}
		defer watcher.Stop()

		if waitDuration != 0 {
			dsWatcherLog.Info("wait before reading channel", "period", waitDuration)
			time.Sleep(waitDuration)
		}

		return consume(watcher.ResultChan())
	}
}

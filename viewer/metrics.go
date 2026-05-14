package viewer

import (
	encoremetrics "encore.dev/metrics"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
)

type encoreCounter struct {
	counter *encoremetrics.Counter[uint64]
}

func (c encoreCounter) Add(delta uint64) {
	if c.counter != nil {
		c.counter.Add(delta)
	}
}

func (c encoreCounter) Increment() {
	if c.counter != nil {
		c.counter.Increment()
	}
}

type encoreCounterGroup[L comparable] struct {
	group *encoremetrics.CounterGroup[L, uint64]
}

func (g encoreCounterGroup[L]) With(labels L) observability.Counter {
	if g.group == nil {
		return noopCounter{}
	}
	return encoreCounter{counter: g.group.With(labels)}
}

type noopCounter struct{}

func (noopCounter) Add(uint64) {}

func (noopCounter) Increment() {}

var (
	encoreHTTPRequests = encoreCounterGroup[observability.HTTPLabels]{
		group: encoremetrics.NewCounterGroup[observability.HTTPLabels, uint64](
			"viewer_http_route_requests_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreOperationRequests = encoreCounterGroup[observability.OperationLabels]{
		group: encoremetrics.NewCounterGroup[observability.OperationLabels, uint64](
			"viewer_operation_requests_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreOperationDurationMS = encoreCounterGroup[observability.OperationLabels]{
		group: encoremetrics.NewCounterGroup[observability.OperationLabels, uint64](
			"viewer_operation_duration_milliseconds_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreOperationErrors = encoreCounterGroup[observability.OperationErrorLabels]{
		group: encoremetrics.NewCounterGroup[observability.OperationErrorLabels, uint64](
			"viewer_operation_errors_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreKubernetesRequests = encoreCounterGroup[observability.KubernetesLabels]{
		group: encoremetrics.NewCounterGroup[observability.KubernetesLabels, uint64](
			"viewer_kubernetes_operation_requests_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreKubernetesDuration = encoreCounterGroup[observability.KubernetesLabels]{
		group: encoremetrics.NewCounterGroup[observability.KubernetesLabels, uint64](
			"viewer_kubernetes_operation_duration_milliseconds_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreKubernetesErrors = encoreCounterGroup[observability.KubernetesErrorLabels]{
		group: encoremetrics.NewCounterGroup[observability.KubernetesErrorLabels, uint64](
			"viewer_kubernetes_operation_errors_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreViewerSessionEvents = encoreCounterGroup[observability.EventLabels]{
		group: encoremetrics.NewCounterGroup[observability.EventLabels, uint64](
			"viewer_session_events_total",
			encoremetrics.CounterConfig{},
		),
	}
	encorePodSessionEvents = encoreCounterGroup[observability.EventLabels]{
		group: encoremetrics.NewCounterGroup[observability.EventLabels, uint64](
			"viewer_pod_session_events_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreAuthRequestEvents = encoreCounterGroup[observability.EventLabels]{
		group: encoremetrics.NewCounterGroup[observability.EventLabels, uint64](
			"viewer_auth_request_events_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreFileBrowserLogins = encoreCounterGroup[observability.FileBrowserLoginLabels]{
		group: encoremetrics.NewCounterGroup[observability.FileBrowserLoginLabels, uint64](
			"viewer_filebrowser_logins_total",
			encoremetrics.CounterConfig{},
		),
	}
	encoreCleanupDeleted = encoreCounter{
		counter: encoremetrics.NewCounter[uint64](
			"viewer_cleanup_deleted_total",
			encoremetrics.CounterConfig{},
		),
	}
)

func encoreMetricSources() observability.MetricSources {
	return observability.MetricSources{
		HTTPRequests:        encoreHTTPRequests,
		OperationRequests:   encoreOperationRequests,
		OperationDurationMS: encoreOperationDurationMS,
		OperationErrors:     encoreOperationErrors,
		KubernetesRequests:  encoreKubernetesRequests,
		KubernetesDuration:  encoreKubernetesDuration,
		KubernetesErrors:    encoreKubernetesErrors,
		ViewerSessionEvents: encoreViewerSessionEvents,
		PodSessionEvents:    encorePodSessionEvents,
		AuthRequestEvents:   encoreAuthRequestEvents,
		FileBrowserLogins:   encoreFileBrowserLogins,
		CleanupDeleted:      encoreCleanupDeleted,
	}
}

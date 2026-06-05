package observability

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type localMetricInfo struct {
	name string
	help string
}

var (
	httpRequestsMetric = localMetricInfo{
		name: "viewer_http_route_requests_total",
		help: "Total HTTP requests handled by viewer route, method, and status class.",
	}
	operationRequestsMetric = localMetricInfo{
		name: "viewer_operation_requests_total",
		help: "Total traced viewer operations by operation name and result.",
	}
	operationDurationMSMetric = localMetricInfo{
		name: "viewer_operation_duration_milliseconds_total",
		help: "Total traced viewer operation duration in milliseconds by operation name and result.",
	}
	operationErrorsMetric = localMetricInfo{
		name: "viewer_operation_errors_total",
		help: "Total traced viewer operation errors by operation name.",
	}
	kubernetesRequestsMetric = localMetricInfo{
		name: "viewer_kubernetes_operation_requests_total",
		help: "Total Kubernetes client operations by operation, resource, and result.",
	}
	kubernetesDurationMetric = localMetricInfo{
		name: "viewer_kubernetes_operation_duration_milliseconds_total",
		help: "Total Kubernetes client operation duration in milliseconds by operation, resource, and result.",
	}
	kubernetesErrorsMetric = localMetricInfo{
		name: "viewer_kubernetes_operation_errors_total",
		help: "Total Kubernetes client operation errors by operation and resource.",
	}
	viewerSessionEventsMetric = localMetricInfo{
		name: "viewer_session_events_total",
		help: "Total viewer session lifecycle events by event.",
	}
	podSessionEventsMetric = localMetricInfo{
		name: "viewer_pod_session_events_total",
		help: "Total File Browser pod session lifecycle events by event.",
	}
	authRequestEventsMetric = localMetricInfo{
		name: "viewer_auth_request_events_total",
		help: "Total File Browser auth request lifecycle events by event.",
	}
	fileBrowserLoginsMetric = localMetricInfo{
		name: "viewer_filebrowser_logins_total",
		help: "Total File Browser login attempts by result.",
	}
	cleanupDeletedMetric = localMetricInfo{
		name: "viewer_cleanup_deleted_total",
		help: "Total Kubernetes resources deleted by viewer cleanup.",
	}
)

func (r *Recorder) writeLocalPrometheus(w io.Writer) {
	_, _ = io.WriteString(w, "# Metrics are mirrored locally for /metrics and exported by the Encore runtime according to infra-config.json.\n")
	writeLocalGroup(w, httpRequestsMetric, r.metrics.httpRequests.Values(), func(labels HTTPLabels) string {
		return prometheusLabels("Method", labels.Method, "Route", labels.Route, "StatusClass", labels.StatusClass)
	})
	writeLocalGroup(w, operationRequestsMetric, r.metrics.operationRequests.Values(), func(labels OperationLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Result", labels.Result)
	})
	writeLocalGroup(w, operationDurationMSMetric, r.metrics.operationDurationMS.Values(), func(labels OperationLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Result", labels.Result)
	})
	writeLocalGroup(w, operationErrorsMetric, r.metrics.operationErrors.Values(), func(labels OperationErrorLabels) string {
		return prometheusLabels("Operation", labels.Operation)
	})
	writeLocalGroup(w, kubernetesRequestsMetric, r.metrics.kubernetesRequests.Values(), func(labels KubernetesLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource, "Result", labels.Result)
	})
	writeLocalGroup(w, kubernetesDurationMetric, r.metrics.kubernetesDuration.Values(), func(labels KubernetesLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource, "Result", labels.Result)
	})
	writeLocalGroup(w, kubernetesErrorsMetric, r.metrics.kubernetesErrors.Values(), func(labels KubernetesErrorLabels) string {
		return prometheusLabels("Operation", labels.Operation, "Resource", labels.Resource)
	})
	writeLocalGroup(w, viewerSessionEventsMetric, r.metrics.viewerSessionEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, podSessionEventsMetric, r.metrics.podSessionEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, authRequestEventsMetric, r.metrics.authRequestEvents.Values(), func(labels EventLabels) string {
		return prometheusLabels("Event", labels.Event)
	})
	writeLocalGroup(w, fileBrowserLoginsMetric, r.metrics.fileBrowserLogins.Values(), func(labels FileBrowserLoginLabels) string {
		return prometheusLabels("Result", labels.Result)
	})
	writeLocalCounter(w, cleanupDeletedMetric, r.metrics.cleanupDeleted.Value())
}

func writeLocalGroup[L comparable](
	w io.Writer,
	metric localMetricInfo,
	values map[L]uint64,
	labels func(L) string,
) {
	writeLocalMetricHeader(w, metric)
	lines := make([]string, 0, len(values))
	for labelSet, value := range values {
		if value == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s%s %d\n", metric.name, labels(labelSet), value))
	}
	sort.Strings(lines)
	for _, line := range lines {
		_, _ = io.WriteString(w, line)
	}
}

func writeLocalCounter(w io.Writer, metric localMetricInfo, value uint64) {
	writeLocalMetricHeader(w, metric)
	_, _ = fmt.Fprintf(w, "%s %d\n", metric.name, value)
}

func writeLocalMetricHeader(w io.Writer, metric localMetricInfo) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n", metric.name, metric.help)
	_, _ = fmt.Fprintf(w, "# TYPE %s counter\n", metric.name)
}

func prometheusLabels(pairs ...string) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, pairs[i], prometheusEscape(pairs[i+1])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func prometheusEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

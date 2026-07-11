package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/health"
)

const (
	metricsContentType  = "text/plain; version=0.0.4; charset=utf-8"
	metricsReadyTimeout = 2 * time.Second
	unknownMetricPath   = "/__unknown__"
)

var defaultHTTPDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type DatabaseStatsProvider interface {
	Stats() sql.DBStats
}

type Metrics struct {
	mu              sync.Mutex
	startedAt       time.Time
	version         string
	storageBackend  string
	requests        map[httpMetricKey]*httpMetricValue
	readyChecks     []health.Check
	dbStatsProvider DatabaseStatsProvider
}

type httpMetricKey struct {
	method string
	path   string
	status string
}

type httpMetricValue struct {
	count           uint64
	bytes           uint64
	durationSum     float64
	durationBuckets []uint64
}

type dependencyMetricState struct {
	name  string
	ready bool
}

func NewMetrics(version string, storageBackend string) *Metrics {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}
	storageBackend = strings.ToLower(strings.TrimSpace(storageBackend))
	if storageBackend == "" {
		storageBackend = "local"
	}

	return &Metrics{
		startedAt:      time.Now(),
		version:        version,
		storageBackend: storageBackend,
		requests:       map[httpMetricKey]*httpMetricValue{},
	}
}

func (m *Metrics) SetReadyChecks(checks []health.Check) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyChecks = append([]health.Check(nil), checks...)
}

func (m *Metrics) SetDBStatsProvider(provider DatabaseStatsProvider) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dbStatsProvider = provider
}

func (m *Metrics) ObserveHTTPRequest(method string, path string, status int, duration time.Duration, bytes int64) {
	if m == nil {
		return
	}
	if status <= 0 {
		status = http.StatusOK
	}
	if bytes < 0 {
		bytes = 0
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}

	key := httpMetricKey{
		method: normalizeMetricMethod(method),
		path:   normalizeMetricPath(path),
		status: strconv.Itoa(status),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	metric := m.requests[key]
	if metric == nil {
		metric = &httpMetricValue{durationBuckets: make([]uint64, len(defaultHTTPDurationBuckets))}
		m.requests[key] = metric
	}
	metric.count++
	metric.bytes += uint64(bytes)
	metric.durationSum += seconds
	for i, bucket := range defaultHTTPDurationBuckets {
		if seconds <= bucket {
			metric.durationBuckets[i]++
		}
	}
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{
			Error: ErrorBody{Code: "METHOD_NOT_ALLOWED", Message: "method not allowed"},
		})
		return
	}

	w.Header().Set("Content-Type", metricsContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(m.Render(r.Context())))
}

func (m *Metrics) Render(ctx context.Context) string {
	if m == nil {
		return ""
	}

	m.mu.Lock()
	startedAt := m.startedAt
	version := m.version
	storageBackend := m.storageBackend
	requests := cloneHTTPMetrics(m.requests)
	readyChecks := append([]health.Check(nil), m.readyChecks...)
	dbStatsProvider := m.dbStatsProvider
	m.mu.Unlock()

	dependencies := dependencyMetrics(ctx, readyChecks)

	var builder strings.Builder
	writeMetricHeader(&builder, "mm_chat_build_info", "Build and storage configuration information for this mm-chat API process.", "gauge")
	fmt.Fprintf(
		&builder,
		"mm_chat_build_info{version=%s,storage_backend=%s} 1\n",
		promLabelValue(version),
		promLabelValue(storageBackend),
	)

	writeMetricHeader(&builder, "mm_chat_process_uptime_seconds", "Seconds since this mm-chat API process metrics collector was created.", "gauge")
	fmt.Fprintf(&builder, "mm_chat_process_uptime_seconds %.3f\n", time.Since(startedAt).Seconds())

	writeHTTPMetrics(&builder, requests)
	writeDependencyMetrics(&builder, dependencies)
	writeDBStatsMetrics(&builder, dbStatsProvider)

	return builder.String()
}

func withRequestMetrics(metrics *Metrics) Middleware {
	return func(next http.Handler) http.Handler {
		if metrics == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &loggingResponseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			}

			defer func() {
				metrics.ObserveHTTPRequest(r.Method, r.URL.Path, recorder.status, time.Since(start), recorder.bytes)
			}()

			next.ServeHTTP(recorder, r)
		})
	}
}

func cloneHTTPMetrics(metrics map[httpMetricKey]*httpMetricValue) map[httpMetricKey]httpMetricValue {
	cloned := make(map[httpMetricKey]httpMetricValue, len(metrics))
	for key, value := range metrics {
		if value == nil {
			continue
		}
		copyValue := *value
		copyValue.durationBuckets = append([]uint64(nil), value.durationBuckets...)
		cloned[key] = copyValue
	}

	return cloned
}

func writeHTTPMetrics(builder *strings.Builder, metrics map[httpMetricKey]httpMetricValue) {
	writeMetricHeader(builder, "mm_chat_http_requests_total", "Total HTTP requests observed by method, bounded path label, and status code.", "counter")
	for _, key := range sortedHTTPMetricKeys(metrics) {
		value := metrics[key]
		fmt.Fprintf(builder, "mm_chat_http_requests_total%s %d\n", httpMetricLabels(key, ""), value.count)
	}

	writeMetricHeader(builder, "mm_chat_http_response_bytes_total", "Total HTTP response bytes observed by method, bounded path label, and status code.", "counter")
	for _, key := range sortedHTTPMetricKeys(metrics) {
		value := metrics[key]
		fmt.Fprintf(builder, "mm_chat_http_response_bytes_total%s %d\n", httpMetricLabels(key, ""), value.bytes)
	}

	writeMetricHeader(builder, "mm_chat_http_request_duration_seconds", "HTTP request latency histogram by method, bounded path label, and status code.", "histogram")
	for _, key := range sortedHTTPMetricKeys(metrics) {
		value := metrics[key]
		for i, bucket := range defaultHTTPDurationBuckets {
			var count uint64
			if i < len(value.durationBuckets) {
				count = value.durationBuckets[i]
			}
			fmt.Fprintf(
				builder,
				"mm_chat_http_request_duration_seconds_bucket%s %d\n",
				httpMetricLabels(key, strconv.FormatFloat(bucket, 'f', -1, 64)),
				count,
			)
		}
		fmt.Fprintf(builder, "mm_chat_http_request_duration_seconds_bucket%s %d\n", httpMetricLabels(key, "+Inf"), value.count)
		fmt.Fprintf(builder, "mm_chat_http_request_duration_seconds_sum%s %.6f\n", httpMetricLabels(key, ""), value.durationSum)
		fmt.Fprintf(builder, "mm_chat_http_request_duration_seconds_count%s %d\n", httpMetricLabels(key, ""), value.count)
	}
}

func sortedHTTPMetricKeys(metrics map[httpMetricKey]httpMetricValue) []httpMetricKey {
	keys := make([]httpMetricKey, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].status < keys[j].status
	})

	return keys
}

func writeDependencyMetrics(builder *strings.Builder, states []dependencyMetricState) {
	writeMetricHeader(builder, "mm_chat_dependency_ready", "Dependency readiness observed by the backend metrics endpoint. 1 means ready, 0 means not ready.", "gauge")
	for _, state := range states {
		value := 0
		if state.ready {
			value = 1
		}
		fmt.Fprintf(builder, "mm_chat_dependency_ready{dependency=%s} %d\n", promLabelValue(state.name), value)
	}
}

func dependencyMetrics(ctx context.Context, checks []health.Check) []dependencyMetricState {
	states := make([]dependencyMetricState, 0, len(checks))
	if len(checks) == 0 {
		return states
	}

	ctx, cancel := context.WithTimeout(ctx, metricsReadyTimeout)
	defer cancel()

	for _, check := range checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			name = "dependency"
		}
		state := dependencyMetricState{name: name, ready: true}
		if check.Checker != nil {
			if err := check.Checker.CheckReady(ctx); err != nil {
				state.ready = false
			}
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].name < states[j].name })

	return states
}

func writeDBStatsMetrics(builder *strings.Builder, provider DatabaseStatsProvider) {
	if provider == nil {
		return
	}
	stats := provider.Stats()

	writeMetricHeader(builder, "mm_chat_postgres_max_open_connections", "Configured maximum open Postgres connections for the API pool.", "gauge")
	fmt.Fprintf(builder, "mm_chat_postgres_max_open_connections %d\n", stats.MaxOpenConnections)
	writeMetricHeader(builder, "mm_chat_postgres_open_connections", "Current open Postgres connections for the API pool.", "gauge")
	fmt.Fprintf(builder, "mm_chat_postgres_open_connections %d\n", stats.OpenConnections)
	writeMetricHeader(builder, "mm_chat_postgres_in_use_connections", "Current in-use Postgres connections for the API pool.", "gauge")
	fmt.Fprintf(builder, "mm_chat_postgres_in_use_connections %d\n", stats.InUse)
	writeMetricHeader(builder, "mm_chat_postgres_idle_connections", "Current idle Postgres connections for the API pool.", "gauge")
	fmt.Fprintf(builder, "mm_chat_postgres_idle_connections %d\n", stats.Idle)
	writeMetricHeader(builder, "mm_chat_postgres_wait_count_total", "Total waits for a Postgres connection from the API pool.", "counter")
	fmt.Fprintf(builder, "mm_chat_postgres_wait_count_total %d\n", stats.WaitCount)
	writeMetricHeader(builder, "mm_chat_postgres_wait_duration_seconds_total", "Total time spent waiting for a Postgres connection from the API pool.", "counter")
	fmt.Fprintf(builder, "mm_chat_postgres_wait_duration_seconds_total %.6f\n", stats.WaitDuration.Seconds())
}

func writeMetricHeader(builder *strings.Builder, name string, help string, metricType string) {
	fmt.Fprintf(builder, "# HELP %s %s\n", name, help)
	fmt.Fprintf(builder, "# TYPE %s %s\n", name, metricType)
}

func httpMetricLabels(key httpMetricKey, le string) string {
	labels := []string{
		"method=" + promLabelValue(key.method),
		"path=" + promLabelValue(key.path),
		"status=" + promLabelValue(key.status),
	}
	if le != "" {
		labels = append(labels, "le="+promLabelValue(le))
	}

	return "{" + strings.Join(labels, ",") + "}"
}

func promLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return "\"" + value + "\""
}

func normalizeMetricPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return unknownMetricPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	if normalized, ok := knownMetricPath(path); ok {
		return normalized
	}

	return unknownMetricPath
}

func normalizeMetricMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet:
		return http.MethodGet
	case http.MethodHead:
		return http.MethodHead
	case http.MethodPost:
		return http.MethodPost
	case http.MethodPut:
		return http.MethodPut
	case http.MethodPatch:
		return http.MethodPatch
	case http.MethodDelete:
		return http.MethodDelete
	case http.MethodOptions:
		return http.MethodOptions
	default:
		return "OTHER"
	}
}

func knownMetricPath(path string) (string, bool) {
	switch path {
	case "/", "/health", "/ready", "/metrics", "/v1/version", "/v1/me",
		"/v1/me/sessions":
		return path, true
	case "/v1/auth/login", "/v1/auth/logout", "/v1/auth/invites/accept",
		"/v1/auth/recovery/request", "/v1/auth/recovery/complete":
		return path, true
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[1] == "v1" && parts[2] == "knowledge" && parts[3] == "collections" {
		switch len(parts) {
		case 4:
			return "/v1/knowledge/collections", true
		case 5:
			return "/v1/knowledge/collections/{collectionId}", true
		case 6:
			if parts[5] == "documents" {
				return "/v1/knowledge/collections/{collectionId}/documents", true
			}
		}
	}
	if len(parts) >= 5 && parts[1] == "v1" && parts[2] == "knowledge" && parts[3] == "documents" {
		switch len(parts) {
		case 5:
			return "/v1/knowledge/documents/{documentId}", true
		case 6:
			switch parts[5] {
			case "content":
				return "/v1/knowledge/documents/{documentId}/content", true
			case "versions":
				return "/v1/knowledge/documents/{documentId}/versions", true
			}
		}
	}
	if len(parts) >= 3 && parts[1] == "v1" && parts[2] == "teams" {
		switch len(parts) {
		case 3:
			return "/v1/teams", true
		case 4:
			return "/v1/teams/{teamId}", true
		case 5:
			switch parts[4] {
			case "members":
				return "/v1/teams/{teamId}/members", true
			case "membership":
				return "/v1/teams/{teamId}/membership", true
			case "invites":
				return "/v1/teams/{teamId}/invites", true
			}
		case 6:
			switch parts[4] {
			case "members":
				return "/v1/teams/{teamId}/members/{userId}", true
			case "invites":
				return "/v1/teams/{teamId}/invites/{inviteId}", true
			}
		}
	}
	if len(parts) >= 4 && parts[1] == "v1" && parts[2] == "chat" && parts[3] == "conversations" {
		switch len(parts) {
		case 4:
			return "/v1/chat/conversations", true
		case 5:
			return "/v1/chat/conversations/{id}", true
		case 6:
			switch parts[5] {
			case "messages":
				return "/v1/chat/conversations/{id}/messages", true
			case "stream":
				return "/v1/chat/conversations/{id}/stream", true
			}
		}
	}
	if len(parts) == 6 && parts[1] == "v1" && parts[2] == "chat" && parts[3] == "runs" && parts[5] == "cancel" {
		return "/v1/chat/runs/{id}/cancel", true
	}
	if len(parts) >= 3 && parts[1] == "v1" && parts[2] == "files" {
		switch len(parts) {
		case 3:
			return "/v1/files", true
		case 4:
			return "/v1/files/{id}", true
		case 5:
			if parts[4] == "content" {
				return "/v1/files/{id}/content", true
			}
		}
	}
	if len(parts) >= 4 && parts[1] == "v1" && parts[2] == "import" && parts[3] == "browser" {
		switch len(parts) {
		case 4:
			return "/v1/import/browser", true
		case 5:
			if parts[4] == "preview" {
				return "/v1/import/browser/preview", true
			}
			return "/v1/import/browser/{id}", true
		}
	}

	return "", false
}

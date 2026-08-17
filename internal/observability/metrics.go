package observability

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsOnce sync.Once

	httpRequests *prometheus.CounterVec
	httpLatency  *prometheus.HistogramVec
	cmdTotal     *prometheus.CounterVec
	sagaPending  prometheus.Gauge
	sagaOldest   prometheus.Gauge
	jobDuration  *prometheus.HistogramVec
	jobFailures  *prometheus.CounterVec
)

func initMetrics() {
	metricsOnce.Do(func() {
		httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_http_requests_total",
			Help: "HTTP requests by method, path and status",
		}, []string{"method", "path", "status"})
		httpLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ledger_http_request_duration_seconds",
			Help:    "HTTP request latency",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"})
		cmdTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_command_total",
			Help: "Ledger commands by name and result",
		}, []string{"command", "result"})
		sagaPending = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ledger_saga_pending",
			Help: "Open transfer saga count",
		})
		sagaOldest = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ledger_saga_oldest_pending_seconds",
			Help: "Age of oldest open saga in seconds",
		})
		jobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ledger_worker_job_duration_seconds",
			Help:    "Worker job duration",
			Buckets: prometheus.DefBuckets,
		}, []string{"job"})
		jobFailures = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_worker_job_failures_total",
			Help: "Worker job failures",
		}, []string{"job"})
	})
}

func MetricsHandler() gin.HandlerFunc {
	initMetrics()
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func HTTPMetrics() gin.HandlerFunc {
	initMetrics()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		status := strconv.Itoa(c.Writer.Status())
		httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
		httpLatency.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

func ObserveCommand(command, result string) {
	initMetrics()
	if command == "" {
		command = "unknown"
	}
	if result == "" {
		result = "ok"
	}
	cmdTotal.WithLabelValues(command, result).Inc()
}

func SetSagaPending(count int, oldest time.Duration) {
	initMetrics()
	sagaPending.Set(float64(count))
	sagaOldest.Set(oldest.Seconds())
}

func ObserveWorkerJob(name, status string, d time.Duration) {
	initMetrics()
	if name == "" {
		return
	}
	jobDuration.WithLabelValues(name).Observe(d.Seconds())
	if status == "failed" || status == "dead" {
		jobFailures.WithLabelValues(name).Inc()
	}
}

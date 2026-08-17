package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func InitTrace(ctx context.Context, service string) func() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	ep := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if ep == "" {
		return func() {}
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(ep), otlptracehttp.WithInsecure())
	if err != nil {
		return func() {}
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(service),
		)),
	)
	otel.SetTracerProvider(tp)
	return func() { _ = tp.Shutdown(context.Background()) }
}

func GinTrace(service string) gin.HandlerFunc {
	return otelgin.Middleware(service)
}

func TraceIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ParseTraceID(r.Header.Get("traceparent"))
}

func ParseTraceID(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) >= 4 && len(parts[1]) == 32 {
		return parts[1]
	}
	return ""
}

func EnsureTraceparent() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("traceparent") == "" {
			c.Request.Header.Set("traceparent", NewTraceparent())
		}
		c.Writer.Header().Set("traceparent", c.GetHeader("traceparent"))
		c.Set("trace_id", ParseTraceID(c.GetHeader("traceparent")))
		c.Next()
	}
}

func NewTraceparent() string {
	tid := make([]byte, 16)
	sid := make([]byte, 8)
	_, _ = rand.Read(tid)
	_, _ = rand.Read(sid)
	return "00-" + hex.EncodeToString(tid) + "-" + hex.EncodeToString(sid) + "-01"
}

func RequestLog(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		c.Next()
		rid := c.Writer.Header().Get("X-Request-Id")
		if rid == "" {
			rid = c.GetHeader("X-Request-Id")
		}
		tid, _ := c.Get("trace_id")
		traceID, _ := tid.(string)
		if traceID == "" {
			traceID = TraceIDFromRequest(c.Request)
		}
		tenant := c.GetHeader("X-Tenant-Id")
		clientID := c.GetHeader("X-Client-Id")
		if v, ok := c.Get("client_id"); ok {
			if s, ok := v.(string); ok && s != "" {
				clientID = s
			}
		}
		bizNo, _ := c.Get("biz_no")
		journalID, _ := c.Get("journal_id")
		log.Info("http",
			zap.String("request_id", rid),
			zap.String("trace_id", traceID),
			zap.String("tenant_id", tenant),
			zap.String("client_id", clientID),
			zap.String("biz_no", stringify(bizNo)),
			zap.String("journal_id", stringify(journalID)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}

func stringify(v interface{}) string {
	s, _ := v.(string)
	return s
}

func InjectUpstream(req *http.Request, requestID, traceparent string) {
	if req == nil {
		return
	}
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
}

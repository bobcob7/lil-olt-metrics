package ingest

import (
	"context"

	"github.com/bobcob7/lil-olt-metrics/internal/sessions"
	"github.com/bobcob7/lil-olt-metrics/internal/store"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

//go:generate moq -out moq_test.go . metricsTranslator metricsStore logsTranslator sessionsStore

type metricsTranslator interface {
	Translate(req *colmetricspb.ExportMetricsServiceRequest, app store.Appender) (int, error)
}

type metricsStore interface {
	Appender(ctx context.Context) store.Appender
}

type logsTranslator interface {
	Translate(req *collogspb.ExportLogsServiceRequest) ([]sessions.Event, error)
}

type sessionsStore interface {
	AppendEvent(ctx context.Context, evt sessions.Event) error
}

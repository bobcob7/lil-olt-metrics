package ingest

import (
	"context"

	"github.com/bobcob7/lil-olt-metrics/internal/store"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

//go:generate moq -out moq_test.go . metricsTranslator metricsStore

type metricsTranslator interface {
	Translate(req *colmetricspb.ExportMetricsServiceRequest, app store.Appender) (int, error)
}

type metricsStore interface {
	Appender(ctx context.Context) store.Appender
}

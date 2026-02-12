package ingest

import (
	"context"
	"log/slog"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCHandler implements the OTLP MetricsService gRPC endpoint.
type GRPCHandler struct {
	colmetricspb.UnimplementedMetricsServiceServer
	logger     *slog.Logger
	translator metricsTranslator
	store      metricsStore
}

// NewGRPCHandler creates a GRPCHandler with the given dependencies.
func NewGRPCHandler(logger *slog.Logger, translator metricsTranslator, store metricsStore) *GRPCHandler {
	return &GRPCHandler{
		logger:     logger,
		translator: translator,
		store:      store,
	}
}

// Export handles an OTLP ExportMetricsServiceRequest.
func (h *GRPCHandler) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	if req == nil || len(req.GetResourceMetrics()) == 0 {
		return &colmetricspb.ExportMetricsServiceResponse{}, nil
	}
	app := h.store.Appender(ctx)
	count, err := h.translator.Translate(req, app)
	if err != nil {
		h.logger.WarnContext(ctx, "partial translation failure", "error", err, "samples_written", count)
		if count == 0 {
			_ = app.Rollback()
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if commitErr := app.Commit(); commitErr != nil {
			h.logger.ErrorContext(ctx, "commit failed after partial translation", "error", commitErr)
			_ = app.Rollback()
			return nil, status.Error(codes.Internal, commitErr.Error())
		}
		resp := &colmetricspb.ExportMetricsServiceResponse{
			PartialSuccess: &colmetricspb.ExportMetricsPartialSuccess{
				ErrorMessage: err.Error(),
			},
		}
		h.logger.InfoContext(ctx, "ingested metrics (partial)", "samples", count)
		return resp, nil
	}
	if err := app.Commit(); err != nil {
		h.logger.ErrorContext(ctx, "commit failed", "error", err)
		_ = app.Rollback()
		return nil, status.Error(codes.Internal, err.Error())
	}
	h.logger.DebugContext(ctx, "ingested metrics", "samples", count)
	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

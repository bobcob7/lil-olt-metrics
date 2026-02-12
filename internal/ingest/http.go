package ingest

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// HTTPHandler handles OTLP metrics over HTTP (protobuf only).
type HTTPHandler struct {
	logger      *slog.Logger
	translator  metricsTranslator
	store       metricsStore
	maxBodySize int
}

// NewHTTPHandler creates an HTTPHandler with the given dependencies.
func NewHTTPHandler(logger *slog.Logger, translator metricsTranslator, store metricsStore, maxBodySize int) *HTTPHandler {
	return &HTTPHandler{
		logger:      logger,
		translator:  translator,
		store:       store,
		maxBodySize: maxBodySize,
	}
}

// ServeHTTP handles POST /v1/metrics with Content-Type: application/x-protobuf.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := h.readBody(r)
	if err != nil {
		h.logger.WarnContext(r.Context(), "failed to read request body", "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	req := &colmetricspb.ExportMetricsServiceRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		h.logger.WarnContext(r.Context(), "failed to unmarshal protobuf", "error", err)
		http.Error(w, "invalid protobuf payload", http.StatusBadRequest)
		return
	}
	if len(req.GetResourceMetrics()) == 0 {
		h.writeProtoResponse(w, &colmetricspb.ExportMetricsServiceResponse{})
		return
	}
	app := h.store.Appender(r.Context())
	count, translateErr := h.translator.Translate(req, app)
	if translateErr != nil {
		h.logger.WarnContext(r.Context(), "partial translation failure", "error", translateErr, "samples_written", count)
		if count == 0 {
			_ = app.Rollback()
			http.Error(w, translateErr.Error(), http.StatusBadRequest)
			return
		}
		if commitErr := app.Commit(); commitErr != nil {
			h.logger.ErrorContext(r.Context(), "commit failed after partial translation", "error", commitErr)
			_ = app.Rollback()
			http.Error(w, commitErr.Error(), http.StatusInternalServerError)
			return
		}
		resp := &colmetricspb.ExportMetricsServiceResponse{
			PartialSuccess: &colmetricspb.ExportMetricsPartialSuccess{
				ErrorMessage: translateErr.Error(),
			},
		}
		h.logger.InfoContext(r.Context(), "ingested metrics (partial)", "samples", count)
		h.writeProtoResponse(w, resp)
		return
	}
	if err := app.Commit(); err != nil {
		h.logger.ErrorContext(r.Context(), "commit failed", "error", err)
		_ = app.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logger.DebugContext(r.Context(), "ingested metrics", "samples", count)
	h.writeProtoResponse(w, &colmetricspb.ExportMetricsServiceResponse{})
}

func (h *HTTPHandler) readBody(r *http.Request) ([]byte, error) {
	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}
	return io.ReadAll(io.LimitReader(reader, int64(h.maxBodySize)))
}

func (h *HTTPHandler) writeProtoResponse(w http.ResponseWriter, resp *colmetricspb.ExportMetricsServiceResponse) {
	data, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

package ingest

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode"

	"github.com/bobcob7/lil-olt-metrics/internal/config"
	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/prometheus/prometheus/model/labels"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// Translator converts OTLP metrics into Prometheus-compatible samples.
type Translator struct {
	logger *slog.Logger
	cfg    config.TranslationConfig
}

// NewTranslator creates a Translator with the given config and logger.
func NewTranslator(logger *slog.Logger, cfg config.TranslationConfig) *Translator {
	return &Translator{logger: logger, cfg: cfg}
}

// Translate converts an OTLP ExportMetricsServiceRequest into samples
// written to the given Appender. Returns the number of samples written.
func (tr *Translator) Translate(req *colmetricspb.ExportMetricsServiceRequest, app store.Appender) (int, error) {
	if req == nil {
		return 0, nil
	}
	count := 0
	for _, rm := range req.GetResourceMetrics() {
		resourceLabels := tr.resourceLabels(rm.GetResource().GetAttributes())
		for _, sm := range rm.GetScopeMetrics() {
			scopeLabels := scopeLabels(sm.GetScope())
			for _, metric := range sm.GetMetrics() {
				n, err := tr.translateMetric(metric, resourceLabels, scopeLabels, app)
				if err != nil {
					return count, fmt.Errorf("translating metric %q: %w", metric.GetName(), err)
				}
				count += n
			}
		}
	}
	return count, nil
}

func (tr *Translator) translateMetric(m *metricspb.Metric, resourceLabels, scopeLabels labels.Labels, app store.Appender) (int, error) {
	name := m.GetName()
	if tr.cfg.SanitizeMetricNames {
		name = sanitizeMetricName(name)
	}
	if tr.cfg.AddUnitSuffix && m.GetUnit() != "" {
		name = addUnitSuffix(name, m.GetUnit())
	}
	switch {
	case m.GetGauge() != nil:
		return tr.translateGauge(name, m.GetGauge(), resourceLabels, scopeLabels, app)
	case m.GetSum() != nil:
		return tr.translateSum(name, m.GetSum(), resourceLabels, scopeLabels, app)
	case m.GetHistogram() != nil:
		return tr.translateHistogram(name, m.GetHistogram(), resourceLabels, scopeLabels, app)
	default:
		tr.logger.Warn("unsupported metric type, skipping", "name", name)
		return 0, nil
	}
}

func (tr *Translator) translateGauge(name string, g *metricspb.Gauge, resourceLabels, scopeLabels labels.Labels, app store.Appender) (int, error) {
	count := 0
	for _, dp := range g.GetDataPoints() {
		lset := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels)
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		v := numberValue(dp)
		if _, err := app.Append(0, lset, ts, v); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateSum(name string, s *metricspb.Sum, resourceLabels, scopeLabels labels.Labels, app store.Appender) (int, error) {
	if s.GetAggregationTemporality() == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		tr.logger.Warn("delta temporality not supported, skipping", "name", name)
		return 0, nil
	}
	if s.GetIsMonotonic() && tr.cfg.AddTypeSuffix {
		name = addTypeSuffix(name, "total")
	}
	count := 0
	for _, dp := range s.GetDataPoints() {
		lset := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels)
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		v := numberValue(dp)
		if _, err := app.Append(0, lset, ts, v); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateHistogram(name string, h *metricspb.Histogram, resourceLabels, scopeLabels labels.Labels, app store.Appender) (int, error) {
	if h.GetAggregationTemporality() == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		tr.logger.Warn("delta temporality not supported, skipping", "name", name)
		return 0, nil
	}
	count := 0
	for _, dp := range h.GetDataPoints() {
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		baseLabels := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels)
		n, err := tr.appendHistogramDataPoint(name, baseLabels, ts, dp, app)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

func (tr *Translator) appendHistogramDataPoint(name string, baseLabels labels.Labels, ts int64, dp *metricspb.HistogramDataPoint, app store.Appender) (int, error) {
	count := 0
	bounds := dp.GetExplicitBounds()
	bucketCounts := dp.GetBucketCounts()
	var cumulativeCount uint64
	for i, bound := range bounds {
		if i < len(bucketCounts) {
			cumulativeCount += bucketCounts[i]
		}
		bucketName := name + "_bucket"
		lset := appendLabel(baseLabels, bucketName, "le", fmt.Sprintf("%g", bound))
		if _, err := app.Append(0, lset, ts, float64(cumulativeCount)); err != nil {
			return count, err
		}
		count++
	}
	if len(bucketCounts) > len(bounds) {
		cumulativeCount += bucketCounts[len(bounds)]
	}
	infBucketName := name + "_bucket"
	infLset := appendLabel(baseLabels, infBucketName, "le", "+Inf")
	if _, err := app.Append(0, infLset, ts, float64(cumulativeCount)); err != nil {
		return count, err
	}
	count++
	countName := name + "_count"
	countLset := replaceMetricName(baseLabels, countName)
	if _, err := app.Append(0, countLset, ts, float64(dp.GetCount())); err != nil {
		return count, err
	}
	count++
	if dp.Sum != nil {
		sumName := name + "_sum"
		sumLset := replaceMetricName(baseLabels, sumName)
		if _, err := app.Append(0, sumLset, ts, dp.GetSum()); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) resourceLabels(attrs []*commonpb.KeyValue) labels.Labels {
	b := labels.NewBuilder(labels.EmptyLabels())
	for _, kv := range attrs {
		key := kv.GetKey()
		val := anyValueString(kv.GetValue())
		if mapped, ok := tr.cfg.ResourceAttributes.LabelMap[key]; ok {
			b.Set(mapped, val)
			continue
		}
		for _, promote := range tr.cfg.ResourceAttributes.Promote {
			if key == promote {
				b.Set(sanitizeLabelName(key), val)
				break
			}
		}
	}
	return b.Labels()
}

func scopeLabels(scope *commonpb.InstrumentationScope) labels.Labels {
	if scope == nil {
		return labels.EmptyLabels()
	}
	b := labels.NewBuilder(labels.EmptyLabels())
	if scope.GetName() != "" {
		b.Set("otel_scope_name", scope.GetName())
	}
	if scope.GetVersion() != "" {
		b.Set("otel_scope_version", scope.GetVersion())
	}
	return b.Labels()
}

func (tr *Translator) buildLabels(metricName string, attrs []*commonpb.KeyValue, resourceLabels, scopeLabels labels.Labels) labels.Labels {
	b := labels.NewBuilder(labels.EmptyLabels())
	resourceLabels.Range(func(l labels.Label) {
		b.Set(l.Name, l.Value)
	})
	scopeLabels.Range(func(l labels.Label) {
		b.Set(l.Name, l.Value)
	})
	for _, kv := range attrs {
		b.Set(sanitizeLabelName(kv.GetKey()), anyValueString(kv.GetValue()))
	}
	b.Set("__name__", metricName)
	return b.Labels()
}

func appendLabel(base labels.Labels, metricName, name, value string) labels.Labels {
	b := labels.NewBuilder(base)
	b.Set("__name__", metricName)
	b.Set(name, value)
	return b.Labels()
}

func replaceMetricName(base labels.Labels, newName string) labels.Labels {
	b := labels.NewBuilder(base)
	b.Set("__name__", newName)
	return b.Labels()
}

func numberValue(dp *metricspb.NumberDataPoint) float64 {
	switch dp.GetValue().(type) {
	case *metricspb.NumberDataPoint_AsDouble:
		return dp.GetAsDouble()
	case *metricspb.NumberDataPoint_AsInt:
		return float64(dp.GetAsInt())
	default:
		return 0
	}
}

func anyValueString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return v.GetStringValue()
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.GetBoolValue())
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.GetIntValue())
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", v.GetDoubleValue())
	default:
		return ""
	}
}

var consecutiveUnderscores = regexp.MustCompile(`_+`)

func sanitizeMetricName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' {
			return r
		}
		return '_'
	}, name)
	name = consecutiveUnderscores.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) > 0 && unicode.IsDigit(rune(name[0])) {
		name = "_" + name
	}
	return name
}

func sanitizeLabelName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return '_'
	}, name)
	name = consecutiveUnderscores.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) > 0 && unicode.IsDigit(rune(name[0])) {
		name = "_" + name
	}
	return name
}

var unitMap = map[string]string{
	"s":   "seconds",
	"ms":  "milliseconds",
	"us":  "microseconds",
	"ns":  "nanoseconds",
	"m":   "meters",
	"km":  "kilometers",
	"By":  "bytes",
	"KBy": "kilobytes",
	"MBy": "megabytes",
	"GBy": "gigabytes",
	"1":   "ratio",
	"%":   "percent",
}

func addUnitSuffix(name, unit string) string {
	if mapped, ok := unitMap[unit]; ok {
		unit = mapped
	}
	suffix := "_" + sanitizeMetricName(unit)
	if strings.HasSuffix(name, suffix) {
		return name
	}
	return name + suffix
}

func addTypeSuffix(name, suffix string) string {
	s := "_" + suffix
	if strings.HasSuffix(name, s) {
		return name
	}
	return name + s
}

// Exemplar holds Prometheus-style exemplar data extracted from OTLP.
type Exemplar struct {
	Labels  labels.Labels
	Value   float64
	Ts      int64
	TraceID string
	SpanID  string
}

// ExtractExemplars converts OTLP exemplars to Prometheus-style exemplars.
func ExtractExemplars(exemplars []*metricspb.Exemplar) []Exemplar {
	if len(exemplars) == 0 {
		return nil
	}
	result := make([]Exemplar, 0, len(exemplars))
	for _, e := range exemplars {
		if e == nil {
			continue
		}
		ex := Exemplar{
			Ts: int64(e.GetTimeUnixNano() / 1_000_000),
		}
		switch e.GetValue().(type) {
		case *metricspb.Exemplar_AsDouble:
			ex.Value = e.GetAsDouble()
		case *metricspb.Exemplar_AsInt:
			ex.Value = float64(e.GetAsInt())
		}
		if len(e.GetTraceId()) > 0 {
			ex.TraceID = hex.EncodeToString(e.GetTraceId())
		}
		if len(e.GetSpanId()) > 0 {
			ex.SpanID = hex.EncodeToString(e.GetSpanId())
		}
		b := labels.NewBuilder(labels.EmptyLabels())
		for _, kv := range e.GetFilteredAttributes() {
			b.Set(sanitizeLabelName(kv.GetKey()), anyValueString(kv.GetValue()))
		}
		if ex.TraceID != "" {
			b.Set("trace_id", ex.TraceID)
		}
		if ex.SpanID != "" {
			b.Set("span_id", ex.SpanID)
		}
		ex.Labels = b.Labels()
		result = append(result, ex)
	}
	return result
}

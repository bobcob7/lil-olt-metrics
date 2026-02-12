package ingest

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/bobcob7/lil-olt-metrics/internal/config"
	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/prometheus/prometheus/model/labels"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

const deltaTTL = 10 * time.Minute

// Translator converts OTLP metrics into Prometheus-compatible samples.
type Translator struct {
	logger *slog.Logger
	cfg    config.TranslationConfig
	mu     sync.Mutex
	delta  map[uint64]*deltaEntry
}

type deltaEntry struct {
	value    float64
	lastSeen time.Time
}

// NewTranslator creates a Translator with the given config and logger.
func NewTranslator(logger *slog.Logger, cfg config.TranslationConfig) *Translator {
	return &Translator{
		logger: logger,
		cfg:    cfg,
		delta:  make(map[uint64]*deltaEntry),
	}
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
		var schemaLabel labels.Labels
		if tr.cfg.SchemaURL && rm.GetSchemaUrl() != "" {
			schemaLabel = labels.FromStrings("__schema_url__", rm.GetSchemaUrl())
		}
		for _, sm := range rm.GetScopeMetrics() {
			scopeLabels := scopeLabels(sm.GetScope())
			if tr.cfg.SchemaURL && sm.GetSchemaUrl() != "" {
				schemaLabel = labels.FromStrings("__schema_url__", sm.GetSchemaUrl())
			}
			for _, metric := range sm.GetMetrics() {
				n, err := tr.translateMetric(metric, resourceLabels, scopeLabels, schemaLabel, app)
				if err != nil {
					return count, fmt.Errorf("translating metric %q: %w", metric.GetName(), err)
				}
				count += n
			}
		}
	}
	return count, nil
}

func (tr *Translator) translateMetric(m *metricspb.Metric, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	name := m.GetName()
	if tr.cfg.SanitizeMetricNames {
		name = sanitizeMetricName(name)
	}
	if tr.cfg.AddUnitSuffix && m.GetUnit() != "" {
		name = addUnitSuffix(name, m.GetUnit())
	}
	switch {
	case m.GetGauge() != nil:
		return tr.translateGauge(name, m.GetGauge(), resourceLabels, scopeLabels, schemaLabels, app)
	case m.GetSum() != nil:
		return tr.translateSum(name, m.GetSum(), resourceLabels, scopeLabels, schemaLabels, app)
	case m.GetHistogram() != nil:
		return tr.translateHistogram(name, m.GetHistogram(), resourceLabels, scopeLabels, schemaLabels, app)
	case m.GetExponentialHistogram() != nil:
		return tr.translateExponentialHistogram(name, m.GetExponentialHistogram(), resourceLabels, scopeLabels, schemaLabels, app)
	case m.GetSummary() != nil:
		return tr.translateSummary(name, m.GetSummary(), resourceLabels, scopeLabels, schemaLabels, app)
	default:
		tr.logger.Warn("unsupported metric type, skipping", "name", name)
		return 0, nil
	}
}

func (tr *Translator) translateGauge(name string, g *metricspb.Gauge, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	count := 0
	for _, dp := range g.GetDataPoints() {
		lset := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels, schemaLabels)
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		v := numberValue(dp)
		if _, err := app.Append(0, lset, ts, v); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateSum(name string, s *metricspb.Sum, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	isDelta := s.GetAggregationTemporality() == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	if isDelta && !tr.cfg.DeltaConversion {
		tr.logger.Warn("delta temporality not supported, skipping", "name", name)
		return 0, nil
	}
	if s.GetIsMonotonic() && tr.cfg.AddTypeSuffix {
		name = addTypeSuffix(name, "total")
	}
	count := 0
	for _, dp := range s.GetDataPoints() {
		lset := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels, schemaLabels)
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		v := numberValue(dp)
		if isDelta {
			v = tr.accumulateDelta(lset, v)
		}
		if _, err := app.Append(0, lset, ts, v); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateHistogram(name string, h *metricspb.Histogram, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	isDelta := h.GetAggregationTemporality() == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	if isDelta && !tr.cfg.DeltaConversion {
		tr.logger.Warn("delta temporality not supported, skipping", "name", name)
		return 0, nil
	}
	count := 0
	for _, dp := range h.GetDataPoints() {
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		baseLabels := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels, schemaLabels)
		n, err := tr.appendHistogramDataPoint(name, baseLabels, ts, dp, isDelta, app)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

func (tr *Translator) appendHistogramDataPoint(name string, baseLabels labels.Labels, ts int64, dp *metricspb.HistogramDataPoint, isDelta bool, app store.Appender) (int, error) {
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
		v := float64(cumulativeCount)
		if isDelta {
			v = tr.accumulateDelta(lset, v)
		}
		if _, err := app.Append(0, lset, ts, v); err != nil {
			return count, err
		}
		count++
	}
	if len(bucketCounts) > len(bounds) {
		cumulativeCount += bucketCounts[len(bounds)]
	}
	infBucketName := name + "_bucket"
	infLset := appendLabel(baseLabels, infBucketName, "le", "+Inf")
	infV := float64(cumulativeCount)
	if isDelta {
		infV = tr.accumulateDelta(infLset, infV)
	}
	if _, err := app.Append(0, infLset, ts, infV); err != nil {
		return count, err
	}
	count++
	countName := name + "_count"
	countLset := replaceMetricName(baseLabels, countName)
	countV := float64(dp.GetCount())
	if isDelta {
		countV = tr.accumulateDelta(countLset, countV)
	}
	if _, err := app.Append(0, countLset, ts, countV); err != nil {
		return count, err
	}
	count++
	if dp.Sum != nil {
		sumName := name + "_sum"
		sumLset := replaceMetricName(baseLabels, sumName)
		sumV := dp.GetSum()
		if isDelta {
			sumV = tr.accumulateDelta(sumLset, sumV)
		}
		if _, err := app.Append(0, sumLset, ts, sumV); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateExponentialHistogram(name string, h *metricspb.ExponentialHistogram, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	isDelta := h.GetAggregationTemporality() == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	if isDelta && !tr.cfg.DeltaConversion {
		tr.logger.Warn("delta temporality not supported for exponential histogram, skipping", "name", name)
		return 0, nil
	}
	count := 0
	for _, dp := range h.GetDataPoints() {
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		baseLabels := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels, schemaLabels)
		n, err := tr.appendExponentialHistogramDataPoint(name, baseLabels, ts, dp, isDelta, app)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

func (tr *Translator) appendExponentialHistogramDataPoint(name string, baseLabels labels.Labels, ts int64, dp *metricspb.ExponentialHistogramDataPoint, isDelta bool, app store.Appender) (int, error) {
	count := 0
	scale := dp.GetScale()
	base := math.Pow(2, math.Pow(2, float64(-scale)))
	bucketName := name + "_bucket"
	var cumulativeCount uint64
	cumulativeCount += dp.GetZeroCount()
	positive := dp.GetPositive()
	if positive != nil {
		offset := int(positive.GetOffset())
		for i, bc := range positive.GetBucketCounts() {
			cumulativeCount += bc
			bound := math.Pow(base, float64(offset+i+1))
			lset := appendLabel(baseLabels, bucketName, "le", fmt.Sprintf("%g", bound))
			v := float64(cumulativeCount)
			if isDelta {
				v = tr.accumulateDelta(lset, v)
			}
			if _, err := app.Append(0, lset, ts, v); err != nil {
				return count, err
			}
			count++
		}
	}
	infLset := appendLabel(baseLabels, bucketName, "le", "+Inf")
	infV := float64(cumulativeCount)
	if isDelta {
		infV = tr.accumulateDelta(infLset, infV)
	}
	if _, err := app.Append(0, infLset, ts, infV); err != nil {
		return count, err
	}
	count++
	countName := name + "_count"
	countLset := replaceMetricName(baseLabels, countName)
	countV := float64(dp.GetCount())
	if isDelta {
		countV = tr.accumulateDelta(countLset, countV)
	}
	if _, err := app.Append(0, countLset, ts, countV); err != nil {
		return count, err
	}
	count++
	if dp.Sum != nil {
		sumName := name + "_sum"
		sumLset := replaceMetricName(baseLabels, sumName)
		sumV := dp.GetSum()
		if isDelta {
			sumV = tr.accumulateDelta(sumLset, sumV)
		}
		if _, err := app.Append(0, sumLset, ts, sumV); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) translateSummary(name string, s *metricspb.Summary, resourceLabels, scopeLabels, schemaLabels labels.Labels, app store.Appender) (int, error) {
	count := 0
	for _, dp := range s.GetDataPoints() {
		ts := int64(dp.GetTimeUnixNano() / 1_000_000)
		baseLabels := tr.buildLabels(name, dp.GetAttributes(), resourceLabels, scopeLabels, schemaLabels)
		for _, qv := range dp.GetQuantileValues() {
			lset := appendLabel(baseLabels, name, "quantile", fmt.Sprintf("%g", qv.GetQuantile()))
			if _, err := app.Append(0, lset, ts, qv.GetValue()); err != nil {
				return count, err
			}
			count++
		}
		sumName := name + "_sum"
		sumLset := replaceMetricName(baseLabels, sumName)
		if _, err := app.Append(0, sumLset, ts, dp.GetSum()); err != nil {
			return count, err
		}
		count++
		countName := name + "_count"
		countLset := replaceMetricName(baseLabels, countName)
		if _, err := app.Append(0, countLset, ts, float64(dp.GetCount())); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (tr *Translator) accumulateDelta(lset labels.Labels, delta float64) float64 {
	fp := lset.Hash()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.evictStale()
	entry, ok := tr.delta[fp]
	if !ok {
		tr.delta[fp] = &deltaEntry{value: delta, lastSeen: time.Now()}
		return delta
	}
	entry.value += delta
	entry.lastSeen = time.Now()
	return entry.value
}

func (tr *Translator) evictStale() {
	cutoff := time.Now().Add(-deltaTTL)
	for fp, e := range tr.delta {
		if e.lastSeen.Before(cutoff) {
			delete(tr.delta, fp)
		}
	}
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

func (tr *Translator) buildLabels(metricName string, attrs []*commonpb.KeyValue, resourceLabels, scopeLabels, schemaLabels labels.Labels) labels.Labels {
	b := labels.NewBuilder(labels.EmptyLabels())
	resourceLabels.Range(func(l labels.Label) {
		b.Set(l.Name, l.Value)
	})
	scopeLabels.Range(func(l labels.Label) {
		b.Set(l.Name, l.Value)
	})
	schemaLabels.Range(func(l labels.Label) {
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

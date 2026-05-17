package ingest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bobcob7/lil-olt-metrics/internal/sessions"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// LogsTranslator converts OTLP log records into sessions.Event records,
// applying the configured content-capture privacy gate.
type LogsTranslator struct {
	logger         *slog.Logger
	captureContent bool
}

// NewLogsTranslator returns a LogsTranslator with the given logger and content-capture toggle.
func NewLogsTranslator(logger *slog.Logger, captureContent bool) *LogsTranslator {
	return &LogsTranslator{
		logger:         logger,
		captureContent: captureContent,
	}
}

// Translate walks an OTLP ExportLogsServiceRequest and emits sessions.Event records.
// Records missing a session id (in either resource or record attributes) are dropped.
func (t *LogsTranslator) Translate(req *collogspb.ExportLogsServiceRequest) ([]sessions.Event, error) {
	if req == nil {
		return nil, nil
	}
	var events []sessions.Event
	for _, rl := range req.GetResourceLogs() {
		resAttrs := attrMapToStrings(rl.GetResource().GetAttributes())
		for _, sl := range rl.GetScopeLogs() {
			for _, rec := range sl.GetLogRecords() {
				evt, ok := t.recordToEvent(rec, resAttrs)
				if !ok {
					continue
				}
				events = append(events, evt)
			}
		}
	}
	return events, nil
}

func (t *LogsTranslator) recordToEvent(rec *logspb.LogRecord, resAttrs map[string]string) (sessions.Event, bool) {
	recAttrs := attrMapToStrings(rec.GetAttributes())
	sessionID := firstNonEmpty(recAttrs["session.id"], resAttrs["session.id"])
	bodyJSON := parseBodyJSON(rec.GetBody())
	if sessionID == "" {
		if v, ok := bodyJSON["session_id"].(string); ok {
			sessionID = v
		}
	}
	if sessionID == "" {
		t.logger.Debug("logs translator: dropping record without session.id")
		return sessions.Event{}, false
	}
	name := firstNonEmpty(recAttrs["event.name"], rec.GetEventName())
	if name == "" {
		if v, ok := bodyJSON["event_name"].(string); ok {
			name = v
		}
	}
	attrs := make(map[string]string, len(recAttrs))
	for k, v := range recAttrs {
		attrs[k] = v
	}
	for _, k := range []string{"user.id", "host.name", "os.type"} {
		if v, ok := resAttrs[k]; ok && v != "" {
			if _, exists := attrs[k]; !exists {
				attrs[k] = v
			}
		}
	}
	evt := sessions.Event{
		SessionID: sessionID,
		Name:      name,
		ToolName:  recAttrs["tool_name"],
		Model:     recAttrs["model"],
		Timestamp: recordTimestamp(rec),
		Attrs:     attrs,
	}
	if t.captureContent {
		evt.Body = attrToString(rec.GetBody())
	}
	return evt, true
}

func recordTimestamp(rec *logspb.LogRecord) time.Time {
	if ts := rec.GetTimeUnixNano(); ts != 0 {
		return time.Unix(0, int64(ts))
	}
	if ts := rec.GetObservedTimeUnixNano(); ts != 0 {
		return time.Unix(0, int64(ts))
	}
	return time.Now()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// attrMapToStrings flattens a slice of KeyValue attributes into a string map.
func attrMapToStrings(attrs []*commonpb.KeyValue) map[string]string {
	if len(attrs) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		if kv == nil || kv.GetKey() == "" {
			continue
		}
		out[kv.GetKey()] = attrToString(kv.GetValue())
	}
	return out
}

// attrToString coerces an OTLP AnyValue into a string representation suitable
// for the flat sessions.Event Attrs map. Complex values are JSON-encoded.
func attrToString(v *commonpb.AnyValue) string {
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
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(v.GetBytesValue())
	case *commonpb.AnyValue_ArrayValue:
		arr := v.GetArrayValue().GetValues()
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, anyValueToJSON(item))
		}
		b, err := json.Marshal(out)
		if err != nil {
			return ""
		}
		return string(b)
	case *commonpb.AnyValue_KvlistValue:
		m := kvListToMap(v.GetKvlistValue().GetValues())
		b, err := json.Marshal(m)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return ""
	}
}

func anyValueToJSON(v *commonpb.AnyValue) any {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return v.GetStringValue()
	case *commonpb.AnyValue_BoolValue:
		return v.GetBoolValue()
	case *commonpb.AnyValue_IntValue:
		return v.GetIntValue()
	case *commonpb.AnyValue_DoubleValue:
		return v.GetDoubleValue()
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(v.GetBytesValue())
	case *commonpb.AnyValue_ArrayValue:
		arr := v.GetArrayValue().GetValues()
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, anyValueToJSON(item))
		}
		return out
	case *commonpb.AnyValue_KvlistValue:
		return kvListToMap(v.GetKvlistValue().GetValues())
	default:
		return nil
	}
}

func kvListToMap(kvs []*commonpb.KeyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		if kv == nil || kv.GetKey() == "" {
			continue
		}
		out[kv.GetKey()] = anyValueToJSON(kv.GetValue())
	}
	return out
}

// parseBodyJSON returns the body as a JSON object when it is a string value
// containing a JSON object (used by the Claude Code variant), or a map directly
// from a KvlistValue body. Returns nil for any other shape.
func parseBodyJSON(body *commonpb.AnyValue) map[string]any {
	if body == nil {
		return nil
	}
	switch body.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		s := body.GetStringValue()
		if len(s) == 0 || s[0] != '{' {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil
		}
		return m
	case *commonpb.AnyValue_KvlistValue:
		return kvListToMap(body.GetKvlistValue().GetValues())
	default:
		return nil
	}
}

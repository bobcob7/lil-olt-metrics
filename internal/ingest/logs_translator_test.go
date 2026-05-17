package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestLogsTranslator_UserPromptCaptureOff(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	ts := uint64(1_700_000_000_000_000_000)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					strAttr("session.id", "sess-1"),
					strAttr("user.id", "alice"),
				},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					TimeUnixNano: ts,
					Attributes: []*commonpb.KeyValue{
						strAttr("event.name", "claude_code.user_prompt"),
					},
					Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "private prompt content"}},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	evt := events[0]
	assert.Equal(t, "sess-1", evt.SessionID)
	assert.Equal(t, "claude_code.user_prompt", evt.Name)
	assert.Empty(t, evt.Body)
	assert.Equal(t, time.Unix(0, int64(ts)), evt.Timestamp)
	assert.Equal(t, "alice", evt.Attrs["user.id"])
}

func TestLogsTranslator_UserPromptCaptureOn(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), true)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-2")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					Attributes: []*commonpb.KeyValue{strAttr("event.name", "claude_code.user_prompt")},
					Body:       &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello world"}},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "hello world", events[0].Body)
}

func TestLogsTranslator_ToolResultPromotesToolName(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-3")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					Attributes: []*commonpb.KeyValue{
						strAttr("event.name", "claude_code.tool_result"),
						strAttr("tool_name", "Bash"),
					},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "claude_code.tool_result", events[0].Name)
	assert.Equal(t, "Bash", events[0].ToolName)
	assert.Equal(t, "Bash", events[0].Attrs["tool_name"])
}

func TestLogsTranslator_APIRequestPromotesModel(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-4")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					Attributes: []*commonpb.KeyValue{
						strAttr("event.name", "claude_code.api_request"),
						strAttr("model", "claude-opus-4-7"),
					},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "claude-opus-4-7", events[0].Model)
}

func TestLogsTranslator_DropsRecordsWithoutSessionID(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("user.id", "anon")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					Attributes: []*commonpb.KeyValue{strAttr("event.name", "claude_code.user_prompt")},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestLogsTranslator_MultipleResourceScopeRecords(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-A")},
				},
				ScopeLogs: []*logspb.ScopeLogs{{
					LogRecords: []*logspb.LogRecord{
						{Attributes: []*commonpb.KeyValue{strAttr("event.name", "claude_code.user_prompt")}},
						{Attributes: []*commonpb.KeyValue{strAttr("event.name", "claude_code.tool_result"), strAttr("tool_name", "Read")}},
					},
				}},
			},
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-B")},
				},
				ScopeLogs: []*logspb.ScopeLogs{{
					LogRecords: []*logspb.LogRecord{
						{Attributes: []*commonpb.KeyValue{strAttr("event.name", "claude_code.api_request"), strAttr("model", "claude-sonnet-4-6")}},
					},
				}},
			},
		},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, "sess-A", events[0].SessionID)
	assert.Equal(t, "claude_code.user_prompt", events[0].Name)
	assert.Equal(t, "sess-A", events[1].SessionID)
	assert.Equal(t, "Read", events[1].ToolName)
	assert.Equal(t, "sess-B", events[2].SessionID)
	assert.Equal(t, "claude-sonnet-4-6", events[2].Model)
}

func TestLogsTranslator_FallsBackToObservedTimestamp(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), false)
	observed := uint64(1_650_000_000_000_000_000)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-5")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					ObservedTimeUnixNano: observed,
					Attributes:           []*commonpb.KeyValue{strAttr("event.name", "claude_code.tool_decision")},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, time.Unix(0, int64(observed)), events[0].Timestamp)
}

func TestLogsTranslator_BodyEventNameFallback(t *testing.T) {
	t.Parallel()
	tr := NewLogsTranslator(testLogger(), true)
	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{strAttr("session.id", "sess-6")},
			},
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: `{"event_name":"claude_code.user_prompt","prompt":"hi"}`}},
				}},
			}},
		}},
	}
	events, err := tr.Translate(req)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "claude_code.user_prompt", events[0].Name)
}

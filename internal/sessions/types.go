package sessions

import "time"

// Session captures rolling metadata for a Claude Code session as derived from
// the events emitted by the client.
type Session struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id,omitempty"`
	Model         string    `json:"model,omitempty"`
	CWD           string    `json:"cwd,omitempty"`
	LastEventName string    `json:"last_event_name,omitempty"`
	LastToolName  string    `json:"last_tool_name,omitempty"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	EventCount    int       `json:"event_count"`
}

// Event is a single Claude Code OTLP log record translated into the shape the
// dashboard needs.
type Event struct {
	SessionID string            `json:"session_id"`
	Name      string            `json:"name"`
	ToolName  string            `json:"tool_name,omitempty"`
	Model     string            `json:"model,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Attrs     map[string]string `json:"attrs,omitempty"`
	Body      string            `json:"body,omitempty"`
}

# Plan 19 — Live Sessions UI

## Summary

Add a "Live Sessions" collapsible section to the dashboard. Polls `/api/v1/sessions?since=5m` every 5 seconds, renders one card per active session showing the last event/tool/cwd/model and last-seen age, and lets the user expand a card to see a timeline of recent events fetched from `/api/v1/sessions/{id}/events`.

## Dependencies

- **Plan 18** (sessions HTTP API) — the endpoints must exist

## Scope

### In Scope

- `web/src/api/sessions.ts`:
  ```ts
  export interface Session {
    id: string;
    user_id?: string;
    model?: string;
    cwd?: string;
    last_event_name?: string;
    last_tool_name?: string;
    first_seen: string; // RFC3339
    last_seen: string;
    event_count: number;
  }
  export interface SessionEvent {
    session_id: string;
    name: string;
    tool_name?: string;
    model?: string;
    timestamp: string;
    attrs?: Record<string, string>;
    body?: string;
  }
  export const fetchSessions = async (sinceSeconds: number): Promise<readonly Session[]> => { /* fetch + parse */ };
  export const fetchSessionEvents = async (id: string, limit: number): Promise<readonly SessionEvent[]> => { /* fetch + parse */ };
  ```
  - On 404 from `/api/v1/sessions` (i.e. logs feature disabled on the server), `fetchSessions` returns `[]` rather than throwing — this lets the UI gracefully render an empty state
- `web/src/composables/useLiveSessions.ts`:
  - Exports `useLiveSessions(intervalMs = 5000)` returning `{ sessions: Ref<readonly Session[]>, error: Ref<string|null>, lastUpdated: Ref<Date|null>, refresh: () => Promise<void> }`
  - Owns the `setInterval` lifecycle via `onMounted` / `onUnmounted`
  - Polls `fetchSessions(300)` (5-minute since window)
- `web/src/components/LiveSessions/LiveSessions.vue`:
  - `<script setup lang="ts">` — calls `useLiveSessions()`; named export only (no default)
  - Renders `<div class="grid">` of `SessionCard` per session, sorted by `last_seen` desc
  - Empty state: "No active sessions in the last 5 minutes. Send Claude Code OTLP logs to this server to see them here."
  - Error state: red banner with retry button
- `web/src/components/LiveSessions/SessionCard.vue`:
  - Props: `session: Session`
  - Shows: short id (last 6 chars), last-event name + tool name + model, basename of cwd, "X seconds/minutes ago" badge derived from `last_seen` using existing time helpers
  - Click toggles `expanded` ref; when expanded, mounts `<SessionEventList :session-id="session.id" />` below the card
- `web/src/components/LiveSessions/SessionEventList.vue`:
  - Props: `sessionId: string`
  - On mount, calls `fetchSessionEvents(sessionId, 50)`
  - Renders a vertical timeline; each event row shows timestamp, name, tool name (if any), and (if `body` present) a `<details><summary>body</summary><pre>{{body}}</pre></details>`
- `web/src/App.vue` integration:
  - Import `LiveSessions` and add a new `<CollapsibleSection title="Live Sessions" :open="true">` immediately after `DashboardHeader` (before `SummaryCards` so it's the most-prominent block — this is the user's main use case)
- `web/src/__tests__/LiveSessions.test.ts`:
  - Vitest + RTL — mock `fetch` to return canned `sessions` payloads
  - Cases:
    * Empty list renders empty-state message
    * Two sessions render two cards in correct order (newest last_seen first)
    * Clicking a card calls `fetchSessionEvents` and renders the returned events
    * 404 on `/api/v1/sessions` renders empty state, not an error
    * Polling: fast-forward `vi.useFakeTimers()` 5 seconds → fetch called a second time
- Styling: follow existing CSS Modules pattern (`.module.css` colocated with component). Reuse `--text`, `--accent`, `--card-bg`, etc. from existing palette
- Type safety: `strict: true` and `noUncheckedIndexedAccess: true` already enabled — handle the index-access narrowing for `parts[0]` style code

### Out of Scope

- WebSocket push (polling is fine; the spec said v1)
- Per-event filtering or search
- Re-using the existing `useDashboardData` polling cadence — sessions need their own faster cadence

## Acceptance Criteria

1. `cd web && yarn build` succeeds with no TypeScript errors
2. `cd web && yarn test` passes including the five new test cases
3. `cd web && yarn lint` clean
4. Manual: with the server running and a Claude Code session emitting OTLP logs, opening the dashboard shows a "Live Sessions" section listing the session, updating every 5s
5. With the server running but `Logs.Enabled=false`, the section renders the empty state (no console errors)
6. Click-to-expand on a card fetches and renders that session's recent events; body content is empty unless server has `CaptureContent=true`

## Key Snippets

```vue
<!-- LiveSessions.vue -->
<script setup lang="ts">
import { useLiveSessions } from '../../composables/useLiveSessions';
import SessionCard from './SessionCard.vue';
const { sessions, error } = useLiveSessions();
</script>

<template>
  <div v-if="error" :class="$style.error">{{ error }}</div>
  <div v-else-if="sessions.length === 0" :class="$style.empty">
    No active sessions in the last 5 minutes.
  </div>
  <div v-else :class="$style.grid">
    <SessionCard v-for="s in sessions" :key="s.id" :session="s" />
  </div>
</template>
```

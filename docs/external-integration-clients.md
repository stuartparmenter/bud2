---
topic: External Integration Clients
repo: bud2
generated_at: 2026-04-06T00:00:00Z
commit: 893eab92
key_modules: [internal/integrations, internal/senses]
score: 0.56
---

# External Integration Clients

> Repo: `bud2` | Generated: 2026-04-06 | Commit: 893eab92

## Summary

This subsystem is Bud's boundary layer with the external world: `internal/integrations/` holds raw API clients (Google Calendar, GitHub) that handle auth and wire protocol, while `internal/senses/` wraps those clients as percept-producing observers that feed the attention system. The split keeps protocol-level concerns (JWT signing, GraphQL pagination) separate from behavioral concerns (when to fire a meeting reminder, how to deduplicate notifications across restarts).

## Key Data Structures

### `calendar.Client` (`internal/integrations/calendar/client.go`)
HTTP client for the Google Calendar API. Holds a slice of `calendarIDs` (supports multiple calendars), parsed service account credentials, and a cached OAuth2 `accessToken` with expiry tracked under `sync.RWMutex`. Token refresh uses the double-checked lock pattern: read-lock checks expiry; if expired, re-check after acquiring write-lock before issuing a new JWT. Credentials can come from a file path or base64-encoded JSON in `BUD_CALENDAR_CREDENTIALS_JSON`/`BUD_CALENDAR_CREDENTIALS_FILE`.

### `calendar.Event` (`internal/integrations/calendar/client.go`)
Normalized event type. `CalendarID` tracks which of the multi-calendar sources this event belongs to. `AllDay bool` controls whether `Start`/`End` are treated as date-only. `MeetLink` is extracted from `conferenceData.entryPoints`. `Metadata map[string]string` carries `extendedProperties.private` key-value pairs for arbitrary enrichment.

### `calendar.ListEventsParams` (`internal/integrations/calendar/client.go`)
Query parameters for `ListEvents`. `SingleEvents bool` (default true) expands recurring events into individual instances — the client always sets this.

### `CalendarSense` (`internal/senses/calendar.go`)
The polling observer. Key fields: `notifiedEvents map[string]time.Time` (dedup by event ID), `notifiedBriefs map[string]time.Time` (dedup sprint briefs by date string), `lastDailyAgenda`/`lastPredictionReview` (dedup daily impulses). All state persists to `statePath` as JSON to survive daemon restarts. `onMessage func(*memory.InboxMessage)` is the injection point to the executive's inbox.

### `calendarState` (`internal/senses/calendar.go`)
Serialization struct for `CalendarSense` state. Contains the four dedup maps. Loaded at startup via `Load()`, written after each notification via `Save()`.

### `github.Client` (`internal/integrations/github/client.go`)
GitHub GraphQL client scoped to one organization. Holds a PAT `token` and an `*http.Client`. All queries go through `graphqlRequest`, which inlines error checking for GraphQL-level errors (distinct from HTTP-level errors).

### `github.ProjectItem` (`internal/integrations/github/client.go`)
Unified representation of ISSUE, PULL_REQUEST, or DRAFT_ISSUE. `FieldValues map[string]string` flattens all project custom fields (Status, Sprint, Priority, Team/Area) into a single map for easy filtering.

### `DiscordSense` (`internal/senses/discord.go`)
WebSocket-based observer backed by `discordgo`. Tracks connection health (`connected`, `disconnectCount`, `lastDisconnected`) and manages a `pendingInteractions map[string]*PendingInteraction` for slash command followups. `onMessage`, `onStop`, and `onDebugExecutive` are three separate callbacks injected at construction.

### `PendingInteraction` (`internal/senses/discord.go`)
Stores a Discord interaction token and `AppID` for followup response. Tokens expire after 15 minutes; `GetPendingInteraction` enforces this and removes the entry on retrieval (one-time use).

## Lifecycle

### Calendar polling

1. **Start**: `CalendarSense.Start()` calls `Load()` to restore persisted state, then spawns `pollLoop()` as a goroutine.
2. **Immediate poll**: `pollLoop` fires `poll()` once before entering the 5-minute ticker loop, so the first check happens at daemon startup without waiting.
3. **poll()**: Sequentially runs four checks — `checkDailyAgenda`, `checkPredictionReview`, `checkUpcomingMeetings`, `checkSprintBrief`. Each check independently fetches events from `calendar.Client`.
4. **checkDailyAgenda**: Fires between 07:00–09:00 in user's timezone, once per calendar day. Calls `GetTodayEvents`, filters to confirmed non-cancelled events, formats a text summary, creates an `InboxMessage` with source `calendar`, and calls `onMessage` directly (no queue).
5. **checkUpcomingMeetings**: Looks ahead by `reminderBefore + pollInterval` (default 20 min) to account for poll jitter. Skips cancelled, all-day, and not-accepted events. Deduplicates via `notifiedEvents`; also stamps a deterministic `inbox_id` on the message for cross-restart dedup inside `internal/memory`.
6. **checkSprintBrief**: Delegates to `checkSprintCluster` twice (once for "sprint review", once for "sprint planning"). Clusters events by local date; fires the impulse if 2+ matching events exist and the earliest starts within `sprintBriefBefore` (default 45 min).
7. **cleanupNotifications**: Removes entries from `notifiedEvents` older than 24 hours to bound map growth.
8. **Save**: Called after any notification is sent. Serializes the four dedup maps to `statePath`.
9. **Stop**: Closes `stopChan`, causing `pollLoop` to exit at next tick.

### GitHub query

1. **NewClient**: Reads `GITHUB_TOKEN` and `GITHUB_ORG` environment variables.
2. **QueryItems**: Paginates backwards through project items using GraphQL `last/before` cursor (newest-first, likely ordered by insertion time). Fetches in batches of 100 up to 500 items when any filter is set; fetches 100 without pagination when unfiltered.
3. **Filtering**: Applied client-side after fetching. Status, Sprint, TeamArea, Priority are matched case-insensitively against `FieldValues`. Sprint accepts a special `"current"` value (matches the lexicographically latest sprint) and `"backlog"` (no sprint assigned).

### Discord event handling

1. **Start**: Creates a `discordgo.Session`, registers WebSocket handlers (`handleMessage`, `handleConnect`, `handleDisconnect`, `handleResumed`, `handleInteraction`), opens the connection, and starts `StartHealthMonitor()`.
2. **handleMessage**: Filters by channel and self; extracts reply chain and attachment info; calls `onMessage` directly.
3. **handleInteraction**: Handles slash commands. Owner check is enforced here. `/stop` calls `onStop` synchronously before acking. Other commands send a deferred ack immediately (within Discord's 3-second window), store a `PendingInteraction`, then call `onMessage`. The effector later retrieves the pending interaction via `GetPendingInteraction` to send the followup edit.
4. **Health monitor**: Goroutine wakes every minute and calls `checkConnectionHealth`. If `DisconnectedDuration()` exceeds `MaxDisconnectDuration` (default 10 min), fires `onProlongedOutage` callback then calls `HardReset()`.
5. **HardReset**: Closes the existing `discordgo.Session` and creates a new one from scratch, re-registering all handlers. Increments `hardResetCount`.

## Design Decisions

- **Multi-calendar deduplication in two layers**: `ListEvents` deduplicates by event ID (same event returned by multiple calendar IDs) and by title+start time (same event shared across calendars gets different IDs). This handles the common case where a personal and a work calendar both have an invite.
- **Token refresh with double-checked locking**: The calendar client acquires a read lock to check token expiry, then re-acquires a write lock and rechecks before refreshing. This avoids a thundering-herd on token refresh if multiple goroutines request a token simultaneously.
- **tokenExpiry = 55 minutes**: Google access tokens last 60 minutes; the client refreshes 5 minutes early to ensure tokens don't expire mid-request during slow operations.
- **GraphQL-only GitHub client**: The client uses only the GitHub GraphQL API (not REST), because GitHub Projects v2 fields (Sprint, Status, etc.) are not accessible via the REST API.
- **Client-side filtering in QueryItems**: GitHub's GraphQL API does not support server-side filtering on custom project fields, so the client fetches up to 500 items and filters in Go. The 500-item cap prevents unbounded API usage on large projects.
- **Callback injection over channels**: Both `CalendarSense` and `DiscordSense` accept an `onMessage func(*memory.InboxMessage)` at construction time rather than publishing to a channel. This makes the caller (main.go) responsible for the routing decision and keeps senses free of queue implementation details.
- **Pending interactions map**: Discord slash commands require an initial response within 3 seconds and allow followup edits for 15 minutes. The `pendingInteractions` map bridges the gap between the synchronous ack and the asynchronous Claude response. The effector (not the sense) is responsible for calling `GetPendingInteraction` when sending a reply.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `internal/senses/calendar.go` | `internal/integrations/calendar` | Creates a `*calendar.Client` and calls `GetTodayEvents`, `GetUpcomingEvents`, `FreeBusy`, `ListEvents` |
| `internal/senses/calendar.go` | `internal/memory` | Creates `*memory.InboxMessage` values and delivers them via `onMessage` callback |
| `internal/senses/discord.go` | `github.com/bwmarrin/discordgo` | Manages the WebSocket session; all Discord events arrive as `discordgo` struct callbacks |
| `internal/senses/discord.go` | `internal/memory` | Creates `*memory.InboxMessage` values; `ExtraData` map carries Discord-specific metadata (channel ID, interaction token, reply chain) |
| `internal/mcp/tools/` | `internal/integrations/calendar` | Calendar MCP tools (`calendar_list_events`, `calendar_get_event`, etc.) instantiate a `calendar.Client` and call its methods directly |
| `internal/mcp/tools/` | `internal/integrations/github` | GitHub MCP tools (`github_list_projects`, `github_project_items`, etc.) instantiate a `github.Client` and call `ListProjects`, `QueryItems`, etc. |
| `cmd/bud/main.go` | `internal/senses` | Constructs `DiscordSense` and `CalendarSense`, wires `onMessage` to the executive's `processPercept`, calls `Start()` and deferred `Stop()` |

## Non-Obvious Behaviors

- **All-day event timezone drift**: `GetTodayEvents` adds a post-filter that compares the event's raw `date` string against the local date string. Google Calendar returns all-day events with UTC midnight timestamps; converting to a non-UTC timezone can shift the date by ±1 day. The raw string comparison bypasses this drift.
- **Sprint "current" uses lexicographic max**: `QueryItems` with `Sprint: "current"` selects the sprint with the lexicographically highest name. This works for sprint names like "Sprint 65", "Sprint 66" but would break if sprint names were not zero-padded or followed a different pattern.
- **Meeting reminder window is `reminderBefore + pollInterval`**: To avoid missing a meeting that starts between polls, `checkUpcomingMeetings` looks ahead by 20 minutes (15 + 5), not just 15. An event whose start is 16 minutes away will be caught in the current poll.
- **DiscordSense shares its session with the effector**: `Session()` exposes the `*discordgo.Session` so `internal/effectors/discord.go` can reuse the same WebSocket connection for sending. After a `HardReset`, the effector must re-acquire the session via `Session()` or it will be holding a stale reference.
- **Slash command `/stop` bypasses the inbox**: `handleInteraction` calls `onStop` synchronously before routing to `onMessage`. This means the stop signal reaches the executive via a direct callback, not through the normal percept/focus pipeline — it's designed to interrupt a running session immediately.
- **`notifiedEvents` dedup vs. inbox dedup**: `CalendarSense` deduplicates meeting reminders in two independent ways: the `notifiedEvents` in-memory map (fast, lost on restart) and a deterministic `inbox_id` on the `InboxMessage` (slower, survives restarts via `internal/memory`). Both must be clear for a duplicate reminder to be sent.

## Start Here

- `internal/integrations/calendar/client.go` — full calendar API client; start here to understand auth (JWT service account), multi-calendar support, and the event model
- `internal/integrations/github/client.go` — GitHub GraphQL client; read `QueryItems` to understand the pagination and client-side filtering model
- `internal/senses/calendar.go` — `poll()` and the four `check*` functions define all the behavioral triggers; `CalendarSense` struct shows what state survives across polls
- `internal/senses/discord.go` — `handleMessage` and `handleInteraction` show how Discord input becomes an `InboxMessage`; `handleInteraction` has the slash command routing logic
- `cmd/bud/main.go` — wiring point: shows how senses are constructed, what callbacks are injected, and the startup/shutdown order

---
type: concept
title: "Codex Cache Continuity Mechanism"
createdAt: "2026-07-08T22:00:00Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, continuity]
see_also: ["adrs/0001-session-header-casing.adr.md", "concepts/0001-codex-session-header-architecture.concept.md", "adrs/0002-continuity-key-override.adr.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Concept: Codex Cache Continuity Mechanism

## What

Cache continuity is the mechanism by which CPA maintains a stable cache key across multiple conversation turns. Without continuity, each turn generates a different `prompt_cache_key`, and the upstream Codex API treats each turn as a new session — resulting in zero cache reuse between turns.

## Why

OpenCode and Claude Code make multiple API requests per conversation (user turn, tool calls, follow-ups). Without continuity, each request gets a different cache key, and the upstream cannot reuse cached responses. This increases latency and cost significantly for multi-turn conversations.

## Key Details

### Continuity Key Resolution (`resolveCodexContinuity`)

The function resolves one stable key per session using a 3-priority fallback chain:

**Priority 1: Client-supplied session headers**
- Scans headers: `X-Session-ID`, `Session-Id`, `Session_id`, `session_id`, `Conversation_id`, `conversation_id`
- If ANY of these headers exist, uses their value as the cache key
- ⚠️ **Risk:** If the client sends a *changing* session header (e.g., `session_id` that differs per request), Priority 1 actively breaks cache stability. The fix is in the OpenCode client, which now sends a stable `X-Session-Id`.

**Priority 2: Execution session metadata**
- Extracts from the request's execution context
- Used when the client SDK provides session metadata in the request body

**Priority 3: Auth hash (fallback)**
- `hash = uuid.NewSHA1("cli-proxy-api:codex:continuity:" + auth.ID)`
- Stable fallback when no client headers or metadata are provided
- Ensures at minimum auth-level cache stability

### Application

The resolved continuity key is applied in two ways:

1. **`applyCodexContinuityBody`** — Sets `prompt_cache_key` in the request body
2. **`applyCodexContinuityHeaders`** — Sets `Session-Id` header on the upstream request

### Clients That Benefit

| Client | Session Header | Continuity Source |
|--------|---------------|-------------------|
| OpenCode (patched) | `X-Session-Id` (stable) | Priority 1 (header) |
| OpenCode (Homebrew 1.14.29) | `session_id` (volatile) | ⚠️ Priority 1 wins → broken continuity |
| Claude Code | `X-Claude-Code-Session-Id` | Priority 2 (metadata) |
| Codex CLI | `session-id` (stable per session) | Priority 1 (header) |
| Direct API (no headers) | None | Priority 3 (auth hash) — stable |

## Post-sync confirmed mechanism (2026-08-25)

On the `v7.2.141` sync base the fork's own 3-priority `resolveCodexContinuity` chain is
**superseded** by upstream `f43aad76` (canonical `Session-Id` + preloads) + `cacheHelper`,
plus the fork-local override from **ADR-010**.

**Confirmed root cause:** `cacheHelper` (`codex_executor_request.go`) and
`applyCodexPromptCacheHeadersWithContext` (`codex_websockets_request.go`) treat a payload
`prompt_cache_key` as authoritative. opencode mints a random v4 UUID per request (43/43
unique keys, zero reuse in one conversation) — so the upstream cache partition changes
every turn. Cross-turn reuse is impossible.

**New resolve policy (config-gated):**

1. `codex.force-stable-prompt-cache-key` enabled **and** `ProviderSessionUUID("codex",
   req.Metadata)` resolves (i.e. `execution_session_id` present) → outbound
   `prompt_cache_key` = **derived stable key** (overrides client key), HTTP and WS.
2. No stable identity → client key preserved (stateless fallback).
3. Flag off → upstream behavior exactly (client key wins when present).

The client-supplied key is no longer forwarded verbatim when the flag is on and identity
exists. See [[adrs/0002-continuity-key-override]].

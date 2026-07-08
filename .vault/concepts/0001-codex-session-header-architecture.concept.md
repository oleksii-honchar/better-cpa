---
type: concept
title: "Codex Session Header Architecture"
createdAt: "2026-07-08T22:00:00Z"
updatedAt: "2026-07-08T22:00:00Z"
tags: [better-cpa, codex, caching, headers, architecture]
see_also: ["adrs/0001-session-header-casing.adr.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Concept: Codex Session Header Architecture

## What

The Codex session header architecture is the mechanism by which CPA (CliProxyAPI) forwards session identifiers to the upstream Codex API for prompt caching. Each outbound request carries a `Session-Id` header that the upstream uses to route cache lookups. If the session ID is stable across requests, the upstream can reuse cached responses — improving latency and reducing cost.

## Why

The upstream Codex API expects a `session-id` header (hyphen, lowercase) to associate requests with a session. Without a stable session identifier, each request appears as a new session, resulting in **zero cache hits**. This directly impacts latency for OpenCode and Claude Code users routing through CPA.

## Key Details

### Header Name Standardization

The official Codex CLI (`codex-rs/codex-api`) sends `session-id` (hyphen, lowercase). CPA was sending two wrong variants:
- **HTTP executor:** `Session_id` (underscore, capitalized)
- **WebSocket executor:** `session_id` (underscore, lowercase)

The upstream Codex API treats headers as distinct by name — these are different headers from `session-id`.

### Continuity Key Resolution Chain

CPA resolves a stable continuity key per session via `resolveCodexContinuity()`:

1. **Priority 1 — Client-supplied session headers:** Scans for `X-Session-ID`, `Session-Id`, `Session_id`, `session_id`, `Conversation_id`, `conversation_id` in the inbound request
2. **Priority 2 — Execution session metadata:** Extracts from request context (if provided by client SDK)
3. **Priority 3 — Auth hash:** Generates `uuid.NewSHA1("cli-proxy-api:codex:continuity:" + auth.ID)` as stable fallback

The resolved key is applied as:
- `prompt_cache_key` in the request body (via `applyCodexContinuityBody`)
- `Session-Id` header on the upstream request (via `applyCodexContinuityHeaders`)

### Affected Code Paths

| Path | Executor file | Function | Used by |
|------|--------------|----------|---------|
| HTTP | `codex_executor.go` | `applyCodexContinuityHeaders` | OpenCode, Codex CLI, LiteLLM models proxied through Codex executor |
| HTTP | `codex_executor.go` | `codexPromptCacheKeyFromHeader` | Continuity key resolution (Priority 1) |
| HTTP | `codex_executor.go` | `applyCodexIdentityConfuseHeaders` | Header normalization with session ID |
| HTTP | `codex_executor.go` | `applyCodexHeadersFromSources` | Header assembly for upstream request |
| WebSocket | `codex_websockets_executor.go` | `applyCodexPromptCacheHeaders` | Claude Code WebSocket path |
| WebSocket | `codex_websockets_executor.go` | `ensureCodexWebsocketSessionHeader` | WebSocket session header normalization |

### Out of Scope

The Claude executor (`claude_executor.go`) uses its own session header (`X-Claude-Code-Session-Id`) and has a separate TTL issue (Issue #3398). It is NOT part of the Codex session header architecture.

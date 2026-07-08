---
type: adr
id: ADR-009
title: "Standardize Session Header Casing — Use Session-Id (Hyphen)"
status: accepted
createdAt: "2026-07-08T21:15:00Z"
updatedAt: "2026-07-08T22:00:00Z"
tags: [better-cpa, codex, caching, headers]
supersedes: []
superseded_by: []
see_also: ["concepts/0001-codex-session-header-architecture.concept.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# ADR-009: Standardize Session Header Casing — Use `Session-Id` (Hyphen)

## Context

The Codex proxy cache continuity fix (PR #3141) introduced `applyCodexContinuityHeaders()` which sets `Session_id` (underscore, capitalized) on outgoing requests to the upstream Codex API. The WebSocket executor sets `session_id` (underscore, lowercase). However, the official Codex CLI (`codex-rs/codex-api/src/requests/headers.rs`) sends `session-id` (hyphen, lowercase):

```rust
insert_header(&mut headers, "session-id", &id);
```

The upstream Codex API treats headers as distinct by name — `session-id` and `Session_id` are different headers. CPA was sending the wrong header name, causing **zero cache hits**.

Additionally:
- `codexPromptCacheKeyFromHeader()` scans session headers to derive a cache key but did not include `Session-Id` in its scan list
- `ensureCodexWebsocketSessionHeader()` explicitly **deleted** the `Session-Id` header via `deleteHeaderCaseInsensitive(target, "Session-Id")` — actively making cache worse

### Affected Code Paths

| Path | Executor | Location | Issue |
|------|----------|----------|-------|
| HTTP (OpenCode/Codex CLI) | `codex_executor.go` | `applyCodexContinuityHeaders` (line 1592) | `Session_id` → should be `Session-Id` |
| HTTP (OpenCode/Codex CLI) | `codex_executor.go` | `codexPromptCacheKeyFromHeader` (line 1457) | Missing `Session-Id` in scan list |
| WebSocket (Claude Code) | `codex_websockets_executor.go` | `applyCodexPromptCacheHeaders` (line 888) | `session_id` → should be `Session-Id` |
| WebSocket (Claude Code) | `codex_websockets_executor.go` | `ensureCodexWebsocketSessionHeader` (line 971) | `session_id` → should be `Session-Id` |
| WebSocket (Claude Code) | `codex_websockets_executor.go` | `ensureCodexWebsocketSessionHeader` (line 973) | Actively deletes `Session-Id` header |

### Claude Executor

The Claude executor (`claude_executor.go`) was intentionally left alone — it has a separate TTL issue (Issue #3398) that does not affect Codex or LiteLLM models.

## Decision

**Standardize all outbound session headers to `Session-Id` (hyphen, mixed case)** to match the official Codex CLI behavior. Five changes across two files:

1. **`codex_executor.go:1457`** — Add `"Session-Id"` to `codexPromptCacheKeyFromHeader` scan list
2. **`codex_executor.go:1592`** — `Session_id` → `Session-Id` in `applyCodexContinuityHeaders`
3. **`codex_websockets_executor.go:888`** — `session_id` → `Session-Id` in `applyCodexPromptCacheHeaders`
4. **`codex_websockets_executor.go:971`** — `session_id` → `Session-Id` in `ensureCodexWebsocketSessionHeader`
5. **`codex_websockets_executor.go:973`** — Remove `deleteHeaderCaseInsensitive(target, "Session-Id")`

### Legacy Read Paths

Inbound parsing (`codexSessionHeaderValue` at `codex_websockets_executor.go:976`) was left unchanged — it already scans for `["Session-Id", "Session_id", "session_id"]`, ensuring backward compatibility with clients sending old header names.

## Alternatives Considered

1. **Keep underscore headers** (`Session_id` / `session_id`)
   - Pros: Fewer changes, backward compatibility with existing CPA clients
   - Cons: Breaks upstream cache — the Codex API ignores wrong header name

2. **Send both** (`Session_id` + `Session-Id`)
   - Pros: Graceful migration
   - Cons: Redundant headers, risk of confusion, WebSocket executor actively deletes `Session-Id`

3. **Send with `X-` prefix** (`X-Session-Id`)
   - Pros: Common proxy pattern
   - Cons: Official Codex CLI does NOT use `X-` prefix — mismatches upstream

4. **Do nothing (leave as-is)**
   - Cons: Zero cache hits confirmed by Keeper dashboard

## Consequences

- **Positive:** Upstream Codex API receives correctly-named session header. Cache hits become possible for OpenCode and Codex CLI traffic through CPA.
- **Positive:** Both HTTP and WebSocket paths now use consistent `Session-Id` header name.
- **Positive:** `codexPromptCacheKeyFromHeader` now recognizes `Session-Id` from official Codex CLI clients.
- **Negative:** Existing CPA clients parsing outbound `Session_id` headers need to also read `Session-Id`.
- **Neutral:** The `deleteHeaderCaseInsensitive` removal at line 973 was originally added by upstream author `sususu98` (commit `603a08fc`) as normalization to `session_id`. Since standardizing on `Session-Id`, the deletion is no longer correct. Verified: no separate history.
- **Tests:** 18 assertions in `codex_websockets_executor_test.go` updated; new `TestApplyCodexContinuityHeaders` in `codex_executor_test.go` added.
- **Additional fixes discovered:** `applyCodexIdentityConfuseHeaders` (line 1685) and `applyCodexHeadersFromSources` (line 1799) also used wrong casing — fixed as bonus.

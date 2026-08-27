---
type: specification
title: "Codex Prompt Cache Remediation (rev. 4 — scoped to cache-hit fix)"
kind: feature
status: active
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, fork-local]
owner: ""
target: ""
see_also: ["adrs/0002-continuity-key-override.adr.md", "adrs/0003-sync-upstream-v7-2-141.adr.md", "concepts/0002-codex-cache-continuity-mechanism.concept.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Specification: Codex Prompt Cache Remediation

## Goal

Fix the confirmed cache-hit mechanism: on `/v1/responses` (openai-response) paths,
override a client-supplied per-request `prompt_cache_key` with the stable per-session key
derived from `execution_session_id` metadata (via `helps.ProviderSessionUUID`), gated by
`codex.force-stable-prompt-cache-key` (default `true` at cutover) for staged rollout and
instant rollback. Verify manually via CPA response logs (`request-log: true`).

Confirmed root cause: the proxy forwards the opencode client's per-request random v4 UUID
`prompt_cache_key` verbatim (43/43 unique keys, zero reuse) — upstream v7.2.141 provides no
override mechanism. The stable-key machinery already exists but only fires when the client
sends no key.

## Scope

- In scope: HTTP `cacheHelper` + WS `applyCodexPromptCacheHeadersWithContext`
  openai-response branches; config key + example; trip-wire/wire test updates.
- **Dropped per human direction (2026-08-25):** plan Tasks 6–11 — telemetry adoption,
  catalog guard, offline reports, comparison harness, correlation analysis, GPT-5.6
  canary. No change to GPT-5.6 request semantics or `prompt_cache_retention`.

## Phases

### Phase 1 — Continuity key override adapter (DONE in code, uncommitted)

- Config key `codex.force-stable-prompt-cache-key` (default `true`) + compat drift test.
- Resolve policy in `cacheHelper` HTTP openai-response branch.
- Mirror in WS `applyCodexPromptCacheHeadersWithContext` (adds `cfg *config.Config`).
- Ported tripwire tests updated; new wire tests (random-key+identity → derived, same
  affinity → same key, flag-off preservation). Full executor suite 954 PASS / 0 FAIL.
- Build verified `v7.2.141-5-g274c2619-dirty`; old image preserved. **Deploy human-gated.**

### Phase 2 — Manual verification (acceptance, human-gated)

- Confirm deployed version string is the new build (not `13123d7c`).
- Multi-turn conversation in opencode: outbound `prompt_cache_key` stable across turns in
  same `X-Session-Affinity` conversation.
- Warm turns report `cached_tokens > 0`, first turn `cache_write_tokens > 0`.
- No `Session_id`/`session_id` churn (canonical `Session-Id` preserved).
- If hits do not appear, flip flag off and compare — isolates the override from GPT-5.6
  implicit semantics.

### Phase 3 — Optional no-key path hardening (codex-tui)

Only if manual test shows no improvement on that path: populate `execution_session_id` so
`ProviderSessionUUID` derives a stable key. Small; skippable.

## Behaviors

- Flag on + `execution_session_id` present → outbound `prompt_cache_key` = derived stable
  key (HTTP and WS).
- Flag on + no identity → client key preserved (stateless fallback).
- Flag off → upstream behavior exactly (client key wins when present).

## Risks

- Trip tests flip red — updated in same commit; wire harness proves behavior first.
- Client with intentional custom key overridden — derived key is conversation-scoped;
  flag-off preserves legacy; documented.
- `execution_session_id` absent (resumed sessions) — fall back to client key; no regression.
- Manual verification confounded by GPT-5.6 implicit semantics — flag on/off A/B isolates
  the override's contribution.

## Milestones

- 2026-08-25: Tasks 1–5 implemented + verified; deploy and manual verification pending.
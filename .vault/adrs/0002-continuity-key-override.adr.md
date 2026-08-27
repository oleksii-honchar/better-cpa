---
type: adr
id: ADR-010
title: "Override client-supplied prompt_cache_key with a stable per-session key (force-stable-prompt-cache-key)"
status: accepted
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, continuity, fork-local]
supersedes: []
superseded_by: []
see_also: ["adrs/0001-session-header-casing.adr.md", "concepts/0002-codex-cache-continuity-mechanism.concept.md", "specifications/0001-codex-prompt-cache-remediation.spec.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# ADR-010: Override client-supplied `prompt_cache_key` with a stable per-session key

## Context

Post-sync capture on the deployed `v7.2.141-4-g13123d7c` disproved the assumption that
upstream `f43aad76` + `cacheHelper` restores cache continuity for key-sending clients:
43 captured `/v1/responses` requests from opencode carried 43 unique `prompt_cache_key`
values with zero reuse inside the same `X-Session-Affinity` conversation
(`materials/evidence-prompt-cache-key-continuity.md`).

Code ground truth on the sync base:
- `cacheHelper` openai-response branch (`codex_executor_request.go`) treats a payload
  `prompt_cache_key` as authoritative; the stable-key fallback
  (`helps.ProviderSessionUUID` ← `execution_session_id` metadata) fires only when the
  client sends no key.
- `applyCodexPromptCacheHeadersWithContext` (WS, `codex_websockets_request.go`) applies
  the same payload-key-wins policy.
- opencode (openai-sdk provider-utils) mints a random v4 UUID per request and sends
  `previous_response_id: null` with the full transcript — so the proxy changes the
  upstream cache partition every turn.

## Decision

Add a fork-local, config-gated continuity override on branch `sync/upstream-v7.2.141`:
when `codex.force-stable-prompt-cache-key` is enabled (default `true` at cutover) and
`ProviderSessionUUID("codex", req.Metadata)` resolves (i.e. `execution_session_id` is
present), override any client-supplied `prompt_cache_key` with the stable derived key, in
both the HTTP `cacheHelper` and the WebSocket `applyCodexPromptCacheHeadersWithContext`
openai-response branches. When no stable identity exists, fall back to the client key
(today's behavior). Update the ported tripwire tests that assert client-key preservation;
add wire tests proving random-key → stable-key, same-affinity-group → same key, and
flag-off preservation.

Implementation status (2026-08-25): Tasks 1–5 done and verified on
`sync/upstream-v7.2.141` (HEAD `274c2619`, uncommitted); full suite 89 ok / 1 pre-existing
FAIL (Pion WebRTC, unrelated); build OK; version `v7.2.141-5-g274c2619-dirty`. **Deploy +
manual Phase 2 verification are human-gated.** Old image `better-cpa:local`
(`v7.2.49-7-gb50dba3c`) preserved for rollback.

## Alternatives Considered

1. **Stateful churn detection** (track keys per affinity group, switch to derived key on
   change) — precise but complex, loses the first request, needs state.
2. **User-Agent sniffing (opencode only)** — fragile; misses other key-sending clients.
3. **Never override** — leaves the confirmed zero-reuse mechanism unfixed.
4. **Always override, no gate** — simplest, but no rollback without rebuild and risks
   clients with intentional custom keys.

## Consequences

- **Positive:** opencode conversations get a stable cache partition; append-only turns can
  hit; the confirmed mechanism is removed.
- **Positive:** flag-off preserves upstream behavior exactly; rollback is a config flip or
  the preserved old binary.
- **Negative:** several ported tripwire tests flip and were updated in the same commit;
  fork-local divergence reintroduced on two executor files (must re-verify after future
  upstream syncs).
- **Negative:** clients with intentional custom keys are overridden when the flag is on
  (derived key is conversation-stable, so reuse still works; documented).
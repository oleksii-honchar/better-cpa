---
type: memory
title: "opencode sends a random v4 UUID prompt_cache_key per request"
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, opencode, gotcha]
see_also: ["concepts/0002-codex-cache-continuity-mechanism.concept.md", "adrs/0002-continuity-key-override.adr.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Memory: opencode sends a random v4 UUID `prompt_cache_key` per request

## Fact

opencode (openai-sdk provider-utils) mints a fresh random v4 UUID `prompt_cache_key` per
request and sends `previous_response_id: null` with the full transcript each turn. A proxy
that forwards the client's key verbatim therefore changes the upstream cache partition every
turn.

## Context

Observed in 43 captured `/v1/responses` response logs from `better-cpa:v7.2.141-4-g13123d7c`
(2026-08-25): 43 unique keys, zero reuse within the same `X-Session-Affinity` conversation.

## Impact

This is the confirmed root cause of the cache-hit drop. Any client-key-forwarding path
(`cacheHelper`, `applyCodexPromptCacheHeadersWithContext`) must override the key with a
stable per-session key when identity is present — implemented as ADR-010
(`codex.force-stable-prompt-cache-key`).
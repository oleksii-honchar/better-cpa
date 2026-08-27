---
type: runbook
title: "Verify Codex Prompt Cache Hit Fix (manual, post-deploy)"
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, operations]
see_also: ["specifications/0001-codex-prompt-cache-remediation.spec.md", "adrs/0002-continuity-key-override.adr.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Runbook: Verify Codex Prompt Cache Hit Fix (manual)

## Prerequisites

- Deployed new build on `cpa-codex` (image built from Tasks 1–5 commit; NOT `13123d7c`).
- Old image `better-cpa:local` (`v7.2.49-7-gb50dba3c`) preserved for rollback.
- CPA response logging enabled (`request-log: true`) with usage fields (`cached_tokens`,
  `cache_write_tokens`).

## Steps

1. Confirm deployed version string is the new build — binary startup line
   `CLIProxyAPI Version: ...` (expect `v7.2.141-5-g<commit>` after commit + rebuild; no
   `-dirty`).
2. Run a multi-turn conversation in opencode (affected client).
3. In CPA response logs for that conversation (`X-Session-Affinity`): confirm outbound
   `prompt_cache_key` is **stable across turns**.
4. Confirm warm turns report `cached_tokens > 0`; first turn `cache_write_tokens > 0`.
5. Confirm no `Session_id`/`session_id` churn — canonical `Session-Id` preserved.
6. If hits do not appear: flip `codex.force-stable-prompt-cache-key` to `false` and compare
   behavior — isolates the override from GPT-5.6 implicit semantics.

## Verification

- Stable key across ≥2 turns in same conversation.
- `cached_tokens > 0` on warm turns (compare `cached_tokens`, `cache_write_tokens`,
  latency, cost per GPT-5.6 implicit-cache guidance).

## Rollback

- Flip config key off (instant, no rebuild) or redeploy old image `better-cpa:local`.
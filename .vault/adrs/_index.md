---
type: index
title: "Architecture Decision Records"
createdAt: "2026-07-08T22:20:00Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: []
---

# Architecture Decision Records

Notable architecture decisions made for the better-cpa fork.

## Nodes

- [[adrs/0001-session-header-casing|ADR-009: Standardize Session Header Casing — Use Session-Id (Hyphen)]] — Accepted — Standardize all outbound session headers to `Session-Id` (hyphen) matching official Codex CLI. Both HTTP and WebSocket executors affected. (2026-07-08)

- [[adrs/0002-continuity-key-override|ADR-010: Override Client-Supplied prompt_cache_key with a Stable Per-Session Key]] — Accepted — Config-gated (`force-stable-prompt-cache-key`) override of the opencode client's random per-request `prompt_cache_key` with a stable derived key on HTTP + WS openai-response paths. Fixes the confirmed zero-reuse root cause. (2026-08-25)

- [[adrs/0003-sync-upstream-v7-2-141|ADR-011: Sync the Fork onto Upstream v7.2.141]] — Accepted — Executed sync of the 631-commit-behind fork onto upstream `v7.2.141`, carrying `.vault` knowledge via cherry-pick, dropping superseded fork code. Deployed `v7.2.141-4-g13123d7c`. (2026-08-25)

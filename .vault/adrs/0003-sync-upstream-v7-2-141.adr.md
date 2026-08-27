---
type: adr
id: ADR-011
title: "Sync the fork onto upstream v7.2.141 (dc3c3b1e) instead of rebuilding fork v7.2.49 in place"
status: accepted
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, sync, upstream, caching]
supersedes: []
superseded_by: []
see_also: ["adrs/0002-continuity-key-override.adr.md", "concepts/0001-codex-session-header-architecture.concept.md", "concepts/0002-codex-cache-continuity-mechanism.concept.md", "specifications/0001-codex-prompt-cache-remediation.spec.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# ADR-011: Sync the fork onto upstream v7.2.141

## Context

Human-directed architecture change (Option 2). The running CPA artifact was
`v7.2.49-7-gb50dba3c` (sends `Session_id`); fork HEAD `v7.2.49-9-g3b643a70` was
631 commits / ~92 releases behind upstream. Upstream independently shipped `f43aad76`
(2026-08-12) normalizing the session header to `Session-Id` and preloading codex headers
— the same direction as ADR-009, with a different implementation — plus a
cache/continuity/telemetry fix cluster landing exactly in the Aug 12–19 regression window
(`2ab25eae`, `6edf9c48`, `e0b49562`, `98c98d66`, `2005788f`, `baffbe2c`, `e05ae094`).
Fork-only commits were only `3b643a70` (`.vault` docs + docker-build.sh), `0341f179`
(wip), and `b50dba3c` (prompt_cache_key extraction + `Session_id` outbound).

## Decision

Sync the fork onto upstream `v7.2.141` (`dc3c3b1e`, identical to upstream/main): create
sync branch from the upstream tag, cherry-pick only `3b643a70` to carry the fork's
`.vault` knowledge, **drop** `b50dba3c`/`0341f179` as superseded by upstream, dispose the
stale branches (`patched/main` empty; `pr-3003`/`pr-3141` review-and-decide — port their
test assertions as regression tripwires, carry code only on test failure), build, deploy,
and verify continuity and cache behavior on the new base.

**Supersede note (rev. 3):** executed — branch `sync/upstream-v7.2.141`, deployed
`v7.2.141-4-g13123d7c`. Its continuity assumption is amended by ADR-010: upstream fixes
the *no-key* path, but a fork-local override is required for *key-sending* clients. The
sync itself remains the correct base.

## Alternatives Considered

1. **Rebuild/deploy from current fork source (v7.2.49-9-g3b643a70)** — smallest delta,
   but leaves the fork 631 commits behind and does not adopt upstream's Aug 12–19 fixes.
2. **Hybrid: sync upstream then re-apply fork deltas as patches** — preserves fork
   behavior, but re-introduces code upstream supersedes; highest conflict surface against
   the executor split.
3. **Mid-point tag (v7.2.100-ish)** — smaller jump, but misses the tail of the fix cluster
   (`2005788f` Aug 19) and requires a second sync later.
4. **Stay on v7.2.49 and only patch `Session_id` → `Session-Id`** — minimal fix, but
   leaves all other cache/telemetry/catalog fixes absent; the human directed otherwise.

## Consequences

- **Positive:** deployed artifact becomes `v7.2.141`-based with upstream-maintained
  continuity, cache stripping, token-detail telemetry, and catalog invalidation; fork-local
  cache code nearly eliminated.
- **Positive:** upstream tests assert canonical `Session-Id`; verification cheaper than
  maintaining fork tests.
- **Negative:** 631-commit jump broadens regression surface; required config validation
  (33/33 passed), staged rollout, old-binary rollback.
- **Negative:** fork-specific behavior in `b50dba3c` dropped on the assumption upstream
  covers it — proven by ported fork tripwires.
- **Negative:** `.vault` knowledge is fork-local and had to be carried manually
  (cherry-pick `3b643a70`), or it would be lost.
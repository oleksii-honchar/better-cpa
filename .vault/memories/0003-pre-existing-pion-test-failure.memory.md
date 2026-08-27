---
type: memory
title: "go test ./... has one pre-existing Pion WebRTC failure (internal/client/codex/live)"
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, testing, gotcha]
see_also: ["adrs/0003-sync-upstream-v7-2-141.adr.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Memory: pre-existing Pion WebRTC test failure

## Fact

`TestPionMediaRelayBridgesAudioAndDataChannel` in `internal/client/codex/live` FAILS in
`go test ./...`. Verified pre-existing at clean HEAD `274c2619` on `sync/upstream-v7.2.141`
via a temp worktree; the package is untouched by the continuity override work.

## Context

Full suite on 2026-08-25: 89 ok / 1 FAIL / 32 no-test-files (122 pkgs). The failure is
flaky-or-upstream, out of scope for cache remediation.

## Impact

Treat this as a known baseline failure when triaging future full-suite runs on the sync
branch. Confirm flake vs upstream issue before assuming regression from new work.
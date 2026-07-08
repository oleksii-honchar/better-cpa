---
type: index
title: "Domain Concepts"
createdAt: "2026-07-08T22:20:00Z"
updatedAt: "2026-07-08T22:20:00Z"
tags: []
---

# Domain Concepts

Key domain concepts and mental models for the better-cpa proxy and its caching architecture.

## Nodes

- [[concepts/0001-codex-session-header-architecture|Codex Session Header Architecture]] — How CPA forwards session identifiers to the upstream Codex API for prompt caching. Header standardization, continuity key resolution chain, affected code paths. (2026-07-08)

- [[concepts/0002-codex-cache-continuity-mechanism|Codex Cache Continuity Mechanism]] — How CPA maintains stable cache keys across multiple conversation turns via 3-priority fallback chain. Client compatibility matrix included. (2026-07-08)

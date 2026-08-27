---
type: index
title: "Memories"
createdAt: "2026-07-08T22:20:00Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: []
---

# Memories

Atomic durable facts, incident learnings, API quirks, and deployment gotchas.

## Nodes

- [[memories/0001-opencode-random-prompt-cache-key|opencode sends a random v4 UUID prompt_cache_key per request]] — opencode mints a random v4 UUID per request; a key-forwarding proxy changes the cache partition every turn. Confirmed root cause of the cache-hit drop. (2026-08-25)

- [[memories/0002-gpt56-implicit-breakpoint-cache|GPT-5.6 implicit caching breakpoint behavior]] — GPT-5.6 manages its breakpoint near the latest user/tool message; stable-prefix + changing suffix can miss. And factor; codex cannot emit explicit breakpoints. (2026-08-25)

- [[memories/0003-pre-existing-pion-test-failure|go test ./... one pre-existing Pion WebRTC failure]] — `TestPionMediaRelayBridgesAudioAndDataChannel` fails at clean HEAD; known baseline on the sync branch. (2026-08-25)

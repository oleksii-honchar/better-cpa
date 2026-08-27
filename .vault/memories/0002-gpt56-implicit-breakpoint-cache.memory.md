---
type: memory
title: "GPT-5.6 implicit caching manages the breakpoint near the latest user/tool message"
createdAt: "2026-08-25T10:38:25Z"
updatedAt: "2026-08-25T10:38:25Z"
tags: [better-cpa, codex, caching, gpt-5.6, platform-behavior]
see_also: ["concepts/0002-codex-cache-continuity-mechanism.concept.md"]
deprecated:
  date: null
  reason: null
  superseded_by: null
---

# Memory: GPT-5.6 implicit caching breakpoint behavior

## Fact

GPT-5.6 implicit caching places a managed breakpoint near the latest user/tool message and
no longer relies on prior 128-token rounding. A large stable prefix followed by a changing
suffix can lose cache hits. Codex cannot emit explicit `prompt_cache_breakpoint`
(`prompt_cache_options`/`prompt_cache_breakpoint`/`prompt_cache_retention` are stripped by
upstream `2ab25eae`, `6edf9c48`, `2005788f` on v7.2.141).

## Context

Vendored migration guide + upstream openai/codex#35300. Reported A/B on GPT-5.6 (AWS
Bedrock Mantle backend): 0% warm hits without explicit breakpoint vs ~98.6% with; append-
only intra-session turns reached ~98%. Backend-specific validation required. `gpt-5.6-luna`
first appears in CPA logs 2026-08-13 16:30 — inside the regression window.

## Impact

Secondary factor in the cache-hit drop (after ADR-010's key churn fix). If stable keys
still miss on divergent-suffix turns, this is the likely cause; the deferred breakpoint
canary (not implemented) would be the fix path. Compare `cached_tokens`, `cache_write_tokens`,
latency, and cost when judging manual verification results.
---
status: approved
approved: 2026-10-20T22:36:59.362585743Z
kind: bug
---

## Summary

Sentry issue `NUKE-PROD-CF` is a real bug in `bborbe/agent-claude` at `bborbe/agent-claude/pkg/factory/factory.go:183`, filed automatically by the sentry-fix-agent from the High/High real bug deep verdict — that verdict is the approval gate, so this spec is approved rather than drafted.

## Problem

Task publisher dispatched a Kafka message to agent-claude with an empty task_type frontmatter field; agent-claude's dispatch table only registers [healthcheck, llm, oauth-probe] and has no catch-all for empty string, causing a map miss and returning an error

## Reproduction

The implicated source location from the deep verdict is `bborbe/agent-claude/pkg/factory/factory.go:183` in `bborbe/agent-claude` (resolution rule: `candidate position 1 (bborbe/agent-claude)`), Sentry issue `NUKE-PROD-CF`.

## Recommended Fix

Audit the task-publishing side (executor/task-controller) to ensure all dispatched tasks carry a non-empty, validated task_type from the agent.TaskType* constants before spawning a job for agent-claude; agent-claude itself requires no code change

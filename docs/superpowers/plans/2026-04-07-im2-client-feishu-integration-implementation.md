# IM2 Client Feishu Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in `im.version: 2` runtime path that connects `client.Client` to `im2.Router` and implements Feishu IM2 outbound ACP/decision handling without using `hub/im.ImAdapter`.

**Architecture:** Client owns session lifecycle and binds IM2 chats after it creates or loads sessions. IM2 Router owns chat/session bindings, fanout, channel running, and decision routing. IM2 Feishu owns Feishu-specific ACP update rendering and permission/card-action handling, referencing IM1 Feishu behavior as migration material but not composing `hub/im.ImAdapter`.

**Tech Stack:** Go 1.22+, existing `internal/hub/client`, `internal/im2`, `internal/im2/feishu`, existing Feishu SDK wrapper under `internal/hub/im/feishu` for transport, `go test ./...`.

---

## Scope Check

This plan covers one end-to-end IM2 integration slice: opt-in config, hub/client wiring, router decision support, and Feishu IM2 rendering/decision handling. It does not remove IM1 or implement the App channel network protocol.

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server/internal/shared/config.go` | Modify | Add `im.version` config field |
| `server/internal/im2/protocol.go` | Modify | Add text, ACP, and decision payload contracts |
| `server/internal/im2/router.go` | Modify | Add channel running and decision routing |
| `server/internal/im2/router_test.go` | Modify | Cover `Run` and `RequestDecision` |
| `server/internal/im2/app/app.go` | Modify | Implement new `RequestDecision` channel method |
| `server/internal/im2/app/app_test.go` | Modify | Keep App stub contract compiling |
| `server/internal/im2/feishu/feishu.go` | Modify | Replace thin string adapter with IM2-owned ACP/decision handling |
| `server/internal/im2/feishu/feishu_test.go` | Modify | Cover outbound ACP routing and decision resolution |
| `server/internal/hub/client/client.go` | Modify | Add IM2 runtime field, inbound handler, run support, binding helpers |
| `server/internal/hub/client/session.go` | Modify | Add IM2 source/router on Session, route replies and permissions through IM2 |
| `server/internal/hub/client/commands.go` | Modify | Use new session for `/new` reply and bind IM2 source on `/new`/`/load` |
| `server/internal/hub/client/client_internal_test.go` | Modify | Add test helper for IM2 router |
| `server/internal/hub/client/client_test.go` | Modify | Add client IM2 inbound/output tests |
| `server/internal/hub/hub.go` | Modify | Branch on `im.version == 2` and build IM2 Feishu |
| `server/internal/hub/hub_test.go` | Create/Modify | Cover IM2 opt-in and unsupported runtime |
| `docs/superpowers/plans/2026-04-07-im2-client-feishu-integration-implementation.md` | Create | This plan |

## Tasks

- [ ] Add config and IM2 protocol payload tests, then implement `IMConfig.Version`, `TextPayload`, `ACPPayload`, `Decision*` types, and `Channel.RequestDecision`.
- [ ] Add router tests for `Run` and `RequestDecision`, then implement router channel running and source-only decision routing.
- [ ] Add App stub compile tests, then update App channel for `RequestDecision`.
- [ ] Add Feishu tests using an injected fake Feishu transport, then implement IM2-owned `Send` routing for message/system/acp and `RequestDecision` with card-action/text fallback.
- [ ] Add client tests for `SetIM2Router`, unbound `/list` direct response, unbound prompt bind, existing session routing, ACP outbound payload, and permission mapping; then implement client/session IM2 bridge.
- [ ] Add hub tests for IM1 default, IM2 Feishu opt-in, and unsupported IM2 type; then implement hub config branching.
- [ ] Run `gofmt`, `go test ./internal/im2/... -v`, `go test ./internal/hub/client -v`, `go test ./internal/hub -v`, and `go test ./...` from `server/`.
- [ ] Commit and push branch, then trigger `scripts/signal_update_now.ps1 -DelaySeconds 30` after merge/push per `CLAUDE.md`.

## TDD Notes

Each task must be implemented red-green: write the focused test first, run it and verify the expected failure, then add minimal code to pass. For Feishu behavior, tests should use an injected fake transport so no network or Feishu credentials are needed.

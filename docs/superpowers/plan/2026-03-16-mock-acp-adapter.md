# Mock ACP Adapter Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a built-in mock ACP adapter with in-memory ACP transport and scenario-driven responses for integration testing.

**Architecture:** Extend `acp.Conn` with an in-memory startup path and embed a mock ACP server loop in the same process. Implement `provider/mock` to construct this connection and register it in hub. Keep upper layers unchanged by returning a normal `*acp.Conn`.

**Tech Stack:** Go, existing ACP JSON-RPC transport, go test.

---

### Task 1: Add failing tests for mock adapter and mock scenarios

**Files:**
- Create: `internal/agent/provider/mock/adapter_unit_test.go`
- Create: `internal/agent/provider/mock_server_test.go`

- [ ] **Step 1: Write failing tests for adapter name/connect/close**
- [ ] **Step 2: Write failing tests for scenarios 1/2/3/4 and 10+**
- [ ] **Step 3: Run targeted tests and confirm failures**

### Task 2: Implement in-memory ACP transport and mock server

**Files:**
- Modify: `internal/agent/provider/connect.go`
- Create: `internal/agent/provider/mock_server.go`

- [ ] **Step 1: Add in-memory constructor/start path to Conn**
- [ ] **Step 2: Implement mock server request router and scenario handlers**
- [ ] **Step 3: Run acp tests and make them pass**

### Task 3: Add mock adapter and hub registration

**Files:**
- Create: `internal/agent/provider/mock/provider.go`
- Modify: `internal/hub/hub.go`

- [ ] **Step 1: Implement provider.Provider for mock**
- [ ] **Step 2: Register `mock` in hub**
- [ ] **Step 3: Run adapter/hub tests and make them pass**

### Task 4: Verify end-to-end and regressions

**Files:**
- Modify as needed based on failures.

- [ ] **Step 1: Run package-level tests for touched packages**
- [ ] **Step 2: Run `go test ./...`**
- [ ] **Step 3: Summarize behavior coverage and remaining gaps**








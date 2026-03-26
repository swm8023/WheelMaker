# App Project Workspace State Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a project-scoped workspace store so Chat/Files/Diff share a single state model with consistent behavior across wide and narrow layouts.

**Architecture:** Add a lightweight `ChangeNotifier` store and project-scoped state models, then refactor `WorkspaceDebugScreen` to bind UI interactions to store state. Keep mock data source abstraction for future server-backed refresh while preserving current behavior.

**Tech Stack:** Flutter, Dart, ChangeNotifier, existing mock data models.

---

## Chunk 1: Data Layer Foundations

### Task 1: Add project workspace models

**Files:**
- Create: `app/lib/models/project_workspace_state.dart`

- [ ] **Step 1: Write model definitions**
- [ ] **Step 2: Add helper methods for selection fallback**
- [ ] **Step 3: Verify compile references locally (`flutter analyze`)**
- [ ] **Step 4: Commit**

### Task 2: Add mock project data source abstraction

**Files:**
- Create: `app/lib/data/project_data_source.dart`

- [ ] **Step 1: Define `ProjectDataSource` interface**
- [ ] **Step 2: Implement mock source with 2 projects**
- [ ] **Step 3: Ensure files/commits/chat session mapping works per project**
- [ ] **Step 4: Commit**

### Task 3: Add store

**Files:**
- Create: `app/lib/stores/project_workspace_store.dart`

- [ ] **Step 1: Implement active project + state map + mutating APIs**
- [ ] **Step 2: Implement `refreshProject(projectId)` with per-pane isolation**
- [ ] **Step 3: Add sidebar/tab actions and selected item actions**
- [ ] **Step 4: Commit**

## Chunk 2: Screen Refactor

### Task 4: Refactor `WorkspaceDebugScreen` to store-driven UI

**Files:**
- Modify: `app/lib/screens/workspace_debug_screen.dart`

- [ ] **Step 1: Remove local duplicated state fields**
- [ ] **Step 2: Add store initialization and listener lifecycle**
- [ ] **Step 3: Add project dropdown in title area**
- [ ] **Step 4: Wire chat/files/diff drawer and content to active project state**
- [ ] **Step 5: Keep narrow drawer + wide collapse behavior consistent**
- [ ] **Step 6: Commit**

## Chunk 3: Verification and Completion

### Task 5: Static verification

**Files:**
- Modify: (none expected)

- [ ] **Step 1: Run `flutter analyze`**
- [ ] **Step 2: Run `flutter test`**
- [ ] **Step 3: Fix any issues and re-run**
- [ ] **Step 4: Commit if fixes required**

### Task 6: Completion gate and push

**Files:**
- Modify: all changed files

- [ ] **Step 1: `git add -A`**
- [ ] **Step 2: `git commit -m "feat(app): add project-scoped workspace store"`**
- [ ] **Step 3: `git push origin feat/server-remote-observe`**
- [ ] **Step 4: Run app refresh script because `app/` changed**

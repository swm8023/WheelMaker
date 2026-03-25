# App Explorer VS Code Modern Dark Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the app file explorer UI to closely match VS Code Modern Dark and provide reliable syntax highlighting.

**Architecture:** Keep `FileExplorerScreen` as the feature shell, split language resolution into a focused utility, and render code using `flutter_highlight` with a VS Code-like theme map. Preserve current responsive split/drawer behavior while improving visual states and editor polish.

**Tech Stack:** Flutter, flutter_test, flutter_highlight/highlight.

---

## Chunk 1: Test-First Baseline

### Task 1: Add language mapping tests

**Files:**
- Create: `app/test/screens/code_language_test.dart`
- Create: `app/lib/screens/code_language.dart`

- [ ] **Step 1: Write the failing test**

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/screens/code_language.dart';

void main() {
  test('resolves common extensions and defaults to plaintext', () {
    expect(languageFromPath('/a/main.dart'), 'dart');
    expect(languageFromPath('/a/server.go'), 'go');
    expect(languageFromPath('/a/config.yaml'), 'yaml');
    expect(languageFromPath('/a/README.md'), 'markdown');
    expect(languageFromPath('/a/run.ps1'), 'powershell');
    expect(languageFromPath('/a/unknown.xyz'), 'plaintext');
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `flutter test test/screens/code_language_test.dart`
Expected: FAIL because resolver file/function does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```dart
String languageFromPath(String path) { ... }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `flutter test test/screens/code_language_test.dart`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/lib/screens/code_language.dart app/test/screens/code_language_test.dart
git commit -m "test: add language resolver coverage"
```

## Chunk 2: Explorer UI and Syntax Highlighter

### Task 2: Add explorer interaction widget tests

**Files:**
- Create: `app/test/screens/file_explorer_screen_test.dart`
- Modify: `app/lib/screens/file_explorer_screen.dart`

- [ ] **Step 1: Write failing widget tests for selected row, expand/collapse, and editor header updates**
- [ ] **Step 2: Run test to verify failures**
Run: `flutter test test/screens/file_explorer_screen_test.dart`
Expected: FAIL due to missing state hooks / styles.
- [ ] **Step 3: Implement minimal UI/state updates in `file_explorer_screen.dart`**
- [ ] **Step 4: Re-run tests to verify pass**
Run: `flutter test test/screens/file_explorer_screen_test.dart`
Expected: PASS.
- [ ] **Step 5: Commit**

### Task 3: Integrate syntax highlighting package

**Files:**
- Modify: `app/pubspec.yaml`
- Modify: `app/lib/screens/file_explorer_screen.dart`

- [ ] **Step 1: Add failing assertion in widget test that highlighted content renders**
- [ ] **Step 2: Add `flutter_highlight` dependency and render path**
- [ ] **Step 3: Run targeted tests**
Run: `flutter test test/screens/file_explorer_screen_test.dart`
- [ ] **Step 4: Run full app tests**
Run: `flutter test`
- [ ] **Step 5: Commit**

## Chunk 3: Verification and Refresh

### Task 4: Final verification and app refresh

**Files:**
- No additional source files (verification only)

- [ ] **Step 1: Run full test suite**
Run: `flutter test`
Expected: PASS.
- [ ] **Step 2: Refresh app web artifacts (repo policy)**
Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_flutter_web.ps1`
Expected: script completes without error.
- [ ] **Step 3: Final commit/push sequence**
Run:
`git add -A`
`git commit -m "feat(app): match vscode modern dark explorer and add syntax highlighting"`
`git push origin <branch>`


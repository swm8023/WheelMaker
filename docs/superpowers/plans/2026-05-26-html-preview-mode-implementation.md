# HTML Preview Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add MD-like preview/source switching for `.html` and `.htm` files, with scripts disabled by default and manually enabled per current file view.

**Architecture:** Keep the feature inside the existing React file viewer in `app/web/src/main.tsx`. Add path detection, two local state values, an iframe preview component, toolbar controls, and a render branch. Extend the existing structure test that already protects Markdown preview behavior.

**Tech Stack:** React 19, TypeScript, webpack, Jest string-structure tests, Shiki source rendering, CSS in `app/web/src/styles.css`.

---

### Task 1: HTML Preview Structure Test

**Files:**
- Modify: `app/__tests__/web-markdown-preview-mode.test.ts`

- [ ] **Step 1: Write the failing test**

Add expectations to the existing test so the current code fails because HTML preview is not implemented:

```ts
expect(mainTsx).toContain('function isHtmlPath(path: string): boolean {');
expect(mainTsx).toContain("return ext === 'html' || ext === 'htm';");
expect(mainTsx).toContain('const selectedFileIsHtml = isHtmlPath(selectedFile);');
expect(mainTsx).toContain('const [htmlPreviewEnabled, setHtmlPreviewEnabled] = useState(false);');
expect(mainTsx).toContain('const [htmlPreviewScriptsEnabled, setHtmlPreviewScriptsEnabled] = useState(false);');
expect(mainTsx).toContain('setHtmlPreviewEnabled(isHtmlPath(selectedFile));');
expect(mainTsx).toContain('setHtmlPreviewScriptsEnabled(false);');
expect(mainTsx).toContain('aria-label="Toggle HTML preview"');
expect(mainTsx).toContain('<span className="html-preview-toggle-text">HTML</span>');
expect(mainTsx).toContain('aria-label="Toggle HTML scripts"');
expect(mainTsx).toContain('<HtmlPreview');
expect(mainTsx).toContain('sandbox={scriptsEnabled ? \\'allow-scripts\\' : \\'\\'}');
expect(mainTsx).toContain('srcDoc={content}');
expect(stylesCss).toContain('.html-preview {');
expect(stylesCss).toContain('.html-preview-frame {');
expect(stylesCss).toContain('.html-preview-toggle {');
expect(stylesCss).toContain('.html-script-toggle {');
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
cd app
npm test -- web-markdown-preview-mode.test.ts
```

Expected: fail on missing `isHtmlPath` or another HTML preview assertion.

### Task 2: HTML Preview Implementation

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Implement path detection and state**

Add `isHtmlPath` near `isMarkdownPath`:

```ts
function isHtmlPath(path: string): boolean {
  const ext = getFileExtension(path);
  return ext === 'html' || ext === 'htm';
}
```

Add state in the main component:

```ts
const [htmlPreviewEnabled, setHtmlPreviewEnabled] = useState(false);
const [htmlPreviewScriptsEnabled, setHtmlPreviewScriptsEnabled] = useState(false);
```

Reset it when the selected file changes:

```ts
useEffect(() => {
  setMarkdownPreviewEnabled(isMarkdownPath(selectedFile));
  setHtmlPreviewEnabled(isHtmlPath(selectedFile));
  setHtmlPreviewScriptsEnabled(false);
}, [selectedFile]);
```

- [ ] **Step 2: Add iframe preview component**

Add a memoized component near `MarkdownPreview`:

```tsx
const HtmlPreview = React.memo(function HtmlPreview({
  content,
  scriptsEnabled,
}: HtmlPreviewProps) {
  return (
    <div className="html-preview">
      <iframe
        className="html-preview-frame"
        title="HTML preview"
        sandbox={scriptsEnabled ? 'allow-scripts' : ''}
        srcDoc={content}
      />
    </div>
  );
});
```

- [ ] **Step 3: Add toolbar controls**

Extend `renderViewTools` so HTML files get an `HTML` toggle and, while preview is active, a script toggle:

```tsx
{selectedFileIsHtml ? (
  <button
    type="button"
    className={`view-tool html-preview-toggle ${
      htmlPreviewEnabled ? 'active' : ''
    }`}
    onClick={() => setHtmlPreviewEnabled(value => !value)}
    title={htmlPreviewEnabled ? 'Switch to source mode' : 'Switch to HTML preview'}
    aria-label="Toggle HTML preview"
  >
    <span className="html-preview-toggle-text">HTML</span>
  </button>
) : null}
{selectedFileIsHtml && htmlPreviewEnabled ? (
  <button
    type="button"
    className={`view-tool html-script-toggle ${
      htmlPreviewScriptsEnabled ? 'active' : ''
    }`}
    onClick={() => setHtmlPreviewScriptsEnabled(value => !value)}
    title={htmlPreviewScriptsEnabled ? 'Disable HTML scripts' : 'Enable HTML scripts'}
    aria-label="Toggle HTML scripts"
  >
    <span className="codicon codicon-run-all view-tool-icon" />
  </button>
) : null}
```

- [ ] **Step 4: Add render branch and styles**

Add the branch after Markdown preview and before source fallback:

```tsx
) : selectedFileIsHtml && htmlPreviewEnabled ? (
  <HtmlPreview
    key={`${selectedFile}:${htmlPreviewScriptsEnabled ? 'scripts' : 'static'}`}
    content={fileContent}
    scriptsEnabled={htmlPreviewScriptsEnabled}
  />
```

Add styles:

```css
.html-preview-toggle,
.html-script-toggle {
  font-size: 10px;
  font-weight: 700;
}

.html-preview-toggle-text {
  line-height: 1;
}

.html-preview {
  width: 100%;
  min-height: 100%;
  background: #fff;
}

.html-preview-frame {
  display: block;
  width: 100%;
  min-height: calc(100vh - 170px);
  border: 0;
  background: #fff;
}
```

### Task 3: Verification and Completion

**Files:**
- Verify: `app/__tests__/web-markdown-preview-mode.test.ts`
- Verify: `app/web/src/main.tsx`
- Verify: `app/web/src/styles.css`

- [ ] **Step 1: Run targeted test**

Run:

```powershell
cd app
npm test -- web-markdown-preview-mode.test.ts
```

Expected: pass.

- [ ] **Step 2: Run TypeScript check**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: exit code 0.

- [ ] **Step 3: Review diff against acceptance criteria**

Run:

```powershell
git diff -- app/__tests__/web-markdown-preview-mode.test.ts app/web/src/main.tsx app/web/src/styles.css
```

Confirm the diff implements:

1. HTML opens in preview mode by default.
2. Source mode remains available.
3. Scripts are disabled by default.
4. Script permission resets on file change.
5. Markdown and image behavior remains structurally unchanged.

- [ ] **Step 4: Commit and push**

Run from repository root:

```powershell
git add -A
git commit -m "feat: add html preview mode"
git push origin feature/html-preview-mode
```

# HTML Preview Mode Design

Date: 2026-05-26
Status: Approved for specification

## Background

The workspace file viewer already has a Markdown preview mode. Markdown files are detected by path, open in rendered preview by default, and can be switched back to source with a compact toolbar button. Non-previewed files fall back to the Shiki source viewer.

HTML files currently use the source viewer only. Users need the same preview/source workflow for `.html` and `.htm` files, with an explicit way to opt into script execution when inspecting interactive HTML.

## Goals

1. Add preview/source switching for `.html` and `.htm` files in the existing file viewer.
2. Open HTML files in preview mode by default, matching the Markdown behavior.
3. Render HTML in an isolated iframe.
4. Disable script execution by default.
5. Let the user enable scripts manually for the current file and current viewing session only.
6. Keep the first version scoped to the current HTML document content.
7. Preserve the existing image, Markdown, and source viewer behavior.

## Non-goals

1. Do not resolve relative workspace resources such as `./style.css`, `./image.png`, or `./app.js`.
2. Do not add a registry or backend static file proxy.
3. Do not persist HTML preview mode or script permission across files, projects, browser refreshes, or sessions.
4. Do not redesign the shared file viewer toolbar.
5. Do not change Markdown preview behavior.
6. Do not parse and rewrite HTML or CSS content.

## Chosen Approach

Reuse the Markdown preview interaction pattern and add an HTML-specific iframe preview branch.

HTML files get a compact toolbar button that toggles between preview and source. When preview is active, a separate script control appears. The iframe uses `srcDoc` with the selected file content. Its sandbox is script-free by default. Enabling scripts adds `allow-scripts` for the current selected file while the user remains in the current view.

This keeps the feature inside the current frontend file-viewer boundary. The existing `fs.read` flow still reads only the selected file. Source mode continues to use the existing Shiki code pane, including the existing `html` language support.

## User Experience

When a user selects an `.html` or `.htm` file:

1. The file opens in preview mode.
2. The toolbar shows an `HTML` preview/source toggle.
3. Preview mode shows an isolated iframe rendering the current file content.
4. A script toggle is visible only while an HTML preview is active.
5. Scripts are disabled by default.
6. If the user enables scripts, the iframe is remounted with script permission.
7. Switching files resets script permission to disabled.
8. Switching the HTML file to source mode hides the script toggle.

The preview may display incomplete pages when the HTML depends on relative workspace assets. Inline CSS, inline scripts after manual enablement, data URLs, and absolute `http(s)` URLs follow browser iframe behavior. Relative workspace paths are not fetched or rewritten in this version.

## Components

### Path Detection

Add `isHtmlPath(path: string): boolean` next to `isMarkdownPath`. It should detect `.html` and `.htm` extensions using the existing `getFileExtension` helper.

### Viewer State

Add two pieces of React state:

1. `htmlPreviewEnabled`
2. `htmlPreviewScriptsEnabled`

On `selectedFile` change:

1. `markdownPreviewEnabled` keeps the current behavior.
2. `htmlPreviewEnabled` becomes `true` only for HTML files.
3. `htmlPreviewScriptsEnabled` always resets to `false`.

This gives each selected HTML file a fresh default-safe preview.

### Toolbar

Extend `renderViewTools`:

1. Markdown files keep the current `MD` button.
2. HTML files show an `HTML` button with the same active styling and source/preview toggle semantics.
3. HTML files in preview mode show a script toggle button.

The script toggle should be a clear, compact control with an accessible label. It should not appear for Markdown, image, diff, or non-preview file types.

### HTML Preview

Add a memoized `HtmlPreview` component with props:

1. `content: string`
2. `scriptsEnabled: boolean`

It renders:

```tsx
<iframe
  className="html-preview-frame"
  title="HTML preview"
  sandbox={scriptsEnabled ? 'allow-scripts' : ''}
  srcDoc={content}
/>
```

If the iframe needs to be forced to reload when script permission changes, key it by the selected file plus `scriptsEnabled`.

### Render Branch

Keep the existing render priority:

1. Loading state.
2. Image preview.
3. Markdown preview when selected file is Markdown and Markdown preview is enabled.
4. HTML preview when selected file is HTML and HTML preview is enabled.
5. Source viewer fallback.

The source viewer receives `detectCodeLanguage(selectedFile)`, which already maps HTML to Shiki's `html` language.

## Security

The default sandbox must not include `allow-scripts`. The first version should not add `allow-same-origin`, `allow-forms`, `allow-popups`, or navigation permissions. This avoids giving local workspace HTML broad browser capabilities by default.

When scripts are enabled, only `allow-scripts` is added. The permission is not stored. It is cleared when the selected file changes, which makes script execution a deliberate per-file action.

Relative workspace resources are not resolved. This prevents the preview from becoming an implicit local file server and keeps path traversal, MIME detection, cache policy, and binary resource handling out of scope.

## Error Handling

The iframe preview does not need a custom parse error state. Browser HTML parsing is tolerant. If the HTML is malformed, the iframe shows the browser's rendered result.

If the file content is empty, the iframe displays a blank document. Source mode remains available.

## Testing

Add or extend frontend structure tests near `web-markdown-preview-mode.test.ts`:

1. `isHtmlPath` exists and `.html` / `.htm` are detected.
2. `selectedFileIsHtml` is derived from `isHtmlPath(selectedFile)`.
3. HTML preview state exists and defaults to enabled on HTML file selection.
4. HTML script permission state exists and resets to disabled on file selection.
5. The toolbar contains an `HTML` preview/source toggle.
6. The script toggle is present only for active HTML preview mode.
7. The render branch includes `<HtmlPreview`.
8. `HtmlPreview` uses an iframe with `srcDoc` and a sandbox that adds `allow-scripts` only when enabled.

Run:

```powershell
cd app
npm test -- web-markdown-preview-mode.test.ts
npm run tsc:web
```

If the implementation touches broader app structure, run the full app test suite as well.

## Alternatives Considered

### General Preview/Source Abstraction

A shared preview/source abstraction for Markdown and HTML would reduce naming duplication. It is not chosen for the first version because it would require renaming existing Markdown state and broadening tests without changing user-visible behavior.

### Frontend Resource Rewriting

The frontend could parse HTML, read relative workspace resources through `fs.read`, and rewrite resource URLs to data or blob URLs. That would improve visual fidelity, but it also introduces HTML parsing, CSS `url()` rewriting, MIME handling, script dependency ordering, binary reads, and path traversal concerns.

### Backend Static Resource Proxy

A registry-backed static resource proxy would produce the most realistic preview for workspace HTML. It is intentionally left out because it changes backend and protocol surface area for a first UI iteration.

## Acceptance Criteria

1. Opening an HTML file shows rendered preview by default.
2. The user can switch the same HTML file between preview and source.
3. Source mode renders HTML with the existing code viewer.
4. Scripts do not run by default.
5. The user can enable scripts explicitly for the current file view.
6. Switching files disables script execution again.
7. Markdown preview mode still behaves as before.
8. Image preview behavior is unchanged.

# App Theme and Highlight Design (VS Code Modern Dark)

## Goal

Add a switchable theme system with a default style close to VS Code Modern Dark, plus broad code highlighting support and rendered Markdown output.

## Scope

- Add central theme tokens and runtime theme mode switching.
- Restyle workspace UI to consume theme tokens.
- Add VS Code-like file icon mapping (first-pass extension set).
- Add language resolver for code highlighting with wide fallback coverage.
- Render Markdown files as rendered Markdown, not plain source.

## Non-Goals (This Iteration)

- Perfect parity with VS Code semantic tokenization.
- Full icon theme parity for every extension.
- Rich markdown plugins (mermaid/math/table-of-contents).

## Architecture

- `src/theme/tokens.ts`: theme definitions (`modernDark`, `modernLight`) and shared semantic tokens.
- `src/theme/index.ts`: theme exports and mode helper.
- `src/utils/codeLanguage.ts`: file path -> language id resolver for highlighter.
- `src/utils/fileIcon.ts`: file/folder icon + color mapping.
- `src/components/CodeView.tsx`: code renderer with syntax highlighting.
- `src/components/MarkdownView.tsx`: markdown renderer for `.md`/`.markdown`.
- `WorkspaceScreen` reads current theme and toggles mode in header.

## Highlight Strategy

- Use `react-native-syntax-highlighter` for broad language coverage (Highlight.js style set).
- Resolve language from extension/filename.
- Unknown files fallback to `plaintext` without crash.

## Markdown Strategy

- Use `react-native-markdown-display` to render markdown output.
- For markdown files, file pane shows rendered content instead of raw text.

## Testing

- Unit: language resolver mappings and fallback.
- Unit: file icon resolver mappings and fallback.
- Unit: theme mode helper.
- Integration sanity: existing app tests + web build continue to pass.

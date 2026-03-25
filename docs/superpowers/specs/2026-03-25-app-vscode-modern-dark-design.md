# App Explorer VS Code Modern Dark Design

## Goal

Make the app file explorer and code viewer feel like VS Code Modern Dark, with normal syntax highlighting quality and stable fallback behavior.

## Scope

- Update explorer visual hierarchy to match VS Code dark panel style.
- Improve interaction details: selected row, hover feedback, folder/file icon usage, and editor header row.
- Replace ad-hoc tokenizer-based highlighting with language-aware highlighting.
- Keep current split layout behavior for wide and narrow screens.

## Architecture

- `FileExplorerScreen` remains the main feature entry for tree + editor.
- Introduce a small language resolver utility to map file extensions to highlight language names.
- Use `flutter_highlight` to render code with a VS Code-like dark theme map.
- Keep a safe fallback path to plain text when language is unknown.

## Components

- Explorer pane:
  - Section title (`EXPLORER`)
  - Collapsible folder rows
  - File rows with selected/hover states
- Editor pane:
  - Header strip with active file icon/path
  - Scrollable highlighted code content

## Data Flow

- User taps a tree row -> updates `_activeFile`.
- Active file path is resolved to language via extension mapping.
- Editor renders highlighted text using language + theme map.
- Unknown extension resolves to `plaintext`.

## Error Handling

- Unknown extension: fallback to `plaintext`.
- Empty content: render empty editor without crash.
- Highlight parsing failure: show plain content text.

## Testing

- Unit test extension-to-language mapping, including fallback.
- Widget test explorer interactions:
  - selected file row state transitions after tap
  - folder expand/collapse updates visible children
  - editor header reflects selected file path


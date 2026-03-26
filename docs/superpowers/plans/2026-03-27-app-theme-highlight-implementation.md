# App Theme and Highlight Implementation Plan

## Step 1: Foundation utilities (TDD)

- Add tests for:
  - language resolver (common extensions + fallback)
  - file icon resolver (common types + fallback)
  - theme mode helper
- Implement corresponding utility modules.

## Step 2: Theme system

- Add semantic theme tokens and default VS Code-like dark theme.
- Add optional light theme and mode switch helper.
- Wire workspace styles to read from theme tokens.

## Step 3: File icons + typography

- Add first-pass VS Code-like icon glyph mapping.
- Apply icon and color in explorer rows.
- Apply editor font stack for code readability.

## Step 4: Code/Markdown rendering

- Install and integrate syntax highlighter component.
- Install and integrate markdown renderer.
- Route `.md`/`.markdown` to markdown renderer; other text files to code view.

## Step 5: Verification

- Run targeted tests.
- Run full test suite.
- Run web production build.

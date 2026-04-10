# Shiki Theme And Code Display Settings Design

## Context
The web app already supports Shiki-based code highlighting, a small hand-maintained list of code themes, and global code display settings for font, font size, line height, tab size, wrapping, and line numbers.

The current gap is not missing settings infrastructure. The gap is that the theme picker only exposes a narrow subset of the Shiki themes already available in the installed package, which makes themes such as `material-theme-darker` feel inconsistently supported.

## Confirmed Scope
- Runtime target: `app/web/` only.
- Expand the code theme picker from a short hand-maintained list to the full set of bundled themes available from the installed Shiki version.
- Keep `Auto (Dark+/Light+)` as a first-class virtual option.
- Keep code font, font size, line height, and tab size as explicit selectable global settings.
- Preserve global persistence semantics for these settings across all projects.

## Non-Goals
- No per-project theme or font preferences.
- No visual preview cards, modal picker, or search UI in this phase.
- No redesign of the overall app layout or non-code typography.
- No change to the main page `themeMode` model beyond continuing to coexist with code theme selection.

## Theme Source And Selection Behavior
### 1) Theme source of truth
The code theme options should be derived from the bundled themes exposed by the installed Shiki package, instead of being maintained as a hard-coded shortlist.

This keeps the UI aligned with the actual dependency version and removes the need to manually add individual bundled themes in multiple places.

### 2) Auto option
Keep a synthetic `auto-plus` option at the top of the list:
- dark page mode -> `dark-plus`
- light page mode -> `light-plus`

This remains the default fallback when persisted theme values are missing or invalid.

### 3) Theme independence
`themeMode` and `codeTheme` remain independent:
- users may choose a dark UI with a light code theme
- users may choose a light UI with a dark code theme

This preserves current behavior and avoids coupling page chrome preferences to code block preferences.

### 4) Required compatibility
Themes already in use, including `material-theme-darker`, must remain valid persisted values after the change.

## Settings UI Design
### 1) Keep the current entry point
Retain the existing sidebar `Settings` panel. Do not introduce a modal, drawer-within-drawer, or separate settings page.

### 2) Code display settings group
Keep these settings visible as direct controls in the sidebar settings panel:
- `Code Theme`
- `Code Font`
- `Font Size`
- `Line Height`
- `Tab Size`

These are already part of the app state model and should remain first-class configurable options.

### 3) Theme list organization
Render the `Code Theme` select in this order:
1. `Auto (Dark+/Light+)`
2. dark bundled themes
3. light bundled themes

The grouped order keeps the full list manageable without adding a heavier searchable control.

### 4) Labels
Theme labels should be readable display names generated from theme ids unless an explicit label override is needed.

Examples:
- `material-theme-darker` -> `Material Theme Darker`
- `github-dark-high-contrast` -> `GitHub Dark High Contrast`
- `vitesse-black` -> `Vitesse Black`

## Persistence And Validation
### 1) Persistence model
Continue storing these values in global workspace state:
- `themeMode`
- `codeTheme`
- `codeFont`
- `codeFontSize`
- `codeLineHeight`
- `codeTabSize`
- `wrapLines`
- `showLineNumbers`

No schema version bump is required if the stored shape remains unchanged.

### 2) Validation rules
Theme validation must be driven by the generated bundled theme option set plus the synthetic `auto-plus` option.

If a persisted theme value is not recognized, sanitize it back to the default `auto-plus`.

Existing font and numeric sanitization rules remain in place.

## Architecture And Touch Points
- `app/web/src/services/shikiRenderer.ts`
  - derive bundled theme ids from Shiki
  - expose grouped theme options
  - keep theme id validation centralized
  - keep font option definitions and font-family resolution centralized
- `app/web/src/main.tsx`
  - consume grouped theme options in the sidebar settings UI
  - keep current global state wiring unchanged except for using the richer options source
- `app/web/src/services/workspacePersistence.ts`
  - continue to sanitize persisted `codeTheme` using centralized validation
- `app/__tests__/web-code-layout.test.ts`
  - update assertions to reflect the bundled-theme-based implementation and the continued presence of code display controls

## Testing Strategy
- Add a failing test first that proves the renderer no longer relies on a narrow hard-coded theme union or shortlist.
- Verify the implementation still includes `material-theme-darker` as a selectable valid theme.
- Verify the settings UI still exposes `Code Theme`, `Code Font`, `Font Size`, `Line Height`, and `Tab Size`.
- Verify invalid persisted `codeTheme` values sanitize back to the default theme.
- Keep existing layout-oriented regression assertions that protect Shiki rendering and diff integration behavior.

## Risks And Mitigations
- Risk: exposing all bundled themes makes the picker long.
  - Mitigation: keep `Auto` first and group remaining themes by dark/light instead of leaving a flat unsorted list.
- Risk: a future Shiki upgrade could rename or remove a bundled theme.
  - Mitigation: derive validity from installed bundled metadata and keep invalid persisted values on a safe fallback path.
- Risk: label generation may produce awkward names for a few ids.
  - Mitigation: allow a small explicit override map for exceptional labels if needed, while keeping generation as the default path.

## Acceptance Criteria
- The code theme picker exposes the full bundled theme inventory from the installed Shiki version, plus `Auto`.
- `material-theme-darker` is selectable and remains valid for persisted state.
- Code font, font size, line height, and tab size remain directly configurable in global settings.
- Invalid persisted theme values safely fall back to the default theme.
- Existing code and diff rendering continue using the selected code theme and display settings.

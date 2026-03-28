# Shared App Layer

This folder now contains only shared code used by:

- Native shell (`App.native.tsx`)
- Pure React web app (`web/`)

Current modules:

- `src/config/runtime.ts`: runtime address/source helpers
- `src/services/*`: registry websocket client and workspace service
- `src/types/observe.ts`: registry protocol types

Legacy RN page-layer modules were removed during cleanup.

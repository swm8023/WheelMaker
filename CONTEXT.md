# WheelMaker Workspace

WheelMaker Workspace is the interactive shell for browsing projects, reading files, inspecting git state, and driving agent sessions from narrow and wide screens. This context exists to keep the product language for workspace surfaces, navigation, and status presentation consistent as the UI evolves.

## Language

**Header Bubble**:
A compact floating control that shows the current project title and refresh action on narrow screens.
_Avoid_: Title bar, top bar, header row

**Primary Navigation Bubble Group**:
A vertical pill-shaped floating control that contains the three main workspace tabs: Chat, File, and Git.
_Avoid_: 3-tab button, vertical tabs, tab stack

**Drawer Toggle Bubble**:
A separate floating control that opens or closes the narrow-screen drawer or sidebar surface.
_Avoid_: Expand button, menu button, unfold button

**Floating Control Stack**:
The draggable narrow-screen control layer that combines the **Primary Navigation Bubble Group** and the **Drawer Toggle Bubble** into one vertically arranged unit.
_Avoid_: Right-side buttons, floating tab stack, button pack

**Edge Snap**:
The landing behavior that keeps the **Floating Control Stack** aligned to its allowed screen edge after dragging ends.
_Avoid_: Free drop, loose placement, arbitrary resting point

**Inspector Status Bar**:
A narrow-screen bottom bar shared by the File and Git tabs that carries context-specific inspection status, such as file path, pinned items, or git-specific state.
_Avoid_: File header, git footer, bottom bar, pin strip

**Layout Mode**:
The viewport-width driven shell mode for WheelMaker Workspace. `desktop` starts at the 900px breakpoint; `mobile` is any narrower viewport, including a narrow PC browser window.
_Avoid_: Device type, phone mode, orientation mode

**Workspace UI State**:
The single root UI state that holds shared navigation state plus desktop-only, mobile-only, and transient layout state.
_Avoid_: Desktop state tree, mobile state tree, duplicated UI state

**Responsive Shell**:
The layout container that renders either the desktop shell or mobile shell from the same **Workspace UI State** and shared workspace content.
_Avoid_: Responsive component, mobile app, alternate app

## Relationships

- A **Header Bubble** is independent from the **Primary Navigation Bubble Group**
- A **Header Bubble** is fixed to the top-left anchor on narrow screens and is not draggable
- A **Drawer Toggle Bubble** is visually separate from the **Primary Navigation Bubble Group**
- A **Floating Control Stack** contains one **Primary Navigation Bubble Group** and one **Drawer Toggle Bubble**
- An **Edge Snap** determines the final resting position of the **Floating Control Stack** after dragging
- An **Inspector Status Bar** complements the **File** and **Git** tabs and does not replace the **Header Bubble**
- A **Layout Mode** is resolved from viewport width, not from device detection
- A **Responsive Shell** may have separate desktop and mobile structure, but both shells read the same **Workspace UI State**
- **Workspace UI State** preserves shared navigation and layout preference state while clearing transient gesture and keyboard state during **Layout Mode** transitions

## Example dialogue

> **Dev:** "Should the **Primary Navigation Bubble Group** stay visible when the user switches from Chat to File?"
> **Domain expert:** "Yes — it is the narrow-screen navigation surface, while the **Header Bubble** only carries title and refresh."

> **Dev:** "If a PC browser is narrowed below 900px, should it use the mobile shell?"
> **Domain expert:** "Yes — **Layout Mode** follows viewport width. Input capability is handled separately."

## Flagged ambiguities

- "expand" was used to mean both opening the drawer and exposing more controls — resolved: use **Drawer Toggle Bubble** for the drawer control
- "3tab button" was used to mean the floating Chat/File/Git control — resolved: use **Primary Navigation Bubble Group**
- "the four buttons" was used to imply independent dragging — resolved: drag the **Floating Control Stack** as one unit
- "floating bubbles" could imply every bubble is draggable — resolved: only the **Floating Control Stack** is draggable; the **Header Bubble** stays fixed
- "snap to the edge" could imply free horizontal dragging — resolved: the **Floating Control Stack** already stays on the right edge; **Edge Snap** governs the allowed resting position after vertical dragging
- "bottom status bar" was first used to mean a File-only surface — resolved: use **Inspector Status Bar** for the shared File/Git bottom bar
- "mobile" could imply physical phone hardware — resolved: use **Layout Mode** when discussing responsive shell structure
- "state" could mean business data or layout state — resolved: use **Workspace UI State** for UI-only state and keep workspace data separate

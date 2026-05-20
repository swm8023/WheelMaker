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

**Turn**:
The smallest ordered chat session item that can be synced, cached, and rendered.
_Avoid_: Message when discussing sync identity, prompt row, update

**Raw Turn**:
The wire/cache/source-store representation of a **Turn**, containing `turnIndex`, `content`, and `finished`, but not `sessionId`.
_Avoid_: Parsed message, render message, session message row

**Turn Index**:
The session-local 1-based position of a **Turn**; `0` is reserved for the empty sync cursor.
_Avoid_: Zero-based turn number, message index, prompt index

**Empty Sync Cursor**:
The cursor value that means no finished **Turn** has been cached yet.
_Avoid_: Turn zero, first turn

**Finished Cursor**:
The largest contiguous **Turn Index** whose turns are finished and durable in the client cache.
_Avoid_: Local latest turn, visible cursor, live cursor

**Live Turn Buffer**:
The in-memory, non-durable **Raw Turns** that can be rendered while streaming or while a read repair is in flight.
_Avoid_: Cache, persisted messages, cursor source

**Finished Store**:
The active-session in-memory store of finished **Raw Turns** used for sync, cache prefix calculation, and display derivation.
_Avoid_: Mixed message store, live message store, React message list

**Durable Turn Cache**:
The IndexedDB cache that stores only raw turns from the contiguous finished prefix for a chat session.
_Avoid_: Sparse cache, full local history, IndexedDB message store

**Cache Reset Boundary**:
The incompatible-schema recovery rule: preserve only token/auth credentials, delete all other local persistent workspace/app cache, and recreate IndexedDB tables from the current schema.
_Avoid_: Partial table migration, chat-only compatibility shim, best-effort old row reuse

**Read Repair**:
The client pull operation that asks `session.read` for the authoritative continuous turn range after a **Finished Cursor**.
_Avoid_: Background merge, prompt refresh, message patch

**Repair Pending Flag**:
The per-session marker that records a missed or out-of-order turn while a **Read Repair** is already in flight.
_Avoid_: Second read, queued request list, sync race flag

**Active Turn Runtime Set**:
The bounded set of chat sessions whose realtime turns are fully consumed into source stores and eligible for read repair.
_Avoid_: Open tabs, all sessions, subscribed sessions

**Session Summary**:
The server-owned projection that drives chat session list status, including running, done, success, and read cursors.
_Avoid_: Local status, inferred session state, optimistic session marker

**Raw Turn Coordinate**:
The use of raw **Turn Index** values as metadata for display items, copy ranges, gap checks, and cursor boundaries.
_Avoid_: Manual render window, array offset, visible message index

**Virtualized Chat View**:
The selected chat display surface built from a full lightweight **Display Index** and mounted through the **Dynamic Turn Virtualizer**.
_Avoid_: Render store, cached visible messages, manual raw turn range

**Display Index**:
The lightweight display projection built from selected-session **Raw Turns** by shallow parsing only enough metadata to decide what can be rendered and how it participates in scrolling.
_Avoid_: Rendered messages, parsed store, virtual DOM cache

**Display Item**:
One lightweight row-like unit in the **Display Index**, identified by a stable key and **Turn Index**, with a render kind and height estimate but no full decoded message body.
_Avoid_: Message object, React node, decoded turn

**Dynamic Turn Virtualizer**:
The chat scrolling engine, implemented with `react-virtuoso`, that owns logical list height, mounts only visible plus overscan **Display Items**, and keeps the browser scrollbar meaningful for long sessions.
_Avoid_: Manual slice, DOM window, fake scrollbar

**Lazy Height Estimate**:
The initial height used for an unmounted **Display Item** before its real DOM height has been measured.
_Avoid_: Final height, pre-rendered height, exact offscreen height

**Virtuoso Measurement Cache**:
The internal `react-virtuoso` measurement state for actual mounted **Display Item** heights, restored or changed only through Virtuoso interfaces rather than an app-owned height store.
_Avoid_: App height cache, persistent layout store, turn cache, markdown cache

**Viewport Anchor**:
The currently stable visible **Display Item** and offset used to keep the user's reading position fixed while the window changes or heights are corrected.
_Avoid_: Sync cursor, scroll position only, latest turn marker

**Tail Lock**:
The display mode entered when the selected chat is at the bottom; new turns and streaming growth keep the viewport attached to the latest **Display Item**.
_Avoid_: Finished cursor, read cursor, session running state

**Gap Turn**:
A non-empty placeholder **Turn** emitted by the server when durable history has a missing turn slot.
_Avoid_: Missing slot, skipped index, empty message

**Hot Gap Turn**:
A **Gap Turn** emitted by the live `session.read` path with method `session/gap`.
_Avoid_: Archive gap, system fallback

**Hub Skill**:
A skill installed in the global skill scope of one WheelMaker Hub machine.
_Avoid_: Repository skill, source skill, global project skill

**Project Skill**:
A skill installed in one Project workspace and shared by that Project's supported agents.
_Avoid_: Local skill when the scope matters, workspace addon

**Skill Managed Project**:
A Project returned by project.list under a WheelMaker Hub and eligible for Project Skill display.
_Avoid_: Arbitrary cwd, manually entered project path

**Skill Source**:
The installable origin that provides one or more skills, such as a GitHub repository, well-known endpoint, or local path.
_Avoid_: Hub, marketplace when referring to a concrete install source

**Remote Skill Source**:
An installable Skill Source accepted by the Skills Page, limited to GitHub owner/repo, GitHub HTTPS repository URL, or a well-known HTTPS skill endpoint.
_Avoid_: Local path, arbitrary git URL, SSH git URL

**Skill Target Agent**:
One of the fixed agent targets WheelMaker supports for skill installation: Codex, Claude Code, OpenCode, or GitHub Copilot.
_Avoid_: Provider when referring to install target, arbitrary agent

**Skill Category**:
The display grouping for an installed skill as reported by the upstream skills list model, derived from the skill source's plugin manifest name when available.
_Avoid_: Path-derived category, agent type

**Skills Page**:
The Settings detail surface that manages Hub Skills and Project Skills across connected WheelMaker Hubs.
_Avoid_: Update page, marketplace page

**Skill Install Entry**:
The Skills Page action that installs selected skills from a Skill Source into either a Hub Skill scope or a Project Skill scope.
_Avoid_: Search marketplace when no source is selected, package update

**Skill Scope Update**:
The Skills Page update action that updates all installed skills in one Hub Skill scope or one Project Skill scope.
_Avoid_: Category update, single skill update

**Skill Inventory Scan**:
The Skills Page action that lists already installed Hub Skills and Project Skills for one Hub.
_Avoid_: Source list, marketplace list

**Skill Source List**:
The Skills Page action that lists installable skills from one Skill Source.
_Avoid_: Installed scan, scope inventory

**Skill Operation**:
A synchronous Skills Page command that completes before the Hub returns its response.
_Avoid_: Background operation, polling job

**Skill Operation Summary**:
The structured result of a Skill Operation, including snapshots or candidates plus a short message or error summary.
_Avoid_: Raw CLI log, stdout page

**Skill Scope Uninstall**:
The Skills Page action that completely removes one installed skill from one Hub Skill scope or one Project Skill scope.
_Avoid_: Agent unlink, partial uninstall

**Symlink Install**:
The default skill installation method that keeps one canonical skill copy and links supported agent skill directories to it.
_Avoid_: Symbol install, copy install

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
- A **Turn Index** starts at `1`; an **Empty Sync Cursor** uses `0` and is not a real **Turn**
- A **Turn** belongs to exactly one chat session and is identified within that session by its **Turn Index**
- A **Raw Turn** does not carry `sessionId`; the enclosing payload or local session key supplies session identity
- A **Finished Cursor** may advance only through contiguous finished cached **Turns**
- A **Live Turn Buffer** may display unfinished or out-of-order **Turns**, but it must not advance a **Finished Cursor**
- Realtime turns may enter the **Live Turn Buffer** before missing earlier turns are repaired
- A **Finished Store** contains only finished **Turns** and replaces any mixed store that combines durable and live turns
- A **Finished Store** may contain finished **Turns** beyond a hole; the **Finished Cursor** is the continuous prefix derived from it
- A **Durable Turn Cache** stores only turns from `1..Finished Cursor`; it must not store sparse finished turns beyond a hole
- A chat render stream is derived by merging the **Finished Store** with the **Live Turn Buffer**
- When the same **Turn Index** exists in both the **Finished Store** and **Live Turn Buffer**, the finished turn is authoritative and the live turn is removed
- If a later authoritative finished/read turn differs from a live turn with the same **Turn Index**, the authoritative turn replaces the live content without client-side text merging
- A **Read Repair** replaces the **Finished Store** range after its requested **Finished Cursor** with the authoritative returned range
- A **Read Repair** clears **Live Turn Buffer** entries covered by the returned range and keeps later live turns for further gap detection
- A chat session may have only one **Read Repair** in flight
- A **Repair Pending Flag** records that another repair check is needed after the current **Read Repair** completes
- An **Active Turn Runtime Set** has a default capacity of five sessions and may become configurable later
- A selected session is always in the **Active Turn Runtime Set** and is never evicted
- Sessions in the **Active Turn Runtime Set** fully consume realtime **Raw Turns**, run **Read Repair**, and write **Durable Turn Cache**
- Sessions outside the **Active Turn Runtime Set** do not parse turn content, do not write turn cache, and do not trigger **Read Repair** from realtime messages
- Evicting a dirty session from the **Active Turn Runtime Set** attempts to flush its **Durable Turn Cache**, but flush failure does not block eviction
- A `session.read` response for this iteration returns the full continuous range after the requested **Finished Cursor**; future pagination may truncate only at a continuous **Turn Index** boundary
- A `session.read` response may include at most one unfinished live turn, and that turn must be the tail
- The server must seal an unfinished text turn as finished before publishing a larger **Turn Index**
- The client routes `finished: true` **Raw Turns** to the **Finished Store** and `finished: false` **Raw Turns** to the **Live Turn Buffer**
- If a continuous `session.read` response ends with an unfinished turn, the **Finished Cursor** advances only through the finished prefix while the unfinished tail remains in the **Live Turn Buffer**
- A `session.read` response may return no turns only when `latestTurnIndex <= afterTurnIndex`
- If `latestTurnIndex` is lower than the client **Finished Cursor**, the client treats local state as stale and starts a full **Read Repair** from the **Empty Sync Cursor**
- A **Session Summary** is the source of truth for session status display
- Client optimistic send state may affect the composer, but it must not become a **Session Summary** substitute
- A realtime `session.message` does not directly update session title, preview, running, done, or read status
- A realtime `prompt_done` may end composer-local activity or trigger `session.markRead`, but it must not directly set session list status without a **Session Summary**
- The selected session calls `session.markRead` with the `prompt_done` **Turn Index** when a visible prompt completes; background sessions do not mark read
- **Raw Turn Coordinates** stay available on **Display Items**, but the client does not use a manual raw turn range as the primary scroll/render window
- A **Virtualized Chat View** owns no durable data; it is rebuilt from source stores and display rules
- A **Virtualized Chat View** may keep lightweight selected-session display state, but it must not drive cache or **Finished Cursor** logic
- A **Display Index** may cover the selected active session's raw source turns, but it stores only lightweight metadata
- A **Display Index** is derived through shallow parsing; full markdown/render decoding is limited to mounted visible plus overscan **Display Items**
- A **Display Item** has a stable key, **Turn Index**, render kind, content revision, and height estimate
- A/B/C option replies and Chinese confirmation replies are affordances on a text **Display Item**, not separate scroll items
- Hidden tool or thought turns should not become visible-height **Display Items**
- A **Gap Turn** becomes a visible **Display Item** so the missing durable slot is explicit in the scroll surface
- The **Dynamic Turn Virtualizer** owns scrollbar height for the selected chat; mounted DOM node count must not define scrollbar semantics
- A **Lazy Height Estimate** may be imprecise until the item is mounted and measured
- The **Virtuoso Measurement Cache** improves scrollbar accuracy over time, but it is not a durable chat cache and must not be duplicated in app state
- A **Viewport Anchor** keeps the reading position stable when windows expand, windows trim, height estimates are corrected, or streaming text grows
- **Tail Lock** keeps the selected chat attached to the latest **Display Item** only while the user is at the bottom
- Outside **Tail Lock**, new turns and streaming growth update source/display data without forcing the viewport to jump
- UI state that must survive virtualizer unmounts is stored by session and **Display Item** key, or derived from raw turns and **Session Summary**
- Scrolling the **Virtualized Chat View** never triggers server read; active sessions already hold their full raw source turns in memory
- Prompt copy ranges are found by raw **Turn Index** boundaries and then filtered by copyable turn type
- A **Gap Turn** preserves **Turn Index** continuity and must be non-empty
- A **Hot Gap Turn** uses method `session/gap` with a reason such as `missing_turn`
- A **Hot Gap Turn** content is `{"method":"session/gap","param":{"reason":"missing_turn","turnIndex":N}}`
- A **Hot Gap Turn** is finished, durable, cacheable, and may advance the **Finished Cursor**
- A **Hot Gap Turn** renders as a lightweight unrecoverable message placeholder and does not participate in prompt copy ranges
- A prompt copy range containing a **Hot Gap Turn** is not copyable
- A `session.read` response must return a continuous **Turn** range after the requested **Finished Cursor**, using **Gap Turns** if durable storage has holes
- A **Hot Gap Turn** is synthesized by the read projection and is not written back to WMT2
- Durable turn writes must reject holes; **Hot Gap Turns** are defensive read recovery, not a normal write strategy
- Each client session keeps exactly one **Finished Cursor**; legacy prompt/index cursor layers are not part of the current chat sync model
- The **Durable Turn Cache** is persisted with a five-second debounce and flushed at critical boundaries such as active-set eviction or page hide
- A schema or shape mismatch crosses the **Cache Reset Boundary**; the client clears all non-token local persistent cache and rebuilds tables instead of migrating old rows
- A client may show turns beyond a local cache hole in the **Live Turn Buffer**, but the **Finished Cursor** remains before the hole until `session.read` repairs it
- If a realtime turn arrives after a gap, the client keeps it in the **Live Turn Buffer** and triggers serialized **Read Repair**
- A **Hub Skill** belongs to exactly one WheelMaker Hub machine, not to every Hub connected through the Registry
- A **Project Skill** belongs to exactly one **Skill Managed Project** and is shown under that Project inside its owning Hub
- A **Skill Managed Project** is selected from Hub-reported project state, never from an App-supplied filesystem path
- A **Skill Source** may provide multiple **Hub Skills** or **Project Skills**
- A **Remote Skill Source** is the only Skill Source accepted by the Skills Page install flow
- WheelMaker installs skills only for the fixed **Skill Target Agents**
- A **Skill Install Entry** targets every fixed **Skill Target Agent** and does not offer per-agent selection
- A **Skill Category** groups installed skills for display but does not identify or scope a skill
- A **Skills Page** shows each connected WheelMaker Hub with its **Hub Skills** first and that Hub's Project Skill sections below
- A **Skills Page** is separate from the Update detail page
- A **Skill Install Entry** must choose exactly one target scope: a Hub for **Hub Skills** or a Project for **Project Skills**
- A **Skill Install Entry** discovers installable skills from one **Skill Source**
- A **Skill Inventory Scan** returns installed Hub Skills and Project Skills, not installable source candidates
- A **Skill Source List** returns installable candidates from one Skill Source, not installed scope state
- A **Skill Operation** completes in the request that starts it and does not require client polling
- A **Skill Operation Summary** is the only command output shown by the App; full stdout and stderr are not part of the product surface
- A **Skill Scope Update** targets an entire Hub Skill scope or Project Skill scope, not a Skill Category
- A **Skill Scope Uninstall** removes a skill from all linked Skill Target Agents in the selected scope
- A **Symlink Install** is preferred for both **Hub Skills** and **Project Skills**

## Example dialogue

> **Dev:** "Should the **Primary Navigation Bubble Group** stay visible when the user switches from Chat to File?"
> **Domain expert:** "Yes — it is the narrow-screen navigation surface, while the **Header Bubble** only carries title and refresh."

> **Dev:** "If a PC browser is narrowed below 900px, should it use the mobile shell?"
> **Domain expert:** "Yes — **Layout Mode** follows viewport width. Input capability is handled separately."

> **Dev:** "Does the first synced chat **Turn** use index 0 or 1?"
> **Domain expert:** "Use **Turn Index** 1 for the first real turn; 0 is only the **Empty Sync Cursor**."

> **Dev:** "Can an unfinished streaming turn move the **Finished Cursor** forward during gap repair?"
> **Domain expert:** "No — keep it in the **Live Turn Buffer** until the server sends it as finished."

> **Dev:** "Can the client keep one mixed local message store for finished and live turns?"
> **Domain expert:** "No — delete the mixed store and derive rendering from **Finished Store** plus **Live Turn Buffer**."

> **Dev:** "Must the **Finished Store** itself be contiguous?"
> **Domain expert:** "No — it may retain finished turns beyond a hole; only the **Finished Cursor** represents the contiguous prefix."

> **Dev:** "Should IndexedDB keep finished turns beyond a local hole?"
> **Domain expert:** "No — the **Durable Turn Cache** keeps only the continuous prefix and reloads the rest from `session.read`."

> **Dev:** "Should read results be merged item-by-item with stale local state?"
> **Domain expert:** "No — a **Read Repair** replaces the range after the requested **Finished Cursor** with the server's continuous result."

> **Dev:** "Can one session run multiple read repairs at the same time?"
> **Domain expert:** "No — run one **Read Repair** at a time and use a **Repair Pending Flag** to trigger a follow-up check."

> **Dev:** "Should every session consume realtime turn content?"
> **Domain expert:** "No — only sessions in the **Active Turn Runtime Set** fully consume **Raw Turns**. Outside sessions wait for activation or summary refresh."

> **Dev:** "Should `session.read` paginate chat turns now?"
> **Domain expert:** "No — this iteration returns the full continuous range after the requested **Finished Cursor**. If pagination is added later, it may only return a continuous prefix."

> **Dev:** "Can `session.read` return an unfinished streaming turn?"
> **Domain expert:** "Yes, but at most one, and it must be the tail. The client advances the **Finished Cursor** only with finished turns."

> **Dev:** "If a read returns turn 3 finished and turn 4 unfinished, where does the cursor land?"
> **Domain expert:** "At turn 3. Turn 4 stays in the **Live Turn Buffer** until a finished version arrives."

> **Dev:** "What if `session.read` returns no turns?"
> **Domain expert:** "That is valid only when the client is caught up or its local cursor is stale and must be reset."

> **Dev:** "Should the client infer session list status from local turns?"
> **Domain expert:** "No — status display trusts the server **Session Summary**."

> **Dev:** "Can a realtime `prompt_done` mark a session completed in the list by itself?"
> **Domain expert:** "No — the list waits for a server **Session Summary**."

> **Dev:** "Should a background session be marked read when its done turn arrives?"
> **Domain expert:** "No — only the selected visible session calls `session.markRead`."

> **Dev:** "Is the rendered turn list a third store?"
> **Domain expert:** "No — it is a **Virtualized Chat View**, derived from source stores for display only."

> **Dev:** "Can **Virtualized Chat View** state live in React?"
> **Domain expert:** "Yes, but only as lightweight selected-session display state, never as cache or cursor source."

> **Dev:** "If a live turn and a finished turn share the same **Turn Index**, do both remain active?"
> **Domain expert:** "No — the **Finished Store** absorbs the authoritative turn and the matching **Live Turn Buffer** entry is deleted."

> **Dev:** "Should the client merge text if read returns different content for a live turn?"
> **Domain expert:** "No — trust the authoritative finished/read turn and replace the live content."

> **Dev:** "Should the source stores keep parsed message objects?"
> **Domain expert:** "No — source stores keep **Raw Turns**. Parsing happens only for display, copy, preview, and selected prompt completion."

> **Dev:** "If tool calls are hidden, should they still take visible scroll height?"
> **Domain expert:** "No — keep raw **Turn Index** metadata for sync/copy boundaries, but hidden turns do not become visible-height **Display Items**."

> **Dev:** "Does scrolling the window change the sync cursor?"
> **Domain expert:** "No — the **Virtualized Chat View** is display-only. The **Finished Cursor** is cache state."

> **Dev:** "Does scrolling up or down trigger `session.read`?"
> **Domain expert:** "No — active sessions already hold full raw source turns in memory. Scrolling only changes the **Dynamic Turn Virtualizer** visible range."

> **Dev:** "Does hiding tool calls change the range copied from a `prompt_done` separator?"
> **Domain expert:** "No — copy range boundaries use raw **Turn Index** values, then copyable turn types are filtered inside that range."

> **Dev:** "Should density be counted by prompt groups?"
> **Domain expert:** "No — count renderable **Turns** so the UI stays turn-first."

> **Dev:** "Does the client need to render all turns to make the right scrollbar accurate?"
> **Domain expert:** "No — the **Dynamic Turn Virtualizer** uses **Lazy Height Estimates** and the **Virtuoso Measurement Cache**. Exact height is measured only for mounted **Display Items**."

> **Dev:** "Can hidden tool calls still take scroll height so raw turn positions stay aligned?"
> **Domain expert:** "No — raw coordinates stay on **Display Items** and copy/cursor logic, but hidden turns do not contribute visible scroll height."

> **Dev:** "If a confirmation card is unmounted by virtualization, can its local component state be the source of truth?"
> **Domain expert:** "No — persistent interaction state must be keyed by session and **Display Item** key, or derived from raw turns and **Session Summary**."

> **Dev:** "When stream text grows while the user is reading older content, should the chat jump?"
> **Domain expert:** "No — preserve the **Viewport Anchor** outside **Tail Lock**. Only **Tail Lock** keeps the view attached to the latest item."

> **Dev:** "If the client cache has turns 1, 2, and 4, can it cache through 4?"
> **Domain expert:** "No — keep turn 4 in the **Live Turn Buffer**, read after **Finished Cursor** 2, and let the server return a continuous range."

> **Dev:** "Can a skipped-ahead realtime turn be displayed before repair completes?"
> **Domain expert:** "Yes — display it from the **Live Turn Buffer**, but do not advance the **Finished Cursor** or durable cache."

> **Dev:** "Should a hot read gap use the archive placeholder method?"
> **Domain expert:** "No — use **Hot Gap Turn** method `session/gap`; `session/archive_gap` stays archive-specific."

> **Dev:** "Can a **Hot Gap Turn** move the **Finished Cursor**?"
> **Domain expert:** "Yes — it is the server's durable placeholder for a confirmed hole."

> **Dev:** "Should a **Hot Gap Turn** look like assistant text?"
> **Domain expert:** "No — render it as an unrecoverable message placeholder and exclude it from prompt copy ranges."

> **Dev:** "Should the server write gap turns into WMT2 during normal append?"
> **Domain expert:** "No — normal writes must be continuous; **Hot Gap Turns** only protect read recovery."

> **Dev:** "Should `session.read` write synthesized **Hot Gap Turns** back into WMT2?"
> **Domain expert:** "No — synthesize them in the read projection only. WMT2 normal writes already guarantee continuous turns."

> **Dev:** "Do we need both a sync index and sub-index cursor in the client?"
> **Domain expert:** "No — the current model has one **Finished Cursor** per chat session."

> **Dev:** "When the Settings page shows skills grouped by Hub, are those skills from a remote skill repository?"
> **Domain expert:** "No — those are **Hub Skills** installed in that WheelMaker Hub's global skill scope; the repository is the **Skill Source**."

> **Dev:** "Can the App ask a remote Hub to install skills from a local path?"
> **Domain expert:** "No — the Skills Page accepts only a **Remote Skill Source**."

> **Dev:** "Can the App pass a custom cwd for project skill installation?"
> **Domain expert:** "No — it selects a **Skill Managed Project**, and the Hub resolves that Project's configured path."

> **Dev:** "Should users choose any agent supported by the upstream skills CLI?"
> **Domain expert:** "No — WheelMaker exposes only the fixed **Skill Target Agents** and defaults to **Symlink Install**."

> **Dev:** "Can users install a skill only for OpenCode from the Skills Page?"
> **Domain expert:** "No — a **Skill Install Entry** installs to every fixed **Skill Target Agent**."

> **Dev:** "Can WheelMaker invent **Skill Categories** from repository folders?"
> **Domain expert:** "No — use the grouping from the upstream skills list model."

> **Dev:** "Is the **Skills Page** only for updating and uninstalling existing skills?"
> **Domain expert:** "No — it also provides a **Skill Install Entry** for adding skills from a selected **Skill Source**."

> **Dev:** "Should skill management be added inside the Update page?"
> **Domain expert:** "No — use a separate **Skills Page** because skills have Hub and Project scopes."

> **Dev:** "Can one list action serve both installed state and source discovery?"
> **Domain expert:** "No — use **Skill Inventory Scan** for installed state and **Skill Source List** for source candidates."

> **Dev:** "Should skill commands return accepted and be polled like npm updates?"
> **Domain expert:** "No — a **Skill Operation** waits for completion and returns the final result directly."

> **Dev:** "Should the Skills Page show the full CLI stdout and stderr after a failure?"
> **Domain expert:** "No — show a **Skill Operation Summary** with a short error summary."

> **Dev:** "Does clicking Update beside a category update only that category?"
> **Domain expert:** "No — **Skill Scope Update** updates every installed skill in that Hub Skill or Project Skill scope."

> **Dev:** "Can users uninstall a skill only for Codex but keep it linked for Claude Code?"
> **Domain expert:** "No — **Skill Scope Uninstall** removes the skill from all linked **Skill Target Agents** in that selected scope."

## Flagged ambiguities

- "expand" was used to mean both opening the drawer and exposing more controls — resolved: use **Drawer Toggle Bubble** for the drawer control
- "3tab button" was used to mean the floating Chat/File/Git control — resolved: use **Primary Navigation Bubble Group**
- "the four buttons" was used to imply independent dragging — resolved: drag the **Floating Control Stack** as one unit
- "floating bubbles" could imply every bubble is draggable — resolved: only the **Floating Control Stack** is draggable; the **Header Bubble** stays fixed
- "snap to the edge" could imply free horizontal dragging — resolved: the **Floating Control Stack** already stays on the right edge; **Edge Snap** governs the allowed resting position after vertical dragging
- "bottom status bar" was first used to mean a File-only surface — resolved: use **Inspector Status Bar** for the shared File/Git bottom bar
- "mobile" could imply physical phone hardware — resolved: use **Layout Mode** when discussing responsive shell structure
- "state" could mean business data or layout state — resolved: use **Workspace UI State** for UI-only state and keep workspace data separate
- "from zero" could imply a real zero-based **Turn Index** — resolved: use `0` only as the **Empty Sync Cursor**, while real turns start at `1`
- "local K" could mean either the latest visible turn or the durable cache cursor — resolved: use **Finished Cursor** for sync repair and keep unfinished turns in the **Live Turn Buffer**
- "window" could mean raw turns, rendered turns, or array offsets — resolved: use **Dynamic Turn Virtualizer** for mounted range and **Raw Turn Coordinate** for turn metadata
- "window end" could mean finished cursor or latest visible turn — resolved: use **Tail Lock** for display follow behavior
- "empty turn" could mean a missing index — resolved: expose a non-empty **Gap Turn** rather than skipping a **Turn Index**
- "sync index" and "sub index" could imply two active cursor layers — resolved: use one **Finished Cursor** per chat session
- "message store" could mean durable cache or live UI state — resolved: use **Finished Store** for durable finished turns and **Live Turn Buffer** for transient turns
- "render turns" could imply another stored copy — resolved: use **Virtualized Chat View** for the derived display surface
- "same turn in live and finished" could imply long-term duplication — resolved: the finished turn is authoritative and deletes the matching live entry
- "message" could mean wire turn or parsed display object — resolved: source stores use **Raw Turns** and display uses decoded messages
- "local cache" could mean all finished turns ever seen — resolved: **Durable Turn Cache** stores only the continuous finished prefix
- "background session" could mean every session still consumes full realtime content — resolved: only the **Active Turn Runtime Set** consumes full **Raw Turns**
- "refresh" could mean list refresh or history repair — resolved: use **Read Repair** for `session.read`-based turn reconciliation
- "retry read" could mean concurrent reads or a serialized follow-up — resolved: use one in-flight **Read Repair** plus a **Repair Pending Flag**
- "session status" could mean local inferred state or server projection — resolved: display status comes from **Session Summary**
- "window" could imply mounted DOM count defines scrollbar height — resolved: the **Dynamic Turn Virtualizer** owns logical scrollbar height from the **Display Index**
- "accurate height" could imply pre-rendering all turns — resolved: use **Lazy Height Estimates** plus **Virtuoso Measurement Cache**
- "special render" could imply React components guess from raw JSON on mount — resolved: classify special UI as **Display Items** during shallow parsing
- "Hub 上的 Skill" could mean skills available from a skill repository — resolved: use **Hub Skill** for globally installed skills on a WheelMaker Hub machine and **Skill Source** for the repository or endpoint they came from
- "Skill Source" could imply local paths or arbitrary git remotes — resolved: the Skills Page install flow accepts only **Remote Skill Sources**
- "Skills in Settings" could mean extending the Update detail — resolved: the **Skills Page** is a separate Settings detail
- "Project" could imply any path typed into the App — resolved: project skill operations target a **Skill Managed Project**
- "固定 agent" could mean a fixed supported set but still selectable per install — resolved: **Skill Install Entry** targets the full fixed set
- "symbol" was used for the default install method — resolved: use **Symlink Install**
- "Skill 分类" could mean a repository folder or metadata tag — resolved: use **Skill Category** from the upstream skills list model
- "Skills 入口" could mean only a read-only inventory page — resolved: the **Skills Page** includes a **Skill Install Entry**
- "list skills" could mean installed inventory or source discovery — resolved: use **Skill Inventory Scan** for installed state and **Skill Source List** for source candidates
- "skill operation" could imply the background polling model used by npm — resolved: **Skill Operation** is synchronous
- "skill command output" could imply full raw CLI logs — resolved: App shows a **Skill Operation Summary**
- "Update skill" could mean updating one skill, one category, or one scope — resolved: use **Skill Scope Update** for the page-level update action
- "Uninstall skill" could mean unlinking one agent target — resolved: use **Skill Scope Uninstall** for complete removal from the selected scope

# Chat Inline Confirmation Replies Design

## Goal

Add a lightweight inline quick-reply affordance for assistant confirmation questions that are not already represented as A/B/C option choices.

When the latest assistant message ends with a Chinese confirmation-style question, the web chat should render a small check frame before that sentence. Clicking either the check frame or the sentence sends the mapped short reply.

## Context

The web chat already supports quick replies for explicit A/B/C option blocks through `app/web/src/chat/chatOptionReplies.ts`. It replaces option lines with inline option buttons only for the latest eligible assistant message.

Recent active WheelMaker sessions show another common pattern:

- `你认这个边界吗？`
- `你同意这个定义吗？`
- `确认这个修正版？`
- `确认这个事件边界？`
- `你接受这个例外吗？`

The user normally replies with short acknowledgements such as `确认`, `接受`, `同意`, or `ok`. The existing A/B/C parser does not detect these confirmation-only questions.

## Confirmed Scope

- Runtime target: `app/web/`.
- Extend the existing chat option/quick-reply parser path rather than adding ad hoc detection in `main.tsx`.
- Only detect Chinese confirmation questions.
- Only detect confirmation questions near the end of the latest assistant message.
- If the latest assistant message has A/B/C option replies, those take priority and no confirmation affordance is shown.
- Historical messages remain normal Markdown with no confirmation frame.
- The check frame contains only a check icon.
- Both the check frame and the matched sentence are clickable.
- Clicking sends a short direct reply through the existing direct-send path and preserves the composer draft and attachments.
- Reply mapping:
  - questions containing `接受` send `接受`
  - questions containing `同意` send `同意`
  - questions containing `确认` or `认` send `确认`

## Non-Goals

- Do not support English confirmation questions in this slice.
- Do not make historical confirmation prompts visibly framed or clickable.
- Do not add a new backend protocol for decision requests.
- Do not change the existing A/B/C option behavior.
- Do not add more quick phrase menu items.
- Do not change composer focus behavior.

## Recognition Rules

The parser should expose a small pure helper for latest-message confirmation detection.

Detection should:

- ignore code fences, matching the current option parser behavior
- inspect only the trailing portion of the assistant text
- require a Chinese question ending, such as `？` or `?`
- prefer the final matching sentence when more than one confirmation sentence appears near the end
- reject messages that already produce A/B/C option replies

Representative positive examples:

- `我的推荐是 B。你认这个边界吗？`
- `确认这个修正版？`
- `确认这个事件边界？`
- `你同意这个定义吗？`
- `你接受这个例外吗？`

Representative negative examples:

- explanatory text that merely mentions `确认` or `接受`
- old A/B/C choice prompts
- code blocks containing confirmation-looking text
- English prompts such as `Does this look right?`

## Rendering

For the latest eligible assistant message:

- split the Markdown into three parts: content before the matched sentence, the matched sentence, and content after it if any
- render the surrounding content as normal Markdown
- render the matched sentence as an inline confirmation row
- show a compact check frame immediately before the sentence
- make both the check frame and the sentence text activate the mapped reply

The visual treatment should be consistent with the current utilitarian chat surface. It should read as an inline action, not a large button or separate choice panel.

## Data Flow

`chatOptionReplies.ts` remains the parser boundary. `main.tsx` asks the parser for:

- A/B/C option parts and selectable replies
- latest-message confirmation reply data when option replies are absent

`ChatTurnView` receives the confirmation data only for the latest selectable assistant message. On click it calls the existing `sendDirectChatText(replyText)` path, preserving current draft and attachments.

## Testing

Add focused tests to the existing option-reply parser test file:

- detects the representative Chinese confirmation sentences and maps replies correctly
- rejects English confirmation sentences
- rejects confirmation-looking text inside code fences
- returns no confirmation reply when A/B/C options are present

Update existing web chat source-structure tests to assert:

- parser helper is imported/used from `main.tsx`
- latest-message confirmation data is only used when option replies are absent
- inline confirmation classes and click handler are present
- historical/static path does not render confirmation affordances

Run targeted parser/UI tests and the web TypeScript check before the final app build.

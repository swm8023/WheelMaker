# Local Hub Read endpoint proof before token authentication

WheelMaker will support default-on **Local Hub Read Acceleration** by letting the App use a same-machine Hub endpoint for eligible File and Git reads while Chat and Session traffic stay on the **Conversation Registry**. Because the Hub reports a dynamic localhost candidate, the App must prove that the WebSocket endpoint matches the Hub-reported **Local Hub Read Candidate** before sending the current Registry token.

We chose an ephemeral signed challenge instead of token-first localhost authentication because `127.0.0.1:<port>` can be occupied by an unrelated local service, especially when the Hub binds a runtime-selected port. The Hub reports a public proof identity with its local read candidate, the App sends a nonce challenge that carries no token, and only after the endpoint proves ownership does the App send `connect.init` with the `local_read` role and the existing Conversation Registry token.

This keeps local File/Git reads opportunistic and safe: a failed proof falls back to Remote reads without exposing credentials, local read candidates never carry secrets, and the normal Workspace UI only shows `Local` or `Remote` while detailed proof or routing failures remain debug information.

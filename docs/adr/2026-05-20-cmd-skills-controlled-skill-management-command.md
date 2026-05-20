# cmd.skills controlled skill management command

WheelMaker will manage agent skills through a new controlled `cmd.skills` command routed from App to Registry to the target Hub by `hubId`. Registry remains a forwarder only, while Hub validates structured actions and executes the upstream `skills` CLI for `scan`, `list`, `install`, `uninstall`, and `update`.

We chose this instead of letting the App parse CLI output or send raw commands because skill management touches Hub-global and Project-scoped filesystem state. The App receives structured **Skill Operation Summary** responses, not full stdout or stderr, and Project-scope operations target Hub-reported Projects rather than arbitrary paths.

`cmd.skills` uses the same Hub-owned operation pattern as `cmd.npm` for write actions because upstream skill install, uninstall, and update can exceed the App request timeout. `scan` and `list` remain synchronous. `install`, `uninstall`, and `update` validate the request, accept one running operation per Hub, and return an operation snapshot immediately; the App refreshes with `scan` until the operation reaches a terminal state. Install sources are limited to remote skill sources, and installation always targets the fixed WheelMaker skill agents with the upstream CLI's default symlink behavior.

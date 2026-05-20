# cmd.skills controlled skill management command

WheelMaker will manage agent skills through a new controlled `cmd.skills` command routed from App to Registry to the target Hub by `hubId`. Registry remains a forwarder only, while Hub validates structured actions and executes the upstream `skills` CLI for `scan`, `list`, `install`, `uninstall`, and `update`.

We chose this instead of letting the App parse CLI output or send raw commands because skill management touches Hub-global and Project-scoped filesystem state. The App receives structured **Skill Operation Summary** responses, not full stdout or stderr, and Project-scope operations target Hub-reported Projects rather than arbitrary paths.

`cmd.skills` intentionally differs from `cmd.npm`: skill operations are expected to be quick enough to run synchronously, so the Hub waits for completion and returns the final result directly instead of accepting a background operation and requiring polling. Install sources are limited to remote skill sources, and installation always targets the fixed WheelMaker skill agents with the upstream CLI's default symlink behavior.

# Issue Tracker: Local Markdown

Issues and PRDs for this repo live as markdown files under `docs/issues/`.

## Conventions

- One feature per directory: `docs/issues/<feature-slug>/`
- The PRD copy or parent reference is `docs/issues/<feature-slug>/PRD.md` when needed.
- Implementation issues live under `docs/issues/<feature-slug>/issues/`.
- Issue files are numbered from `01`, using `NN-<slug>.md`.
- Triage state is recorded as a `Status:` line near the top of each issue file.
- Comments and conversation history append to the bottom of the file under a `## Comments` heading.

## When a skill says "publish to the issue tracker"

Create a new markdown issue file under `docs/issues/<feature-slug>/issues/`, creating the directory if needed.

## When a skill says "fetch the relevant ticket"

Read the referenced markdown file. The user will normally pass the path or issue number directly.

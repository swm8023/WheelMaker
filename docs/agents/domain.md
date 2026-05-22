# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Layout

This is a single-context repo.

- Read `CONTEXT.md` at the repo root for project vocabulary and domain language.
- Read relevant ADRs under `docs/adr/` before proposing changes that touch architecture or durable protocol decisions.

If a file is missing, proceed silently. Do not suggest creating new domain docs upfront; create or update them only when a task resolves new terminology or architectural decisions.

## Use The Glossary's Vocabulary

When output names a domain concept in an issue title, refactor proposal, hypothesis, or test name, use the term as defined in `CONTEXT.md`. Avoid drifting to synonyms the glossary explicitly avoids.

If the concept needed is not in the glossary yet, note the gap for a future domain-doc update instead of inventing new public terminology casually.

## Flag ADR Conflicts

If proposed work contradicts an existing ADR, surface it explicitly rather than silently overriding the decision.

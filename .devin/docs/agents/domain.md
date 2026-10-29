# Domain docs

## Layout: single-context

- Glossary: `GLOSSARY.md` at the repository root.
- Architecture decisions: `docs/adr/` at the repository root.

## Before exploring domain code

Read the glossary and ADRs relevant to the area being explored.

If these files do not exist, proceed silently. Create them lazily
through domain-modeling when terminology or decisions are resolved.

## Use the glossary's vocabulary

Use defined domain terms in issue titles, proposals, hypotheses,
and test names. Follow any explicitly preferred terminology.

If a needed concept is absent, reconsider invented language or
note the gap for domain-modeling.

## Surface ADR conflicts

Explicitly identify proposals that contradict an existing ADR,
including the ADR reference and why reopening it is warranted.

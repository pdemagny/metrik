---
title: Contributing
weight: 50
---

# Contributing

Metrik is developed with AI coding agents, driven by a spec-driven workflow. You are free to
contribute the traditional way, but the conventions below apply either way, and the workflow
explains why the repository contains a `specs/` directory and agent instruction files.

## Development environment

Follow the [getting started](../README.md#getting-started) section. `mise install` also provides
the Spec Kit CLI (`specify`) and `uv`, so there is nothing else to install. Check your changes
with:

```shell
mise run lint   # golangci-lint on every Go module
mise run test   # go test -race on every Go module (needs a C compiler)
```

Both must pass before a change is considered done. New or changed logic in `internal/` and `pkg/`
needs tests, table-driven when inputs vary.

## Conventions

- [Conventional Commits](https://www.conventionalcommits.org): `type(scope): short message`.
- One logical change per commit, with a concise message, authored by the maintainer only (no
  co-author trailers, including for agent-written code).
- Stage only the files related to your change.
- Keep the README and these docs accurate when a change alters setup, configuration or usage.
- Pin tool versions in `.mise.toml`.

The project principles (simplicity, configuration over code, observability, bounded checking,
declarative delivery) are written down in
[`.specify/memory/constitution.md`](https://github.com/pdemagny/metrik/blob/main/.specify/memory/constitution.md).
Every change, human or AI-written, is expected to respect them.

## AI-assisted development

### Principles

- **The spec comes first.** Agents are most reliable when they implement a reviewed specification
  instead of a one-line prompt. The artifacts under `specs/` are the shared context between you
  and the agent.
- **The constitution is the guardrail.** Agents must check their plans against it, and violations
  have to be justified in writing.
- **The human reviews every step.** Agents never commit or push on their own: they code, then stop
  for review. Read each generated artifact before moving on to the next stage.
- **Instructions live in the repository.** Rules for agents are in [`AGENTS.md`](https://github.com/pdemagny/metrik/blob/main/AGENTS.md)
  style files, not in anyone's chat history, so every contributor and every tool gets the same
  behavior.

### Harness-agnostic by design

[Spec Kit](https://github.com/github/spec-kit) is an open toolkit that supports many AI coding
agents (Claude Code, GitHub Copilot, Gemini CLI, Codex, Cursor and others). The workflow itself
is a sequence of prompts plus templates and scripts, so it does not depend on one tool. This
repository was set up with Claude Code, and its integration files live in `.claude/skills/`, but
you can use the harness you prefer.

| Part                         | Where                       | Tool-specific?                          |
| :--------------------------- | :-------------------------- | :-------------------------------------- |
| Constitution, templates, scripts | `.specify/`             | No                                      |
| Specs, plans, tasks          | `specs/NNN-feature-name/`   | No                                      |
| Agent rules                  | `AGENTS.md` (+ tool files)  | Mostly: some tools read their own file  |
| Slash commands / skills      | `.claude/skills/speckit-*`  | Yes, generated per agent                |

To work with another agent, list the available integrations and install yours, then commit the
generated files next to the existing ones:

```shell
specify integration list
specify integration install <agent>
```

The commands below are then exposed under the naming your tool uses (for example
`/speckit-specify`, `/speckit.specify`, or a prompt file); the stages and outputs are identical.

### Workflow

Run the stages in order, from the repository root, in your agent of choice. Each one reads the
previous stage's files from `specs/NNN-feature-name/`.

| Stage | Command (Claude Code naming) | Output                         | What you review                                        |
| :---- | :--------------------------- | :----------------------------- | :----------------------------------------------------- |
| 0     | `/speckit-constitution`      | `.specify/memory/constitution.md` | Principles; run once, amend deliberately (`docs(constitution): ...` commits). |
| 1     | `/speckit-specify`           | `spec.md`, feature branch      | What and why only, no implementation detail.           |
| 2     | `/speckit-clarify`           | Updated `spec.md`              | Answers to the agent's questions about ambiguities.    |
| 3     | `/speckit-plan`              | `plan.md`, `research.md`, `data-model.md`, `contracts/` | The design and its Constitution Check. |
| 4     | `/speckit-tasks`             | `tasks.md`                     | Ordered, testable tasks.                               |
| 5     | `/speckit-analyze`           | Report                         | Inconsistencies across spec, plan and tasks, before any code. |
| 6     | `/speckit-implement`         | Code and tests                 | The diff, lint and test results.                       |

Optional stages: `/speckit-checklist` (requirement quality checklist), `/speckit-converge`
(reconcile the spec artifacts with what was actually built) and `/speckit-taskstoissues` (turn
tasks into GitHub issues). The `specs/001-*` to `specs/003-*` directories are complete examples.

### Tips

- Keep each spec small enough for one reviewable change; split big ideas into several features.
- Put decisions in the spec or plan, not only in the chat, so the next session (or another agent)
  starts with the same context.
- If the agent proposes something that conflicts with the constitution, fix the plan rather than
  bending the rule, or amend the constitution on purpose.
- Re-run `analyze` after editing a spec or plan by hand.
- You remain the author: review, run the checks, and commit yourself, one logical change at a time.

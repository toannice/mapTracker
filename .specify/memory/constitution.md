<!--
SYNC IMPACT REPORT
==================
Version change: 1.0.0 → 1.1.0
Status: MINOR — new principle added

Principles added:
  IV.  Phase Test Gate           (new)

Templates updated:
  ✅ .specify/memory/constitution.md       — this file (v1.1.0)
  ✅ .specify/templates/plan-template.md   — Constitution Check gate IV added
  ⚠  .specify/templates/spec-template.md  — no changes required
  ⚠  .specify/templates/tasks-template.md — no changes required

Deferred items:
  None.
-->

# Blind Map Survival Constitution

## Core Principles

### I. Code Cleanliness

No source file MUST exceed 500 lines of code (excluding blank lines and comments).

- When a file approaches 500 lines, it MUST be split into focused, single-responsibility units before new code is added.
- Each file MUST have one clear purpose; organizational-only files with no logic are not permitted.
- Refactoring to meet this limit is not optional — it is required before a feature is considered complete.

### II. Spec-First Development

All implementation MUST conform to `blind_map_survival_tech_spec.docx` as the authoritative source of truth.

- Any deviation from the tech spec — whether a bug fix, feature addition, or architectural change — MUST be presented to the user for approval before work begins.
- Claude MUST NOT implement changes unilaterally; it MUST propose and wait for explicit confirmation.
- Recommendations are welcome, but implementation only follows explicit user approval.

### III. Commit Discipline

Whenever a single work session accumulates more than 100 lines of code changed (added + modified + deleted combined), a git commit MUST be made before continuing.

- Commit messages MUST follow this structure exactly:
  `add|fix|delete: <description of the work>`
- Use `add` for new files or features, `fix` for corrections or adjustments, `delete` for removals.
- One logical unit of work per commit; do not batch unrelated changes.
- Execute via `/speckit-git-commit` or equivalent git commit command.

### IV. Phase Test Gate

Before marking any implementation phase complete and proceeding to the next phase, the corresponding `specs/001-blind-map-survival/test-phase-<N>.md` guide MUST be run in full and all Pass Criteria in that document MUST be met against actual running behavior.

- "Seems complete" is not complete. Code that compiles and exists does not mean it works correctly.
- Each test guide defines exact commands, expected outputs, and "Wrong if" failure modes — these are the definition of done for that phase, not the task checklist.
- A phase is only complete when every item in the guide's Pass Criteria table is verified against a live running system (server, wscat, or Android emulator as appropriate).
- Skipping or abbreviating the test gate is prohibited even under time pressure. If a pass criterion cannot be verified, the phase is not done.
- If a test reveals a defect, the defect MUST be fixed before moving to the next phase — do not carry known failures forward.

## Development Constraints

- Tech stack, architecture, and deployment are defined in `blind_map_survival_tech_spec.docx`. No technology choices outside that document MUST be introduced without user approval (see Principle II).
- The project targets free-tier cloud hosting (Render.com Singapore); implementation choices MUST remain within those resource constraints.
- All game state MUST reside server-side (authoritative server model); clients are display-only.

## Governance

- This constitution supersedes all other development practices for this project.
- Amendments require: (1) proposed change presented to user, (2) explicit user approval, (3) version bump applied per semantic versioning below, (4) CLAUDE.md updated if affected.
- **Version semantics**:
  - MAJOR: Removal or redefinition of an existing principle.
  - MINOR: New principle or section added.
  - PATCH: Wording clarification, non-semantic refinement.
- All implementation plans MUST include a Constitution Check gate before Phase 0 research begins.
- Any PR or task review MUST verify compliance with all three Core Principles before marking complete.

**Version**: 1.1.0 | **Ratified**: 2026-05-16 | **Last Amended**: 2026-05-16

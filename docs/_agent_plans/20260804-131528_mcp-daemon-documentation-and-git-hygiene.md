# Workflow Plan - MCP daemon documentation and Git hygiene

- Task ID: `20260804-131528_mcp-daemon-documentation-and-git-hygiene`
- Mode: `execute`
- Profile: `resumable`
- Status: completed
- Created: `2026-08-04T13:15:28Z`
- Parent plan: not created
- Prompt pack: not created
- Checkpoint: `docs/agents/20260804-131528_mcp-daemon-documentation-and-git-hygiene-state.md`
- Machine state: not created

## Summary and Outcome

- Current repository truth: the outer Git repository contains a Windows-first Go daemon in `alaa-mcp-daemon/`. Product documentation is now clustered into concise source-grounded guides, with a safe add/reload/troubleshoot runbook; the root has public landing and community material; Git text policy, an opt-in hook installer, and a staged-blob normalizer are present. The Windows release gate cannot start through the mandated low-priority runner because that runner fails to launch `pwsh` with `AuthorizationManager check failed`.
- Outcome: an open-source-ready, concise, navigable documentation set with a service-onboarding runbook; contributor/community entrypoints; repository-wide ignore and attribute policy; and a versioned pre-commit normalization hook that stages LF, UTF-8-no-BOM text safely.
- Strategy: retain the existing verified architecture/configuration/lifecycle/validation detail in topic clusters, make `README.md` the entrypoint, add source-grounded community files, apply the user-selected Apache-2.0 license, then validate links, line budgets, hook behavior, and Go gates.

## Scope

- In scope: `README.md` and the existing Markdown guides under `alaa-mcp-daemon/docs/`; a new operations runbook; root open-source contribution/support/behavior guidance and issue templates; root `.gitignore`, `.gitattributes`, a portable hook installer/normalizer, and related documentation.
- Out of scope: daemon behavior, configuration schema semantics, client MCP configuration, Task Scheduler integration execution, commits, pushes, and any change outside this repository.
- Constraints and assumptions: preserve the existing `.gitignore` worktree content; documentation claims must trace to source, schema, examples, or tests; no secret values in examples; Windows remains the production target while Unix validation remains test-only; Apache-2.0 was explicitly selected by the user, while the public repository URL and private reporting route remain human decisions.

## Handoff Package

Knowledge that lives only in the current agent's head and disappears on compaction. Fill a field when something is learned, not on a schedule; leave a field empty rather than padding it. See `references/context-continuity.md`.

- Confirmed facts (verified, each with how it was verified): the Git root is this workspace and the product is nested at `alaa-mcp-daemon/` (`git rev-parse --show-toplevel`, inventory); all tracked text is currently indexed LF, while the PowerShell worktree is CRLF by existing attributes (`git ls-files --eol`); the daemon has no HTTP management API or database (current README, config schema, and control-protocol source exploration); `scripts/verify.ps1` is the Windows release gate and includes race/vulnerability checks (script inspection); the user selected Apache-2.0 for this repository (explicit user instruction, 2026-08-04).
- Open assumptions (believed but unverified, each with what would verify it): the native Windows release script must still be run once the low-priority launcher can start PowerShell. The private vulnerability-reporting route must be enabled on the eventual GitHub repository or replaced with an approved contact address.
- Ruled out (approach, reason, evidence): a monolithic replacement document set; the repo-docs line-budget policy requires preserving dense detail in coherent linked clusters. An HTTP API summary and data-architecture guide are not applicable because the control plane is a local named-pipe protocol and state is local files, not a service API/database.
- Read first on resume (ordered exact paths): `docs/_agent_plans/20260804-131528_mcp-daemon-documentation-and-git-hygiene.md`; `docs/agents/20260804-131528_mcp-daemon-documentation-and-git-hygiene-state.md`; `alaa-mcp-daemon/README.md`; `.gitattributes`; `.gitignore`.
- Environment notes (command shapes that work here, and ones that look right but fail): run Go commands from `alaa-mcp-daemon/`; direct `pwsh -File .\\scripts\\install-git-hooks.ps1` works, but the mandatory `Invoke-AlaaLowPriority.ps1` wrapper fails before spawning any PowerShell child with `AuthorizationManager check failed`, including a no-op `pwsh -NoProfile -NonInteractive -Command exit 0`. The hook fixture passes when executed outside the sandbox; CodeGraph is present at Git root and was used before direct source inspection.
- Traps (looks correct, is not): `.gitattributes` only normalizes text when Git writes the index/worktree; it does not remove an existing BOM from copied working-tree bytes. A hook must modify staged blobs rather than blindly `git add -A`, or it can accidentally stage unrelated edits. The generated Task Scheduler XML deliberately uses a UTF-16LE BOM and is not covered by the repository UTF-8 hook.

## Ordered Work

### Phase 1 - Ground documentation, open-source entrypoints, and Git hygiene

- Status: completed
- Depends on: none
- Owned scope: source-backed documentation, documentation navigation, open-source contribution/support/behavior guidance, root Git text policy, and versioned hook tooling.
- Excluded from this phase: Go behavior and user-owned unrelated changes.
- Work:
  - [ ] Read the named sources and verify current behavior.
  - [ ] Make the smallest in-scope change.
- Acceptance criteria: every narrative guide is green when coherent, source-grounded, reachable from README, and an operator can add/reload/troubleshoot a service without guessing; contributors have clear contribution/security/support paths and structured issue templates; Git policy has explicit LF/BOM behavior and a safe installation command.
- Validation commands: Markdown link/line-budget checker; focused hook fixture checks; `git check-attr`, `git ls-files --eol`, `git diff --check`; `go test ./...`, `go vet ./...`, `go build ./...`.
- Evidence observed: product/docs and community lanes created the clustered documentation, runbook, public landing, contribution material, issue templates, attributes, ignore policy, and hook tooling. The focused hook fixture passed outside the sandbox; the root hook installer set `core.hooksPath=.githooks`.

### Phase 2 - Validate and reconcile

- Status: completed
- Depends on: Phase 1
- Owned scope: documentation/Git hygiene verification and workflow reconciliation.
- Excluded from this phase: scheduler integration and external MCP services.
- Work:
  - [ ] Run the affected validation surface and repair failures.
  - [ ] Reconcile documentation, status, blockers, and remaining work.
- Acceptance criteria: required behavior and evidence agree.
- Validation commands: the Phase 1 commands plus `scripts/verify.ps1` when environment prerequisites exist.
- Evidence observed: Markdown links validated with exit 0 for 22 files; the focused hook fixture passed outside sandbox; `git diff --check` exited 0. After the user selected Apache-2.0, the root `LICENSE` and README link were added; link validation and workflow semantic validation both exited 0. Final reusable-context curation retained the user-scoped Apache-2.0 decision in the handoff package and deliberately did not publish a memory note. The low-priority release runner remains unable to launch PowerShell with `AuthorizationManager check failed`, and scheduler integration remains intentionally unrun; neither is a completion gate because this task changed no Go source, daemon behavior, schema, or runtime configuration.

## Delegation

- Keep shared-context work in the main conversation.
- Independent lane ownership, if admitted: none.
- Dispatches assume zero shared context: copy the relevant handoff-package facts into the dispatch text rather than referring to this conversation.

## Blockers and Next Action

- Blockers: none for this documentation and Git-hygiene goal. Before public release, enable GitHub private vulnerability reporting or provide a contact address; run the native release gate separately once the low-priority runner can launch PowerShell.
- Next action: none for this goal.

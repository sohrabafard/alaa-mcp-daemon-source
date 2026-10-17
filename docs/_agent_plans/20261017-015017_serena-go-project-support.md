# Workflow Plan - Serena Go project support

- Task ID: `20261017-015017_serena-go-project-support`
- Mode: `execution`
- Profile: `direct`
- Task store: `local`
- Task binding: not applicable
- Status: complete
- Created: `2026-10-17T01:50:17Z`
- Parent plan: not created
- Prompt pack: not created
- Checkpoint: not created
- Machine state: not created
- Base branch and commit: master, 2e592c3c50db119e16e5589edcb1fc8f78a47499
- Work branch: master
- Worktree: D:/Sohrab/Project/alaa-mcp-daemon-source

## Summary and Outcome

Enable Serena for alaa-go-chi and this daemon source checkout. Two independent
read-only Luna lanes inspected project bindings and daemon configuration.
The parent applied all edits and reviewed the combined configuration.
The existing alaa-go-chi endpoint incorrectly targeted entitlement-api on 42103.
The repaired endpoints are 42108 (kit) and 42109 (daemon source).

## Scope

Client JSON/TOML, Serena Go workspace configuration, and ignored local daemon
configuration only. No application code, dependencies, installation or commits.
Both checkouts were clean before work. Local runtime config was backed up beside
the original as alaa-mcp-daemon.json.backup-serena-go-20261017.

## Skill Bindings

| Skill | Source | Load before | When | If unavailable |
|---|---|---|---|---|
| alaa-code-intelligence-routing | C:/Users/Alaa/.codex/skills/alaa-code-intelligence-routing/SKILL.md | discovery | config and proof selection | report gap |
| alaa-codex-runtime-ops | C:/Users/Alaa/.codex/skills/alaa-codex-runtime-ops/SKILL.md | runtime recovery | sandbox, PATH, named pipe | report blocker |
| alaa-workflow | C:/Users/Alaa/.codex/skills/alaa-workflow/SKILL.md | edits | evidence coordination | report gap |

## Ordered Work

### Phase 1 - Inspect, configure and verify

- Status: complete
- Depends on: none
- Required skills: alaa-code-intelligence-routing, alaa-codex-runtime-ops, alaa-workflow
- Reasoning: bounded configuration repair; two independent read-only scopes.
- Ownership: parent owns all writes; Luna lanes own only their inspections.
- Owned scope: both project MCP bindings, daemon source Serena project config,
  and ignored daemon runtime configuration.
- Excluded from this phase: application code, dependencies, unrelated services.
- Validation commands: daemon validate; tmp/verify-serena-setup.py;
  tmp/smoke-serena-mcp.py; git diff --check in both repositories.
- Evidence observed: passing configuration and MCP symbol proof described below.
- Work:
  - [x] T1: Inspect both checkouts and resolve port ownership. [depends: none] [skills: inherit]
  - [x] T2: Apply matching client, project and daemon settings. [depends: T1] [skills: inherit]
  - [x] T3: Validate and perform MCP semantic smoke checks. [depends: T2] [skills: inherit]
- Acceptance: both endpoints initialize and return actual Go symbols.
- Evidence: daemon validate passed with 10 services. Installed Serena schema,
  JSON/TOML endpoint agreement and original-service preservation passed using
  tmp/verify-serena-setup.py. Both repository diff whitespace checks passed.
- Runtime: authorized hidden daemon process 36768; both new services ready.
  tmp/smoke-serena-mcp.py passed initialize, tools/list (23 tools per endpoint),
  and get_symbols_overview for idkit/codec.go and
  alaa-mcp-daemon/internal/app/app.go. These prove actual Go LSP operation.
- Recovery: CIM and named-pipe reads were denied inside sandbox and passed with
  escalation. First MCP probe was refused before serialized startup completed;
  retry after observed ready passed. No application fix was needed.
- Parser verification initially bypassed default expansion; corrected to use
  the installed YAML loader before parsing. Serena expanded its project defaults
  during startup using CRLF; normalized the generated configuration to LF.
- Unrelated runtime issue: existing Sentry service exits 1 and entered backoff.
  No Sentry configuration was changed or troubleshooting attempted.
- Final reusable-context scan: no admitted candidates; repository config and
  this evidence already own the facts. No memory writes.
- Snapshot: HEAD 2e592c3c50db119e16e5589edcb1fc8f78a47499; scoped path/content
  SHA-256 e74a41b3a281d6498afbf00606bdca76f07a3f0504b8dc66faa411cbb01d800a.
  Method: sorted absolute paths and SHA-256 of each file, newline-joined and
  hashed, implemented in tmp/verify-serena-setup.py. Scope: both client JSON,
  client TOML, Serena project YAML files and ignored daemon runtime JSON.
  Recovery is uncommitted local files, plus ignored runtime config backup.

## Task Allocation

User explicitly selected Luna for both inspections: inspect_go_chi and
inspect_daemon dispatched as explorer / gpt-6-luna, runtime-default effort,
separate context, read-only authority. Underlying identity is not observable.
No additional agents were dispatched. Shared writes were serialized by parent.

## Handoff Package

- Confirmed facts: installed Serena and gopls work on both named Go workspaces.
- Open assumptions: a newly opened Codex client will load its project MCP binding;
  the current session tool inventory does not hot-reload this change.
- Ruled out: reusing 42103 (entitlement-api) or 42107 (Sentry).
- Read first on resume: .codex/config.toml, .mcp.json, .serena/project.yml,
  alaa-mcp-daemon/dist/alaa-mcp-daemon.json.
- Environment notes: installed Serena uses absolute local-bin launcher; daemon
  named-pipe status requires outside-sandbox access in this runtime.
- Traps: TCP ready alone is weaker than the completed MCP symbol smoke check.

## Blockers and Next Action

- Current phase: complete
- Blockers: none for Serena. Sentry issue remains outside scope.
- Next action: reopen the Codex project/session to load the configured tools.
- Lifecycle: IMPLEMENTED proven; MERGE_CANDIDATE not assessed (no integration
  requested); RELEASE_CANDIDATE and PUBLISHED not requested.

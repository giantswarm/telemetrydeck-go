<!-- BEGIN devctl-managed: edits in this block are overwritten -->
<!-- markdownlint-disable-file MD041 -->

## Giant Swarm conventions

devctl maintains this block from [its template][devctl-agents-block] and
overwrites changes to it. Instructions for this repository go in the section
below. Where they differ from this block, they take precedence.

[devctl-agents-block]: https://github.com/giantswarm/devctl/blob/main/pkg/gen/input/llm/internal/file/agents_block.md.template

- Never put customer names or other customer information in code, commit
  messages, issues or pull requests. Most Giant Swarm repositories are public,
  and private ones are kept ready to become public.
- Giant Swarm's style guides are in <https://github.com/giantswarm/fmt>, one
  directory per topic (go, helm, kubernetes, prometheus, …). Read the relevant
  guide before you write or review that kind of code.
- Use conventional commits for commit messages and pull request titles.
- If `.github/workflows/zz_generated.auto_release.yaml` exists, GitHub Releases
  are written from pull request titles: do not edit CHANGELOG.md. Otherwise,
  add your change to CHANGELOG.md as described in
  <https://github.com/giantswarm/fmt/blob/main/releases/changelog.md>.
- If `.pre-commit-config.yaml` exists, run
  `pre-commit run --files <changed files>` before committing; not every clone
  has the hooks installed.
- Do not add linters, formatters or other tooling unless asked; use what the
  repository already has.
- Put documentation in `docs/` rather than growing README.md, which gives an
  overview and links into `docs/`. When a change affects documented behaviour,
  update the documentation in the same pull request.
- If the skills of the `giantswarm/claude-code` plugin marketplace are not
  available to you and the user works at Giant Swarm, tell them once that
  they can install the `gs-base`, `gs-dev` and `gs-godev`
  plugins from it.
- Go: run `goimports -local github.com/giantswarm/<repository>` on the files
  you change. CI with architect-orb fails when imports are grouped
  differently.

<!-- END devctl-managed -->

# telemetrydeck-go

Go SDK for TelemetryDeck's Ingest API v2, one package in `telemetrydeck.go`.
Giant Swarm CLIs (kubectl-gs, agentlab) send one `GiantSwarm.command` signal
per command through it. Owned by Team Bumblebee; `docs/development.md` has the
build, test and release details.

## Invariants

- The `TelemetryDeck.*` parameter names are TelemetryDeck's reserved namespace
  and the dashboard's standard insights read them. Add to them from
  TelemetryDeck's documentation; never rename or repurpose one.
- The user identifier leaves the machine only as a salted SHA-256 hash, and
  `generateUserId` stays deterministic for one machine and OS user.
- `SendSignal` returns immediately and never returns a delivery error (errors go
  to the optional logger). `Flush(ctx)` and `SendSignalSync(ctx, …)` are the
  bounded waits: they return `ctx.Err()` when the deadline comes first and
  never hold a caller past it; the default HTTP client times out on its own.
  Changing any of these contracts is a `feat!:`.
- Consumers keep the legacy `appVersion` payload key; the usage reports query it.
- Dependencies are the standard library and `google/uuid`. A new one needs a
  reason in the PR.

## Working here

- `make test` is what CI runs; the tests use `httptest`. Every new injected
  parameter gets a test (`telemetrydeck_appinfo_test.go` is the pattern).
- Conventional commits. Every merge to `main` is released automatically
  (git-cliff), so the PR title is the release note; `docs:` releases nothing.
- Generated files (`Makefile*`, `.circleci/`, `.github/workflows/zz_generated.*`,
  `cliff.toml`, `renovate.json5`, `.nancy-ignore.generated`,
  `.pre-commit-config.yaml`, `.cursor/rules/zz_generated.*`) come from devctl
  through giantswarm/github's align-files workflow. Do not edit them here.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).

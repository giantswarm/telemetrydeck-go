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
  to the optional logger). Changing that contract is a `feat!:`; a bounded wait
  is an addition (#122).
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

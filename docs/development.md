# Developing on telemetrydeck-go

The library is one package in `telemetrydeck.go`. The tests next to it run
against an `httptest` server; nothing reaches TelemetryDeck.

## Build and test

- `make test` is what CI runs: `go test ./...`, with the race detector when a
  C toolchain is available.
- `pre-commit run --all-files` runs gofmt, go-mod-tidy, golangci-lint (with
  gosec, goconst and govet) and goimports with
  `-local github.com/giantswarm/telemetrydeck-go`, the same hooks as the
  `pre-commit` check on a pull request.
- Every parameter the client injects has a test; `telemetrydeck_appinfo_test.go`
  is the pattern: decode the request body the test server received and assert
  on the payload keys.

## Trying a change against the real ingest endpoint

Use test mode and a logger, so the signals are filed as test data and delivery
errors are printed. Use a test app, never a production tool's app ID.

```go
client, _ := telemetrydeck.NewClient(appID,
    telemetrydeck.WithTestMode(),
    telemetrydeck.WithLogger(log.New(os.Stderr, "telemetry: ", 0)),
    telemetrydeck.WithAppVersion("dev"),
)
_ = client.SendSignal(ctx, "GiantSwarm.command", map[string]interface{}{"command": "probe"})
time.Sleep(2 * time.Second) // SendSignal returns before the request is sent (#122)
```

`https://nom.telemetrydeck.com/v2/` answers `200 OK` to an accepted batch. The
dashboard shows the signal only with its "Test Mode" toggle on, and ingestion
is queued, so allow a few minutes.

## Generated files

`Makefile`, `Makefile.gen.go.mk`, `.circleci/config.yml` and
`.circleci/workflows.yml`, `.github/workflows/zz_generated.*`, `cliff.toml`,
`renovate.json5`, `.nancy-ignore.generated`, `.pre-commit-config.yaml` and
`.cursor/rules/zz_generated.*` are rendered by
[devctl](https://github.com/giantswarm/devctl) and kept current by the
align-files workflow of [giantswarm/github](https://github.com/giantswarm/github),
from this repository's entry in `repositories/team-bumblebee.yaml`
(`componentType: library`, flavour `generic`, language `go`,
`ci.generate: true`). Change the generator or the entry, not the rendered
file: an edit here is overwritten by the next alignment PR. Repo-owned files
next to them are `.github/workflows/go-coverage.yaml`, `.nancy-ignore` and an
optional `.circleci/custom.yml` for extra CircleCI jobs.

Renovate extends the `default` and `lang-go` presets of
[renovate-presets](https://github.com/giantswarm/renovate-presets); the
Dependency Dashboard is issue #1.

## Releases

Every push to `main` runs `zz_generated.auto_release.yaml`: git-cliff reads the
conventional commits since the latest tag and, when they warrant a bump,
creates the tag and the GitHub release with generated notes. `fix:`, `chore:`,
`ci:`, `refactor:` and `test:` bump the patch version, `feat:` the minor,
`feat!:` or a `BREAKING CHANGE` footer the major; `docs:` and `style:` release
nothing. There is no release PR and no manual step, so a PR title is a release
note. CircleCI runs `go-build` on the tag; a library publishes nothing beyond
the tag, and consumers pick the version up through Renovate.

## Consumers

- [kubectl-gs](https://github.com/giantswarm/kubectl-gs), `cmd/root.go` (Team Honeybadger)
- [agentlab](https://github.com/giantswarm/agentlab), `internal/telemetry` (Team Bumblebee)

An API change is followed by a bump PR in both.

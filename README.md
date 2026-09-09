[![Go Reference](https://pkg.go.dev/badge/github.com/giantswarm/telemetrydeck-go.svg)](https://pkg.go.dev/github.com/giantswarm/telemetrydeck-go)

# TelemetryDeck SDK for Go

Sends anonymous usage signals to [TelemetryDeck](https://telemetrydeck.com/)
through the [Ingest API v2](https://telemetrydeck.com/docs/ingest/v2/). Giant
Swarm's command-line tools use it to report one signal per command:
[kubectl-gs](https://github.com/giantswarm/kubectl-gs) since 4.3.0 and
[agentlab](https://github.com/giantswarm/agentlab).

Maintained by Team Bumblebee. The API is pre-1.0: additions ship as minor
releases, and a breaking change is a `feat!:` commit that bumps the major
version. Every merge to `main` is released automatically from its conventional
commits, so the [releases page](https://github.com/giantswarm/telemetrydeck-go/releases)
is the changelog.

## Usage

```go
client, err := telemetrydeck.NewClient(appID,
    telemetrydeck.WithAppVersion(version),   // TelemetryDeck.AppInfo.version
    telemetrydeck.WithBuildNumber(gitSHA),   // TelemetryDeck.AppInfo.buildNumber (optional)
)
if err != nil {
    return err
}
err = client.SendSignal(ctx, "MyNamespace.command", map[string]interface{}{
    "command": cmd.CommandPath(),
})
```

Every signal carries the operating system, the architecture and this SDK's
name and version (`TelemetryDeck.Device.operatingSystem`,
`TelemetryDeck.Device.architecture`, `TelemetryDeck.SDK.nameAndVersion`) next
to your payload. Pass the application's version with `WithAppVersion` so it is
sent as `TelemetryDeck.AppInfo.version`: that is the parameter the TelemetryDeck
dashboard's standard "App Versions" insight breaks down by; a version kept only
in a custom payload key (for example the legacy `appVersion`) does not show up
there. `WithBuildNumber` fills `TelemetryDeck.AppInfo.buildNumber` (the
insight's "Builds" view) and, together with the version,
`TelemetryDeck.AppInfo.versionAndBuildNumber`.

Further options: `WithUserID` and `WithHashSalt` (the user identifier is
hashed before it is sent; without `WithUserID` one is derived from the
machine), `WithSessionID`, `WithTestMode` (marks the signals as test data and
logs delivery errors), `WithLogger`, `WithEndpoint`.

`SendSignal` returns after handing the request to a goroutine; delivery
errors are logged, never returned.

## Conventions for Giant Swarm CLIs

The consumers share one shape, so the usage reports and the dashboard treat
every tool alike. A new CLI copies it:

- **One signal per user-facing command**, sent from cobra's root
  `PersistentPreRun`. Skip hidden commands, `help` and `completion` (agentlab
  does; kubectl-gs predates the rule).
- **Signal type `GiantSwarm.command`** with the payload keys `command`
  (`cmd.CommandPath()`) and `appVersion` (the tool's version). Keep the
  `appVersion` payload key even though `WithAppVersion` carries the same value:
  the usage reports query the payload key, the dashboard's insights read
  `TelemetryDeck.AppInfo.version`.
- **One TelemetryDeck app per tool.** Never send a tool's signals under another
  tool's app ID. The app ID is not a secret and lives in the source.
- **Opt-out** through `<TOOL>_TELEMETRY_OPTOUT` (kubectl-gs:
  `KUBECTL_GS_TELEMETRY_OPTOUT`, agentlab: `AGENTLAB_TELEMETRY_OPTOUT`) and the
  cross-tool [`DO_NOT_TRACK`](https://consoledonottrack.com/) variable, both
  documented where the tool is documented.
- **Test mode for development** through `<TOOL>_TELEMETRY_TESTMODE=1`, which
  turns on `WithTestMode()` and a `WithLogger` to stderr: the signals are filed
  as test data and delivery errors become visible.
- A command that exits within milliseconds can lose its signal, because
  `SendSignal` does not wait for delivery. A bounded wait is tracked in
  [#122](https://github.com/giantswarm/telemetrydeck-go/issues/122).

## Development

See [docs/development.md](docs/development.md) for building and testing, the
generated files, and the release flow.

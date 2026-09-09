[![Go Reference](https://pkg.go.dev/badge/github.com/giantswarm/telemetrydeck-go.svg)](https://pkg.go.dev/github.com/giantswarm/telemetrydeck-go)

# TelemetryDeck SDK for Go

**The development on this project has just started. Please don't expect anything to work until there are a few releases. Also please expect breaking API changes in every release until v1.0.0 is reached.**

The Goal of this library is to facilitate sending telemetry data to the [TelemetryDeck](https://telemetrydeck.com/) backend via the [Ingest API v2](https://telemetrydeck.com/docs/ingest/v2/).

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
dashboard's standard "App Versions" insight breaks down by — a version kept only
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

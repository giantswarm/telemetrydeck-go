// Package telemetrydeck sends anonymous usage signals to TelemetryDeck
// through its Ingest API v2.
//
// A Client stands for one application (the app ID) used by one user in one
// session. SendSignal hands a signal to a goroutine and returns at once, so
// delivery overlaps the program's own work; Flush bounds the wait for that
// delivery before the process exits, and SendSignalSync sends in the caller's
// goroutine instead:
//
//	client, err := telemetrydeck.NewClient(appID,
//	    telemetrydeck.WithAppVersion(version),
//	)
//	if err != nil {
//	    return err
//	}
//	_ = client.SendSignal(ctx, "MyNamespace.command", map[string]interface{}{
//	    "command": "create",
//	})
//	// ... the command's own work ...
//	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
//	defer cancel()
//	_ = client.Flush(ctx)
//
// The user identifier leaves the machine only as a salted SHA-256 hash, see
// WithUserID and WithHashSalt.
package telemetrydeck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// The TelemetryDeck Ingest v2 API endpoint we use
	endpoint = "https://nom.telemetrydeck.com/v2/"

	// defaultTimeout bounds one request of the default HTTP client: connecting,
	// sending and reading the response. A hanging network therefore never holds
	// a send, and with it a Flush or a SendSignalSync, longer than this.
	defaultTimeout = 5 * time.Second

	// modulePath is this library's module path, looked up in the consuming
	// binary's build info to report which version of the SDK sent a signal.
	modulePath = "github.com/giantswarm/telemetrydeck-go"

	// Parameter names in TelemetryDeck's reserved namespace. The SDKs set
	// these on every signal, and the dashboard's standard insights read them:
	// the Overview's "App Versions" chart breaks down by AppInfo.version,
	// its "Builds" view by AppInfo.buildNumber. A custom payload key such
	// as the legacy "appVersion" is not picked up by those insights.
	paramOperatingSystem       = "TelemetryDeck.Device.operatingSystem"
	paramArchitecture          = "TelemetryDeck.Device.architecture"
	paramSDKNameAndVersion     = "TelemetryDeck.SDK.nameAndVersion"
	paramAppVersion            = "TelemetryDeck.AppInfo.version"
	paramBuildNumber           = "TelemetryDeck.AppInfo.buildNumber"
	paramVersionAndBuildNumber = "TelemetryDeck.AppInfo.versionAndBuildNumber"
)

var (
	ErrNoAppID      = errors.New("no app ID specified")
	ErrNoSignalType = errors.New("no signal type specified")
)

// Client represents a TelemetryDeck client, configured to represent
// one distinct user interacting with one distinct application.
type Client struct {
	// The HTTP client we use to submit our data to the TelemetryDeck API.
	httpClient *http.Client

	// Logger used to log errors.
	logger *log.Logger

	appID       string
	endpoint    string
	hashSalt    string
	userID      string
	userIDHash  string
	sessionID   string
	testMode    bool
	appVersion  string
	buildNumber string
	sdkVersion  string

	// The sends SendSignal has in flight, for Flush: their number and a
	// channel that is closed when it drops back to zero.
	mu       sync.Mutex
	inflight int
	idle     chan struct{}
}

type SignalBody struct {
	AppID      string                 `json:"appID"`
	ClientUser string                 `json:"clientUser"`
	SessionID  string                 `json:"sessionID"`
	IsTestMode bool                   `json:"isTestMode"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
}

// NewClient instantiates a new client to send data to TelemetryDeck, and
// also starts a new session. The appID is the only required parameter.
// Any number of optional parameters can be passed using the With...() functions.
//
// The client's HTTP requests time out after five seconds, so a program that
// waits for delivery (Flush, SendSignalSync) is never held longer than that
// by an unreachable network.
func NewClient(appID string, options ...func(*Client)) (*Client, error) {
	if appID == "" {
		return nil, ErrNoAppID
	}

	// Create client with defaults
	defaultUid := generateUserId()
	client := &Client{
		appID:      appID,
		endpoint:   endpoint,
		sessionID:  uuid.New().String(),
		userID:     defaultUid,
		userIDHash: hashUserId(defaultUid, ""),
		httpClient: &http.Client{Timeout: defaultTimeout},
		sdkVersion: sdkNameAndVersion(),
	}

	// Apply options overriding defaults
	for _, o := range options {
		o(client)
	}

	return client, nil
}

// WithEndpoint allows to specify an alternative API endpoint.
// This is mainly useful for testing. To be used as an option
// parameter in the NewClient() func.
func WithEndpoint(endpoint string) func(*Client) {
	return func(c *Client) {
		c.endpoint = endpoint
	}
}

// WithLogger specifies a logger to use for logging errors
// caught during sending telemetry signals. If not given,
// these errors will be ignored.
//
// To be used as an option parameter in the NewClient() func.
func WithLogger(logger *log.Logger) func(*Client) {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithHashSalt specifies a hash salt string (recommended).
//
// This salt will be appended to the user identifier before it
// gets hashed and submitted to TelemetryDeck. This makes it
// a lot harder to de-anonymize user ID hashes, e.g. via some
// rainbow tables.
//
// To be used as an option parameter in the NewClient() func.
func WithHashSalt(salt string) func(*Client) {
	return func(c *Client) {
		c.hashSalt = salt

		// Re-hash the user ID with the new salt
		c.userIDHash = hashUserId(c.userID, c.hashSalt)
	}
}

// WithUserID specifies a unique user identifier.
//
// The identifier will be salted and hashed before
// submitting to the TelemetryDeck API.
//
// If no unique user ID is specific, an identifier is generated.
//
// To be used as an option parameter in the NewClient() func.
func WithUserID(userID string) func(*Client) {
	return func(c *Client) {
		c.userID = userID
		c.userIDHash = hashUserId(userID, c.hashSalt)
	}
}

// WithSessionID specifies a session identifier. This should be the same value for
// the same session/user combination. If not given, a UUID will be
// generated at the creation of the client.
//
// To be used as an option parameter in the NewClient() func.
func WithSessionID(sessionID string) func(*Client) {
	return func(c *Client) {
		c.sessionID = sessionID
	}
}

// WithAppVersion records the version of the application that sends the
// signals, e.g. "1.4.2" or "v0.23.3". It is sent with every signal as
// TelemetryDeck.AppInfo.version, the parameter the TelemetryDeck SDKs set and
// the dashboard's standard "App Versions" insight breaks down by. A version
// carried only in a custom payload key (such as "appVersion") does not show
// up there. An empty version sends nothing.
//
// To be used as an option parameter in the NewClient() func.
func WithAppVersion(version string) func(*Client) {
	return func(c *Client) {
		c.appVersion = version
	}
}

// WithBuildNumber records the build identifier of the application, e.g. a
// CI build number or the git commit. It is sent with every signal as
// TelemetryDeck.AppInfo.buildNumber (the "Builds" view of the dashboard's
// "App Versions" insight); together with WithAppVersion it also fills
// TelemetryDeck.AppInfo.versionAndBuildNumber ("<version> <build>"). An empty
// build number sends nothing.
//
// To be used as an option parameter in the NewClient() func.
func WithBuildNumber(buildNumber string) func(*Client) {
	return func(c *Client) {
		c.buildNumber = buildNumber
	}
}

// WithTestMode activates test mode.
//
// When set, data will be sent with isTestMode=true, to avoid polluting
// production data. Also, a signal the endpoint rejects is logged together
// with its request and response bodies (given a WithLogger), which the
// production mode keeps to the one error line.
//
// To be used as an option parameter in the NewClient() func.
func WithTestMode() func(*Client) {
	return func(c *Client) {
		c.testMode = true
	}
}

// Returns a SHA256 hash of the provided user ID, with the salt
// applied before hashing.
func hashUserId(id, salt string) string {
	h := sha256.New()
	h.Write([]byte(id + salt))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)
}

// Returns a pseudo-unique user identifier based on machine, OS
// and OS user details.
func generateUserId() (id string) {
	// OS and architecture
	id += "|" + runtime.GOOS
	id += "|" + runtime.GOARCH

	// Host name
	hostname, err := os.Hostname()
	if err == nil {
		id += "|" + hostname
	}

	// MAC addresses
	{
		ifas, err := net.Interfaces()
		if err == nil {
			var as []string
			for _, ifa := range ifas {
				a := ifa.HardwareAddr.String()
				if a != "" {
					as = append(as, a)
				}
			}
			sort.Strings(as)
			id += fmt.Sprintf("|%s", strings.Join(as, " "))
		}
	}

	// User ID (won't work on Windows)
	id += fmt.Sprintf("|%d", os.Getuid())

	// Group ID (won't work on Windows)
	id += fmt.Sprintf("|%d", os.Getgid())

	// User name
	id += fmt.Sprintf("|%s|%s|%s", os.Getenv("USER"), os.Getenv("USERNAME"), os.Getenv("%USERNAME%"))

	return id
}

// SendSignal sends a signal to the TelemetryDeck backend.
//
// The signalType is a string of your choice, identifying the type of the signal
// you are sending, e.g. "command". From the TelemetryDeck docs:
//
// "While it is not enforced, we recommend structuring your signal names in
// namespaces separated by dots, with the signal type beginning with a lower
// case letter and any namespaced beginning with an uppercase letter."
//
// The payload is a map of key-value pairs, containing the data you want to send.
//
// The request is sent from a goroutine and carries ctx: SendSignal returns as
// soon as the request is built, and cancelling ctx abandons the delivery.
// Errors that occur during submission of the request to TelemetryDeck are not
// returned. Instead they are printed if the client has been configured with a
// logger (see WithLogger). Call Flush before the process exits to give the
// delivery a bounded amount of time to finish.
func (c *Client) SendSignal(ctx context.Context, signalType string, payload map[string]interface{}) error {
	request, body, err := c.newRequest(ctx, signalType, payload)
	if err != nil {
		return err
	}

	done := c.track()
	go func() {
		defer done()
		if err := c.deliver(request, body); err != nil {
			c.logf("error submitting %s: %s", signalType, err)
		}
	}()

	return nil
}

// SendSignalSync sends a signal like SendSignal, but in the caller's
// goroutine: it returns once TelemetryDeck has accepted the signal, or with
// the error that prevented that, be it a transport failure, a rejecting
// status, or ctx being done. A deadline on ctx bounds the wait. Use it where
// one round trip at the end is affordable; SendSignal followed by Flush lets
// the delivery overlap the program's own work instead.
func (c *Client) SendSignalSync(ctx context.Context, signalType string, payload map[string]interface{}) error {
	request, body, err := c.newRequest(ctx, signalType, payload)
	if err != nil {
		return err
	}
	return c.deliver(request, body)
}

// Flush waits until every signal SendSignal has handed to the network so far
// has been delivered or has failed, or until ctx is done, whichever comes
// first, and returns ctx.Err() in the latter case. A short-lived program
// calls it before exiting, with a deadline that caps how long the exit may
// take; a send still pending then continues in the background until the
// process ends or the HTTP client's timeout strikes.
func (c *Client) Flush(ctx context.Context) error {
	c.mu.Lock()
	idle, pending := c.idle, c.inflight > 0
	c.mu.Unlock()
	if !pending {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// track counts one send as in flight and returns the function that marks it
// done; the idle channel is renewed when the count leaves zero and closed
// when it returns there, so Flush can wait on it.
func (c *Client) track() (done func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight == 0 {
		c.idle = make(chan struct{})
	}
	c.inflight++
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.inflight--
		if c.inflight == 0 {
			close(c.idle)
		}
	}
}

// newRequest builds the ingest request for one signal: the standard
// parameters are injected into the payload, the signal is wrapped in the
// array body the API expects, and the request carries ctx. The encoded body
// is returned as well, for the diagnostics of a rejected signal.
func (c *Client) newRequest(ctx context.Context, signalType string, payload map[string]interface{}) (*http.Request, []byte, error) {
	if signalType == "" {
		return nil, nil, ErrNoSignalType
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if payload == nil {
		payload = make(map[string]interface{})
	}
	// Inject standard fields into the payload
	payload[paramOperatingSystem] = runtime.GOOS
	payload[paramArchitecture] = runtime.GOARCH
	payload[paramSDKNameAndVersion] = c.sdkVersion
	if c.appVersion != "" {
		payload[paramAppVersion] = c.appVersion
	}
	if c.buildNumber != "" {
		payload[paramBuildNumber] = c.buildNumber
	}
	if c.appVersion != "" && c.buildNumber != "" {
		payload[paramVersionAndBuildNumber] = c.appVersion + " " + c.buildNumber
	}

	signal := SignalBody{
		AppID:      c.appID,
		ClientUser: c.userIDHash,
		SessionID:  c.sessionID,
		IsTestMode: c.testMode,
		Type:       signalType,
		Payload:    payload,
	}

	// Body must be an array of signals. We only send one signal at a time.
	body, err := json.Marshal([]SignalBody{signal})
	if err != nil {
		return nil, nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	return request, body, nil
}

// deliver performs one request and reports a failed transport (including a
// context that ended first) or a rejecting status as an error. In test mode
// the rejected request and the response body are logged as well.
func (c *Client) deliver(request *http.Request, body []byte) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusBadRequest {
		return nil
	}
	if c.testMode {
		c.logf("request body: %s", body)
		if responseBody, err := io.ReadAll(response.Body); err == nil {
			c.logf("response body: %s", responseBody)
		}
	}
	return fmt.Errorf("%s answered %s", c.endpoint, response.Status)
}

// logf prints to the configured logger, if any.
func (c *Client) logf(format string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// sdkNameAndVersion is the TelemetryDeck.SDK.nameAndVersion value:
// "telemetrydeck-go/<version>", with the version this library was built into
// the consuming binary at (from the module build info, so `go install` and
// `go build` both report the real module version). "telemetrydeck-go/dev"
// when the build info has no entry for it (e.g. this library's own tests).
func sdkNameAndVersion() string {
	const name = "telemetrydeck-go/"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return name + "dev"
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Replace != nil {
			dep = dep.Replace
		}
		// A directory replace records "(devel)"; that is a dev build too.
		if v := dep.Version; v != "" && v != "(devel)" {
			return name + strings.TrimPrefix(v, "v")
		}
	}
	return name + "dev"
}

// Returns the user ID set in the client (unhashed).
func (c *Client) UserID() string {
	return c.userID
}

// Returns the user ID hash set in the client.
func (c *Client) UserIDHash() string {
	return c.userIDHash
}

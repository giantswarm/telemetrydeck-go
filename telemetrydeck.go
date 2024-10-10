/*
A library to send telemetry data to TelemetryDeck.

Usage synopsis

	import "github.com/giantswarm/telemetrydeck-go" // imported as telemetrydeck

	// Represents an event we want to track
	type MySignal struct {
		// Some arbitrary string
		Command string
	}

	func myfunc() {
		// This is required!
		appID := os.Getenv("TELEMETRY_APP_ID")

		// This is recommended
		salt := os.Getenv("TELEMETRY_USER_HASH_SALT")

		// A unique user identifier, if desired
		email := ...

		// Create new client
		client, err := telemetrydeck.NewClient(appID).WithUserID(email).WithHashSalt(salt)
		if err != nil {
			panic(err)
		}

		// Define and transmit event to track
		signalPayload := map[string]interface{
			"command": "create",
		}
		signalType := "MyNamespace.mySignalType"
		err = client.SendSignal(context.Background(), signalType, signalPayload)
		if err != nil {
			panic(err)
		}
	}
*/
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
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	// The TelemetryDeck Ingest v2 API endpoint we use
	endpoint = "https://nom.telemetrydeck.com/v2/"

	version = "telemetrydeck-go/0.0.1" // TODO: set this version via linker flags
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

	appID      string
	endpoint   string
	hashSalt   string
	userID     string
	userIDHash string
	sessionID  string
	testMode   bool
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
func NewClient(appID string, options ...func(*Client)) (*Client, error) {
	if appID == "" {
		return nil, ErrNoAppID
	}

	// Create client with defaults
	client := &Client{
		appID:      appID,
		endpoint:   endpoint,
		userID:     generateUserId(),
		sessionID:  uuid.New().String(),
		httpClient: &http.Client{},
	}

	// Apply options overriding defaults
	for _, o := range options {
		o(client)
	}

	return client, nil
}

// Specify an alternative API endpoint. This is mainly useful
// for testing.
//
// To be used as an option paramter in the NewClient() func.
func WithEndpoint(endpoint string) func(*Client) {
	return func(c *Client) {
		c.endpoint = endpoint
	}
}

// Specify a hash salt string (recommended)
//
// This salt will be appended to the user identifier before it
// gets hashed and submitted to TelemetryDeck. This makes it
// a lot harder to de-anonymize user ID hashes, e. g. via some
// rainbow tables.
//
// To be used as an option paramter in the NewClient() func.
func WithHashSalt(salt string) func(*Client) {
	return func(c *Client) {
		c.hashSalt = salt

		// Re-hash the user ID with the new salt
		c.userIDHash = hashUserId(c.userID, c.hashSalt)
	}
}

// Specify a unique user identifier.
//
// The identifier will be salted and hashed before
// submitting to the TelemetryDeck API.
//
// If no unique user ID is specific, an identifier
// is generated.
//
// To be used as an option paramter in the NewClient() func.
func WithUserID(userID string) func(*Client) {
	return func(c *Client) {
		c.userID = userID
		c.userIDHash = hashUserId(userID, c.hashSalt)
	}
}

// Specify a session identifier. This should be the same value for
// the same session/user combination. If not given, a UUID will be
// generated at the creation of the client.
//
// To be used as an option paramter in the NewClient() func.
func WithSessionID(sessionID string) func(*Client) {
	return func(c *Client) {
		c.sessionID = sessionID
	}
}

// When set, data will be send with isTestMode=true, to avoid
// polluting production data. Also errors will be logged that
// would otherwise be silently ignored.
//
// To be used as an option paramter in the NewClient() func.
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

// Send a signal to the TelemetryDeck backend.
//
// The signalType is a string of your choice, identifying the type of the signal
// you are sending, e. g. "command". From the TelemetryDeck docs:
//
// "While it is not enforced, we recommend structuring your signal names in
// namespaces separated by dots, with the signal type beginning with a lower
// case letter and any namespaced beginning with an uppercase letter."
//
// The payload is a map of key-value pairs, containing the data you want to send.
func (c *Client) SendSignal(ctx context.Context, signalType string, payload map[string]interface{}) error {
	if signalType == "" {
		return ErrNoSignalType
	}

	if payload == nil {
		payload = make(map[string]interface{})
	}
	// Inject standard fields into the payload
	payload["TelemetryDeck.Device.operatingSystem"] = runtime.GOOS
	payload["TelemetryDeck.Device.architecture"] = runtime.GOARCH
	payload["TelemetryDeck.SDK.nameAndVersion"] = version

	signal := &SignalBody{
		AppID:      c.appID,
		ClientUser: c.userIDHash,
		SessionID:  c.sessionID,
		IsTestMode: c.testMode,
		Type:       signalType,
		Payload:    payload,
	}

	// Body must be an array of signals. We only send one signal at a time.
	signals := []SignalBody{*signal}

	b, err := json.Marshal(signals)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if c.testMode && response.StatusCode >= 400 {
		log.Printf("response status: %d", response.StatusCode)
		log.Printf("request body: %s", b)
		bodyBytes, err := io.ReadAll(response.Body)
		if err == nil {
			log.Printf("details: %s", string(bodyBytes))
		}
	}

	return nil
}
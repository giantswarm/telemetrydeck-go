package telemetrydeck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capture runs a server that hands the decoded signal array of each request
// to the returned channel.
func capture(t *testing.T) (*httptest.Server, <-chan []SignalBody) {
	t.Helper()
	got := make(chan []SignalBody, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var signals []SignalBody
		if err := json.Unmarshal(body, &signals); err != nil {
			t.Errorf("body is not a signal array: %v\n%s", err, body)
		}
		got <- signals
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// testAppID is the app the tests send under; nothing reaches TelemetryDeck.
const testAppID = "11111111-2222-3333-4444-555555555555"

// newTestClient builds a client that sends to srv.
func newTestClient(t *testing.T, srv *httptest.Server, opts ...func(*Client)) *Client {
	t.Helper()
	c, err := NewClient(testAppID, append(opts, WithEndpoint(srv.URL))...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func send(t *testing.T, srv *httptest.Server, got <-chan []SignalBody, opts ...func(*Client)) map[string]interface{} {
	t.Helper()
	c := newTestClient(t, srv, opts...)
	if err := c.SendSignal(context.Background(), "TestNamespace.command", map[string]interface{}{"appVersion": "legacy"}); err != nil {
		t.Fatal(err)
	}
	select {
	case signals := <-got:
		if len(signals) != 1 {
			t.Fatalf("got %d signals, want 1", len(signals))
		}
		return signals[0].Payload
	case <-time.After(5 * time.Second):
		t.Fatal("no signal reached the endpoint")
	}
	return nil
}

// The standard parameters the dashboard's insights read are set from the
// options; the caller's own payload keys pass through untouched.
func TestSendSignal_AppInfoParameters(t *testing.T) {
	srv, got := capture(t)

	payload := send(t, srv, got, WithAppVersion("v1.2.3"), WithBuildNumber("abc1234"))
	for k, want := range map[string]string{
		paramAppVersion:            "v1.2.3",
		paramBuildNumber:           "abc1234",
		paramVersionAndBuildNumber: "v1.2.3 abc1234",
		"appVersion":               "legacy",
	} {
		if payload[k] != want {
			t.Errorf("payload[%s] = %v, want %q", k, payload[k], want)
		}
	}

	payload = send(t, srv, got, WithAppVersion("v1.2.3"))
	if payload[paramAppVersion] != "v1.2.3" {
		t.Errorf("payload[%s] = %v", paramAppVersion, payload[paramAppVersion])
	}
	for _, k := range []string{paramBuildNumber, paramVersionAndBuildNumber} {
		if _, ok := payload[k]; ok {
			t.Errorf("payload has %s without a build number", k)
		}
	}

	payload = send(t, srv, got)
	for _, k := range []string{paramAppVersion, paramBuildNumber, paramVersionAndBuildNumber} {
		if _, ok := payload[k]; ok {
			t.Errorf("payload has %s although no app version or build number was given", k)
		}
	}
}

// Every signal names the operating system, the architecture and this SDK.
func TestSendSignal_DeviceAndSDKParameters(t *testing.T) {
	srv, got := capture(t)
	payload := send(t, srv, got)
	for _, k := range []string{paramOperatingSystem, paramArchitecture} {
		if payload[k] == nil || payload[k] == "" {
			t.Errorf("payload lacks %s", k)
		}
	}
	sdk, _ := payload[paramSDKNameAndVersion].(string)
	if !strings.HasPrefix(sdk, "telemetrydeck-go/") || strings.TrimPrefix(sdk, "telemetrydeck-go/") == "" {
		t.Errorf("%s = %q, want telemetrydeck-go/<version>", paramSDKNameAndVersion, sdk)
	}
}

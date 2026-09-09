package telemetrydeck

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testSignalType = "TestNamespace.command"

	// shortDeadline is what a CLI grants its exit; generous is what a test
	// waits for a delivery that must happen.
	shortDeadline = 100 * time.Millisecond
	generous      = 5 * time.Second
)

// stall runs a server whose handler answers only once release is called or
// the request is abandoned by its client, so a send can be held in flight.
func stall(t *testing.T) (srv *httptest.Server, release func()) {
	t.Helper()
	gate := make(chan struct{})
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-gate:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(func() {
		release()
		srv.Close()
	})
	return srv, release
}

func within(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// Flush returns once the signal SendSignal handed off has reached the
// endpoint, and at once when nothing is in flight.
func TestFlush_WaitsForDelivery(t *testing.T) {
	srv, got := capture(t)
	c := newTestClient(t, srv)

	if err := c.Flush(within(t, shortDeadline)); err != nil {
		t.Fatalf("Flush with nothing in flight: %v", err)
	}

	if err := c.SendSignal(context.Background(), testSignalType, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Flush(within(t, generous)); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	select {
	case signals := <-got:
		if len(signals) != 1 || signals[0].Type != testSignalType {
			t.Errorf("endpoint received %+v", signals)
		}
	default:
		t.Fatal("Flush returned before the signal reached the endpoint")
	}
}

// A deadline caps Flush: past it the error is the context's, the process is
// not held, and the send itself goes on in the background.
func TestFlush_DeadlineExceeded(t *testing.T) {
	srv, release := stall(t)
	c := newTestClient(t, srv)
	if err := c.SendSignal(context.Background(), testSignalType, nil); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err := c.Flush(within(t, shortDeadline))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Flush = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Flush held the caller for %s past a %s deadline", elapsed, shortDeadline)
	}

	release()
	if err := c.Flush(within(t, generous)); err != nil {
		t.Fatalf("Flush after the endpoint answered: %v", err)
	}
}

// SendSignalSync returns after the endpoint accepted the signal.
func TestSendSignalSync_Delivers(t *testing.T) {
	srv, got := capture(t)
	c := newTestClient(t, srv)
	if err := c.SendSignalSync(within(t, generous), testSignalType, map[string]interface{}{"command": "probe"}); err != nil {
		t.Fatalf("SendSignalSync: %v", err)
	}
	select {
	case signals := <-got:
		if len(signals) != 1 || signals[0].Payload["command"] != "probe" {
			t.Errorf("endpoint received %+v", signals)
		}
	default:
		t.Fatal("SendSignalSync returned before the signal reached the endpoint")
	}
}

// SendSignalSync honours the deadline instead of blocking on a stalled endpoint.
func TestSendSignalSync_DeadlineExceeded(t *testing.T) {
	srv, _ := stall(t)
	c := newTestClient(t, srv)

	start := time.Now()
	err := c.SendSignalSync(within(t, shortDeadline), testSignalType, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendSignalSync = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("SendSignalSync held the caller for %s past a %s deadline", elapsed, shortDeadline)
	}
}

// A rejecting status is an error for the synchronous send and, in test mode,
// comes with the request and response bodies on the logger.
func TestSendSignalSync_RejectedSignal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("appID is not a UUID"))
	}))
	t.Cleanup(srv.Close)
	var logged bytes.Buffer
	c := newTestClient(t, srv, WithTestMode(), WithLogger(log.New(&logged, "", 0)))

	err := c.SendSignalSync(within(t, generous), testSignalType, nil)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("SendSignalSync = %v, want the 400 status", err)
	}
	for _, want := range []string{testAppID, "appID is not a UUID"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("test-mode log lacks %q:\n%s", want, logged.String())
		}
	}
}

// SendSignal keeps its contract (returns nil at once) while the request
// carries the caller's context: a cancelled one abandons the delivery, and
// the failure goes to the logger.
func TestSendSignal_UsesTheCallersContext(t *testing.T) {
	srv, got := capture(t)
	var logged bytes.Buffer
	c := newTestClient(t, srv, WithLogger(log.New(&logged, "", 0)))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.SendSignal(ctx, testSignalType, nil); err != nil {
		t.Fatalf("SendSignal with a cancelled context returned %v, want nil", err)
	}
	if err := c.Flush(within(t, generous)); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	select {
	case signals := <-got:
		t.Fatalf("the cancelled send reached the endpoint: %+v", signals)
	default:
	}
	if !strings.Contains(logged.String(), context.Canceled.Error()) {
		t.Errorf("logger has %q, want the cancellation", logged.String())
	}
}

// The default HTTP client times out, so an unreachable network never holds a
// waiting program indefinitely.
func TestNewClient_HTTPClientTimeout(t *testing.T) {
	c, err := NewClient(testAppID)
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient.Timeout != defaultTimeout || defaultTimeout <= 0 {
		t.Errorf("http client timeout = %s, want %s", c.httpClient.Timeout, defaultTimeout)
	}
}

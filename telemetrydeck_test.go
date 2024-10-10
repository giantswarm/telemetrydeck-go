package telemetrydeck

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_SendSignal(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		signalType string
		payload    map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "valid signal with user",
			userID:     "test-user",
			signalType: "TestNamespace.testSignal",
			payload: map[string]interface{}{
				"TestNamespace.someString":          "some-string-value",
				"TestNamespace.someStringWithSpace": "string value with space",
				"TestNamespace.someInt":             42,
				"TestNamespace.someBool":            true,
				"TestNamespace.someFloat":           3.14,
			},
		},
		{
			name:       "generated user id",
			userID:     generateUserId(),
			signalType: "TestNamespace.userIdTest",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		t.Logf("Request payload: %s", string(content))
	}))
	defer server.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient("11111111-2222-3333-4444-555555555555",
				WithTestMode(),
				WithEndpoint(server.URL),
				WithUserID(tt.userID),
			)
			if err != nil && !tt.wantErr {
				t.Errorf("NewClient() error = %v", err)
			}

			ctx := context.Background()
			if err := c.SendSignal(ctx, tt.signalType, tt.payload); (err != nil) != tt.wantErr {
				t.Errorf("Client.SendSignal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func ExampleNewClient() {
	client, err := NewClient("my-app-id", WithUserID("somebody@example.com"), WithHashSalt("mySalt"))
	if err != nil {
		panic(err)
	}

	err = client.SendSignal(context.Background(), "mySignalType", map[string]interface{}{"command": "sample command"})
	if err != nil {
		panic(err)
	}
	// Output:
}

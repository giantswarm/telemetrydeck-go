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

func Test_generateUserId(t *testing.T) {
	gotId := generateUserId()
	t.Logf("generateUserId(): %q", gotId)
	if gotId == "" {
		t.Errorf("generateUserId() returned empty string")
	}
}

func Test_UserIdHashingWithSalt(t *testing.T) {
	userID := "somebody@example.com"
	salt := "MySalt"
	expectedHash := "c05dc5334d83cca7382bca040f2f6e9de56d57d22814cfc4c39b5a55dbc9ef16" // sha256 -s "somebody@example.comMySalt"

	client, err := NewClient("my-app-id", WithUserID(userID), WithHashSalt(salt))
	if err != nil {
		t.Fatalf("unexpected error when creating the client: %s", err)
	}

	hash := hashUserId(userID, salt)

	if hash != expectedHash {
		t.Errorf("hashUserId() did not return the expected hash. got %q, expected %q", hash, expectedHash)
	}
	if client.userIDHash != expectedHash {
		t.Errorf("client.userIDHash wasn't the expected value. got %q, expected %q", client.userIDHash, expectedHash)
	}
}

func Test_UserIdHashingWithoutSalt(t *testing.T) {
	userID := "somebody@example.com"
	salt := ""
	expectedHash := "787d5b7c06ca0a21c96436cc7c8117e6fe046d0fd7dedcae8c93bfc14b8e5df7" // sha256 -s "somebody@example.com"

	client, err := NewClient("my-app-id", WithUserID(userID))
	if err != nil {
		t.Fatalf("unexpected error when creating the client: %s", err)
	}

	hash := hashUserId(userID, salt)

	if hash != expectedHash {
		t.Errorf("hashUserId() did not return the expected hash. got %q, expected %q", hash, expectedHash)
	}
	if client.userIDHash != expectedHash {
		t.Errorf("client.userIDHash wasn't the expected value. got %q, expected %q", client.userIDHash, expectedHash)
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

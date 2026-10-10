package fieldservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestStatusStreamPublishesOnlyTransitions(t *testing.T) {
	var paths []string
	var publishedChannel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header was not shared")
		}
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("missing idempotency key")
		}
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/realtime/publish" {
			var payload struct {
				Channel string `json:"channel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			publishedChannel = payload.Channel
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{}, "error": nil, "metadata": map[string]any{}})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", "field-ops", server.Client())
	stream := NewStatusStream(client)
	tests := []struct {
		name       string
		status     string
		decision   string
		requestSum int
	}{
		{name: "first device report", status: "dispatched", decision: "published", requestSum: 2},
		{name: "duplicate device report", status: "dispatched", decision: "unchanged", requestSum: 2},
		{name: "technician arrives", status: "on_site", decision: "published", requestSum: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := stream.Apply(context.Background(), DeviceUpdate{
				DeviceID:       "meter-17",
				WorkOrderID:    "wo-2048",
				DispatchStatus: test.status,
				PhotoURL:       "https://photos.example/wo-2048.jpg",
				FollowUp:       true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != test.decision {
				t.Fatalf("decision = %q, want %q", result.Decision, test.decision)
			}
			if len(paths) != test.requestSum {
				t.Fatalf("requests = %d, want %d", len(paths), test.requestSum)
			}
		})
	}

	wantPaths := []string{
		"/v1/realtime/publish", "/v1/metrics/report",
		"/v1/realtime/publish", "/v1/metrics/report",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	if publishedChannel != "field-ops" {
		t.Fatalf("published channel = %q, want field-ops", publishedChannel)
	}
}

func TestClientDecodesBusinessRejectionBeforeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": map[string]string{"code": "request_rejected", "message": "channel is required"},
		})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", "field-ops", server.Client())
	err := client.Publish(context.Background(), DeviceUpdate{WorkOrderID: "wo-9"}, "request-9")
	apiErr, ok := err.(*InfraiError)
	if !ok || apiErr.HTTPStatus != http.StatusBadRequest || !strings.Contains(apiErr.Error(), "channel is required") {
		t.Fatalf("error = %#v", err)
	}
}

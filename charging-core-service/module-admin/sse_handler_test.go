package admin

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/state"
)

// End-to-end through the real HTTP handler: publish a connector update into
// the hub and confirm an SSE client receives a well-formed status event.
func TestSSEDeliversConnectorUpdate(t *testing.T) {
	hub := state.NewHub()
	api := NewAPI(nil, nil, state.NewMemoryStore()).WithStateHub(hub)
	mux := http.NewServeMux()
	api.RegisterSSE(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/admin/connector-events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %s", ct)
	}

	reader := bufio.NewReader(resp.Body)
	// First line is the connected comment.
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, ":") {
		t.Fatalf("first line = %q", line)
	}

	// Publish after the client is subscribed.
	time.Sleep(50 * time.Millisecond)
	hub.Publish(state.Event{Type: "status", Connector: state.ConnectorState{
		DeviceID: "55031412782301", GunNo: 1, Status: "occupied",
	}})

	var eventLine, dataLine string
	for i := 0; i < 10; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event: ") {
			eventLine = line
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if eventLine != "event: status" {
		t.Errorf("event line = %q", eventLine)
	}
	var ev state.Event
	if err := json.Unmarshal([]byte(dataLine), &ev); err != nil {
		t.Fatalf("bad SSE data %q: %v", dataLine, err)
	}
	if ev.Connector.DeviceID != "55031412782301" || ev.Connector.Status != "occupied" {
		t.Errorf("event = %+v", ev)
	}
}

func TestSSEQueryTokenAuth(t *testing.T) {
	// apiKeyFrom must accept ?api_key= because EventSource cannot set headers.
	r := httptest.NewRequest(http.MethodGet, "/api/admin/connector-events?api_key=secret123", nil)
	if got := apiKeyFrom(r); got != "secret123" {
		t.Errorf("apiKeyFrom query = %q", got)
	}
}

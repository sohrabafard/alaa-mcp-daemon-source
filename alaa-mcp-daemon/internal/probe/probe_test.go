package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"alaa-mcp-daemon/internal/config"
)

func TestProcessProbe(t *testing.T) {
	result := Check(context.Background(), config.EffectiveProbe{Type: "process"}, func() bool { return true })
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
}

func TestTCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, portRaw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portRaw)
	result := Check(context.Background(), config.EffectiveProbe{Type: "tcp", Host: host, Port: port, Timeout: time.Second}, func() bool { return true })
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPProbeStatusGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	result := Check(context.Background(), config.EffectiveProbe{Type: "http", URL: server.URL, Method: "GET", Timeout: time.Second, AllowedStatuses: map[int]struct{}{204: {}}}, func() bool { return true })
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
}

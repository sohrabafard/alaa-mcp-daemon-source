package probe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"alaa-mcp-daemon/internal/config"
)

type Result struct {
	OK      bool
	Message string
	At      time.Time
}

func Check(ctx context.Context, p config.EffectiveProbe, processAlive func() bool) Result {
	result := Result{At: time.Now().UTC()}
	if processAlive != nil && !processAlive() {
		result.Message = "process exited"
		return result
	}
	switch p.Type {
	case "process":
		result.OK = true
		result.Message = "process is running"
	case "tcp":
		dialer := net.Dialer{Timeout: p.Timeout}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
		if err != nil {
			result.Message = fmt.Sprintf("tcp probe failed: %v", err)
			return result
		}
		_ = conn.Close()
		result.OK = true
		result.Message = "tcp connection succeeded"
	case "http":
		client := &http.Client{Timeout: p.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, nil)
		if err != nil {
			result.Message = fmt.Sprintf("create HTTP request: %v", err)
			return result
		}
		resp, err := client.Do(req)
		if err != nil {
			result.Message = fmt.Sprintf("http probe failed: %v", err)
			return result
		}
		_ = resp.Body.Close()
		if _, ok := p.AllowedStatuses[resp.StatusCode]; !ok {
			result.Message = fmt.Sprintf("http probe returned status %d", resp.StatusCode)
			return result
		}
		result.OK = true
		result.Message = fmt.Sprintf("http probe returned status %d", resp.StatusCode)
	default:
		result.Message = fmt.Sprintf("unsupported probe type %q", p.Type)
	}
	return result
}

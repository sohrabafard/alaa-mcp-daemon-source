package control

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

func Call(configPath, stateDir string, request Request, timeout time.Duration) (Response, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := dial(configPath, stateDir, DialOptions{Timeout: timeout})
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	timer := time.AfterFunc(timeout, func() { _ = conn.Close() })
	defer timer.Stop()
	request.Version = ProtocolVersion
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return Response{}, fmt.Errorf("write control request: %w", err)
	}
	var response Response
	dec := json.NewDecoder(io.LimitReader(conn, maxMessageBytes))
	if err := dec.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("read control response: %w", err)
	}
	if response.Version != ProtocolVersion {
		return Response{}, fmt.Errorf("daemon returned protocol version %d", response.Version)
	}
	if !response.OK {
		return response, fmt.Errorf("daemon rejected command: %s", response.Error)
	}
	return response, nil
}

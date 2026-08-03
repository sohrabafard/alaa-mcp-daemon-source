package control

import "alaa-mcp-daemon/internal/model"

const ProtocolVersion = 1

type Request struct {
	Version int    `json:"version"`
	Command string `json:"command"`
	Service string `json:"service,omitempty"`
}

type LogPaths struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type Response struct {
	Version int                 `json:"version"`
	OK      bool                `json:"ok"`
	Error   string              `json:"error,omitempty"`
	Status  *model.DaemonStatus `json:"status,omitempty"`
	Logs    *LogPaths           `json:"logs,omitempty"`
}

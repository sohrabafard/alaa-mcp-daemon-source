package main

import (
	"os"

	"alaa-mcp-daemon/internal/app"
)

func main() { os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr)) }

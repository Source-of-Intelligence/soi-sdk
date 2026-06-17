//go:build !tinygo && !wasip1

// Package sdk provides the SOI plugin development kit for standard Go WASM plugins.
// This file provides the stdio-based runner for local development and testing.
//
// Build with: go build -o plugin.exe .
package sdk

import (
	"bufio"
	"encoding/json"
	"os"
)

func Run() {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		writeErrorSDK("no input")
		return
	}
	result := ExecuteRequest(scanner.Bytes())
	if result.Error != "" {
		writeErrorSDK(result.Error)
		return
	}
	os.Stdout.Write(result.Output)
	os.Stdout.Write([]byte{'\n'})
}

func ExecuteRequest(reqJSON []byte) ExecuteResponse {
	var req struct {
		Tool        string          `json:"tool"`
		Args        json.RawMessage `json:"args"`
		SandboxRoot string          `json:"sandbox_root,omitempty"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return ExecuteResponse{Error: "parse input: " + err.Error()}
	}
	// In real WASM mode there is no host bridge yet; pass nil.
	return CallTool(req.Tool, req.Args, req.SandboxRoot, nil)
}

func writeErrorSDK(msg string) {
	data, _ := json.Marshal(map[string]string{"error": msg})
	os.Stderr.Write(data)
	os.Stderr.Write([]byte{'\n'})
}

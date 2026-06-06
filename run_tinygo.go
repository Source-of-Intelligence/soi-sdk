//go:build tinygo

// Package sdk provides TinyGo-specific helpers for SOI plugins.
//
// Plugins compiled with TinyGo should have a minimal main_tinygo.go:
//
//	//go:build tinygo
//	package main
//	import sdk "soi.dev/soi-sdk"
//	func init() { sdk.SetBuildTag("tinygo") }
//	func main() { registerTools(); sdk.RunTinyGo() }
//
// The registerTools() function (exported or not) should call sdk.RegisterSOITool().
// sdk.RunTinyGo() handles the execute loop and result packing.
package sdk

import (
	"encoding/json"
	"unsafe"
)

var resultBuf []byte
var inputBuf []byte

// RunTinyGo starts the TinyGo SOI runtime loop.
// It reads requests from stdin, dispatches to registered tools, and writes results.
// This is the stdio-based fallback for TinyGo; real WASM hosts call execute directly.
func RunTinyGo() {
	// TinyGo plugins using stdio ABI can call this in main().
	// For export ABI, the host calls execute directly (see below).
}

// ExecuteTinyGoRequest parses a JSON request and executes the named tool.
// It returns the raw JSON response bytes.
func ExecuteTinyGoRequest(reqJSON []byte) []byte {
	var req struct {
		Tool        string          `json:"tool"`
		Args        json.RawMessage `json:"args"`
		SandboxRoot string          `json:"sandbox_root,omitempty"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		b, _ := json.Marshal(map[string]string{"error": "parse input: " + err.Error()})
		return b
	}
	resp := ExecuteTool(GetTools(), GetSOITools(), req.Tool, req.Args, req.SandboxRoot, NewTinyGoHostAPI())
	if resp.Error != "" {
		b, _ := json.Marshal(map[string]string{"error": resp.Error})
		return b
	}
	return resp.Output
}

// PackResult packs data into a (ptr<<32)|length uint64 for WASM export ABI.
func PackResult(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	ptr := uint64(uintptr(unsafe.Pointer(&data[0])))
	length := uint64(len(data))
	return (ptr << 32) | length
}

// SetResultBuf stores data in the persistent result buffer to prevent GC.
func SetResultBuf(data []byte) {
	resultBuf = data
}

// GetResultBuf returns the current result buffer.
func GetResultBuf() []byte {
	return resultBuf
}

// SetInputBuf stores input data in the persistent input buffer to prevent GC.
func SetInputBuf(data []byte) {
	inputBuf = data
}

// GetInputBuf returns the current input buffer.
func GetInputBuf() []byte {
	return inputBuf
}

//go:build !tinygo && wasip1

// Package sdk provides the SOI plugin development kit for standard Go WASM plugins.
//
// Build with: GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm .
//
// Plugin must export an "execute" function that receives JSON requests and returns
// JSON responses via WASM memory.
package sdk

import (
	"encoding/json"
	"unsafe"

	"github.com/Source-of-Intelligence/soi-vos"
)

// Run is the main entry point for standard Go WASM plugins.
// This function is called by the WASM runtime and never returns.
func Run() {
	// The execute export is called by the host; Run() exists to satisfy
	// the standard Go WASM main module requirement. The actual dispatch
	// is handled by the exported execute function.
	select {}
}

// Shared buffer address in WASM linear memory for host function data transfer.
// This address is chosen to be above TinyGo's typical heap/stack usage.
// For standard Go WASM, we use a fixed address and rely on the host
// pre-allocating the WASM memory (which wazero does with MemoryCapacityFromMax).
const sharedMemAddr = 65536

// execute is the main WASM export that handles tool invocations.
//
//go:export execute
func execute(ptr uint32, length uint32) uint64 {
	// Read input from WASM linear memory at the provided pointer
	// The host writes the request JSON at this address.
	input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

	// Make a copy since the input buffer may be overwritten
	reqJSON := make([]byte, length)
	copy(reqJSON, input)

	// Parse request
	var req struct {
		Tool        string          `json:"tool"`
		Args        json.RawMessage `json:"args"`
		SandboxRoot string          `json:"sandbox_root,omitempty"`
	}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return respondError("parse request: " + err.Error())
	}

	// Create host API instance
	host := NewGoHostAPI()

	// Execute tool
	resp := CallTool(req.Tool, req.Args, req.SandboxRoot, host)
	if resp.Error != "" {
		return respondError(resp.Error)
	}

	// Return success response
	return respondOK(resp.Output)
}

// respondError creates an error response and returns it as a packed pointer+length.
func respondError(msg string) uint64 {
	errResp, _ := json.Marshal(map[string]string{"error": msg})
	return writeResult(errResp)
}

// respondOK creates a success response and returns it as a packed pointer+length.
func respondOK(data []byte) uint64 {
	return writeResult(data)
}

// writeResult writes data to WASM linear memory and returns (ptr<<32)|len.
// The host will read from the returned pointer.
func writeResult(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	// Ensure we don't exceed max output size
	maxSize := 1 << 20 // 1MB
	if len(data) > maxSize {
		data = data[:maxSize]
	}
	// Write to shared memory region
	for i := 0; i < len(data); i++ {
		*(*byte)(unsafe.Pointer(uintptr(sharedMemAddr + i))) = data[i]
	}
	return vos.Pack(uint32(sharedMemAddr), uint32(len(data)))
}

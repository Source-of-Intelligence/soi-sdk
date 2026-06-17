//go:build !tinygo && wasip1

package sdk

import (
	"encoding/json"
	"unsafe"

	"github.com/Source-of-Intelligence/soi-vos"
)

// GoHostAPI implements vos.HostFunctions using WASM host function imports.
// This is used inside the standard Go-compiled WASM plugin to call back to the executor.
type GoHostAPI struct{}

// Ensure GoHostAPI implements vos.HostFunctions
var _ vos.HostFunctions = (*GoHostAPI)(nil)

// NewGoHostAPI creates a new GoHostAPI instance.
func NewGoHostAPI() *GoHostAPI {
	return &GoHostAPI{}
}

//go:wasmimport soi soi_log
func hostLog(level int32, ptr int64, len int64)

// Log emits a log message via the host.
func (h *GoHostAPI) Log(level int32, msg string) {
	p := packStringGo(msg)
	hostLog(level, int64(p>>32), int64(p&0xFFFFFFFF))
}

//go:wasmimport soi soi_now
func hostNow() int64

// Now returns the current Unix timestamp in milliseconds.
func (h *GoHostAPI) Now() int64 {
	return hostNow()
}

//go:wasmimport soi soi_random
func hostRandom(ptr int64, len int64)

// Random fills buf with cryptographically secure random bytes.
func (h *GoHostAPI) Random(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	p := packBytesGo(buf)
	hostRandom(int64(p>>32), int64(p&0xFFFFFFFF))
	return nil
}

//go:wasmimport soi soi_sandbox_read
func hostSandboxRead(ptr int64, len int64) int64

// SandboxRead reads a file from the sandbox.
func (h *GoHostAPI) SandboxRead(path string) ([]byte, error) {
	pp := packStringGo(path)
	result := hostSandboxRead(int64(pp>>32), int64(pp&0xFFFFFFFF))
	return unpackBytesGo(uint64(result))
}

//go:wasmimport soi soi_sandbox_write
func hostSandboxWrite(pathPtr int64, pathLen int64, dataPtr int64, dataLen int64) int32

// SandboxWrite writes data to a file in the sandbox.
func (h *GoHostAPI) SandboxWrite(path string, data []byte) error {
	pp := packStringGo(path)
	dp := packBytesGo(data)
	ret := hostSandboxWrite(int64(pp>>32), int64(pp&0xFFFFFFFF), int64(dp>>32), int64(dp&0xFFFFFFFF))
	if ret != 0 {
		return vos.ErrHostFunctionFailed
	}
	return nil
}

//go:wasmimport soi soi_sandbox_list
func hostSandboxList(ptr int64, len int64) int64

// SandboxList lists entries in a sandbox directory.
func (h *GoHostAPI) SandboxList(path string) ([]string, error) {
	pp := packStringGo(path)
	result := hostSandboxList(int64(pp>>32), int64(pp&0xFFFFFFFF))
	data, err := unpackBytesGo(uint64(result))
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, err
	}
	return names, nil
}

//go:wasmimport soi soi_sandbox_stat
func hostSandboxStat(ptr int64, len int64) int64

// SandboxStat returns metadata for a sandbox file or directory.
func (h *GoHostAPI) SandboxStat(path string) (*vos.FileInfo, error) {
	pp := packStringGo(path)
	result := hostSandboxStat(int64(pp>>32), int64(pp&0xFFFFFFFF))
	data, err := unpackBytesGo(uint64(result))
	if err != nil {
		return nil, err
	}
	var info vos.FileInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

//go:wasmimport soi soi_sandbox_exec
func hostSandboxExec(ptr int64, len int64) int64

// SandboxExec executes a command within the sandbox.
func (h *GoHostAPI) SandboxExec(cmd string) (*vos.ExecResult, error) {
	cp := packStringGo(cmd)
	result := hostSandboxExec(int64(cp>>32), int64(cp&0xFFFFFFFF))
	data, err := unpackBytesGo(uint64(result))
	if err != nil {
		return nil, err
	}
	var res vos.ExecResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

//go:wasmimport soi soi_sandbox_http
func hostSandboxHttp(ptr int64, len int64) int64

// SandboxHttp makes an HTTP request.
func (h *GoHostAPI) SandboxHttp(req *vos.HttpRequest) (*vos.HttpResponse, error) {
	if req == nil {
		return nil, nil
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	p := packBytesGo(payload)
	result := hostSandboxHttp(int64(p>>32), int64(p&0xFFFFFFFF))
	data, err := unpackBytesGo(uint64(result))
	if err != nil {
		return nil, err
	}
	var resp vos.HttpResponse
	if len(data) > 0 {
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// --- Memory packing helpers for standard Go WASM ---

// Shared buffer address in WASM linear memory for host function data transfer.
const sharedBufAddrGo = 65536
const sharedBufSizeGo = 8192

// packStringGo writes the string to the shared buffer and returns a packed ptr+len.
func packStringGo(s string) uint64 {
	if s == "" {
		return 0
	}
	n := len(s)
	if n > sharedBufSizeGo {
		n = sharedBufSizeGo
	}
	// Write directly to WASM linear memory
	for i := 0; i < n; i++ {
		*(*byte)(unsafe.Pointer(uintptr(sharedBufAddrGo + i))) = s[i]
	}
	return (uint64(sharedBufAddrGo) << 32) | uint64(n)
}

// packBytesGo writes the byte slice to the shared buffer and returns a packed ptr+len.
func packBytesGo(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	n := len(b)
	if n > sharedBufSizeGo {
		n = sharedBufSizeGo
	}
	offset := sharedBufAddrGo + sharedBufSizeGo
	for i := 0; i < n; i++ {
		*(*byte)(unsafe.Pointer(uintptr(offset + i))) = b[i]
	}
	return (uint64(offset) << 32) | uint64(n)
}

// unpackBytesGo reads a packed ptr+len from WASM linear memory.
func unpackBytesGo(packed uint64) ([]byte, error) {
	if packed == 0 {
		return nil, nil
	}
	ptr := uint32(packed >> 32)
	length := uint32(packed & 0xFFFFFFFF)
	if length == 0 {
		return nil, nil
	}
	// Read from WASM linear memory
	src := (*[1 << 30]byte)(unsafe.Pointer(uintptr(ptr)))[:length:length]
	dst := make([]byte, length)
	copy(dst, src)
	return dst, nil
}

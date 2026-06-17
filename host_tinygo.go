//go:build tinygo

package sdk

import (
	"encoding/json"
	"unsafe"

	"github.com/Source-of-Intelligence/soi-vos"
)

// TinyGoHostAPI implements vos.HostFunctions using WASM host function imports.
// This is used inside the TinyGo-compiled WASM plugin to call back to the executor.
type TinyGoHostAPI struct{}

// Ensure TinyGoHostAPI implements vos.HostFunctions
var _ vos.HostFunctions = (*TinyGoHostAPI)(nil)

func NewTinyGoHostAPI() *TinyGoHostAPI {
	return &TinyGoHostAPI{}
}

//go:wasmimport soi soi_log
func hostLog(level int32, ptr int64, len int64)

func (h *TinyGoHostAPI) Log(level int32, msg string) {
	p := packString(msg)
	hostLog(level, int64(p>>32), int64(p&0xFFFFFFFF))
}

//go:wasmimport soi soi_now
func hostNow() int64

func (h *TinyGoHostAPI) Now() int64 {
	return hostNow()
}

//go:wasmimport soi soi_random
func hostRandom(ptr int64, len int64)

func (h *TinyGoHostAPI) Random(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	p := packBytes(buf)
	hostRandom(int64(p>>32), int64(p&0xFFFFFFFF))
	return nil
}

//go:wasmimport soi soi_sandbox_read
func hostSandboxRead(pathPtr int64, pathLen int64) int64

func (h *TinyGoHostAPI) SandboxRead(path string) ([]byte, error) {
	pp := packString(path)
	result := hostSandboxRead(int64(pp>>32), int64(pp&0xFFFFFFFF))
	return unpackBytes(uint64(result))
}

//go:wasmimport soi soi_sandbox_write
func hostSandboxWrite(pathPtr int64, pathLen int64, dataPtr int64, dataLen int64) int32

func (h *TinyGoHostAPI) SandboxWrite(path string, data []byte) error {
	pp := packString(path)
	dp := packBytes(data)
	ret := hostSandboxWrite(int64(pp>>32), int64(pp&0xFFFFFFFF), int64(dp>>32), int64(dp&0xFFFFFFFF))
	if ret != 0 {
		return vos.ErrHostFunctionFailed
	}
	return nil
}

//go:wasmimport soi soi_sandbox_list
func hostSandboxList(pathPtr int64, pathLen int64) int64

func (h *TinyGoHostAPI) SandboxList(path string) ([]string, error) {
	pp := packString(path)
	result := hostSandboxList(int64(pp>>32), int64(pp&0xFFFFFFFF))
	data, err := unpackBytes(uint64(result))
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
func hostSandboxStat(pathPtr int64, pathLen int64) int64

func (h *TinyGoHostAPI) SandboxStat(path string) (*vos.FileInfo, error) {
	pp := packString(path)
	result := hostSandboxStat(int64(pp>>32), int64(pp&0xFFFFFFFF))
	data, err := unpackBytes(uint64(result))
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
func hostSandboxExec(cmdPtr int64, cmdLen int64) int64

func (h *TinyGoHostAPI) SandboxExec(cmd string) (*vos.ExecResult, error) {
	cp := packString(cmd)
	result := hostSandboxExec(int64(cp>>32), int64(cp&0xFFFFFFFF))
	data, err := unpackBytes(uint64(result))
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
func hostSandboxHttp(reqPtr int64, reqLen int64) int64

func (h *TinyGoHostAPI) SandboxHttp(req *vos.HttpRequest) (*vos.HttpResponse, error) {
	if req == nil {
		return nil, nil
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	p := packBytes(payload)
	result := hostSandboxHttp(int64(p>>32), int64(p&0xFFFFFFFF))
	data, err := unpackBytes(uint64(result))
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

// --- Memory packing helpers ---

// Shared buffer address in WASM linear memory for host function data transfer.
// This address is chosen to be above TinyGo's typical heap/stack usage.
const sharedBufferAddr = 65536

// packString writes the string to the shared buffer and returns a packed ptr+len.
func packString(s string) uint64 {
	if s == "" {
		return 0
	}
	n := len(s)
	if n > 4096 {
		n = 4096
	}
	// Write directly to WASM linear memory at the shared buffer address
	for i := 0; i < n; i++ {
		*(*byte)(unsafe.Pointer(uintptr(sharedBufferAddr + i))) = s[i]
	}
	return (uint64(sharedBufferAddr) << 32) | uint64(n)
}

// packBytes writes the byte slice to the shared buffer and returns a packed ptr+len.
func packBytes(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	n := len(b)
	if n > 4096 {
		n = 4096
	}
	// Write to shared buffer + offset to avoid overwriting path data
	offset := sharedBufferAddr + 4096
	for i := 0; i < n; i++ {
		*(*byte)(unsafe.Pointer(uintptr(offset + i))) = b[i]
	}
	return (uint64(offset) << 32) | uint64(n)
}

// unpackBytes reads a packed ptr+len from WASM linear memory.
// The host function writes data into WASM linear memory and returns a packed
// pointer+length. We read from that memory location.
func unpackBytes(packed uint64) ([]byte, error) {
	if packed == 0 {
		return nil, nil
	}
	ptr := uint32(packed >> 32)
	length := uint32(packed & 0xFFFFFFFF)
	if length == 0 {
		return nil, nil
	}
	// In TinyGo WASM, linear memory is accessible via unsafe.Pointer.
	// We copy the data into a new Go slice to avoid issues with GC.
	src := (*[1 << 30]byte)(unsafe.Pointer(uintptr(ptr)))[:length:length]
	dst := make([]byte, length)
	copy(dst, src)
	return dst, nil
}

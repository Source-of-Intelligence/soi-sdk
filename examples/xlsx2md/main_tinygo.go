//go:build tinygo

package main

import (
	"encoding/json"
	"unsafe"

	sdk "soi.dev/soi-sdk"
)

func init() {
	sdk.SetBuildTag("tinygo")
}

//export execute
func execute(ptr uint32, length uint32) uint64 {
	input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

	// Persistent copy to avoid TinyGo GC issues
	sdk.SetInputBuf(make([]byte, length))
	copy(sdk.GetInputBuf(), input)

	// Manual JSON parsing to avoid TinyGo json.Unmarshal UTF-8 corruption
	toolVal := extractStringField(sdk.GetInputBuf(), "tool")
	argsVal := extractRawField(sdk.GetInputBuf(), "args")
	sandboxVal := extractStringField(sdk.GetInputBuf(), "sandbox_root")

	resp := sdk.CallTool(toolVal, json.RawMessage(argsVal), sandboxVal, sdk.NewTinyGoHostAPI())
	if resp.Error != "" {
		sdk.SetResultBuf(jsonError(resp.Error))
	} else {
		sdk.SetResultBuf(resp.Output)
	}
	return sdk.PackResult(sdk.GetResultBuf())
}

func extractStringField(data []byte, key string) string {
	keyBytes := []byte(`"` + key + `":"`)
	for i := 0; i <= len(data)-len(keyBytes); i++ {
		if matchAt(data, i, keyBytes) {
			start := i + len(keyBytes)
			end := start
			for end < len(data) && data[end] != '"' {
				end++
			}
			if end > start {
				return string(data[start:end])
			}
			break
		}
	}
	return ""
}

func extractRawField(data []byte, key string) []byte {
	keyBytes := []byte(`"` + key + `":`)
	for i := 0; i <= len(data)-len(keyBytes); i++ {
		if matchAt(data, i, keyBytes) {
			start := i + len(keyBytes)
			for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n' || data[start] == '\r') {
				start++
			}
			if start >= len(data) {
				return nil
			}
			end := start + 1
			if data[start] == '{' || data[start] == '[' {
				open := data[start]
				var close byte
				if open == '{' {
					close = '}'
				} else {
					close = ']'
				}
				depth := 1
				inString := false
				for end < len(data) && depth > 0 {
					c := data[end]
					if !inString {
						if c == open {
							depth++
						} else if c == close {
							depth--
						} else if c == '"' {
							inString = true
						}
					} else {
						if c == '"' && data[end-1] != '\\' {
							inString = false
						}
					}
					end++
				}
			} else if data[start] == '"' {
				end = start + 1
				for end < len(data) {
					if data[end] == '"' && data[end-1] != '\\' {
						end++
						break
					}
					end++
				}
			} else {
				for end < len(data) && data[end] != ',' && data[end] != '}' {
					end++
				}
			}
			return data[start:end]
		}
	}
	return nil
}

func matchAt(data []byte, offset int, pattern []byte) bool {
	for j := 0; j < len(pattern); j++ {
		if data[offset+j] != pattern[j] {
			return false
		}
	}
	return true
}

func jsonError(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

func main() {
	registerTools()
}

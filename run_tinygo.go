//go:build tinygo

// Package sdk provides TinyGo-specific helpers for SOI plugins.
//
// TinyGo插件的最小main.go示例：
//
//	func init() {
//	    registerTools()
//	}
//
//	//export registerTools
//	func registerTools() {
//	    sdk.NewTool("word_to_md").
//	        Desc("...").
//	        Param("source", "string", true, "", "...").
//	        RegisterSOI(handler)
//	}
//
//	func main() {
//	    sdk.RunTinyGo()
//	}
//
// SDK会自动处理：
// - WASM导出函数execute
// - JSON请求解析
// - 结果打包
// - 工具注册
package sdk

import (
	"encoding/json"
	"unsafe"
)

var resultBuf []byte
var inputBuf []byte

// RunTinyGo 启动TinyGo SOI运行时
// 在TinyGo编译时，SDK会自动导出execute函数
// 用户只需在main中调用此函数即可
func RunTinyGo() {
	// 空函数，仅用于满足TinyGo的main函数要求
	// 实际的execute逻辑在下面的execute导出函数中
}

// execute 是TinyGo WASM导出的主入口点
// 由WASM运行时调用，接收JSON格式的请求
//
//export execute
func execute(ptr uint32, length uint32) uint64 {
	input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

	// 持久化副本，避免TinyGo GC问题
	SetInputBuf(make([]byte, length))
	copy(GetInputBuf(), input)

	// 手动JSON解析，避免TinyGo json.Unmarshal的UTF-8损坏问题
	toolVal := extractStringField(GetInputBuf(), "tool")
	argsVal := extractRawField(GetInputBuf(), "args")
	sandboxVal := extractStringField(GetInputBuf(), "sandbox_root")

	resp := CallTool(toolVal, json.RawMessage(argsVal), sandboxVal, NewTinyGoHostAPI())
	if resp.Error != "" {
		SetResultBuf(jsonError(resp.Error))
	} else {
		SetResultBuf(resp.Output)
	}
	return PackResult(GetResultBuf())
}

// extractStringField 从JSON数据中提取字符串字段
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

// extractRawField 从JSON数据中提取原始字段（支持嵌套对象）
func extractRawField(data []byte, key string) []byte {
	keyBytes := []byte(`"` + key + `":`)
	for i := 0; i <= len(data)-len(keyBytes); i++ {
		if matchAt(data, i, keyBytes) {
			start := i + len(keyBytes)
			// 跳过空白字符
			for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n' || data[start] == '\r') {
				start++
			}
			if start >= len(data) {
				return nil
			}
			end := start + 1
			if data[start] == '{' || data[start] == '[' {
				// 嵌套对象/数组
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
				// 字符串
				end = start + 1
				for end < len(data) {
					if data[end] == '"' && data[end-1] != '\\' {
						end++
						break
					}
					end++
				}
			} else {
				// 其他值（数字、布尔等）
				for end < len(data) && data[end] != ',' && data[end] != '}' {
					end++
				}
			}
			return data[start:end]
		}
	}
	return nil
}

// matchAt 检查data[offset:]是否以pattern开头
func matchAt(data []byte, offset int, pattern []byte) bool {
	for j := 0; j < len(pattern); j++ {
		if data[offset+j] != pattern[j] {
			return false
		}
	}
	return true
}

// jsonError 创建错误响应的JSON
func jsonError(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

// ExecuteTinyGoRequest 解析JSON请求并执行工具（stdio模式备选）
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

// PackResult 将数据打包为(ptr<<32)|length的uint64格式
func PackResult(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	ptr := uint64(uintptr(unsafe.Pointer(&data[0])))
	length := uint64(len(data))
	return (ptr << 32) | length
}

// SetResultBuf 存储结果到持久化缓冲区，防止GC
func SetResultBuf(data []byte) {
	resultBuf = data
}

// GetResultBuf 返回当前结果缓冲区
func GetResultBuf() []byte {
	return resultBuf
}

// SetInputBuf 存储输入到持久化缓冲区，防止GC
func SetInputBuf(data []byte) {
	inputBuf = data
}

// GetInputBuf 返回当前输入缓冲区
func GetInputBuf() []byte {
	return inputBuf
}

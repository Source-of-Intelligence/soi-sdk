// Package sdk provides the SOI plugin development kit for WASM plugins.
// This file contains symbols shared by both Go and TinyGo builds.
package sdk

import (
	"encoding/json"
	"fmt"

	"github.com/Source-of-Intelligence/soi-vos"
)

const (
	SDKVersion = "2.0.0"
	ABIVersion = vos.ABIVersion
)

var buildTag = "go"

// SetBuildTag allows TinyGo init to override the build tag.
func SetBuildTag(tag string) {
	buildTag = tag
}

// ToolHandler — regular (non-sandbox) plugin handler.
type ToolHandler func(args json.RawMessage) (interface{}, error)

// SOIToolHandler — SOI-level plugin handler with sandbox access.
type SOIToolHandler func(args json.RawMessage, ctx *SandboxContext) (interface{}, error)

// HostAPI defines host-provided capabilities available to SOI plugins.
// It is now an alias for vos.HostFunctions to maintain backward compatibility.
// Existing code using sdk.HostAPI will continue to work.
type HostAPI = vos.HostFunctions

// SandboxContext is injected into every SOIToolHandler call.
type SandboxContext struct {
	SandboxRoot string
	Host        HostAPI
}

// ToolExample provides usage example for a tool.
type ToolExample struct {
	Input  map[string]interface{} `json:"input" yaml:"input"`
	Output string                 `json:"output" yaml:"output"`
}

// ToolDef describes a tool's metadata.
type ToolDef struct {
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Parameters  []ParamDef    `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Returns     string        `json:"returns,omitempty" yaml:"returns,omitempty"`
	Examples    []ToolExample `json:"examples,omitempty" yaml:"examples,omitempty"`
	// Uses is kept for backward compatibility but plugin-level uses is preferred
	Uses []string `json:"uses,omitempty" yaml:"uses,omitempty"`
}

// ProvidesTrigger defines trigger configuration at provides level.
type ProvidesTrigger struct {
	Keywords []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	Prefix   string   `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Regex    string   `json:"regex,omitempty" yaml:"regex,omitempty"`
	Events   []string `json:"events,omitempty" yaml:"events,omitempty"`
	Priority int      `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// Predefined sandbox capability constants
const (
	// SandboxFS enables sandbox file system access (ReadFile / WriteFile / ListDir)
	SandboxFS = "sandbox_fs"
	// HostLog enables logging output (ctx.Host.Log)
	HostLog = "host_log"
	// HostNow enables timestamp access (ctx.Host.Now)
	HostNow = "host_now"
	// HostRandom enables secure random number generation (ctx.Host.Random)
	HostRandom = "host_random"
	// HostHTTP enables HTTP requests (ctx.Host.HTTP)
	HostHTTP = "host_http"
	// HostEnv enables environment variable access (ctx.Host.Env)
	HostEnv = "host_env"
	// HostProcess enables process management (ctx.Host.Process)
	HostProcess = "host_process"
)

// ParamDef describes a single parameter.
type ParamDef struct {
	Name        string      `json:"name" yaml:"name"`
	Type        string      `json:"type" yaml:"type"`
	Required    bool        `json:"required" yaml:"required"`
	Default     interface{} `json:"default,omitempty" yaml:"default,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Enum        []string    `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// Manifest is the plugin's self-describing metadata.
type Manifest struct {
	SDKVersion string    `json:"sdk_version"`
	ABIVersion string    `json:"abi_version"`
	Tools      []ToolDef `json:"tools"`
	BuildTag   string    `json:"build_tag"`
}

// ExecuteResponse is the result of executing a tool.
type ExecuteResponse struct {
	Output []byte
	Error  string
}

// registeredTool holds a regular tool's handler + definition.
type registeredTool struct {
	Handler ToolHandler
	Def     ToolDef
}

// registeredSOITool holds a SOI tool's handler + definition.
type registeredSOITool struct {
	Handler SOIToolHandler
	Def     ToolDef
}

var toolRegistry = make(map[string]registeredTool)
var soiToolRegistry = make(map[string]registeredSOITool)
var pluginUses []string             // Plugin-level sandbox capabilities
var providesTrigger ProvidesTrigger // Provides-level trigger configuration

// RegisterTool registers a regular tool (no sandbox access).
func RegisterTool(name string, handler ToolHandler) {
	toolRegistry[name] = registeredTool{Handler: handler, Def: ToolDef{Name: name}}
}

// RegisterToolWithDef registers a regular tool with full metadata.
func RegisterToolWithDef(def ToolDef, handler ToolHandler) {
	toolRegistry[def.Name] = registeredTool{Handler: handler, Def: def}
}

// RegisterSOITool registers a SOI tool (with sandbox access).
func RegisterSOITool(def ToolDef, handler SOIToolHandler) {
	soiToolRegistry[def.Name] = registeredSOITool{Handler: handler, Def: def}
}

// GetTools returns all regular tool handlers.
func GetTools() map[string]ToolHandler {
	result := make(map[string]ToolHandler, len(toolRegistry))
	for name, rt := range toolRegistry {
		result[name] = rt.Handler
	}
	return result
}

// GetSOITools returns all SOI tool handlers.
func GetSOITools() map[string]SOIToolHandler {
	result := make(map[string]SOIToolHandler, len(soiToolRegistry))
	for name, rt := range soiToolRegistry {
		result[name] = rt.Handler
	}
	return result
}

// GetManifest returns the merged manifest from both registries.
func GetManifest() Manifest {
	tools := GetToolDefs()
	return Manifest{
		SDKVersion: SDKVersion,
		ABIVersion: ABIVersion,
		Tools:      tools,
		BuildTag:   buildTag,
	}
}

// GetToolDefs returns tool definitions from both registries.
func GetToolDefs() []ToolDef {
	defs := make([]ToolDef, 0, len(toolRegistry)+len(soiToolRegistry))
	for _, rt := range toolRegistry {
		defs = append(defs, rt.Def)
	}
	for _, rt := range soiToolRegistry {
		defs = append(defs, rt.Def)
	}
	return defs
}

// SetPluginUses sets the plugin-level sandbox capabilities.
func SetPluginUses(capabilities ...string) {
	pluginUses = make([]string, len(capabilities))
	copy(pluginUses, capabilities)
}

// GetPluginUses returns the plugin-level sandbox capabilities.
func GetPluginUses() []string {
	if pluginUses == nil {
		return []string{}
	}
	result := make([]string, len(pluginUses))
	copy(result, pluginUses)
	return result
}

// SetProvidesTrigger sets the provides-level trigger configuration.
func SetProvidesTrigger(keywords []string, prefix string, regex string, events []string, priority int) {
	providesTrigger = ProvidesTrigger{
		Keywords: keywords,
		Prefix:   prefix,
		Regex:    regex,
		Events:   events,
		Priority: priority,
	}
}

// GetProvidesTrigger returns the provides-level trigger configuration.
func GetProvidesTrigger() ProvidesTrigger {
	return providesTrigger
}

// CallTool is a convenience wrapper for ExecuteTool that automatically uses
// the global tool registries. Use this in tests and plugin code instead of
// manually calling GetTools() and GetSOITools().
func CallTool(toolName string, args json.RawMessage, sandboxRoot string, host HostAPI) ExecuteResponse {
	return ExecuteTool(GetTools(), GetSOITools(), toolName, args, sandboxRoot, host)
}

// ExecuteTool dispatches a tool call through both registries.
// If the tool is found in the regular registry, it is called without sandbox context.
// If found in the SOI registry, a SandboxContext is constructed and passed.
//
// For most use cases, prefer CallTool() which automatically uses the global registries.
func ExecuteTool(
	handlers map[string]ToolHandler,
	soiHandlers map[string]SOIToolHandler,
	toolName string,
	args json.RawMessage,
	sandboxRoot string,
	host HostAPI,
) ExecuteResponse {
	// 1) regular handler (NO sandbox access)
	if handler, ok := handlers[toolName]; ok {
		result, err := handler(args)
		if err != nil {
			return ExecuteResponse{Error: err.Error()}
		}
		data, err := json.Marshal(result)
		if err != nil {
			return ExecuteResponse{Error: "marshal result: " + err.Error()}
		}
		return ExecuteResponse{Output: data}
	}

	// 2) SOI handler (WITH sandbox / host context)
	if handler, ok := soiHandlers[toolName]; ok {
		ctx := &SandboxContext{SandboxRoot: sandboxRoot, Host: host}
		result, err := handler(args, ctx)
		if err != nil {
			return ExecuteResponse{Error: err.Error()}
		}
		data, err := json.Marshal(result)
		if err != nil {
			return ExecuteResponse{Error: "marshal result: " + err.Error()}
		}
		return ExecuteResponse{Output: data}
	}

	return ExecuteResponse{Error: "unknown tool: " + toolName}
}

// ==========================================
// 简化的 API (SDK v2 新增)
// ==========================================

// Builder 工具构建器 - 链式API，简化工具注册
type Builder struct {
	def ToolDef
}

// NewTool 创建一个新的工具构建器。如果名称为空，默认使用"execute"
func NewTool(name string) *Builder {
	if name == "" {
		name = "execute"
	}
	return &Builder{
		def: ToolDef{Name: name},
	}
}

// Desc 设置工具描述
func (b *Builder) Desc(description string) *Builder {
	b.def.Description = description
	return b
}

// Param 添加一个参数
func (b *Builder) Param(name, paramType string, required bool, defaultVal interface{}, description string) *Builder {
	b.def.Parameters = append(b.def.Parameters, ParamDef{
		Name:        name,
		Type:        paramType,
		Required:    required,
		Default:     defaultVal,
		Description: description,
	})
	return b
}

// Returns 设置返回值说明
func (b *Builder) Returns(returns string) *Builder {
	b.def.Returns = returns
	return b
}

// Example 添加一个使用示例
func (b *Builder) Example(input map[string]interface{}, output string) *Builder {
	b.def.Examples = append(b.def.Examples, ToolExample{
		Input:  input,
		Output: output,
	})
	return b
}

// TriggerKeywords 设置 provides 级别的关键词触发
// 示例：TriggerKeywords("word", "docx", "document")
func (b *Builder) TriggerKeywords(keywords ...string) *Builder {
	providesTrigger.Keywords = keywords
	return b
}

// TriggerPrefix 设置 provides 级别的前缀触发
// 示例：TriggerPrefix("/")
func (b *Builder) TriggerPrefix(prefix string) *Builder {
	providesTrigger.Prefix = prefix
	return b
}

// TriggerRegex 设置 provides 级别的正则表达式触发
// 示例：TriggerRegex(`^file:\/\/.*`)
func (b *Builder) TriggerRegex(pattern string) *Builder {
	providesTrigger.Regex = pattern
	return b
}

// TriggerEvents 设置 provides 级别的事件触发
// 示例：TriggerEvents("file_created", "timer")
func (b *Builder) TriggerEvents(events ...string) *Builder {
	providesTrigger.Events = events
	return b
}

// TriggerPriority 设置 provides 级别的触发优先级
// 优先级越高的工具会优先匹配
func (b *Builder) TriggerPriority(priority int) *Builder {
	providesTrigger.Priority = priority
	return b
}

// WithSandbox 声明工具需要的沙箱能力
// 支持的沙箱能力：
//   - sandbox_fs : 沙箱文件系统读写 (ctx.Host.SandboxRead / SandboxWrite / SandboxList)
//   - host_log   : 日志输出 (ctx.Host.Log)
//   - host_now   : 时间戳 (ctx.Host.Now)
//   - host_random: 安全随机数 (ctx.Host.Random)
//   - host_http  : HTTP请求 (ctx.Host.HTTP)
//   - host_env   : 环境变量 (ctx.Host.Env)
//   - host_process: 进程管理 (ctx.Host.Process)
//
// 示例：
//
//	WithSandbox(sdk.SandboxFS, sdk.HostLog)
//	WithSandbox(sdk.SandboxFS, sdk.HostRandom)
func (b *Builder) WithSandbox(capabilities ...string) *Builder {
	b.def.Uses = append(b.def.Uses, capabilities...)
	return b
}

// WithSandboxFS 声明需要沙箱文件系统访问
// 等同于 WithSandbox(sdk.SandboxFS)
func (b *Builder) WithSandboxFS() *Builder {
	return b.WithSandbox(SandboxFS)
}

// WithHostLog 声明需要日志输出能力
// 等同于 WithSandbox(sdk.HostLog)
func (b *Builder) WithHostLog() *Builder {
	return b.WithSandbox(HostLog)
}

// WithHostNow 声明需要时间戳能力
// 等同于 WithSandbox(sdk.HostNow)
func (b *Builder) WithHostNow() *Builder {
	return b.WithSandbox(HostNow)
}

// WithHostRandom 声明需要随机数能力
// 等同于 WithSandbox(sdk.HostRandom)
func (b *Builder) WithHostRandom() *Builder {
	return b.WithSandbox(HostRandom)
}

// WithHostHTTP 声明需要HTTP请求能力
// 等同于 WithSandbox(sdk.HostHTTP)
func (b *Builder) WithHostHTTP() *Builder {
	return b.WithSandbox(HostHTTP)
}

// WithHostEnv 声明需要环境变量能力
// 等同于 WithSandbox(sdk.HostEnv)
func (b *Builder) WithHostEnv() *Builder {
	return b.WithSandbox(HostEnv)
}

// WithHostProcess 声明需要进程管理能力
// 等同于 WithSandbox(sdk.HostProcess)
func (b *Builder) WithHostProcess() *Builder {
	return b.WithSandbox(HostProcess)
}

// RegisterSimple 注册普通工具（无沙箱）
func (b *Builder) RegisterSimple(handler ToolHandler) {
	RegisterToolWithDef(b.def, handler)
}

// RegisterSOI 注册SOI工具（有沙箱）
func (b *Builder) RegisterSOI(handler SOIToolHandler) {
	RegisterSOITool(b.def, handler)
}

// ParseArgs 解析参数到结构体 - 类型安全
func ParseArgs[T any](args json.RawMessage) (*T, error) {
	var result T
	if err := json.Unmarshal(args, &result); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	return &result, nil
}

// MustParseArgs 必须解析成功，否则返回错误
func MustParseArgs[T any](args json.RawMessage) (*T, error) {
	return ParseArgs[T](args)
}

// ParseArgsMap 解析参数为 map
func ParseArgsMap(args json.RawMessage) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(args, &result); err != nil {
		return nil, fmt.Errorf("parse args map: %w", err)
	}
	if result == nil {
		return make(map[string]interface{}), nil
	}
	return result, nil
}

// GetString 从 map 安全地获取字符串
func GetString(m map[string]interface{}, key string, defaultValue string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

// GetFloat64 从 map 安全地获取 float64
func GetFloat64(m map[string]interface{}, key string, defaultValue float64) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return defaultValue
}

// GetInt 从 map 安全地获取 int
func GetInt(m map[string]interface{}, key string, defaultValue int) int {
	f := GetFloat64(m, key, float64(defaultValue))
	return int(f)
}

// GetBool 从 map 安全地获取 bool
func GetBool(m map[string]interface{}, key string, defaultValue bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultValue
}

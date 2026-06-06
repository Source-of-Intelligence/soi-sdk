// Package sdk provides the SOI plugin development kit for WASM plugins.
// This file contains symbols shared by both Go and TinyGo builds.
package sdk

import (
	"encoding/json"

	"github.com/Source-of-Intelligence/soi-vos"
)

const (
	SDKVersion = "1.0.0"
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
}

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

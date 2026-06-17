package sdk

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// ============================================================
// S-01: 有效 Manifest 测试
// ============================================================
func TestS01_ValidManifest(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	// Register a simple tool
	RegisterTool("echo", func(args json.RawMessage) (interface{}, error) {
		return "echo response", nil
	})

	manifest := GetManifest()

	if manifest.SDKVersion != SDKVersion {
		t.Errorf("Expected SDKVersion %s, got %s", SDKVersion, manifest.SDKVersion)
	}

	if manifest.ABIVersion != ABIVersion {
		t.Errorf("Expected ABIVersion %s, got %s", ABIVersion, manifest.ABIVersion)
	}

	if len(manifest.Tools) == 0 {
		t.Error("Expected at least one tool in manifest")
	}
}

// ============================================================
// S-02: 工具注册测试
// ============================================================
func TestS02_ToolRegistration(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("add", func(args json.RawMessage) (interface{}, error) {
		var params struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		return params.A + params.B, nil
	})

	handlers := GetTools()
	if _, ok := handlers["add"]; !ok {
		t.Error("Tool 'add' not found in registry")
	}
}

// ============================================================
// S-03: 工具执行测试
// ============================================================
func TestS03_ToolExecution(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("multiply", func(args json.RawMessage) (interface{}, error) {
		var params struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		return params.A * params.B, nil
	})

	args := json.RawMessage(`{"a": 3, "b": 4}`)
	// Pass nil for host - testing without actual host implementation
	result := CallTool("multiply", args, "", nil)

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	var output float64
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if output != 12.0 {
		t.Errorf("Expected 12.0, got %f", output)
	}
}

// ============================================================
// S-04: SOI 工具注册与执行测试
// ============================================================
func TestS04_SOIToolExecution(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterSOITool(ToolDef{Name: "sandbox_echo"}, func(args json.RawMessage, ctx *SandboxContext) (interface{}, error) {
		var params struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"echo":    params.Message,
			"sandbox": ctx.SandboxRoot,
		}, nil
	})

	args := json.RawMessage(`{"message": "hello"}`)
	result := CallTool("sandbox_echo", args, "/sandbox/path", nil)

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	var output map[string]interface{}
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if output["echo"] != "hello" {
		t.Errorf("Expected echo 'hello', got %v", output["echo"])
	}
	if output["sandbox"] != "/sandbox/path" {
		t.Errorf("Expected sandbox '/sandbox/path', got %v", output["sandbox"])
	}
}

// ============================================================
// S-05: 未知工具测试
// ============================================================
func TestS05_UnknownTool(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	result := CallTool("non_existent_tool", json.RawMessage(`{}`), "", nil)

	if result.Error == "" {
		t.Error("Expected error for unknown tool, got nil")
	}

	if result.Error != "unknown tool: non_existent_tool" {
		t.Errorf("Expected 'unknown tool: non_existent_tool', got '%s'", result.Error)
	}
}

// ============================================================
// S-06: 工具执行错误处理测试
// ============================================================
func TestS06_ToolErrorHandling(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("error_tool", func(args json.RawMessage) (interface{}, error) {
		return nil, errors.New("intentional error")
	})

	result := CallTool("error_tool", json.RawMessage(`{}`), "", nil)

	if result.Error == "" {
		t.Error("Expected error from tool, got nil")
	}

	if result.Error != "intentional error" {
		t.Errorf("Expected 'intentional error', got '%s'", result.Error)
	}
}

// ============================================================
// S-07: ParseArgs 泛型解析测试
// ============================================================
func TestS07_ParseArgs(t *testing.T) {
	args := json.RawMessage(`{"name": "test", "value": 42}`)

	type MyArgs struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	parsed, err := ParseArgs[MyArgs](args)
	if err != nil {
		t.Fatalf("ParseArgs failed: %v", err)
	}

	if parsed.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", parsed.Name)
	}
	if parsed.Value != 42 {
		t.Errorf("Expected value 42, got %d", parsed.Value)
	}
}

// ============================================================
// S-08: ParseArgsMap 测试
// ============================================================
func TestS08_ParseArgsMap(t *testing.T) {
	args := json.RawMessage(`{"key1": "value1", "key2": 123}`)

	result, err := ParseArgsMap(args)
	if err != nil {
		t.Fatalf("ParseArgsMap failed: %v", err)
	}

	if result["key1"] != "value1" {
		t.Errorf("Expected key1='value1', got %v", result["key1"])
	}

	if result["key2"].(float64) != 123 {
		t.Errorf("Expected key2=123, got %v", result["key2"])
	}
}

// ============================================================
// S-09: GetString 测试
// ============================================================
func TestS09_GetString(t *testing.T) {
	m := map[string]interface{}{
		"existing": "hello",
		"missing":  123,
		"empty":    "",
	}

	if GetString(m, "existing", "default") != "hello" {
		t.Error("GetString failed for existing key")
	}

	if GetString(m, "non_existent", "default") != "default" {
		t.Error("GetString failed for missing key")
	}

	if GetString(m, "empty", "default") != "" {
		t.Error("GetString should return empty string, not default")
	}

	if GetString(m, "missing", "default") != "default" {
		t.Error("GetString should return default for non-string value")
	}
}

// ============================================================
// S-10: GetFloat64 测试
// ============================================================
func TestS10_GetFloat64(t *testing.T) {
	m := map[string]interface{}{
		"value":     float64(3.14),
		"int_val":   float64(100),
		"string":    "not a number",
		"non_exist": nil,
	}

	if GetFloat64(m, "value", 0) != 3.14 {
		t.Error("GetFloat64 failed for float64 value")
	}

	if GetFloat64(m, "int_val", 0) != 100 {
		t.Error("GetFloat64 failed for int value")
	}

	if GetFloat64(m, "string", 0) != 0 {
		t.Error("GetFloat64 should return default for string value")
	}

	if GetFloat64(m, "non_exist", 42.0) != 42.0 {
		t.Error("GetFloat64 should return default for non-existent key")
	}
}

// ============================================================
// S-11: GetInt 测试
// ============================================================
func TestS11_GetInt(t *testing.T) {
	m := map[string]interface{}{
		"value": float64(42),
		"zero":  float64(0),
	}

	if GetInt(m, "value", 0) != 42 {
		t.Error("GetInt failed")
	}

	if GetInt(m, "zero", -1) != 0 {
		t.Error("GetInt failed for zero value")
	}

	if GetInt(m, "non_exist", -1) != -1 {
		t.Error("GetInt should return default for non-existent key")
	}
}

// ============================================================
// S-12: GetBool 测试
// ============================================================
func TestS12_GetBool(t *testing.T) {
	m := map[string]interface{}{
		"true_val":  true,
		"false_val": false,
		"string":    "not bool",
	}

	if !GetBool(m, "true_val", false) {
		t.Error("GetBool failed for true value")
	}

	if GetBool(m, "false_val", true) {
		t.Error("GetBool failed for false value")
	}

	if GetBool(m, "string", false) {
		t.Error("GetBool should return default for string value")
	}

	if !GetBool(m, "non_exist", true) {
		t.Error("GetBool should return default for non-existent key")
	}
}

// ============================================================
// S-13: Builder API 测试
// ============================================================
func TestS13_BuilderAPI(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	NewTool("my_tool").
		Desc("A test tool").
		Param("input", "string", true, nil, "Input string").
		Returns("string").
		Example(map[string]interface{}{"input": "test"}, "processed: test").
		WithSandbox(SandboxFS, HostLog).
		RegisterSimple(func(args json.RawMessage) (interface{}, error) {
			m, _ := ParseArgsMap(args)
			input := GetString(m, "input", "")
			return "processed: " + input, nil
		})

	manifest := GetManifest()
	if len(manifest.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(manifest.Tools))
	}

	tool := manifest.Tools[0]
	if tool.Name != "my_tool" {
		t.Errorf("Expected name 'my_tool', got '%s'", tool.Name)
	}
	if tool.Description != "A test tool" {
		t.Errorf("Expected description 'A test tool', got '%s'", tool.Description)
	}
	if len(tool.Parameters) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(tool.Parameters))
	}
	if len(tool.Uses) != 2 {
		t.Errorf("Expected 2 sandbox uses, got %d", len(tool.Uses))
	}
}

// ============================================================
// S-14: 链式 Builder 测试
// ============================================================
func TestS14_ChainedBuilder(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	// Test chained SOI registration
	NewTool("chained_soi").
		Desc("Chained SOI tool").
		TriggerKeywords("keyword1", "keyword2").
		TriggerPrefix("/cmd").
		TriggerPriority(10).
		WithSandbox(HostNow).
		RegisterSOI(func(args json.RawMessage, ctx *SandboxContext) (interface{}, error) {
			return "ok", nil
		})

	// Execute it
	result := CallTool("chained_soi", json.RawMessage(`{}`), "", nil)
	if result.Error != "" {
		t.Errorf("Chained tool execution failed: %s", result.Error)
	}
}

// ============================================================
// S-15: 工具列表获取测试
// ============================================================
func TestS15_GetToolDefs(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("tool1", func(args json.RawMessage) (interface{}, error) { return nil, nil })
	RegisterSOITool(ToolDef{Name: "tool2", Description: "SOI tool"}, func(args json.RawMessage, ctx *SandboxContext) (interface{}, error) { return nil, nil })

	defs := GetToolDefs()
	if len(defs) != 2 {
		t.Errorf("Expected 2 tool definitions, got %d", len(defs))
	}

	found := make(map[string]bool)
	for _, d := range defs {
		found[d.Name] = true
	}

	if !found["tool1"] {
		t.Error("tool1 not found in GetToolDefs")
	}
	if !found["tool2"] {
		t.Error("tool2 not found in GetToolDefs")
	}
}

// ============================================================
// S-16: SDK 版本常量测试
// ============================================================
func TestS16_SDKVersion(t *testing.T) {
	if SDKVersion == "" {
		t.Error("SDKVersion should not be empty")
	}

	if ABIVersion == "" {
		t.Error("ABIVersion should not be empty")
	}
}

// ============================================================
// S-17: SandboxContext 结构测试
// ============================================================
func TestS17_SandboxContext(t *testing.T) {
	ctx := &SandboxContext{
		SandboxRoot: "/test/root",
		Host:        nil, // nil is valid for HostAPI interface
	}

	if ctx.SandboxRoot != "/test/root" {
		t.Errorf("Expected SandboxRoot '/test/root', got '%s'", ctx.SandboxRoot)
	}
}

// ============================================================
// S-18: 沙箱能力常量测试
// ============================================================
func TestS18_SandboxCapabilities(t *testing.T) {
	caps := []string{
		SandboxFS,
		HostLog,
		HostNow,
		HostRandom,
		HostHTTP,
		HostEnv,
		HostProcess,
	}

	expected := []string{
		"sandbox_fs",
		"host_log",
		"host_now",
		"host_random",
		"host_http",
		"host_env",
		"host_process",
	}

	for i, cap := range caps {
		if cap != expected[i] {
			t.Errorf("Expected capability '%s', got '%s'", expected[i], cap)
		}
	}
}

// ============================================================
// S-19: ToolDef 结构测试
// ============================================================
func TestS19_ToolDef(t *testing.T) {
	def := ToolDef{
		Name:        "test",
		Description: "A test tool",
		Parameters: []ParamDef{
			{Name: "arg1", Type: "string", Required: true},
			{Name: "arg2", Type: "number", Required: false, Default: 10},
		},
		Returns: "string",
		Uses:    []string{SandboxFS},
	}

	if def.Name != "test" {
		t.Error("ToolDef.Name mismatch")
	}
	if len(def.Parameters) != 2 {
		t.Error("ToolDef.Parameters length mismatch")
	}
	if !def.Parameters[0].Required {
		t.Error("First param should be required")
	}
	if def.Parameters[1].Required {
		t.Error("Second param should not be required")
	}
}

// ============================================================
// S-20: ExecuteResponse 结构测试
// ============================================================
func TestS20_ExecuteResponse(t *testing.T) {
	resp := ExecuteResponse{
		Output: []byte(`{"result": "success"}`),
		Error:  "",
	}

	if len(resp.Output) == 0 {
		t.Error("ExecuteResponse.Output should not be empty")
	}

	resp2 := ExecuteResponse{
		Output: nil,
		Error:  "some error",
	}

	if resp2.Error != "some error" {
		t.Error("ExecuteResponse.Error mismatch")
	}
}

// ============================================================
// S-21: JSON RawMessage 参数测试
// ============================================================
func TestS21_JSONRawMessage(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("complex_args", func(args json.RawMessage) (interface{}, error) {
		var params struct {
			Nested struct {
				Deep string `json:"deep"`
			} `json:"nested"`
			Array []int `json:"array"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"deep":   params.Nested.Deep,
			"length": len(params.Array),
		}, nil
	})

	args := json.RawMessage(`{"nested": {"deep": "value"}, "array": [1, 2, 3]}`)
	result := CallTool("complex_args", args, "", nil)

	if result.Error != "" {
		t.Errorf("Complex args test failed: %s", result.Error)
	}

	var output map[string]interface{}
	json.Unmarshal(result.Output, &output)

	if output["deep"] != "value" {
		t.Errorf("Expected deep='value', got %v", output["deep"])
	}
	if output["length"].(float64) != 3 {
		t.Error("Expected array length 3")
	}
}

// ============================================================
// S-22: BuildTag 测试
// ============================================================
func TestS22_BuildTag(t *testing.T) {
	if buildTag == "" {
		t.Error("buildTag should not be empty")
	}

	manifest := GetManifest()
	if manifest.BuildTag != buildTag {
		t.Errorf("Expected BuildTag '%s', got '%s'", buildTag, manifest.BuildTag)
	}
}

// ============================================================
// S-23: PluginUses 测试
// ============================================================
func TestS23_PluginUses(t *testing.T) {
	// Reset
	pluginUses = nil

	SetPluginUses(SandboxFS, HostLog, HostHTTP)

	uses := GetPluginUses()
	if len(uses) != 3 {
		t.Errorf("Expected 3 plugin uses, got %d", len(uses))
	}

	expected := []string{SandboxFS, HostLog, HostHTTP}
	for i, u := range uses {
		if u != expected[i] {
			t.Errorf("PluginUse[%d]: expected '%s', got '%s'", i, expected[i], u)
		}
	}
}

// ============================================================
// S-24: GetTools 返回类型测试
// ============================================================
func TestS24_GetToolsReturnType(t *testing.T) {
	// Reset registries
	toolRegistry = make(map[string]registeredTool)
	soiToolRegistry = make(map[string]registeredSOITool)

	RegisterTool("type_test", func(args json.RawMessage) (interface{}, error) {
		return "test", nil
	})

	handlers := GetTools()
	handlerType := reflect.TypeOf(handlers["type_test"])

	// Handler should be func(json.RawMessage) (interface{}, error)
	if handlerType.Kind() != reflect.Func {
		t.Error("GetTools should return functions")
	}

	if handlerType.NumIn() != 1 {
		t.Error("Handler should have 1 input parameter")
	}

	if handlerType.NumOut() != 2 {
		t.Error("Handler should have 2 output parameters")
	}
}

// ============================================================
// S-25: WasmConfig 测试
// ============================================================
func TestS25_WasmConfig(t *testing.T) {
	// Reset
	pluginWasmConfig = WasmConfig{}

	SetPluginWasmConfig("/plugins", "60s")

	cfg := GetPluginWasmConfig()
	if cfg.SandboxSubdir != "/plugins" {
		t.Errorf("Expected SandboxSubdir '/plugins', got '%s'", cfg.SandboxSubdir)
	}
	if cfg.Timeout != "60s" {
		t.Errorf("Expected Timeout '60s', got '%s'", cfg.Timeout)
	}
}

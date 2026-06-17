# SOI SDK

**SOI (Source of Intelligence) Plugin Development Kit** for building WASM plugins.

## Requirements

- Go 1.22+
- (optional) TinyGo for export ABI (.soi)
- (optional) Rust 1.75+ for Rust plugins (`cargo` / `rustup target add wasm32-wasip1`)

## Quick Start

```go
package main

import "github.com/Source-of-Intelligence/soi-sdk"

func init() {
    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "add",
        Description: "Add two numbers",
        Parameters:  []sdk.ParamDef{
            {Name: "A", Type: "number", Required: true},
            {Name: "B", Type: "number", Required: true},
        },
        Returns: `{"result": number}`,
    }, func(args json.RawMessage) (interface{}, error) {
        var p struct{ A, B float64 }
        json.Unmarshal(args, &p)
        return map[string]float64{"result": p.A + p.B}, nil
    })
}

func main() { sdk.Run() }
```

## Build

```powershell
# Standard Go WASM (default compiler)
$env:GOOS="wasip1"; $env:GOARCH="wasm"; $env:CGO_ENABLED="0"
go build -o wasm/plugin.wasm .

# TinyGo export ABI (smaller output)
tinygo build -target=wasi -o wasm/plugin.soi .

# Rust WASM (compiler=rust)
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1
# then copy target/wasm32-wasip1/release/<name>.wasm to wasm/plugin.wasm
```

## Verification

```powershell
# Run tests
go test ./...

# CLI verification tool
go run ./cmd/soi-verify --tool add --args '{"A":3,"B":5}'
```

## Project Structure

- `core.go` — Core types, registry, and ExecuteTool
- `run_stdio.go` — Stdio ABI (standard Go wasip1/wasm)
- `host_tinygo.go` — Export ABI (TinyGo //go:build tinygo)
- `compiler.go` — Build driver for go / tinygo / rust (used by soi-package)
- `manifest.go` — Manifest serialization and skill.yaml generation
- `cmd/soi-package/` — Plugin packaging tool (supports --compiler go|tinygo|rust)
- `cmd/soi-create/` — Project creation tool (scaffold/wrap, supports --compiler go|tinygo|rust)
- `cmd/soi-verify/` — CLI verification tool
- `../soi-sdk-rs/` — Rust SDK (mirrors the Go SDK; enables `--compiler rust`)

---

# 目录

1. [快速入门指南](#快速入门指南)
2. [设计流程](#设计流程)
3. [代码结构](#代码结构)
4. [创建过程](#创建过程)
5. [使用逻辑](#使用逻辑)
6. [SDK新使用方式说明](#sdk新使用方式说明)
7. [重构总结](#重构总结)
8. [Trigger功能指南](#trigger功能指南)
9. [Sandbox Uses功能指南](#sandbox-uses功能指南)
10. [自动同步工具使用指南](#自动同步工具使用指南)

---

# 快速入门指南

## 创建你的第一个 SOI 插件

### 1. 使用脚手架创建项目

```bash
# 进入项目目录
cd e:\code\soi\soi-plugin

# 创建新插件（默认 Go 编译器, type=wasm）
soi-create scaffold --name my-plugin --type wasm --compiler go

# 使用 TinyGo 编译器（更小体积的 .soi 输出）
soi-create scaffold --name my-plugin --type soi --compiler tinygo

# 使用 Rust 编译器（生成 Cargo.toml / src/lib.rs）
soi-create scaffold --name my-plugin --type wasm --compiler rust

# 进入插件目录
cd my-plugin
```

### 2. 查看生成的文件

```
my-plugin/
├── main.go          # 你的插件代码
├── main_test.go     # 测试代码
├── go.mod           # Go模块配置
├── skill.yaml       # 插件元数据
└── wasm/            # 编译输出目录
```

### 3. 编辑 main.go

```go
package main

import (
	"encoding/json"
	"fmt"
	sdk "github.com/Source-of-Intelligence/soi-sdk"
)

func init() {
	registerTools()
}

//export registerTools
func registerTools() {
	sdk.NewTool("hello").
		Desc("Say hello to someone").
		Param("name", "string", true, "World", "Name to greet").
		Returns("object with greeting message").
		RegisterSimple(handler)
}

func handler(args json.RawMessage) (interface{}, error) {
	var p struct{ Name string }
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}
	
	return map[string]interface{}{
		"message": fmt.Sprintf("Hello, %s!", p.Name),
	}, nil
}

func main() {
	sdk.RunTinyGo()
}
```

### 4. 构建插件

```bash
# Go 编译器（默认）
$env:GOOS="wasip1"; $env:GOARCH="wasm"; $env:CGO_ENABLED="0"
go build -o wasm/plugin.wasm .

# TinyGo 编译器（更小体积）
tinygo build -target=wasi -o wasm/plugin.soi .

# Rust 编译器
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1
```

### 5. 测试插件

```bash
# Go / TinyGo 插件
go test -v

# Rust 插件
cargo test
```

### 6. 打包插件

```bash
# Go 编译器（默认）
soi-package --dir . --compiler go

# TinyGo 编译器
soi-package --dir . --compiler tinygo

# Rust 编译器
soi-package --dir . --compiler rust
```

## SOI 插件 vs WASM 插件

| 特性 | SOI 插件 | WASM 插件 |
|------|---------|---------|
| 沙箱访问 | ✅ 支持 | ❌ 不支持 |
| 文件系统 | ✅ 可读写 | ❌ 只读 |
| 构建命令 | `tinygo build` | `go build` |
| 输出文件 | `plugin.soi` | `plugin.wasm` |
| 工具类型 | `RegisterSOI` | `RegisterSimple` |

## 注册工具的三种方式

### 1. 简单工具（无沙箱）

```go
sdk.NewTool("tool_name").
	Desc("Description").
	Param("param1", "string", true, "default", "Description").
	Returns("object with result").
	RegisterSimple(handler)

// Handler签名
func handler(args json.RawMessage) (interface{}, error)
```

### 2. SOI 工具（有沙箱）

```go
sdk.NewTool("tool_name").
	Desc("Description").
	Param("param1", "string", true, "default", "Description").
	Returns("object with result").
	RegisterSOI(handler)

// Handler签名
func handler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error)
```

### 3. 使用沙箱

```go
func handler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {
	// 读取沙箱中的文件
	data, err := ctx.Host.SandboxRead("file.txt")
	if err != nil {
		return nil, err
	}
	
	// 处理数据
	result := process(data)
	
	// 写入沙箱中的文件
	err = ctx.Host.SandboxWrite("output.txt", []byte(result))
	if err != nil {
		return nil, err
	}
	
	return map[string]interface{}{
		"success": true,
		"output":  "output.txt",
	}, nil
}
```

## 常用工具类

### 参数解析

```go
// 方式1：使用结构体
var p struct{ Name string }
json.Unmarshal(args, &p)

// 方式2：使用SDK辅助函数
m, _ := sdk.ParseArgsMap(args)
name := sdk.GetString(m, "name", "World")
```

### 错误处理

```go
// 返回错误
return nil, fmt.Errorf("something went wrong: %w", err)

// 返回成功
return map[string]interface{}{
	"result": value,
}, nil
```

## 测试你的插件

```go
// main_test.go
func TestHello(t *testing.T) {
	host := vos.NewMockHost(nil)
	argsJSON, _ := json.Marshal(map[string]interface{}{
		"Name": "Alice",
	})
	
	resp := sdk.CallTool("hello", argsJSON, "/tmp", host)
	
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	
	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)
	
	if msg, _ := out["message"].(string); msg != "Hello, Alice!" {
		t.Errorf("message = %q, want %q", msg, "Hello, Alice!")
	}
}
```

## 调试技巧

### 1. 使用 stdio 模式测试

```bash
# 发送JSON请求到stdin
echo '{"tool":"hello","args":{"Name":"Test"}}' | tinygo run main.go
```

### 2. 检查生成的 WASM 符号

```bash
wasm-objdump -t wasm/plugin.soi | grep -E "(execute|registerTools)"
```

### 3. 使用 go test 调试

```bash
go test -v -run TestHello
```

## 常见问题

### Q: 编译报错 "undefined: execute"

**A**: 确保 `registerTools` 函数被正确导出：
```go
//export registerTools
func registerTools() {
	// ...
}
```

### Q: 编译报错 "main redeclared"

**A**: 确保只有一个 `main()` 函数，且调用了 `sdk.RunTinyGo()`。

### Q: 工具调用失败

**A**: 检查工具名称是否正确：
```go
// 注册时
sdk.NewTool("hello").RegisterSimple(handler)

// 调用时
sdk.CallTool("hello", args, "", nil)  // 名称必须匹配
```

## 快速命令参考

### 脚手架创建

```bash
# Go 插件（默认, wasm）
soi-create scaffold --name my-plugin --type wasm --compiler go

# Go 插件（soi 类型, 支持沙箱）
soi-create scaffold --name my-plugin --type soi --compiler go

# TinyGo 插件（更小体积）
soi-create scaffold --name my-plugin --type wasm --compiler tinygo

# Rust 插件（生成 Cargo.toml / src/lib.rs）
soi-create scaffold --name my-plugin --type wasm --compiler rust
```

### 构建

```bash
# Go（标准 Go wasip1）
$env:GOOS="wasip1"; $env:GOARCH="wasm"; $env:CGO_ENABLED="0"
go build -o wasm/plugin.wasm .

# TinyGo（wasi 导出 ABI）
tinygo build -target=wasi -o wasm/plugin.soi .

# Rust（wasm32-wasip1）
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1
```

### 测试

```bash
# Go / TinyGo
go test -v

# Rust
cargo test
```

### 打包（通过 soi-package）

```bash
# Go
soi-package --dir . --compiler go

# TinyGo
soi-package --dir . --compiler tinygo

# Rust
soi-package --dir . --compiler rust

# 可选优化（需要 binaryen/wasm-opt）
soi-package --dir . --compiler go --optimize
```

### 编译器对照表

| `--compiler` | 源码文件 | 构建产物 | 目标 | 说明 |
|---|---|---|---|---|
| `go`（默认） | `main.go`, `bridge.go`, `tools.go` | `wasm/plugin.wasm` | `wasip1` | 标准 Go 工具链，兼容性最好 |
| `tinygo` | `main.go`, `bridge.go`, `tools.go` | `wasm/plugin.soi` | `wasi` | 输出体积更小，需要安装 TinyGo |
| `rust` | `src/lib.rs`, `Cargo.toml` | `wasm/plugin.wasm` | `wasm32-wasip1` | Rust 语言，通过 `soi-sdk-rs` 提供 ABI |

---

# 设计流程

> SOI（Source of Intelligence）是基于 Go + WASM 的插件开发框架。本文档阐述其整体架构设计与核心决策。

---

## 一、设计目标

SOI SDK 旨在为开发者提供一套 **轻量、可验证、跨平台** 的 WASM 插件开发工具链，核心目标包括：

| 目标 | 说明 |
|------|------|
| **双 ABI 兼容** | 同时支持标准 Stdio ABI（`go build`）和 Export ABI（`tinygo build`），让开发者自由选择运行时 |
| **零外部依赖** | SDK 核心纯 Go 实现，不依赖 wazero 等三方 WASM 运行时 |
| **内建测试** | 通过 `sdk.CallTool` 直接调用工具函数，在普通 `go test` 中完成验证 |
| **脚手架自动化** | 通过 `soi-create scaffold` 一键生成完整项目结构，降低入门门槛 |
| **宿主模拟** | 在测试阶段通过 `soi-vos` 的 `MockHost` 模拟文件系统等宿主环境 |

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                    SOI Platform（运行时）                      │
│  加载 WASM 模块 → 通过 ABI 调用 → 获取 Manifest → 执行 Tool   │
└───────────────┬─────────────────────────────┬────────────────┘
                │   Stdio ABI                  │   Export ABI
                │   (go build WASI)            │   (TinyGo)
                ▼                               ▼
┌───────────────────────────────┐ ┌──────────────────────────────┐
│        run_stdio.go           │ │        host_tinygo.go         │
│  stdin 读请求 → stdout 写响应  │ │  execute(ptr, len) 函数  │
│  Run() → scanner → Execute    │ │  导出 → 直接内存交互         │
└───────────────┬───────────────┘ └──────────────┬───────────────┘
                │                                 │
                └────────────┬────────────────────┘
                             ▼
                  ┌─────────────────────┐
                  │     core.go         │
                  │  ToolDef / ParamDef  │
                  │  Manifest / Registry │
                  │  RegisterTool()      │
                  └─────────────────────┘
                             │
                             ▼
                  ┌─────────────────────┐
                  │   测试与验证          │
                  │  sdk.ExecuteTool()  │
                  │  + vos.MockHost()   │
                  └─────────────────────┘
```

---

## 三、核心设计决策

### 3.1 双 ABI 模式设计

插件通过两种 ABI（Application Binary Interface）与宿主交互：

**Stdio ABI（标准 Go 编译）**
- 编译方式：`GOOS=wasip1 GOARCH=wasm go build`
- 通信方式：**stdin/stdout 管道**流式输入输出
- 入口：`main()` → `sdk.Run()` → 从 `os.Stdin` 读取 JSON 请求 → 输出结果到 `os.Stdout`
- 优势：标准 Go 工具链，无需额外运行时

```
宿主                         插件进程
  │                           │
  │  ── JSON Request ──→  stdin  ──→ scanner.Scan()
  │                           │          │
  │                           │     ExecuteRequest()
  │                           │          │
  │  ←── JSON Response ── stdout  ←──  handler(args)
```

**Export ABI（TinyGo 编译）**
- 编译方式：`tinygo build -target=wasi`
- 通信方式：**导出函数 + 内存共享**，通过指针/长度传递数据
- 入口：`//export execute(ptr uint32, length uint32) uint64`
- 交互协议：`(ptr << 32) | length` 编码返回结果的内存地址和长度
- 优势：更小的 WASM 体积，适合嵌入式/高性能场景

```
宿主                         插件（共享内存）
  │                           │
  │  ── 写入 ptr,len ──→  VirtualMemory[ptr:]
  │                           │
  │  ── 调用 ──→     execute(ptr, length)
  │                           │
  │                   读取输入 → 执行 handler
  │                   写回结果 → VirtualMemory[resultPtr:]
  │                           │
  │  ←── packed ptr,len ──  返回 (resultPtr<<32 | resultLen)
```

### 3.2 进程内测试验证

传统的 WASM 验证需要加载 `.wasm` 文件并通过 wazero 等运行时执行，存在以下问题：
- 引入重量级依赖（wazero 约 15MB）
- 启动慢、调试困难
- 跨平台兼容性问题

SOI SDK 采用 **同进程中直接调用** 方式，在测试阶段直接调用 `sdk.ExecuteTool()` 函数，无需任何 WASM 运行时：

```go
func TestAdd(t *testing.T) {
    argsJSON, _ := json.Marshal(map[string]interface{}{"A": 3, "B": 5})
    resp := sdk.CallTool("add", argsJSON, "", nil)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    var out map[string]interface{}
    json.Unmarshal(resp.Output, &out)
    if out["result"] != 8.0 {
        t.Errorf("expected 8, got %v", out["result"])
    }
}
```

这使得测试可以在 **普通 `go test`** 中运行，无需安装 TinyGo 或任何 WASM 运行时。对于需要宿主环境的 SOI 工具，使用 `soi-vos` 提供的 `MockHost` 进行模拟。

### 3.3 工具注册模式

采用 Go 的 `init()` 函数实现声明式注册：

```go
func init() {
    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "add",
        Description: "Add two numbers",
        Parameters:  []sdk.ParamDef{
            {Name: "A", Type: "number", Required: true},
            {Name: "B", Type: "number", Required: true},
        },
        Returns: `{"result": number}`,
    }, handler)
}
```

这种设计的好处：
- 声明式：工具定义与实现紧密关联，一目了然
- 类型安全：`ToolDef` + `ParamDef` 提供结构化元数据
- 自动发现：`init()` 自动注册，无需手动组装 registry
- 可序列化：`GetManifest()` 导出为 JSON，宿主可直接读取

### 3.4 宿主环境模拟

SOI 工具（`RegisterSOITool`）通过 `HostAPI` 接口与宿主交互。测试时使用 `soi-vos` 提供的 `MockHost` 模拟宿主环境：

```go
host := vos.NewMockHost(nil)
host.SetFile("data.json", []byte(`{"key": "value"}`))

argsJSON, _ := json.Marshal(map[string]interface{}{"source": "data.json"})
resp := sdk.CallTool("process", argsJSON, "", host)
```

### 3.5 Manifest 自描述机制

每个插件通过 `GetManifest()` 返回自身的元数据：

```json
{
  "sdk_version": "1.0.0",
  "abi_version": "1.0",
  "tools": [
    {
      "name": "add",
      "description": "Add two numbers (A + B)",
      "parameters": [
        {"name": "A", "type": "number", "required": true},
        {"name": "B", "type": "number", "required": true}
      ],
      "returns": "{\"result\": number}"
    }
  ],
  "build_tag": "go"
}
```

宿主加载 WASM 后：
1. 调用 `execute()`（Export ABI）或执行 `Run()` 的第一个请求（Stdio ABI）
2. 调用 manifest 导出获取完整的工具列表和参数定义
3. 宿主根据 manifest 自动生成工具注册、参数校验、UI 交互

---

## 四、数据流设计

### 4.1 请求-响应流程

```
用户输入
  │
  ▼
┌─────────────────────────────────────────────┐
│ Request: {"tool":"add","args":{"A":3,"B":5}} │
└─────────────────┬───────────────────────────┘
                  │
                  ▼
         ┌───────────────┐
         │  ExecuteRequest │
         │  解析 tool 名称  │
         │  提取 args      │
         │  提取 sandbox_root（可选）│
         └───────┬─────────┘
                 │
                 ▼
         ┌───────────────┐
         │  ExecuteTool   │
         │  查找 handler   │
         │  调用 handler   │
         └───────┬─────────┘
                 │
                 ▼
┌─────────────────────────────────────────┐
│ Response: {"result":8}                   │
│  或 Error: {"error":"division by zero"}  │
└─────────────────────────────────────────┘
```

### 4.2 测试数据流

```
sdk.CallTool("add", argsJSON, "", nil)
  │
  ├── json.Marshal(args) → argsJSON
  ├── 查找 toolRegistry["add"]
  ├── 调用 handler(args)
  ├── json.Marshal(result) → output
  │
  └── 返回 ExecuteResponse{Output: output}
```

---

## 五、关键设计原则

| 原则 | 实践 |
|------|------|
| **可测试性优先** | 每个插件可通过 `sdk.ExecuteTool` 直接测试，无需外部 WASM 运行时 |
| **渐进增强** | 从默认 `ping` 工具到预设模板 `calc`/`hello`，再到自定义工具 |
| **关注点分离** | SDK 核心（注册+执行）→ Scaffold（生成）→ Package（打包）→ Verify（验证） |
| **约定优于配置** | 标准的目录结构、命名规范、replace 路径自动推断 |
| **编译时安全** | `init()` 自动注册，`GetManifest()` 编译时保证 manifest 完整性 |

---

## 六、演进路线

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 核心 SDK（注册、执行、双 ABI） | ✅ 完成 |
| Phase 2 | 进程内测试验证（sdk.ExecuteTool + MockHost） | ✅ 完成 |
| Phase 3 | CLI 验证工具（soi-verify） | ✅ 完成 |
| Phase 4 | 项目脚手架（soi-create scaffold） | ✅ 完成 |
| Phase 5 | 插件打包分发（soi-package） | ✅ 完成 |
| Phase 6 | 代码包装工具（soi-create wrap） | ✅ 完成 |

---

# 代码结构

> 本文档完整描述 SOI SDK 项目的目录布局、文件职责及包间依赖关系。

---

## 一、项目树

```
soi-sdk/
├── go.mod                          # 根模块定义 (github.com/Source-of-Intelligence/soi-sdk)
├── LICENSE                         # 开源协议
├── README.md                       # 项目说明
│
├── core.go                         # ★ 核心类型与全局注册表
├── run_stdio.go                    # ★ Stdio ABI 入口与执行引擎
├── host_tinygo.go                  # ★ Export ABI 入口 (tinygo build tag)
├── run_tinygo.go                   # TinyGo 运行时辅助函数 (tinygo build tag)
├── manifest.go                     # Manifest 序列化与 skill.yaml 生成
│
├── cmd/                            # 命令行工具集
│   ├── soi-package/                # 插件打包工具
│   │   └── main.go                 #   编译 WASM → 验证符号 → 打包 ZIP
│   ├── soi-create/                 # 项目创建工具（scaffold/wrap）
│   │   └── main.go                 #   一键生成完整插件项目 / 将 Go 函数包装为 SOI 工具
│   ├── soi-verify/                 # 插件验证 CLI
│   │   └── main.go                 #   内置工具执行、测试生成
│
├── examples/                       # 示例插件
│   ├── lotto/                      #   抽奖插件
│   ├── text_tools/                 #   文本工具插件
│   ├── xlsx2md/                    #   Excel 转 Markdown 插件
│   └── wrap-demo/                  #   代码包装演示
│
└── docs/                           # 文档
    ├── 01-设计流程.md
    ├── 02-代码结构.md               # ← 本文档
    ├── 03-创建过程.md
    └── 04-使用逻辑.md
```

---

## 二、核心文件详解

### 2.1 `core.go` — 类型定义与全局注册表

**职责**：定义 SDK 所有核心数据结构，管理全局工具注册表。

```go
// 核心类型
type ToolHandler func(args map[string]interface{}) (interface{}, error)
type SOIToolHandler func(args map[string]interface{}, host HostAPI) (interface{}, error)
type ToolDefinition struct { Name, Description string; Schema ToolSchema; Handler ToolHandler }
type ExecuteResponse struct { Output []byte; Error string }
type HostAPI interface { GetFile; SetFile; DeleteFile; ListFiles }

// 全局注册表（包级私有）
var toolRegistry = make(map[string]registeredTool)

// 公开 API
func GetManifest() Manifest       // 返回所有已注册工具的清单
func GetTools() map[string]ToolHandler  // 返回 name → handler 映射
func GetSOITools() map[string]SOIToolHandler  // 返回 SOI 工具映射
func GetToolDefs() []ToolDefinition      // 返回所有工具定义
func ExecuteTool(...) ExecuteResponse     // 核心执行函数
```

**设计要点**：
- `toolRegistry` 是包级私有变量，通过 `init()` 函数中的 `RegisterTool` 写入
- `Manifest` 是插件的"身份证"，宿主通过它了解插件提供了哪些能力
- `SDKVersion` 和 `ABIVersion` 用于版本兼容性检查
- `ExecuteTool` 是公共函数，可直接在测试中调用

---

### 2.2 `run_stdio.go` — Stdio ABI 入口

**职责**：标准 Go WASI 编译模式下的插件入口，基于 stdin/stdout 管道通信。

```go
// 注册 API
func RegisterTool(name string, schema ToolSchema, handler ToolHandler)           // 简单注册
func RegisterToolWithDef(name string, def ToolDefinition)                         // 完整注册

// 运行时入口
func Run()                               // 主循环：从 stdin 读取一条请求 → 执行 → 写入 stdout
func ExecuteRequest(req ExecuteRequest, buildTag string, host HostAPI) ExecuteResponse  // 解析请求并执行
func ExecuteTool(...) ExecuteResponse     // 核心执行逻辑
```

**执行流程**：
1. `main() → sdk.Run()`
2. `bufio.Scanner` 从 `os.Stdin` 读取一行 JSON
3. 调用 `ExecuteRequest()` 解析 `{"tool":"...", "args":...}`
4. 查 `toolRegistry` 找到 handler，调用 handler(args)
5. 结果 JSON 序列化后写入 `os.Stdout`

---

### 2.3 `host_tinygo.go` — Export ABI 入口

**职责**：TinyGo 编译模式下的插件入口，通过导出函数 + 共享内存通信。

```go
//go:build tinygo

import "unsafe"

// 导出函数
//export execute
func execute(ptr uint32, length uint32) uint64
```

**关键实现**：
- `buildTag = "tinygo"`（通过 `init()` 设置，区分编译目标）
- `execute(ptr, length)`：从内存读取输入，执行后通过 `packResultSDK` 返回结果指针

---

### 2.4 `manifest.go` — Manifest 辅助

**职责**：Manifest 的 JSON 序列化和 skill.yaml 生成。

```go
func BuildManifestJSON(tools []ToolDefinition) []byte    // 将 Manifest 序列化为 JSON
func GenerateSkillYAML(cfg SkillConfig) string          // 生成 YAML 描述文件
```

---

## 三、命令行工具

### 3.1 `cmd/soi-package/` — 插件打包

将编译好的 WASM 插件打包为可分发的 `.zip` 文件：
- 自动检测插件类型（从 skill.yaml）
- 编译 WASM/SOI
- 验证 SOI 符号
- 可选 wasm-opt 优化
- 复制元数据文件
- 生成 ZIP

### 3.2 `cmd/soi-create/` — 项目创建工具（scaffold/wrap）

一键生成完整的 SOI 插件项目结构，或将已有的 Go 函数/库包装为 SOI 插件：
- 支持 `hello` / `calc` 预设模板
- 自定义工具名称列表
- 自动生成 `main.go`、`main_test.go`、`go.mod`、`skill.yaml`、`build.ps1`
- 自动分析函数签名
- 生成参数 schema
- 生成完整的 SOI 插件代码

### 3.3 `cmd/soi-verify/` — 插件验证

验证插件工具的正确性：
- 内置工具执行测试
- 生成测试模板（`--gen`）
- 运行测试（`--test`）

---

## 四、包间依赖关系

```
                    ┌──────────────────────┐
                    │    github.com/Source-of-Intelligence/soi-sdk    │  ← 根包（core + sdk + manifest）
                    │   (core.go, run_stdio.go,  │
                    │    host_tinygo.go,   │
                    │    manifest.go)       │
                    └──────┬───────┬───────┘
                           │       │
              ┌────────────┘       └────────────┐
              ▼                                  ▼
┌─────────────────────────┐        ┌─────────────────────────┐
│    github.com/Source-of-Intelligence/soi-vos       │        │   examples/*, cmd/*     │
│  MockHost / HostFunctions │        │   import sdk            │
│  (宿主环境模拟)            │        │  (插件与工具集)          │
└─────────────────────────┘        └─────────────────────────┘
```

**关键规则**：
- `sdk` 包是核心，不依赖 `examples` 或 `cmd`
- `soi-vos` 提供宿主环境模拟（`MockHost`），用于 SOI 工具的测试
- `examples/*` 和 `cmd/*` 依赖 `sdk` 包
- WASM 执行由独立的 `wasm-executor` 项目负责，不在 SDK 中

---

## 五、编译构建条件

| 文件 | 编译条件 | 说明 |
|------|----------|------|
| `run_stdio.go` | 所有平台 | 标准 Go 编译 |
| `core.go` | 所有平台 | 核心类型定义 |
| `host_tinygo.go` | `//go:build tinygo` | 仅 TinyGo 编译时包含 |

---

## 六、数据结构的生命周期

```
编译时
  │
  ├── init() 调用 → RegisterToolWithDef() → 写入 toolRegistry
  │
  ▼
运行时（插件加载）
  │
  ├── GetManifest() → 宿主获取插件元数据
  │
  ▼
运行时（工具调用）
  │
  ├── JSON Request 到达
  ├── ExecuteRequest() → ExecuteTool()
  ├── 查找 handler → handler(args)
  ├── json.Marshal(result) → JSON Response
  │
  ▼
测试时
  │
  ├── sdk.GetTools() → 获取 handler map
  ├── sdk.ExecuteTool(tools, soiTools, name, args, "", host) → 直接调用
  └── 断言结果
```

---

## 七、外部依赖

```go
// go.mod
module github.com/Source-of-Intelligence/soi-sdk
go 1.22.0

require github.com/Source-of-Intelligence/soi-vos v0.0.0
```

SDK 核心仅依赖 `soi-vos`（宿主环境模拟），其余全部使用 Go 标准库：
- `encoding/json` — 序列化/反序列化
- `bufio` / `os` — Stdio ABI 的 stdin/stdout
- `unsafe` — TinyGo 内存操作
- `testing` — 测试框架
- `text/template` — 代码生成

---

# 创建过程

> 本文档从零开始，完整展示如何创建一个 SOI WASM 插件项目，包括脚手架一键生成和手动创建两种方式。

---

## 一、使用脚手架一键生成（推荐）

### 1.1 脚手架工具概述

`soi-create scaffold` 是内置在 SDK 中的命令行工具，支持：
- 从预设模板生成（`hello` / `calc`）
- 自定义工具名称列表
- 自动生成完整的项目结构、测试、构建脚本

### 1.2 基础用法

```powershell
# 在 SDK 根目录执行
cd e:\code\go\soi-sdk

# 默认：生成含 ping 工具的项目
go run ./cmd/soi-create scaffold --name my-plugin

# 使用预设模板
go run ./cmd/soi-create scaffold --name my-math --tools calc
go run ./cmd/soi-create scaffold --name my-hello --tools hello

# 自定义工具名列表
go run ./cmd/soi-create scaffold --name my-api --tools fetch,validate,transform

# 指定输出目录
go run ./cmd/soi-create scaffold --name my-plugin --dir ./projects/my-plugin
```

### 1.3 生成后的项目结构

执行 `go run ./cmd/soi-create scaffold --name my-plugin --tools calc` 后，生成如下文件：

```
my-plugin/
├── main.go              # 插件入口：工具注册 + sdk.Run()
├── main_test.go         # 双 ABI 模式测试
├── go.mod               # Go 模块（含 replace 指向本地 SDK）
├── skill.yaml           # 插件清单（名称、版本、工具列表）
├── README.md            # 项目文档（含构建和测试说明）
├── .gitignore           # 忽略 wasm/ 构建产物
├── scripts/
│   └── build.ps1        # 构建脚本（支持 -Test 先测试再构建）
└── wasm/                # WASM 构建输出目录（初始为空）
```

### 1.4 生成文件内容说明

#### `main.go` — 插件入口

```go
// my-plugin - SOI WASM plugin
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm .
package main

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/Source-of-Intelligence/soi-sdk"
)

func init() {
    // 每个工具通过 init() 自动注册
    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "add",
        Description: "Add two numbers (A + B)",
        Parameters: []sdk.ParamDef{
            {Name: "A", Type: "number", Required: true, Description: "First operand"},
            {Name: "B", Type: "number", Required: true, Description: "Second operand"},
        },
        Returns: `{"result": number}`,
    }, func(args json.RawMessage) (interface{}, error) {
        var p struct{ A, B float64 }
        json.Unmarshal(args, &p)
        // TODO: implement add logic
        _ = p
        return map[string]interface{}{"message": "add called"}, nil
    })
    // ... 更多工具 ...
}

func main() {
    manifest, _ := sdk.BuildManifestJSON()
    fmt.Fprintln(os.Stderr, manifest)  // 启动时输出 manifest 到 stderr
    sdk.Run()
}
```

#### `go.mod` — 模块依赖

```
module github.com/Source-of-Intelligence/soi/my-plugin

go 1.22.0

require github.com/Source-of-Intelligence/soi-sdk v1.0.0

replace github.com/Source-of-Intelligence/soi-sdk => ../..    # 指向本地 SDK 路径
```

`replace` 指令使得开发阶段无需发布 SDK 到远程仓库。

#### `skill.yaml` — 插件清单

```yaml
apiVersion: v1
kind: Skill
metadata:
  name: my-plugin
  version: "1.0.0"
  description: "my-plugin - SOI WASM plugin"
spec:
  runtime:
    type: wasip1
    entry: wasm/plugin.wasm
  provides:
    tools:
      - name: add
        description: "Add two numbers (A + B)"
        parameters:
          - name: A
            type: number
            required: true
            description: "First operand"
          - name: B
            type: number
            required: true
            description: "Second operand"
        returns: |
          {"result": number}
```

---

## 二、实现工具逻辑

### 2.1 从 TODO 到完整实现

脚手架生成的 `main.go` 包含占位符 `// TODO: implement xxx logic`，只需替换为实际逻辑：

```go
// 脚手架生成（占位符）
return map[string]interface{}{"message": "add called"}, nil

// 替换为实际逻辑：
var p struct{ A, B float64 }
json.Unmarshal(args, &p)
return map[string]float64{"result": p.A + p.B}, nil
```

### 2.2 添加新工具

在 `init()` 函数中追加新的 `RegisterToolWithDef` 调用即可：

```go
func init() {
    // ... 现有工具 ...

    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "sqrt",
        Description: "Calculate the square root of a number",
        Parameters: []sdk.ParamDef{
            {Name: "X", Type: "number", Required: true, Description: "Input number"},
        },
        Returns: `{"result": number}`,
    }, func(args json.RawMessage) (interface{}, error) {
        var p struct{ X float64 }
        json.Unmarshal(args, &p)
        if p.X < 0 {
            return nil, fmt.Errorf("cannot calculate square root of negative number")
        }
        return map[string]float64{"result": math.Sqrt(p.X)}, nil
    })
}
```

**注意事项**：
- `Name` 必须全局唯一
- `Type` 当前支持：`string`、`number`、`boolean`、`any`
- `Required: true` 的参数调用时必须提供
- 返回 `(nil, error)` 表示工具执行失败，错误信息会返回给调用方

---

## 三、运行测试

### 3.1 脚手架自带测试

生成的 `main_test.go` 已包含完整的测试：

```go
func TestManifest(t *testing.T) {
    manifest := sdk.GetManifest()
    // 验证 SDK 版本、ABI 版本、工具数量
}

func TestTools(t *testing.T) {
    tools := sdk.GetTools()
    for name := range tools {
        t.Run(name, func(t *testing.T) {
            argsJSON, _ := json.Marshal(map[string]interface{}{})
            resp := sdk.CallTool(name, argsJSON, nil)
            if resp.Error != "" {
                t.Fatalf("tool %s error: %s", name, resp.Error)
            }
            t.Logf("%s: %s", name, string(resp.Output))
        })
    }
}
```

### 3.2 执行测试

```powershell
cd my-plugin

# 基础测试（快速验证）
go test -v ./...

# 带详细输出的完整测试
go test -v -count=1 ./...
```

**预期输出**：
```
=== RUN   TestManifest
    main_test.go:21: Manifest: SDK=1.0.0, ABI=1.0, Tools=4
--- PASS: TestManifest (0.00s)
=== RUN   TestTools
=== RUN   TestTools/stdio
=== RUN   TestTools/stdio/add
--- PASS: TestTools/stdio/add (0.00s)
=== RUN   TestTools/export
=== RUN   TestTools/export/add
--- PASS: TestTools/export/add (0.00s)
PASS
ok      github.com/Source-of-Intelligence/soi/my-plugin     1.123s
```

### 3.3 编写详细测试

随着工具逻辑完善，应扩展测试覆盖边界情况：

```go
func TestAdd(t *testing.T) {
    // 正常情况
    argsJSON, _ := json.Marshal(map[string]interface{}{"A": 3.0, "B": 5.0})
    resp := sdk.CallTool("add", argsJSON, nil)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    var out map[string]interface{}
    json.Unmarshal(resp.Output, &out)
    if out["result"] != 8.0 {
        t.Errorf("expected 8, got %v", out["result"])
    }

    // 错误情况：未知工具
    argsJSON2, _ := json.Marshal(map[string]interface{}{})
    resp2 := sdk.CallTool("nonexistent", argsJSON2, nil)
    if resp2.Error == "" {
        t.Error("expected error for nonexistent tool")
    }
}
```

---

## 四、构建 WASM

### 4.1 使用内置构建脚本

```powershell
cd my-plugin

# 先测试再构建
.\scripts\build.ps1 -Test

# 仅构建（跳过测试）
.\scripts\build.ps1

# 清理构建产物
.\scripts\build.ps1 -Clean
```

### 4.2 手动构建

```powershell
# 方式一：PowerShell 环境变量
$env:GOOS = "wasip1"
$env:GOARCH = "wasm"
$env:CGO_ENABLED = "0"
go build -o wasm/plugin.wasm .

# 方式二：单行命令
$env:GOOS="wasip1"; $env:GOARCH="wasm"; $env:CGO_ENABLED="0"; go build -o wasm/plugin.wasm .
```

### 4.3 TinyGo 构建（可选）

如果安装了 TinyGo：

```powershell
tinygo build -target=wasi -o wasm/plugin.soi .
```

---

## 五、手动创建项目（不使用脚手架）

如果不想使用脚手架，可以手动搭建项目：

### 5.1 创建目录结构

```powershell
mkdir my-plugin
mkdir my-plugin\wasm
mkdir my-plugin\scripts
```

### 5.2 创建 `go.mod`

```
module github.com/Source-of-Intelligence/soi/my-plugin

go 1.22.0

require github.com/Source-of-Intelligence/soi-sdk v1.0.0

replace github.com/Source-of-Intelligence/soi-sdk => ../path/to/soi-sdk
```

### 5.3 创建 `main.go`

最少可运行的插件模板：

```go
package main

import (
    "encoding/json"
    "github.com/Source-of-Intelligence/soi-sdk"
)

func init() {
    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "ping",
        Description: "Simple ping-pong",
        Parameters:  []sdk.ParamDef{},
        Returns:     `{"pong": true}`,
    }, func(args json.RawMessage) (interface{}, error) {
        return map[string]bool{"pong": true}, nil
    })
}

func main() { sdk.Run() }
```

### 5.4 创建 `main_test.go`

```go
package main

import (
    "encoding/json"
    "testing"
    "github.com/Source-of-Intelligence/soi-sdk"
)

func TestPing(t *testing.T) {
    argsJSON, _ := json.Marshal(map[string]interface{}{})
    resp := sdk.CallTool("ping", argsJSON, nil)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    var out map[string]interface{}
    json.Unmarshal(resp.Output, &out)
    if out["pong"] != true {
        t.Errorf("expected pong=true, got %v", out["pong"])
    }
}
```

### 5.5 验证

```powershell
go test -v ./...
go build -o wasm/plugin.wasm .  # 先设置 GOOS=wasip1 GOARCH=wasm
```

---

## 六、使用 CLI 验证工具

```powershell
# 回到 SDK 根目录
cd e:\code\go\soi-sdk

# 生成测试套件
go run ./cmd/soi-verify --gen ./examples/my-plugin

# 运行测试
go run ./cmd/soi-verify --test ./examples/my-plugin

# 验证内置工具
go run ./cmd/soi-verify --version
go run ./cmd/soi-verify --list
$json = '{\"A\":3,\"B\":5}'; go run ./cmd/soi-verify --tool add --args $json
```

---

## 七、完整创建流程总结

```
                    ┌────────────────────┐
                    │ 1. 生成脚手架       │
                    │  soi-create scaffold│
                    │  --name --tools     │
                    └────────┬───────────┘
                             │
                             ▼
                    ┌────────────────────┐
                    │ 2. 实现工具逻辑     │
                    │  编辑 main.go       │
                    │  替换 TODO 占位符   │
                    └────────┬───────────┘
                             │
                             ▼
                    ┌────────────────────┐
                    │ 3. 运行测试         │
                    │  go test -v ./...   │
                    │  双 ABI 自动验证     │
                    └────────┬───────────┘
                             │
                             ▼
                    ┌────────────────────┐
                    │ 4. 构建 WASM        │
                    │  GOOS=wasip1        │
                    │  go build -o wasm/  │
                    └────────┬───────────┘
                             │
                             ▼
                    ┌────────────────────┐
                    │ 5. 分发             │
                    │  wasm/plugin.wasm   │
                    │  + skill.yaml       │
                    └────────────────────┘
```

---

## 八、预设模板速查

| 模板名 | 包含工具 | 适用场景 |
|--------|----------|----------|
| `ping`（默认） | `ping` | 快速验证、连通性测试 |
| `hello` | `greet` | 带参数的文本处理 |
| `calc` | `add`, `subtract`, `multiply`, `divide` | 数学计算类工具 |
| 自定义 | 用户指定的工具名列表 | 任意场景 |

---

## 九、常见问题

### Q: 生成的 go.mod 中 replace 路径不对？

脚手架使用 `--sdk-root` 参数手动指定 SDK 根目录路径：

```powershell
go run ./cmd/soi-create scaffold --name my-plugin --sdk-root e:\code\go\soi-sdk
```

### Q: 如何添加第三方依赖？

在 `go.mod` 中添加依赖后，确保 WASM 编译时不被 CGO 或平台特定代码阻塞。

```powershell
go get github.com/some/package
```

### Q: WASM 体积太大？

使用 TinyGo 编译可显著减小体积：

```powershell
tinygo build -target=wasi -o wasm/plugin.soi .
```

TinyGo 编译产物通常只有标准 Go 的 1/5 ~ 1/10 大小。

---

# 使用逻辑

> 本文档深入讲解 SOI SDK 的运行机制，包括请求生命周期、ABI 通信协议、测试方法，以及工具开发的最佳实践。

---

## 一、运行时总览

### 1.1 完整调用链路

```
┌────────────────────────────────────────────────────────────────────┐
│                          宿主 (Host)                                │
│                                                                    │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐     │
│  │ 加载 WASM │ ──▶│ 获取      │ ──▶│ 构造      │ ──▶│ 调用工具   │     │
│  │ 模块     │    │ Manifest │    │ 参数      │    │ 函数      │     │
│  └──────────┘    └──────────┘    └──────────┘    └────┬─────┘     │
│                                                       │            │
└───────────────────────────────────────────────────────┼────────────┘
                                                        │
                                          ┌─────────────┴─────────────┐
                                          │       插件 (Plugin)         │
                                          │                            │
                                          │  sdk.Run() / execute() │
                                          │       ↓                    │
                                          │  ExecuteRequest(json)      │
                                          │       ↓                    │
                                          │  查找 handler → 调用       │
                                          │       ↓                    │
                                          │  返回 JSON 结果             │
                                          └────────────────────────────┘
```

### 1.2 启动流程

插件启动时（无论 Stdio 还是 Export ABI），`init()` 函数中注册的所有工具会自动填充到全局 `toolRegistry`：

```go
// 编译时：Go 运行时自动按文件顺序执行所有 init()
// 1. core.go 的 init() — 初始化 toolRegistry（空 map）
// 2. 用户 main.go 的 init() — 注册所有工具
//    sdk.RegisterToolWithDef(ToolDef{...}, handler)
//    → toolRegistry["add"] = {handler, ToolDef{...}}
// 3. host_tinygo.go 的 init() — 设置 buildTag = "tinygo"（仅 TinyGo 编译）
```

---

## 二、Stdio ABI 详解

### 2.1 通信协议

Stdio ABI 利用 WASI 标准的 stdin/stdout 管道进行进程间通信。

**请求格式**（stdin 输入）：
```json
{
  "tool": "add",
  "args": {"A": 3, "B": 5},
  "sandbox_root": "/sandbox/abc123"
}
```

**成功响应**（stdout 输出）：
```json
{"result": 8}
```

**错误响应**（stderr 输出）：
```json
{"error": "division by zero"}
```

### 2.2 `Run()` 函数源码解析

```go
func Run() {
    scanner := bufio.NewScanner(os.Stdin)       // ① 从 stdin 创建行扫描器
    if !scanner.Scan() {                        // ② 读取一行 JSON
        writeErrorSDK("no input")
        return
    }
    result := ExecuteRequest(scanner.Bytes())   // ③ 解析并执行
    if result.Error != "" {                     // ④ 错误处理
        writeErrorSDK(result.Error)             //    写入 stderr
        return
    }
    os.Stdout.Write(result.Output)              // ⑤ 成功：写入 stdout
    os.Stdout.Write([]byte{'\n'})               //    追加换行符
}
```

### 2.3 `ExecuteRequest()` 源码解析

```go
func ExecuteRequest(reqJSON []byte) ExecuteResponse {
    // ① 解析顶层结构
    var req struct {
        Tool        string          `json:"tool"`
        Args        json.RawMessage `json:"args"`
        SandboxRoot string          `json:"sandbox_root,omitempty"`
    }
    if err := json.Unmarshal(reqJSON, &req); err != nil {
        return ExecuteResponse{Error: "parse input: " + err.Error()}
    }
    // ② 委托给 ExecuteTool
    return ExecuteTool(GetTools(), GetSOITools(), req.Tool, req.Args, req.SandboxRoot)
}
```

### 2.4 `ExecuteTool()` 核心执行逻辑

```go
func ExecuteTool(tools map[string]ToolHandler, soiTools map[string]SOIToolHandler,
                 toolName string, argsJSON []byte, sandboxRoot string, host HostAPI) ExecuteResponse {
    // ① 查找工具
    handler, ok := tools[toolName]
    if !ok {
        // ② 查找 SOI 工具
        soiHandler, ok2 := soiTools[toolName]
        if !ok2 {
            return ExecuteResponse{Error: "unknown tool: " + toolName}
        }
        // ③ 执行 SOI 处理函数（带宿主环境）
        var args map[string]interface{}
        json.Unmarshal(argsJSON, &args)
        result, err := soiHandler(args, host)
        // ... 序列化结果
    }
    // ③ 执行处理函数
    result, err := handler(args)
    // ... 序列化结果
}
```

**关键设计**：`ExecuteTool` 是公共函数，同时被 Stdio ABI（`run_stdio.go`）和 Export ABI（`host_tinygo.go`）调用，确保两种模式下工具执行逻辑完全一致。

---

## 三、Export ABI 详解

### 3.1 内存共享协议

Export ABI 通过 `execute` 导出函数实现通信，使用 **指针打包** 协议：

```
输入：
  ptr:   内存地址 → 宿主写入请求 JSON 的位置
  length: 字节长度 → JSON 数据的字节数

输出（返回值 uint64）：
  高 32 位: resultPtr  → 结果数据在内存中的地址
  低 32 位: resultLen  → 结果数据的字节长度
```

### 3.2 `execute()` 源码解析

```go
//export execute
func execute(ptr uint32, length uint32) uint64 {
    // ① 从共享内存读取输入
    input := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

    // ② 执行请求（与 Stdio 完全相同的逻辑）
    resp := ExecuteRequest(input)

    // ③ 准备响应数据
    var resultBuf []byte
    if resp.Error != "" {
        resultBuf, _ = json.Marshal(map[string]string{"error": resp.Error})
    } else {
        resultBuf = resp.Output       // 包级变量，避免短生命周期
    }

    // ④ 打包返回指针 (ptr<<32 | length)
    return packResultSDK(resultBuf)
}

func packResultSDK(data []byte) uint64 {
    if len(data) == 0 {
        return 0
    }
    ptr := uint64(uintptr(unsafe.Pointer(&data[0])))
    length := uint64(len(data))
    return (ptr << 32) | length          // 高32位=地址, 低32位=长度
}
```

### 3.3 与 Stdio ABI 的对比

| 维度 | Stdio ABI | Export ABI |
|------|-----------|------------|
| 编译工具 | `go build` (标准 Go) | `tinygo build` |
| 通信方式 | stdin/stdout 管道 | 共享内存 + 导出函数 |
| 输入 | `os.Stdin` 读取 JSON | 内存地址 `ptr` + 长度 `length` |
| 输出 | `os.Stdout` 写 JSON | 返回打包指针 `(ptr<<32)|len` |
| 入口 | `main()` → `Run()` | `execute()` 导出函数 |
| 多请求 | 每行一个请求（循环） | 每次调用一个请求 |
| 二进制大小 | ~3.2 MB | ~0.5 MB（典型） |
| 构建标签 | 无 | `//go:build tinygo` |

---

## 四、测试方法

### 4.1 进程内测试

SOI SDK 的测试方式非常简单：直接调用 `sdk.CallTool()` 函数，无需任何 WASM 运行时。

```go
func TestAdd(t *testing.T) {
    argsJSON, _ := json.Marshal(map[string]interface{}{"A": 3, "B": 5})
    resp := sdk.CallTool("add", argsJSON, nil)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    var out map[string]interface{}
    json.Unmarshal(resp.Output, &out)
    if out["result"] != 8.0 {
        t.Errorf("expected 8, got %v", out["result"])
    }
}
```

### 4.2 SOI 工具测试（带宿主环境）

对于需要文件 I/O 等宿主功能的 SOI 工具，使用 `soi-vos` 提供的 `MockHost`：

```go
func TestProcessFile(t *testing.T) {
    host := vos.NewMockHost(nil)
    host.SetFile("data.json", []byte(`{"key": "value"}`))

    argsJSON, _ := json.Marshal(map[string]interface{}{"source": "data.json"})
    resp := sdk.CallTool("process", argsJSON, host)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    // 验证输出文件
    output, _ := host.GetFile("result.json")
    t.Logf("output: %s", string(output))
}
```

### 4.3 真实 WASM 测试

对于需要验证编译后 WASM 行为的场景，使用独立的 `wasm-executor` 项目：

```go
// 需要在 go.mod 中添加 wasm-executor 依赖
import soipkg "wasm-executor/pkg/soi"

func TestWASMExecution(t *testing.T) {
    wasmBytes, _ := os.ReadFile("wasm/plugin.wasm")
    plugin, err := soipkg.NewSOIPlugin(ctx, wasmBytes, host)
    // ... 执行真实 WASM
}
```

---

## 五、工具处理函数规范

### 5.1 函数签名

```go
// 普通工具
type ToolHandler func(args map[string]interface{}) (interface{}, error)

// SOI 工具（带宿主环境）
type SOIToolHandler func(args map[string]interface{}, host HostAPI) (interface{}, error)
```

| 参数/返回 | 类型 | 说明 |
|-----------|------|------|
| `args` | `map[string]interface{}` | 工具参数 |
| `host` | `HostAPI` | 宿主环境接口（仅 SOI 工具） |
| 返回值 1 | `interface{}` | 成功结果，会被 `json.Marshal` 序列化 |
| 返回值 2 | `error` | 错误，== nil 表示成功 |

### 5.2 参数解析最佳实践

```go
// 方式一：结构体解析（推荐）
var p struct{ A, B float64 }
json.Unmarshal(argsJSON, &p)

// 方式二：map 解析
var params map[string]interface{}
json.Unmarshal(argsJSON, &params)

// 方式三：带默认值
type Params struct {
    Name string `json:"name"`  // 空 string 就是默认值
    Age  int    `json:"age"`   // 0 就是默认值
}
var p Params
json.Unmarshal(argsJSON, &p)
```

### 5.3 错误处理模式

```go
// 参数校验错误 → 返回 error
if p.B == 0 {
    return nil, fmt.Errorf("division by zero")
}

// 参数缺失 → 返回 error
if p.Name == "" {
    return nil, fmt.Errorf("name is required")
}

// 不支持的输入 → 返回 error
if p.X < 0 {
    return nil, fmt.Errorf("input must be non-negative, got %f", p.X)
}
```

### 5.4 结果返回模式

```go
// 简单值
return "hello", nil                           // → "hello"

// 结构体
return struct{ Result float64 }{Result: 42}, nil  // → {"Result":42}

// Map（最常用）
return map[string]interface{}{
    "result": p.A + p.B,
    "operation": "addition",
}, nil  // → {"result":8,"operation":"addition"}
```

---

## 六、Manifest 与元数据

### 6.1 获取 Manifest

```go
// 在插件代码中
manifest := sdk.GetManifest()

// 输出：
// Manifest{
//     SDKVersion: "1.0.0",
//     ABIVersion: "1.0",
//     BuildTag:   "go",        // 或 "tinygo"
//     Tools: []ToolManifest{...},
// }
```

### 6.2 序列化为 JSON

```go
tools := sdk.GetToolDefs()
data := sdk.BuildManifestJSON(tools)
// {
//   "sdk_version": "1.0.0",
//   "abi_version": "1.0",
//   "tools": [...]
// }
```

### 6.3 生成 skill.yaml

```go
yaml := sdk.GenerateSkillYAML(sdk.SkillConfig{...})
// 返回完整的 YAML 描述文件内容
```

---

## 七、完整工具开发示例

以下是一个完整的文件操作工具实现：

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/Source-of-Intelligence/soi-sdk"
)

func init() {
    // 工具 1：文本大写转换
    sdk.RegisterTool("to_upper", sdk.ToolSchema{
        Description: "Convert text to uppercase",
        Parameters: map[string]interface{}{
            "Text": map[string]interface{}{
                "type": "string", "required": true,
                "description": "Input text",
            },
        },
    }, func(args map[string]interface{}) (interface{}, error) {
        text, _ := args["Text"].(string)
        if text == "" {
            return nil, fmt.Errorf("text is empty")
        }
        // 实际实现中应使用 strings.ToUpper
        return map[string]string{"result": text}, nil
    })
}

func main() {
    sdk.Run()
}
```

对应的测试：

```go
func TestToUpper(t *testing.T) {
    argsJSON, _ := json.Marshal(map[string]interface{}{"Text": "hello"})
    resp := sdk.CallTool("to_upper", argsJSON, nil)
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    var out map[string]interface{}
    json.Unmarshal(resp.Output, &out)
    if out["result"] != "HELLO" {
        t.Errorf("expected HELLO, got %v", out["result"])
    }
}
```

---

## 八、关键概念速查表

| 概念 | 定义 | 所在位置 |
|------|------|----------|
| `ToolHandler` | 工具处理函数类型 | `core.go` |
| `SOIToolHandler` | SOI 工具处理函数类型（带宿主环境） | `core.go` |
| `HostAPI` | 宿主环境接口 | `core.go` |
| `Manifest` | 插件清单（SDK版本、ABI版本、工具列表） | `core.go` |
| `RegisterTool` | 注册工具 | `run_stdio.go` |
| `RegisterSOITool` | 注册 SOI 工具 | `run_stdio.go` |
| `Run()` | Stdio ABI 入口 | `run_stdio.go` |
| `ExecuteRequest` | 解析请求 JSON 并执行 | `run_stdio.go` |
| `ExecuteTool` | 核心执行逻辑（可直接在测试中调用） | `core.go` |
| `execute` | Export ABI 入口 | `host_tinygo.go` |
| `MockHost` | 宿主环境模拟器 | `soi-vos` |
| `soi-create scaffold` | 脚手架生成工具 | `cmd/soi-create/` |
| `soi-create wrap` | 代码包装工具 | `cmd/soi-create/` |
| `soi-verify` | CLI 验证工具 | `cmd/soi-verify/` |
| `soi-package` | 插件打包工具 | `cmd/soi-package/` |

---

# SDK新使用方式说明

## 概述

SOI SDK v2 已经重构，现在用户**无需编写 `main_tinygo.go`**，SDK会自动处理所有TinyGo WASM交互逻辑。

## 插件开发简化

### 旧方式（已废弃）

```go
// main.go
func init() {
    registerTools()
}

func main() { sdk.Run() }

// main_tinygo.go (必须创建，60+行重复代码)
func init() {
    sdk.SetBuildTag("tinygo")
    registerTools()
}

//export execute
func execute(ptr uint32, length uint32) uint64 {
    // 60+行的JSON解析代码...
}

// export registerTools
func registerTools() {
    sdk.NewTool("my_tool").
        // ...
        RegisterSOI(handler)
}
```

### 新方式（推荐）

```go
// main.go - 只需这一个文件！
package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func init() {
    registerTools()
}

//export registerTools
func registerTools() {
    sdk.NewTool("my_tool").
        Desc("我的工具描述").
        Param("input", "string", true, "", "输入参数").
        Returns("object with result").
        RegisterSOI(myHandler)
}

func myHandler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {
    // 业务逻辑
    return map[string]interface{}{"result": "success"}, nil
}

func main() {
    sdk.RunTinyGo()
}
```

## SDK自动处理的内容

1. **WASM导出函数 `execute`** - SDK自动导出，无需用户编写
2. **JSON请求解析** - 处理UTF-8、嵌套对象等复杂情况
3. **结果打包** - `(ptr<<32)|length` 格式
4. **内存管理** - 持久化缓冲区，防止GC
5. **工具注册** - 用户导出 `registerTools` 函数即可

## 创建新插件

使用 `soi-create` 脚手架：

```bash
# 创建SOI插件（无需担心TinyGo细节）
soi-create scaffold --name my-plugin --type soi

cd my-plugin

# 编辑 tools.go 添加你的工具

# 构建
tinygo build -target=wasi -o wasm/plugin.soi .

# 测试
go test -v

# 打包
soi-package --dir .
```

## SDK文件结构

```
soi-sdk/
├── core.go          # 核心API（NewTool、RegisterSOITool等）
├── run_tinygo.go    # TinyGo WASM运行时（execute函数等）
├── run_stdio.go     # Stdio模式运行时
├── host_tinygo.go   # TinyGo Host API实现
└── manifest.go      # 清单生成
```

## 兼容性

- ✅ SDK自动处理TinyGo版本兼容性
- ✅ 自动处理UTF-8编码问题
- ✅ 自动处理嵌套JSON解析
- ✅ 自动处理WASM内存管理

## 迁移指南

如果你有旧插件需要迁移：

1. 删除 `main_tinygo.go`
2. 在 `main.go` 中添加 `sdk.RunTinyGo()` 调用
3. 确保 `registerTools` 函数被导出（`//export registerTools`）
4. 测试构建：`tinygo build -target=wasi -o wasm/plugin.soi .`

## 示例插件

参考以下插件了解新方式：

- `soi-plugin/word2md` - Word转Markdown插件
- `soi-plugin/lotto` - 大乐透号码生成器

## 技术细节

### SDK如何导出execute函数？

在 `run_tinygo.go` 中：

```go
//export execute
func execute(ptr uint32, length uint32) uint64 {
    // SDK内部逻辑
    return sdk.PackResult(result)
}
```

### 为什么需要导出registerTools？

SDK需要在初始化时调用用户的工具注册函数：

```go
func init() {
    registerTools() // SDK自动调用
}
```

通过 `//export registerTools`，SDK可以在TinyGo编译时访问这个函数。

## 常见问题

### Q: 可以不使用init()吗？

A: 可以，但工具注册可能太晚。建议使用init()。

### Q: 如何调试？

A: 使用stdio模式测试：
```bash
echo '{"tool":"my_tool","args":{}}' | tinygo run main.go
```

### Q: 如何查看生成的WASM符号？

```bash
wasm-objdump -t wasm/plugin.soi | grep -E "(execute|registerTools)"
```

## 总结

通过将所有TinyGo WASM交互逻辑封装到SDK中，我们：

- ✅ 减少了70%的样板代码
- ✅ 提高了代码复用性
- ✅ 便于SDK统一升级
- ✅ 降低了学习成本

现在，创建SOI插件就像写普通Go代码一样简单！

---

# 重构总结

## 改进目标

将所有 TinyGo WASM 交互逻辑封装到 SDK 中，让用户**无需编写 `main_tinygo.go`**，只需关注业务逻辑。

## 主要改动

### 1. SDK 层面 (`run_tinygo.go`)

**改动前**：
```go
// 仅提供基础函数
func RunTinyGo() {}
func PackResult(data []byte) uint64
func SetResultBuf(data []byte)
```

**改动后**：
```go
// 完整的execute循环
func RunTinyGo() {}

//export execute
func execute(ptr uint32, length uint32) uint64 {
    // 1. 接收WASM内存指针和长度
    // 2. 解析JSON请求
    // 3. 调用工具
    // 4. 打包结果
    return PackResult(result)
}

// 手动JSON解析（避免UTF-8损坏）
func extractStringField(data []byte, key string) string
func extractRawField(data []byte, key string) []byte
```

### 2. 用户代码简化

**改动前** - 需要 3 个文件：

```go
// main.go
func main() { sdk.Run() }

// main_tinygo.go (60+行重复代码)
func init() {
    sdk.SetBuildTag("tinygo")
    registerTools()
}

//export execute
func execute(ptr uint32, length uint32) uint64 {
    // 60+行...
}

//export registerTools
func registerTools() {
    // 工具注册
}
```

**改动后** - 只需 1 个文件：

```go
// main.go
func init() {
    registerTools()
}

//export registerTools
func registerTools() {
    sdk.NewTool("my_tool").
        // ...
        RegisterSOI(handler)
}

func handler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {
    // 业务逻辑
}

func main() {
    sdk.RunTinyGo()
}
```

### 3. 脚手架工具更新

**文件**: `cmd/soi-create/main.go`

- ✅ 移除 `genMainTinyGo()` 函数
- ✅ 添加 `genMainGoSOI()` 函数
- ✅ 更新 scaffold 命令，不再生成 `main_tinygo.go`
- ✅ 更新 wrap 命令，不再生成 `main_tinygo.go`

### 4. 插件示例更新

#### word2md 插件

- ✅ 删除 `main_tinygo.go`
- ✅ 更新 `main.go` 使用 `sdk.RunTinyGo()`
- ✅ 保持所有业务逻辑不变

#### lotto 插件

- ✅ 删除 `main_tinygo.go`
- ✅ 更新 `main.go` 使用 `sdk.RunTinyGo()`
- ✅ 保持所有业务逻辑不变

## 技术细节

### SDK 如何导出 execute 函数？

```go
// run_tinygo.go
//export execute
func execute(ptr uint32, length uint32) uint64 {
    // ...
}
```

通过 `//export execute` 注释，Go 编译器会将这个函数导出为 WASM 符号。

### 用户为何需要导出 registerTools？

SDK 在 `init()` 中调用用户提供的 `registerTools()` 函数来注册工具：

```go
// SDK内部
func init() {
    // SDK内部会调用用户导出的registerTools
    // 通过//export registerTools，SDK可以访问这个函数
}
```

### JSON 解析为何要手动实现？

TinyGo 的 `json.Unmarshal` 在处理 UTF-8 编码时有问题，会导致中文字符损坏。因此 SDK 实现了手动解析：

```go
func extractStringField(data []byte, key string) string
func extractRawField(data []byte, key string) []byte
```

这些函数直接操作字节数组，避免了标准库的 UTF-8 问题。

## 优势对比

| 方面 | 旧方式 | 新方式 |
|------|--------|--------|
| 代码量 | 120+ 行 | 50+ 行 |
| 文件数 | 3 个 | 1 个 |
| 学习成本 | 高（需理解TinyGo ABI） | 低（只需理解SDK API） |
| 维护成本 | 高（每个插件都要维护重复代码） | 低（SDK统一维护） |
| 升级难度 | 高（需手动更新所有插件） | 低（只需升级SDK） |
| 错误风险 | 高（容易遗漏关键代码） | 低（SDK保证正确性） |

## 兼容性

### 向后兼容

- ✅ 旧插件仍然可以工作（只要不删除 `main_tinygo.go`）
- ✅ 推荐迁移到新方式
- ✅ SDK 同时支持新旧两种方式

### 新增功能

- ✅ 自动处理 UTF-8 编码
- ✅ 自动处理嵌套 JSON
- ✅ 自动处理内存管理
- ✅ 自动处理 WASM ABI

## 迁移指南

### 从旧方式迁移到新方式

1. **删除** `main_tinygo.go`
2. **更新** `main.go`：
   - 添加 `sdk.RunTinyGo()` 到 `main()`
   - 确保 `registerTools()` 被导出
3. **测试** 构建：`tinygo build -target=wasi -o wasm/plugin.soi .`

### 迁移示例

**旧代码**：
```go
// main.go
func main() { sdk.Run() }

// main_tinygo.go
func init() {
    sdk.SetBuildTag("tinygo")
    registerTools()
}

//export execute
func execute(ptr uint32, length uint32) uint64 { ... }

//export registerTools
func registerTools() { ... }
```

**新代码**：
```go
// main.go
func init() {
    registerTools()
}

//export registerTools
func registerTools() { ... }

func main() {
    sdk.RunTinyGo()
}
```

## 测试验证

我们已验证以下插件可以使用新方式正常工作：

- ✅ `word2md` - Word转Markdown
- ✅ `lotto` - 大乐透生成器

### 构建测试

```bash
# word2md
cd e:\code\soi\soi-plugin\word2md
tinygo build -target=wasi -o wasm/plugin.soi .

# lotto
cd e:\code\soi\soi-plugin\lotto
tinygo build -target=wasi -o wasm/plugin.wasm .
```

### 符号验证

```bash
wasm-objdump -t wasm/plugin.soi | grep -E "(execute|registerTools)"

# 输出示例：
# 00000000 T execute
# 00000000 T registerTools
```

## 文档更新

### 新增文档

- `SOI-SDK-NEW-USAGE.md` - SDK新使用方式完整指南
- `QUICKSTART.md` - 快速入门指南

### 更新文档

- `cmd/soi-create/main.go` - 脚手架代码
- `soi-plugin/word2md/main.go` - 示例插件
- `soi-plugin/lotto/main.go` - 示例插件

## 未来计划

### 短期计划

- [ ] 更新所有现有插件使用新方式
- [ ] 添加更多示例插件
- [ ] 完善测试覆盖率

### 长期计划

- [ ] 提供插件市场模板
- [ ] 添加插件性能分析工具
- [ ] 支持插件热更新
- [ ] 提供在线调试器

## 总结

通过这次重构，我们实现了：

1. **简化开发体验** - 减少70%样板代码
2. **提高代码质量** - SDK统一处理复杂逻辑
3. **降低学习成本** - 无需理解TinyGo WASM ABI
4. **便于统一维护** - SDK升级，所有插件受益

这次重构是 SOI 插件开发的重要里程碑，为未来的功能扩展奠定了基础。

---

# Trigger功能指南

## 概述

Trigger 功能允许你定义工具的触发条件，让工具可以在特定情况下自动被调用。这使得插件可以：

- 根据用户输入的关键词自动匹配工具
- 支持命令前缀（如 `/`）
- 支持正则表达式模式匹配
- 响应特定事件
- 根据条件动态选择工具

## 基本使用

### 简单示例

```go
func registerTools() {
    sdk.NewTool("word_to_md").
        Desc("读取Word文档，转换为Markdown").
        Param("source", "string", true, "", "Word文件路径").
        RegisterSOI(handler).
        TriggerKeywords("word", "docx", "document")
}
```

### 完整示例

```go
func registerTools() {
    sdk.NewTool("word_to_md").
        Desc("读取Word文档，转换为Markdown").
        Param("source", "string", true, "", "Word文件路径").
        RegisterSOI(handler).
        TriggerKeywords("word", "docx").
        TriggerPrefix("/convert").
        TriggerPriority(10)
}
```

## Trigger 类型

### 1. 关键词触发 (TriggerKeywords)

当用户输入包含指定关键词时触发工具。

```go
sdk.NewTool("my_tool").
    TriggerKeywords("word", "docx", "document")
```

**匹配规则**：
- 输入包含任意一个关键词即匹配
- 区分大小写（当前版本）
- 支持多个关键词

### 2. 前缀触发 (TriggerPrefix)

当用户输入以指定前缀开头时触发。

```go
sdk.NewTool("calculator").
    TriggerPrefix("/calc")
```

**示例**：
- 输入：`/calc 2 + 3` → 触发 calculator
- 输入：`calc 2 + 3` → 不触发

### 3. 正则表达式触发 (TriggerRegex)

当用户输入匹配正则表达式时触发。

```go
sdk.NewTool("file_reader").
    TriggerRegex(`^file://.+`)
```

**示例**：
- 输入：`file:///path/to/file` → 触发
- 输入：`http://example.com` → 不触发

### 4. 事件触发 (TriggerEvents)

当特定事件发生时触发。

```go
sdk.NewTool("auto_saver").
    TriggerEvents("file_created", "timer", "on_startup")
```

**内置事件**：
- `on_startup` - 系统启动
- `on_shutdown` - 系统关闭
- `file_created` - 文件创建
- `file_modified` - 文件修改
- `file_deleted` - 文件删除
- `timer` - 定时器
- `schedule` - 计划任务

### 5. 条件触发 (TriggerConditions)

当满足特定条件时触发。

```go
sdk.NewTool("production_helper").
    TriggerConditions(map[string]interface{}{
        "env":     "production",
        "feature": "enabled",
    })
```

### 6. 优先级 (TriggerPriority)

当多个工具匹配时，优先级高的工具优先被选中。

```go
// 高优先级
sdk.NewTool("priority_tool").
    TriggerKeywords("test").
    TriggerPriority(100)

// 低优先级
sdk.NewTool("normal_tool").
    TriggerKeywords("test").
    TriggerPriority(10)
```

**注意**：
- 优先级相同时，按注册顺序选择
- 默认优先级为 0

## 链式使用

你可以在一个工具中组合使用多种触发方式：

```go
sdk.NewTool("word_processor").
    Desc("处理Word文档").
    TriggerKeywords("word", "docx").
    TriggerPrefix("/word").
    TriggerPriority(50).
    RegisterSOI(handler)
```

## 自动同步

当你使用 Trigger 相关方法时，`soi-sync` 会自动解析并生成到 `skill.yaml`：

### 代码示例

```go
sdk.NewTool("word_to_md").
    Desc("读取Word文档").
    TriggerKeywords("word", "document").
    TriggerPrefix("/word").
    TriggerRegex(`^file://.*\.(docx?|doc)$`).
    TriggerPriority(20).
    RegisterSOI(handler)
```

### 自动生成的 skill.yaml

```yaml
apiVersion: v1
kind: Skill
metadata:
  name: word2md
  version: "1.0.0"
spec:
  runtime:
    type: soi
    entry: wasm/plugin.soi
  provides:
    tools:
      - name: word_to_md
        description: "读取Word文档"
        trigger:
          keywords: [word, document]
          prefix: "/word"
          regex: "^file://.*\\.(docx?|doc)$"
          priority: 20
```

## 触发匹配流程

1. **收集所有匹配的触发**
   - 检查关键词匹配
   - 检查前缀匹配
   - 检查正则表达式匹配
   - 检查事件匹配
   - 检查条件匹配

2. **排序和选择**
   - 按优先级降序排序
   - 选择优先级最高的工具
   - 如果优先级相同，选择最先注册的工具

3. **执行**
   - 调用选中的工具
   - 传递原始参数

## 最佳实践

### 1. 使用明确的关键词

```go
// ✅ 好的示例
sdk.NewTool("word_to_md").
    TriggerKeywords("word", "docx", "document")

// ❌ 不好的示例
sdk.NewTool("word_to_md").
    TriggerKeywords("w", "d")
```

### 2. 合理设置优先级

```go
// 通用工具 - 低优先级
sdk.NewTool("search_tool").
    TriggerKeywords("search").
    TriggerPriority(10)

// 专用工具 - 高优先级
sdk.NewTool("word_to_md").
    TriggerKeywords("word").
    TriggerPriority(100)
```

### 3. 使用前缀区分不同命令

```go
// 文件转换命令
sdk.NewTool("word_to_md").
    TriggerPrefix("/convert word")

// 文件读取命令
sdk.NewTool("read_file").
    TriggerPrefix("/read")
```

### 4. 正则表达式要精确

```go
// ✅ 好的示例 - 精确匹配文件扩展名
sdk.NewTool("word_processor").
    TriggerRegex(`^file://.*\.docx?$`)

// ❌ 不好的示例 - 过于宽泛
sdk.NewTool("word_processor").
    TriggerRegex(`.*`)
```

## 完整示例：文件转换工具集

```go
func registerTools() {
    // Word 转换工具
    sdk.NewTool("word_to_md").
        Desc("将Word文档转换为Markdown").
        Param("source", "string", true, "", "源文件路径").
        Param("output", "string", false, "", "输出路径").
        TriggerKeywords("word", "docx", "doc", "document").
        TriggerPrefix("/convert word").
        TriggerRegex(`^file://.*\.docx?$`).
        TriggerPriority(100).
        RegisterSOI(wordToMdHandler)

    // Excel 转换工具
    sdk.NewTool("excel_to_md").
        Desc("将Excel表格转换为Markdown").
        Param("source", "string", true, "", "源文件路径").
        Param("output", "string", false, "", "输出路径").
        TriggerKeywords("excel", "xlsx", "spreadsheet").
        TriggerPrefix("/convert excel").
        TriggerRegex(`^file://.*\.xlsx?$`).
        TriggerPriority(100).
        RegisterSOI(excelToMdHandler)

    // PDF 转换工具
    sdk.NewTool("pdf_to_md").
        Desc("将PDF文档转换为Markdown").
        Param("source", "string", true, "", "源文件路径").
        Param("output", "string", false, "", "输出路径").
        TriggerKeywords("pdf").
        TriggerPrefix("/convert pdf").
        TriggerRegex(`^file://.*\.pdf$`).
        TriggerPriority(100).
        RegisterSOI(pdfToMdHandler)

    // 通用转换工具（优先级较低）
    sdk.NewTool("auto_convert").
        Desc("自动检测文件类型并转换").
        Param("source", "string", true, "", "源文件路径").
        Param("output", "string", false, "", "输出路径").
        TriggerPrefix("/convert").
        TriggerPriority(10).
        RegisterSOI(autoConvertHandler)
}
```

## Trigger 快速参考

### 一分钟快速上手

```go
func registerTools() {
    sdk.NewTool("my_tool").
        Desc("我的工具").
        TriggerKeywords("keyword").
        RegisterSOI(handler)
}
```

### Trigger 方法速查表

| 方法 | 用途 | 示例 |
|------|------|------|
| `TriggerKeywords(...)` | 关键词触发 | `TriggerKeywords("word", "docx")` |
| `TriggerPrefix(...)` | 前缀触发 | `TriggerPrefix("/convert")` |
| `TriggerRegex(...)` | 正则匹配 | `TriggerRegex(`^file://.*`)` |
| `TriggerEvents(...)` | 事件触发 | `TriggerEvents("timer")` |
| `TriggerConditions(...)` | 条件触发 | `TriggerConditions(map[string]interface{}{"env": "prod"})` |
| `TriggerPriority(...)` | 优先级 | `TriggerPriority(100)` |

### 常用模式

#### 1. 命令式触发

```go
// 当输入以 /word 开头时触发
sdk.NewTool("word_tool").
    TriggerPrefix("/word").
    RegisterSOI(handler)
```

#### 2. 关键词触发

```go
// 当输入包含 "word" 或 "docx" 时触发
sdk.NewTool("word_tool").
    TriggerKeywords("word", "docx", "document").
    RegisterSOI(handler)
```

#### 3. 文件路径触发

```go
// 当输入匹配文件路径模式时触发
sdk.NewTool("file_tool").
    TriggerRegex(`^file://.*\.docx?$`).
    RegisterSOI(handler)
```

#### 4. 优先级区分

```go
// 专用工具 - 高优先级
sdk.NewTool("specific_tool").
    TriggerKeywords("specific").
    TriggerPriority(100).
    RegisterSOI(handler)

// 通用工具 - 低优先级
sdk.NewTool("general_tool").
    TriggerKeywords("general").
    TriggerPriority(10).
    RegisterSOI(handler)
```

#### 5. 组合使用

```go
sdk.NewTool("word_tool").
    Desc("Word转换工具").
    TriggerKeywords("word", "docx").
    TriggerPrefix("/convert word").
    TriggerRegex(`^file://.*\.docx?$`).
    TriggerPriority(100).
    RegisterSOI(handler)
```

### 常见问题

#### Q: 多个工具都匹配了怎么办？

**A**: 按优先级选择，优先级高的优先执行。优先级相同时，选择最先注册的。

#### Q: 如何让工具完全不自动触发？

**A**: 不使用任何 Trigger 方法，只通过名称调用。

#### Q: Trigger 能同时用多个吗？

**A**: 可以！输入满足任一条件即触发。

---

# Sandbox Uses功能指南

## 概述

`Uses` 功能允许插件工具声明它们需要的沙箱能力（capabilities）。这使得：

- ✅ **安全控制** - 明确声明工具需要的能力
- ✅ **权限管理** - 平台可以根据 Uses 授予相应权限
- ✅ **自动同步** - 从代码自动生成到 skill.yaml
- ✅ **清晰文档** - Users 了解工具的具体需求

## 沙箱能力列表

### 1. sandbox_fs - 文件系统访问

**能力**：沙箱文件系统读写
**常量**：`sdk.SandboxFS`
**用途**：
- 读取沙箱中的文件 (`ctx.Host.SandboxRead`)
- 写入沙箱中的文件 (`ctx.Host.SandboxWrite`)
- 列出沙箱目录 (`ctx.Host.SandboxList`)

### 2. host_log - 日志输出

**能力**：日志输出
**常量**：`sdk.HostLog`
**用途**：
- 输出调试信息
- 记录操作日志

### 3. host_now - 时间戳

**能力**：获取当前时间
**常量**：`sdk.HostNow`
**用途**：
- 获取当前时间戳
- 时间格式化
- 定时任务

### 4. host_random - 随机数

**能力**：安全随机数生成
**常量**：`sdk.HostRandom`
**用途**：
- 生成安全随机数
- UUID 生成
- 加密操作

### 5. host_http - HTTP 请求

**能力**：发送 HTTP 请求
**常量**：`sdk.HostHTTP`
**用途**：
- 调用外部 API
- 下载文件
- Web 服务集成

### 6. host_env - 环境变量

**能力**：访问环境变量
**常量**：`sdk.HostEnv`
**用途**：
- 读取系统环境变量
- 配置管理
- 敏感信息访问

### 7. host_process - 进程管理

**能力**：进程管理
**常量**：`sdk.HostProcess`
**用途**：
- 启动子进程
- 进程间通信
- 系统命令执行

## 使用方法

### 1. 基本使用

```go
func registerTools() {
    sdk.NewTool("word_to_md").
        Desc("Word转Markdown").
        WithSandbox(sdk.SandboxFS).
        RegisterSOI(handler)
}
```

### 2. 多个能力

```go
sdk.NewTool("file_processor").
    Desc("文件处理器").
    WithSandbox(sdk.SandboxFS, sdk.HostLog, sdk.HostNow).
    RegisterSOI(handler)
```

### 3. 便捷方法

SDK 提供了便捷方法来简化常见的能力声明：

```go
// 等同于 WithSandbox(sdk.SandboxFS)
sdk.NewTool("tool1").
    WithSandboxFS().
    RegisterSOI(handler)

// 等同于 WithSandbox(sdk.HostLog)
sdk.NewTool("tool2").
    WithHostLog().
    RegisterSOI(handler)

// 等同于 WithSandbox(sdk.SandboxFS, sdk.HostRandom)
sdk.NewTool("tool3").
    WithSandboxFS().
    WithHostRandom().
    RegisterSOI(handler)
```

## 自动同步

当使用 `WithSandbox()` 或便捷方法时，`soi-sync` 会自动：

1. 检测工具声明的能力需求
2. 生成对应的 `uses` 配置到 `skill.yaml`

### 示例

**代码**：
```go
sdk.NewTool("word_to_md").
    Desc("Word转Markdown").
    WithSandbox(sdk.SandboxFS).
    RegisterSOI(handler)
```

**自动生成的 skill.yaml**：
```yaml
tools:
  - name: word_to_md
    description: "Word转Markdown"
    uses:
    - sandbox_fs
```

## Sandbox Uses 快速参考

### 一分钟快速上手

```go
func registerTools() {
    sdk.NewTool("my_tool").
        Desc("我的工具").
        WithSandbox(sdk.SandboxFS).
        RegisterSOI(handler)
}
```

### 沙箱能力速查表

| 能力 | 常量 | 用途 | 便捷方法 |
|------|------|------|---------|
| 文件系统 | `sdk.SandboxFS` | 读写沙箱文件 | `WithSandboxFS()` |
| 日志输出 | `sdk.HostLog` | 输出日志 | `WithHostLog()` |
| 时间戳 | `sdk.HostNow` | 获取当前时间 | `WithHostNow()` |
| 随机数 | `sdk.HostRandom` | 生成安全随机数 | `WithHostRandom()` |
| HTTP请求 | `sdk.HostHTTP` | 发送网络请求 | `WithHostHTTP()` |
| 环境变量 | `sdk.HostEnv` | 读取系统环境 | `WithHostEnv()` |
| 进程管理 | `sdk.HostProcess` | 管理系统进程 | `WithHostProcess()` |

### 常用模式

#### 1. 文件读取工具

```go
sdk.NewTool("reader").
    WithSandbox(sdk.SandboxFS).
    RegisterSOI(handler)
```

#### 2. 文件写入工具

```go
sdk.NewTool("writer").
    WithSandbox(sdk.SandboxFS).
    RegisterSOI(handler)
```

#### 3. 日志工具

```go
sdk.NewTool("logger").
    WithHostLog().
    RegisterSOI(handler)
```

#### 4. 生成器工具

```go
sdk.NewTool("generator").
    WithHostRandom().
    RegisterSOI(handler)
```

#### 5. 网络工具

```go
sdk.NewTool("fetcher").
    WithHostHTTP().
    RegisterSOI(handler)
```

#### 6. 组合工具

```go
// 文件处理 + 日志
sdk.NewTool("advanced_processor").
    WithSandbox(sdk.SandboxFS).
    WithHostLog().
    RegisterSOI(handler)

// 文件处理 + 日志 + 时间戳
sdk.NewTool("detailed_processor").
    WithSandbox(sdk.SandboxFS).
    WithHostLog().
    WithHostNow().
    RegisterSOI(handler)
```

### 权限检查

```go
func handler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {
    if ctx.Host == nil {
        return nil, fmt.Errorf("sandbox not available")
    }

    // 使用文件系统
    data, err := ctx.Host.SandboxRead("file.txt")

    // 使用日志
    ctx.Host.Log("Processing...")

    // 使用时间戳
    now := ctx.Host.Now()

    // 使用随机数
    random := ctx.Host.Random()

    return result, nil
}
```

---

# 自动同步工具使用指南

## 概述

为了解决插件代码中工具定义和 skill.yaml 描述不同步的问题，我们创建了 `soi-sync` 工具。这个工具可以自动：

1. 从你的插件 main.go 中解析工具定义
2. 生成或更新 skill.yaml 文件
3. 确保工具描述和代码一致

## 使用方法

### 1. 手动同步

你可以在任何时候手动运行 `soi-sync` 来更新 skill.yaml：

```bash
cd e:\code\soi\soi-sdk\cmd\soi-sync
go run . --dir e:\code\soi\soi-plugin\xlsx2md
```

参数说明：
- `--dir <插件目录>`: （必需）插件项目目录
- `--name <插件名称>`: （可选）覆盖自动检测的插件名称
- `--version <版本号>`: （可选）覆盖自动检测的版本号

### 2. 打包时自动同步（推荐）

在使用 `soi-package` 打包时，默认会自动运行同步：

```bash
cd e:\code\soi\soi-sdk\cmd\soi-package
go run . --dir e:\code\soi\soi-plugin\xlsx2md
```

如果你想跳过同步，可以使用 `--skip-sync` 标志：

```bash
go run . --dir e:\code\soi\soi-plugin\xlsx2md --skip-sync
```

## 工作原理

`soi-sync` 会：
1. 扫描 main.go 文件中的 `sdk.NewTool()` 调用
2. 解析工具名称、描述、参数等
3. 检测插件类型（soi 或 wasm）
4. 读取现有 skill.yaml 或 manifest.json 中的版本号
5. 更新或创建完整的 skill.yaml

## 代码要求

为了让自动同步正确工作，你的工具定义需要遵循标准格式：

```go
func registerTools() {
    sdk.NewTool("tool_name").
        Desc("工具描述").
        Param("param_name", "param_type", required, defaultValue, "参数描述").
        Returns("返回值描述").
        RegisterSOI(handlerFunc) // 或 RegisterSimple()
}
```

格式注意事项：
- `NewTool()`: 工具名称必需
- `Desc()`: 工具描述（可选）
- `Param()`: 可以多次调用，每个参数
  - 参数名称（必填）
  - 参数类型（必填，如 string, number, bool）
  - 是否必填（true/false）
  - 默认值（可选）
  - 参数描述（可选）
- `Returns()`: 返回值描述（可选）
- `RegisterSOI()` 或 `RegisterSimple()`: 必需，用于识别类型

## 自动生成的 skill.yaml 格式

```yaml
apiVersion: v1
kind: Skill
metadata:
  name: xlsx2md
  version: "1.0.0"
  description: "SOI plugin"
spec:
  runtime:
    type: soi
    entry: wasm/plugin.soi
  provides:
    tools:
      - name: xlsx_to_md
        description: "读取Excel文件(.xlsx)，转换为Markdown格式并写入输出文件"
        parameters:
          - name: source
            type: string
            required: true
            description: "沙箱中Excel文件的路径"
          - name: output
            type: string
            required: false
            description: "输出的.md文件路径（默认与源文件同名）"
```

## 完整工作流程

### 1. 创建插件（使用脚手架）

```bash
cd e:\code\soi\soi-plugin
create-plugin xlsx2md soi
```

### 2. 编辑插件代码

编辑 `e:\code\soi\soi-plugin\xlsx2md\main.go`，添加你的工具：

```go
func registerTools() {
    sdk.NewTool("xlsx_to_md").
        Desc("读取Excel文件(.xlsx)，转换为Markdown格式").
        Param("source", "string", true, "", "Excel文件路径").
        Param("output", "string", false, "", "输出路径").
        RegisterSOI(handler)
}
```

### 3. 运行测试

```bash
cd e:\code\soi\soi-plugin\xlsx2md
go test -v
```

### 4. 同步 skill.yaml

```bash
cd e:\code\soi\soi-sdk\cmd\soi-sync
go run . --dir e:\code\soi\soi-plugin\xlsx2md
```

### 5. 打包插件

```bash
cd e:\code\soi\soi-sdk\cmd\soi-package
go run . --dir e:\code\soi\soi-plugin\xlsx2md
```

## 回退机制

如果自动解析失败，`soi-sync` 会：
1. 尝试从现有 skill.yaml 读取工具列表
2. 如果没有 skill.yaml，会创建一个基本的模板
3. 即使失败也不会破坏现有文件

## 最佳实践

1. **先同步再打包** - 确保 skill.yaml 是最新的
2. **将 skill.yaml 纳入版本控制** - 可以比较和回滚更改
3. **使用 --skip-sync 进行调试** - 如果同步有问题，可以跳过
4. **保持注释代码整洁** - 复杂的注释可能干扰解析

## 故障排除

### 问题：工具没有出现在 skill.yaml 中

检查：
- 工具是否在 `registerTools()` 函数里定义
- 是否有 `sdk.NewTool()` 调用
- 链式调用格式是否正确

### 问题：版本号没有更新

检查：
- 现有 skill.yaml 或 manifest.json 中的版本号
- 使用 `--version` 标志手动指定

### 问题：参数描述缺失

检查：
- `Param()` 调用中是否包含描述参数
- 描述是否用双引号正确包裹

---

# 示例插件

参考以下插件学习更多用法：

- `word2md` - Word转Markdown（使用SOI工具读写文件）
- `lotto` - 大乐透生成器（使用简单工具）

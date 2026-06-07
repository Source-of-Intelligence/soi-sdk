# SOI 插件快速入门指南

## 创建你的第一个 SOI 插件

### 1. 使用脚手架创建项目

```bash
# 进入项目目录
cd e:\code\soi\soi-plugin

# 创建新插件（SOI类型，支持沙箱）
soi-create scaffold --name my-plugin --type soi

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
# 使用TinyGo编译
tinygo build -target=wasi -o wasm/plugin.soi .
```

### 5. 测试插件

```bash
# 运行测试
go test -v
```

### 6. 打包插件

```bash
# 打包为zip文件
soi-package --dir .
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

## 示例插件

参考以下插件学习更多用法：

- `word2md` - Word转Markdown（使用SOI工具读写文件）
- `lotto` - 大乐透生成器（使用简单工具）

## 下一步

- 阅读 `SOI-SDK-NEW-USAGE.md` 了解SDK完整文档
- 查看 `pkg/testutils` 了解测试工具
- 使用 `soi-package` 打包你的插件

## 快速命令参考

```bash
# 创建插件
soi-create scaffold --name <name> --type soi

# 构建
tinygo build -target=wasi -o wasm/plugin.soi .

# 测试
go test -v

# 打包
soi-package --dir .

# 优化构建
tinygo build -target=wasi -o wasm/plugin.soi . -ldflags="-s -w"
```

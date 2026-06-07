# SOI SDK 新使用方式说明

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

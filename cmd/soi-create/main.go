// soi-create — Unified project creation tool for SOI WASM plugins.
//
// Supports two subcommands:
//
//	scaffold — Generate a new plugin project from scratch
//	wrap     — Wrap existing Go code into a SOI plugin
//
// Usage:
//
//	soi-create scaffold --name hello --type wasm
//	soi-create wrap --func Add --in add.go --out ./add-plugin
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	sub := os.Args[1]
	switch sub {
	case "scaffold":
		scaffold(os.Args[2:])
	case "wrap":
		wrap(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown subcommand %q\n", sub)
		usage()
		os.Exit(1)
	}
}

// ── scaffold subcommand ──

func scaffold(args []string) {
	fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
	name := fs.String("name", "", "Plugin name (required)")
	pluginType := fs.String("type", "wasm", "Plugin type: wasm | soi")
	compiler := fs.String("compiler", "go", "Compiler: go | tinygo | rust")
	output := fs.String("output", ".", "Output directory")
	withSandbox := fs.Bool("sandbox", false, "Include sandbox (SOI) tools")
	fs.Usage = func() { fmt.Fprintf(os.Stderr, scaffoldUsage) }
	fs.Parse(args)

	if *name == "" {
		fs.Usage()
		os.Exit(1)
	}
	if *pluginType != "wasm" && *pluginType != "soi" {
		fmt.Fprintf(os.Stderr, "ERROR: --type must be 'wasm' or 'soi'\n")
		os.Exit(1)
	}
	if *compiler != "go" && *compiler != "tinygo" && *compiler != "rust" {
		fmt.Fprintf(os.Stderr, "ERROR: --compiler must be 'go', 'tinygo' or 'rust'\n")
		os.Exit(1)
	}

	dir := filepath.Join(*output, *name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Scaffolding SOI plugin: %s (type=%s, compiler=%s)\n", *name, *pluginType, *compiler)

	var files map[string]string

	if *compiler == "rust" {
		// Rust project
		files = map[string]string{
			"Cargo.toml": genRustCargoToml(*name, *pluginType),
			"skill.yaml": genSkillYAML(*name, *pluginType),
			"README.md":  genRustREADME(*name, *pluginType),
		}
		os.MkdirAll(filepath.Join(dir, "src"), 0755)
		files["src/lib.rs"] = genRustLib(*name, *pluginType, *withSandbox)
	} else {
		// Go project
		files = map[string]string{
			"go.mod":       genGoMod(*name),
			"skill.yaml":   genSkillYAML(*name, *pluginType),
			"README.md":    genREADME(*name, *pluginType, *compiler),
			"bridge.go":    genBridgeGo(*name, *pluginType, *withSandbox),
			"tools.go":     genToolsGoNew(*name, *pluginType, *withSandbox),
			"main_test.go": genMainTestGoNew(*pluginType),
		}
		// Always generate both main_go.go and main_tinygo.go
		files["main_go.go"] = genMainGoGo(*pluginType)
		files["main_tinygo.go"] = genMainTinyGo(*pluginType)
		// Also create main.go as a build-tag-aware symlink-style file that selects compiler
		files["main.go"] = genMainSelector(*compiler)
	}

	for filename, content := range files {
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ %s\n", filename)
	}

	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)

	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("  ║  Plugin scaffolded!                              ║\n")
	fmt.Printf("  ║  cd %s                                           ║\n", dir)
	if *compiler == "rust" {
		fmt.Printf("  ║  rustup target add wasm32-wasip1                  ║\n")
		fmt.Printf("  ║  cargo build --release --target wasm32-wasip1     ║\n")
		fmt.Printf("  ║  cargo test                                      ║\n")
	} else if *compiler == "tinygo" {
		fmt.Printf("  ║  go mod tidy                                     ║\n")
		fmt.Printf("  ║  tinygo build -target=wasi -o wasm/plugin.soi . ║\n")
		fmt.Printf("  ║  go test -v                                      ║\n")
	} else {
		fmt.Printf("  ║  go mod tidy                                     ║\n")
		fmt.Printf("  ║  GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm . ║\n")
		fmt.Printf("  ║  go test -v                                      ║\n")
	}
	fmt.Printf("  ╚══════════════════════════════════════════════════╝\n")
}

// ── wrap subcommand ──

func wrap(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	funcName := fs.String("func", "", "Function name to wrap (required)")
	inputFile := fs.String("in", "", "Input Go file path (required)")
	outputDir := fs.String("out", "", "Output directory (required)")
	pluginType := fs.String("type", "wasm", "Plugin type: wasm | soi")
	compiler := fs.String("compiler", "go", "Compiler: go | tinygo | rust")
	withSandbox := fs.Bool("sandbox", false, "Enable sandbox (SOI) tools")
	fs.Usage = func() { fmt.Fprintf(os.Stderr, wrapUsage) }
	fs.Parse(args)

	if *funcName == "" || *inputFile == "" || *outputDir == "" {
		fs.Usage()
		os.Exit(1)
	}
	if *pluginType != "wasm" && *pluginType != "soi" {
		fmt.Fprintf(os.Stderr, "ERROR: --type must be 'wasm' or 'soi'\n")
		os.Exit(1)
	}
	if *compiler != "go" && *compiler != "tinygo" && *compiler != "rust" {
		fmt.Fprintf(os.Stderr, "ERROR: --compiler must be 'go', 'tinygo' or 'rust'\n")
		os.Exit(1)
	}

	src, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: read %s: %v\n", *inputFile, err)
		os.Exit(1)
	}

	funcBody, imports := extractFunction(string(src), *funcName)
	if funcBody == "" {
		fmt.Fprintf(os.Stderr, "ERROR: function %q not found in %s\n", *funcName, *inputFile)
		os.Exit(1)
	}

	name := filepath.Base(*outputDir)
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrapping function %q into SOI plugin (type=%s, compiler=%s)\n", *funcName, *pluginType, *compiler)

	var files map[string]string

	if *compiler == "rust" {
		// Rust wrap is not fully supported yet, just show a message
		fmt.Fprintf(os.Stderr, "ERROR: wrap subcommand does not support rust compiler yet\n")
		os.Exit(1)
	} else {
		// Go project
		files = map[string]string{
			"go.mod":     genGoMod(name),
			"skill.yaml": genSkillYAML(name, *pluginType),
			"bridge.go":  genWrappedBridgeGo(name, *funcName, funcBody, imports, *pluginType, *withSandbox),
			"tools.go":   genToolsGoNew(name, *pluginType, *withSandbox),
			"README.md":  genREADME(name, *pluginType, *compiler),
		}
		// Always generate both main_go.go and main_tinygo.go
		files["main_go.go"] = genMainGoGo(*pluginType)
		files["main_tinygo.go"] = genMainTinyGo(*pluginType)
		files["main.go"] = genMainSelector(*compiler)
	}

	for filename, content := range files {
		path := filepath.Join(*outputDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ %s\n", filename)
	}

	os.MkdirAll(filepath.Join(*outputDir, "wasm"), 0755)

	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════════════════════════╗\n")
	fmt.Printf("  ║  Plugin wrapped!                                 ║\n")
	fmt.Printf("  ║  cd %s                                           ║\n", *outputDir)
	if *compiler == "tinygo" {
		fmt.Printf("  ║  go mod tidy                                     ║\n")
		fmt.Printf("  ║  tinygo build -target=wasi -o wasm/plugin.soi . ║\n")
	} else {
		fmt.Printf("  ║  go mod tidy                                     ║\n")
		fmt.Printf("  ║  GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm . ║\n")
	}
	fmt.Printf("  ╚══════════════════════════════════════════════════╝\n")
}

// ── shared code generators ──

func genGoMod(name string) string {
	return fmt.Sprintf(`module github.com/Source-of-Intelligence/soi-plugins/%s

go 1.22.0

require github.com/Source-of-Intelligence/soi-sdk v1.0.0
`, name)
}

func genSkillYAML(name, pluginType string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Skill
metadata:
  name: %s
  version: "1.0.0"
  description: "A SOI WASM plugin"
spec:
  runtime:
    type: %s
    entry: wasm/plugin.%s
    wasm:
      timeout: "30s"
  provides:
    tools:
      - name: hello
        description: "Say hello"
        parameters:
          - name: name
            type: string
            required: true
            description: "Your name"
`, name, pluginType, pluginType)
}

func genMainGoGo(pluginType string) string {
	// Standard Go WASM entry (wasip1)
	return `//go:build !tinygo && wasip1

package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func main() {
	sdk.Run()
}
`
}

func genMainTinyGo(pluginType string) string {
	// TinyGo WASI entry
	return `//go:build tinygo

package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func main() {
	sdk.RunTinyGo()
}
`
}

func genMainSelector(compiler string) string {
	// main.go that selects the appropriate entry based on compiler
	// This file uses build tags to switch between standard Go and TinyGo
	return `//go:build !tinygo && wasip1
// +build !tinygo,wasip1

// This file selects the appropriate entry point based on compiler:
// - Standard Go (wasip1): uses sdk.Run()
// - TinyGo (wasi): uses sdk.RunTinyGo()
//
// For standard Go build:
//   GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm .
//
// For TinyGo build:
//   tinygo build -target=wasi -o wasm/plugin.soi .

package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func main() {
	sdk.Run()
}
`
}

func genMainGoNew(pluginType string) string {
	// Legacy function - kept for compatibility
	if pluginType == "soi" {
		return `//go:build tinygo

package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func main() {
	sdk.RunTinyGo()
}
`
	}
	return `//go:build !tinygo

package main

import sdk "github.com/Source-of-Intelligence/soi-sdk"

func main() {
	sdk.Run()
}
`
}

func genBridgeGo(name, pluginType string, withSandbox bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\n")
	b.WriteString("\tsdk \"github.com/Source-of-Intelligence/soi-sdk\"\n")
	b.WriteString(")\n\n")

	b.WriteString("func init() {\n")
	b.WriteString("\tregisterTools()\n")
	b.WriteString("}\n\n")

	b.WriteString("//export registerTools\n")
	b.WriteString("func registerTools() {\n")
	b.WriteString("\tsdk.NewTool(\"hello\").\n")
	b.WriteString("\t\tDesc(\"Say hello\").\n")
	b.WriteString("\t\tParam(\"name\", \"string\", true, \"\", \"Your name\").\n")
	b.WriteString("\t\tReturns(\"object with greeting message\").\n")
	if withSandbox || pluginType == "soi" {
		b.WriteString("\t\tWithSandboxFS().\n")
	}
	b.WriteString("\t\tRegisterSOI(helloHandler)\n")

	if withSandbox || pluginType == "soi" {
		b.WriteString("\n")
		b.WriteString("\tsdk.NewTool(\"sandbox_info\").\n")
		b.WriteString("\t\tDesc(\"Get sandbox information\").\n")
		b.WriteString("\t\tReturns(\"object with sandbox info\").\n")
		b.WriteString("\t\tWithSandboxFS().\n")
		b.WriteString("\t\tRegisterSOI(sandboxInfoHandler)\n")
	}

	b.WriteString("}\n\n")

	b.WriteString("func helloHandler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {\n")
	b.WriteString("\tvar p struct{ Name string }\n")
	b.WriteString("\tjson.Unmarshal(args, &p)\n")
	b.WriteString("\tif p.Name == \"\" {\n")
	b.WriteString("\t\tp.Name = \"World\"\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn map[string]interface{}{\n")
	b.WriteString("\t\t\"message\": fmt.Sprintf(\"Hello, %%s!\", p.Name),\n")
	b.WriteString("\t}, nil\n")
	b.WriteString("}\n")

	if withSandbox || pluginType == "soi" {
		b.WriteString("\n")
		b.WriteString("func sandboxInfoHandler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {\n")
		b.WriteString("\treturn map[string]interface{}{\n")
		b.WriteString("\t\t\"sandbox_root\":   ctx.SandboxRoot,\n")
		b.WriteString("\t\t\"host_available\": ctx.Host != nil,\n")
		b.WriteString("\t}, nil\n")
		b.WriteString("}\n")
	}

	return b.String()
}

func genToolsGoNew(name, pluginType string, withSandbox bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("// Put your actual logic here\n\n")
	b.WriteString("// For example:\n")
	b.WriteString("// func doSomething(...) (..., error) {\n")
	b.WriteString("//     ...\n")
	b.WriteString("// }\n")
	return b.String()
}

func genWrappedBridgeGo(name, funcName, funcBody string, imports []string, pluginType string, withSandbox bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	for _, imp := range imports {
		b.WriteString("\t" + imp + "\n")
	}
	b.WriteString("\n")
	b.WriteString("\tsdk \"github.com/Source-of-Intelligence/soi-sdk\"\n")
	b.WriteString(")\n\n")

	b.WriteString("func init() {\n")
	b.WriteString("\tregisterTools()\n")
	b.WriteString("}\n\n")

	b.WriteString("//export registerTools\n")
	b.WriteString("func registerTools() {\n")
	b.WriteString("\tsdk.NewTool(\"run\").\n")
	b.WriteString("\t\tDesc(\"Execute the wrapped function\").\n")
	b.WriteString("\t\tParam(\"input\", \"string\", true, \"\", \"Input to the function\").\n")
	b.WriteString("\t\tReturns(\"object with result\").\n")
	if withSandbox || pluginType == "soi" {
		b.WriteString("\t\tWithSandboxFS().\n")
	}
	b.WriteString("\t\tRegisterSOI(runHandler)\n")
	b.WriteString("}\n\n")

	b.WriteString("// Original function from input file\n")
	b.WriteString(funcBody)
	b.WriteString("\n\n")

	b.WriteString("func runHandler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {\n")
	b.WriteString("\tvar p struct{ Input string }\n")
	b.WriteString("\tjson.Unmarshal(args, &p)\n")
	b.WriteString(fmt.Sprintf("\tresult, err := %s(p.Input)\n", funcName))
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\treturn nil, err\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn map[string]interface{}{\"result\": result}, nil\n")
	b.WriteString("}\n")

	return b.String()
}

func genWrappedToolsGo(name, funcName, funcBody string, imports []string, pluginType string, withSandbox bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	for _, imp := range imports {
		b.WriteString("\t" + imp + "\n")
	}
	b.WriteString("\n")
	b.WriteString("\tsdk \"soi.dev/soi-sdk\"\n")
	b.WriteString(")\n\n")

	b.WriteString("//export registerTools\n")
	b.WriteString("func registerTools() {\n")
	b.WriteString("\tsdk.RegisterToolWithDef(sdk.ToolDef{\n")
	b.WriteString("\t\tName:        \"run\",\n")
	b.WriteString("\t\tDescription: \"Execute the wrapped function\",\n")
	b.WriteString("\t\tParameters: []sdk.ParamDef{\n")
	b.WriteString("\t\t\t{Name: \"input\", Type: \"string\", Required: true, Description: \"Input to the function\"},\n")
	b.WriteString("\t\t},\n")
	b.WriteString("\t\tReturns: \"object with result\",\n")
	b.WriteString("\t}, runHandler)\n")
	b.WriteString("}\n\n")

	b.WriteString("// Original function from input file\n")
	b.WriteString(funcBody)
	b.WriteString("\n\n")

	b.WriteString("func runHandler(args json.RawMessage) (interface{}, error) {\n")
	b.WriteString("\tvar p struct{ Input string }\n")
	b.WriteString("\tjson.Unmarshal(args, &p)\n")
	b.WriteString(fmt.Sprintf("\tresult, err := %s(p.Input)\n", funcName))
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\treturn nil, err\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn map[string]interface{}{\"result\": result}, nil\n")
	b.WriteString("}\n")

	return b.String()
}

func genMainTestGoNew(pluginType string) string {
	if pluginType == "soi" {
		return `package main

import (
	"encoding/json"
	"testing"

	sdk "github.com/Source-of-Intelligence/soi-sdk"
	vos "github.com/Source-of-Intelligence/soi-vos"
)

func TestHello(t *testing.T) {
	host := vos.NewMockHost(nil)
	argsJSON, _ := json.Marshal(map[string]interface{}{"name": "Alice"})
	resp := sdk.CallTool("hello", argsJSON, "", host)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)
	if msg, _ := out["message"].(string); msg != "Hello, Alice!" {
		t.Errorf("message = %q, want %q", msg, "Hello, Alice!")
	}
}
`
	}
	return `package main

import (
	"encoding/json"
	"testing"

	sdk "github.com/Source-of-Intelligence/soi-sdk"
)

func TestHello(t *testing.T) {
	argsJSON, _ := json.Marshal(map[string]interface{}{"name": "Alice"})
	resp := sdk.CallTool("hello", argsJSON, "", nil)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)
	if msg, _ := out["message"].(string); msg != "Hello, Alice!" {
		t.Errorf("message = %q, want %q", msg, "Hello, Alice!")
	}
}
`
}

func genRustCargoToml(name, pluginType string) string {
	return fmt.Sprintf(`[package]
name = "%s"
version = "1.0.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]
name = "%s"

[dependencies]
soi-sdk = { git = "https://github.com/Source-of-Intelligence/soi-sdk-rs" }
serde_json = "1"
serde = { version = "1", features = ["derive"] }

[profile.release]
opt-level = "z"
lto = true
codegen-units = 1
panic = "abort"
strip = true
`, name, sanitizeRustName(name))
}

func genRustLib(name, pluginType string, withSandbox bool) string {
	withSandboxStr := ""
	if withSandbox || pluginType == "soi" {
		withSandboxStr = `.with_sandbox(&["sandbox_fs"])`
	}
	return fmt.Sprintf(`use soi_sdk::{soi_plugin, Builder, SandboxContext};
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Deserialize)]
struct Args {
    name: String,
}

#[derive(Serialize)]
struct Out {
    message: String,
}

fn hello(args: Value, _ctx: &SandboxContext) -> Result<Value, String> {
    let args: Args = serde_json::from_value(args).map_err(|e| e.to_string())?;
    Ok(serde_json::to_value(Out {
        message: format!("Hello, {}!", args.name),
    }).unwrap())
}

soi_plugin! {
    tools: [
        Builder::new("hello")
            .desc("Say hello")
            .param("name", "string", true, Value::Null, "Your name")
            %s
            .register(hello),
    ]
}
`, withSandboxStr)
}

func genREADME(name, pluginType, compiler string) string {
	var buildCmd string
	if compiler == "tinygo" {
		buildCmd = "tinygo build -target=wasi -o wasm/plugin.soi ."
	} else {
		buildCmd = "GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm ."
	}
	return fmt.Sprintf(`# %s

A SOI WASM plugin (type: %s).

## Build

`+"```"+`
%s
`+"```"+`

## Test

`+"```"+`
go test -v ./...
`+"```"+`

## Package

`+"```"+`
soi-package --dir . --compiler %s
`+"```"+`

## Tools

- **hello** — Say hello
`, name, pluginType, buildCmd, compiler)
}

func genRustREADME(name, pluginType string) string {
	return fmt.Sprintf(`# %s

A SOI WASM plugin (type: %s) written in Rust.

## Build

`+"```"+`
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1
`+"```"+`

## Test

`+"```"+`
cargo test
`+"```"+`

## Package

`+"```"+`
soi-package --dir . --compiler rust
`+"```"+`

## Tools

- **hello** — Say hello
`, name, pluginType)
}

// sanitizeRustName converts a name to a valid Rust crate name
func sanitizeRustName(name string) string {
	// Replace hyphens with underscores
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

// ── helpers ──

func extractFunction(src, funcName string) (string, []string) {
	pattern := fmt.Sprintf(`func\s+%s\s*\([^)]*\)\s*(?:\([^)]*\))?\s*\{`, regexp.QuoteMeta(funcName))
	re := regexp.MustCompile(pattern)
	match := re.FindStringIndex(src)
	if match == nil {
		return "", nil
	}

	start := match[0]
	braceCount := 0
	end := start
	for i := start; i < len(src); i++ {
		if src[i] == '{' {
			braceCount++
		} else if src[i] == '}' {
			braceCount--
			if braceCount == 0 {
				end = i + 1
				break
			}
		}
	}

	funcBody := src[start:end]

	var imports []string
	importRe := regexp.MustCompile(`import\s*\(\s*([^)]+)\s*\)`)
	importMatch := importRe.FindStringSubmatch(src)
	if len(importMatch) > 1 {
		for _, line := range strings.Split(importMatch[1], "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.Contains(line, "soi.dev/soi-sdk") {
				imports = append(imports, line)
			}
		}
	}

	return funcBody, imports
}

func usage() {
	fmt.Fprintf(os.Stderr, `soi-create — Create SOI WASM plugin projects

USAGE:
  soi-create <subcommand> [flags]

SUBCOMMANDS:
  scaffold   Generate a new plugin project from scratch
  wrap       Wrap existing Go code into a SOI plugin

EXAMPLES:
  soi-create scaffold --name hello --type wasm
  soi-create scaffold --name hello --type soi --sandbox
  soi-create wrap --func Add --in add.go --out ./add-plugin
  soi-create wrap --func Multiply --in math.go --out ./math-plugin --type soi

Use "soi-create <subcommand> --help" for more information.
`)
}

const scaffoldUsage = `soi-create scaffold — Generate a new SOI WASM plugin project

USAGE:
  soi-create scaffold --name <name> [flags]

FLAGS:
  --name     Plugin name (required)
  --type     Plugin type: wasm | soi (default: wasm)
  --compiler Compiler: go | tinygo | rust (default: go)
  --output   Output directory (default: .)
  --sandbox  Include sandbox (SOI) tools

EXAMPLES:
  soi-create scaffold --name hello --type wasm
  soi-create scaffold --name hello --type soi --sandbox
  soi-create scaffold --name hello --compiler tinygo
  soi-create scaffold --name hello --compiler rust
  soi-create scaffold --name hello --output ./my-plugins
`

const wrapUsage = `soi-create wrap — Wrap existing Go code into a SOI WASM plugin

USAGE:
  soi-create wrap --func <name> --in <file> --out <dir> [flags]

FLAGS:
  --func     Function name to wrap (required)
  --in       Input Go file path (required)
  --out      Output directory (required)
  --type     Plugin type: wasm | soi (default: wasm)
  --compiler Compiler: go | tinygo (default: go)
  --sandbox  Enable sandbox (SOI) tools

EXAMPLES:
  soi-create wrap --func Add --in add.go --out ./add-plugin
  soi-create wrap --func Multiply --in math.go --out ./math-plugin --type soi
  soi-create wrap --func Reverse --in strings.go --out ./str-plugin --sandbox
  soi-create wrap --func Add --in add.go --out ./add-plugin --compiler tinygo
`

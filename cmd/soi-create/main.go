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

	dir := filepath.Join(*output, *name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Scaffolding SOI plugin: %s (type=%s)\n", *name, *pluginType)

	files := map[string]string{
		"go.mod":     genGoMod(*name),
		"skill.yaml": genSkillYAML(*name, *pluginType),
		"tools.go":   genToolsGo(*name, *pluginType, *withSandbox),
		"README.md":  genREADME(*name, *pluginType),
	}

	if *pluginType == "wasm" {
		files["main.go"] = genMainGo()
		files["main_test.go"] = genMainTestGo()
	} else {
		files["main.go"] = genMainGo()
		files["main_tinygo.go"] = genMainTinyGo()
		files["main_test.go"] = genMainTestGoSOI()
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
	fmt.Printf("  ║  go mod tidy                                     ║\n")
	if *pluginType == "soi" {
		fmt.Printf("  ║  tinygo build -target=wasi -o wasm/plugin.soi . ║\n")
	} else {
		fmt.Printf("  ║  GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm . ║\n")
	}
	fmt.Printf("  ║  go test -v                                      ║\n")
	fmt.Printf("  ╚══════════════════════════════════════════════════╝\n")
}

// ── wrap subcommand ──

func wrap(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	funcName := fs.String("func", "", "Function name to wrap (required)")
	inputFile := fs.String("in", "", "Input Go file path (required)")
	outputDir := fs.String("out", "", "Output directory (required)")
	pluginType := fs.String("type", "wasm", "Plugin type: wasm | soi")
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

	fmt.Printf("Wrapping function %q into SOI plugin (type=%s)\n", *funcName, *pluginType)

	files := map[string]string{
		"go.mod":     genGoMod(name),
		"skill.yaml": genSkillYAML(name, *pluginType),
		"tools.go":   genWrappedToolsGo(name, *funcName, funcBody, imports, *pluginType, *withSandbox),
		"README.md":  genREADME(name, *pluginType),
	}

	if *pluginType == "wasm" {
		files["main.go"] = genMainGo()
	} else {
		files["main.go"] = genMainGo()
		files["main_tinygo.go"] = genMainTinyGo()
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
	fmt.Printf("  ║  go mod tidy                                     ║\n")
	if *pluginType == "soi" {
		fmt.Printf("  ║  tinygo build -target=wasi -o wasm/plugin.soi . ║\n")
	} else {
		fmt.Printf("  ║  GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm . ║\n")
	}
	fmt.Printf("  ╚══════════════════════════════════════════════════╝\n")
}

// ── shared code generators ──

func genGoMod(name string) string {
	return fmt.Sprintf(`module soi.dev/%s

go 1.22.0

require soi.dev/soi-sdk v1.0.0

replace soi.dev/soi-sdk => ../soi-sdk
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

func genMainGo() string {
	return `//go:build !tinygo

package main

import sdk "soi.dev/soi-sdk"

func main() {
	sdk.Run()
}
`
}

func genMainTinyGo() string {
	return `//go:build tinygo

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

	// Parse request
	var req struct {
		Tool        string          ` + "`json:\"tool\"`" + `
		Args        json.RawMessage ` + "`json:\"args\"`" + `
		SandboxRoot string          ` + "`json:\"sandbox_root,omitempty\"`" + `
	}
	if err := json.Unmarshal(sdk.GetInputBuf(), &req); err != nil {
		sdk.SetResultBuf(jsonError(err.Error()))
		return sdk.PackResult(sdk.GetResultBuf())
	}

	resp := sdk.CallTool(req.Tool, req.Args, req.SandboxRoot, sdk.NewTinyGoHostAPI())
	if resp.Error != "" {
		sdk.SetResultBuf(jsonError(resp.Error))
	} else {
		sdk.SetResultBuf(resp.Output)
	}
	return sdk.PackResult(sdk.GetResultBuf())
}

func jsonError(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

func main() {
	registerTools()
}
`
}

func genToolsGo(name, pluginType string, withSandbox bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\n")
	b.WriteString("\tsdk \"soi.dev/soi-sdk\"\n")
	b.WriteString(")\n\n")

	b.WriteString("//export registerTools\n")
	b.WriteString("func registerTools() {\n")
	b.WriteString("\tsdk.RegisterToolWithDef(sdk.ToolDef{\n")
	b.WriteString("\t\tName:        \"hello\",\n")
	b.WriteString("\t\tDescription: \"Say hello\",\n")
	b.WriteString("\t\tParameters: []sdk.ParamDef{\n")
	b.WriteString("\t\t\t{Name: \"name\", Type: \"string\", Required: true, Description: \"Your name\"},\n")
	b.WriteString("\t\t},\n")
	b.WriteString("\t\tReturns: \"object with greeting message\",\n")
	b.WriteString("\t}, helloHandler)\n")

	if withSandbox || pluginType == "soi" {
		b.WriteString("\n")
		b.WriteString("\tsdk.RegisterSOITool(sdk.ToolDef{\n")
		b.WriteString("\t\tName:        \"sandbox_info\",\n")
		b.WriteString("\t\tDescription: \"Get sandbox information\",\n")
		b.WriteString("\t\tReturns:     \"object with sandbox info\",\n")
		b.WriteString("\t}, sandboxInfoHandler)\n")
	}

	b.WriteString("}\n\n")

	b.WriteString("func helloHandler(args json.RawMessage) (interface{}, error) {\n")
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

func genMainTestGo() string {
	return `package main

import (
	"encoding/json"
	"testing"

	"soi.dev/soi-sdk"
)

func TestHello(t *testing.T) {
	argsJSON, _ := json.Marshal(map[string]interface{}{"Name": "Alice"})
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

func genMainTestGoSOI() string {
	return `package main

import (
	"encoding/json"
	"testing"

	"soi.dev/soi-sdk"
	"soi.dev/soi-vos"
)

func TestHello(t *testing.T) {
	host := vos.NewMockHost(nil)
	argsJSON, _ := json.Marshal(map[string]interface{}{"Name": "Alice"})
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

func genREADME(name, pluginType string) string {
	var buildCmd string
	if pluginType == "soi" {
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
soi-package --dir .
`+"```"+`

## Tools

- **hello** — Say hello
`, name, pluginType, buildCmd)
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
  --output   Output directory (default: .)
  --sandbox  Include sandbox (SOI) tools

EXAMPLES:
  soi-create scaffold --name hello --type wasm
  soi-create scaffold --name hello --type soi --sandbox
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
  --sandbox  Enable sandbox (SOI) tools

EXAMPLES:
  soi-create wrap --func Add --in add.go --out ./add-plugin
  soi-create wrap --func Multiply --in math.go --out ./math-plugin --type soi
  soi-create wrap --func Reverse --in strings.go --out ./str-plugin --sandbox
`

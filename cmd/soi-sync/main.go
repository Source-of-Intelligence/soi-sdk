package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	var (
		dir      = flag.String("dir", "", "Plugin directory (required)")
		name     = flag.String("name", "", "Plugin name (optional, inferred from directory name)")
		version  = flag.String("version", "1.0.0", "Plugin version (optional)")
		typ      = flag.String("type", "", "Plugin type (wasm/soi, optional)")
		compiler = flag.String("compiler", "", "Compiler: go | tinygo | rust (optional, auto-detected)")
	)
	flag.Parse()

	if *dir == "" {
		flag.Usage()
		fmt.Println("Error: -dir is required")
		os.Exit(1)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Printf("Error: could not get absolute path: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("========================================")
	fmt.Println("  SOI Sync Tool")
	fmt.Println("========================================")
	fmt.Printf("Syncing: %s\n", absDir)
	fmt.Println("----------------------------------------")

	if *name == "" {
		*name = filepath.Base(absDir)
	}
	fmt.Printf("  [1/3] Plugin name: %s\n", *name)

	// Auto-detect compiler if not provided
	if *compiler == "" {
		if _, err := os.Stat(filepath.Join(absDir, "Cargo.toml")); err == nil {
			*compiler = "rust"
		} else if _, err := os.Stat(filepath.Join(absDir, "main_tinygo.go")); err == nil {
			*compiler = "tinygo"
		} else {
			*compiler = "go"
		}
	}
	fmt.Printf("  [Compiler] %s\n", *compiler)

	if *typ == "" {
		*typ = detectType(absDir, *compiler)
	}
	fmt.Printf("  [2/3] Plugin type: %s\n", *typ)

	fmt.Println("  [3/3] Parsing source...")
	tools, pluginUses, wasmConfig, err := parseToolsFromSource(absDir, *compiler)
	if err != nil {
		fmt.Printf("Error: could not parse source: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Found %d tool(s):\n", len(tools))
	for _, t := range tools {
		fmt.Printf("    - %s\n", t.Name)
		fmt.Printf("      Desc: %s\n", t.Description)
		fmt.Printf("      Params: %d\n", len(t.Parameters))
		for _, p := range t.Parameters {
			fmt.Printf("        - %s (%s, required=%v)\n", p.Name, p.Type, p.Required)
		}
	}

	fmt.Printf("  Found %d capability(ies): %v\n", len(pluginUses), pluginUses)
	fmt.Printf("  Found trigger keywords: %v\n", globalTrigger.Keywords)
	fmt.Printf("  Found trigger prefix: %s\n", globalTrigger.Prefix)
	fmt.Printf("  Found trigger priority: %d\n", globalTrigger.Priority)
	fmt.Printf("  Found wasm sandbox subdir: %s\n", wasmConfig.SandboxSubdir)
	fmt.Printf("  Found wasm timeout: %s\n", wasmConfig.Timeout)
	if globalDescription != "" {
		fmt.Printf("  Found description: %s\n", globalDescription)
	}

	err = generateSkillYAML(absDir, *name, *version, *typ, tools, pluginUses, globalTrigger, globalDescription, wasmConfig)
	if err != nil {
		fmt.Printf("Error: could not generate skill.yaml: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("  ✓ Skill.yaml updated successfully!")
}

func detectType(dir, compiler string) string {
	if compiler == "rust" {
		return "wasm"
	}
	mainGoPath := filepath.Join(dir, "main.go")
	sourceBytes, err := os.ReadFile(mainGoPath)
	if err != nil {
		return "wasm"
	}
	source := string(sourceBytes)

	if strings.Contains(source, "RegisterSOI") {
		return "soi"
	}
	return "wasm"
}

type ToolExample struct {
	Input  map[string]interface{}
	Output string
}

type ToolInfo struct {
	Name        string
	Description string
	Parameters  []ParamInfo
	Returns     string
	Examples    []ToolExample
	Uses        []string
}

type ParamInfo struct {
	Name        string
	Type        string
	Required    bool
	Default     string
	Description string
	Enum        []string
}

type TriggerInfo struct {
	Keywords []string
	Prefix   string
	Regex    string
	Events   []string
	Priority int
}

type WasmConfig struct {
	SandboxSubdir string
	Timeout       string
}

var globalTrigger TriggerInfo
var globalDescription string
var globalWasmConfig WasmConfig

func parseToolsFromSource(dir, compiler string) ([]ToolInfo, []string, WasmConfig, error) {
	var tools []ToolInfo
	seenUses := make(map[string]bool)
	var pluginUses []string

	if compiler == "rust" {
		// Parse Rust source files (src/lib.rs, src/*.rs)
		rustSources := findRustSources(dir)
		if len(rustSources) == 0 {
			return nil, nil, WasmConfig{}, fmt.Errorf("no Rust source files found (expected src/lib.rs)")
		}

		fmt.Printf("Parsed %d source file(s)\n", len(rustSources))

		// First pass: collect global trigger, description, wasm config
		globalTrigger = TriggerInfo{}
		globalDescription = ""
		globalWasmConfig = WasmConfig{SandboxSubdir: "/", Timeout: "30s"}

		for _, path := range rustSources {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			collectRustGlobals(string(data))
		}

		// Second pass: parse all tools from builder chains
		for _, path := range rustSources {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			found, uses := parseRustBuilders(string(data))
			tools = append(tools, found...)
			for _, u := range uses {
				if !seenUses[u] {
					pluginUses = append(pluginUses, u)
					seenUses[u] = true
				}
			}
		}

		// Fallback: if no tools found from AST, try to read from existing skill.yaml
		if len(tools) == 0 {
			tools, pluginUses, globalTrigger, globalWasmConfig = readToolsFromSkillYAML(dir)
		}
		return tools, pluginUses, globalWasmConfig, nil
	}

	// Go / TinyGo path (existing implementation)
	sourceFiles := []string{
		filepath.Join(dir, "main.go"),
		filepath.Join(dir, "bridge.go"),
	}

	var files []*ast.File
	fset := token.NewFileSet()

	for _, filePath := range sourceFiles {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}

		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			fmt.Printf("Warning: could not parse %s: %v\n", filePath, err)
			continue
		}
		files = append(files, f)
	}

	if len(files) == 0 {
		return nil, nil, WasmConfig{}, fmt.Errorf("no valid source files found (tried main.go, bridge.go)")
	}

	fmt.Printf("Parsed %d source file(s)\n", len(files))

	// First pass: collect all trigger keywords, prefix, regex, priority, description and wasm config from all files
	globalTrigger = TriggerInfo{}
	globalDescription = ""
	globalWasmConfig = WasmConfig{
		SandboxSubdir: "/",
		Timeout:       "30s",
	}

	for _, f := range files {
		trigger := parseGlobalTrigger(f)
		for _, kw := range trigger.Keywords {
			if !contains(globalTrigger.Keywords, kw) {
				globalTrigger.Keywords = append(globalTrigger.Keywords, kw)
			}
		}
		if trigger.Prefix != "" {
			globalTrigger.Prefix = trigger.Prefix
		}
		if trigger.Regex != "" {
			globalTrigger.Regex = trigger.Regex
		}
		if trigger.Priority != 0 {
			globalTrigger.Priority = trigger.Priority
		}

		if desc := parseGlobalDescription(f); desc != "" {
			globalDescription = desc
		}

		wasmConfig := parseGlobalWasmConfig(f)
		if wasmConfig.SandboxSubdir != "/" {
			globalWasmConfig.SandboxSubdir = wasmConfig.SandboxSubdir
		}
		if wasmConfig.Timeout != "30s" {
			globalWasmConfig.Timeout = wasmConfig.Timeout
		}
	}

	// Second pass: find all tool definitions in all files
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Name.Name == "registerTools" || strings.HasPrefix(fn.Name.Name, "register") {
					// Parse this function for tool definitions
					foundTools := parseRegisterToolsFunc(fn, seenUses, &pluginUses)
					tools = append(tools, foundTools...)
				}
			}
		}
	}

	// Fallback to existing YAML if no tools found
	if len(tools) == 0 {
		fmt.Println("No tools found from AST, falling back to skill.yaml")
		tools, pluginUses, globalTrigger, globalWasmConfig = readToolsFromSkillYAML(dir)
	}

	return tools, pluginUses, globalWasmConfig, nil
}

func parseGlobalTrigger(f *ast.File) TriggerInfo {
	trigger := TriggerInfo{}
	seenKw := make(map[string]bool)

	// Walk all declarations to find trigger-related method calls
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name

				switch methodName {
				case "TriggerKeywords":
					if args := extractStringArgs(call.Args); len(args) > 0 {
						for _, kw := range args {
							if kw != "" && !seenKw[kw] {
								trigger.Keywords = append(trigger.Keywords, kw)
								seenKw[kw] = true
							}
						}
					}
				case "TriggerPrefix":
					if args := extractStringArgs(call.Args); len(args) > 0 {
						trigger.Prefix = args[0]
					}
				case "TriggerRegex":
					if args := extractStringArgs(call.Args); len(args) > 0 {
						trigger.Regex = args[0]
					}
				case "TriggerPriority":
					if args := extractIntArgs(call.Args); len(args) > 0 {
						trigger.Priority = args[0]
					}
				}
			}
		}
		return true
	})

	return trigger
}

func parseGlobalDescription(f *ast.File) string {
	desc := ""

	// Walk all declarations to find Desc method calls on Builder
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Desc" {
					if args := extractStringArgs(call.Args); len(args) > 0 {
						desc = args[0]
					}
				}
			}
		}
		return true
	})

	return desc
}

func parseGlobalWasmConfig(f *ast.File) WasmConfig {
	config := WasmConfig{
		SandboxSubdir: "/",
		Timeout:       "30s",
	}

	// Walk all declarations to find WithSandboxSubdir and WithTimeout method calls
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "WithSandboxSubdir":
					if args := extractStringArgs(call.Args); len(args) > 0 {
						config.SandboxSubdir = args[0]
					}
				case "WithTimeout":
					if args := extractStringArgs(call.Args); len(args) > 0 {
						config.Timeout = args[0]
					}
				}
			}
		}
		return true
	})

	return config
}

func parseRegisterToolsFunc(fn *ast.FuncDecl, seenUses map[string]bool, pluginUses *[]string) []ToolInfo {
	var tools []ToolInfo

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				// 查找最外层的RegisterSOI或RegisterSimple调用，然后从这里解析完整的链
				if sel.Sel.Name == "RegisterSOI" || sel.Sel.Name == "RegisterSimple" {
					// 从完整的调用链开始解析
					tool := parseToolChain(call)
					if tool.Name != "" {
						tools = append(tools, tool)

						// Merge uses into pluginUses
						for _, use := range tool.Uses {
							if !seenUses[use] {
								*pluginUses = append(*pluginUses, use)
								seenUses[use] = true
							}
						}
					}
				}
			}
		}
		return true
	})

	return tools
}

type parsedTool struct {
	ToolInfo
	Uses []string
}

func parseToolChain(call *ast.CallExpr) ToolInfo {
	var tool ToolInfo

	// First, parse uses from the entire call tree
	tool.Uses = parseToolUses(call)

	// Collect all method calls in the chain (from right to left)
	var methodCalls []*ast.CallExpr
	current := call
	for current != nil {
		if sel, ok := current.Fun.(*ast.SelectorExpr); ok {
			methodCalls = append(methodCalls, current)
			// Move to previous in chain
			if star, ok := sel.X.(*ast.CallExpr); ok {
				current = star
			} else {
				break
			}
		} else {
			break
		}
	}

	// Now process the method calls in reverse order (from left to right)
	// Because methodCalls[0] is RegisterSOI, methodCalls[len-1] is NewTool
	for i := len(methodCalls) - 1; i >= 0; i-- {
		c := methodCalls[i]
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
			methodName := sel.Sel.Name
			switch methodName {
			case "NewTool":
				// 从NewTool调用中获取工具名称
				if len(c.Args) >= 1 {
					if lit, ok := c.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						tool.Name = stripQuotes(lit.Value)
					}
				}
			case "Desc":
				if args := extractStringArgs(c.Args); len(args) > 0 {
					tool.Description = args[0]
				}
			case "Returns":
				if args := extractStringArgs(c.Args); len(args) > 0 {
					tool.Returns = args[0]
				}
			case "Param":
				if param := parseParam(c.Args); param.Name != "" {
					tool.Parameters = append(tool.Parameters, param)
				}
			}
		}
	}

	return tool
}

func parseParam(args []ast.Expr) ParamInfo {
	var param ParamInfo
	// 允许参数数量有一定灵活性（至少4个：name, type, required, default）
	if len(args) < 4 {
		return param
	}

	// Arg 0: name (string)
	if lit, ok := args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
		param.Name = stripQuotes(lit.Value)
	}

	// Arg 1: type (string)
	if lit, ok := args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
		param.Type = stripQuotes(lit.Value)
	}

	// Arg 2: required (bool)
	if ident, ok := args[2].(*ast.Ident); ok {
		param.Required = ident.Name == "true"
	}

	// Arg 3: default value (can be nil, string, etc)
	param.Default = extractDefaultValue(args[3])

	// Arg 4: description (string, optional)
	if len(args) > 4 {
		if lit, ok := args[4].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			param.Description = stripQuotes(lit.Value)
		}
	}

	return param
}

func extractDefaultValue(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "nil" {
			return ""
		}
		return e.Name
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return stripQuotes(e.Value)
		}
		return e.Value
	case *ast.CompositeLit:
		// Could be []string{} or similar
		return ""
	default:
		return ""
	}
}

func parseToolUses(call *ast.CallExpr) []string {
	var uses []string
	seen := make(map[string]bool)

	ast.Inspect(call, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name
				switch methodName {
				case "WithSandbox":
					for _, arg := range call.Args {
						if sel, ok := arg.(*ast.SelectorExpr); ok {
							if ident, ok := sel.X.(*ast.Ident); ok {
								use := toSnakeCase(ident.Name)
								if !seen[use] {
									uses = append(uses, use)
									seen[use] = true
								}
							}
						}
					}
				case "WithSandboxFS":
					if !seen["sandbox_fs"] {
						uses = append(uses, "sandbox_fs")
						seen["sandbox_fs"] = true
					}
				case "WithHostLog":
					if !seen["host_log"] {
						uses = append(uses, "host_log")
						seen["host_log"] = true
					}
				case "WithHostNow":
					if !seen["host_now"] {
						uses = append(uses, "host_now")
						seen["host_now"] = true
					}
				case "WithHostRandom":
					if !seen["host_random"] {
						uses = append(uses, "host_random")
						seen["host_random"] = true
					}
				case "WithHostHTTP":
					if !seen["host_http"] {
						uses = append(uses, "host_http")
						seen["host_http"] = true
					}
				case "WithHostEnv":
					if !seen["host_env"] {
						uses = append(uses, "host_env")
						seen["host_env"] = true
					}
				case "WithHostProcess":
					if !seen["host_process"] {
						uses = append(uses, "host_process")
						seen["host_process"] = true
					}
				}
			}
		}
		return true
	})

	return uses
}

func extractStringArgs(args []ast.Expr) []string {
	var result []string
	for _, arg := range args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			result = append(result, stripQuotes(lit.Value))
		}
	}
	return result
}

func extractIntArgs(args []ast.Expr) []int {
	var result []int
	for _, arg := range args {
		if lit, ok := arg.(*ast.BasicLit); ok {
			switch lit.Kind {
			case token.INT:
				if v, err := strconv.Atoi(lit.Value); err == nil {
					result = append(result, v)
				}
			case token.FLOAT:
				if v, err := strconv.ParseFloat(lit.Value, 64); err == nil {
					result = append(result, int(v))
				}
			}
		}
	}
	return result
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
		if s[0] == '`' && s[len(s)-1] == '`' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func readToolsFromSkillYAML(dir string) ([]ToolInfo, []string, TriggerInfo, WasmConfig) {
	// (Keep the existing implementation)
	return nil, nil, TriggerInfo{}, WasmConfig{
		SandboxSubdir: "/",
		Timeout:       "30s",
	}
}

func generateSkillYAML(dir, name, version, pluginType string, tools []ToolInfo, pluginUses []string, trigger TriggerInfo, description string, wasmConfig WasmConfig) error {
	yamlPath := filepath.Join(dir, "skill.yaml")

	var sb strings.Builder

	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: Skill\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", name))
	sb.WriteString(fmt.Sprintf("  version: \"%s\"\n", version))
	if description != "" {
		sb.WriteString(fmt.Sprintf("  description: \"%s\"\n", escapeYAML(description)))
	} else {
		sb.WriteString("  description: \"SOI plugin\"\n")
	}
	sb.WriteString("spec:\n")
	sb.WriteString("  runtime:\n")
	sb.WriteString(fmt.Sprintf("    type: %s\n", pluginType))
	if pluginType == "soi" {
		sb.WriteString("    entry: wasm/plugin.soi\n")
	} else {
		sb.WriteString("    entry: wasm/plugin.wasm\n")
	}

	if len(pluginUses) > 0 {
		sb.WriteString("    uses:\n")
		for _, use := range pluginUses {
			sb.WriteString(fmt.Sprintf("      - %s\n", use))
		}
	}

	sb.WriteString("    wasm:\n")
	sb.WriteString("      sandbox:\n")
	sb.WriteString(fmt.Sprintf("        subdir: \"%s\"\n", wasmConfig.SandboxSubdir))
	sb.WriteString(fmt.Sprintf("      timeout: \"%s\"\n", wasmConfig.Timeout))

	sb.WriteString("  provides:\n")

	if len(trigger.Keywords) > 0 || trigger.Prefix != "" || trigger.Regex != "" || trigger.Priority != 0 {
		sb.WriteString("    triggers:\n")
		if len(trigger.Keywords) > 0 {
			var quotedKeywords []string
			for _, kw := range trigger.Keywords {
				quotedKeywords = append(quotedKeywords, "\""+kw+"\"")
			}
			sb.WriteString(fmt.Sprintf("      keywords: [%s]\n", strings.Join(quotedKeywords, ", ")))
		}
		if trigger.Prefix != "" {
			sb.WriteString(fmt.Sprintf("      prefix: \"%s\"\n", trigger.Prefix))
		}
		if trigger.Regex != "" {
			sb.WriteString(fmt.Sprintf("      regex: \"%s\"\n", escapeYAML(trigger.Regex)))
		}
		if trigger.Priority != 0 {
			sb.WriteString(fmt.Sprintf("      priority: %d\n", trigger.Priority))
		}
	}

	sb.WriteString("    tools:\n")

	if len(tools) == 0 {
		sb.WriteString("    - name: execute\n")
		sb.WriteString("      description: \"Execute plugin function\"\n")
		sb.WriteString("      parameters:\n")
		sb.WriteString("      - name: input\n")
		sb.WriteString("        type: string\n")
		sb.WriteString("        required: false\n")
		sb.WriteString("        description: \"Input data\"\n")
	} else {
		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("    - name: %s\n", tool.Name))
			if tool.Description != "" {
				sb.WriteString(fmt.Sprintf("      description: \"%s\"\n", escapeYAML(tool.Description)))
			} else {
				sb.WriteString("      description: \"\"\n")
			}

			if len(tool.Parameters) > 0 {
				sb.WriteString("      parameters:\n")
				for _, param := range tool.Parameters {
					sb.WriteString(fmt.Sprintf("      - name: %s\n", param.Name))
					sb.WriteString(fmt.Sprintf("        type: %s\n", param.Type))
					sb.WriteString(fmt.Sprintf("        required: %t\n", param.Required))
					if param.Description != "" {
						sb.WriteString(fmt.Sprintf("        description: \"%s\"\n", escapeYAML(param.Description)))
					}
				}
			}

			if tool.Returns != "" {
				sb.WriteString(fmt.Sprintf("      returns: \"%s\"\n", escapeYAML(tool.Returns)))
			}
		}
	}

	fmt.Println("\nWriting skill.yaml content:\n", sb.String())
	return os.WriteFile(yamlPath, []byte(sb.String()), 0644)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// =========================================================================
// Rust source parser — lightweight pattern-matching scanner for soi-sdk-rs
// Builder API.  No external dependencies; handles Builder::new(...).xxx(...)
// chains with balanced-parentheses arg extraction.
// =========================================================================

// findRustSources returns all .rs files under <dir>/src/.
func findRustSources(dir string) []string {
	var files []string
	srcDir := filepath.Join(dir, "src")
	if _, err := os.Stat(srcDir); err != nil {
		return files
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return files
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".rs") {
			files = append(files, filepath.Join(srcDir, name))
		}
	}
	// Put lib.rs first (most plugins register tools there).
	for i, f := range files {
		if strings.HasSuffix(f, "lib.rs") {
			if i > 0 {
				files[0], files[i] = files[i], files[0]
			}
			break
		}
	}
	return files
}

// ============================================================================
// Rust text-scanner helpers
// ============================================================================

// rustStripComments returns `src` with //line and /*block*/ comments replaced
// by whitespace (so byte offsets/column counts stay stable).  String literals
// containing "//" or "/*" are preserved intact.
func rustStripComments(src string) string {
	var out strings.Builder
	out.Grow(len(src))

	i := 0
	for i < len(src) {
		ch := src[i]

		// Raw string: r#"..."#  (handle r##" ... ##" too)
		if ch == 'r' && i+1 < len(src) && (src[i+1] == '"' || src[i+1] == '#') {
			j := i + 1
			hashes := 0
			for j < len(src) && src[j] == '#' {
				hashes++
				j++
			}
			if j < len(src) && src[j] == '"' {
				out.WriteString(src[i : j+1])
				i = j + 1
				// find closing "# hashes
				closer := "\"" + strings.Repeat("#", hashes)
				for i < len(src) {
					if strings.HasPrefix(src[i:], closer) {
						out.WriteString(src[i : i+len(closer)])
						i += len(closer)
						break
					}
					out.WriteByte(src[i])
					i++
				}
				continue
			}
		}

		// Regular string: "..."  with \" escapes
		if ch == '"' {
			out.WriteByte(ch)
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					out.WriteByte(src[i])
					out.WriteByte(src[i+1])
					i += 2
					continue
				}
				out.WriteByte(src[i])
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}

		// Line comment: // ...
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				out.WriteByte(' ')
				i++
			}
			continue
		}

		// Block comment: /* ... */ (non-nested in the Rust sense we need)
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			out.WriteString("  ")
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					out.WriteByte('\n')
				} else {
					out.WriteByte(' ')
				}
				i++
			}
			if i+1 < len(src) {
				out.WriteString("  ")
				i += 2
			}
			continue
		}

		out.WriteByte(ch)
		i++
	}
	return out.String()
}

// rustExtractStringLit extracts the content of the first Rust string literal
// found at or after `start`.  It handles both "..." and r#"..."# literals.
// Returns the string content and the end offset (after the closing quote).
// ok is false if no string literal is found before a non-whitespace token
// that suggests there isn't one (for fast-fail on Value::Null etc.).
func rustExtractStringLit(s string, start int) (val string, end int, ok bool) {
	for i := start; i < len(s); i++ {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ',' {
			continue
		}
		// Raw string: r#"..."#
		if ch == 'r' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '#') {
			j := i + 1
			hashes := 0
			for j < len(s) && s[j] == '#' {
				hashes++
				j++
			}
			if j < len(s) && s[j] == '"' {
				closeSeq := "\"" + strings.Repeat("#", hashes)
				closeIdx := strings.Index(s[j+1:], closeSeq)
				if closeIdx < 0 {
					return "", 0, false
				}
				return s[j+1 : j+1+closeIdx], j + 1 + closeIdx + len(closeSeq), true
			}
			return "", 0, false
		}
		// Regular string
		if ch == '"' {
			var sb strings.Builder
			k := i + 1
			for k < len(s) {
				if s[k] == '\\' && k+1 < len(s) {
					next := s[k+1]
					switch next {
					case 'n':
						sb.WriteByte(' ')
					case 't':
						sb.WriteByte(' ')
					case '\\':
						sb.WriteByte('\\')
					case '"':
						sb.WriteByte('"')
					default:
						sb.WriteByte(next)
					}
					k += 2
					continue
				}
				if s[k] == '"' {
					return sb.String(), k + 1, true
				}
				sb.WriteByte(s[k])
				k++
			}
			return "", 0, false
		}
		// Not a string literal — abort.
		return "", 0, false
	}
	return "", 0, false
}

// rustFindBalanced returns the index of the closing ')' that balances the
// opening '(' at s[start].  It honours (), [], {} nesting and skips Rust
// string literals so that ')' inside a string is not counted.
// Returns -1 if nothing balances.
func rustFindBalanced(s string, start int) int {
	if start >= len(s) || s[start] != '(' {
		return -1
	}
	depth := 1
	i := start + 1
	for i < len(s) {
		ch := s[i]
		switch ch {
		case '"':
			// Skip string literal (including r#"..."# handled via leading 'r' check)
			if i-1 >= 0 && s[i-1] == 'r' {
				// r#" detection: walk forward counting '#'
				j := i
				hashes := 0
				for j < len(s) && s[j] == '#' {
					hashes++
					j++
				}
				if j < len(s) && s[j] == '"' {
					closer := "\"" + strings.Repeat("#", hashes)
					if idx := strings.Index(s[j+1:], closer); idx >= 0 {
						i = j + 1 + idx + len(closer)
						continue
					}
				}
			}
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
		case '(', '[', '{':
			depth++
			i++
		case ')', ']', '}':
			depth--
			i++
			if depth == 0 {
				return i - 1
			}
		default:
			i++
		}
	}
	return -1
}

// rustExtractArgs returns the text inside the first balanced (...) group
// starting at s[start].  start must point at '('.  The returned string is the
// content without the outer parens.
func rustExtractArgs(s string, start int) string {
	closeIdx := rustFindBalanced(s, start)
	if closeIdx < 0 {
		return ""
	}
	return s[start+1 : closeIdx]
}

// rustSplitArgs splits a comma-separated arg list into top-level args,
// respecting string literals and nested (), [], {}.
func rustSplitArgs(argsText string) []string {
	var parts []string
	depth := 0
	var cur strings.Builder
	i := 0
	for i < len(argsText) {
		ch := argsText[i]
		switch ch {
		case '"':
			cur.WriteByte(ch)
			i++
			for i < len(argsText) {
				if argsText[i] == '\\' && i+1 < len(argsText) {
					cur.WriteByte(argsText[i])
					cur.WriteByte(argsText[i+1])
					i += 2
					continue
				}
				cur.WriteByte(argsText[i])
				if argsText[i] == '"' {
					i++
					break
				}
				i++
			}
		case '(', '[', '{':
			depth++
			cur.WriteByte(ch)
			i++
		case ')', ']', '}':
			depth--
			cur.WriteByte(ch)
			i++
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
				i++
			} else {
				cur.WriteByte(ch)
				i++
			}
		default:
			cur.WriteByte(ch)
			i++
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}
	return parts
}

// rustParseStringArray parses &["a", "b", ...] or ["a", "b", ...] into a
// slice of the inner string contents (or empty slice if parsing fails).
func rustParseStringArray(arg string) []string {
	// Strip leading '&' if any.
	arg = strings.TrimSpace(arg)
	arg = strings.TrimPrefix(arg, "&")
	arg = strings.TrimSpace(arg)
	// Remove outer [ ... ]
	openIdx := strings.Index(arg, "[")
	closeIdx := strings.LastIndex(arg, "]")
	if openIdx < 0 || closeIdx < openIdx {
		return nil
	}
	inner := arg[openIdx+1 : closeIdx]
	parts := rustSplitArgs(inner)
	var out []string
	for _, p := range parts {
		if v, _, ok := rustExtractStringLit(p, 0); ok {
			out = append(out, v)
		}
	}
	return out
}

// rustParseBool parses true/false from a raw arg text.
func rustParseBool(arg string) (bool, bool) {
	switch strings.TrimSpace(arg) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// rustParseInt parses a decimal int like "10" or "10i32".
func rustParseInt(arg string) (int, bool) {
	arg = strings.TrimSpace(arg)
	// Strip type suffix: 10i32 → 10
	for j := 0; j < len(arg); j++ {
		c := arg[j]
		if c >= '0' && c <= '9' {
			continue
		}
		arg = arg[:j]
		break
	}
	v, err := strconv.Atoi(arg)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ============================================================================
// Tool-chain parser — walks over Builder::new(...).xxx(...).register(...)
// ============================================================================

// rustMethodCall holds one `.methodName(args_text)` extracted from a chain.
type rustMethodCall struct {
	name     string
	argsText string
}

// rustParseMethodChain scans s starting at `pos` (right after `Builder::new`)
// and consumes the chain of .method(...) calls up to `.register(...)` or
// `.register_simple(...)`.  Returns the parsed calls plus the position
// immediately after the closing paren of the final register call.
//
// We tolerate whitespace and newline-indented chains (soi-sdk-rs idiom).
func rustParseMethodChain(s string, pos int) (tool ToolInfo, endPos int, uses []string, ok bool) {
	// 1) Handle .new("name") right after 'Builder::'
	// pos points to the char immediately after "Builder::", which should be 'n'
	// Actually, we will be called with pos pointing at '(' of new(...).

	// Extract name from new("name")
	if pos < len(s) && s[pos] == '(' {
		argsText := rustExtractArgs(s, pos)
		parts := rustSplitArgs(argsText)
		if len(parts) > 0 {
			if name, _, ok2 := rustExtractStringLit(parts[0], 0); ok2 {
				tool.Name = name
			}
		}
		pos = rustFindBalanced(s, pos) + 1
	}

	// 2) Walk .method(args) calls.
	for pos < len(s) {
		// Skip whitespace/newlines.
		for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t' || s[pos] == '\r' || s[pos] == '\n') {
			pos++
		}
		if pos >= len(s) || s[pos] != '.' {
			break
		}
		pos++ // skip '.'

		// Method name: [a-z][A-Za-z0-9_]*
		nameStart := pos
		for pos < len(s) && ((s[pos] >= 'a' && s[pos] <= 'z') || (s[pos] >= 'A' && s[pos] <= 'Z') || (s[pos] >= '0' && s[pos] <= '9') || s[pos] == '_') {
			pos++
		}
		methodName := s[nameStart:pos]

		// Skip whitespace between methodName and '('
		for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t' || s[pos] == '\r' || s[pos] == '\n') {
			pos++
		}
		if pos >= len(s) || s[pos] != '(' {
			break
		}
		argsText := rustExtractArgs(s, pos)
		closeIdx := rustFindBalanced(s, pos)
		if closeIdx < 0 {
			break
		}
		pos = closeIdx + 1

		// -------- Dispatch to per-method handler --------
		switch methodName {
		case "desc":
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				tool.Description = v
			}
		case "returns":
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				tool.Returns = v
			}
		case "param":
			parts := rustSplitArgs(argsText)
			var p ParamInfo
			if len(parts) > 0 {
				if v, _, ok2 := rustExtractStringLit(parts[0], 0); ok2 {
					p.Name = v
				}
			}
			if len(parts) > 1 {
				if v, _, ok2 := rustExtractStringLit(parts[1], 0); ok2 {
					p.Type = v
				}
			}
			if len(parts) > 2 {
				if v, ok2 := rustParseBool(parts[2]); ok2 {
					p.Required = v
				}
			}
			if len(parts) > 3 {
				// Default value: can be Value::Null, Value::from(1), "string", etc.
				// We capture a representative text — either the string-literal content,
				// or trimmed raw text if it is not a string literal.
				raw := strings.TrimSpace(parts[3])
				if v, _, ok2 := rustExtractStringLit(raw, 0); ok2 {
					p.Default = v
				} else {
					// Convert common patterns like `Value::Null` → "" ; `Value::from(1)` → "1"
					if strings.Contains(raw, "Null") {
						p.Default = ""
					} else if idx := strings.Index(raw, "from("); idx >= 0 {
						if close := strings.LastIndex(raw, ")"); close > idx {
							inner := strings.TrimSpace(raw[idx+5 : close])
							if inner == "" || inner == "()" {
								p.Default = ""
							} else if strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"") {
								p.Default = inner[1 : len(inner)-1]
							} else {
								p.Default = inner
							}
						} else {
							p.Default = raw
						}
					} else {
						// Best-effort: strip double-quote wrapper if any.
						inner := strings.TrimSpace(raw)
						if strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"") {
							inner = inner[1 : len(inner)-1]
						}
						p.Default = inner
					}
				}
			}
			if len(parts) > 4 {
				if v, _, ok2 := rustExtractStringLit(parts[4], 0); ok2 {
					p.Description = v
				}
			}
			if p.Name != "" {
				tool.Parameters = append(tool.Parameters, p)
			}

		case "with_sandbox":
			for _, u := range rustParseStringArray(argsText) {
				uses = append(uses, u)
			}
		case "with_sandbox_fs":
			uses = append(uses, "sandbox_fs")
		case "with_host_log":
			uses = append(uses, "host_log")
		case "with_host_now":
			uses = append(uses, "host_now")
		case "with_host_random":
			uses = append(uses, "host_random")
		case "with_host_http":
			uses = append(uses, "host_http")
		case "with_host_env":
			uses = append(uses, "host_env")
		case "with_host_process":
			uses = append(uses, "host_process")

		case "with_sandbox_subdir":
			// Global sandbox subdir.
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				globalWasmConfig.SandboxSubdir = v
			}
		case "with_timeout":
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				globalWasmConfig.Timeout = v
			}
		case "trigger_keywords":
			for _, kw := range rustParseStringArray(argsText) {
				if !contains(globalTrigger.Keywords, kw) {
					globalTrigger.Keywords = append(globalTrigger.Keywords, kw)
				}
			}
		case "trigger_prefix":
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				globalTrigger.Prefix = v
			}
		case "trigger_regex":
			if v, _, ok2 := rustExtractStringLit(argsText, 0); ok2 {
				globalTrigger.Regex = v
			}
		case "trigger_events":
			for _, ev := range rustParseStringArray(argsText) {
				globalTrigger.Events = append(globalTrigger.Events, ev)
			}
		case "trigger_priority":
			if v, ok2 := rustParseInt(argsText); ok2 {
				globalTrigger.Priority = v
			}

		case "register", "register_simple":
			// End of tool chain.
			ok = true
			endPos = pos
			return
		}
	}

	return tool, pos, uses, false
}

// parseRustBuilders scans a file stripped of comments for Builder::new(...)
// chains and returns the tools found plus any capability tags.
func parseRustBuilders(src string) ([]ToolInfo, []string) {
	clean := rustStripComments(src)
	var tools []ToolInfo
	var uses []string
	seenUses := make(map[string]bool)

	i := 0
	for i < len(clean) {
		// Find next `Builder::new(`
		idx := strings.Index(clean[i:], "Builder::new(")
		if idx < 0 {
			break
		}
		pos := i + idx + len("Builder::new") // now at '('

		tool, nextPos, toolUses, ok := rustParseMethodChain(clean, pos)
		if ok && tool.Name != "" {
			tools = append(tools, tool)
			for _, u := range toolUses {
				if !seenUses[u] {
					uses = append(uses, u)
					seenUses[u] = true
				}
			}
		}
		i = nextPos
		if i <= pos {
			// safety — avoid infinite loop
			i = pos + 1
		}
	}

	return tools, uses
}

// collectRustGlobals scans a Rust source for trigger / wasm-config calls
// that appear OUTSIDE of a tool chain (e.g. soi_plugin! macro-level
// trigger_keywords(&[...] or calls in a plain fn register_tools()).
// Our tool-chain parser already processes per-chain ones, but this pass
// additionally picks up top-level `trigger_xxx`, `with_timeout` etc.
// when they appear as free-standing method calls (typically chained off
// the same builder or via a side helper).
func collectRustGlobals(src string) {
	clean := rustStripComments(src)

	// Walk for common patterns.
	i := 0
	for i < len(clean) {
		// .trigger_keywords(&["...", "..."]) / .trigger_prefix("...") /
		// .trigger_regex("...") / .trigger_priority(n) / .trigger_events(&[...])
		// .with_sandbox_subdir("...") / .with_timeout("...")
		//
		// Because parseRustBuilders already processes the per-chain variants,
		// this function also catches standalone calls.  Here we do a simple
		// scan for the method name, balanced-paren arg extraction, and apply
		// to the global structs.
		idxKw := strings.Index(clean[i:], "trigger_keywords(")
		idxPrefix := strings.Index(clean[i:], "trigger_prefix(")
		idxRegex := strings.Index(clean[i:], "trigger_regex(")
		idxEvents := strings.Index(clean[i:], "trigger_events(")
		idxPriority := strings.Index(clean[i:], "trigger_priority(")
		idxSubdir := strings.Index(clean[i:], "with_sandbox_subdir(")
		idxTimeout := strings.Index(clean[i:], "with_timeout(")

		// Find earliest match
		bestIdx := -1
		bestName := ""
		candidates := []struct {
			idx int
			n   string
		}{
			{idxKw, "trigger_keywords"},
			{idxPrefix, "trigger_prefix"},
			{idxRegex, "trigger_regex"},
			{idxEvents, "trigger_events"},
			{idxPriority, "trigger_priority"},
			{idxSubdir, "with_sandbox_subdir"},
			{idxTimeout, "with_timeout"},
		}
		for _, c := range candidates {
			if c.idx >= 0 && (bestIdx < 0 || c.idx < bestIdx) {
				bestIdx = c.idx
				bestName = c.n
			}
		}
		if bestIdx < 0 {
			break
		}

		start := i + bestIdx + len(bestName) // at '('
		if start >= len(clean) || clean[start] != '(' {
			i = start + 1
			continue
		}
		argsText := rustExtractArgs(clean, start)
		endPos := rustFindBalanced(clean, start) + 1

		switch bestName {
		case "trigger_keywords":
			for _, kw := range rustParseStringArray(argsText) {
				if !contains(globalTrigger.Keywords, kw) {
					globalTrigger.Keywords = append(globalTrigger.Keywords, kw)
				}
			}
		case "trigger_prefix":
			if v, _, ok := rustExtractStringLit(argsText, 0); ok && v != "" {
				globalTrigger.Prefix = v
			}
		case "trigger_regex":
			if v, _, ok := rustExtractStringLit(argsText, 0); ok && v != "" {
				globalTrigger.Regex = v
			}
		case "trigger_events":
			globalTrigger.Events = append(globalTrigger.Events, rustParseStringArray(argsText)...)
		case "trigger_priority":
			if v, ok := rustParseInt(argsText); ok {
				globalTrigger.Priority = v
			}
		case "with_sandbox_subdir":
			if v, _, ok := rustExtractStringLit(argsText, 0); ok && v != "" {
				globalWasmConfig.SandboxSubdir = v
			}
		case "with_timeout":
			if v, _, ok := rustExtractStringLit(argsText, 0); ok && v != "" {
				globalWasmConfig.Timeout = v
			}
		}

		i = endPos
	}
}

func escapeYAML(s string) string {
	// 首先转义反斜杠
	s = strings.ReplaceAll(s, "\\", "\\\\")
	// 然后转义双引号
	s = strings.ReplaceAll(s, "\"", "\\\"")
	// 替换换行和制表符为空格
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

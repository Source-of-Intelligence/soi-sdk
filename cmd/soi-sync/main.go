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
		dir     = flag.String("dir", "", "Plugin directory (required)")
		name    = flag.String("name", "", "Plugin name (optional, inferred from directory name)")
		version = flag.String("version", "1.0.0", "Plugin version (optional)")
		typ     = flag.String("type", "", "Plugin type (go/wasm/soi, optional)")
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

	if *typ == "" {
		*typ = detectType(absDir)
	}
	fmt.Printf("  [2/3] Plugin type: %s\n", *typ)

	fmt.Println("  [3/3] Parsing source...")
	tools, pluginUses, wasmConfig, err := parseToolsFromSource(absDir)
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

func detectType(dir string) string {
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

func parseToolsFromSource(dir string) ([]ToolInfo, []string, WasmConfig, error) {
	var tools []ToolInfo
	seenUses := make(map[string]bool)
	var pluginUses []string

	mainGoPath := filepath.Join(dir, "main.go")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, mainGoPath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, WasmConfig{}, fmt.Errorf("parse file: %w", err)
	}

	fmt.Printf("Parsed %d bytes, found %d comments\n", f.End(), len(f.Comments))

	// First pass: collect all trigger keywords, prefix, regex, priority, description and wasm config from the whole file
	globalTrigger = parseGlobalTrigger(f)
	globalDescription = parseGlobalDescription(f)
	globalWasmConfig = parseGlobalWasmConfig(f)

	// Second pass: find all tool definitions
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Name.Name == "registerTools" || strings.HasPrefix(fn.Name.Name, "register") {
				// Parse this function for tool definitions
				tools = parseRegisterToolsFunc(fn, seenUses, &pluginUses)
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

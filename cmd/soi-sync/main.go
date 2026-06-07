// soi-sync — Synchronize plugin tool definitions to skill.yaml.
//
// Extracts tool definitions from plugin source code and updates
// skill.yaml automatically. Eliminates manual synchronization.
//
// Usage:
//
//	go run ./cmd/soi-sync --dir ./my-plugin
//	go run ./cmd/soi-sync --dir ./my-plugin --name my-plugin-name
//	go run ./cmd/soi-sync --dir ./my-plugin --version 1.0.1
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
	dir := flag.String("dir", "", "Plugin project directory (required)")
	name := flag.String("name", "", "Plugin name (auto-detected if not set)")
	version := flag.String("version", "", "Plugin version (auto-detected if not set)")
	flag.Usage = usage
	flag.Parse()

	if *dir == "" {
		usage()
		os.Exit(1)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: resolve dir: %v\n", err)
		os.Exit(1)
	}

	// Get plugin name
	pluginName := *name
	if pluginName == "" {
		pluginName = filepath.Base(absDir)
	}

	// Read version from existing skill.yaml or manifest
	pluginVersion := *version
	if pluginVersion == "" {
		pluginVersion = readVersion(absDir)
	}

	fmt.Println()
	fmt.Printf("  SOI Sync Tool\n")
	fmt.Printf("  Source:  %s\n", absDir)
	fmt.Printf("  Plugin:  %s\n", pluginName)
	fmt.Printf("  Version: %s\n", pluginVersion)
	fmt.Println()

	// Step 1: Parse plugin source code for tool definitions
	fmt.Println("  [1/3] Parsing plugin source...")
	tools, err := parseToolsFromSource(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse source: %v\n", err)
		os.Exit(1)
	}
	if len(tools) == 0 {
		fmt.Println("    ⚠  No tools found")
	} else {
		fmt.Printf("    ✓  Found %d tool(s)\n", len(tools))
		for _, t := range tools {
			fmt.Printf("       - %s\n", t.Name)
		}
	}

	// Step 2: Detect plugin type
	fmt.Println("  [2/3] Detecting plugin type...")
	pluginType := detectType(absDir)
	fmt.Printf("    ✓  %s\n", strings.ToUpper(pluginType))

	// Step 3: Generate/update skill.yaml
	fmt.Println("  [3/3] Generating skill.yaml...")
	err = generateSkillYAML(absDir, pluginName, pluginVersion, pluginType, tools)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: generate skill.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("    ✓  skill.yaml updated")

	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════════╗\n")
	fmt.Printf("  ║  Sync complete!                   ║\n")
	fmt.Printf("  ╚══════════════════════════════════╝\n")
	fmt.Println()
}

type ToolInfo struct {
	Name        string
	Description string
	Parameters  []ParamInfo
	Returns     string
	Trigger     *TriggerInfo
	Uses        []string
}

type ParamInfo struct {
	Name        string
	Type        string
	Required    bool
	Default     string
	Description string
}

type TriggerInfo struct {
	Keywords   []string
	Prefix     string
	Regex      string
	Events     []string
	Conditions map[string]string
	Priority   int
}

func parseToolsFromSource(dir string) ([]ToolInfo, error) {
	var tools []ToolInfo

	mainGoPath := filepath.Join(dir, "main.go")
	data, err := os.ReadFile(mainGoPath)
	if err != nil {
		return nil, fmt.Errorf("read main.go: %w", err)
	}
	source := string(data)

	// 1. Find all NewTool calls
	newToolRe := regexp.MustCompile(`NewTool\s*\(\s*"([^"]+)"\s*\)`)
	descRe := regexp.MustCompile(`\.Desc\s*\(\s*"([^"]+)"\s*\)`)
	returnsRe := regexp.MustCompile(`\.Returns\s*\(\s*"([^"]+)"\s*\)`)
	paramRe := regexp.MustCompile(`\.Param\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,\s*(true|false)\s*,\s*([^,]+)\s*,\s*"([^"]+)"\s*\)`)

	// Split source into sections for each NewTool
	sections := splitByNewTool(source)

	for _, section := range sections {
		if !strings.Contains(section, ".NewTool") {
			continue
		}

		var tool ToolInfo

		// Extract name
		if m := newToolRe.FindStringSubmatch(section); len(m) > 1 {
			tool.Name = m[1]
		} else {
			continue // no name found
		}

		// Extract description
		if m := descRe.FindStringSubmatch(section); len(m) > 1 {
			tool.Description = m[1]
		}

		// Extract returns
		if m := returnsRe.FindStringSubmatch(section); len(m) > 1 {
			tool.Returns = m[1]
		}

		// Extract parameters - find all .Param calls in this section
		paramMatches := paramRe.FindAllStringSubmatch(section, -1)
		for _, m := range paramMatches {
			if len(m) >= 6 {
				defaultVal := strings.TrimSpace(m[4])
				// Remove quotes from default value if present
				defaultVal = strings.Trim(defaultVal, `"`)
				if defaultVal == "nil" || defaultVal == "null" {
					defaultVal = ""
				}

				tool.Parameters = append(tool.Parameters, ParamInfo{
					Name:        m[1],
					Type:        m[2],
					Required:    m[3] == "true",
					Default:     defaultVal,
					Description: m[5],
				})
			}
		}

		// Extract trigger
		trigger := parseTrigger(section)
		if trigger != nil {
			tool.Trigger = trigger
		}

		// Extract sandbox uses
		tool.Uses = parseUses(section)

		if tool.Name != "" {
			tools = append(tools, tool)
		}
	}

	// Fallback: if no tools found from parse, try to use existing skill.yaml as base
	if len(tools) == 0 {
		tools = readToolsFromSkillYAML(dir)
	}

	return tools, nil
}

func splitByNewTool(source string) []string {
	var sections []string

	re := regexp.MustCompile(`(sdk\.)?NewTool\s*\(`)
	indices := re.FindAllStringIndex(source, -1)

	if len(indices) == 0 {
		return []string{source}
	}

	last := 0
	for _, idx := range indices {
		if last > 0 {
			sections = append(sections, source[last:idx[0]])
		}
		last = idx[0]
	}
	if last < len(source) {
		sections = append(sections, source[last:])
	}

	return sections
}

// parseTrigger 从工具定义中解析触发条件
func parseTrigger(section string) *TriggerInfo {
	// 查找 Trigger(...)
	triggerRe := regexp.MustCompile(`\.Trigger\s*\(`)
	if !triggerRe.MatchString(section) {
		return nil
	}

	trigger := &TriggerInfo{}

	// 解析 TriggerKeywords
	keywordsRe := regexp.MustCompile(`\.TriggerKeywords\s*\(\s*"([^"]+)"(?:\s*,\s*"([^"]+)")*\s*\)`)
	keywordsMatches := keywordsRe.FindAllStringSubmatch(section, -1)
	for _, match := range keywordsMatches {
		if len(match) >= 2 && match[1] != "" {
			trigger.Keywords = append(trigger.Keywords, match[1])
		}
		if len(match) >= 3 && match[2] != "" {
			trigger.Keywords = append(trigger.Keywords, match[2])
		}
	}

	// 解析 TriggerPrefix
	prefixRe := regexp.MustCompile(`\.TriggerPrefix\s*\(\s*"([^"]+)"\s*\)`)
	if m := prefixRe.FindStringSubmatch(section); len(m) > 1 {
		trigger.Prefix = m[1]
	}

	// 解析 TriggerRegex
	regexRe := regexp.MustCompile(`\.TriggerRegex\s*\(\s*"([^"]+)"\s*\)`)
	if m := regexRe.FindStringSubmatch(section); len(m) > 1 {
		trigger.Regex = m[1]
	}
	// 同时支持反引号
	regexReBacktick := regexp.MustCompile(`\.TriggerRegex\s*\(\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\)`)
	if m := regexReBacktick.FindStringSubmatch(section); len(m) > 1 {
		trigger.Regex = m[1]
	}

	// 解析 TriggerEvents
	eventsRe := regexp.MustCompile(`\.TriggerEvents\s*\(\s*"([^"]+)"(?:\s*,\s*"([^"]+)")*\s*\)`)
	eventsMatches := eventsRe.FindAllStringSubmatch(section, -1)
	for _, match := range eventsMatches {
		if len(match) >= 2 && match[1] != "" {
			trigger.Events = append(trigger.Events, match[1])
		}
		if len(match) >= 3 && match[2] != "" {
			trigger.Events = append(trigger.Events, match[2])
		}
	}

	// 解析 TriggerPriority
	priorityRe := regexp.MustCompile(`\.TriggerPriority\s*\(\s*(\d+)\s*\)`)
	if m := priorityRe.FindStringSubmatch(section); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &trigger.Priority)
	}

	// 如果没有找到任何触发条件，返回 nil
	if len(trigger.Keywords) == 0 && trigger.Prefix == "" && trigger.Regex == "" &&
		len(trigger.Events) == 0 && trigger.Priority == 0 {
		return nil
	}

	return trigger
}

// extractQuotedString 从字符串中提取引号包裹的内容
func extractQuotedString(s string) string {
	// 查找第一个引号的位置
	start := -1
	for i, c := range s {
		if c == '"' || c == '`' {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}

	// 查找结束引号
	end := -1
	quoteChar := rune(s[start-1])
	for i := start; i < len(s); i++ {
		if rune(s[i]) == quoteChar && (i == start || s[i-1] != '\\') {
			end = i
			break
		}
	}

	if end == -1 {
		return ""
	}

	return s[start:end]
}

// parseUses 解析工具的沙箱能力需求
// 支持的格式：
//   - .WithSandbox(sdk.SandboxFS, sdk.HostLog)
//   - .WithSandboxFS()
//   - .WithHostLog()
//   - .WithSandboxFS().WithHostLog()
func parseUses(section string) []string {
	var uses []string
	seen := make(map[string]bool)

	// 解析 WithSandbox(sdk.XXX, sdk.YYY)
	wsRe := regexp.MustCompile(`\.WithSandbox\s*\(\s*(?:sdk\.(\w+)(?:\s*,\s*sdk\.(\w+))*\s*)?\)`)
	wsMatches := wsRe.FindAllStringSubmatch(section, -1)
	for _, match := range wsMatches {
		for _, cap := range match[1:] {
			if cap != "" && !seen[cap] {
				uses = append(uses, cap)
				seen[cap] = true
			}
		}
	}

	// 解析便捷方法
	convenienceMethods := []string{
		"WithSandboxFS",
		"WithHostLog",
		"WithHostNow",
		"WithHostRandom",
		"WithHostHTTP",
		"WithHostEnv",
		"WithHostProcess",
	}

	for _, method := range convenienceMethods {
		re := regexp.MustCompile(`\.` + method + `\s*\(\s*\)`)
		if re.MatchString(section) {
			// 提取能力名称（去掉 With 前缀，转为小写）
			cap := strings.ToLower(method[4:]) // 去掉 "With" 前缀
			if !seen[cap] {
				uses = append(uses, cap)
				seen[cap] = true
			}
		}
	}

	return uses
}

func readVersion(dir string) string {
	// Try skill.yaml
	yamlPath := filepath.Join(dir, "skill.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		re := regexp.MustCompile(`version:\s*["']?([^\s"']+)`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			return m[1]
		}
	}

	// Try manifest.json
	jsonPath := filepath.Join(dir, "manifest.json")
	if data, err := os.ReadFile(jsonPath); err == nil {
		re := regexp.MustCompile(`"version"\s*:\s*"([^"]+)"`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			return m[1]
		}
	}

	return "1.0.0"
}

func detectType(dir string) string {
	yamlPath := filepath.Join(dir, "skill.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		re := regexp.MustCompile(`type:\s*["']?(\S+)`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			t := strings.ToLower(strings.Trim(m[1], `"'`))
			if t == "soi" {
				return "soi"
			}
			return "wasm"
		}
	}

	// Check for TinyGo markers
	if _, err := os.Stat(filepath.Join(dir, "main_tinygo.go")); err == nil {
		return "soi"
	}

	// Check if main.go uses sdk.RunTinyGo()
	if data, err := os.ReadFile(filepath.Join(dir, "main.go")); err == nil {
		if strings.Contains(string(data), "sdk.RunTinyGo") {
			return "soi"
		}
	}

	return "wasm"
}

func readToolsFromSkillYAML(dir string) []ToolInfo {
	var tools []ToolInfo

	yamlPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return tools
	}

	// Simple YAML parser for tools section
	content := string(data)

	// Find tools section
	toolsRe := regexp.MustCompile(`tools:\s*(?:-.*)*`)
	toolsSection := toolsRe.FindString(content)
	if toolsSection == "" {
		return tools
	}

	// Parse each tool
	toolRe := regexp.MustCompile(`-\s*name:\s*"([^"]+)"\s*description:\s*"([^"]+)"`)
	toolMatches := toolRe.FindAllStringSubmatch(content, -1)
	for _, m := range toolMatches {
		if len(m) >= 3 {
			tools = append(tools, ToolInfo{
				Name:        m[1],
				Description: m[2],
			})
		}
	}

	return tools
}

func generateSkillYAML(dir, name, version, pluginType string, tools []ToolInfo) error {
	yamlPath := filepath.Join(dir, "skill.yaml")

	var sb strings.Builder

	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: Skill\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", name))
	sb.WriteString(fmt.Sprintf("  version: \"%s\"\n", version))
	sb.WriteString("  description: \"SOI plugin\"\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  runtime:\n")
	sb.WriteString(fmt.Sprintf("    type: %s\n", pluginType))
	if pluginType == "soi" {
		sb.WriteString("    entry: wasm/plugin.soi\n")
	} else {
		sb.WriteString("    entry: wasm/plugin.wasm\n")
	}
	sb.WriteString("  provides:\n")
	sb.WriteString("    tools:\n")

	if len(tools) == 0 {
		// If no tools, add a placeholder
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
					if param.Default != "" {
						sb.WriteString(fmt.Sprintf("        default: %s\n", formatYAMLValue(param.Default)))
					}
				}
			}

			// 添加 trigger
			if tool.Trigger != nil {
				sb.WriteString("      trigger:\n")
				if len(tool.Trigger.Keywords) > 0 {
					sb.WriteString(fmt.Sprintf("        keywords: [%s]\n", strings.Join(tool.Trigger.Keywords, ", ")))
				}
				if tool.Trigger.Prefix != "" {
					sb.WriteString(fmt.Sprintf("        prefix: \"%s\"\n", tool.Trigger.Prefix))
				}
				if tool.Trigger.Regex != "" {
					sb.WriteString(fmt.Sprintf("        regex: \"%s\"\n", tool.Trigger.Regex))
				}
				if len(tool.Trigger.Events) > 0 {
					sb.WriteString(fmt.Sprintf("        events: [%s]\n", strings.Join(tool.Trigger.Events, ", ")))
				}
				if tool.Trigger.Priority != 0 {
					sb.WriteString(fmt.Sprintf("        priority: %d\n", tool.Trigger.Priority))
				}
			}

			// 添加 sandbox uses
			if len(tool.Uses) > 0 {
				sb.WriteString("      uses:\n")
				for _, use := range tool.Uses {
					sb.WriteString(fmt.Sprintf("      - %s\n", use))
				}
			}
		}
	}

	return os.WriteFile(yamlPath, []byte(sb.String()), 0644)
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

func formatYAMLValue(v string) string {
	if v == "true" || v == "false" {
		return v
	}
	if _, err := fmt.Sscanf(v, "%f", &v); err == nil {
		return v
	}
	return fmt.Sprintf("\"%s\"", escapeYAML(v))
}

func usage() {
	fmt.Fprintf(os.Stderr, `soi-sync — Synchronize plugin tool definitions to skill.yaml

Extracts tool definitions from source code and updates skill.yaml.

USAGE:
  soi-sync --dir <plugin-dir> [flags]

FLAGS:
  --dir      Plugin project directory (required)
  --name     Plugin name (auto-detected if not set)
  --version  Plugin version (auto-detected if not set)

EXAMPLES:
  soi-sync --dir ./my-plugin
  soi-sync --dir ./my-plugin --name custom-name
  soi-sync --dir ./my-plugin --version 1.1.0

`)
}

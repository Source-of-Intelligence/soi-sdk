//go:build !tinygo

package sdk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// SkillManifest — skill.yaml 结构化定义（与 soi/pkg/skill/types.go 对齐）
// ---------------------------------------------------------------------------

// SkillManifest represents the complete skill.yaml manifest structure.
// This mirrors soi/pkg/skill/types.go.Manifest for round-trip compatibility.
type SkillManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   SkillMetadata `yaml:"metadata"`
	Spec       SkillSpec     `yaml:"spec"`
}

// SkillMetadata contains skill metadata fields.
type SkillMetadata struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Author      string            `yaml:"author,omitempty"`
	Description string            `yaml:"description"`
	Tags        []string          `yaml:"tags,omitempty"`
	Icon        string            `yaml:"icon,omitempty"`
	License     string            `yaml:"license,omitempty"`
	Homepage    string            `yaml:"homepage,omitempty"`
	Repository  string            `yaml:"repository,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// SkillSpec contains skill specification.
type SkillSpec struct {
	Runtime  SkillRuntime      `yaml:"runtime"`
	Requires []SkillDependency `yaml:"requires,omitempty"`
	Provides SkillProvides     `yaml:"provides"`
}

// SkillRuntime is the runtime configuration.
type SkillRuntime struct {
	Type  string            `yaml:"type"`
	Entry string            `yaml:"entry,omitempty"`
	Main  string            `yaml:"main,omitempty"`
	Wasm  *SkillRuntimeWasm `yaml:"wasm,omitempty"`
	Uses  []string          `yaml:"uses,omitempty"`
}

// SkillRuntimeWasm is WASM-specific runtime configuration.
type SkillRuntimeWasm struct {
	Sandbox        SkillRuntimeWasmSandbox `yaml:"sandbox,omitempty"`
	Memory         SkillRuntimeWasmMemory  `yaml:"memory,omitempty"`
	Timeout        string                  `yaml:"timeout,omitempty"`
	MaxConcurrency int                     `yaml:"maxConcurrency,omitempty"`
	AllowedHosts   []string                `yaml:"allowedHosts,omitempty"`
	AllowExec      bool                    `yaml:"allowExec,omitempty"`
	ExecWhitelist  []string                `yaml:"execWhitelist,omitempty"`
	HostAPIVersion string                  `yaml:"hostAPIVersion,omitempty"`
}

type SkillRuntimeWasmSandbox struct {
	Subdir string `yaml:"subdir,omitempty"`
}

type SkillRuntimeWasmMemory struct {
	Initial uint32 `yaml:"initial,omitempty"`
	Maximum uint32 `yaml:"maximum,omitempty"`
}

type SkillDependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// SkillProvides declares what the skill offers.
type SkillProvides struct {
	Tools        []ToolDef    `yaml:"tools,omitempty"`
	Triggers     []string     `yaml:"triggers,omitempty"` // legacy: simple string triggers
	Trigger      *TriggersCfg `yaml:"trigger,omitempty"`  // new: structured trigger config
	Instructions string       `yaml:"instructions,omitempty"`
}

// TriggersCfg defines trigger configuration at provides level.
type TriggersCfg struct {
	Keywords []string `yaml:"keywords,omitempty"`
	Prefix   string   `yaml:"prefix,omitempty"`
	Regex    string   `yaml:"regex,omitempty"`
	Events   []string `yaml:"events,omitempty"`
	Priority int      `yaml:"priority,omitempty"`
}

// SkillConfig is the configuration passed to GenerateSkillYAML.
type SkillConfig struct {
	// Required fields
	Name        string
	Version     string
	RuntimeType string // "go", "wasm", "soi"
	Entry       string

	// Optional metadata
	Author      string
	Description string
	Tags        []string
	Icon        string
	License     string
	Homepage    string
	Repository  string
	Labels      map[string]string

	// Optional runtime
	Main string
	Wasm *SkillRuntimeWasm
	Uses []string // Sandbox capabilities at plugin level

	// Optional provides
	Triggers     *TriggersCfg // Trigger configuration at provides level
	Instructions string

	// Optional dependencies
	Requires []SkillDependency
}

// ---------------------------------------------------------------------------
// GenerateSkillYAML — 从 SDK 注册表 + 配置生成完整 skill.yaml
// ---------------------------------------------------------------------------

// GenerateSkillYAML builds a complete skill.yaml string from the registered
// tools and the provided SkillConfig. It replaces the old manual string
// concatenation approach with structured YAML generation.
func GenerateSkillYAML(cfg SkillConfig) string {
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}

	var b strings.Builder

	// --- apiVersion / kind ---
	b.WriteString("apiVersion: v1\n")
	b.WriteString("kind: Skill\n")

	// --- metadata ---
	b.WriteString("metadata:\n")
	writeYAMLString(&b, "  name", cfg.Name, true)
	writeYAMLString(&b, "  version", quoteString(cfg.Version), true)
	if cfg.Author != "" {
		writeYAMLString(&b, "  author", cfg.Author, true)
	}
	writeYAMLString(&b, "  description", cfg.Description, true)
	if len(cfg.Tags) > 0 {
		writeYAMLStringArray(&b, "  tags", cfg.Tags)
	}
	if cfg.Icon != "" {
		writeYAMLString(&b, "  icon", cfg.Icon, true)
	}
	if cfg.License != "" {
		writeYAMLString(&b, "  license", cfg.License, true)
	}
	if cfg.Homepage != "" {
		writeYAMLString(&b, "  homepage", cfg.Homepage, true)
	}
	if cfg.Repository != "" {
		writeYAMLString(&b, "  repository", cfg.Repository, true)
	}
	if len(cfg.Labels) > 0 {
		writeYAMLStringMap(&b, "  labels", cfg.Labels)
	}

	// --- spec ---
	b.WriteString("spec:\n")

	// spec.runtime
	b.WriteString("  runtime:\n")
	writeYAMLString(&b, "    type", cfg.RuntimeType, true)
	if cfg.Entry != "" {
		writeYAMLString(&b, "    entry", cfg.Entry, true)
	}
	if cfg.Main != "" {
		writeYAMLString(&b, "    main", cfg.Main, true)
	}
	// Use plugin-level uses from SDK registry if cfg.Uses is empty
	usesToWrite := cfg.Uses
	if len(usesToWrite) == 0 {
		usesToWrite = GetPluginUses()
	}
	if len(usesToWrite) > 0 {
		writeYAMLStringArray(&b, "    uses", usesToWrite)
	}
	if cfg.Wasm != nil {
		writeWasmConfig(&b, cfg.Wasm)
	}

	// spec.requires
	if len(cfg.Requires) > 0 {
		b.WriteString("  requires:\n")
		for _, dep := range cfg.Requires {
			b.WriteString("    - name: " + dep.Name + "\n")
			b.WriteString("      version: " + dep.Version + "\n")
		}
	}

	// spec.provides
	b.WriteString("  provides:\n")

	// Trigger configuration at provides level
	var triggerToUse *TriggersCfg
	if cfg.Triggers != nil {
		triggerToUse = cfg.Triggers
	} else {
		// Check SDK's global providesTrigger
		globalTrigger := GetProvidesTrigger()
		if len(globalTrigger.Keywords) > 0 || globalTrigger.Prefix != "" || globalTrigger.Regex != "" || len(globalTrigger.Events) > 0 || globalTrigger.Priority != 0 {
			triggerToUse = &TriggersCfg{
				Keywords: globalTrigger.Keywords,
				Prefix:   globalTrigger.Prefix,
				Regex:    globalTrigger.Regex,
				Events:   globalTrigger.Events,
				Priority: globalTrigger.Priority,
			}
		}
	}
	if triggerToUse != nil {
		b.WriteString("    triggers:\n")
		if len(triggerToUse.Keywords) > 0 {
			b.WriteString("      keywords: [")
			for i, kw := range triggerToUse.Keywords {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(quoteString(kw))
			}
			b.WriteString("]\n")
		}
		if triggerToUse.Prefix != "" {
			b.WriteString("      prefix: " + quoteString(triggerToUse.Prefix) + "\n")
		}
		if triggerToUse.Regex != "" {
			b.WriteString("      regex: " + quoteString(triggerToUse.Regex) + "\n")
		}
		if len(triggerToUse.Events) > 0 {
			b.WriteString("      events: [")
			for i, ev := range triggerToUse.Events {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(quoteString(ev))
			}
			b.WriteString("]\n")
		}
		if triggerToUse.Priority != 0 {
			fmt.Fprintf(&b, "      priority: %d\n", triggerToUse.Priority)
		}
	}

	// tools from SDK registry
	tools := GetToolDefs()
	if len(tools) > 0 {
		b.WriteString("    tools:\n")
		for i, t := range tools {
			writeToolDefYAML(&b, t, "      ")
			if i < len(tools)-1 {
				b.WriteString("\n")
			}
		}
	}

	if cfg.Instructions != "" {
		writeYAMLString(&b, "    instructions", cfg.Instructions, true)
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// YAML helpers
// ---------------------------------------------------------------------------

func writeYAMLString(b *strings.Builder, key, value string, newline bool) {
	b.WriteString(key + ": " + value + "\n")
}

func writeYAMLStringArray(b *strings.Builder, key string, values []string) {
	b.WriteString(key + ":\n")
	for _, v := range values {
		b.WriteString("  - " + quoteString(v) + "\n")
	}
}

func writeYAMLStringMap(b *strings.Builder, prefix string, m map[string]string) {
	b.WriteString(prefix + ":\n")
	for k, v := range m {
		b.WriteString("  " + k + ": " + quoteString(v) + "\n")
	}
}

func writeWasmConfig(b *strings.Builder, w *SkillRuntimeWasm) {
	b.WriteString("    wasm:\n")
	if w.Sandbox.Subdir != "" {
		b.WriteString("      sandbox:\n")
		b.WriteString("        subdir: " + w.Sandbox.Subdir + "\n")
	}
	if w.Memory.Initial > 0 || w.Memory.Maximum > 0 {
		b.WriteString("      memory:\n")
		if w.Memory.Initial > 0 {
			fmt.Fprintf(b, "        initial: %d\n", w.Memory.Initial)
		}
		if w.Memory.Maximum > 0 {
			fmt.Fprintf(b, "        maximum: %d\n", w.Memory.Maximum)
		}
	}
	if w.Timeout != "" {
		b.WriteString("      timeout: " + w.Timeout + "\n")
	}
	if w.MaxConcurrency > 0 {
		fmt.Fprintf(b, "      maxConcurrency: %d\n", w.MaxConcurrency)
	}
	if len(w.AllowedHosts) > 0 {
		writeYAMLStringArrayWithIndent(b, "      allowedHosts", w.AllowedHosts)
	}
	if w.AllowExec {
		b.WriteString("      allowExec: true\n")
	}
	if len(w.ExecWhitelist) > 0 {
		writeYAMLStringArrayWithIndent(b, "      execWhitelist", w.ExecWhitelist)
	}
	if w.HostAPIVersion != "" {
		b.WriteString("      hostAPIVersion: " + w.HostAPIVersion + "\n")
	}
}

func writeYAMLStringArrayWithIndent(b *strings.Builder, key string, values []string) {
	b.WriteString(key + ":\n")
	for _, v := range values {
		b.WriteString("  - " + quoteString(v) + "\n")
	}
}

func writeToolDefYAML(b *strings.Builder, t ToolDef, indent string) {
	b.WriteString(indent + "- name: " + t.Name + "\n")
	b.WriteString(indent + "  description: " + quoteString(t.Description) + "\n")
	if len(t.Uses) > 0 {
		writeYAMLStringArrayWithIndent(b, indent+"  uses", t.Uses)
	}
	if len(t.Parameters) > 0 {
		b.WriteString(indent + "  parameters:\n")
		for _, p := range t.Parameters {
			b.WriteString(indent + "    - name: " + p.Name + "\n")
			b.WriteString(indent + "      type: " + p.Type + "\n")
			if p.Required {
				b.WriteString(indent + "      required: true\n")
			}
			if p.Default != nil {
				b.WriteString(indent + "      default: " + formatYAMLValue(p.Default) + "\n")
			}
			if p.Description != "" {
				b.WriteString(indent + "      description: " + quoteString(p.Description) + "\n")
			}
			if len(p.Enum) > 0 {
				b.WriteString(indent + "      enum:\n")
				for _, e := range p.Enum {
					b.WriteString(indent + "        - " + quoteString(e) + "\n")
				}
			}
		}
	}
	if t.Returns != "" {
		b.WriteString(indent + "  returns: " + quoteString(t.Returns) + "\n")
	}
	if len(t.Examples) > 0 {
		b.WriteString(indent + "  examples:\n")
		for _, ex := range t.Examples {
			b.WriteString(indent + "    - input:\n")
			for k, v := range ex.Input {
				b.WriteString(indent + "        " + k + ": " + formatYAMLValue(v) + "\n")
			}
			b.WriteString(indent + "      output: " + quoteString(ex.Output) + "\n")
		}
	}
}

func quoteString(s string) string {
	if s == "" {
		return "\"\""
	}
	// Only quote if contains special characters
	if strings.ContainsAny(s, ":#{}[]|>!\"'&*?`@\\, \t\n") {
		// Use double quotes, escape internal double quotes
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}

func formatYAMLValue(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return quoteString(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		// Format integers without decimal
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	default:
		// Fallback to JSON
		data, _ := json.Marshal(v)
		return string(data)
	}
}

// ---------------------------------------------------------------------------
// BuildManifestJSON — 生成运行时 Manifest JSON（保持向后兼容）
// ---------------------------------------------------------------------------

func BuildManifestJSON() (string, error) {
	m := GetManifest()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

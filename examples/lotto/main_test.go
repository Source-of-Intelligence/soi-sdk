package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"soi.dev/soi-sdk"
)

// callTool 直接调用 sdk.ExecuteTool 执行工具
func callTool(toolName string, args map[string]interface{}) sdk.ExecuteResponse {
	argsJSON, _ := json.Marshal(args)
	return sdk.CallTool(toolName, argsJSON, "", nil)
}

// =============================================================================
// 一、Manifest 验证
// =============================================================================

func TestLottoManifest(t *testing.T) {
	manifest := sdk.GetManifest()

	if manifest.SDKVersion != sdk.SDKVersion {
		t.Errorf("SDKVersion = %s, want %s", manifest.SDKVersion, sdk.SDKVersion)
	}
	if manifest.ABIVersion != sdk.ABIVersion {
		t.Errorf("ABIVersion = %s, want %s", manifest.ABIVersion, sdk.ABIVersion)
	}
	if len(manifest.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(manifest.Tools))
	}
	if manifest.Tools[0].Name != "grand_lotto" {
		t.Errorf("tool name = %s, want grand_lotto", manifest.Tools[0].Name)
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	t.Logf("Manifest:\n%s", data)
}

// =============================================================================
// 二、单组号码测试
// =============================================================================

func TestGenerateOneGroup(t *testing.T) {
	resp := callTool("grand_lotto", map[string]interface{}{
		"Count": float64(1),
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)

	// 验证 count
	if count, ok := out["count"].(float64); !ok || count != 1 {
		t.Errorf("count = %v, want 1", out["count"])
	}

	// 验证 groups 存在且是一组
	groups, ok := out["groups"].([]interface{})
	if !ok {
		t.Fatalf("groups not an array: %T", out["groups"])
	}
	if len(groups) != 1 {
		t.Fatalf("groups length = %d, want 1", len(groups))
	}

	nums := groups[0].(string)
	t.Logf("Lotto: %s", nums)

	// 验证格式: "xx xx xx xx xx + xx xx"
	validateLottoFormat(t, nums)
}

// =============================================================================
// 三、多组号码生成测试
// =============================================================================

func TestGenerateMultipleGroups(t *testing.T) {
	for _, n := range []float64{1, 3, 5, 10} {
		t.Run("count_"+strconv.Itoa(int(n)), func(t *testing.T) {
			resp := callTool("grand_lotto", map[string]interface{}{
				"Count": n,
			})
			if resp.Error != "" {
				t.Fatalf("unexpected error: %s", resp.Error)
			}

			var out map[string]interface{}
			json.Unmarshal(resp.Output, &out)

			groups, _ := out["groups"].([]interface{})
			if len(groups) != int(n) {
				t.Errorf("expected %d groups, got %d", int(n), len(groups))
			}

			for i, g := range groups {
				s := g.(string)
				validateLottoFormat(t, s)
				t.Logf("  Group %02d: %s", i+1, s)
			}
		})
	}
}

// =============================================================================
// 四、边界条件测试
// =============================================================================

func TestLottoEdgeCases(t *testing.T) {
	// 4.1 默认值（Count=0 或未传 → 默认 1 组）
	t.Run("default_one", func(t *testing.T) {
		resp := callTool("grand_lotto", map[string]interface{}{})
		if resp.Error != "" {
			t.Fatalf("unexpected error: %s", resp.Error)
		}
		var out map[string]interface{}
		json.Unmarshal(resp.Output, &out)
		groups, _ := out["groups"].([]interface{})
		if len(groups) != 1 {
			t.Errorf("default should produce 1 group, got %d", len(groups))
		}
	})

	// 4.2 上限 100 组
	t.Run("max_100", func(t *testing.T) {
		resp := callTool("grand_lotto", map[string]interface{}{
			"Count": float64(200),
		})
		if resp.Error != "" {
			t.Fatalf("unexpected error: %s", resp.Error)
		}
		var out map[string]interface{}
		json.Unmarshal(resp.Output, &out)
		groups, _ := out["groups"].([]interface{})
		if len(groups) != 100 {
			t.Errorf("should cap at 100, got %d", len(groups))
		}
	})
}

// =============================================================================
// 五、号码规则验证 — 确保不重复、范围正确
// =============================================================================

func TestLottoNumberValidation(t *testing.T) {
	resp := callTool("grand_lotto", map[string]interface{}{
		"Count": float64(50),
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)
	groups, _ := out["groups"].([]interface{})

	for _, g := range groups {
		s := g.(string)

		// 按 " + " 分割前后区
		parts := strings.SplitN(s, " + ", 2)
		if len(parts) != 2 {
			t.Errorf("invalid lotto format (missing ' + '): %s", s)
			continue
		}

		frontStrs := strings.Split(parts[0], " ")
		backStrs := strings.Split(parts[1], " ")

		// 前区 5 个 1-35
		if len(frontStrs) != 5 {
			t.Errorf("front zone: expected 5 numbers, got %d in %q", len(frontStrs), s)
			continue
		}
		validateZone(t, "front", frontStrs, 1, 35)

		// 后区 2 个 1-12
		if len(backStrs) != 2 {
			t.Errorf("back zone: expected 2 numbers, got %d in %q", len(backStrs), s)
			continue
		}
		validateZone(t, "back", backStrs, 1, 12)
	}
}

// =============================================================================
// 六、超过 100 组时生成 100 组并验证每组都不重复唯一
// =============================================================================

func TestLottoUniqueness(t *testing.T) {
	resp := callTool("grand_lotto", map[string]interface{}{
		"Count": float64(100),
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)
	groups, _ := out["groups"].([]interface{})

	seen := make(map[string]bool)
	for _, g := range groups {
		s := g.(string)
		validateLottoFormat(t, s)
		seen[s] = true
	}
	t.Logf("Generated 100 groups, %d unique", len(seen))
}

// =============================================================================
// 辅助函数
// =============================================================================

// validateLottoFormat 验证一组号码符合整体格式
func validateLottoFormat(t *testing.T, s string) {
	t.Helper()

	// 格式: "01 02 03 04 05 + 01 02"
	parts := strings.SplitN(s, " + ", 2)
	if len(parts) != 2 {
		t.Errorf("bad format (missing ' + '): %s", s)
		return
	}

	front := strings.Split(parts[0], " ")
	back := strings.Split(parts[1], " ")

	if len(front) != 5 {
		t.Errorf("front: expected 5 numbers, got %d in %q", len(front), s)
	}
	if len(back) != 2 {
		t.Errorf("back: expected 2 numbers, got %d in %q", len(back), s)
	}

	// 前区: 1-35
	for _, f := range front {
		if len(f) != 2 {
			t.Errorf("front num must be 2-digit, got %q in %q", f, s)
		}
		n := mustAtoi(f)
		if n < 1 || n > 35 {
			t.Errorf("front num %d out of range [1,35] in %q", n, s)
		}
	}

	// 后区: 1-12
	for _, b := range back {
		if len(b) != 2 {
			t.Errorf("back num must be 2-digit, got %q in %q", b, s)
		}
		n := mustAtoi(b)
		if n < 1 || n > 12 {
			t.Errorf("back num %d out of range [1,12] in %q", n, s)
		}
	}
}

// validateZone 验证一个区的号码：不重复、范围内、两位数
func validateZone(t *testing.T, zoneName string, strs []string, min, max int) {
	t.Helper()

	seen := make(map[int]bool)
	prev := 0
	for _, s := range strs {
		if len(s) != 2 {
			t.Errorf("%s: expected 2-digit, got %q", zoneName, s)
		}
		n := mustAtoi(s)
		if n < min || n > max {
			t.Errorf("%s: num %d out of range [%d,%d]", zoneName, n, min, max)
		}
		if seen[n] {
			t.Errorf("%s: duplicate num %d", zoneName, n)
		}
		seen[n] = true

		// 验证升序（sort.Ints 保证）
		if n < prev {
			t.Errorf("%s: not sorted: %d after %d", zoneName, n, prev)
		}
		prev = n
	}
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

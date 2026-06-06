// lotto — 大乐透号码生成器 SOI WASM 插件
//
// 生成大乐透号码：5 个 1-35 不重复前区号 + 2 个 1-12 不重复后区号
// 格式: "01 02 03 04 05 + 01 02"
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o wasm/plugin.wasm .
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"soi.dev/soi-sdk"
)

// random source，插件加载时初始化一次
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

func init() {
	sdk.RegisterToolWithDef(sdk.ToolDef{
		Name:        "grand_lotto",
		Description: "大乐透号码生成器 — 生成指定组数的大乐透号码（前区 5 个 1-35 不重复 + 后区 2 个 1-12 不重复），格式「xx xx xx xx xx + xx xx」，不足两位补前置 0",
		Parameters: []sdk.ParamDef{
			{
				Name:        "Count",
				Type:        "number",
				Required:    true,
				Default:     1,
				Description: "生成组数（正整数，1-100）",
			},
		},
		Returns: `{"groups": ["string array"], "count": number}`,
	}, handler)
}

func handler(args json.RawMessage) (interface{}, error) {
	var p struct{ Count float64 }
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	n := int(p.Count)
	if n < 1 {
		n = 1
	}
	if n > 100 {
		n = 100
	}

	groups := make([]string, n)
	for i := 0; i < n; i++ {
		groups[i] = generateOne()
	}

	return map[string]interface{}{
		"groups": groups,
		"count":  n,
	}, nil
}

// generateOne 生成一组大乐透号码
// 返回格式: "03 12 18 27 31 + 05 09"
func generateOne() string {
	front := pickN(5, 1, 35)
	back := pickN(2, 1, 12)
	return fmt.Sprintf("%s + %s", formatGroup(front), formatGroup(back))
}

// pickN 从 [min, max] 范围内不重复随机选取 n 个整数，返回排序后的切片
func pickN(n, min, max int) []int {
	pool := make([]int, max-min+1)
	for i := range pool {
		pool[i] = min + i
	}

	// Fisher-Yates 洗牌，只洗前 n 个
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(pool)-i)
		pool[i], pool[j] = pool[j], pool[i]
	}

	result := make([]int, n)
	copy(result, pool[:n])
	sort.Ints(result)
	return result
}

// formatGroup 将数字切片格式化为两位数空格分隔字符串
func formatGroup(nums []int) string {
	parts := make([]string, len(nums))
	for i, v := range nums {
		parts[i] = fmt.Sprintf("%02d", v)
	}
	return joinStrings(parts, " ")
}

// joinStrings 简单的字符串拼接，避免引入 strings 包（WASM 体积优化）
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	var result []byte
	for i, s := range parts {
		if i > 0 {
			result = append(result, sep...)
		}
		result = append(result, s...)
	}
	return string(result)
}

func main() { sdk.Run() }

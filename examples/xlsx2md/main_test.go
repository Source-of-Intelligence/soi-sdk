package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Source-of-Intelligence/soi-sdk"
	"github.com/Source-of-Intelligence/soi-vos"
)

// =========================================================================
// 测试辅助：在内存中生成合法 XLSX 文件
// =========================================================================

// makeTestXLSX 生成一个最小合法 XLSX（ZIP 包）。
// sheets 的 key 是工作表名，value 是 [][]string（第一行为表头）。
func makeTestXLSX(sheets map[string][][]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// -- [Content_Types].xml --
	writeZip(zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`+
		`<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>`+
		xmlOverrides(sheets)+
		`</Types>`)

	// -- _rels/.rels --
	writeZip(zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>`+
		`</Relationships>`)

	// -- shared strings (collect all text distinct values) --
	ssXML, ssIndex := buildSharedStrings(sheets)
	writeZip(zw, "xl/sharedStrings.xml", ssXML)

	// -- workbook.xml --
	writeZip(zw, "xl/workbook.xml", buildWorkbookXML(sheets))

	// -- workbook.xml.rels --
	writeZip(zw, "xl/_rels/workbook.xml.rels", buildWorkbookRels(sheets))

	// -- 每个 sheet 的 worksheet --
	idx := 1
	for _, data := range sheets {
		writeZip(zw, fmt.Sprintf("xl/worksheets/sheet%d.xml", idx), buildSheetXML(data, ssIndex))
		idx++
	}

	zw.Close()
	return buf.Bytes()
}

func writeZip(zw *zip.Writer, name, content string) {
	w, _ := zw.Create(name)
	w.Write([]byte(content))
}

func xmlOverrides(sheets map[string][][]string) string {
	var b strings.Builder
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>`)
	i := 1
	for range sheets {
		b.WriteString(fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i))
		i++
	}
	return b.String()
}

func buildSharedStrings(sheets map[string][][]string) (xml string, index map[string]int) {
	index = make(map[string]int)
	var si []string
	for _, rows := range sheets {
		for _, row := range rows {
			for _, cell := range row {
				if cell != "" {
					if _, ok := index[cell]; !ok {
						index[cell] = len(si)
						si = append(si, cell)
					}
				}
			}
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(fmt.Sprintf(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, len(si), len(si)))
	for _, s := range si {
		b.WriteString(`<si><t>` + xmlEscape(s) + `</t></si>`)
	}
	b.WriteString(`</sst>`)
	return b.String(), index
}

func buildWorkbookXML(sheets map[string][][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	idx := 1
	for name := range sheets {
		b.WriteString(fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(name), idx, idx))
		idx++
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func buildWorkbookRels(sheets map[string][][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	idx := 1
	for range sheets {
		b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, idx, idx))
		idx++
	}
	b.WriteString(`<Relationship Id="rId99" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>`)
	b.WriteString(`</Relationships>`)
	return b.String()
}

func buildSheetXML(data [][]string, ssIndex map[string]int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for ri, row := range data {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, ri+1))
		for ci, cell := range row {
			ref := cellRef(ri+1, ci+1)
			if cell == "" {
				continue
			}
			// 尝试作数字，否则走 shared string
			if isNumeric(cell) {
				b.WriteString(fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, cell))
			} else {
				idx := ssIndex[cell]
				b.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, ref, idx))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func cellRef(row, col int) string {
	colStr := ""
	for c := col; c > 0; c = (c - 1) / 26 {
		colStr = string(rune('A'+(c-1)%26)) + colStr
	}
	return fmt.Sprintf("%s%d", colStr, row)
}

func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	dots := 0
	for i, c := range s {
		if c == '-' && i == 0 {
			continue
		}
		if c == '.' {
			dots++
			if dots > 1 {
				return false
			}
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// =========================================================================
// 测试辅助：使用 MockHost 直接调用 SOI 工具
// =========================================================================

// callSOITool 使用 MockHost 直接调用 sdk.ExecuteTool 执行 SOI 工具
func callSOITool(t *testing.T, toolName string, args map[string]interface{}, files map[string][]byte) sdk.ExecuteResponse {
	t.Helper()
	host := vos.NewMockHost(nil)
	for path, data := range files {
		host.SetFile(path, data)
	}
	argsJSON, _ := json.Marshal(args)
	return sdk.CallTool(toolName, argsJSON, "", host)
}

// ---- 1) 基础单表转换 ----

func TestBasicSingleSheet(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Sheet1": {
			{"Name", "Age", "City"},
			{"Alice", "30", "NYC"},
			{"Bob", "25", "LA"},
		},
	})

	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "report.xlsx",
	}, map[string][]byte{
		"report.xlsx": xlsx,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)

	if out["output_path"] != "report.md" {
		t.Errorf("expected output_path=report.md, got %v", out["output_path"])
	}
	if v, _ := out["sheets_processed"].(float64); v != 1 {
		t.Errorf("expected 1 sheet processed, got %v", v)
	}

	// 读取生成的 MD 并校验
	host := vos.NewMockHost(nil)
	host.SetFile("report.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "report.xlsx",
	}, map[string][]byte{
		"report.xlsx": xlsx,
	})
	md, err := host.GetFile("report.md")
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	mdStr := string(md)
	if !strings.Contains(mdStr, "# report.xlsx") {
		t.Error("md missing title")
	}
	if !strings.Contains(mdStr, "## Sheet1") {
		t.Error("md missing sheet heading")
	}
	if !strings.Contains(mdStr, "| Name | Age | City |") {
		t.Errorf("md missing header row, got:\n%s", mdStr)
	}
	if !strings.Contains(mdStr, "| Alice | 30 | NYC |") {
		t.Errorf("md missing data row, got:\n%s", mdStr)
	}
	if !strings.Contains(mdStr, "| --- | --- | --- |") {
		t.Error("md missing separator row")
	}
}

// ---- 2) 多表 XLSX ----

func TestMultiSheet(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Users": {
			{"ID", "Name"},
			{"1", "Alice"},
			{"2", "Bob"},
		},
		"Products": {
			{"SKU", "Price"},
			{"A100", "9.99"},
			{"B200", "19.99"},
		},
	})

	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "data.xlsx",
	}, map[string][]byte{
		"data.xlsx": xlsx,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)

	if v, _ := out["sheets_processed"].(float64); v != 2 {
		t.Errorf("expected 2 sheets, got %v", v)
	}

	host := vos.NewMockHost(nil)
	host.SetFile("data.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "data.xlsx",
	}, map[string][]byte{
		"data.xlsx": xlsx,
	})
	md, _ := host.GetFile("data.md")
	mdStr := string(md)
	if !strings.Contains(mdStr, "## Users") {
		t.Error("md missing Users sheet")
	}
	if !strings.Contains(mdStr, "## Products") {
		t.Error("md missing Products sheet")
	}
}

// ---- 3) 指定单个 sheet ----

func TestSpecificSheet(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"SheetA": {{"Col1"}, {"A"}},
		"SheetB": {{"ColX"}, {"B"}},
	})

	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "two.xlsx",
		"sheet":  "SheetB",
	}, map[string][]byte{
		"two.xlsx": xlsx,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)

	if v, _ := out["sheets_processed"].(float64); v != 1 {
		t.Errorf("expected 1 sheet, got %v", v)
	}

	host := vos.NewMockHost(nil)
	host.SetFile("two.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "two.xlsx",
		"sheet":  "SheetB",
	}, map[string][]byte{
		"two.xlsx": xlsx,
	})
	md, _ := host.GetFile("two.md")
	mdStr := string(md)
	if strings.Contains(mdStr, "## SheetA") {
		t.Error("md should NOT contain SheetA")
	}
	if !strings.Contains(mdStr, "## SheetB") {
		t.Error("md missing SheetB")
	}
}

// ---- 4) 自定义输出路径 ----

func TestCustomOutputPath(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Data": {{"K"}, {"V"}},
	})

	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "input.xlsx",
		"output": "/out/result.md",
	}, map[string][]byte{
		"input.xlsx": xlsx,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	var out map[string]interface{}
	json.Unmarshal(resp.Output, &out)

	if out["output_path"] != "/out/result.md" {
		t.Errorf("expected /out/result.md, got %v", out["output_path"])
	}

	host := vos.NewMockHost(nil)
	host.SetFile("input.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "input.xlsx",
		"output": "/out/result.md",
	}, map[string][]byte{
		"input.xlsx": xlsx,
	})
	md, _ := host.GetFile("/out/result.md")
	if !strings.Contains(string(md), "| K |") {
		t.Error("md missing header")
	}
}

// ---- 5) 错误情况：缺少 source ----

func TestErrorMissingSource(t *testing.T) {
	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{}, nil)
	if resp.Error == "" {
		t.Error("expected error for missing source")
	}
}

// ---- 6) 错误情况：文件不存在 ----

func TestErrorFileNotFound(t *testing.T) {
	resp := callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "nonexistent.xlsx",
	}, nil)
	if resp.Error == "" {
		t.Error("expected error for nonexistent file")
	}
}

// ---- 7) Markdown 特殊字符转义 ----

func TestMarkdownEscape(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Data": {
			{"Text"},
			{"a | b"},
			{"# heading"},
			{"[link](url)"},
		},
	})

	host := vos.NewMockHost(nil)
	host.SetFile("escape.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "escape.xlsx",
	}, map[string][]byte{
		"escape.xlsx": xlsx,
	})
	md, _ := host.GetFile("escape.md")
	mdStr := string(md)
	// 管道符应该被转义
	if !strings.Contains(mdStr, `a \| b`) {
		t.Errorf("pipe not escaped in md:\n%s", mdStr)
	}
}

// ---- 8) 空表格 ----

func TestEmptySheet(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Empty": {},
	})

	host := vos.NewMockHost(nil)
	host.SetFile("empty.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "empty.xlsx",
	}, map[string][]byte{
		"empty.xlsx": xlsx,
	})
	md, _ := host.GetFile("empty.md")
	if !strings.Contains(string(md), "空表格") {
		t.Error("empty sheet should show placeholder")
	}
}

// ---- 9) 纯数字表格（无 shared strings） ----

func TestNumericOnly(t *testing.T) {
	xlsx := makeTestXLSX(map[string][][]string{
		"Stats": {
			{"10", "20", "30"},
			{"100", "200", "300"},
		},
	})

	host := vos.NewMockHost(nil)
	host.SetFile("nums.xlsx", xlsx)
	callSOITool(t, "xlsx_to_md", map[string]interface{}{
		"source": "nums.xlsx",
	}, map[string][]byte{
		"nums.xlsx": xlsx,
	})
	md, _ := host.GetFile("nums.md")
	if !strings.Contains(string(md), "| 10 | 20 | 30 |") {
		t.Errorf("missing numeric header, got:\n%s", string(md))
	}
}

// ---- 10) manifest 验证：确认是 SOI 插件 ----

func TestXLSXManifest(t *testing.T) {
	tools := sdk.GetToolDefs()
	found := false
	for _, td := range tools {
		if td.Name == "xlsx_to_md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("xlsx_to_md not found in manifest")
	}
}

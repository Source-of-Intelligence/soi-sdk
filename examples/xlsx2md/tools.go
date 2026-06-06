package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	sdk "soi.dev/soi-sdk"
)

//export registerTools
func registerTools() {
	sdk.RegisterSOITool(sdk.ToolDef{
		Name:        "xlsx_to_md",
		Description: "读取 XLSX 文件，将每个工作表转换为 Markdown 表格格式并写入输出文件",
		Parameters: []sdk.ParamDef{
			{Name: "source", Type: "string", Required: true,
				Description: "沙箱中 XLSX 文件的路径"},
			{Name: "output", Type: "string", Required: false,
				Description: "输出的 .md 文件路径（默认：与源文件同名，扩展名改为 .md）"},
			{Name: "sheet", Type: "string", Required: false,
				Description: "要转换的指定工作表名称（留空则转换全部工作表）"},
		},
		Returns: "object with output_path / sheets_processed / sheet_names",
	}, xlsxToMdHandler)
}

func xlsxToMdHandler(args json.RawMessage, ctx *sdk.SandboxContext) (interface{}, error) {
	// Avoid TinyGo json.Unmarshal for UTF-8 strings — manually extract fields.
	p := struct {
		Source string
		Output string
		Sheet  string
	}{
		Source: extractJSONString([]byte(args), "source"),
		Output: extractJSONString([]byte(args), "output"),
		Sheet:  extractJSONString([]byte(args), "sheet"),
	}

	if p.Source == "" {
		return nil, fmt.Errorf("缺少参数 source（XLSX 文件路径）")
	}

	if ctx == nil || ctx.Host == nil {
		return nil, fmt.Errorf("sandbox host 不可用，请使用 SOI 运行时")
	}

	// 1) 从 sandbox 读取 XLSX 字节
	data, err := ctx.Host.SandboxRead(p.Source)
	if err != nil {
		return nil, fmt.Errorf("读取 XLSX 文件失败: %w", err)
	}

	// 2) 解析
	reader, err := newXLSXReader(data)
	if err != nil {
		return nil, fmt.Errorf("解析 XLSX 失败: %w", err)
	}

	sheetNames, err := reader.getSheetNames()
	if err != nil {
		return nil, fmt.Errorf("获取工作表列表失败: %w", err)
	}

	// 3) 确定要转换的 sheet 列表
	var toConvert []string
	if p.Sheet != "" {
		toConvert = []string{p.Sheet}
	} else {
		toConvert = sheetNames
	}

	// 4) 逐 sheet 转换
	var mdBuilder strings.Builder
	mdBuilder.WriteString(fmt.Sprintf("# %s\n\n", path.Base(p.Source)))
	processed := 0
	for _, name := range toConvert {
		sheetData, err := reader.getSheetData(name)
		if err != nil {
			return nil, fmt.Errorf("读取工作表 [%s] 失败: %w", name, err)
		}
		mdBuilder.WriteString(fmt.Sprintf("## %s\n\n", name))
		mdBuilder.WriteString(convertToMarkdown(sheetData))
		mdBuilder.WriteString("\n\n")
		processed++
	}

	// 5) 确定输出路径
	outPath := p.Output
	if outPath == "" {
		ext := path.Ext(p.Source)
		outPath = p.Source[:len(p.Source)-len(ext)] + ".md"
	}

	// 6) 写入 sandbox
	if err := ctx.Host.SandboxWrite(outPath, []byte(mdBuilder.String())); err != nil {
		return nil, fmt.Errorf("写入 MD 文件失败: %w", err)
	}

	return map[string]interface{}{
		"output_path":      outPath,
		"sheets_processed": processed,
		"sheet_names":      sheetNames,
	}, nil
}

// =========================================================================
// XLSX 解析引擎（无外部 XML 库依赖，纯手工字符串解析）
// =========================================================================

type sheetInfo struct {
	Name    string
	SheetID int
	RID     string
	Path    string
}

type xlsxReader struct {
	files    map[string][]byte
	sheetMap map[string]sheetInfo
}

func newXLSXReader(data []byte) (*xlsxReader, error) {
	files, err := readZipFiles(data)
	if err != nil {
		return nil, fmt.Errorf("创建 zip 读取器失败: %w", err)
	}
	r := &xlsxReader{files: files}
	r.parseWorkbook()
	return r, nil
}

// =========================================================================
// Minimal ZIP reader — avoids archive/zip (incompatible with TinyGo + Go 1.26)
// =========================================================================

const (
	zipLocalFileHeaderSig = 0x04034b50
	zipCentralDirSig      = 0x02014b50
	zipEndCentralDirSig   = 0x06054b50
	zipLocalFileHeaderLen = 30
	zipCentralDirEntryLen = 46
	zipEndCentralDirLen   = 22
)

func readZipFiles(data []byte) (map[string][]byte, error) {
	files := make(map[string][]byte)

	// Find End of Central Directory record (scan from end)
	eocdOffset := findEOCD(data)
	if eocdOffset < 0 {
		return nil, fmt.Errorf("zip: not a valid zip file")
	}

	// Parse EOCD
	eocd := data[eocdOffset:]
	if len(eocd) < zipEndCentralDirLen {
		return nil, fmt.Errorf("zip: not a valid zip file")
	}
	cdSize := int(binary.LittleEndian.Uint32(eocd[12:16]))
	cdOffset := int(binary.LittleEndian.Uint32(eocd[16:20]))

	// Parse Central Directory entries
	pos := cdOffset
	end := cdOffset + cdSize
	for pos < end && pos < len(data) {
		if pos+4 > len(data) {
			break
		}
		sig := binary.LittleEndian.Uint32(data[pos : pos+4])
		if sig != zipCentralDirSig {
			break
		}
		if pos+zipCentralDirEntryLen > len(data) {
			break
		}

		method := binary.LittleEndian.Uint16(data[pos+10 : pos+12])
		compSize := binary.LittleEndian.Uint32(data[pos+20 : pos+24])
		uncompSize := binary.LittleEndian.Uint32(data[pos+24 : pos+28])
		nameLen := int(binary.LittleEndian.Uint16(data[pos+28 : pos+30]))
		extraLen := int(binary.LittleEndian.Uint16(data[pos+30 : pos+32]))
		commentLen := int(binary.LittleEndian.Uint16(data[pos+32 : pos+34]))
		localOffset := int(binary.LittleEndian.Uint32(data[pos+42 : pos+46]))

		nameStart := pos + zipCentralDirEntryLen
		if nameStart+nameLen > len(data) {
			break
		}
		name := string(data[nameStart : nameStart+nameLen])

		// Read from local file header
		if localOffset+zipLocalFileHeaderLen > len(data) {
			pos = nameStart + nameLen + extraLen + commentLen
			continue
		}
		localNameLen := int(binary.LittleEndian.Uint16(data[localOffset+26 : localOffset+28]))
		localExtraLen := int(binary.LittleEndian.Uint16(data[localOffset+28 : localOffset+30]))
		dataStart := localOffset + zipLocalFileHeaderLen + localNameLen + localExtraLen

		if dataStart+int(compSize) > len(data) {
			pos = nameStart + nameLen + extraLen + commentLen
			continue
		}

		compData := data[dataStart : dataStart+int(compSize)]
		var content []byte
		if method == 0 { // Stored (no compression)
			content = compData
		} else if method == 8 { // Deflate
			content = inflateData(compData)
		} else {
			// Unsupported method, skip
			pos = nameStart + nameLen + extraLen + commentLen
			continue
		}

		// Verify uncompressed size if known
		if uncompSize > 0 && uint32(len(content)) != uncompSize {
			// Size mismatch — still keep the data, might work
		}

		files[name] = content
		pos = nameStart + nameLen + extraLen + commentLen
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("zip: not a valid zip file")
	}
	return files, nil
}

func findEOCD(data []byte) int {
	// EOCD signature is 22 bytes, search from end
	minLen := zipEndCentralDirLen
	if len(data) < minLen {
		return -1
	}
	// Search backwards, allowing up to 64KB comment
	searchLen := len(data)
	if searchLen > 65536+minLen {
		searchLen = 65536 + minLen
	}
	for i := len(data) - minLen; i >= len(data)-searchLen; i-- {
		if i < 0 {
			break
		}
		if binary.LittleEndian.Uint32(data[i:i+4]) == zipEndCentralDirSig {
			return i
		}
	}
	return -1
}

// inflateData decompresses DEFLATE data using compress/flate (avoids archive/zip).
func inflateData(compressed []byte) []byte {
	r := flate.NewReader(bytes.NewReader(compressed))
	defer r.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.Bytes()
}

func (r *xlsxReader) parseWorkbook() {
	r.sheetMap = make(map[string]sheetInfo)
	var availableSheets []string
	for f := range r.files {
		if strings.HasPrefix(f, "xl/worksheets/sheet") && strings.HasSuffix(f, ".xml") {
			availableSheets = append(availableSheets, f)
		}
	}
	ridToPath := r.readWorkbookRels()
	content, ok := r.files["xl/workbook.xml"]
	if !ok {
		return
	}
	contentStr := string(content)
	idx := 0
	for {
		sheetStart := strings.Index(contentStr[idx:], "<sheet ")
		if sheetStart == -1 {
			break
		}
		sheetStart += idx
		nameEnd, name := extractAttr(contentStr, sheetStart, "name")
		if name == "" {
			idx = sheetStart + 6
			continue
		}
		_, sheetID := extractIntAttr(contentStr, nameEnd, "sheetId")
		_, rid := extractAttr(contentStr, nameEnd, "r:id")

		path := ""
		if rid != "" && ridToPath[rid] != "" {
			path = ridToPath[rid]
			if _, ok := r.files[path]; !ok {
				path = ""
			}
		}
		if path == "" && rid != "" {
			sheetNum := strings.TrimPrefix(rid, "rId")
			path = "xl/worksheets/sheet" + sheetNum + ".xml"
			if _, ok := r.files[path]; !ok {
				path = ""
			}
		}
		if path == "" && sheetID > 0 {
			path = fmt.Sprintf("xl/worksheets/sheet%d.xml", sheetID)
			if _, ok := r.files[path]; !ok {
				path = ""
			}
		}
		if path == "" {
			used := make(map[string]bool)
			for _, info := range r.sheetMap {
				used[info.Path] = true
			}
			for _, p := range availableSheets {
				if !used[p] {
					path = p
					break
				}
			}
		}
		r.sheetMap[name] = sheetInfo{Name: name, SheetID: sheetID, RID: rid, Path: path}
		idx = nameEnd + 1
	}
}

func (r *xlsxReader) readWorkbookRels() map[string]string {
	m := make(map[string]string)
	content, ok := r.files["xl/_rels/workbook.xml.rels"]
	if !ok {
		return m
	}
	s := string(content)
	idx := 0
	for {
		start := strings.Index(s[idx:], "<Relationship ")
		if start == -1 {
			break
		}
		start += idx
		_, rid := extractAttr(s, start, "Id")
		end, target := extractAttr(s, start, "Target")
		if strings.HasPrefix(target, "worksheets/") {
			m[rid] = "xl/" + target
		}
		idx = end
	}
	return m
}

func (r *xlsxReader) getSheetNames() ([]string, error) {
	var names []string
	for name := range r.sheetMap {
		names = append(names, name)
	}
	return names, nil
}

func (r *xlsxReader) getSheetData(sheetName string) ([][]string, error) {
	info, ok := r.sheetMap[sheetName]
	if !ok {
		return nil, fmt.Errorf("找不到工作表: %s", sheetName)
	}
	if info.Path == "" {
		return nil, fmt.Errorf("无法确定工作表 %s 的文件路径", sheetName)
	}
	content, ok := r.files[info.Path]
	if !ok {
		return nil, fmt.Errorf("找不到工作表文件: %s", info.Path)
	}
	shared := r.loadSharedStrings()
	return parseSheetXML(string(content), shared), nil
}

func (r *xlsxReader) loadSharedStrings() []string {
	content, ok := r.files["xl/sharedStrings.xml"]
	if !ok {
		return nil
	}
	s := string(content)
	var ss []string
	siIdx := 0
	for {
		siStart := strings.Index(s[siIdx:], "<si>")
		if siStart == -1 {
			break
		}
		siStart += siIdx
		siEnd := strings.Index(s[siStart:], "</si>")
		if siEnd == -1 {
			break
		}
		siEnd += siStart
		siContent := s[siStart+4 : siEnd]
		var text strings.Builder
		tIdx := 0
		for {
			tStart := strings.Index(siContent[tIdx:], "<t")
			if tStart == -1 {
				break
			}
			tStart += tIdx
			tClose := strings.Index(siContent[tStart:], ">")
			if tClose == -1 {
				break
			}
			tClose += tStart
			tEnd := strings.Index(siContent[tClose:], "</t>")
			if tEnd == -1 {
				break
			}
			tEnd += tClose
			text.WriteString(siContent[tClose+1 : tEnd])
			tIdx = tEnd + 4
		}
		ss = append(ss, text.String())
		siIdx = siEnd + 5
	}
	return ss
}

func parseSheetXML(contentStr string, shared []string) [][]string {
	sdStart := strings.Index(contentStr, "<sheetData>")
	if sdStart == -1 {
		return nil
	}
	sdEnd := strings.Index(contentStr[sdStart:], "</sheetData>")
	if sdEnd == -1 {
		return nil
	}
	sdEnd += sdStart
	sheetData := contentStr[sdStart+11 : sdEnd]

	var result [][]string
	maxCol := 0
	rowIdx := 0

	for {
		rowStart := strings.Index(sheetData[rowIdx:], "<row ")
		if rowStart == -1 {
			break
		}
		rowStart += rowIdx
		rowEnd := strings.Index(sheetData[rowStart:], "</row>")
		if rowEnd == -1 {
			break
		}
		rowEnd += rowStart
		rowContent := sheetData[rowStart : rowEnd+6]

		rowNum := len(result) + 1
		if _, rn := extractIntAttr(rowContent, 0, "r"); rn > 0 {
			for len(result) < rn-1 {
				result = append(result, []string{})
			}
			rowNum = rn
		}

		rowData := []string{}
		cellIdx := 0
		for {
			cellStart := strings.Index(rowContent[cellIdx:], "<c ")
			if cellStart == -1 {
				break
			}
			cellStart += cellIdx
			cellEnd := strings.Index(rowContent[cellStart:], "</c>")
			if cellEnd == -1 {
				break
			}
			cellEnd += cellStart
			cellContent := rowContent[cellStart : cellEnd+4]

			_, cellRef := extractAttr(cellContent, 0, "r")
			if cellRef == "" {
				cellIdx = cellEnd + 4
				continue
			}
			colIndex := colIndexFromRef(cellRef)

			_, cellType := extractAttr(cellContent, 0, "t")

			val := ""
			vStart := strings.Index(cellContent, "<v>")
			if vStart != -1 {
				vStart += 3
				vEnd := strings.Index(cellContent[vStart:], "</v>")
				if vEnd != -1 {
					val = cellContent[vStart : vStart+vEnd]
					if cellType == "s" && shared != nil {
						if idx, err := strconv.Atoi(val); err == nil && idx >= 0 && idx < len(shared) {
							val = shared[idx]
						}
					}
				}
			}
			for len(rowData) <= colIndex {
				rowData = append(rowData, "")
			}
			rowData[colIndex] = val
			if colIndex > maxCol {
				maxCol = colIndex
			}
			cellIdx = cellEnd + 4
		}
		for len(result) < rowNum {
			result = append(result, []string{})
		}
		result[rowNum-1] = rowData
		rowIdx = rowEnd + 6
	}

	// normalize column count
	for i := range result {
		for len(result[i]) <= maxCol {
			result[i] = append(result[i], "")
		}
	}
	return result
}

func colIndexFromRef(ref string) int {
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		i++
	}
	col := 0
	for _, c := range ref[:i] {
		col = col*26 + int(c-'A') + 1
	}
	return col - 1
}

// =========================================================================
// Markdown 生成
// =========================================================================

func convertToMarkdown(data [][]string) string {
	if len(data) == 0 {
		return "（空表格）\n"
	}

	numCols := 0
	for _, row := range data {
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	var b strings.Builder
	for rowIdx, row := range data {
		b.WriteString("|")
		for colIdx := 0; colIdx < numCols; colIdx++ {
			val := ""
			if colIdx < len(row) {
				val = escapeMD(row[colIdx])
			}
			b.WriteString(fmt.Sprintf(" %s |", val))
		}
		b.WriteString("\n")
		if rowIdx == 0 {
			b.WriteString("|")
			for colIdx := 0; colIdx < numCols; colIdx++ {
				b.WriteString(" --- |")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func escapeMD(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	text = strings.ReplaceAll(text, "#", "\\#")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "_", "\\_")
	return text
}

// =========================================================================
// XML 属性提取辅助函数（无外部依赖）
// =========================================================================

func extractAttr(s string, offset int, name string) (endIdx int, value string) {
	search := name + `="`
	pos := strings.Index(s[offset:], search)
	if pos == -1 {
		return offset, ""
	}
	pos += offset + len(search)
	close := strings.Index(s[pos:], `"`)
	if close == -1 {
		return pos, ""
	}
	return pos + close, s[pos : pos+close]
}

func extractIntAttr(s string, offset int, name string) (endIdx int, value int) {
	endIdx, strVal := extractAttr(s, offset, name)
	if strVal == "" {
		return endIdx, 0
	}
	v, err := strconv.Atoi(strVal)
	if err != nil {
		return endIdx, 0
	}
	return endIdx, v
}

// extractJSONString extracts a string value from a JSON object byte slice.
// It searches for "key":"value" and returns the raw string value (without quotes).
// This avoids TinyGo's json.Unmarshal which corrupts UTF-8 Chinese characters.
func extractJSONString(data []byte, key string) string {
	keyBytes := []byte(`"` + key + `":"`)
	for i := 0; i <= len(data)-len(keyBytes); i++ {
		match := true
		for j := 0; j < len(keyBytes); j++ {
			if data[i+j] != keyBytes[j] {
				match = false
				break
			}
		}
		if match {
			start := i + len(keyBytes)
			end := start
			for end < len(data) && data[end] != '"' {
				end++
			}
			if end > start {
				return string(data[start:end])
			}
			break
		}
	}
	return ""
}

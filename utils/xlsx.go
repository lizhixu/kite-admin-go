package utils

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type XLSXSheet struct {
	Name    string
	Headers []string
	Rows    [][]string
}

func NewXLSX(sheet XLSXSheet) ([]byte, error) {
	if sheet.Name == "" {
		sheet.Name = "Sheet1"
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        contentTypesXML,
		"_rels/.rels":                rootRelsXML,
		"xl/workbook.xml":            workbookXML(sheet.Name),
		"xl/_rels/workbook.xml.rels": workbookRelsXML,
		"xl/styles.xml":              stylesXML,
		"docProps/core.xml":          coreXML,
		"docProps/app.xml":           appXML,
		"xl/worksheets/sheet1.xml":   worksheetXML(sheet.Headers, sheet.Rows),
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ReadXLSX(r io.ReaderAt, size int64) ([][]string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}

	sharedStrings, err := readSharedStrings(zr)
	if err != nil {
		return nil, err
	}

	var sheetFile *zip.File
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheetFile = f
			break
		}
	}
	if sheetFile == nil {
		return nil, fmt.Errorf("sheet1 not found")
	}

	rc, err := sheetFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var sheet worksheet
	if err := xml.NewDecoder(rc).Decode(&sheet); err != nil {
		return nil, err
	}

	rows := make([][]string, 0, len(sheet.SheetData.Rows))
	for _, row := range sheet.SheetData.Rows {
		values := make([]string, 0, len(row.Cells))
		for _, cell := range row.Cells {
			idx := columnIndex(cell.Ref)
			for len(values) < idx {
				values = append(values, "")
			}
			values = append(values, cellValue(cell, sharedStrings))
		}
		rows = append(rows, values)
	}
	return rows, nil
}

func worksheetXML(headers []string, rows [][]string) string {
	allRows := make([][]string, 0, len(rows)+1)
	allRows = append(allRows, headers)
	allRows = append(allRows, rows...)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	b.WriteString(`<sheetData>`)
	for rowIdx, row := range allRows {
		r := rowIdx + 1
		b.WriteString(`<row r="` + strconv.Itoa(r) + `">`)
		for colIdx, value := range row {
			ref := columnName(colIdx+1) + strconv.Itoa(r)
			b.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t>`)
			xml.EscapeText(&b, []byte(value))
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	for _, f := range zr.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		var sst sharedStringTable
		if err := xml.NewDecoder(rc).Decode(&sst); err != nil {
			return nil, err
		}
		values := make([]string, 0, len(sst.Items))
		for _, item := range sst.Items {
			values = append(values, item.Text())
		}
		return values, nil
	}
	return nil, nil
}

func cellValue(cell xlsxCell, sharedStrings []string) string {
	switch cell.Type {
	case "inlineStr":
		return strings.TrimSpace(cell.InlineString.Text())
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err != nil || i < 0 || i >= len(sharedStrings) {
			return ""
		}
		return strings.TrimSpace(sharedStrings[i])
	case "b":
		if strings.TrimSpace(cell.Value) == "1" {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(cell.Value)
	}
}

func columnIndex(ref string) int {
	idx := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		idx = idx*26 + int(r-'A'+1)
	}
	if idx < 1 {
		return 1
	}
	return idx
}

func columnName(index int) string {
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

type worksheet struct {
	SheetData sheetData `xml:"sheetData"`
}

type sheetData struct {
	Rows []xlsxRow `xml:"row"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref          string       `xml:"r,attr"`
	Type         string       `xml:"t,attr"`
	Value        string       `xml:"v"`
	InlineString inlineString `xml:"is"`
}

type inlineString struct {
	T string `xml:"t"`
}

func (s inlineString) Text() string {
	return s.T
}

type sharedStringTable struct {
	Items []sharedStringItem `xml:"si"`
}

type sharedStringItem struct {
	T    string      `xml:"t"`
	Runs []sharedRun `xml:"r"`
}

func (s sharedStringItem) Text() string {
	if s.T != "" {
		return s.T
	}
	var b strings.Builder
	for _, run := range s.Runs {
		b.WriteString(run.T)
	}
	return b.String()
}

type sharedRun struct {
	T string `xml:"t"`
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="1"><fill><patternFill patternType="none"/></fill></fills>
  <borders count="1"><border/></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
</styleSheet>`

const coreXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:creator>Kite Admin</dc:creator>
</cp:coreProperties>`

const appXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Application>Kite Admin</Application>
</Properties>`

func workbookXML(sheetName string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="`)
	xml.EscapeText(&b, []byte(sheetName))
	b.WriteString(`" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	return b.String()
}

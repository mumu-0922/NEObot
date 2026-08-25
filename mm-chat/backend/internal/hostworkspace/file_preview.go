package hostworkspace

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxPreviewArchiveEntries = 2048
	maxPreviewXMLBytes       = 16 << 20
	maxPreviewTotalXMLBytes  = 32 << 20
	maxPreviewTextBytes      = 512 << 10
	maxPreviewSheets         = 20
	maxPreviewRows           = 200
	maxPreviewColumns        = 50
	maxPreviewCellBytes      = 4096
	maxPreviewSharedStrings  = 100000
)

type workspaceFilePreview struct {
	Kind      string                  `json:"kind"`
	FileName  string                  `json:"fileName"`
	MimeType  string                  `json:"mimeType"`
	Size      int64                   `json:"size"`
	Version   string                  `json:"version"`
	Truncated bool                    `json:"truncated"`
	Text      string                  `json:"text,omitempty"`
	Sheets    []workspaceSheetPreview `json:"sheets,omitempty"`
}

type workspaceSheetPreview struct {
	Name      string     `json:"name"`
	Rows      [][]string `json:"rows"`
	Truncated bool       `json:"truncated"`
}

func buildWorkspaceFilePreview(snapshot WorkspaceFileSnapshot) (workspaceFilePreview, error) {
	preview := workspaceFilePreview{
		Kind: "unsupported", FileName: snapshot.FileName, MimeType: snapshot.MimeType,
		Size: int64(len(snapshot.Body)), Version: snapshot.Version,
	}
	extension := strings.ToLower(filepath.Ext(snapshot.FileName))
	switch {
	case extension == ".xlsx":
		sheets, truncated, err := previewXLSX(snapshot.Body)
		if err != nil {
			return workspaceFilePreview{}, err
		}
		preview.Kind, preview.Sheets, preview.Truncated = "xlsx", sheets, truncated
	case extension == ".docx":
		text, truncated, err := previewDOCX(snapshot.Body)
		if err != nil {
			return workspaceFilePreview{}, err
		}
		preview.Kind, preview.Text, preview.Truncated = "docx", text, truncated
	case workspacePreviewTextFile(extension, snapshot.MimeType):
		if !utf8.Valid(snapshot.Body) || bytes.IndexByte(snapshot.Body, 0) >= 0 {
			return workspaceFilePreview{}, errors.New("workspace preview text is invalid")
		}
		preview.Kind = "text"
		body := snapshot.Body
		if len(body) > maxPreviewTextBytes {
			body, preview.Truncated = body[:maxPreviewTextBytes], true
			for !utf8.Valid(body) && len(body) > 0 {
				body = body[:len(body)-1]
			}
		}
		preview.Text = string(body)
	}
	return preview, nil
}

func workspacePreviewTextFile(extension string, mimeType string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "text/") {
		return true
	}
	switch extension {
	case ".md", ".markdown", ".txt", ".csv", ".tsv", ".json", ".jsonl",
		".xml", ".yaml", ".yml", ".toml", ".ini", ".log", ".sql",
		".go", ".py", ".rs", ".js", ".jsx", ".ts", ".tsx", ".css",
		".html", ".sh", ".bash", ".zsh", ".ps1":
		return true
	default:
		return false
	}
}

func openPreviewArchive(body []byte) (*zip.Reader, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil || len(archive.File) > maxPreviewArchiveEntries {
		return nil, errors.New("workspace preview archive is invalid")
	}
	return archive, nil
}

func previewXLSX(body []byte) ([]workspaceSheetPreview, bool, error) {
	archive, err := openPreviewArchive(body)
	if err != nil {
		return nil, false, err
	}
	worksheets := previewArchiveEntries(archive, "xl/worksheets/sheet", ".xml")
	if len(worksheets) == 0 || !previewEntriesBounded(worksheets) {
		return nil, false, errors.New("workspace workbook has no readable sheets")
	}
	shared, err := previewSharedStrings(archive)
	if err != nil {
		return nil, false, err
	}
	names, _ := previewWorkbookSheetNames(archive)
	sort.Slice(worksheets, func(left, right int) bool {
		return previewSheetNumber(worksheets[left].Name) < previewSheetNumber(worksheets[right].Name)
	})
	truncated := len(worksheets) > maxPreviewSheets
	if len(worksheets) > maxPreviewSheets {
		worksheets = worksheets[:maxPreviewSheets]
	}
	sheets := make([]workspaceSheetPreview, 0, len(worksheets))
	for index, entry := range worksheets {
		rows, sheetTruncated, parseErr := previewWorksheet(entry, shared)
		if parseErr != nil {
			return nil, false, parseErr
		}
		name := "Sheet " + strconv.Itoa(index+1)
		if index < len(names) && strings.TrimSpace(names[index]) != "" {
			name = truncatePreviewValue(names[index])
		}
		sheets = append(sheets, workspaceSheetPreview{
			Name: name, Rows: rows, Truncated: sheetTruncated,
		})
		truncated = truncated || sheetTruncated
	}
	return sheets, truncated, nil
}

func previewWorkbookSheetNames(archive *zip.Reader) ([]string, error) {
	entry := previewArchiveEntry(archive, "xl/workbook.xml")
	if entry == nil || entry.UncompressedSize64 > maxPreviewXMLBytes {
		return nil, errors.New("workspace workbook metadata is invalid")
	}
	decoder, closeReader, err := previewXMLDecoder(entry)
	if err != nil {
		return nil, err
	}
	defer closeReader()
	names := make([]string, 0)
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			return names, nil
		}
		if tokenErr != nil {
			return nil, tokenErr
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "name" {
				names = append(names, attribute.Value)
				break
			}
		}
	}
}

func previewSharedStrings(archive *zip.Reader) ([]string, error) {
	entry := previewArchiveEntry(archive, "xl/sharedStrings.xml")
	if entry == nil {
		return nil, nil
	}
	if entry.UncompressedSize64 > maxPreviewXMLBytes {
		return nil, errors.New("workspace shared strings are too large")
	}
	decoder, closeReader, err := previewXMLDecoder(entry)
	if err != nil {
		return nil, err
	}
	defer closeReader()
	values := make([]string, 0)
	var current strings.Builder
	inItem, inText := false, false
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			return values, nil
		}
		if tokenErr != nil {
			return nil, tokenErr
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "si":
				if len(values) >= maxPreviewSharedStrings {
					return nil, errors.New("workspace shared strings are too complex")
				}
				inItem = true
				current.Reset()
			case "t":
				inText = inItem
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "si":
				values = append(values, truncatePreviewValue(current.String()))
				inItem = false
			}
		case xml.CharData:
			if inText && current.Len() <= maxPreviewCellBytes {
				current.Write(value)
			}
		}
	}
}

func previewWorksheet(entry *zip.File, shared []string) ([][]string, bool, error) {
	if entry.UncompressedSize64 > maxPreviewXMLBytes {
		return nil, false, errors.New("workspace worksheet is too large")
	}
	decoder, closeReader, err := previewXMLDecoder(entry)
	if err != nil {
		return nil, false, err
	}
	defer closeReader()
	rows := make([][]string, 0)
	currentRow := make([]string, 0)
	cellType, cellReference := "", ""
	inCell, inValue, inInlineText := false, false, false
	var cellValue strings.Builder
	truncated := false
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			return rows, truncated, nil
		}
		if tokenErr != nil {
			return nil, false, tokenErr
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "row":
				currentRow = make([]string, 0)
			case "c":
				inCell, cellType, cellReference = true, "", ""
				cellValue.Reset()
				for _, attribute := range value.Attr {
					switch attribute.Name.Local {
					case "t":
						cellType = attribute.Value
					case "r":
						cellReference = attribute.Value
					}
				}
			case "v":
				inValue = inCell
			case "t":
				inInlineText = inCell
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "v":
				inValue = false
			case "t":
				inInlineText = false
			case "c":
				column := previewCellColumn(cellReference)
				if column < 0 {
					column = len(currentRow)
				}
				if column >= maxPreviewColumns {
					truncated = true
				} else {
					for len(currentRow) <= column {
						currentRow = append(currentRow, "")
					}
					currentRow[column] = previewCellValue(cellType, cellValue.String(), shared)
				}
				inCell = false
			case "row":
				if len(rows) < maxPreviewRows {
					rows = append(rows, currentRow)
				} else {
					truncated = true
				}
			}
		case xml.CharData:
			if (inValue || inInlineText) && cellValue.Len() <= maxPreviewCellBytes {
				cellValue.Write(value)
			}
		}
	}
}

func previewCellValue(cellType string, value string, shared []string) string {
	value = truncatePreviewValue(value)
	if cellType == "s" {
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && index >= 0 && index < len(shared) {
			return shared[index]
		}
	}
	if cellType == "b" {
		if strings.TrimSpace(value) == "1" {
			return "TRUE"
		}
		return "FALSE"
	}
	return value
}

func previewCellColumn(reference string) int {
	column := 0
	letters := 0
	for _, character := range strings.ToUpper(strings.TrimSpace(reference)) {
		if character < 'A' || character > 'Z' {
			break
		}
		column = column*26 + int(character-'A'+1)
		letters++
	}
	if letters == 0 {
		return -1
	}
	return column - 1
}

func previewDOCX(body []byte) (string, bool, error) {
	archive, err := openPreviewArchive(body)
	if err != nil {
		return "", false, err
	}
	entry := previewArchiveEntry(archive, "word/document.xml")
	if entry == nil || entry.UncompressedSize64 > maxPreviewXMLBytes {
		return "", false, errors.New("workspace document is invalid")
	}
	decoder, closeReader, err := previewXMLDecoder(entry)
	if err != nil {
		return "", false, err
	}
	defer closeReader()
	var output strings.Builder
	inText, truncated := false, false
	for output.Len() < maxPreviewTextBytes {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			return output.String(), truncated, nil
		}
		if tokenErr != nil {
			return "", false, tokenErr
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "p":
				output.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				remaining := maxPreviewTextBytes - output.Len()
				if len(value) > remaining {
					output.Write(value[:remaining])
					truncated = true
				} else {
					output.Write(value)
				}
			}
		}
	}
	return output.String(), true, nil
}

func previewArchiveEntry(archive *zip.Reader, name string) *zip.File {
	for _, entry := range archive.File {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func previewArchiveEntries(archive *zip.Reader, prefix string, suffix string) []*zip.File {
	entries := make([]*zip.File, 0)
	for _, entry := range archive.File {
		if strings.HasPrefix(entry.Name, prefix) && strings.HasSuffix(entry.Name, suffix) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func previewEntriesBounded(entries []*zip.File) bool {
	var total uint64
	for _, entry := range entries {
		if entry.UncompressedSize64 > maxPreviewXMLBytes {
			return false
		}
		total += entry.UncompressedSize64
		if total > maxPreviewTotalXMLBytes {
			return false
		}
	}
	return true
}

func previewXMLDecoder(entry *zip.File) (*xml.Decoder, func(), error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, nil, err
	}
	return xml.NewDecoder(io.LimitReader(reader, maxPreviewXMLBytes+1)), func() {
		_ = reader.Close()
	}, nil
}

func previewSheetNumber(name string) int {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	index := len(base)
	for index > 0 && base[index-1] >= '0' && base[index-1] <= '9' {
		index--
	}
	value, err := strconv.Atoi(base[index:])
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return value
}

func truncatePreviewValue(value string) string {
	if len(value) <= maxPreviewCellBytes {
		return value
	}
	value = value[:maxPreviewCellBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

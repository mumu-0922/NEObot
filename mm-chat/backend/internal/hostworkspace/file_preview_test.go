package hostworkspace

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestBuildWorkspaceFilePreviewReadsBasicXLSXSheetsAndValues(t *testing.T) {
	body := buildPreviewArchive(t, map[string]string{
		"xl/workbook.xml":      `<workbook><sheets><sheet name="Prices"/></sheets></workbook>`,
		"xl/sharedStrings.xml": `<sst><si><t>Date</t></si><si><t>Price</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
			`<row r="2"><c r="A2" t="inlineStr"><is><t>2026-08-25</t></is></c><c r="B2"><v>4610</v></c></row>` +
			`</sheetData></worksheet>`,
	})
	preview, err := buildWorkspaceFilePreview(WorkspaceFileSnapshot{
		Path: "reports/prices.xlsx", FileName: "prices.xlsx",
		MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Body:     body, Version: "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "xlsx" || len(preview.Sheets) != 1 ||
		preview.Sheets[0].Name != "Prices" || len(preview.Sheets[0].Rows) != 2 ||
		preview.Sheets[0].Rows[0][0] != "Date" ||
		preview.Sheets[0].Rows[1][1] != "4610" {
		t.Fatalf("preview=%#v", preview)
	}
}

func TestBuildWorkspaceFilePreviewRejectsOversizedWorksheet(t *testing.T) {
	body := buildPreviewArchive(t, map[string]string{
		"xl/workbook.xml":          `<workbook><sheets><sheet name="Too large"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": strings.Repeat("x", maxPreviewXMLBytes+1),
	})
	if _, err := buildWorkspaceFilePreview(WorkspaceFileSnapshot{
		FileName: "large.xlsx", Body: body,
	}); err == nil {
		t.Fatal("oversized worksheet was accepted")
	}
}

func buildPreviewArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func agentWorkspaceArtifact(path string, body []byte, version string) localskills.WorkspaceArtifactSnapshot {
	return localskills.WorkspaceArtifactSnapshot{Path: path, Body: body, Version: version}
}

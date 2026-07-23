package chat

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func extractDOCXAttachmentText(
	attachment ProviderAttachment,
) (extractedAttachmentText, error) {
	archive, err := openOfficeArchive(attachment)
	if err != nil {
		return extractedAttachmentText{}, err
	}
	entry := officeEntryByName(archive, "word/document.xml")
	if entry == nil {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	if err := validateOfficeXMLEntries(attachment, []*zip.File{entry}); err != nil {
		return extractedAttachmentText{}, err
	}

	collector := newAttachmentTextCollector(maxDirectAttachmentContentChars + 1)
	if err := collectXMLText(entry, collector, func(name xml.Name) xmlTextAction {
		switch name.Local {
		case "t":
			return xmlTextCapture
		case "tab":
			return xmlTextTab
		case "br", "cr", "p":
			return xmlTextNewline
		default:
			return xmlTextIgnore
		}
	}); err != nil {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	return boundedExtractedText(collector.String(), collector.Truncated(), attachment)
}

func extractPPTXAttachmentText(
	attachment ProviderAttachment,
) (extractedAttachmentText, error) {
	archive, err := openOfficeArchive(attachment)
	if err != nil {
		return extractedAttachmentText{}, err
	}
	entries := officeEntriesWithPrefix(archive, "ppt/slides/slide", ".xml")
	if len(entries) == 0 {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	if err := validateOfficeXMLEntries(attachment, entries); err != nil {
		return extractedAttachmentText{}, err
	}
	sort.Slice(entries, func(i int, j int) bool {
		return numberedOfficeEntryLess(entries[i].Name, entries[j].Name)
	})

	collector := newAttachmentTextCollector(maxDirectAttachmentContentChars + 1)
	for slideIndex, entry := range entries {
		if collector.Full() {
			break
		}
		if slideIndex > 0 {
			collector.Append("\n")
		}
		if err := collectXMLText(entry, collector, func(name xml.Name) xmlTextAction {
			switch name.Local {
			case "t":
				return xmlTextCapture
			case "br", "p":
				return xmlTextNewline
			default:
				return xmlTextIgnore
			}
		}); err != nil {
			return extractedAttachmentText{}, attachmentParseFailed(attachment)
		}
	}
	return boundedExtractedText(collector.String(), collector.Truncated(), attachment)
}

func extractXLSXAttachmentText(
	attachment ProviderAttachment,
) (extractedAttachmentText, error) {
	archive, err := openOfficeArchive(attachment)
	if err != nil {
		return extractedAttachmentText{}, err
	}
	worksheets := officeEntriesWithPrefix(archive, "xl/worksheets/sheet", ".xml")
	if len(worksheets) == 0 {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	relevantEntries := append([]*zip.File(nil), worksheets...)
	if sharedStringsEntry := officeEntryByName(archive, "xl/sharedStrings.xml"); sharedStringsEntry != nil {
		relevantEntries = append(relevantEntries, sharedStringsEntry)
	}
	if err := validateOfficeXMLEntries(attachment, relevantEntries); err != nil {
		return extractedAttachmentText{}, err
	}
	sharedStrings, err := readXLSXSharedStrings(archive)
	if err != nil {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	sort.Slice(worksheets, func(i int, j int) bool {
		return numberedOfficeEntryLess(worksheets[i].Name, worksheets[j].Name)
	})

	collector := newAttachmentTextCollector(maxDirectAttachmentContentChars + 1)
	for sheetIndex, worksheet := range worksheets {
		if collector.Full() {
			break
		}
		if sheetIndex > 0 {
			collector.Append("\n")
		}
		if err := collectXLSXWorksheet(worksheet, sharedStrings, collector); err != nil {
			return extractedAttachmentText{}, attachmentParseFailed(attachment)
		}
	}
	return boundedExtractedText(collector.String(), collector.Truncated(), attachment)
}

func openOfficeArchive(attachment ProviderAttachment) (*zip.Reader, error) {
	archive, err := zip.NewReader(bytes.NewReader(attachment.Data), int64(len(attachment.Data)))
	if err != nil {
		return nil, attachmentParseFailed(attachment)
	}
	if len(archive.File) > maxOfficeArchiveEntries {
		return nil, attachmentTooComplex(attachment)
	}
	return archive, nil
}

func validateOfficeXMLEntries(
	attachment ProviderAttachment,
	entries []*zip.File,
) error {
	var total uint64
	for _, entry := range entries {
		if entry.UncompressedSize64 > uint64(maxOfficeXMLUncompressedBytes) {
			return attachmentTooComplex(attachment)
		}
		total += entry.UncompressedSize64
		if total > uint64(maxOfficeTotalUncompressedBytes) {
			return attachmentTooComplex(attachment)
		}
	}
	return nil
}

func attachmentTooComplex(attachment ProviderAttachment) error {
	return newValidationError(
		"ATTACHMENT_TOO_COMPLEX",
		fmt.Sprintf(
			"attachment %q expands beyond the direct parser limit",
			attachmentDisplayName(attachment),
		),
	)
}

func officeEntryByName(archive *zip.Reader, name string) *zip.File {
	for _, entry := range archive.File {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func officeEntriesWithPrefix(
	archive *zip.Reader,
	prefix string,
	suffix string,
) []*zip.File {
	entries := make([]*zip.File, 0)
	for _, entry := range archive.File {
		if strings.HasPrefix(entry.Name, prefix) && strings.HasSuffix(entry.Name, suffix) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func numberedOfficeEntryLess(left string, right string) bool {
	leftNumber := trailingEntryNumber(left)
	rightNumber := trailingEntryNumber(right)
	if leftNumber == rightNumber {
		return left < right
	}
	return leftNumber < rightNumber
}

func trailingEntryNumber(name string) int {
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

type xmlTextAction uint8

const (
	xmlTextIgnore xmlTextAction = iota
	xmlTextCapture
	xmlTextNewline
	xmlTextTab
)

func collectXMLText(
	entry *zip.File,
	collector *attachmentTextCollector,
	actionFor func(xml.Name) xmlTextAction,
) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(io.LimitReader(reader, maxOfficeXMLUncompressedBytes+1))
	captureDepth := 0
	for !collector.Full() {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			action := actionFor(value.Name)
			switch action {
			case xmlTextCapture:
				captureDepth++
			case xmlTextNewline:
				collector.Append("\n")
			case xmlTextTab:
				collector.Append("\t")
			}
		case xml.EndElement:
			if actionFor(value.Name) == xmlTextCapture && captureDepth > 0 {
				captureDepth--
			}
		case xml.CharData:
			if captureDepth > 0 {
				collector.Append(string(value))
			}
		}
	}
	return nil
}

func readXLSXSharedStrings(archive *zip.Reader) ([]string, error) {
	entry := officeEntryByName(archive, "xl/sharedStrings.xml")
	if entry == nil {
		return nil, nil
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(io.LimitReader(reader, maxOfficeXMLUncompressedBytes+1))
	sharedStrings := make([]string, 0)
	var current strings.Builder
	inItem := false
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return sharedStrings, nil
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "si":
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
				sharedStrings = append(sharedStrings, current.String())
				inItem = false
			}
		case xml.CharData:
			if inText {
				current.Write(value)
			}
		}
	}
}

func collectXLSXWorksheet(
	entry *zip.File,
	sharedStrings []string,
	collector *attachmentTextCollector,
) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(io.LimitReader(reader, maxOfficeXMLUncompressedBytes+1))
	inCell := false
	inValue := false
	inInlineText := false
	cellType := ""
	var cellValue strings.Builder
	rowHasCell := false
	for !collector.Full() {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "row":
				rowHasCell = false
			case "c":
				inCell = true
				cellType = ""
				cellValue.Reset()
				for _, attribute := range value.Attr {
					if attribute.Name.Local == "t" {
						cellType = attribute.Value
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
				if rowHasCell {
					collector.Append("\t")
				}
				collector.Append(resolveXLSXCellValue(cellType, cellValue.String(), sharedStrings))
				rowHasCell = true
				inCell = false
			case "row":
				collector.Append("\n")
			}
		case xml.CharData:
			if inValue || inInlineText {
				cellValue.Write(value)
			}
		}
	}
	return nil
}

func resolveXLSXCellValue(cellType string, value string, sharedStrings []string) string {
	if cellType != "s" {
		return value
	}
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index < 0 || index >= len(sharedStrings) {
		return value
	}
	return sharedStrings[index]
}

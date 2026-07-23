package chat

import (
	"bytes"
	"fmt"

	pdf "github.com/ledongthuc/pdf"
)

func extractPDFAttachmentText(
	attachment ProviderAttachment,
) (result extractedAttachmentText, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = extractedAttachmentText{}
			err = newValidationError(
				"ATTACHMENT_PARSE_FAILED",
				fmt.Sprintf("attachment %q could not be parsed", attachmentDisplayName(attachment)),
			)
		}
	}()

	if len(attachment.Data) < 100 {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	reader, readErr := pdf.NewReader(bytes.NewReader(attachment.Data), int64(len(attachment.Data)))
	if readErr != nil {
		return extractedAttachmentText{}, attachmentParseFailed(attachment)
	}
	pageCount := reader.NumPage()
	if pageCount <= 0 {
		return extractedAttachmentText{}, attachmentContentEmpty(attachment)
	}
	if pageCount > maxDirectPDFPages {
		return extractedAttachmentText{}, newValidationError(
			"ATTACHMENT_TOO_COMPLEX",
			fmt.Sprintf(
				"attachment %q exceeds the %d-page direct-context limit",
				attachmentDisplayName(attachment),
				maxDirectPDFPages,
			),
		)
	}

	collector := newAttachmentTextCollector(maxDirectAttachmentContentChars + 1)
	fonts := make(map[string]*pdf.Font)
	for pageIndex := 1; pageIndex <= pageCount && !collector.Full(); pageIndex++ {
		page := reader.Page(pageIndex)
		for _, name := range page.Fonts() {
			if _, exists := fonts[name]; !exists {
				font := page.Font(name)
				fonts[name] = &font
			}
		}
		text, pageErr := page.GetPlainText(fonts)
		if pageErr != nil {
			return extractedAttachmentText{}, attachmentParseFailed(attachment)
		}
		if pageIndex > 1 {
			collector.Append("\n")
		}
		collector.Append(text)
	}

	return boundedExtractedText(collector.String(), collector.Truncated(), attachment)
}

package knowledge

import "testing"

func TestSelectParseProcessorRoutesPDFToMinerUAndNativeFormatsLocally(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested string
		mimeType  string
		want      string
	}{
		{name: "pdf", requested: automaticParseProcessor, mimeType: "application/pdf", want: "mineru"},
		{name: "docx", requested: automaticParseProcessor, mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", want: nativeParseProcessor},
		{name: "markdown", requested: automaticParseProcessor, mimeType: "text/markdown", want: nativeParseProcessor},
		{name: "explicit compatibility", requested: "mineru", mimeType: "application/pdf", want: "mineru"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := selectParseProcessor(test.requested, test.mimeType); got != test.want {
				t.Fatalf("selectParseProcessor(%q, %q) = %q, want %q", test.requested, test.mimeType, got, test.want)
			}
		})
	}
}

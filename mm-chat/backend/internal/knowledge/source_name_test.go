package knowledge

import "testing"

func TestQueryExplicitlyNamesSourceMirrorsMetadataRoutingKey(t *testing.T) {
	for _, test := range []struct {
		name       string
		query      string
		sourceName string
		want       bool
	}{
		{
			name:       "normalized basename",
			query:      "编号 RAGEVAL-XLSX-ZH-04 的例外代码是什么？",
			sourceName: "rag-eval-xlsx-zh-04.xlsx",
			want:       true,
		},
		{
			name:       "extension in query",
			query:      "Summarize quarterly-report.v2.pdf",
			sourceName: "quarterly-report.v2.pdf",
			want:       true,
		},
		{
			name:       "source not named",
			query:      "What is the approved capacity?",
			sourceName: "quarterly-report.pdf",
			want:       false,
		},
		{
			name:       "short generic basename",
			query:      "Read a.md",
			sourceName: "a.md",
			want:       false,
		},
		{
			name:       "invalid metadata",
			query:      "Read quarterly-report",
			sourceName: "quarterly-report.pdf\nshadow.txt",
			want:       false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := QueryExplicitlyNamesSource(
				test.query,
				test.sourceName,
			); got != test.want {
				t.Fatalf("QueryExplicitlyNamesSource() = %v, want %v", got, test.want)
			}
		})
	}
}

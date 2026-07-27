package voicejobs

import (
	"strings"
	"testing"
)

func TestProjectReadableTextMatchesRenderedWeatherAnswer(t *testing.T) {
	source := `## 西安天气｜7月26日

☀️ **当前晴，约 33°C**

- **今天：**晴间多云，**24～35°C**
- **体感：**炎热、湿度较高
- **明天：**晴间多云，**24～35°C**
- **未来几天：**最高气温约 **32～34°C**，7月28日、31日前后可能有局地雷阵雨。

<div style="padding:10px 14px;color:var(--hidden-token);">
<strong>出行提示：</strong>午后注意防暑、防晒和补水；长时间户外活动应避开高温时段，并随身携带雨具。
</div>

官方天气页面：中央气象台西安预报 ([m.nmc.cn](https://m.nmc.cn/hidden?q=1))｜中国天气网西安预报 ([weather.com.cn](https://weather.com.cn/hidden))

Sources: [W1] [W2] [W3]`

	want := strings.Join([]string{
		"西安天气｜7月26日",
		"☀️ 当前晴，约 33°C",
		"今天：晴间多云，24～35°C",
		"体感：炎热、湿度较高",
		"明天：晴间多云，24～35°C",
		"未来几天：最高气温约 32～34°C，7月28日、31日前后可能有局地雷阵雨。",
		"出行提示：午后注意防暑、防晒和补水；长时间户外活动应避开高温时段，并随身携带雨具。",
		"官方天气页面：中央气象台西安预报 (m.nmc.cn)｜中国天气网西安预报 (weather.com.cn)",
		"Sources: W1 W2 W3",
	}, "\n")

	if got := projectReadableText(source); got != want {
		t.Fatalf("projectReadableText() =\n%q\nwant\n%q", got, want)
	}
}

func TestProjectReadableTextCoversGFMHTMLCodeAndUnsafeContent(t *testing.T) {
	source := `# Heading &amp; escaped \*star\*

| Name | Value |
| --- | --- |
| **Alpha** | [label](https://hidden.example/path) |

- [x] visible task
- inline ` + "`a < b`" + `

` + "```go\nfmt.Println(\"visible code\")\n```" + `

![hidden alt](https://hidden.example/image.png)
<span style="display:none">hidden styled text</span>
<script>hiddenScript()</script><style>.hidden{display:none}</style>
<div>Visible <strong>HTML</strong><br>line</div>

<https://visible.example/path>`

	got := projectReadableText(source)
	for _, visible := range []string{
		"Heading & escaped *star*",
		"Name Value",
		"Alpha label",
		"visible task",
		"inline a < b",
		`fmt.Println("visible code")`,
		"Visible HTML",
		"line",
		"https://visible.example/path",
	} {
		if !strings.Contains(got, visible) {
			t.Errorf("projection %q is missing visible text %q", got, visible)
		}
	}
	for _, hidden := range []string{
		"https://hidden.example",
		"hidden alt",
		"hidden styled text",
		"hiddenScript",
		"display:none",
		"```",
		"**",
	} {
		if strings.Contains(got, hidden) {
			t.Errorf("projection %q contains hidden syntax %q", got, hidden)
		}
	}
}

func TestProjectReadableTextReturnsEmptyForMarkupOnlyInput(t *testing.T) {
	source := `<div style="display:none">hidden</div><!-- comment --><script>bad()</script>`
	if got := projectReadableText(source); got != "" {
		t.Fatalf("projectReadableText() = %q, want empty", got)
	}
}

func TestProjectReadableTextPreservesVisibleCodeAndEscapedDelimiters(t *testing.T) {
	source := "`cache__key` `**literal**` identifier cache__key, escaped \\*stars\\* and ~~deleted~~"
	want := "cache__key **literal** identifier cache__key, escaped *stars* and deleted"
	if got := projectReadableText(source); got != want {
		t.Fatalf("projectReadableText() = %q, want %q", got, want)
	}
}

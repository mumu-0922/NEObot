package strictjson

import (
	"strings"
	"testing"
)

type strictFixture struct {
	Name   string         `json:"name"`
	Target strictTarget   `json:"target"`
	Items  []strictTarget `json:"items"`
}

type strictTarget struct {
	ID string `json:"id"`
}

func TestDecodeRejectsAmbiguousJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"oversized", strings.Repeat(" ", 65)},
		{"root duplicate", `{"name":"a","name":"b","target":{"id":"1"},"items":[]}`},
		{"nested duplicate", `{"name":"a","target":{"id":"1","id":"2"},"items":[]}`},
		{"array duplicate", `{"name":"a","target":{"id":"1"},"items":[{"id":"1","id":"2"}]}`},
		{"unknown", `{"name":"a","target":{"id":"1"},"items":[],"extra":true}`},
		{"trailing", `{"name":"a","target":{"id":"1"},"items":[]} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var target strictFixture
			if err := Decode([]byte(test.body), 64, &target); err == nil {
				t.Fatal("ambiguous JSON unexpectedly decoded")
			}
		})
	}
}

func TestDecodeAcceptsOneStrictValue(t *testing.T) {
	var target strictFixture
	if err := Decode(
		[]byte(`{"name":"a","target":{"id":"1"},"items":[{"id":"2"}]}`),
		1024,
		&target,
	); err != nil || target.Name != "a" || target.Target.ID != "1" ||
		len(target.Items) != 1 || target.Items[0].ID != "2" {
		t.Fatalf("Decode() = %#v/%v", target, err)
	}
}

func TestRequireExactKeysCountsNullableFields(t *testing.T) {
	if err := RequireExactKeys(
		[]byte(`{"content":null,"importance":null}`),
		[]string{"content", "importance"},
	); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"content":null}`,
		`{"content":null,"importance":null,"extra":true}`,
	} {
		if err := RequireExactKeys(
			[]byte(body), []string{"content", "importance"},
		); err == nil {
			t.Fatalf("RequireExactKeys(%s) unexpectedly succeeded", body)
		}
	}
}

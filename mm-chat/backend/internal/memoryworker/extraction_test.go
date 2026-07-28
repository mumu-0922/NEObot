package memoryworker

import "testing"

func TestParseCandidatesRejectsSecretsAndVagueContext(t *testing.T) {
	candidates, err := parseCandidates(`Here is the result:
{"memories":[
  {"type":"preference","content":"Use concise answers","importance":5,"tags":["style"]},
  {"type":"fact","content":"My API key is sk-abcdefghijk","importance":5,"tags":[]},
  {"type":"fact","content":"The user has a login","importance":5,"tags":["password-secret"]},
  {"type":"context","content":"Current question context","importance":3,"tags":[]}
]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Content != "Use concise answers" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestParseCandidatesBoundsItemsAndDefaultsImportance(t *testing.T) {
	candidates, err := parseCandidates(`{"memories":[
  {"type":"fact","content":"one"},
  {"type":"fact","content":"two"},
  {"type":"fact","content":"three"},
  {"type":"fact","content":"four"},
  {"type":"fact","content":"five"},
  {"type":"fact","content":"six"}
]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 5 || candidates[0].Importance != 3 {
		t.Fatalf("candidates = %#v", candidates)
	}
}

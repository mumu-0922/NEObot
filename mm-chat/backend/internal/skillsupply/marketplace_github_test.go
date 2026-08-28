package skillsupply

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMarketplaceGitHubSubtreeFetchesOnlyPinnedSkillFiles(t *testing.T) {
	commit := strings.Repeat("a", 40)
	skill := []byte(validSkillMarkdown("demo-skill"))
	note := []byte("bounded reference\n")
	calls := []string{}
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.URL.String())
		switch request.URL.String() {
		case "https://api.github.com/repos/owner/repo/contents/skills/demo-skill?ref=" + commit:
			return sourceResponse(request, "application/json", fmt.Sprintf(`[
				{"name":"SKILL.md","path":"skills/demo-skill/SKILL.md","sha":"%s","size":%d,"type":"file"},
				{"name":"references","path":"skills/demo-skill/references","sha":"%s","size":0,"type":"dir"}
			]`, gitBlobSHA(skill), len(skill), strings.Repeat("b", 40))), nil
		case "https://api.github.com/repos/owner/repo/contents/skills/demo-skill/references?ref=" + commit:
			return sourceResponse(request, "application/json", fmt.Sprintf(`[
				{"name":"note.txt","path":"skills/demo-skill/references/note.txt","sha":"%s","size":%d,"type":"file"}
			]`, gitBlobSHA(note), len(note))), nil
		case "https://raw.githubusercontent.com/owner/repo/" + commit + "/skills/demo-skill/SKILL.md":
			return sourceResponse(request, "text/plain", string(skill)), nil
		case "https://raw.githubusercontent.com/owner/repo/" + commit + "/skills/demo-skill/references/note.txt":
			return sourceResponse(request, "text/plain", string(note)), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", request.URL)
		}
	})

	source, err := (DirectSkillLinkSource{Client: client}).fetchGitHubSubtree(
		context.Background(),
		"https://github.com/owner/repo/tree/"+commit+"/skills/demo-skill",
		"demo-skill",
	)
	if err != nil || source.Ref != "https://github.com/owner/repo.git#"+commit+":skills/demo-skill" ||
		source.StripPrefix != "" || len(calls) != 4 {
		t.Fatalf("source=%#v calls=%#v error=%v", source, calls, err)
	}
	validated, err := ValidateArchive(source)
	if err != nil || validated.Package.Name != "demo-skill" || validated.Package.FileCount != 2 {
		t.Fatalf("validated=%#v error=%v", validated.Package, err)
	}
	for _, requestURL := range calls {
		if strings.Contains(requestURL, "codeload.github.com") {
			t.Fatalf("subtree transport fetched the enclosing repository: %s", requestURL)
		}
	}
}

func TestMarketplaceGitHubSubtreeRejectsSymlinkBeforeBlobFetch(t *testing.T) {
	commit := strings.Repeat("c", 40)
	rawCalls := 0
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() == "raw.githubusercontent.com" {
			rawCalls++
			return nil, fmt.Errorf("raw fetch must not run")
		}
		return sourceResponse(request, "application/json", `[
			{"name":"SKILL.md","path":"skills/demo-skill/SKILL.md",
			 "sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":12,"type":"symlink"}
		]`), nil
	})

	_, err := (DirectSkillLinkSource{Client: client}).fetchGitHubSubtree(
		context.Background(),
		"https://github.com/owner/repo/tree/"+commit+"/skills/demo-skill",
		"demo-skill",
	)
	if !errors.Is(err, ErrArchiveInvalid) || rawCalls != 0 {
		t.Fatalf("symlink error=%v rawCalls=%d", err, rawCalls)
	}
}

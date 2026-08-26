package skillsupply

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestDirectSkillLinkSourcePinsAndSelectsExactGitHubSkill(t *testing.T) {
	commit := strings.Repeat("a", 40)
	archive := mustRawTestArchive(t, []testZipEntry{
		{
			name: "skills-" + commit + "/skills/productivity/grill-me/SKILL.md",
			body: validSkillMarkdown("grill-me"),
		},
		{
			name: "skills-" + commit + "/AGENTS.md",
			body: "CLAUDE.md",
			mode: os.ModeSymlink | 0o777,
		},
	})
	calls := []string{}
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.URL.String())
		switch request.URL.String() {
		case "https://www.aihero.dev/skills-grill-me":
			return sourceResponse(request, "text/html; charset=utf-8",
				`<code>npx skills@latest add mattpocock/skills --skill=grill-me</code>`), nil
		case "https://api.github.com/repos/mattpocock/skills/commits/HEAD":
			return sourceResponse(request, "application/json", `{"sha":"`+commit+`"}`), nil
		case "https://codeload.github.com/mattpocock/skills/zip/" + commit:
			return &http.Response{StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(string(archive))), Request: request}, nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})

	source, err := (DirectSkillLinkSource{Client: client}).Fetch(
		context.Background(), "https://www.aihero.dev/skills-grill-me", "grill-me",
	)
	if err != nil {
		t.Fatal(err)
	}
	if source.Ref != "https://github.com/mattpocock/skills.git#"+commit+":skills/productivity/grill-me" ||
		source.StripPrefix != "skills-"+commit+"/skills/productivity/grill-me/" ||
		len(calls) != 3 {
		t.Fatalf("source=%#v calls=%#v", source, calls)
	}
	validated, err := ValidateArchive(source)
	if err != nil || validated.Package.Name != "grill-me" {
		t.Fatalf("validated=%#v error=%v", validated.Package, err)
	}
}

func TestDirectSkillLinkSourceRejectsUntrustedOrAmbiguousCoordinates(t *testing.T) {
	for index, test := range []struct {
		url  string
		name string
		page string
	}{
		{url: "https://evil.example/skills-grill-me", name: "grill-me"},
		{url: "https://www.aihero.dev/skills-grill-me?next=evil", name: "grill-me"},
		{url: "https://www.aihero.dev/skills-grill-me", name: "other"},
		{url: "https://www.aihero.dev/skills-grill-me", name: "grill-me", page: `npx skills@latest add one/repo --skill=grill-me npx skills@latest add two/repo --skill=grill-me`},
		{url: "https://www.aihero.dev/skills-grill-me", name: "grill-me", page: `npx skills@latest add one/repo --skill=other`},
	} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return sourceResponse(request, "text/html", test.page), nil
			})
			_, err := (DirectSkillLinkSource{Client: client}).Fetch(
				context.Background(), test.url, test.name,
			)
			if !errors.Is(err, ErrInvalidSource) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func sourceResponse(request *http.Request, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestSourceAdaptersRequireImmutableCoordinates(t *testing.T) {
	archive, err := officialSyntheticArchive()
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeLobeHubFetcher{data: archive}
	lobe := LobeHubSource{Fetcher: fetcher}
	source, err := lobe.Fetch(context.Background(), "neo-readonly-inspector", "1.0.0")
	if err != nil || source.Ref != "lobehub:neo-readonly-inspector@1.0.0" || fetcher.maximum != MaxSourceArchiveBytes {
		t.Fatalf("LobeHub source = %#v/%v maximum=%d", source, err, fetcher.maximum)
	}
	for _, version := range []string{"", "latest", "main", "v1.0.0"} {
		if _, err := lobe.Fetch(context.Background(), "neo-readonly-inspector", version); !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("floating LobeHub version %q error = %v", version, err)
		}
	}

	commit := strings.Repeat("a", 40)
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://codeload.github.com/owner/repo/zip/"+commit ||
			request.Header.Get("Accept") != "application/zip" {
			t.Fatalf("GitHub request = %s headers=%#v", request.URL, request.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(archive)))}, nil
	})
	git := GitHubSource{Client: client}
	gitSource, err := git.Fetch(context.Background(), "https://github.com/owner/repo.git", commit, "skills/demo-skill")
	if err != nil || gitSource.Ref != "https://github.com/owner/repo.git#"+commit+":skills/demo-skill" ||
		gitSource.StripPrefix != "repo-"+commit+"/skills/demo-skill/" || gitSource.ExpectedName != "demo-skill" {
		t.Fatalf("Git source = %#v/%v", gitSource, err)
	}
	for _, invalid := range []struct{ url, commit, subdirectory string }{
		{"http://github.com/owner/repo", commit, ""},
		{"https://github.com/owner/repo?token=x", commit, ""},
		{"https://gitlab.com/owner/repo", commit, ""},
		{"https://github.com/owner/repo", "main", ""},
		{"https://github.com/owner/repo", strings.ToUpper(commit), ""},
		{"https://github.com/owner/repo", commit, "../escape"},
	} {
		if _, err := git.Fetch(context.Background(), invalid.url, invalid.commit, invalid.subdirectory); !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("Git source %#v error = %v", invalid, err)
		}
	}
}

type fakeLobeHubFetcher struct {
	data    []byte
	maximum int64
}

func (fetcher *fakeLobeHubFetcher) FetchSkillPackage(
	_ context.Context,
	_, _ string,
	maximum int64,
) ([]byte, error) {
	fetcher.maximum = maximum
	return append([]byte(nil), fetcher.data...), nil
}

type sourceRoundTripFunc func(*http.Request) (*http.Response, error)

func (function sourceRoundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

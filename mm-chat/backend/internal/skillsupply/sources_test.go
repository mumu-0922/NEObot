package skillsupply

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

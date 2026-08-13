package agentbroker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestArtifactPublicationRejectsStalePolicyAndCleansFailures(t *testing.T) {
	candidate := testArtifactCandidate()
	authority := testArtifactAuthority()
	for _, mutate := range []func(*ArtifactAuthority){
		func(value *ArtifactAuthority) { value.Current = false },
		func(value *ArtifactAuthority) { value.GrantAuthorized = false },
		func(value *ArtifactAuthority) { value.Generation++ },
		func(value *ArtifactAuthority) { value.MaxBytes = candidate.Size - 1 },
		func(value *ArtifactAuthority) { value.AllowedMedia = []string{"image/png"} },
	} {
		quarantine := &fakeQuarantine{body: []byte("artifact")}
		publisher, _ := NewArtifactPublisher(quarantine, &fakeArtifactObjects{}, &fakeArtifactRepository{}, artifactScannerFunc(func(context.Context, ArtifactCandidate, io.Reader) error { return nil }))
		changed := authority
		mutate(&changed)
		if _, err := publisher.Publish(context.Background(), changed, candidate); !errors.Is(err, ErrArtifactDenied) {
			t.Fatalf("policy error = %v", err)
		}
		if quarantine.opens != 0 {
			t.Fatalf("denied candidate opened quarantine %d times", quarantine.opens)
		}
	}

	quarantine := &fakeQuarantine{body: []byte("artifact")}
	objects := &fakeArtifactObjects{}
	publisher, _ := NewArtifactPublisher(quarantine, objects, &fakeArtifactRepository{err: errors.New("row failed")}, artifactScannerFunc(func(context.Context, ArtifactCandidate, io.Reader) error { return nil }))
	if _, err := publisher.Publish(context.Background(), authority, candidate); err == nil || objects.deletes != 1 {
		t.Fatalf("row failure = %v, object deletes=%d", err, objects.deletes)
	}

	quarantine = &fakeQuarantine{body: []byte("artifact")}
	publisher, _ = NewArtifactPublisher(quarantine, &fakeArtifactObjects{}, &fakeArtifactRepository{}, artifactScannerFunc(func(context.Context, ArtifactCandidate, io.Reader) error { return errors.New("malware") }))
	if _, err := publisher.Publish(context.Background(), authority, candidate); !errors.Is(err, ErrArtifactDenied) || quarantine.deletes != 1 {
		t.Fatalf("malware failure = %v, quarantine deletes=%d", err, quarantine.deletes)
	}

	for _, mutate := range []func(*ArtifactCandidate){
		func(value *ArtifactCandidate) { value.Size-- },
		func(value *ArtifactCandidate) { value.Fingerprint = testPackage },
	} {
		candidate := testArtifactCandidate()
		mutate(&candidate)
		quarantine = &fakeQuarantine{body: []byte("artifact")}
		publisher, _ = NewArtifactPublisher(quarantine, &fakeArtifactObjects{}, &fakeArtifactRepository{}, artifactScannerFunc(func(context.Context, ArtifactCandidate, io.Reader) error { return nil }))
		if _, err := publisher.Publish(context.Background(), authority, candidate); !errors.Is(err, ErrArtifactDenied) || quarantine.deletes != 1 {
			t.Fatalf("content binding failure = %v, quarantine deletes=%d", err, quarantine.deletes)
		}
	}
}

func TestArtifactPublicationUsesObjectBeforeRowAndDeletesQuarantine(t *testing.T) {
	order := []string{}
	quarantine := &fakeQuarantine{body: []byte("artifact"), order: &order}
	objects := &fakeArtifactObjects{order: &order}
	repository := &fakeArtifactRepository{order: &order}
	publisher, _ := NewArtifactPublisher(quarantine, objects, repository, artifactScannerFunc(func(context.Context, ArtifactCandidate, io.Reader) error { order = append(order, "scan"); return nil }))
	key, err := publisher.Publish(context.Background(), testArtifactAuthority(), testArtifactCandidate())
	if err != nil || key == "" {
		t.Fatalf("Publish = %q, %v", key, err)
	}
	want := []string{"open", "scan", "put", "attach", "quarantine-delete"}
	if len(order) != len(want) {
		t.Fatalf("publication order = %#v", order)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("publication order = %#v", order)
		}
	}
	if string(objects.body) != "artifact" {
		t.Fatalf("published body = %q", objects.body)
	}
}

func testArtifactCandidate() ArtifactCandidate {
	return ArtifactCandidate{UserID: testUser, RunID: testRun, AttemptID: testAttempt, Generation: 1,
		QuarantineRef: testAttempt + "/result.txt", Name: "result.txt", MediaType: "text/plain",
		Size: int64(len("artifact")), Fingerprint: artifactFingerprint([]byte("artifact"))}
}
func testArtifactAuthority() ArtifactAuthority {
	return ArtifactAuthority{UserID: testUser, RunID: testRun, AttemptID: testAttempt, Generation: 1,
		MaxBytes: 1024, AllowedMedia: []string{"text/plain"}, Current: true, GrantAuthorized: true}
}

type artifactScannerFunc func(context.Context, ArtifactCandidate, io.Reader) error

func (scanner artifactScannerFunc) Scan(ctx context.Context, candidate ArtifactCandidate, reader io.Reader) error {
	return scanner(ctx, candidate, reader)
}

type fakeQuarantine struct {
	body           []byte
	opens, deletes int
	order          *[]string
}

func (store *fakeQuarantine) Open(context.Context, string) (io.ReadCloser, error) {
	store.opens++
	if store.order != nil {
		*store.order = append(*store.order, "open")
	}
	return io.NopCloser(bytes.NewReader(store.body)), nil
}
func (store *fakeQuarantine) Delete(context.Context, string) error {
	store.deletes++
	if store.order != nil {
		*store.order = append(*store.order, "quarantine-delete")
	}
	return nil
}

type fakeArtifactObjects struct {
	puts, deletes int
	order         *[]string
	body          []byte
}

func (store *fakeArtifactObjects) Put(_ context.Context, _ string, reader io.Reader, _ int64, _ string) error {
	store.puts++
	store.body, _ = io.ReadAll(reader)
	if store.order != nil {
		*store.order = append(*store.order, "put")
	}
	return nil
}
func (store *fakeArtifactObjects) Delete(context.Context, string) error {
	store.deletes++
	if store.order != nil {
		*store.order = append(*store.order, "object-delete")
	}
	return nil
}

type fakeArtifactRepository struct {
	err   error
	order *[]string
}

func (repository *fakeArtifactRepository) AttachArtifact(context.Context, ArtifactCandidate, string) error {
	if repository.order != nil {
		*repository.order = append(*repository.order, "attach")
	}
	return repository.err
}

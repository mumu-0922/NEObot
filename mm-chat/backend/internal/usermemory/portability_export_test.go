package usermemory

import (
	"bytes"
	"context"
	"testing"
)

type portabilityExportTestRepository struct {
	*fakeRepository
	records []any
}

func (r *portabilityExportTestRepository) WithPortabilityExportSnapshot(
	ctx context.Context,
	_ bool,
	use func(PortabilityExportSnapshot) error,
) error {
	return use(portabilityExportTestSnapshot{records: r.records})
}

type portabilityExportTestSnapshot struct {
	records []any
}

func (s portabilityExportTestSnapshot) ScanRecords(
	_ context.Context,
	visit func(any) error,
) (PortableRecordCounts, error) {
	var counts PortableRecordCounts
	for _, record := range s.records {
		switch record.(type) {
		case PortableSettingsRecord:
			counts.Settings++
		case PortableProjectRecord:
			counts.Projects++
		case PortableMemoryRecord:
			counts.Memories++
		case PortableRevisionRecord:
			counts.Revisions++
		}
		if err := visit(record); err != nil {
			return PortableRecordCounts{}, err
		}
	}
	return counts, nil
}

func TestExportMemoryPackageUsesOneSnapshotAndAuthenticatedStream(t *testing.T) {
	repo := &portabilityExportTestRepository{
		fakeRepository: &fakeRepository{},
		records: []any{
			PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
			PortableProjectRecord{
				Kind: "project", Ref: "project-000001", Name: "Neo Chat",
				Description: "Memory portability", LifecycleStatus: "active",
			},
			validPortableMemory(1),
		},
	}
	service := NewService(repo, WithPortabilityRelease("test-release"))
	var encrypted bytes.Buffer
	result, err := service.ExportMemoryPackage(
		context.Background(), &encrypted, "fixture-passphrase", false,
	)
	if err != nil {
		t.Fatalf("ExportMemoryPackage() error = %v", err)
	}
	if result.Manifest.ExporterRelease != "test-release" ||
		result.Manifest.Counts.Memories != 1 {
		t.Fatalf("result = %#v", result)
	}
	parsed, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(encrypted.Bytes()),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{},
	)
	if err != nil {
		t.Fatalf("ParseEncryptedMemoryPackage() error = %v", err)
	}
	if parsed.Manifest.RecordsSHA256 != result.Manifest.RecordsSHA256 {
		t.Fatalf("parsed manifest = %#v", parsed.Manifest)
	}
}

var _ PortabilityExportRepository = (*portabilityExportTestRepository)(nil)

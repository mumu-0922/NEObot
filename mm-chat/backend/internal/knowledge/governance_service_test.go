package knowledge

import (
	"context"
	"testing"
)

type fakeGovernanceRepository struct {
	applyCalls int
	manifest   GovernanceManifest
	hash       string
	processor  string
	endpointID string
}

func (r *fakeGovernanceRepository) ApplyGovernance(_ context.Context, manifest GovernanceManifest, hash string) (ProcessorGovernanceHead, error) {
	r.applyCalls++
	r.manifest, r.hash = manifest, hash
	return ProcessorGovernanceHead{Processor: manifest.Processor, EndpointID: manifest.EndpointID}, nil
}
func (r *fakeGovernanceRepository) DisableGovernance(_ context.Context, processor, endpointID, modelID string) (ProcessorGovernanceHead, error) {
	r.processor, r.endpointID = processor, endpointID
	return ProcessorGovernanceHead{Processor: processor, EndpointID: endpointID, ModelID: modelID}, nil
}

func TestGovernanceServiceNormalizesAndHashesCanonicalManifest(t *testing.T) {
	repo := &fakeGovernanceRepository{}
	service := NewGovernanceService(repo)
	input := GovernanceManifest{Processor: " mineru ", EndpointID: " hosted-main ", ModelID: " model-stable-20260712 ", ModelAPIVersion: " api-20260623 ",
		AllowedPurposes: []string{"parse", "answer", "parse"}, AllowedDataTypes: []string{"text/plain", "application/pdf"},
		Region: " global ", RetentionPolicy: " none ", DeletionContract: " delete ", TrainingUse: " disabled "}
	if _, err := service.Apply(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if repo.manifest.Processor != "mineru" ||
		repo.manifest.EndpointID != "hosted-main" ||
		repo.manifest.ModelID != "model-stable-20260712" ||
		repo.manifest.ModelAPIVersion != "api-20260623" ||
		repo.manifest.AllowedPurposes[0] != "answer" || len(repo.hash) != 64 {
		t.Fatalf("normalized manifest/hash = %#v %q", repo.manifest, repo.hash)
	}
	if repo.hash != "3c0887f8e0577407451e452cca1b1f2a1c51adb0e5d8ecc8d7546dc2cd60f45f" {
		t.Fatalf("canonical hash golden vector changed: %s", repo.hash)
	}
	firstHash := repo.hash
	input.AllowedPurposes = []string{"answer", "parse"}
	input.AllowedDataTypes = []string{"application/pdf", "text/plain"}
	if _, err := service.Apply(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if repo.hash != firstHash {
		t.Fatalf("canonical hashes differ: %s != %s", repo.hash, firstHash)
	}
}

func TestGovernanceServiceRejectsInvalidAuthorityFields(t *testing.T) {
	base := GovernanceManifest{Processor: "mineru", EndpointID: "hosted-main", ModelID: "model-stable-20260712", ModelAPIVersion: "api-20260623",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	for name, mutate := range map[string]func(*GovernanceManifest){
		"processor":        func(v *GovernanceManifest) { v.Processor = "MinerU" },
		"model":            func(v *GovernanceManifest) { v.ModelID = "Model V1" },
		"model credential": func(v *GovernanceManifest) { v.ModelAPIVersion = "sk_live_secret123" },
		"purpose":          func(v *GovernanceManifest) { v.AllowedPurposes = []string{"admin"} },
		"data type":        func(v *GovernanceManifest) { v.AllowedDataTypes = []string{" "} },
		"policy":           func(v *GovernanceManifest) { v.RetentionPolicy = "" },
		"credential text":  func(v *GovernanceManifest) { v.RetentionPolicy = "Bearer secret" },
		"credential URL": func(v *GovernanceManifest) {
			v.DeletionContract = "https://user:pass@example.test"
		},
		"invalid MIME": func(v *GovernanceManifest) { v.AllowedDataTypes = []string{"pdf"} },
		"unsupported MIME wildcard": func(v *GovernanceManifest) {
			v.AllowedDataTypes = []string{"application/*"}
		},
		"unreviewed region": func(v *GovernanceManifest) { v.Region = "us" },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if _, err := NewGovernanceService(&fakeGovernanceRepository{}).Apply(context.Background(), value); err == nil {
				t.Fatal("error = nil")
			}
		})
	}
}

func TestGovernanceServiceRejectsPlaceholderProviderIdentity(t *testing.T) {
	base := GovernanceManifest{Processor: "mineru", EndpointID: "hosted-main", ModelID: "model-stable-20260712", ModelAPIVersion: "api-20260623",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	placeholders := []string{
		"default", "model-v1", "v1", "TBD", "todo", "unknown",
		"unverified", "change-me", "64-lowercase-hex",
	}
	fields := map[string]func(*GovernanceManifest, string){
		"endpoint":    func(v *GovernanceManifest, value string) { v.EndpointID = value },
		"model":       func(v *GovernanceManifest, value string) { v.ModelID = value },
		"api version": func(v *GovernanceManifest, value string) { v.ModelAPIVersion = value },
	}
	for field, mutate := range fields {
		for _, placeholder := range placeholders {
			t.Run(field+"/"+placeholder, func(t *testing.T) {
				value := base
				mutate(&value, placeholder)
				repo := &fakeGovernanceRepository{}
				if _, err := NewGovernanceService(repo).Apply(
					context.Background(), value,
				); err == nil {
					t.Fatal("error = nil")
				}
				if repo.applyCalls != 0 {
					t.Fatalf("repository apply calls = %d, want 0", repo.applyCalls)
				}
			})
		}
	}
}

func TestGovernanceServiceNormalizesDisableBinding(t *testing.T) {
	repo := &fakeGovernanceRepository{}
	if _, err := NewGovernanceService(repo).Disable(context.Background(), " mineru ", " default "); err != nil {
		t.Fatal(err)
	}
	if repo.processor != "mineru" || repo.endpointID != "default" {
		t.Fatalf("binding = %q/%q", repo.processor, repo.endpointID)
	}
}

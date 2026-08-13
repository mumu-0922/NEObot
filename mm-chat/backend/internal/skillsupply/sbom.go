package skillsupply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type cycloneDX struct {
	BOMFormat    string              `json:"bomFormat"`
	SpecVersion  string              `json:"specVersion"`
	Version      int                 `json:"version"`
	Metadata     cycloneMetadata     `json:"metadata"`
	Components   []cycloneItem       `json:"components"`
	Dependencies []cycloneDependency `json:"dependencies"`
}

type cycloneMetadata struct {
	Component cycloneItem `json:"component"`
}

type cycloneItem struct {
	Type    string        `json:"type"`
	BOMRef  string        `json:"bom-ref"`
	Name    string        `json:"name"`
	Version string        `json:"version,omitempty"`
	Hashes  []cycloneHash `json:"hashes,omitempty"`
}

type cycloneHash struct {
	Algorithm string `json:"alg"`
	Content   string `json:"content"`
}

type cycloneDependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn"`
}

func buildSBOM(
	name, version, packageFingerprint string,
	inventory []FileInventory,
	manifest *RuntimeManifest,
) ([]byte, string, error) {
	rootRef := "skill:" + packageFingerprint
	root := cycloneItem{Type: "application", BOMRef: rootRef, Name: name, Version: version,
		Hashes: []cycloneHash{{Algorithm: "SHA-256", Content: stringsTrimDigest(packageFingerprint)}}}
	components := make([]cycloneItem, 0, len(inventory)+129)
	dependencies := make([]string, 0, len(inventory)+129)
	for _, file := range inventory {
		ref := "file:" + file.SHA256 + ":" + file.Path
		components = append(components, cycloneItem{Type: "file", BOMRef: ref, Name: file.Path,
			Hashes: []cycloneHash{{Algorithm: "SHA-256", Content: stringsTrimDigest(file.SHA256)}}})
		dependencies = append(dependencies, ref)
	}
	if manifest != nil {
		imageRef := "container:" + manifest.Runtime.Image
		components = append(components, cycloneItem{Type: "container", BOMRef: imageRef,
			Name: manifest.Runtime.Image, Hashes: []cycloneHash{{Algorithm: "SHA-256",
				Content: stringsTrimDigest(manifest.Runtime.Image[stringsLastDigest(manifest.Runtime.Image):])}}})
		dependencies = append(dependencies, imageRef)
		for _, dependency := range manifest.Dependencies {
			ref := "library:" + dependency.Name + "@" + dependency.Digest
			components = append(components, cycloneItem{Type: "library", BOMRef: ref, Name: dependency.Name,
				Hashes: []cycloneHash{{Algorithm: "SHA-256", Content: stringsTrimDigest(dependency.Digest)}}})
			dependencies = append(dependencies, ref)
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].BOMRef < components[j].BOMRef })
	sort.Strings(dependencies)
	bom := cycloneDX{BOMFormat: "CycloneDX", SpecVersion: "1.6", Version: 1,
		Metadata: cycloneMetadata{Component: root}, Components: components,
		Dependencies: []cycloneDependency{{Ref: rootRef, DependsOn: dependencies}}}
	encoded, err := json.Marshal(bom)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return encoded, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func stringsTrimDigest(value string) string {
	if len(value) >= len("sha256:") && value[:len("sha256:")] == "sha256:" {
		return value[len("sha256:"):]
	}
	return value
}

func stringsLastDigest(value string) int {
	for index := len(value) - len("sha256:"); index >= 0; index-- {
		if value[index:index+len("sha256:")] == "sha256:" {
			return index
		}
	}
	return 0
}

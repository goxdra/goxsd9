package specs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateManifest(t *testing.T) {
	t.Parallel()

	valid := validTestManifest()
	if err := ValidateManifest(valid); err != nil {
		t.Fatalf("ValidateManifest(valid) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "unsupported format",
			mutate: func(manifest *Manifest) {
				manifest.FormatVersion = 2
			},
		},
		{
			name: "duplicate ID",
			mutate: func(manifest *Manifest) {
				manifest.Errata[0].ID = manifest.Specifications[0].ID
			},
		},
		{
			name: "invalid URL",
			mutate: func(manifest *Manifest) {
				manifest.Specifications[0].URL = "http://www.w3.org/TR/2020/example/"
			},
		},
		{
			name: "invalid digest",
			mutate: func(manifest *Manifest) {
				manifest.Specifications[0].SHA256 = "not-a-digest"
			},
		},
		{
			name: "unknown dependency",
			mutate: func(manifest *Manifest) {
				manifest.Specifications[0].Dependencies = []string{"missing"}
			},
		},
		{
			name: "invalid representation",
			mutate: func(manifest *Manifest) {
				manifest.BootstrapArtifacts[0].Representation = "yaml"
			},
		},
		{
			name: "XSD 1.0 datatype representation has a fixed artifact",
			mutate: func(manifest *Manifest) {
				manifest.BootstrapArtifacts[0].Representation = manifestXSD10DatatypesRepresentation
			},
		},
		{
			name: "XSD 1.0 datatype representation requires a dependency role",
			mutate: func(manifest *Manifest) {
				artifact := &manifest.BootstrapArtifacts[3]
				artifact.Entry = true
			},
		},
		{
			name: "missing bootstrap version",
			mutate: func(manifest *Manifest) {
				manifest.BootstrapArtifacts[1].Entry = false
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			manifest := validTestManifest()
			test.mutate(&manifest)
			err := ValidateManifest(manifest)
			if err == nil {
				t.Fatal("ValidateManifest() error = nil")
			}
			assertErrorCode(t, err, "specs.manifest.validate")
		})
	}
}

func TestReadManifestIsStrictAndFindCopiesSlices(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "specs", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(validTestManifest())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	writeErr := os.WriteFile(manifestPath, data, 0o600)
	if writeErr != nil {
		t.Fatalf("WriteFile() error = %v", writeErr)
	}
	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	entry, err := manifest.Find("xml-schema")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	entry.Aliases[0] = "changed"
	again, err := manifest.Find("xml-schema")
	if err != nil {
		t.Fatalf("Find() second call error = %v", err)
	}
	if again.Aliases[0] != "http://www.w3.org/2001/xml.xsd" {
		t.Fatalf("Find() returned aliased slice backed by manifest: %q", again.Aliases[0])
	}

	trailing := append(append([]byte(nil), data...), []byte("\n{}")...)
	writeErr = os.WriteFile(manifestPath, trailing, 0o600)
	if writeErr != nil {
		t.Fatalf("WriteFile(trailing) error = %v", writeErr)
	}
	err = readManifestError(root)
	assertErrorCode(t, err, "specs.manifest.decode")
	if !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing manifest error = %v", err)
	}

	unknown := append([]byte(nil), data[:len(data)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	writeErr = os.WriteFile(manifestPath, unknown, 0o600)
	if writeErr != nil {
		t.Fatalf("WriteFile(unknown) error = %v", writeErr)
	}
	err = readManifestError(root)
	assertErrorCode(t, err, "specs.manifest.decode")
}

func readManifestError(root string) error {
	_, err := ReadManifest(root)
	return err
}

func validTestManifest() Manifest {
	digest := strings.Repeat("a", 64)
	return Manifest{
		FormatVersion: 1,
		Specifications: []Source{{
			ID:          "example-spec",
			Title:       "Example specification",
			URL:         "https://www.w3.org/TR/2020/example/",
			SHA256:      digest,
			XSDVersions: []string{"1.1"},
		}},
		Errata: []Source{{
			ID:          "example-errata",
			Title:       "Example errata",
			URL:         "https://www.w3.org/2004/03/example-errata",
			SHA256:      digest,
			XSDVersions: []string{"1.0"},
		}},
		BootstrapArtifacts: []BootstrapArtifact{
			{
				ID:             "xsd10-entry",
				Title:          "XSD 1.0 entry",
				URL:            "https://www.w3.org/TR/2004/REC-example/XMLSchema.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0"},
				Representation: "xml",
				Entry:          true,
				Dependencies:   []string{xsd10DatatypesSchemaID},
			},
			{
				ID:             "xsd11-entry",
				Title:          "XSD 1.1 entry",
				URL:            "https://www.w3.org/TR/2012/REC-example/XMLSchema.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.1"},
				Representation: "xml",
				Entry:          true,
			},
			{
				ID:             "xml-schema",
				Title:          "XML namespace schema",
				URL:            "https://www.w3.org/2001/xml.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0", "1.1"},
				Representation: "xml",
				Aliases:        []string{"http://www.w3.org/2001/xml.xsd"},
			},
			{
				ID:             xsd10DatatypesSchemaID,
				Title:          "XSD 1.0 datatype schema",
				URL:            "https://www.w3.org/TR/2004/REC-example/datatypes.xsd",
				SHA256:         digest,
				XSDVersions:    []string{"1.0"},
				Representation: manifestXSD10DatatypesRepresentation,
			},
		},
	}
}

func assertErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error code = nil, want %q", want)
	}
	var corpusErr *Error
	if !errors.As(err, &corpusErr) {
		t.Fatalf("error %v is not *Error", err)
	}
	if corpusErr.Code != want {
		t.Fatalf("error code = %q, want %q", corpusErr.Code, want)
	}
}

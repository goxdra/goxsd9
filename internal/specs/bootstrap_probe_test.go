package specs

import (
	"bytes"
	"context"
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

//go:embed testdata/bootstrap/*.raw
var bootstrapProbeFixtures embed.FS

const (
	bootstrapProbeConversionStage    = "conversion"
	bootstrapProbeParserStage        = "parser"
	bootstrapProbeParserSuccessStage = "parser-success"
	bootstrapProbeParserMissingStage = "parser-error-without-diagnostic"
	bootstrapProbeCauseTypeXMLSyntax = "*xml.SyntaxError"
)

type bootstrapProbeRow struct {
	id       string
	policy   goxsd9.LanguagePolicy
	expected bootstrapProbeObservation
}

type bootstrapProbeObservation struct {
	stage               string
	conversionCode      string
	conversionID        string
	conversionURL       string
	conversionCause     string
	conversionCauseType string
	diagnostic          bootstrapProbeDiagnostic
}

type bootstrapProbeDiagnostic struct {
	class   goxsd9.FailureClass
	code    string
	feature goxsd9.FeatureID
	source  goxsd9.SourceID
	line    int
	column  int
	specRef string
}

// Rows intentionally follow manifest.BootstrapArtifacts, not the issue's
// acceptance enumeration: xsd10-schema-for-schemas, xsd10-datatypes-schema,
// xsd11-schema-for-schemas, xsd11-datatypes-schema, xml-schema.
var bootstrapProbeRows = []bootstrapProbeRow{
	{
		id:     "xsd10-schema-for-schemas",
		policy: goxsd9.Strict10,
		expected: bootstrapProbeObservation{
			stage: bootstrapProbeParserStage,
			diagnostic: bootstrapProbeDiagnostic{
				class:   goxsd9.FailureUnsupported,
				code:    goxsd9.UnsupportedSchemaSyntaxCode,
				feature: goxsd9.FeatureSchemaSyntax,
				source:  "xsd10-schema-for-schemas",
				line:    126,
				column:  42,
				specRef: "xsd10-structures#schema-document",
			},
		},
	},
	{
		id:     "xsd10-datatypes-schema",
		policy: goxsd9.Strict10,
		expected: bootstrapProbeObservation{
			stage:               bootstrapProbeConversionStage,
			conversionCode:      bootstrapXMLDocumentCode,
			conversionID:        "xsd10-datatypes-schema",
			conversionURL:       "https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/datatypes.xsd",
			conversionCause:     "XML syntax error on line 38: XML processing instruction target is invalid",
			conversionCauseType: bootstrapProbeCauseTypeXMLSyntax,
		},
	},
	{
		id:     "xsd11-schema-for-schemas",
		policy: goxsd9.Strict11,
		expected: bootstrapProbeObservation{
			stage: bootstrapProbeParserStage,
			diagnostic: bootstrapProbeDiagnostic{
				class:   goxsd9.FailureUnsupported,
				code:    goxsd9.UnsupportedSchemaSyntaxCode,
				feature: goxsd9.FeatureSchemaSyntax,
				source:  "xsd11-schema-for-schemas",
				line:    119,
				column:  43,
				specRef: "xsd11-structures#cSchemaDocument",
			},
		},
	},
	{
		id:     "xsd11-datatypes-schema",
		policy: goxsd9.Strict11,
		expected: bootstrapProbeObservation{
			stage: bootstrapProbeParserStage,
			diagnostic: bootstrapProbeDiagnostic{
				class:   goxsd9.FailureUnsupported,
				code:    goxsd9.UnsupportedDatatypeFacetCode,
				feature: goxsd9.FeatureDatatypeFacets,
				source:  "xsd11-datatypes-schema",
				line:    79,
				column:  11,
				specRef: "xsd11-datatypes#decimal",
			},
		},
	},
	{
		id:     "xml-schema",
		policy: goxsd9.Compatibility,
		expected: bootstrapProbeObservation{
			stage: bootstrapProbeParserStage,
			diagnostic: bootstrapProbeDiagnostic{
				class:   goxsd9.FailureUnsupported,
				code:    goxsd9.UnsupportedSchemaSyntaxCode,
				feature: goxsd9.FeatureSchemaSyntax,
				source:  "xml-schema",
				line:    78,
				column:  3,
				specRef: "xsd10-structures#schema-document",
			},
		},
	},
}

func TestBootstrapFirstBlockerMatrix(t *testing.T) {
	manifest := readBootstrapProbeManifest(t)
	if got, want := len(manifest.BootstrapArtifacts), len(bootstrapProbeRows); got != want {
		t.Fatalf("manifest bootstrap artifact count = %d, want %d in stable probe order", got, want)
	}

	for index := range manifest.BootstrapArtifacts {
		artifact := manifest.BootstrapArtifacts[index]
		row := bootstrapProbeRows[index]
		if artifact.ID != row.id {
			t.Fatalf("manifest bootstrap artifact order at index %d = %q, want probe row %q", index, artifact.ID, row.id)
		}
		entry, err := manifest.Find(artifact.ID)
		if err != nil {
			t.Fatalf("Find(%q) error = %v", artifact.ID, err)
		}
		t.Run(artifact.ID, func(t *testing.T) {
			actual := observeBootstrapProbe(t, entry, row.policy)
			if actual != row.expected {
				t.Fatalf("artifact %q outcome mismatch: expected=%+v actual=%+v", row.id, row.expected, actual)
			}
		})
	}
}

func readBootstrapProbeManifest(t *testing.T) Manifest {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest(%q) error = %v", root, err)
	}
	return manifest
}

func observeBootstrapProbe(t *testing.T, entry Entry, policy goxsd9.LanguagePolicy) bootstrapProbeObservation {
	t.Helper()
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatalf("read raw fixture for %q: %v", entry.ID, err)
	}
	if got, want := testDigest(raw), entry.SHA256; !strings.EqualFold(got, want) {
		t.Fatalf("artifact %q raw fixture digest = %q, want manifest digest %q", entry.ID, got, want)
	}

	firstRaw, secondRaw := fetchBootstrapProbeRaw(t, entry, raw)
	firstConverted, conversionFailure, conversionFailed := convertBootstrapProbe(t, entry, firstRaw, secondRaw)
	if conversionFailed {
		return conversionFailure
	}
	validationFailure, validationFailed := validateBootstrapProbe(t, entry, firstConverted)
	if validationFailed {
		return validationFailure
	}

	return parseBootstrapProbeTwice(t, entry, firstConverted, policy)
}

func fetchBootstrapProbeRaw(t *testing.T, entry Entry, fixture []byte) ([]byte, []byte) {
	t.Helper()
	transport := &bootstrapProbeTransport{url: entry.URL, body: fixture}
	first, err := Fetch(context.Background(), &http.Client{Transport: transport}, entry)
	if err != nil {
		t.Fatalf("artifact %q first offline Fetch: %v", entry.ID, err)
	}
	second, err := Fetch(context.Background(), &http.Client{Transport: transport}, entry)
	if err != nil {
		t.Fatalf("artifact %q second offline Fetch: %v", entry.ID, err)
	}
	if !bytes.Equal(first, fixture) || !bytes.Equal(second, fixture) {
		t.Fatalf("artifact %q Fetch changed the checked-in raw fixture", entry.ID)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("artifact %q raw bytes changed between offline Fetch calls", entry.ID)
	}
	if want := []string{entry.URL, entry.URL}; !slices.Equal(transport.calls, want) {
		t.Fatalf("artifact %q offline Fetch calls = %#v, want %#v", entry.ID, transport.calls, want)
	}
	return first, second
}

func convertBootstrapProbe(t *testing.T, entry Entry, firstRaw, secondRaw []byte) ([]byte, bootstrapProbeObservation, bool) {
	t.Helper()
	first, firstErr := convert(entry, firstRaw)
	second, secondErr := convert(entry, secondRaw)
	if firstErr == nil && secondErr == nil {
		if !bytes.Equal(first, second) {
			t.Fatalf("artifact %q converted bytes changed between calls", entry.ID)
		}
		return first, bootstrapProbeObservation{}, false
	}
	if (firstErr == nil) != (secondErr == nil) {
		t.Fatalf("artifact %q conversion status changed between calls: first=%v second=%v", entry.ID, firstErr, secondErr)
	}
	firstObservation := bootstrapProbeConversionObservation(firstErr)
	secondObservation := bootstrapProbeConversionObservation(secondErr)
	if firstObservation != secondObservation {
		t.Fatalf("artifact %q conversion failure changed between calls: first=%+v second=%+v", entry.ID, firstObservation, secondObservation)
	}
	return nil, firstObservation, true
}

func validateBootstrapProbe(t *testing.T, entry Entry, converted []byte) (bootstrapProbeObservation, bool) {
	t.Helper()
	firstErr := validateBootstrapXML(entry, converted)
	secondErr := validateBootstrapXML(entry, converted)
	if firstErr == nil && secondErr == nil {
		return bootstrapProbeObservation{}, false
	}
	if (firstErr == nil) != (secondErr == nil) {
		t.Fatalf("artifact %q XML validation status changed between calls: first=%v second=%v", entry.ID, firstErr, secondErr)
	}
	firstObservation := bootstrapProbeConversionObservation(firstErr)
	secondObservation := bootstrapProbeConversionObservation(secondErr)
	if firstObservation != secondObservation {
		t.Fatalf("artifact %q XML validation failure changed between calls: first=%+v second=%+v", entry.ID, firstObservation, secondObservation)
	}
	return firstObservation, true
}

func parseBootstrapProbeTwice(t *testing.T, entry Entry, converted []byte, policy goxsd9.LanguagePolicy) bootstrapProbeObservation {
	t.Helper()
	firstSchema, firstParseErr := parseBootstrapProbe(t, entry, converted, policy)
	secondSchema, secondParseErr := parseBootstrapProbe(t, entry, converted, policy)
	assertBootstrapProbeNoPartialSchema(t, entry.ID, firstSchema, firstParseErr)
	assertBootstrapProbeNoPartialSchema(t, entry.ID, secondSchema, secondParseErr)
	firstObservation := bootstrapProbeParserObservation(firstParseErr)
	secondObservation := bootstrapProbeParserObservation(secondParseErr)
	if firstObservation != secondObservation {
		t.Fatalf("artifact %q parser outcome changed between calls: first=%+v second=%+v", entry.ID, firstObservation, secondObservation)
	}
	return firstObservation
}

func parseBootstrapProbe(t *testing.T, entry Entry, data []byte, policy goxsd9.LanguagePolicy) (goxsd9.Schema, error) {
	t.Helper()
	root, err := goxsd9.NewResolvedSource(
		context.Background(),
		goxsd9.SourceID(entry.ID),
		io.NopCloser(bytes.NewReader(data)),
	)
	if err != nil {
		t.Fatalf("NewResolvedSource(%q) error = %v", entry.ID, err)
	}
	return goxsd9.ParseSchemaWithPolicy(root, nil, policy)
}

func assertBootstrapProbeNoPartialSchema(t *testing.T, id string, schema goxsd9.Schema, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatalf("artifact %q parser returned schema data with error: documents=%d components=%d error=%v", id, len(schema.Documents()), len(schema.Components()), err)
	}
}

func bootstrapProbeConversionObservation(err error) bootstrapProbeObservation {
	var corpusErr *Error
	code := ""
	id := ""
	sourceURL := ""
	if errors.As(err, &corpusErr) {
		code = corpusErr.Code
		id = corpusErr.ID
		sourceURL = corpusErr.URL
	}
	causeType, cause := bootstrapProbeCause(err)
	return bootstrapProbeObservation{
		stage:               bootstrapProbeConversionStage,
		conversionCode:      code,
		conversionID:        id,
		conversionURL:       sourceURL,
		conversionCause:     cause,
		conversionCauseType: causeType,
	}
}

func bootstrapProbeCause(err error) (string, string) {
	var syntaxErr *xml.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("%T", syntaxErr), syntaxErr.Error()
	}
	cause := errors.Unwrap(err)
	if cause == nil {
		return "", ""
	}
	return fmt.Sprintf("%T", cause), cause.Error()
}

func bootstrapProbeParserObservation(err error) bootstrapProbeObservation {
	if err == nil {
		return bootstrapProbeObservation{stage: bootstrapProbeParserSuccessStage}
	}
	diagnostic, ok := firstBootstrapProbeDiagnostic(err)
	if !ok {
		return bootstrapProbeObservation{stage: bootstrapProbeParserMissingStage}
	}
	loc := diagnostic.Loc()
	return bootstrapProbeObservation{
		stage: bootstrapProbeParserStage,
		diagnostic: bootstrapProbeDiagnostic{
			class:   diagnostic.Class(),
			code:    diagnostic.Code(),
			feature: diagnostic.Feature(),
			source:  loc.Source(),
			line:    loc.Line(),
			column:  loc.Column(),
			specRef: diagnostic.SpecRef(),
		},
	}
}

func firstBootstrapProbeDiagnostic(err error) (goxsd9.Diagnostic, bool) {
	var diagnostics goxsd9.Diagnostics
	if errors.As(err, &diagnostics) {
		return diagnostics.At(0)
	}
	var diagnostic goxsd9.Diagnostic
	if errors.As(err, &diagnostic) {
		return diagnostic, true
	}
	return goxsd9.Diagnostic{}, false
}

type bootstrapProbeTransport struct {
	url   string
	body  []byte
	calls []string
}

func (transport *bootstrapProbeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	requestURL := request.URL.String()
	transport.calls = append(transport.calls, requestURL)
	if requestURL != transport.url {
		return nil, fmt.Errorf("unexpected bootstrap probe request %s", requestURL)
	}
	return testResponse(http.StatusOK, append([]byte(nil), transport.body...)), nil
}

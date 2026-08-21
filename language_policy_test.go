package goxsd9

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type languagePolicyReader struct {
	reader     *strings.Reader
	closeErr   error
	readCount  int
	closeCount int
}

func newLanguagePolicyReader(contents string) *languagePolicyReader {
	return &languagePolicyReader{reader: strings.NewReader(contents)}
}

func (reader *languagePolicyReader) Read(buffer []byte) (int, error) {
	reader.readCount++
	return reader.reader.Read(buffer)
}

func (reader *languagePolicyReader) Close() error {
	reader.closeCount++
	return reader.closeErr
}

type languagePolicyResolver struct {
	calls int
}

func (resolver *languagePolicyResolver) Resolve(context.Context, string, string) (ResolvedSource, error) {
	resolver.calls++
	return ResolvedSource{}, errors.New("resolver must not be called")
}

func newLanguagePolicyRoot(t *testing.T, reader io.ReadCloser) ResolvedSource {
	t.Helper()
	root, err := NewResolvedSource(context.Background(), "root.xsd", reader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	return root
}

func TestParseSchemaWithPolicyAcceptsDefinedValues(t *testing.T) {
	policies := []LanguagePolicy{Compatibility, Strict10, Strict11}
	for _, policy := range policies {
		t.Run(string(policy), func(t *testing.T) {
			reader := newLanguagePolicyReader(`<xs:schema xmlns:xs="` + xsdNamespaceURI + `"/>`)
			resolver := &languagePolicyResolver{}
			schema, err := ParseSchemaWithPolicy(newLanguagePolicyRoot(t, reader), resolver, policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy(%q): %v", policy, err)
			}
			if len(schema.Documents()) != 1 {
				t.Fatalf("document count = %d, want 1", len(schema.Documents()))
			}
			if resolver.calls != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolver.calls)
			}
			if reader.closeCount != 1 {
				t.Fatalf("root close count = %d, want 1", reader.closeCount)
			}
		})
	}
}

func TestParseSchemaWithPolicyRejectsInvalidValuesBeforeDiscovery(t *testing.T) {
	tests := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "zero", policy: LanguagePolicy("")},
		{name: "unknown", policy: LanguagePolicy("Legacy")},
		{name: "malformed case", policy: LanguagePolicy("compatibility")},
		{name: "malformed whitespace", policy: LanguagePolicy("Strict10 ")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := newLanguagePolicyReader("not XML and must not be read")
			resolver := &languagePolicyResolver{}
			schema, err := ParseSchemaWithPolicy(newLanguagePolicyRoot(t, reader), resolver, test.policy)
			assertInvalidLanguagePolicyResult(t, schema, err)
			if !errors.Is(err, errInvalidLanguagePolicy) {
				t.Fatalf("configuration cause was not preserved: %v", err)
			}
			if reader.readCount != 0 {
				t.Fatalf("root read count = %d, want 0", reader.readCount)
			}
			if reader.closeCount != 1 {
				t.Fatalf("root close count = %d, want 1", reader.closeCount)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolver.calls)
			}
		})
	}
}

func TestParseSchemaWithPolicyPreservesPolicyAndCloseFailures(t *testing.T) {
	closeCause := errors.New("root close failed")
	reader := newLanguagePolicyReader("not XML and must not be read")
	reader.closeErr = closeCause
	resolver := &languagePolicyResolver{}
	schema, err := ParseSchemaWithPolicy(newLanguagePolicyRoot(t, reader), resolver, LanguagePolicy("unknown"))
	assertInvalidLanguagePolicyResult(t, schema, err)
	if !errors.Is(err, errInvalidLanguagePolicy) {
		t.Fatalf("configuration cause was not preserved: %v", err)
	}
	if !errors.Is(err, closeCause) {
		t.Fatalf("close cause was not preserved: %v", err)
	}
	diagnostics := syntaxDiagnostics(err)
	if got, want := len(diagnostics), 2; got != want {
		t.Fatalf("diagnostic count = %d, want %d", got, want)
	}
	if got, want := diagnostics[0].Code(), InvalidLanguagePolicyCode; got != want {
		t.Fatalf("primary diagnostic code = %q, want %q", got, want)
	}
	if !diagnostics[0].Loc().IsZero() {
		t.Fatalf("primary diagnostic location = %s, want zero", diagnostics[0].Loc())
	}
	if got, want := diagnostics[1].Code(), SourceCloseCode; got != want {
		t.Fatalf("close diagnostic code = %q, want %q", got, want)
	}
	if !diagnostics[1].Loc().IsZero() {
		t.Fatalf("close diagnostic location = %s, want zero", diagnostics[1].Loc())
	}
	if reader.readCount != 0 {
		t.Fatalf("root read count = %d, want 0", reader.readCount)
	}
	if reader.closeCount != 1 {
		t.Fatalf("root close count = %d, want 1", reader.closeCount)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
}

func TestParseSchemaWithPolicyInvalidPolicyHandlesZeroRoot(t *testing.T) {
	resolver := &languagePolicyResolver{}
	schema, err := ParseSchemaWithPolicy(ResolvedSource{}, resolver, LanguagePolicy("unknown"))
	assertInvalidLanguagePolicyResult(t, schema, err)
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
}

func TestParseSchemaWithPolicyValidPolicyHandlesZeroRoot(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := ParseSchemaWithPolicy(ResolvedSource{}, nil, policy)
			if err == nil {
				t.Fatal("ParseSchemaWithPolicy accepted a zero root")
			}
			if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatal("zero-root parse returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("diagnostic class = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), SourceInvalidCode; got != want {
				t.Fatalf("diagnostic code = %q, want %q", got, want)
			}
		})
	}
}

func assertInvalidLanguagePolicyResult(t *testing.T, schema Schema, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("ParseSchemaWithPolicy accepted an invalid policy")
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("invalid-policy parse returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("diagnostic class = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), InvalidLanguagePolicyCode; got != want {
		t.Fatalf("diagnostic code = %q, want %q", got, want)
	}
	if !diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic location = %s, want zero", diagnostic.Loc())
	}
}

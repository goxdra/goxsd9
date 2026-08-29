package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNamedGroupDirectChoiceIsLocatedUnsupported(t *testing.T) {
	tests := []struct {
		name      string
		policy    LanguagePolicy
		version   string
		groupName string
		children  string
	}{
		{
			name:      "XSD 1.0 structures schemaTop",
			policy:    Strict10,
			version:   "1.0",
			groupName: "schemaTop",
			children:  `<xs:element name="schema" type="xs:string"/>`,
		},
		{
			name:      "XSD 1.1 structures schemaTop",
			policy:    Strict11,
			version:   "1.1",
			groupName: "schemaTop",
			children:  `<xs:element name="schema" type="xs:string"/><xs:element name="component" type="xs:string"/>`,
		},
		{
			name:      "XSD 1.1 datatypes datatypeTop",
			policy:    Strict11,
			version:   "1.1",
			groupName: "datatypeTop",
			children:  `<xs:element name="decimal" type="xs:string"/>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := namedGroupChoiceSchema(test.version, test.groupName, "", "", test.children)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			assertNamedGroupChoiceUnsupported(t, schema, err, root, "<xs:choice")
		})
	}
}

//nolint:gocognit,funlen // Keep the named-group invalid diagnostic matrix together.
func TestNamedGroupDirectChoicePreservesInvalidDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		groupName   string
		groupAttrs  string
		choiceAttrs string
		children    string
		marker      string
		code        string
		message     string
	}{
		{
			name:       "invalid group attribute",
			groupAttrs: ` bogus="value"`,
			children:   `<xs:element name="value" type="xs:string"/>`,
			marker:     `bogus="value"`,
			code:       invalidSchemaCompositionCode,
			message:    "not permitted",
		},
		{
			name:      "invalid group name",
			groupName: "bad:name",
			children:  `<xs:element name="value" type="xs:string"/>`,
			marker:    `name="bad:name"`,
			code:      invalidSchemaDeclarationNameCode,
			message:   "valid NCName",
		},
		{
			name:        "invalid choice attribute",
			choiceAttrs: ` bogus="value"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `bogus="value"`,
			code:        invalidSchemaCompositionCode,
			message:     "forbidden attribute",
		},
		{
			name:        "malformed occurrence lexical",
			choiceAttrs: ` minOccurs="many"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `minOccurs="many"`,
			code:        invalidSchemaCompositionCode,
			message:     "invalid occurrence value",
		},
		{
			name:        "occurrence range",
			choiceAttrs: ` minOccurs="2" maxOccurs="1"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      "<xs:choice",
			code:        invalidSchemaCompositionCode,
			message:     "cannot exceed",
		},
		{
			name:        "duplicate occurrence lexical",
			choiceAttrs: ` minOccurs="0" minOccurs="1"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `minOccurs="1"`,
			code:        InvalidXMLSyntaxCode,
			message:     `duplicate attribute "minOccurs"`,
		},
		{
			name:     "annotation follows content",
			children: `<xs:element name="value" type="xs:string"/><xs:annotation/>`,
			marker:   "<xs:annotation",
			code:     invalidSchemaCompositionCode,
			message:  "annotation must be first",
		},
		{
			name:     "all is forbidden in choice",
			children: `<xs:all/>`,
			marker:   "<xs:all",
			code:     invalidSchemaCompositionCode,
			message:  "cannot contain an all particle",
		},
		{
			name:     "local element requires a declaration name",
			children: `<xs:element/>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
		{
			name:     "local element QName is malformed",
			children: `<xs:element name="value" type="bad:q:name"/>`,
			marker:   `type="bad:q:name"`,
			code:     invalidSchemaConditionalCode,
			message:  "malformed QName",
		},
		{
			name:     "local element QName is unbound",
			children: `<xs:element name="value" type="bad:Type"/>`,
			marker:   `type="bad:Type"`,
			code:     invalidSchemaConditionalCode,
			message:  "unbound QName prefix",
		},
		{
			name:        "staged occurrence unsupported yields to invalid child",
			choiceAttrs: ` minOccurs="0"`,
			children:    `<xs:element/>`,
			marker:      "<xs:element/>",
			code:        invalidSchemaDeclarationNameCode,
			message:     "requires a name or ref",
		},
		{
			name:     "staged namespace policy yields to invalid child",
			children: `<xs:element name="value" type="xs:string" form="qualified"/><xs:element/>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
		{
			name:     "nested choice validates its subtree",
			children: `<xs:choice><xs:element/></xs:choice>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
	}
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "XSD 1.0", policy: Strict10, version: "1.0"},
		{name: "XSD 1.1", policy: Strict11, version: "1.1"},
	} {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupChoiceSchema(profile.version, test.groupName, test.groupAttrs, test.choiceAttrs, test.children)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted malformed named-group choice")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				if diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) {
					t.Fatalf("diagnostic location = %s, want marker %q", diagnostic.Loc(), test.marker)
				}
				if test.message != "" && !strings.Contains(diagnostic.Message(), test.message) {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.message)
				}
				if errors.Is(err, ErrUnsupported) {
					t.Fatalf("invalid named-group choice matches ErrUnsupported: %v", err)
				}
				if errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("invalid named-group choice retained a policy mismatch: %v", err)
				}
			})
		}
	}
}

func TestNamedGroupDirectChoiceKeepsValidOccurrenceAsUnsupported(t *testing.T) {
	root := namedGroupChoiceSchema("1.1", "G", "", ` minOccurs="0" maxOccurs="1"`, `<xs:element name="value" type="xs:string"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertNamedGroupChoiceUnsupported(t, schema, err, root, `minOccurs="0"`)
	diagnostic := requireDiagnostic(t, err)
	if !strings.Contains(diagnostic.Message(), `particle attribute "minOccurs" is not implemented`) {
		t.Fatalf("diagnostic message = %q, want unsupported occurrence attribute", diagnostic.Message())
	}
}

func TestNamedGroupDirectChoiceParsingIsDeterministic(t *testing.T) {
	root := namedGroupChoiceSchema("1.1", "G", ` minOccurs="0"`, "", `<xs:element/>`)
	var first Diagnostic
	var firstErr string
	for iteration := 0; iteration < 16; iteration++ {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if iteration == 0 {
			first = diagnostic
			firstErr = err.Error()
			continue
		}
		if err.Error() != firstErr || diagnostic.Class() != first.Class() || diagnostic.Code() != first.Code() ||
			diagnostic.Feature() != first.Feature() || diagnostic.Loc() != first.Loc() ||
			diagnostic.Message() != first.Message() || diagnostic.SpecRef() != first.SpecRef() ||
			!reflect.DeepEqual(diagnostic.Related(), first.Related()) {
			t.Fatalf("iteration %d diagnostic changed: got %s, want %s", iteration, diagnostic, firstErr)
		}
	}
}

func namedGroupChoiceSchema(version, groupName, groupAttrs, choiceAttrs, children string) string {
	if groupName == "" {
		groupName = "G"
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `">
  <xs:group name="` + groupName + `"` + groupAttrs + `>
    <xs:choice` + choiceAttrs + `>` + children + `</xs:choice>
  </xs:group>
</xs:schema>`
}

func assertNamedGroupChoiceUnsupported(t *testing.T, schema Schema, err error, root, marker string) {
	t.Helper()
	if err == nil {
		t.Fatal("discoverSchema accepted unsupported named-group choice")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want unsupported/%q/%q", diagnostic, FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, root, marker) {
		t.Fatalf("diagnostic location = %s, want marker %q", diagnostic.Loc(), marker)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("valid named-group choice retained a policy mismatch: %v", err)
	}
}

func assertZeroSchema(t *testing.T, schema Schema) {
	t.Helper()
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema")
	}
}

func namedGroupChoiceLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.Index(root, marker)
	if index < 0 {
		t.Fatalf("fixture does not contain location marker %q", marker)
	}
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	if column != utf8.RuneCountInString(root[strings.LastIndex(root[:index], "\n")+1:index])+1 {
		t.Fatalf("fixture marker %q has inconsistent column calculation", marker)
	}
	return mustTestLoc(t, "root.xsd", line, column)
}

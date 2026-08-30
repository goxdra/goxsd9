package goxsd9_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

type elementNamespaceTestResolver struct {
	documents map[string]string
}

func (resolver elementNamespaceTestResolver) Resolve(
	ctx context.Context,
	_, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	contents, ok := resolver.documents[schemaLocation]
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("missing element namespace document %q", schemaLocation)
	}
	return goxsd9.NewResolvedSource(ctx, goxsd9.SourceID(schemaLocation), io.NopCloser(strings.NewReader(contents)))
}

func parseElementNamespaceSchema(
	t *testing.T,
	policy goxsd9.LanguagePolicy,
	root string,
	documents map[string]string,
) goxsd9.Schema {
	t.Helper()
	schema, err := parseElementNamespaceSchemaResult(t, policy, root, documents)
	if err != nil {
		t.Fatalf("ParseSchemaWithPolicy: %v", err)
	}
	return schema
}

func parseElementNamespaceSchemaResult(
	t *testing.T,
	policy goxsd9.LanguagePolicy,
	root string,
	documents map[string]string,
) (goxsd9.Schema, error) {
	t.Helper()
	source, err := goxsd9.NewResolvedSource(
		context.Background(),
		"root.xsd",
		io.NopCloser(strings.NewReader(root)),
	)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	var resolver goxsd9.Resolver
	if len(documents) != 0 {
		resolver = elementNamespaceTestResolver{documents: documents}
	}
	return goxsd9.ParseSchemaWithPolicy(source, resolver, policy)
}

func elementNamespaceChoice(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.ChoiceParticle {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s component count = %d, want 1", name, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex type definition", name)
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatalf("%s particle = %T, want ChoiceParticle", name, definition.Particle())
	}
	return choice
}

func elementNamespaceSequence(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.SequenceParticle {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s component count = %d, want 1", name, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex type definition", name)
	}
	sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
	if !ok {
		t.Fatalf("%s particle = %T, want SequenceParticle", name, definition.Particle())
	}
	return sequence
}

func elementNamespaceAlternative(t *testing.T, alternatives []goxsd9.Particle, index int) goxsd9.ElementParticle {
	t.Helper()
	if index < 0 || index >= len(alternatives) {
		t.Fatalf("alternative index %d is outside %d alternatives", index, len(alternatives))
	}
	element, ok := alternatives[index].(goxsd9.ElementParticle)
	if !ok {
		t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternatives[index])
	}
	return element
}

func elementNamespaceDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T does not contain a Diagnostic: %v", err, err)
	}
	return diagnostic
}

func assertElementNamespaceFailure(t *testing.T, schema goxsd9.Schema, err error, class goxsd9.FailureClass) goxsd9.Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("ParseSchemaWithPolicy accepted invalid or unsupported schema")
	}
	if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("ParseSchemaWithPolicy returned a partial schema")
	}
	diagnostic := elementNamespaceDiagnostic(t, err)
	if diagnostic.Class() != class {
		t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), class)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic location = %s, want located root.xsd diagnostic", diagnostic.Loc())
	}
	return diagnostic
}

//nolint:gocognit // Keep all direct-choice namespace policy combinations together.
func TestParseSchemaDirectChoiceUsesElementNamespacePolicy(t *testing.T) {
	tests := []struct {
		name             string
		defaultAttribute string
		wantDefault      string
	}{
		{name: "default absent", wantDefault: ""},
		{name: "default unqualified", defaultAttribute: ` elementFormDefault="unqualified"`, wantDefault: ""},
		{name: "default qualified", defaultAttribute: ` elementFormDefault="  qualified  "`, wantDefault: "urn:root"},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace=" urn:root "` + test.defaultAttribute + `>` +
					`<xs:complexType name="Choice"><xs:choice>` +
					`<xs:element name="  defaulted  " type="xs:integer"/>` +
					`<xs:element name="qualified" type="xs:decimal" form=" qualified "/>` +
					`<xs:element name="unqualified" type="xs:integer" form="unqualified"/>` +
					`</xs:choice></xs:complexType></xs:schema>`
				schema := parseElementNamespaceSchema(t, policy, root, nil)
				choice := elementNamespaceChoice(t, schema, "urn:root", "Choice")
				alternatives := choice.Alternatives()
				if len(alternatives) != 3 {
					t.Fatalf("alternative count = %d, want 3", len(alternatives))
				}
				wantNamespaces := []string{test.wantDefault, "urn:root", ""}
				wantLocals := []string{"defaulted", "qualified", "unqualified"}
				for index, alternative := range alternatives {
					element, ok := alternative.(goxsd9.ElementParticle)
					if !ok {
						t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternative)
					}
					if got := element.Name().Namespace(); got != wantNamespaces[index] {
						t.Fatalf("alternative %d namespace = %q, want %q", index, got, wantNamespaces[index])
					}
					if got := element.Name().Local(); got != wantLocals[index] {
						t.Fatalf("alternative %d local = %q, want %q", index, got, wantLocals[index])
					}
				}
			})
		}
	}
}

func TestParseSchemaDirectChoiceUsesNoNamespaceForQualifiedPolicyWithoutTarget(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" elementFormDefault="qualified"><xs:complexType name="Choice"><xs:choice>` +
		`<xs:element name="defaulted" type="xs:integer"/>` +
		`<xs:element name="explicit" type="xs:integer" form="qualified"/>` +
		`</xs:choice></xs:complexType></xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := parseElementNamespaceSchema(t, policy, root, nil)
			alternatives := elementNamespaceChoice(t, schema, "", "Choice").Alternatives()
			for index, alternative := range alternatives {
				element, ok := alternative.(goxsd9.ElementParticle)
				if !ok {
					t.Fatalf("alternative %d type = %T, want ElementParticle", index, alternative)
				}
				if got := element.Name().Namespace(); got != "" {
					t.Fatalf("alternative %d namespace = %q, want empty", index, got)
				}
			}
		})
	}
}

//nolint:gocognit // Keep targetNamespace policy and diagnostic precedence together.
func TestParseSchemaDirectChoiceTargetNamespacePolicy(t *testing.T) {
	equal := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace=" urn:root "><xs:complexType name="Choice"><xs:choice>` +
		`<xs:element name="value" type="xs:integer" targetNamespace=" urn:root "/>` +
		`</xs:choice></xs:complexType></xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy)+"/equal", func(t *testing.T) {
			schema := parseElementNamespaceSchema(t, policy, equal, nil)
			alternatives := elementNamespaceChoice(t, schema, "urn:root", "Choice").Alternatives()
			alternative := elementNamespaceAlternative(t, alternatives, 0)
			if got := alternative.Name().String(); got != "{urn:root}value" {
				t.Fatalf("targetNamespace name = %q, want {urn:root}value", got)
			}
		})
	}

	cases := []struct {
		name        string
		root        string
		wantRelated int
		wantMessage string
	}{
		{
			name:        "differing target",
			root:        `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer" targetNamespace="urn:other"/></xs:choice></xs:complexType></xs:schema>`,
			wantRelated: 1,
			wantMessage: "local element targetNamespace must match the containing schema targetNamespace",
		},
		{
			name:        "no containing target",
			root:        `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer" targetNamespace="urn:other"/></xs:choice></xs:complexType></xs:schema>`,
			wantMessage: "local element targetNamespace requires a containing schema targetNamespace",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			schema, err := parseElementNamespaceSchemaResult(t, goxsd9.Strict11, test.root, nil)
			diagnostic := assertElementNamespaceFailure(t, schema, err, goxsd9.FailureInvalid)
			if diagnostic.Code() != "XSD3010" || diagnostic.SpecRef() != "xsd11-structures#dcl.elt.local" {
				t.Fatalf("diagnostic metadata = %q/%q, want XSD3010/local-element specification", diagnostic.Code(), diagnostic.SpecRef())
			}
			if diagnostic.Message() != test.wantMessage {
				t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
			}
			if got := len(diagnostic.Related()); got != test.wantRelated {
				t.Fatalf("related location count = %d, want %d", got, test.wantRelated)
			}
			if diagnostic.Unwrap() == nil {
				t.Fatal("targetNamespace representation diagnostic lost its cause")
			}
		})
	}

	strict10Schema, strict10Err := parseElementNamespaceSchemaResult(t, goxsd9.Strict10, equal, nil)
	strict10 := assertElementNamespaceFailure(t, strict10Schema, strict10Err, goxsd9.FailureUnsupported)
	if strict10.Code() != goxsd9.UnsupportedSchemaSyntaxCode || strict10.Feature() != goxsd9.FeatureSchemaSyntax {
		t.Fatalf("Strict10 diagnostic = %q/%q, want schema-syntax unsupported", strict10.Code(), strict10.Feature())
	}
	if !errors.Is(strict10Err, goxsd9.ErrUnsupported) {
		t.Fatalf("Strict10 diagnostic does not match ErrUnsupported: %v", strict10Err)
	}

	conflict := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer" targetNamespace="urn:root" form="qualified"/></xs:choice></xs:complexType></xs:schema>`
	conflictSchema, conflictErr := parseElementNamespaceSchemaResult(t, goxsd9.Strict11, conflict, nil)
	conflictDiagnostic := assertElementNamespaceFailure(t, conflictSchema, conflictErr, goxsd9.FailureInvalid)
	if conflictDiagnostic.Code() != "XSD3010" || !strings.Contains(conflictDiagnostic.Message(), "cannot combine with form") {
		t.Fatalf("target/form conflict diagnostic = %s, want located composition conflict", conflictDiagnostic)
	}
}

type attributeFormDefaultComponentQuery struct {
	id           goxsd9.ComponentID
	kind         goxsd9.ComponentKind
	name         goxsd9.QName
	loc          goxsd9.Loc
	declaredType goxsd9.QName
}

type attributeFormDefaultDocumentQuery struct {
	source          goxsd9.SourceID
	rootLoc         goxsd9.Loc
	targetNamespace string
	components      []attributeFormDefaultComponentQuery
}

type attributeFormDefaultSchemaQuery struct {
	documents  []attributeFormDefaultDocumentQuery
	components []attributeFormDefaultComponentQuery
	found      []attributeFormDefaultComponentQuery
	lookup     attributeFormDefaultComponentQuery
}

func attributeFormDefaultComponentQueries(components []goxsd9.Component) []attributeFormDefaultComponentQuery {
	queries := make([]attributeFormDefaultComponentQuery, 0, len(components))
	for _, component := range components {
		query := attributeFormDefaultComponentQuery{
			id:   component.ID(),
			kind: component.Kind(),
			name: component.Name(),
			loc:  component.Loc(),
		}
		if declaration, ok := component.Element(); ok {
			query.declaredType = declaration.DeclaredType()
		}
		queries = append(queries, query)
	}
	return queries
}

func attributeFormDefaultQuerySnapshot(t *testing.T, schema goxsd9.Schema) attributeFormDefaultSchemaQuery {
	t.Helper()
	documents := schema.Documents()
	components := schema.Components()
	if len(documents) != 1 || len(components) != 1 {
		t.Fatalf("schema queries = %d documents/%d components, want 1/1", len(documents), len(components))
	}
	name, err := goxsd9.NewQName("urn:root", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(found) != 1 {
		t.Fatalf("FindKind item count = %d, want 1", len(found))
	}
	lookup, ok := schema.Lookup(components[0].ID())
	if !ok {
		t.Fatal("Lookup did not find the declared component")
	}
	return attributeFormDefaultSchemaQuery{
		documents: []attributeFormDefaultDocumentQuery{{
			source:          documents[0].Source(),
			rootLoc:         documents[0].RootLoc(),
			targetNamespace: documents[0].TargetNamespace(),
			components:      attributeFormDefaultComponentQueries(documents[0].Components()),
		}},
		components: attributeFormDefaultComponentQueries(components),
		found:      attributeFormDefaultComponentQueries(found),
		lookup:     attributeFormDefaultComponentQueries([]goxsd9.Component{lookup})[0],
	}
}

func attributeFormDefaultSchemaRoot(attribute string) string {
	return `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root"` + attribute + `>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
}

func attributeFormDefaultLocation(t *testing.T, root string) goxsd9.Loc {
	t.Helper()
	index := strings.LastIndex(root, "attributeFormDefault")
	if index < 0 {
		t.Fatalf("root does not contain attributeFormDefault: %q", root)
	}
	loc, err := goxsd9.NewLoc("root.xsd", 1, index+1)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

type attributeFormDefaultDiagnosticCase struct {
	name      string
	attribute string
	class     goxsd9.FailureClass
	code      string
	feature   goxsd9.FeatureID
	message   string
}

func assertAttributeFormDefaultDiagnostic(t *testing.T, policy goxsd9.LanguagePolicy, test attributeFormDefaultDiagnosticCase) {
	t.Helper()
	root := attributeFormDefaultSchemaRoot(test.attribute)
	schema, err := parseElementNamespaceSchemaResult(t, policy, root, nil)
	diagnostic := assertElementNamespaceFailure(t, schema, err, test.class)
	if diagnostic.Code() != test.code || diagnostic.Message() != test.message {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.code, test.message)
	}
	if diagnostic.Loc() != attributeFormDefaultLocation(t, root) {
		t.Fatalf("diagnostic location = %s, want attribute location", diagnostic.Loc())
	}
	if test.feature != "" && diagnostic.Feature() != test.feature {
		t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
	}
	if test.class == goxsd9.FailureUnsupported {
		if !errors.Is(err, goxsd9.ErrUnsupported) {
			t.Fatalf("unsupported diagnostic lost ErrUnsupported: %v", err)
		}
		return
	}
	if errors.Is(err, goxsd9.ErrUnsupported) || diagnostic.Unwrap() != nil {
		t.Fatalf("invalid diagnostic retained an unsupported cause: %v", err)
	}
}

func TestParseSchemaAcceptsAttributeFormDefaultUnqualifiedAsDefault(t *testing.T) {
	tests := []struct {
		name      string
		attribute string
	}{
		{name: "absent"},
		{name: "explicit unqualified", attribute: ` attributeFormDefault="unqualified"`},
		{name: "XML-whitespace-padded unqualified", attribute: " attributeFormDefault=\" \t unqualified \t \""},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		var want attributeFormDefaultSchemaQuery
		for index, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := attributeFormDefaultSchemaRoot(test.attribute)
				schema, err := parseElementNamespaceSchemaResult(t, policy, root, nil)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				got := attributeFormDefaultQuerySnapshot(t, schema)
				if index == 0 {
					want = got
					return
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("public schema queries differ from absent default:\n got: %#v\nwant: %#v", got, want)
				}
			})
		}
	}
}

func TestParseSchemaAttributeFormDefaultDiagnostics(t *testing.T) {
	tests := []attributeFormDefaultDiagnosticCase{
		{
			name:      "XML-whitespace-padded qualified remains unsupported",
			attribute: " attributeFormDefault=\" \t qualified \t \"",
			class:     goxsd9.FailureUnsupported,
			code:      goxsd9.UnsupportedSchemaSyntaxCode,
			feature:   goxsd9.FeatureSchemaSyntax,
			message:   `schema root attribute "attributeFormDefault" is not implemented`,
		},
		{
			name:      "empty value remains invalid",
			attribute: ` attributeFormDefault=""`,
			class:     goxsd9.FailureInvalid,
			code:      "XSD3010",
			message:   `attribute "attributeFormDefault" has an invalid value`,
		},
		{
			name:      "XML-whitespace-only value remains invalid",
			attribute: " attributeFormDefault=\" \t \t \"",
			class:     goxsd9.FailureInvalid,
			code:      "XSD3010",
			message:   `attribute "attributeFormDefault" has an invalid value`,
		},
		{
			name:      "malformed value remains invalid",
			attribute: ` attributeFormDefault="maybe"`,
			class:     goxsd9.FailureInvalid,
			code:      "XSD3010",
			message:   `attribute "attributeFormDefault" has an invalid value`,
		},
		{
			name:      "duplicate expanded root attribute takes precedence",
			attribute: ` attributeFormDefault="unqualified" attributeFormDefault="qualified"`,
			class:     goxsd9.FailureInvalid,
			code:      goxsd9.InvalidXMLSyntaxCode,
			message:   `duplicate attribute "attributeFormDefault"`,
		},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				assertAttributeFormDefaultDiagnostic(t, policy, test)
			})
		}
	}
}

//nolint:gocognit // Keep namespace-default boundaries and their classifications together.
func TestParseSchemaAttributeFormDefaultDoesNotClaimLocalAttributes(t *testing.T) {
	tests := []struct {
		name            string
		child           string
		class           goxsd9.FailureClass
		code            string
		wantUnsupported bool
	}{
		{name: "local declaration", child: `<xs:attribute name="item" type="xs:string"/>`, class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode, wantUnsupported: true},
		{name: "local default", child: `<xs:attribute name="item" default="value"/>`, class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode, wantUnsupported: true},
		{name: "local fixed", child: `<xs:attribute name="item" fixed="value"/>`, class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode, wantUnsupported: true},
		{name: "local reference", child: `<xs:attribute ref="item"/>`, class: goxsd9.FailureInvalid, code: "XSD3045"},
		{name: "attribute group reference", child: `<xs:attributeGroup ref="items"/>`, class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode, wantUnsupported: true},
	}
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" attributeFormDefault="unqualified"><xs:complexType name="Record">` + test.child + `</xs:complexType></xs:schema>`
				schema, err := parseElementNamespaceSchemaResult(t, policy, root, nil)
				diagnostic := assertElementNamespaceFailure(t, schema, err, test.class)
				if diagnostic.Code() != test.code {
					t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), test.code)
				}
				if test.wantUnsupported {
					if diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("local attribute diagnostic = %s, want schema-syntax unsupported", diagnostic)
					}
					return
				}
				if errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("local reference diagnostic was classified as unsupported: %v", err)
				}
			})
		}
	}
}

func TestParseSchemaDirectChoiceUsesDocumentLocalDefaultsAcrossIncludeAndImport(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root" elementFormDefault="unqualified">` +
		`<xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:child" schemaLocation="imported.xsd"/>` +
		`<xs:complexType name="RootChoice"><xs:choice><xs:element name="rootValue" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`
	included := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root" elementFormDefault="qualified">` +
		`<xs:complexType name="IncludedChoice"><xs:choice><xs:element name="includedValue" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`
	imported := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:child">` +
		`<xs:complexType name="ImportedChoice"><xs:choice><xs:element name="importedValue" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`
	schema := parseElementNamespaceSchema(t, goxsd9.Strict11, root, map[string]string{
		"included.xsd": included,
		"imported.xsd": imported,
	})
	for _, test := range []struct {
		typeNamespace string
		typeName      string
		wantNamespace string
	}{
		{typeNamespace: "urn:root", typeName: "RootChoice", wantNamespace: ""},
		{typeNamespace: "urn:root", typeName: "IncludedChoice", wantNamespace: "urn:root"},
		{typeNamespace: "urn:child", typeName: "ImportedChoice", wantNamespace: ""},
	} {
		choice := elementNamespaceChoice(t, schema, test.typeNamespace, test.typeName)
		element := elementNamespaceAlternative(t, choice.Alternatives(), 0)
		if got := element.Name().Namespace(); got != test.wantNamespace {
			t.Fatalf("%s namespace = %q, want %q", test.typeName, got, test.wantNamespace)
		}
	}
}

func TestParseSchemaKeepsExcludedSequenceNamespacePolicyUnsupported(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:root"%s><xs:complexType name="Record"><xs:sequence>%s</xs:sequence></xs:complexType></xs:schema>`
	qualifiedDefault := fmt.Sprintf(base, ` elementFormDefault="qualified"`, `<xs:element name="value" type="xs:integer"/>`)
	sequenceSchema, sequenceErr := parseElementNamespaceSchemaResult(t, goxsd9.Strict11, qualifiedDefault, nil)
	sequenceDiagnostic := assertElementNamespaceFailure(t, sequenceSchema, sequenceErr, goxsd9.FailureUnsupported)
	if sequenceDiagnostic.Loc().Line() != 1 || !strings.Contains(sequenceDiagnostic.Message(), "elementFormDefault=qualified") {
		t.Fatalf("qualified sequence diagnostic = %s, want located default boundary", sequenceDiagnostic)
	}

	defaultUnqualified := fmt.Sprintf(base, ``, `<xs:element name="value" type="xs:integer"/>`)
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy)+"/default-unqualified", func(t *testing.T) {
			schema := parseElementNamespaceSchema(t, policy, defaultUnqualified, nil)
			elements := elementNamespaceSequence(t, schema, "urn:root", "Record").Elements()
			if len(elements) != 1 || elements[0].Name().Namespace() != "" {
				t.Fatalf("sequence elements = %#v, want one unqualified element", elements)
			}
		})
	}

	for _, particle := range []struct {
		name string
		body string
	}{
		{name: "explicit form", body: `<xs:element name="value" type="xs:integer" form="unqualified"/>`},
		{name: "explicit target", body: `<xs:element name="value" type="xs:integer" targetNamespace="urn:root"/>`},
	} {
		t.Run(particle.name, func(t *testing.T) {
			root := fmt.Sprintf(base, ``, particle.body)
			schema, err := parseElementNamespaceSchemaResult(t, goxsd9.Strict11, root, nil)
			diagnostic := assertElementNamespaceFailure(t, schema, err, goxsd9.FailureUnsupported)
			if diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("excluded sequence diagnostic = %s, want schema-syntax unsupported", diagnostic)
			}
		})
	}
}

func TestParseSchemaDirectChoicePreservesTypeQNameIdentityOrderAndImmutability(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:root" xmlns:alias="urn:root" targetNamespace="urn:root">` +
		`<xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType>` +
		`<xs:complexType name="Choice"><xs:choice>` +
		`<xs:element name="first" type="r:Amount"/><xs:element name="second" type="alias:Amount"/>` +
		`</xs:choice></xs:complexType></xs:schema>`
	schema := parseElementNamespaceSchema(t, goxsd9.Strict11, root, nil)
	choice := elementNamespaceChoice(t, schema, "urn:root", "Choice")
	alternatives := choice.Alternatives()
	if len(alternatives) != 2 {
		t.Fatalf("alternative count = %d, want 2", len(alternatives))
	}
	first, firstOK := alternatives[0].(goxsd9.ElementParticle)
	second, secondOK := alternatives[1].(goxsd9.ElementParticle)
	if !firstOK || !secondOK {
		t.Fatalf("alternatives have types %T and %T, want ElementParticle values", alternatives[0], alternatives[1])
	}
	if first.Name().Local() != "first" || second.Name().Local() != "second" {
		t.Fatalf("choice order = %q, %q, want first, second", first.Name().Local(), second.Name().Local())
	}
	if first.DeclaredType() != second.DeclaredType() || first.DeclaredType().String() != "{urn:root}Amount" {
		t.Fatalf("type QNames = %s and %s, want equal {urn:root}Amount", first.DeclaredType(), second.DeclaredType())
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, parseTestQName(t, "urn:root", "Amount"))
	if len(components) != 1 {
		t.Fatalf("Amount component count = %d, want 1", len(components))
	}
	amountID := components[0].ID()
	if firstID, ok := first.TypeID(); !ok || firstID != amountID {
		t.Fatalf("first type ID = (%v, %t), want (%v, true)", firstID, ok, amountID)
	}
	if secondID, ok := second.TypeID(); !ok || secondID != amountID {
		t.Fatalf("second type ID = (%v, %t), want (%v, true)", secondID, ok, amountID)
	}
	alternatives[0] = nil
	if got := elementNamespaceAlternative(t, choice.Alternatives(), 0).Name().Local(); got != "first" {
		t.Fatalf("mutating Alternatives changed choice order to %q", got)
	}
}

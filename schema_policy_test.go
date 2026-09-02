package goxsd9_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/goxdra/goxsd9"
)

const policyTestXSDVersioningNamespace = "http://www.w3.org/2007/XMLSchema-versioning"

func TestParseSchemaWithPolicyPropagatesAcrossMixedGraph(t *testing.T) {
	tests := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version goxsd9.XSDVersion
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rootReader := newParseTestReader(parseTestRootDocument)
			rootContext := context.WithValue(context.Background(), parseTestContextKey{}, "root")
			root, err := goxsd9.NewResolvedSource(rootContext, "opaque-root", rootReader)
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			resolver := &parseTestResolver{}

			schema, err := goxsd9.ParseSchemaWithPolicy(root, resolver, test.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			if got, want := len(schema.Documents()), 3; got != want {
				t.Fatalf("document count = %d, want %d", got, want)
			}
			if got, want := len(schema.Components()), 5; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			assertParseTestResolverCalls(t, resolver)
			assertParseTestSourcesClosed(t, rootReader, resolver)
			assertParseTestPolicyFacetVersions(t, schema, test.version)
			assertSchemaLanguagePolicyQueries(t, schema, test.policy)
		})
	}
}

func TestParseSchemaTreatsAllUnqualifiedVersionLabelsAsInert(t *testing.T) {
	labels := []struct {
		name      string
		attribute string
	}{
		{name: "absent"},
		{name: "empty", attribute: ` version=""`},
		{name: "arbitrary", attribute: ` version="release-2026"`},
		{name: "XSD 1.0", attribute: ` version="1.0"`},
		{name: "XSD 1.1", attribute: ` version="1.1"`},
	}
	for _, label := range labels {
		t.Run("ParseSchema/"+label.name, func(t *testing.T) {
			assertParseSchemaVersionLabel(t, label.attribute, goxsd9.Compatibility, false)
		})
	}

	policies := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version goxsd9.XSDVersion
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	}
	for _, policy := range policies {
		for _, label := range labels {
			name := policy.name + "/" + label.name
			t.Run(name, func(t *testing.T) {
				assertParseSchemaVersionLabel(t, label.attribute, policy.policy, true)
			})
		}
	}
}

func assertParseSchemaVersionLabel(t *testing.T, attribute string, policy goxsd9.LanguagePolicy, explicitPolicy bool) {
	t.Helper()
	rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `"` + attribute + `><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:schema>`
	rootReader := newParseTestReader(rootContents)
	root, err := goxsd9.NewResolvedSource(context.Background(), "label-root", rootReader)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}

	var schema goxsd9.Schema
	if explicitPolicy {
		schema, err = goxsd9.ParseSchemaWithPolicy(root, nil, policy)
	}
	if !explicitPolicy {
		schema, err = goxsd9.ParseSchema(root, nil)
	}
	if err != nil {
		t.Fatalf("parse schema label: %v", err)
	}
	if got, want := schema.LanguagePolicy(), policy; got != want {
		t.Fatalf("LanguagePolicy = %q, want %q", got, want)
	}
	if got, want := len(schema.Components()), 1; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	definition, ok := schema.Components()[0].SimpleType()
	if !ok {
		t.Fatal("simple type view is missing")
	}
	wantVersion := goxsd9.XSDVersion11
	if explicitPolicy {
		wantVersion = policyXSDVersion(policy)
	}
	if got := definition.DigitFacets().Version(); got != wantVersion {
		t.Fatalf("facet version = %q, want %q", got, wantVersion)
	}
	if got, want := rootReader.closeCount, 1; got != want {
		t.Fatalf("root close count = %d, want %d", got, want)
	}
}

func TestSchemaLanguagePolicyZeroValueIsEmpty(t *testing.T) {
	if got, want := (goxsd9.Schema{}).LanguagePolicy(), goxsd9.LanguagePolicy(""); got != want {
		t.Fatalf("zero Schema LanguagePolicy = %q, want empty", got)
	}
}

func assertSchemaLanguagePolicyQueries(t *testing.T, schema goxsd9.Schema, want goxsd9.LanguagePolicy) {
	t.Helper()
	for query := 0; query < 3; query++ {
		if got := schema.LanguagePolicy(); got != want {
			t.Fatalf("LanguagePolicy query %d = %q, want %q", query, got, want)
		}
	}

	schemaCopy := schema
	if got := schemaCopy.LanguagePolicy(); got != want {
		t.Fatalf("copied Schema LanguagePolicy = %q, want %q", got, want)
	}
	documents := schema.Documents()
	if len(documents) != 0 {
		documents[0] = goxsd9.SchemaDocument{}
	}
	components := schema.Components()
	if len(components) != 0 {
		components[0] = goxsd9.Component{}
	}
	if got := schema.LanguagePolicy(); got != want {
		t.Fatalf("LanguagePolicy after copy mutations = %q, want %q", got, want)
	}
}

//nolint:gocognit // Keep policy, graph, diagnostic, and closure assertions together.
func TestParseSchemaWithPolicyUsesUniformGrammarAcrossGraph(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" version="1.0"><xs:include schemaLocation="child.xsd"/></xs:schema>`
	childContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" version="1.1"><xs:complexType name="item"><xs:choice><xs:element name="value" type="xs:integer"><xs:alternative type="xs:integer"/></xs:element></xs:choice></xs:complexType></xs:schema>`
	tests := []struct {
		name       string
		policy     goxsd9.LanguagePolicy
		class      goxsd9.FailureClass
		feature    goxsd9.FeatureID
		wantCode   string
		wantSource goxsd9.SourceID
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility, class: goxsd9.FailureUnsupported, feature: goxsd9.FeatureSchemaSyntax, wantCode: goxsd9.UnsupportedSchemaSyntaxCode, wantSource: "child.xsd"},
		{name: "Strict10", policy: goxsd9.Strict10, class: goxsd9.FailureUnsupported, feature: goxsd9.FeatureSchemaSyntax, wantCode: goxsd9.UnsupportedSchemaSyntaxCode, wantSource: "child.xsd"},
		{name: "Strict11", policy: goxsd9.Strict11, class: goxsd9.FailureUnsupported, feature: goxsd9.FeatureSchemaSyntax, wantCode: goxsd9.UnsupportedSchemaSyntaxCode, wantSource: "child.xsd"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rootReader := newParseTestReader(rootContents)
			root, err := goxsd9.NewResolvedSource(context.Background(), "grammar-root", rootReader)
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			resolver := &policyGraphResolver{contents: map[string]string{"child.xsd": childContents}}

			schema, err := goxsd9.ParseSchemaWithPolicy(root, resolver, test.policy)
			if err == nil {
				t.Fatal("ParseSchemaWithPolicy accepted policy-incompatible grammar")
			}
			if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatal("ParseSchemaWithPolicy returned a partial schema")
			}
			diagnostic := parseTestDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.wantCode {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.wantCode)
			}
			if diagnostic.Feature() != test.feature {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
			}
			if diagnostic.Loc().Source() != test.wantSource {
				t.Fatalf("diagnostic source = %q, want %q", diagnostic.Loc().Source(), test.wantSource)
			}
			if got, want := len(resolver.calls), 1; got != want {
				t.Fatalf("resolver call count = %d, want %d", got, want)
			}
			if got, want := resolver.calls[0], (parseTestCall{location: "child.xsd"}); !reflect.DeepEqual(got, want) {
				t.Fatalf("resolver call = %#v, want %#v", got, want)
			}
			if rootReader.closeCount != 1 || resolver.opened[0].reader.closeCount != 1 {
				t.Fatalf("source close counts = root %d, child %d; want one each", rootReader.closeCount, resolver.opened[0].reader.closeCount)
			}
		})
	}
}

//nolint:gocognit // Keep exact-boundary cases and source-closure checks together.
func TestParseSchemaWithPolicyUsesExactConditionalCapabilityBounds(t *testing.T) {
	tests := []struct {
		name   string
		policy goxsd9.LanguagePolicy
		label  string
		min    string
		max    string
		keep   bool
	}{
		{name: "Compatibility absent bounds", policy: goxsd9.Compatibility, label: "1.0", keep: true},
		{name: "Compatibility minimum inclusive", policy: goxsd9.Compatibility, label: "1.0", min: "1.1", keep: true},
		{name: "Compatibility maximum exclusive", policy: goxsd9.Compatibility, label: "1.0", max: "1.1", keep: false},
		{name: "Strict10 minimum inclusive", policy: goxsd9.Strict10, label: "1.1", min: "1.0", keep: true},
		{name: "Strict10 maximum exclusive", policy: goxsd9.Strict10, label: "1.1", max: "1.0", keep: false},
		{name: "Strict11 minimum inclusive", policy: goxsd9.Strict11, label: "1.0", min: "1.1", keep: true},
		{name: "Strict11 maximum exclusive", policy: goxsd9.Strict11, label: "1.0", max: "1.1", keep: false},
		{name: "Compatibility decimal equality minimum", policy: goxsd9.Compatibility, label: "1.0", min: "1.1000", keep: true},
		{name: "Compatibility decimal equality maximum", policy: goxsd9.Compatibility, label: "1.0", max: "1.1000", keep: false},
		{name: "Strict10 exact greater minimum", policy: goxsd9.Strict10, label: "1.1", min: "1.0000000000000000000000000001", keep: false},
		{name: "Strict11 exact greater maximum", policy: goxsd9.Strict11, label: "1.0", max: "1.1000000000000000000000000001", keep: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attributes := ""
			if test.min != "" {
				attributes += ` vc:minVersion="` + test.min + `"`
			}
			if test.max != "" {
				attributes += ` vc:maxVersion="` + test.max + `"`
			}
			rootContents := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:vc="` + policyTestXSDVersioningNamespace + `" version="` + test.label + `"><xs:element name="item"` + attributes + `/></xs:schema>`
			rootReader := newParseTestReader(rootContents)
			root, err := goxsd9.NewResolvedSource(context.Background(), "conditional-root", rootReader)
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(root, nil, test.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			wantComponents := 0
			if test.keep {
				wantComponents = 1
			}
			if got := len(schema.Components()); got != wantComponents {
				t.Fatalf("component count = %d, want %d", got, wantComponents)
			}
			if got, want := rootReader.closeCount, 1; got != want {
				t.Fatalf("root close count = %d, want %d", got, want)
			}
		})
	}
}

type policyGraphResolver struct {
	contents map[string]string
	calls    []parseTestCall
	opened   []parseTestOpenedSource
}

func (resolver *policyGraphResolver) Resolve(ctx context.Context, namespaceURN, schemaLocation string) (goxsd9.ResolvedSource, error) {
	contextSource := ""
	if ctx != nil {
		if value, ok := ctx.Value(parseTestContextKey{}).(string); ok {
			contextSource = value
		}
	}
	resolver.calls = append(resolver.calls, parseTestCall{
		contextSource: contextSource,
		namespaceURN:  namespaceURN,
		location:      schemaLocation,
	})
	contents, ok := resolver.contents[schemaLocation]
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("no fixture for %q", schemaLocation)
	}
	reader := newParseTestReader(contents)
	id := goxsd9.SourceID(schemaLocation)
	resolver.opened = append(resolver.opened, parseTestOpenedSource{id: id, reader: reader})
	childContext := context.WithValue(ctx, parseTestContextKey{}, string(id))
	return goxsd9.NewResolvedSource(childContext, id, reader)
}

func assertParseTestPolicyFacetVersions(t *testing.T, schema goxsd9.Schema, want goxsd9.XSDVersion) {
	t.Helper()
	wantTypes := []struct {
		namespace string
		local     string
	}{
		{namespace: "urn:root", local: "rootType"},
		{namespace: "urn:b", local: "bType"},
	}
	for _, wantType := range wantTypes {
		name, err := goxsd9.NewQName(wantType.namespace, wantType.local)
		if err != nil {
			t.Fatalf("NewQName: %v", err)
		}
		found := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
		if len(found) != 1 {
			t.Fatalf("simple type %s count = %d, want one", name, len(found))
		}
		definition, ok := found[0].SimpleType()
		if !ok {
			t.Fatalf("simple type %s view is missing", name)
		}
		if got := definition.DigitFacets().Version(); got != want {
			t.Fatalf("simple type %s facet version = %q, want %q", name, got, want)
		}
	}
}

func policyXSDVersion(policy goxsd9.LanguagePolicy) goxsd9.XSDVersion {
	if policy == goxsd9.Strict10 {
		return goxsd9.XSDVersion10
	}
	return goxsd9.XSDVersion11
}

var _ goxsd9.Resolver = (*policyGraphResolver)(nil)

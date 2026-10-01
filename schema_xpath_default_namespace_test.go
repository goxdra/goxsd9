package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type schemaXPathDefaultNamespaceValue struct {
	name    string
	present bool
	value   string
}

func schemaXPathDefaultNamespaceValues() []schemaXPathDefaultNamespaceValue {
	return []schemaXPathDefaultNamespaceValue{
		{name: "omitted"},
		{name: "default namespace", present: true, value: "##defaultNamespace"},
		{name: "target namespace", present: true, value: "##targetNamespace"},
		{name: "local", present: true, value: "##local"},
		{name: "empty URI", present: true},
		{name: "URI", present: true, value: "urn:example:xpath-default"},
	}
}

func schemaXPathDefaultNamespaceRoot(value schemaXPathDefaultNamespaceValue) string {
	attribute := ""
	if value.present {
		attribute = ` xpathDefaultNamespace="` + value.value + `"`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.0"` + attribute + `>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
}

//nolint:gocognit // Keep the policy and accepted-value snapshot matrix together.
func TestSchemaBridgeAcceptsInertRootXPathDefaultNamespaceAsOmission(t *testing.T) {
	queryName := mustTestQName(t, "", "item")
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	} {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			values := schemaXPathDefaultNamespaceValues()
			omitted := values[0]
			omittedSchema, err := discoverTestSchemaWithPolicy(t, schemaXPathDefaultNamespaceRoot(omitted), nil, profile.policy)
			if err != nil {
				t.Fatalf("omitted xpathDefaultNamespace: %v", err)
			}
			omittedSnapshot := snapshotSchemaForComponentKind(t, omittedSchema, queryName, ComponentKindElementDeclaration)
			for _, value := range values {
				value := value
				t.Run(value.name, func(t *testing.T) {
					firstSnapshot := schemaXPathDefaultNamespaceAcceptedSnapshot(t, value, profile.policy, queryName)
					if !reflect.DeepEqual(firstSnapshot, omittedSnapshot) {
						t.Fatalf("public schema snapshot differs from omission: got=%#v want=%#v", firstSnapshot, omittedSnapshot)
					}
					secondSnapshot := schemaXPathDefaultNamespaceAcceptedSnapshot(t, value, profile.policy, queryName)
					if !reflect.DeepEqual(secondSnapshot, firstSnapshot) {
						t.Fatalf("repeated parse changed public schema snapshot: first=%#v second=%#v", firstSnapshot, secondSnapshot)
					}
				})
			}
		})
	}
}

func schemaXPathDefaultNamespaceAcceptedSnapshot(t *testing.T, value schemaXPathDefaultNamespaceValue, policy LanguagePolicy, queryName QName) schemaFinalDefaultSnapshot {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, schemaXPathDefaultNamespaceRoot(value), nil, policy)
	if err != nil {
		t.Fatalf("xpathDefaultNamespace %q: %v", value.value, err)
	}
	return snapshotSchemaForComponentKind(t, schema, queryName, ComponentKindElementDeclaration)
}

//nolint:gocognit // Keep all valid forms and their mismatch provenance together.
func TestStrict10ValidRootXPathDefaultNamespaceRetainsLocatedMismatch(t *testing.T) {
	for _, value := range schemaXPathDefaultNamespaceValues()[1:] {
		value := value
		t.Run(value.name, func(t *testing.T) {
			root := schemaXPathDefaultNamespaceRoot(value)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err == nil {
				t.Fatal("Strict10 accepted a valid xpathDefaultNamespace")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
				t.Fatalf("diagnostic spec ref = %q, want existing XSD 1.1 schema-document anchor", diagnostic.SpecRef())
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "xpathDefaultNamespace") {
				t.Fatalf("diagnostic location = %s, want xpathDefaultNamespace attribute", diagnostic.Loc())
			}
			if diagnostic.Message() != `schema root attribute "xpathDefaultNamespace" is an XSD 1.1-only construct` {
				t.Fatalf("diagnostic message = %q, want located XSD 1.1 mismatch", diagnostic.Message())
			}
			if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep malformed lexical and policy precedence assertions together.
func TestSchemaBridgeMalformedRootXPathDefaultNamespacePrecedesPolicyClassification(t *testing.T) {
	root := schemaXPathDefaultNamespaceRoot(schemaXPathDefaultNamespaceValue{present: true, value: "%ZZ"})
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	} {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("malformed xpathDefaultNamespace was accepted")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Class(), invalidSchemaCompositionCode)
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "xpathDefaultNamespace") {
				t.Fatalf("diagnostic location = %s, want xpathDefaultNamespace attribute", diagnostic.Loc())
			}
			if diagnostic.Message() != `attribute "xpathDefaultNamespace" has an invalid anyURI value` {
				t.Fatalf("diagnostic message = %q, want existing anyURI diagnostic", diagnostic.Message())
			}
			if errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("malformed value retained unsupported or policy-mismatch cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the unsupported XPath boundary and no-schema assertions together.
func TestSchemaBridgeRootXPathDefaultNamespaceDoesNotHideLaterAssertion(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="##local">
  <xs:complexType name="item">
    <xs:assert test="true()"/>
  </xs:complexType>
</xs:schema>`
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	} {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("schema with an assertion was accepted")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
				t.Fatalf("diagnostic spec ref = %q, want existing XSD 1.1 schema-document anchor", diagnostic.SpecRef())
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, "<xs:assert") {
				t.Fatalf("diagnostic location = %s, want assertion location", diagnostic.Loc())
			}
			if !errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("assertion diagnostic provenance changed: %v", err)
			}
		})
	}
}

func TestSchemaBridgeRootXPathDefaultNamespacePreservesIncludeImportChameleonGraph(t *testing.T) {
	rootWithoutMetadata := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.0">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="root" type="xs:integer"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {
			id: "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">
  <xs:element name="chameleon" type="xs:integer"/>
</xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="1.1">
  <xs:element name="other" type="xs:integer"/>
</xs:schema>`,
		},
	}
	queryName := mustTestQName(t, "urn:root", "root")
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict11", policy: Strict11},
	} {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			baseline := schemaXPathDefaultNamespaceGraphSnapshot(t, rootWithoutMetadata, fixtures, profile.policy, queryName)
			for _, value := range schemaXPathDefaultNamespaceValues()[1:] {
				value := value
				t.Run(value.name, func(t *testing.T) {
					root := schemaXPathDefaultNamespaceGraphRoot(rootWithoutMetadata, value)
					got := schemaXPathDefaultNamespaceGraphSnapshot(t, root, fixtures, profile.policy, queryName)
					if !reflect.DeepEqual(got, baseline) {
						t.Fatalf("graph schema snapshot differs from omission: got=%#v want=%#v", got, baseline)
					}
				})
			}
		})
	}
}

func schemaXPathDefaultNamespaceGraphRoot(root string, value schemaXPathDefaultNamespaceValue) string {
	attribute := ` xpathDefaultNamespace="` + value.value + `"`
	return strings.Replace(root, ` version="1.0">`, ` version="1.0"`+attribute+`>`, 1)
}

func schemaXPathDefaultNamespaceGraphSnapshot(t *testing.T, root string, fixtures map[string]discoveryFixture, policy LanguagePolicy, queryName QName) schemaFinalDefaultSnapshot {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
	if err != nil {
		t.Fatalf("graph schema: %v", err)
	}
	documents := schema.Documents()
	wantSources := []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"}
	if len(documents) != len(wantSources) {
		t.Fatalf("graph document count = %d, want %d", len(documents), len(wantSources))
	}
	for index, want := range wantSources {
		if got := documents[index].Source(); got != want {
			t.Fatalf("graph document %d source = %q, want %q", index, got, want)
		}
	}
	if got := len(schema.Components()); got != 3 {
		t.Fatalf("graph component count = %d, want 3", got)
	}
	if got := schema.Find(mustTestQName(t, "urn:root", "chameleon")); len(got) != 1 {
		t.Fatalf("chameleon component lookup count = %d, want 1", len(got))
	}
	if got := schema.Find(mustTestQName(t, "urn:other", "other")); len(got) != 1 {
		t.Fatalf("imported component lookup count = %d, want 1", len(got))
	}
	return snapshotSchemaForComponentKind(t, schema, queryName, ComponentKindElementDeclaration)
}

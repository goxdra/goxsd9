package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the complete immutable boolean component contract together.
func TestSchemaBridgeBuildsBooleanTypesAndPreservesFacts(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(test.version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="b:boolean"/>
  <xs:element name="named" type="r:Derived"/>
  <xs:element name="cross" type="o:Cross"/>
  <xs:simpleType name="Derived"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="b:boolean"/></xs:simpleType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(test.version) + `">
  <xs:simpleType name="Cross"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: other},
			}, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			components := schema.Components()
			wantComponents := []struct {
				kind ComponentKind
				name QName
			}{
				{kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "direct")},
				{kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "named")},
				{kind: ComponentKindElementDeclaration, name: mustTestQName(t, "urn:root", "cross")},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:root", "Derived")},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:root", "Base")},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:other", "Cross")},
			}
			if len(components) != len(wantComponents) {
				t.Fatalf("component count = %d, want %d", len(components), len(wantComponents))
			}
			for index, want := range wantComponents {
				if components[index].Kind() != want.kind || components[index].Name() != want.name {
					t.Fatalf("component %d = %q/%q, want %q/%q", index, components[index].Kind(), components[index].Name(), want.kind, want.name)
				}
				if got, wantOrdinal := components[index].ID().Ordinal(), uint64(index+1); index < 5 && got != wantOrdinal {
					t.Fatalf("component %d ordinal = %d, want %d", index, got, wantOrdinal)
				}
			}
			if got, want := components[5].ID().Ordinal(), uint64(1); got != want {
				t.Fatalf("cross-document component ordinal = %d, want %d", got, want)
			}

			direct, ok := components[0].ElementDeclaration()
			if !ok {
				t.Fatal("direct boolean element view is missing")
			}
			if got, want := direct.DeclaredType(), mustTestQName(t, testXSDNamespace, "boolean"); got != want {
				t.Fatalf("direct declared type = %q, want %q", got, want)
			}
			if typeID, hasTypeID := direct.TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("direct boolean type ID = (%v, %t), want zero,false", typeID, hasTypeID)
			}
			directReference, hasDirectReference := direct.TypeReference()
			if !hasDirectReference || !directReference.IsBuiltin() || directReference.Name() != mustTestQName(t, testXSDNamespace, "boolean") {
				t.Fatalf("direct boolean type reference = %q/%q/%t, want built-in boolean", directReference.Kind(), directReference.Name(), hasDirectReference)
			}
			if directReference.Variety() != SimpleTypeVarietyAtomicRestriction || directReference.VarietyLoc().IsZero() {
				t.Fatalf("direct boolean type reference facts = %q/%s, want atomic with location", directReference.Variety(), directReference.VarietyLoc())
			}
			if referenceID, hasReferenceID := directReference.ComponentID(); hasReferenceID || !referenceID.IsZero() {
				t.Fatalf("direct boolean type reference ID = (%v, %t), want zero,false", referenceID, hasReferenceID)
			}

			named, ok := components[1].ElementDeclaration()
			if !ok {
				t.Fatal("named boolean element view is missing")
			}
			if typeID, hasTypeID := named.TypeID(); !hasTypeID || typeID != components[3].ID() {
				t.Fatalf("named boolean type ID = (%v, %t), want (%v, true)", typeID, hasTypeID, components[3].ID())
			}
			cross, ok := components[2].ElementDeclaration()
			if !ok {
				t.Fatal("cross-document boolean element view is missing")
			}
			if typeID, hasTypeID := cross.TypeID(); !hasTypeID || typeID != components[5].ID() {
				t.Fatalf("cross-document boolean type ID = (%v, %t), want (%v, true)", typeID, hasTypeID, components[5].ID())
			}

			derived, ok := components[3].SimpleTypeDefinition()
			if !ok {
				t.Fatal("derived boolean simple type view is missing")
			}
			if !derived.IsBoolean() {
				t.Fatal("derived boolean type did not retain its boolean kind")
			}
			if derived.IsString() {
				t.Fatal("derived boolean type reported string identity")
			}
			if got, want := derived.Base(), mustTestQName(t, "urn:root", "Base"); got != want {
				t.Fatalf("derived base = %q, want %q", got, want)
			}
			if baseID, hasBaseID := derived.BaseID(); !hasBaseID || baseID != components[4].ID() {
				t.Fatalf("derived base ID = (%v, %t), want (%v, true)", baseID, hasBaseID, components[4].ID())
			}
			if derived.BaseLoc().IsZero() || derived.BaseLoc().Source() != "root.xsd" {
				t.Fatalf("derived base location = %v, want a root.xsd location", derived.BaseLoc())
			}

			base, ok := components[4].SimpleTypeDefinition()
			if !ok {
				t.Fatal("base boolean simple type view is missing")
			}
			if !base.IsBoolean() {
				t.Fatal("direct boolean restriction did not retain its boolean kind")
			}
			if base.IsString() {
				t.Fatal("direct boolean restriction reported string identity")
			}
			if got, want := base.Base(), mustTestQName(t, testXSDNamespace, "boolean"); got != want {
				t.Fatalf("base built-in QName = %q, want %q", got, want)
			}
			if baseID, hasBaseID := base.BaseID(); hasBaseID || !baseID.IsZero() {
				t.Fatalf("base built-in ID = (%v, %t), want zero,false", baseID, hasBaseID)
			}
			if base.BaseLoc().IsZero() || base.BaseLoc().Source() != "root.xsd" {
				t.Fatalf("base built-in location = %v, want a root.xsd location", base.BaseLoc())
			}
			if facets := base.DigitFacets(); facets.Kind() != "" {
				t.Fatalf("boolean digit facets kind = %q, want empty", facets.Kind())
			}
			if _, present := base.IntegerBounds(); present {
				t.Fatal("boolean type unexpectedly exposed integer bounds")
			}
			if _, present := base.DecimalBounds(); present {
				t.Fatal("boolean type unexpectedly exposed decimal bounds")
			}

			crossType, ok := components[5].SimpleTypeDefinition()
			if !ok || !crossType.IsBoolean() {
				t.Fatal("cross-document boolean type view is missing or not boolean")
			}
			if crossType.IsString() {
				t.Fatal("cross-document boolean type reported string identity")
			}
			if baseID, hasBaseID := crossType.BaseID(); hasBaseID || !baseID.IsZero() {
				t.Fatalf("cross-document built-in base ID = (%v, %t), want zero,false", baseID, hasBaseID)
			}
			if got := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "boolean")); len(got) != 0 {
				t.Fatalf("built-in boolean unexpectedly has schema components: %v", got)
			}

			before := schema.Components()
			queried := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "direct"))
			if len(queried) != 1 {
				t.Fatalf("direct element query count = %d, want 1", len(queried))
			}
			queried[0] = Component{}
			documents := schema.Documents()
			if len(documents) != 2 {
				t.Fatalf("document count = %d, want 2", len(documents))
			}
			documents[0] = SchemaDocument{}
			documentComponents := schema.Documents()[0].Components()
			documentComponents[0] = Component{}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating caller-owned schema query results changed the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep all named-base failure classes and diagnostic metadata together.
func TestSchemaBridgeBooleanBaseFailuresRetainDiagnosticContract(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		code    string
		cause   error
		related int
	}{
		{
			name:  "missing",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing"><xs:simpleType name="item"><xs:restriction base="m:missing"/></xs:simpleType></xs:schema>`,
			code:  diagnosticSchemaSimpleTypeUnresolvedCode,
			cause: errSchemaSimpleTypeBaseUnresolved,
		},
		{
			name:    "wrong kind",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:boolean"/><xs:simpleType name="derived"><xs:restriction base="item"/></xs:simpleType></xs:schema>`,
			code:    diagnosticSchemaSimpleTypeWrongKindCode,
			cause:   errSchemaSimpleTypeBaseWrongKind,
			related: 1,
		},
		{
			name:    "ambiguous",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:simpleType name="item"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:simpleType name="derived"><xs:restriction base="item"/></xs:simpleType></xs:schema>`,
			code:    diagnosticSchemaGlobalDuplicateCode,
			cause:   errSchemaGlobalDeclarationDuplicate,
			related: 1,
		},
		{
			name:    "cyclic",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="one"><xs:restriction base="two"/></xs:simpleType><xs:simpleType name="two"><xs:restriction base="one"/></xs:simpleType></xs:schema>`,
			code:    diagnosticSchemaSimpleTypeCycleCode,
			cause:   errSchemaSimpleTypeBaseCycle,
			related: 1,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.value)
				if err == nil {
					t.Fatal("discoverSchema accepted an invalid boolean base graph")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				wantSpecRef := schemaSimpleTypeSpecRef(policy.version)
				if test.code == diagnosticSchemaGlobalDuplicateCode {
					wantSpecRef = schemaGlobalDuplicateSpecRef(policy.version)
				}
				if diagnostic.SpecRef() != wantSpecRef {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpecRef)
				}
				if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
					t.Fatalf("diagnostic location = %v, want a root.xsd location", diagnostic.Loc())
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
				}
				if len(diagnostic.Related()) < test.related {
					t.Fatalf("related locations = %v, want at least %d", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Keep versioned boolean facet classification and metadata together.
func TestSchemaBridgeRejectsBooleanFacetsWithLocatedClassification(t *testing.T) {
	facets := []struct {
		name string
		body string
	}{
		{name: "pattern", body: `<xs:pattern value="true"/>`},
		{name: "enumeration", body: `<xs:enumeration value="1"/>`},
		{name: "whiteSpace", body: `<xs:whiteSpace value="collapse"/>`},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, facet := range facets {
			t.Run(policy.name+"/"+facet.name, func(t *testing.T) {
				root := booleanFacetSchema(policy.version, facet.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err == nil {
					t.Fatal("discoverSchema accepted an unsupported boolean facet")
				}
				if schema.storage != nil {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode {
					t.Fatalf("diagnostic = %s, want unsupported/%s", diagnostic, UnsupportedDatatypeFacetCode)
				}
				if diagnostic.Feature() != FeatureDatatypeFacets {
					t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), FeatureDatatypeFacets)
				}
				if diagnostic.SpecRef() != schemaBooleanDatatypeSpecRef(policy.version) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaBooleanDatatypeSpecRef(policy.version))
				}
				if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
					t.Fatalf("diagnostic location = %v, want a root.xsd location", diagnostic.Loc())
				}
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep malformed boolean facet classification and zero-schema checks together.
func TestSchemaBridgeBooleanFacetMalformedValuesRemainInvalid(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		code     string
		specFunc func(XSDVersion) string
	}{
		{
			name:     "enumeration lexical value",
			body:     `<xs:enumeration value="maybe"/>`,
			code:     InvalidBooleanLexicalCode,
			specFunc: strictBooleanSpecRef,
		},
		{name: "whiteSpace preserve", body: `<xs:whiteSpace value="preserve"/>`, code: invalidSchemaCompositionCode},
		{name: "whiteSpace replace", body: `<xs:whiteSpace value="replace"/>`, code: invalidSchemaCompositionCode},
		{name: "whiteSpace malformed", body: `<xs:whiteSpace value="preserve-ish"/>`, code: InvalidStringWhiteSpaceCode, specFunc: stringWhiteSpaceSpecRef},
		{
			name: "pattern missing value",
			body: `<xs:pattern/>`,
			code: invalidSchemaCompositionCode,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, booleanFacetSchema(policy.version, test.body), nil, policy.value)
				if err == nil {
					t.Fatal("discoverSchema accepted a malformed boolean facet")
				}
				if schema.storage != nil {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				if test.specFunc != nil && diagnostic.SpecRef() != test.specFunc(policy.version) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specFunc(policy.version))
				}
				if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
					t.Fatalf("diagnostic location = %v, want a root.xsd location", diagnostic.Loc())
				}
			})
		}
	}
}

func TestSchemaBridgeRejectsBooleanAssertionWithPinnedDiagnostic(t *testing.T) {
	root := booleanFacetSchema(XSDVersion11, `<xs:assertion test="true()"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted an unsupported boolean assertion")
	}
	if schema.storage != nil {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want unsupported/%s", diagnostic, UnsupportedDatatypeFacetCode)
	}
	if diagnostic.Feature() != FeatureID("xsd.assertion") || diagnostic.SpecRef() != "xsd11-structures#cAssertions" {
		t.Fatalf("assertion diagnostic metadata = %q/%q, want xsd.assertion/xsd11-structures#cAssertions", diagnostic.Feature(), diagnostic.SpecRef())
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %v, want a root.xsd location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
}

func TestSchemaBridgeBuildsBasicBooleanLocalParticles(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, test := range []struct {
			name string
			body string
		}{
			{name: "direct sequence", body: `<xs:complexType name="Record"><xs:sequence><xs:element name="value" type="xs:boolean"/></xs:sequence></xs:complexType>`},
			{name: "named choice", body: `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:complexType name="Record"><xs:choice><xs:element name="value" type="r:Flag"/></xs:choice></xs:complexType>`},
		} {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(policy.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				if schema.storage == nil {
					t.Fatal("discoverSchema returned no schema")
				}
			})
		}
	}
}

//nolint:gocognit // Keep direct and named local choice unsupported diagnostics symmetric.
func TestValidateInstanceLeavesLocalBooleanChoicesUnsupported(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		typeDef string
		input   string
	}{
		{
			name:  "direct built-in",
			body:  `<xs:element name="value" type="xs:boolean"/>`,
			input: `<choice xmlns="urn:root"><value xmlns="">true</value></choice>`,
		},
		{
			name:    "named restriction",
			body:    `<xs:element name="value" type="r:Flag"/>`,
			typeDef: `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`,
			input:   `<choice xmlns="urn:root"><value xmlns="">true</value></choice>`,
		},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">` +
					`<xs:element name="choice" type="r:Choice"/><xs:complexType name="Choice"><xs:choice>` +
					test.body + `</xs:choice></xs:complexType>` + test.typeDef + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
				if err == nil {
					t.Fatal("ValidateInstance accepted a local boolean choice alternative")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Feature() != FeatureInstanceValidation {
					t.Fatalf("diagnostic = %s/%q/%q, want unsupported/%s/%q", diagnostic.Class(), diagnostic.Code(), diagnostic.Feature(), UnsupportedInstanceValidationCode, FeatureInstanceValidation)
				}
				if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
					t.Fatalf("diagnostic location = %v, want root.xsd location", diagnostic.Loc())
				}
				wantSpec := "xsd11-structures#cvc-elt"
				if policy == Strict10 {
					wantSpec = "xsd10-structures#cvc-elt"
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep the policy, particle-fact, and consumer diagnostic assertions together.
func TestValidateInstanceRejectsNillableLocalChoiceWithoutNilledBehavior(t *testing.T) {
	profiles := []struct {
		name     string
		policy   LanguagePolicy
		version  string
		wantSpec string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.1", wantSpec: "xsd11-structures#cvc-elt"},
		{name: "Strict10", policy: Strict10, version: "1.0", wantSpec: "xsd10-structures#cvc-elt"},
		{name: "Strict11", policy: Strict11, version: "1.1", wantSpec: "xsd11-structures#cvc-elt"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + profile.version + `">` +
				`<xs:element name="choiceRoot" type="r:Choice"/><xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer" nillable="true"/></xs:choice></xs:complexType>` +
				`</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			definition := booleanParticleComplexType(t, schema, "Choice")
			choice, ok := definition.Particle().(ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 1 {
				t.Fatalf("choice particle = %#v, want one alternative", definition.Particle())
			}
			local, ok := choice.Alternatives()[0].(ElementParticle)
			if !ok || !local.IsNillable() {
				t.Fatalf("local particle = %#v, want nillable=true", choice.Alternatives()[0])
			}

			input := `<choiceRoot xmlns="urn:root"><value xmlns="">1</value></choiceRoot>`
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			if err == nil {
				t.Fatal("ValidateInstance accepted a nillable local choice")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Feature() != FeatureInstanceValidation {
				t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance-validation diagnostic", diagnostic, diagnostic.Code(), diagnostic.Feature())
			}
			if diagnostic.Loc() != local.Loc() {
				t.Fatalf("diagnostic location = %s, want local declaration %s", diagnostic.Loc(), local.Loc())
			}
			if diagnostic.SpecRef() != profile.wantSpec {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), profile.wantSpec)
			}
			if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errInstanceLocalElementFacts) {
				t.Fatalf("diagnostic lost local nillable cause: %v", err)
			}
		})
	}
}

func booleanFacetSchema(version XSDVersion, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(version) + `"><xs:simpleType name="Flag"><xs:restriction base="xs:boolean">` + body + `</xs:restriction></xs:simpleType></xs:schema>`
}

package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the ordered public attribute contract together.
func TestSchemaBridgeExposesGlobalAttributeScalarFacts(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(policy.version) + `">
  <xs:attribute name="directInteger" type="xs:integer"/>
  <xs:attribute name="directDecimal" type="xs:decimal"/>
  <xs:attribute name="namedInteger" type="r:Integer"/>
  <xs:attribute name="inheritedDecimal" type="r:Decimal"/>
  <xs:attribute name="generic"/>
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="DecimalBase"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:simpleType name="Decimal"><xs:restriction base="r:DecimalBase"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if got, want := len(components), 8; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			wantNames := []QName{
				mustTestQName(t, "urn:root", "directInteger"),
				mustTestQName(t, "urn:root", "directDecimal"),
				mustTestQName(t, "urn:root", "namedInteger"),
				mustTestQName(t, "urn:root", "inheritedDecimal"),
				mustTestQName(t, "urn:root", "generic"),
				mustTestQName(t, "urn:root", "Integer"),
				mustTestQName(t, "urn:root", "DecimalBase"),
				mustTestQName(t, "urn:root", "Decimal"),
			}
			for index, component := range components {
				if component.Name() != wantNames[index] {
					t.Fatalf("component %d name = %q, want %q", index, component.Name(), wantNames[index])
				}
				if component.ID().Ordinal() != uint64(index+1) {
					t.Fatalf("component %d ordinal = %d, want %d", index, component.ID().Ordinal(), index+1)
				}
			}

			wantTypes := []QName{
				mustTestQName(t, testXSDNamespace, "integer"),
				mustTestQName(t, testXSDNamespace, "decimal"),
				mustTestQName(t, "urn:root", "Integer"),
				mustTestQName(t, "urn:root", "Decimal"),
			}
			wantTypeComponents := []int{-1, -1, 5, 7}
			for index, wantType := range wantTypes {
				declaration, ok := components[index].Attribute()
				if !ok {
					t.Fatalf("component %d has no attribute view", index)
				}
				alias, aliasOK := components[index].AttributeDeclaration()
				if !aliasOK || alias.DeclaredType() != declaration.DeclaredType() {
					t.Fatalf("component %d AttributeDeclaration alias is incomplete", index)
				}
				if declaration.Component().ID() != components[index].ID() || declaration.ID() != components[index].ID() || declaration.Name() != components[index].Name() || declaration.Loc() != components[index].Loc() {
					t.Fatalf("component %d attribute view does not preserve generic facts", index)
				}
				if declaration.DeclaredType() != wantType {
					t.Fatalf("component %d declared type = %q, want %q", index, declaration.DeclaredType(), wantType)
				}
				typeID, hasTypeID := declaration.TypeID()
				if index < 2 {
					if hasTypeID || !typeID.IsZero() {
						t.Fatalf("component %d built-in type ID = %v/%t, want zero/false", index, typeID, hasTypeID)
					}
					continue
				}
				wantID := components[wantTypeComponents[index]].ID()
				if !hasTypeID || typeID != wantID {
					t.Fatalf("component %d named type ID = %v/%t, want %v/true", index, typeID, hasTypeID, wantID)
				}
			}
			if _, ok := components[4].Attribute(); ok {
				t.Fatal("generic global attribute unexpectedly has a typed view")
			}
			if _, ok := components[4].AttributeDeclaration(); ok {
				t.Fatal("generic global attribute unexpectedly has an AttributeDeclaration view")
			}

			before := schema.Components()
			found := schema.FindKind(ComponentKindAttributeDeclaration, wantNames[0])
			if len(found) != 1 {
				t.Fatalf("FindKind count = %d, want 1", len(found))
			}
			found[0] = Component{}
			returned := schema.Components()
			returned[0] = Component{}
			documentComponents := schema.Documents()[0].Components()
			documentComponents[0] = Component{}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating returned component slices changed Schema")
			}

			walked := make([]ComponentID, 0, len(components))
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			wantWalk := make([]ComponentID, 0, len(components))
			for _, component := range components {
				wantWalk = append(wantWalk, component.ID())
			}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("Walk IDs = %#v, want %#v", walked, wantWalk)
			}
		})
	}
}

func TestSchemaBridgeExpandsGlobalAttributeTypeQNameInLocalScope(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns="urn:root" xmlns:r="urn:wrong" targetNamespace="urn:root">
  <xs:attribute name="defaultNamespace" type=" Integer "/>
  <xs:attribute xmlns:r="urn:root" name="localPrefix" type="r:Integer"/>
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if len(components) != 3 {
		t.Fatalf("component count = %d, want 3", len(components))
	}
	wantType := mustTestQName(t, "urn:root", "Integer")
	for index := 0; index < 2; index++ {
		declaration, ok := components[index].Attribute()
		if !ok || declaration.DeclaredType() != wantType {
			t.Fatalf("attribute %d type = %q/%t, want %q/true", index, declaration.DeclaredType(), ok, wantType)
		}
		if typeID, hasTypeID := declaration.TypeID(); !hasTypeID || typeID != components[2].ID() {
			t.Fatalf("attribute %d type ID = %v/%t, want %v/true", index, typeID, hasTypeID, components[2].ID())
		}
	}
}

func TestSchemaBridgeExposesImportedGlobalAttributeScalarType(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="crossDocument" type="o:CrossDecimal"/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="CrossDecimal"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	declaration, ok := components[0].Attribute()
	if !ok {
		t.Fatal("cross-document attribute view is missing")
	}
	if declaration.DeclaredType() != mustTestQName(t, "urn:other", "CrossDecimal") {
		t.Fatalf("declared type = %q, want {urn:other}CrossDecimal", declaration.DeclaredType())
	}
	if typeID, hasTypeID := declaration.TypeID(); !hasTypeID || typeID != components[1].ID() {
		t.Fatalf("cross-document type ID = %v/%t, want %v/true", typeID, hasTypeID, components[1].ID())
	}
	if components[1].ID().Source() != "other.xsd" || components[1].ID().Ordinal() != 1 {
		t.Fatalf("cross-document type identity = %v, want other.xsd ordinal 1", components[1].ID())
	}
}

//nolint:gocognit,funlen // Keep target failures and diagnostic evidence together.
func TestSchemaBridgeGlobalAttributeTypeDiagnostics(t *testing.T) {
	tests := []struct {
		name         string
		root         string
		code         string
		class        FailureClass
		feature      FeatureID
		specRef      string
		cause        error
		primary      string
		related      string
		relatedCount int
	}{
		{
			name:    "unresolved",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing"><xs:attribute name="value" type="m:Missing"/></xs:schema>`,
			code:    diagnosticSchemaAttributeTypeUnresolvedCode,
			class:   FailureInvalid,
			specRef: schemaAttributeTypeXSD11SpecRef,
			cause:   errSchemaAttributeTypeUnresolved,
			primary: "type=",
		},
		{
			name:    "wrong kind",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="target"/><xs:attribute name="value" type="target"/></xs:schema>`,
			code:    diagnosticSchemaAttributeTypeWrongKindCode,
			class:   FailureInvalid,
			specRef: schemaAttributeTypeXSD11SpecRef,
			cause:   errSchemaAttributeTypeWrongKind,
			primary: "type=",
			related: "<xs:element name=",
		},
		{
			name: "cyclic target",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="value" type="r:One"/>
  <xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType>
  <xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>
</xs:schema>`,
			code:         diagnosticSchemaAttributeTypeCycleCode,
			class:        FailureInvalid,
			specRef:      schemaAttributeTypeXSD11SpecRef,
			cause:        errSchemaSimpleTypeBaseCycle,
			primary:      "type=",
			related:      "<xs:attribute name=",
			relatedCount: 2,
		},
		{
			name:    "malformed QName",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="bad:q:name"/></xs:schema>`,
			code:    invalidSchemaConditionalCode,
			class:   FailureInvalid,
			primary: "type=",
		},
		{
			name:    "unbound QName",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="bad:Missing"/></xs:schema>`,
			code:    invalidSchemaConditionalCode,
			class:   FailureInvalid,
			primary: "type=",
		},
		{
			name: "duplicate target declaration",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="value" type="r:Amount"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`,
			code:         diagnosticSchemaGlobalDuplicateCode,
			class:        FailureInvalid,
			specRef:      schemaGlobalDuplicateXSD11SpecRef,
			cause:        errSchemaGlobalDeclarationDuplicate,
			primary:      "<xs:simpleType name=\"Amount\"><xs:restriction base=\"xs:decimal\"",
			related:      "<xs:simpleType name=\"Amount\"><xs:restriction base=\"xs:integer\"",
			relatedCount: 1,
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
					t.Fatal("discoverSchema accepted an invalid attribute type")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
				}
				if test.feature != "" && diagnostic.Feature() != test.feature {
					t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.feature)
				}
				wantSpec := test.specRef
				if test.name == "duplicate target declaration" {
					wantSpec = schemaGlobalDuplicateSpecRef(policy.version)
				}
				if test.name == "unresolved" || test.name == "wrong kind" || test.name == "cyclic target" {
					wantSpec = schemaAttributeTypeSpecRef(policy.version)
				}
				if wantSpec != "" && diagnostic.SpecRef() != wantSpec {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
				if test.primary != "" {
					wantLoc := elementReferenceTestAttributeLoc(t, test.root, test.primary)
					if diagnostic.Loc() != wantLoc {
						t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
					}
				}
				if test.related != "" {
					wantRelated := elementReferenceTestAttributeLoc(t, test.root, test.related)
					found := false
					for _, related := range diagnostic.Related() {
						if related == wantRelated {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("diagnostic related locations = %v, want %s", diagnostic.Related(), wantRelated)
					}
				}
				if len(diagnostic.Related()) < test.relatedCount {
					t.Fatalf("diagnostic related locations = %v, want at least %d", diagnostic.Related(), test.relatedCount)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
				}
			})
		}
	}
}

func TestSchemaBridgeGlobalAttributeUnsupportedTypesAndExcludedShapes(t *testing.T) {
	testSchemaBridgeGlobalAttributeUnsupportedTypes(t)
	testSchemaBridgeGlobalAttributeExcludedShapes(t)
}

func TestSchemaBridgeGlobalAttributePrecisionDecimalStrict10Policy(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:precisionDecimal"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("Strict10 accepted precisionDecimal or returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.SpecRef() != "xsd11-datatypes#dt-primitive" {
		t.Fatalf("Strict10 diagnostic = %s/%q/%q/%q, want precisionDecimal policy mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef())
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "type=") {
		t.Fatalf("Strict10 diagnostic location = %s, want type attribute location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("Strict10 diagnostic lost precisionDecimal policy causes: %v", err)
	}
}

func testSchemaBridgeGlobalAttributeUnsupportedTypes(t *testing.T) {
	unsupportedTypes := []string{"string", "boolean", "precisionDecimal"}
	for _, local := range unsupportedTypes {
		t.Run("unsupported "+local, func(t *testing.T) {
			testSchemaBridgeGlobalAttributeUnsupportedType(t, local)
		})
	}
}

type schemaBridgeGlobalAttributeExcludedShapeCase struct {
	name        string
	policy      LanguagePolicy
	root        string
	class       FailureClass
	code        string
	primary     string
	wantCause   error
	wantFeature FeatureID
}

func testSchemaBridgeGlobalAttributeUnsupportedType(t *testing.T, local string) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:attribute name="value" type="xs:` + local + `"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil || schema.storage != nil {
		t.Fatal("discoverSchema accepted an unsupported attribute scalar or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if diagnostic.SpecRef() != schemaAttributeTypeXSD11SpecRef || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "type=") {
		t.Fatalf("diagnostic metadata = %s/%q, want attribute type location/reference", diagnostic.Loc(), diagnostic.SpecRef())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeTypeUnsupported) {
		t.Fatalf("unsupported diagnostic lost its cause: %v", err)
	}
}

func testSchemaBridgeGlobalAttributeExcludedShapes(t *testing.T) {
	tests := []schemaBridgeGlobalAttributeExcludedShapeCase{
		{
			name:        "default value",
			policy:      Strict11,
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" default="1"/></xs:schema>`,
			class:       FailureUnsupported,
			code:        UnsupportedSchemaSyntaxCode,
			primary:     "default=",
			wantFeature: FeatureSchemaSyntax,
		},
		{
			name:        "fixed value",
			policy:      Strict11,
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" fixed="1"/></xs:schema>`,
			class:       FailureUnsupported,
			code:        UnsupportedSchemaSyntaxCode,
			primary:     "fixed=",
			wantFeature: FeatureSchemaSyntax,
		},
		{
			name:        "inline type",
			policy:      Strict11,
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`,
			class:       FailureUnsupported,
			code:        UnsupportedSchemaSyntaxCode,
			primary:     "<xs:simpleType>",
			wantFeature: FeatureSchemaSyntax,
		},
		{
			name:    "type and inline type",
			policy:  Strict11,
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:integer"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaCompositionCode,
			primary: "<xs:simpleType>",
		},
		{
			name:    "global ref remains invalid",
			policy:  Strict11,
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute ref="value"/></xs:schema>`,
			class:   FailureInvalid,
			code:    invalidSchemaCompositionCode,
			primary: "ref=",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testSchemaBridgeGlobalAttributeExcludedShape(t, test)
		})
	}
}

func testSchemaBridgeGlobalAttributeExcludedShape(t *testing.T, test schemaBridgeGlobalAttributeExcludedShapeCase) {
	schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
	if err == nil || schema.storage != nil {
		t.Fatal("discoverSchema accepted an excluded attribute shape or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
	}
	if test.wantFeature != "" && diagnostic.Feature() != test.wantFeature {
		t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), test.wantFeature)
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.primary) {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), elementReferenceTestAttributeLoc(t, test.root, test.primary))
	}
	if test.wantCause != nil && !errors.Is(err, test.wantCause) {
		t.Fatalf("diagnostic does not preserve cause %v: %v", test.wantCause, err)
	}
}

func TestSchemaBridgeGlobalAttributeDiagnosticsRepeatDeterministically(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing"><xs:attribute name="value" type="m:Missing"/></xs:schema>`
	firstSchema, firstErr := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	secondSchema, secondErr := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if firstErr == nil || secondErr == nil || firstSchema.storage != nil || secondSchema.storage != nil {
		t.Fatal("repeated invalid attribute parses did not return only diagnostics")
	}
	first := requireDiagnostic(t, firstErr)
	second := requireDiagnostic(t, secondErr)
	if first.Class() != second.Class() || first.Code() != second.Code() || first.Loc() != second.Loc() || first.Message() != second.Message() || first.SpecRef() != second.SpecRef() || !reflect.DeepEqual(first.Related(), second.Related()) {
		t.Fatalf("repeated diagnostics differ: first=%s related=%v second=%s related=%v", first, first.Related(), second, second.Related())
	}
}

func TestSchemaBridgeGlobalAttributeCycleDiagnosticMatchesResolverCycle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="first" type="r:FirstOne"/>
  <xs:attribute name="second" type="r:SecondOne"/>
  <xs:simpleType name="SecondOne"><xs:restriction base="r:SecondTwo"/></xs:simpleType>
  <xs:simpleType name="SecondTwo"><xs:restriction base="r:SecondOne"/></xs:simpleType>
  <xs:simpleType name="FirstOne"><xs:restriction base="r:FirstTwo"/></xs:simpleType>
  <xs:simpleType name="FirstTwo"><xs:restriction base="r:FirstOne"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil || schema.storage != nil {
		t.Fatal("discoverSchema accepted independent cyclic attribute types or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaAttributeTypeCycleCode || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
		t.Fatalf("diagnostic = %s, want attribute cycle with shared cycle cause", diagnostic)
	}
	if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:SecondOne"`) {
		t.Fatalf("diagnostic location = %s, want second attribute type", diagnostic.Loc())
	}
	if diagnostic.Message() != `attribute type "{urn:root}SecondOne" resolves through cyclic simple type definitions` {
		t.Fatalf("diagnostic message = %q, want SecondOne cycle", diagnostic.Message())
	}
	for _, unrelated := range []string{
		`<xs:attribute name="first"`,
		`<xs:simpleType name="FirstOne"`,
		`<xs:simpleType name="FirstTwo"`,
	} {
		unrelatedLoc := elementReferenceTestAttributeLoc(t, root, unrelated)
		if schemaLocationListContains(diagnostic.Related(), unrelatedLoc) {
			t.Fatalf("diagnostic related locations = %v, unexpectedly contain unrelated cycle location %s", diagnostic.Related(), unrelatedLoc)
		}
	}
}

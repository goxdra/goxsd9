package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type schemaSimpleTypeFinalEnforcementProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func schemaSimpleTypeFinalEnforcementProfiles() []schemaSimpleTypeFinalEnforcementProfile {
	return []schemaSimpleTypeFinalEnforcementProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
}

type schemaSimpleTypeFinalEnforcementOperation struct {
	name           string
	final          string
	useMarker      string
	useDescription string
}

func schemaSimpleTypeFinalEnforcementOperations() []schemaSimpleTypeFinalEnforcementOperation {
	return []schemaSimpleTypeFinalEnforcementOperation{
		{
			name:           "restriction",
			final:          "restriction",
			useMarker:      `base="t:Base"`,
			useDescription: "restriction base",
		},
		{
			name:           "list",
			final:          "list",
			useMarker:      `itemType="t:Base"`,
			useDescription: "list item type",
		},
		{
			name:           "union",
			final:          "union",
			useMarker:      `memberTypes="t:Base"`,
			useDescription: "union member type",
		},
	}
}

func TestSchemaSimpleTypeFinalControlsRejectNamedDirectDerivations(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, operation := range schemaSimpleTypeFinalEnforcementOperations() {
			t.Run(profile.name+"/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, operation.final)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalFailure(t, schema, err, root, "root.xsd", 3, operation.useMarker, root, "root.xsd", 2, `final="`+operation.final+`"`, profile, operation, operation.final, mustTestQName(t, "urn:test", "Base"))
			})
			t.Run(profile.name+"/all/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, "#all")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalFailure(t, schema, err, root, "root.xsd", 3, operation.useMarker, root, "root.xsd", 2, `final="#all"`, profile, operation, "#all", mustTestQName(t, "urn:test", "Base"))
			})
			t.Run(profile.name+"/nonmatching/"+operation.name, func(t *testing.T) {
				final := schemaSimpleTypeFinalEnforcementNonmatchingFinal(operation.name)
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, final)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalAllowed(t, schema, err, "Base", []string{final})
			})
			t.Run(profile.name+"/explicit-empty/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalAllowed(t, schema, err, "Base", nil)
			})
		}
	}
}

func TestSchemaSimpleTypeFinalExtensionDoesNotInventSimpleExtensionDerivation(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		if profile.version != XSDVersion11 {
			continue
		}
		for _, operation := range schemaSimpleTypeFinalEnforcementOperations() {
			t.Run(profile.name+"/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, "extension")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalAllowed(t, schema, err, "Base", []string{"extension"})
			})
		}
	}
}

func TestSchemaSimpleTypeFinalControlsApplyToNamedNonAtomicDirectReferences(t *testing.T) {
	tests := []struct {
		name           string
		definition     string
		operation      string
		use            string
		useMarker      string
		useDescription string
	}{
		{
			name:           "list item union",
			definition:     `<xs:union memberTypes="xs:string"/>`,
			operation:      "list",
			use:            `<xs:list itemType="t:Base"/>`,
			useMarker:      `itemType="t:Base"`,
			useDescription: "list item type",
		},
		{
			name:           "union member list",
			definition:     `<xs:list itemType="xs:integer"/>`,
			operation:      "union",
			use:            `<xs:union memberTypes="t:Base"/>`,
			useMarker:      `memberTypes="t:Base"`,
			useDescription: "union member type",
		},
	}
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:simpleType name="Base" final="` + test.operation + `">` + test.definition + `</xs:simpleType>
  <xs:simpleType name="Derived">` + test.use + `</xs:simpleType>
</xs:schema>`
				operation := schemaSimpleTypeFinalEnforcementOperation{
					name:           test.operation,
					useMarker:      test.useMarker,
					useDescription: test.useDescription,
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSchemaSimpleTypeFinalFailure(t, schema, err, root, "root.xsd", 3, test.useMarker, root, "root.xsd", 2, `final="`+test.operation+`"`, profile, operation, test.operation, mustTestQName(t, "urn:test", "Base"))
			})
		}
	}
}

func schemaSimpleTypeFinalEnforcementNonmatchingFinal(operation string) string {
	switch operation {
	case "restriction":
		return "list"
	case "list":
		return "restriction"
	case "union":
		return "restriction"
	default:
		panic("unknown simple-type final enforcement operation")
	}
}

func schemaSimpleTypeFinalEnforcementRoot(version XSDVersion, operation, final string) string {
	finalAttribute := ` final="` + final + `"`
	var use string
	switch operation {
	case "restriction":
		use = `<xs:restriction base="t:Base"/>`
	case "list":
		use = `<xs:list itemType="t:Base"/>`
	case "union":
		use = `<xs:union memberTypes="t:Base"/>`
	default:
		panic("unknown simple-type final enforcement operation")
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="Base"` + finalAttribute + `><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Derived">` + use + `</xs:simpleType>
</xs:schema>`
}

func assertSchemaSimpleTypeFinalAllowed(t *testing.T, schema Schema, err error, local string, wantFinal []string) {
	t.Helper()
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", local))
	if len(components) != 1 {
		t.Fatalf("simple type %s matches = %d, want one", local, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("simple type %s view is missing", local)
	}
	if got := definition.Final(); !reflect.DeepEqual(got, wantFinal) {
		t.Fatalf("simple type %s final = %#v, want %#v", local, got, wantFinal)
	}
}

func assertSchemaSimpleTypeFinalFailure(
	t *testing.T,
	schema Schema,
	err error,
	useContents string,
	useSource SourceID,
	useLine int,
	useMarker string,
	finalContents string,
	finalSource SourceID,
	finalLine int,
	finalMarker string,
	profile schemaSimpleTypeFinalEnforcementProfile,
	operation schemaSimpleTypeFinalEnforcementOperation,
	final string,
	reference QName,
) {
	t.Helper()
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSimpleTypeBaseCode)
	}
	if diagnostic.Feature() != "" || errors.Is(err, ErrUnsupported) {
		t.Fatalf("final-control diagnostic was classified as unsupported: %s", diagnostic)
	}
	if diagnostic.SpecRef() != schemaSimpleTypeRestrictionSpecRef(profile.version) {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaSimpleTypeRestrictionSpecRef(profile.version))
	}
	wantUse := mustSchemaTokenLoc(t, useSource, useContents, useLine, useMarker)
	if diagnostic.Loc() != wantUse {
		t.Fatalf("diagnostic location = %s, want derivation use %s", diagnostic.Loc(), wantUse)
	}
	wantFinal := mustSchemaTokenLoc(t, finalSource, finalContents, finalLine, finalMarker)
	if got := diagnostic.Related(); !reflect.DeepEqual(got, []Loc{wantFinal}) {
		t.Fatalf("diagnostic related = %v, want [%s]", got, wantFinal)
	}
	if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("diagnostic lost final-control cause: %v", err)
	}
	wantMessage := fmt.Sprintf(
		"simple type %s %q prohibits %s derivation",
		operation.useDescription,
		reference,
		operation.name,
	)
	if diagnostic.Message() != wantMessage {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), wantMessage)
	}
	if final == "" {
		t.Fatal("final-control failure assertion received an empty final value")
	}
}

func TestSchemaSimpleTypeFinalControlsFollowForwardIncludeImportAndChameleonReferences(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, graph := range []string{"forward", "include-chameleon", "import"} {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				fixture := schemaSimpleTypeFinalEnforcementGraph(t, graph, profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
				assertSchemaSimpleTypeFinalFailure(
					t,
					schema,
					err,
					fixture.useContents,
					fixture.useSource,
					fixture.useLine,
					fixture.useMarker,
					fixture.finalContents,
					fixture.finalSource,
					fixture.finalLine,
					fixture.finalMarker,
					profile,
					fixture.operation,
					fixture.final,
					fixture.reference,
				)
			})
		}
	}
}

type schemaSimpleTypeFinalEnforcementGraphCase struct {
	root          string
	fixtures      map[string]discoveryFixture
	useContents   string
	useSource     SourceID
	useLine       int
	useMarker     string
	finalContents string
	finalSource   SourceID
	finalLine     int
	finalMarker   string
	operation     schemaSimpleTypeFinalEnforcementOperation
	final         string
	reference     QName
}

func schemaSimpleTypeFinalEnforcementGraph(t *testing.T, name string, version XSDVersion) schemaSimpleTypeFinalEnforcementGraphCase {
	t.Helper()
	versionValue := string(version)
	switch name {
	case "forward":
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + versionValue + `">
  <xs:simpleType name="Derived"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="Base" final="restriction"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
		return schemaSimpleTypeFinalEnforcementGraphCase{
			root:          root,
			useContents:   root,
			useSource:     "root.xsd",
			useLine:       2,
			useMarker:     `base="t:Base"`,
			finalContents: root,
			finalSource:   "root.xsd",
			finalLine:     3,
			finalMarker:   `final="restriction"`,
			operation: schemaSimpleTypeFinalEnforcementOperation{
				name:           "restriction",
				useMarker:      `base="t:Base"`,
				useDescription: "restriction base",
			},
			final:     "restriction",
			reference: mustTestQName(t, "urn:test", "Base"),
		}
	case "include-chameleon":
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + versionValue + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:simpleType name="Derived"><xs:list itemType="r:Base"/></xs:simpleType>
</xs:schema>`
		chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" version="` + versionValue + `">
  <xs:simpleType name="Base" final="list"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
		return schemaSimpleTypeFinalEnforcementGraphCase{
			root:          root,
			fixtures:      map[string]discoveryFixture{"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}},
			useContents:   root,
			useSource:     "root.xsd",
			useLine:       3,
			useMarker:     `itemType="r:Base"`,
			finalContents: chameleon,
			finalSource:   "chameleon.xsd",
			finalLine:     2,
			finalMarker:   `final="list"`,
			operation: schemaSimpleTypeFinalEnforcementOperation{
				name:           "list",
				useMarker:      `itemType="r:Base"`,
				useDescription: "list item type",
			},
			final:     "list",
			reference: mustTestQName(t, "urn:root", "Base"),
		}
	case "import":
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root" version="` + versionValue + `">
  <xs:import namespace="urn:base" schemaLocation="base.xsd"/>
  <xs:simpleType name="Derived"><xs:union memberTypes="b:Base"/></xs:simpleType>
</xs:schema>`
		base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="` + versionValue + `">
  <xs:simpleType name="Base" final="union"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
		return schemaSimpleTypeFinalEnforcementGraphCase{
			root:          root,
			fixtures:      map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: base}},
			useContents:   root,
			useSource:     "root.xsd",
			useLine:       3,
			useMarker:     `memberTypes="b:Base"`,
			finalContents: base,
			finalSource:   "base.xsd",
			finalLine:     2,
			finalMarker:   `final="union"`,
			operation: schemaSimpleTypeFinalEnforcementOperation{
				name:           "union",
				useMarker:      `memberTypes="b:Base"`,
				useDescription: "union member type",
			},
			final:     "union",
			reference: mustTestQName(t, "urn:base", "Base"),
		}
	default:
		panic("unknown simple-type final enforcement graph")
	}
}

func TestSchemaSimpleTypeFinalControlsAreNotInheritedThroughNamedChains(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:simpleType name="Base" final="list"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Middle"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="Outer"><xs:list itemType="t:Middle"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertSchemaSimpleTypeFinalAllowed(t, schema, nil, "Base", []string{"list"})
			assertSchemaSimpleTypeFinalAllowed(t, schema, nil, "Middle", nil)
			assertSchemaSimpleTypeFinalAllowed(t, schema, nil, "Outer", nil)
		})
	}
}

//nolint:gocognit // Keep the operation and edition diagnostic matrix together.
func TestSchemaSimpleTypeFinalControlsPreserveWrongKindResolution(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, operation := range schemaSimpleTypeFinalEnforcementOperations() {
			t.Run(profile.name+"/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalWrongKindRoot(profile.version, operation.name)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("discoverTestSchemaWithPolicy accepted a wrong-kind simple-type reference")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeWrongKindCode {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSimpleTypeWrongKindCode)
				}
				if diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaSimpleTypeSpecRef(profile.version))
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, operation.useMarker) {
					t.Fatalf("diagnostic location = %s, want simple-type use", diagnostic.Loc())
				}
				wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:element")}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("diagnostic related = %v, want %v", diagnostic.Related(), wantRelated)
				}
				if !errors.Is(err, errSchemaSimpleTypeBaseWrongKind) {
					t.Fatalf("diagnostic lost wrong-kind cause: %v", err)
				}
			})
		}
	}
}

func schemaSimpleTypeFinalWrongKindRoot(version XSDVersion, operation string) string {
	var use string
	switch operation {
	case "restriction":
		use = `<xs:restriction base="t:Base"/>`
	case "list":
		use = `<xs:list itemType="t:Base"/>`
	case "union":
		use = `<xs:union memberTypes="t:Base"/>`
	default:
		panic("unknown simple-type final enforcement operation")
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="Base"/>
  <xs:simpleType name="Derived">` + use + `</xs:simpleType>
</xs:schema>`
}

//nolint:gocognit // Keep the operation and edition precedence matrix together.
func TestSchemaSimpleTypeFinalControlsPreserveUnsupportedRestrictionPrecedence(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, variety := range []string{"list", "union"} {
			t.Run(profile.name+"/"+variety, func(t *testing.T) {
				root := schemaSimpleTypeFinalUnsupportedRestrictionRoot(profile.version, variety)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("discoverTestSchemaWithPolicy accepted an unsupported simple-type restriction")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want registered unsupported restriction", diagnostic, diagnostic.Feature(), diagnostic.Code())
				}
				if diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaSimpleTypeSpecRef(profile.version))
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `base="t:Base"`) {
					t.Fatalf("diagnostic location = %s, want restriction base use", diagnostic.Loc())
				}
				if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleTypeRestrictionUnsupported) {
					t.Fatalf("unsupported restriction cause was not preserved: %v", err)
				}
				if diagnostic.Code() == diagnosticSchemaSimpleTypeBaseCode {
					t.Fatal("unsupported restriction was misclassified as a final-control derivation failure")
				}
			})
		}
	}
}

func schemaSimpleTypeFinalUnsupportedRestrictionRoot(version XSDVersion, variety string) string {
	definition := `<xs:list itemType="xs:integer"/>`
	if variety == "union" {
		definition = `<xs:union memberTypes="xs:integer xs:string"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="Base" final="restriction">` + definition + `</xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="t:Base"/></xs:simpleType>
</xs:schema>`
}

func TestSchemaSimpleTypeFinalControlsPreserveInvalidListAndUnionPrecedence(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		t.Run(profile.name+"/list", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:simpleType name="Base" final="list"><xs:list itemType="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:list itemType="t:Base"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertSchemaSimpleTypeInvalidDerivation(t, schema, err, root, 3, `itemType="t:Base"`, profile.version)
		})
		t.Run(profile.name+"/union", func(t *testing.T) {
			root := schemaSimpleTypeFinalNestedUnionRoot(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.version == XSDVersion10 {
				assertSchemaSimpleTypeInvalidDerivation(t, schema, err, root, 3, `memberTypes="t:Base"`, profile.version)
				return
			}
			operation := schemaSimpleTypeFinalEnforcementOperation{
				name:           "union",
				useDescription: "union member type",
			}
			assertSchemaSimpleTypeFinalFailure(t, schema, err, root, "root.xsd", 3, `memberTypes="t:Base"`, root, "root.xsd", 2, `final="union"`, profile, operation, "union", mustTestQName(t, "urn:test", "Base"))
		})
	}
}

func assertSchemaSimpleTypeInvalidDerivation(t *testing.T, schema Schema, err error, root string, useLine int, useMarker string, version XSDVersion) {
	t.Helper()
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("discoverTestSchemaWithPolicy accepted an invalid simple-type derivation")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(version) {
		t.Fatalf("diagnostic = %s/%q/%q, want invalid simple-type derivation", diagnostic, diagnostic.Code(), diagnostic.SpecRef())
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, useLine, useMarker) {
		t.Fatalf("diagnostic location = %s, want derivation use", diagnostic.Loc())
	}
	if len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("invalid derivation diagnostic lost related location or cause: %v", err)
	}
	if diagnostic.SpecRef() == schemaSimpleTypeRestrictionSpecRef(version) {
		t.Fatal("invalid shape was misclassified as a final-control failure")
	}
}

func schemaSimpleTypeFinalNestedUnionRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:simpleType name="Base" final="union"><xs:union memberTypes="xs:string"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:union memberTypes="t:Base"/></xs:simpleType>
</xs:schema>`
}

func TestSchemaSimpleTypeFinalControlsDoNotMaskCycles(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">
  <xs:simpleType name="A" final="restriction"><xs:restriction base="t:B"/></xs:simpleType>
  <xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("discoverTestSchemaWithPolicy accepted a cyclic simple-type restriction")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
				t.Fatalf("cycle diagnostic = %s/%q/%q, want located simple-type cycle", diagnostic, diagnostic.Code(), diagnostic.SpecRef())
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="t:B"`) {
				t.Fatalf("cycle diagnostic location = %s, want first base use", diagnostic.Loc())
			}
			if len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("cycle diagnostic lost related location or cause: %v", err)
			}
		})
	}
}

type schemaSimpleTypeFinalDiagnosticSnapshot struct {
	class   FailureClass
	code    string
	feature FeatureID
	loc     Loc
	message string
	related []Loc
	specRef string
}

//nolint:gocognit // Keep the repeated-build diagnostic matrix together.
func TestSchemaSimpleTypeFinalDiagnosticsAreDeterministic(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, operation := range schemaSimpleTypeFinalEnforcementOperations() {
			t.Run(profile.name+"/"+operation.name, func(t *testing.T) {
				root := schemaSimpleTypeFinalEnforcementRoot(profile.version, operation.name, operation.final)
				var first schemaSimpleTypeFinalDiagnosticSnapshot
				for iteration := 0; iteration < 3; iteration++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil {
						t.Fatal("discoverTestSchemaWithPolicy accepted a prohibited simple-type derivation")
					}
					diagnostic := requireDiagnostic(t, err)
					current := schemaSimpleTypeFinalDiagnosticSnapshot{
						class:   diagnostic.Class(),
						code:    diagnostic.Code(),
						feature: diagnostic.Feature(),
						loc:     diagnostic.Loc(),
						message: diagnostic.Message(),
						related: diagnostic.Related(),
						specRef: diagnostic.SpecRef(),
					}
					if iteration == 0 {
						first = current
						continue
					}
					if !reflect.DeepEqual(current, first) {
						t.Fatalf("diagnostic changed on iteration %d: first=%#v current=%#v", iteration, first, current)
					}
					if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
						t.Fatalf("iteration %d lost final-control cause: %v", iteration, err)
					}
				}
			})
		}
	}
}

package goxsd9

import (
	"errors"
	"io"
	"strings"
	"testing"
)

//nolint:gocognit // Keep edition, public facts, mutation, and both consumer boundaries together.
func TestDirectAllParticleRetainsOrderedScalarAndReferenceFacts(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{"compatibility", Compatibility, XSDVersion11},
		{"strict10", Strict10, XSDVersion10},
		{"strict11", Strict11, XSDVersion11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">
  <xs:element name="root" type="r:Record"/>
  <xs:complexType name="Record"><xs:all minOccurs="0">
    <xs:element name="first" type="xs:integer" minOccurs="0"/>
    <xs:element ref="r:forward"/>
    <xs:element name="third" type="xs:decimal"/>
    <xs:element name="fourth" type="xs:boolean"/>
  </xs:all></xs:complexType>
  <xs:element name="forward" type="xs:boolean"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse direct all: %v", err)
			}
			definitions := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
			if len(definitions) != 1 {
				t.Fatalf("Record definitions = %d, want 1", len(definitions))
			}
			definition, ok := definitions[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Record complex type facts missing")
			}
			all, ok := definition.Particle().(AllParticle)
			if !ok {
				t.Fatalf("Record particle = %T, want AllParticle", definition.Particle())
			}
			if got := all.Occurrences().String(); got != "0/1" {
				t.Fatalf("all occurrences = %q, want 0/1", got)
			}
			minimum := all.Occurrences().Minimum()
			minimum.value.SetInt64(9)
			if got := all.Occurrences().String(); got != "0/1" {
				t.Fatalf("caller mutation changed outer bound to %q", got)
			}
			members := all.Members()
			if len(members) != 4 {
				t.Fatalf("member count = %d, want 4", len(members))
			}
			for index, want := range []string{"first", "forward", "third", "fourth"} {
				name, _ := schemaAllMemberNameAndLoc(members[index])
				if name.Local() != want {
					t.Fatalf("member %d name = %q, want %q", index, name, want)
				}
			}
			if got := members[0].Occurrences().String(); got != "0/1" {
				t.Fatalf("first occurrences = %q, want 0/1", got)
			}
			maximum, finite := members[0].Occurrences().Maximum().Finite()
			if !finite {
				t.Fatal("first member maximum is not finite")
			}
			maximum.value.SetInt64(9)
			if got := all.Members()[0].Occurrences().String(); got != "0/1" {
				t.Fatalf("caller mutation changed member bound to %q", got)
			}
			reference, ok := members[1].(ElementReferenceParticle)
			if !ok || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "forward"))[0].ID() {
				t.Fatalf("forward reference = %#v, want target identity", members[1])
			}
			members[0] = nil
			if len(all.Members()) != 4 || all.Members()[0] == nil {
				t.Fatal("caller mutation changed all members")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

func assertDirectAllConsumerRejection(t *testing.T, schema Schema, all AllParticle, version XSDVersion) {
	t.Helper()
	instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:all"/>`)))
	if instanceErr == nil {
		t.Fatal("ValidateInstance accepted an all particle")
	}
	instanceDiagnostic := requireDiagnostic(t, instanceErr)
	if instanceDiagnostic.Class() != FailureUnsupported || instanceDiagnostic.Code() != UnsupportedInstanceValidationCode || instanceDiagnostic.Loc() != all.Loc() || instanceDiagnostic.SpecRef() != instanceValidationSpecRef(version) || !errors.Is(instanceErr, errInstanceChoiceParticle) {
		t.Fatalf("ValidateInstance diagnostic = %s, want located all unsupported with cause", instanceDiagnostic)
	}
	generated, generationErr := GenerateGo(schema, "generated")
	if generated != nil || generationErr == nil || !errors.Is(generationErr, ErrUnsupported) {
		t.Fatalf("GenerateGo = (%q, %v), want nil/unsupported", generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != all.Loc() || generationDiagnostic.SpecRef() != codegenDirectSequenceSpecReference(version, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("GenerateGo diagnostic = %s, want located all unsupported with cause", generationDiagnostic)
	}
}

func allParticleTestRoot(model, extra string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all"><xs:complexType name="Record">` + model + `</xs:complexType>` + extra + `</xs:schema>`
}

func directAllFromSchema(t *testing.T, schema Schema) AllParticle {
	t.Helper()
	definitions := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
	if len(definitions) != 1 {
		t.Fatalf("Record definition count = %d, want 1", len(definitions))
	}
	definition, ok := definitions[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record complex type facts missing")
	}
	all, ok := definition.Particle().(AllParticle)
	if !ok {
		t.Fatalf("Record particle = %T, want AllParticle", definition.Particle())
	}
	return all
}

//nolint:gocognit // Exercise exact all bounds and omission across policies in one table.
func TestDirectAllOccurrenceAndOmissionBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		policy     LanguagePolicy
		model      string
		wantOuter  string
		wantMember []string
	}{
		{"empty XSD 1.0", Strict10, `<xs:all/>`, "1/1", nil},
		{"empty XSD 1.1", Strict11, `<xs:all/>`, "1/1", nil},
		{"optional outer XSD 1.0", Strict10, `<xs:all minOccurs="0"><xs:element name="v" type="xs:integer"/></xs:all>`, "0/1", []string{"1/1"}},
		{"omitted member XSD 1.0", Strict10, `<xs:all><xs:element name="omit" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:boolean"/></xs:all>`, "1/1", []string{"1/1"}},
		{"omitted member XSD 1.1", Strict11, `<xs:all><xs:element name="omit" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:boolean"/></xs:all>`, "1/1", []string{"1/1"}},
		{"exact unbounded member XSD 1.1", Strict11, `<xs:all><xs:element name="v" type="xs:decimal" minOccurs="18446744073709551616" maxOccurs="unbounded"/></xs:all>`, "1/1", []string{"18446744073709551616/unbounded"}},
		{"exact finite member compatibility", Compatibility, `<xs:all><xs:element name="v" type="xs:decimal" minOccurs="2" maxOccurs="18446744073709551616"/></xs:all>`, "1/1", []string{"2/18446744073709551616"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, allParticleTestRoot(test.model, ""), nil, test.policy)
			if err != nil {
				t.Fatalf("parse all: %v", err)
			}
			all := directAllFromSchema(t, schema)
			if got := all.Occurrences().String(); got != test.wantOuter {
				t.Fatalf("outer occurrences = %q, want %q", got, test.wantOuter)
			}
			members := all.Members()
			if len(members) != len(test.wantMember) {
				t.Fatalf("member count = %d, want %d", len(members), len(test.wantMember))
			}
			for index, want := range test.wantMember {
				if got := members[index].Occurrences().String(); got != want {
					t.Fatalf("member %d occurrences = %q, want %q", index, got, want)
				}
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run("omitted outer "+string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:integer"/></xs:all>`, ""), nil, policy)
			if err != nil {
				t.Fatalf("parse omitted all: %v", err)
			}
			definition, _ := schema.Components()[0].ComplexTypeDefinition()
			if definition.Particle() != nil {
				t.Fatalf("omitted all particle = %T, want nil", definition.Particle())
			}
		})
	}
}

//nolint:gocognit // Keep named scalar identity and lexical provenance checks together.
func TestDirectAllNamedScalarTypesRetainProvenance(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:element name="namedInteger" type="r:Count"/><xs:element name="namedBoolean" type="r:Flag"/><xs:element name="namedDecimal" type="r:Amount"/></xs:all>`, `<xs:simpleType name="Count"><xs:restriction base="xs:integer"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType><xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse named all scalars: %v", err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 3 {
				t.Fatalf("member count = %d, want 3", len(members))
			}
			for index, kind := range []string{"Count", "Flag", "Amount"} {
				member, ok := members[index].(ElementParticle)
				if !ok || member.DeclaredType().Local() != kind || member.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="named`+[]string{"Integer", "Boolean", "Decimal"}[index]+`"`, 1) {
					t.Fatalf("named member %d = %#v, want %s with lexical location", index, members[index], kind)
				}
				if _, hasID := member.TypeID(); !hasID {
					t.Fatalf("named member %d has no type ID", index)
				}
			}
		})
	}
}

func TestDirectAllScalarGateAndReferenceTargetSeparation(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		extra   string
		primary string
	}{
		{"named string", `<xs:all><xs:element name="v" type="r:Text"/></xs:all>`, `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Text"`},
		{"inline string", `<xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:element></xs:all>`, "", `<xs:simpleType>`},
		{"inline integer", `<xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:all>`, "", `<xs:simpleType>`},
		{"inline decimal", `<xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:element></xs:all>`, "", `<xs:simpleType>`},
		{"inline boolean", `<xs:all><xs:element name="v"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element></xs:all>`, "", `<xs:simpleType>`},
		{"inline complex", `<xs:all><xs:element name="v"><xs:complexType/></xs:element></xs:all>`, "", `<xs:complexType/>`},
		{"precisionDecimal", `<xs:all><xs:element name="v" type="xs:precisionDecimal"/></xs:all>`, "", `type="xs:precisionDecimal"`},
		{"named precisionDecimal", `<xs:all><xs:element name="v" type="r:Precise"/></xs:all>`, `<xs:simpleType name="Precise"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType>`, `type="r:Precise"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, test.extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("excluded all scalar returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantSpec := schemaAllLimitedSpecRef(XSDVersion11)
			if test.name == "inline string" || test.name == "inline complex" {
				wantSpec = newSchemaSyntaxUnsupportedForVersion(Loc{}, "", XSDVersion11).SpecRef()
			}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || diagnostic.SpecRef() != wantSpec {
				t.Fatalf("diagnostic = %s with spec %q, want all scalar unsupported at %q with spec %q", diagnostic, diagnostic.SpecRef(), test.primary, wantSpec)
			}
		})
	}
	root := allParticleTestRoot(`<xs:all><xs:element ref="r:global"/></xs:all>`, `<xs:element name="global" type="xs:precisionDecimal"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("reference to global precisionDecimal target: %v", err)
	}
	reference, ok := directAllFromSchema(t, schema).Members()[0].(ElementReferenceParticle)
	if !ok || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
		t.Fatalf("reference = %#v, want target ID without copied type facts", directAllFromSchema(t, schema).Members()[0])
	}
}

//nolint:gocognit // Cover local/ref duplicate shapes and omitted terms in one table.
func TestDirectAllRejectsDuplicateSurvivingNames(t *testing.T) {
	tests := []struct {
		name, model, extra, first, second string
	}{
		{"local local", `<xs:all><xs:element name="v" type="xs:integer"/><xs:element name="v" type="xs:decimal"/></xs:all>`, "", `<xs:element name="v" type="xs:integer"`, `<xs:element name="v" type="xs:decimal"`},
		{"local reference", `<xs:all><xs:element name="v" form="qualified" type="xs:integer"/><xs:element ref="r:v"/></xs:all>`, `<xs:element name="v" type="xs:boolean"/>`, `<xs:element name="v" form="qualified"`, `ref="r:v"`},
		{"reference reference", `<xs:all><xs:element ref="r:v"/><xs:element ref="r:v"/></xs:all>`, `<xs:element name="v" type="xs:boolean"/>`, `<xs:element ref="r:v"`, `<xs:element ref="r:v"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, test.extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("duplicate all members returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceDuplicateCode || diagnostic.SpecRef() != schemaAllLimitedSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAllMemberDuplicate) {
				t.Fatalf("duplicate diagnostic = %s, want invalid all duplicate with cause", diagnostic)
			}
			first := allParticleTestTokenLoc(t, "root.xsd", root, test.first, 1)
			second := allParticleTestTokenLoc(t, "root.xsd", root, test.second, 1)
			if test.name == "reference reference" {
				second = allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:v"`, 2)
				first = allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:v"`, 1)
			}
			if diagnostic.Loc() != second || len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != first {
				t.Fatalf("duplicate locations = %s/%v, want %s/%s", diagnostic.Loc(), diagnostic.Related(), second, first)
			}
		})
	}
	root := allParticleTestRoot(`<xs:all><xs:element name="v" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="v" type="xs:decimal"/></xs:all>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil || len(directAllFromSchema(t, schema).Members()) != 1 {
		t.Fatalf("omitted duplicate member = %v/%v, want one surviving member", schema, err)
	}
}

func allParticleTestTokenLoc(t *testing.T, source SourceID, input, token string, occurrence int) Loc {
	t.Helper()
	start := 0
	for index := 0; index < occurrence; index++ {
		relative := strings.Index(input[start:], token)
		if relative < 0 {
			t.Fatalf("missing occurrence %d of %q", occurrence, token)
		}
		start += relative
		if index+1 < occurrence {
			start += len(token)
		}
	}
	line, column := 1, 1
	for _, character := range input[:start] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, source, line, column)
}

//nolint:gocognit // Keep composed ordering, cycle closure, and visibility checks together.
func TestDirectAllReferencesComposeInDiscoveryOrderAndRespectVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" xmlns:o="urn:other" targetNamespace="urn:all"><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Record"><xs:all><xs:element ref="r:included"/><xs:element ref="o:imported"/></xs:all></xs:complexType></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all"><xs:include schemaLocation="root.xsd"/><xs:element name="included" type="xs:integer"/></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="imported" type="xs:boolean"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"included.xsd": {id: "included.xsd", contents: included},
		"other.xsd":    {id: "other.xsd", contents: other},
		"root.xsd":     {id: "root.xsd", contents: root},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err != nil {
		t.Fatalf("parse composed all graph: %v", err)
	}
	components := schema.Components()
	if len(components) != 3 || components[0].ID().Source() != "root.xsd" || components[1].ID().Source() != "included.xsd" || components[2].ID().Source() != "other.xsd" {
		t.Fatalf("component discovery order = %v, want root/include/import", components)
	}
	members := directAllFromSchema(t, schema).Members()
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	for index, target := range components[1:] {
		reference, ok := members[index].(ElementReferenceParticle)
		if !ok || reference.TargetID() != target.ID() {
			t.Fatalf("member %d = %#v, want target %v", index, members[index], target.ID())
		}
	}
	walked := make([]QName, 0, len(components))
	walkErr := schema.Walk(func(component Component) error {
		walked = append(walked, component.Name())
		return nil
	})
	if walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	if len(walked) != len(components) {
		t.Fatalf("walked %d components, want %d", len(walked), len(components))
	}
	for index, component := range components {
		if walked[index] != component.Name() {
			t.Fatalf("walked name %d = %q, want %q", index, walked[index], component.Name())
		}
	}
	outer := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:import namespace="urn:b" schemaLocation="b.xsd"/></xs:schema>`
	a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:a"><xs:complexType name="Record"><xs:all><xs:element ref="b:target"/></xs:all></xs:complexType></xs:schema>`
	b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="target" type="xs:integer"/></xs:schema>`
	invisible, err := discoverTestSchemaWithPolicy(t, outer, map[string]discoveryFixture{
		"a.xsd": {id: "a.xsd", contents: a},
		"b.xsd": {id: "b.xsd", contents: b},
	}, Strict11)
	if err == nil {
		t.Fatal("sibling import target was visible to all reference")
	}
	assertZeroSchema(t, invisible)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceNamespaceCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "a.xsd", a, `ref="b:target"`, 1) || !errors.Is(err, errSchemaElementReferenceNamespace) {
		t.Fatalf("invisible target diagnostic = %s, want located resolution failure at %s, cause %t", diagnostic, allParticleTestTokenLoc(t, "a.xsd", a, `ref="b:target"`, 1), errors.Is(err, errSchemaElementReferenceNamespace))
	}
}

func TestDirectAllReferenceResolutionAndOccurrenceFailuresReturnNoSchema(t *testing.T) {
	tests := []struct {
		name, model, extra, primary, code string
		policy                            LanguagePolicy
		class                             FailureClass
		cause                             error
	}{
		{"unresolved ref", `<xs:all><xs:element ref="r:missing"/></xs:all>`, "", `ref="r:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, Strict11, FailureInvalid, errSchemaElementReferenceUnresolved},
		{"later unresolved ref", `<xs:all><xs:element name="v" type="xs:integer"/><xs:element ref="r:missing"/></xs:all>`, "", `ref="r:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, Strict11, FailureInvalid, errSchemaElementReferenceUnresolved},
		{"wrong-kind ref", `<xs:all><xs:element ref="r:other"/></xs:all>`, `<xs:simpleType name="other"><xs:restriction base="xs:integer"/></xs:simpleType>`, `ref="r:other"`, diagnosticSchemaElementReferenceWrongKindCode, Strict11, FailureInvalid, errSchemaElementReferenceWrongKind},
		{"strict10 outer zero", `<xs:all minOccurs="0" maxOccurs="0"/>`, "", `maxOccurs="0"`, UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"strict10 repeated member", `<xs:all><xs:element name="v" type="xs:integer" maxOccurs="2"/></xs:all>`, "", `maxOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"strict10 repeated minimum", `<xs:all><xs:element name="v" type="xs:integer" minOccurs="2" maxOccurs="2"/></xs:all>`, "", `minOccurs="2"`, diagnosticSchemaAllOccurrenceVersionCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"outer max two", `<xs:all maxOccurs="2"/>`, "", `maxOccurs="2"`, invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"invalid bound lexical", `<xs:all><xs:element name="v" type="xs:integer" maxOccurs="maybe"/></xs:all>`, "", `maxOccurs="maybe"`, invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"member min greater than max", `<xs:all><xs:element name="v" type="xs:integer" minOccurs="2" maxOccurs="1"/></xs:all>`, "", `<xs:element name="v"`, invalidSchemaCompositionCode, Strict11, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
		{"nested choice", `<xs:all><xs:choice><xs:element name="v" type="xs:integer"/></xs:choice></xs:all>`, "", `<xs:choice>`, invalidSchemaCompositionCode, Strict11, FailureInvalid, nil},
		{"wildcard", `<xs:all><xs:any/></xs:all>`, "", `<xs:any`, UnsupportedSchemaSyntaxCode, Strict11, FailureUnsupported, nil},
		{"strict10 wildcard", `<xs:all><xs:any/></xs:all>`, "", `<xs:any`, UnsupportedSchemaSyntaxCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, test.extra)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("invalid or unsupported all returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			primary := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != primary {
				t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, primary)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
		})
	}
}

func TestDirectNestedAllIsInvalidInEveryPolicy(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:all/></xs:all>`, "")
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Compatibility, XSDVersion11},
		{Strict10, XSDVersion10},
		{Strict11, XSDVersion11},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("direct nested all returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all`, 2)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaAllLimitedSpecRef(profile.version) {
				t.Fatalf("nested all diagnostic = %s, want invalid composition at %s for %s", diagnostic, wantLoc, profile.policy)
			}
		})
	}
}

func TestDirectAllXSD11GroupReferenceRemainsUnsupported(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:group ref="r:G"/></xs:all>`, `<xs:group name="G"><xs:all/></xs:group>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("excluded XSD 1.1 all group reference returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:G"`, 1)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc {
		t.Fatalf("all group reference diagnostic = %s, want located unsupported at %s", diagnostic, wantLoc)
	}
}

//nolint:gocognit // Compare live, omitted-member, and omitted-owner all terms under both 1.1 profiles.
func TestDirectAllOmittedOwnerSkipsResolvedUnsupportedMembers(t *testing.T) {
	tests := []struct {
		name, member, zeroMember, primary string
	}{
		{"anonymous integer", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, `<xs:simpleType>`},
		{"precisionDecimal", `<xs:element name="v" type="xs:precisionDecimal"/>`, `<xs:element name="v" type="xs:precisionDecimal" minOccurs="0" maxOccurs="0"/>`, `type="xs:precisionDecimal"`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				live := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, live, nil, policy)
				if err == nil {
					t.Fatal("live excluded all member returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", live, test.primary, 1) || diagnostic.SpecRef() != schemaAllLimitedSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAllMemberScalar) {
					t.Fatalf("live all member diagnostic = %s, want located unsupported scalar with cause", diagnostic)
				}

				memberOmitted := allParticleTestRoot(`<xs:all>`+test.zeroMember+`</xs:all>`, "")
				schema, err = discoverTestSchemaWithPolicy(t, memberOmitted, nil, policy)
				if err != nil {
					t.Fatalf("parse omitted member: %v", err)
				}
				if members := directAllFromSchema(t, schema).Members(); len(members) != 0 {
					t.Fatalf("omitted all member count = %d, want zero", len(members))
				}

				ownerOmitted := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0">`+test.member+`</xs:all>`, "")
				schema, err = discoverTestSchemaWithPolicy(t, ownerOmitted, nil, policy)
				if err != nil {
					t.Fatalf("parse omitted owner: %v", err)
				}
				definitions := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
				if len(definitions) != 1 {
					t.Fatalf("Record definition count = %d, want one", len(definitions))
				}
				definition, ok := definitions[0].ComplexTypeDefinition()
				if !ok {
					t.Fatal("Record complex type facts missing")
				}
				if definition.Particle() != nil {
					t.Fatalf("omitted all owner particle = %v, want nil", definition.Particle())
				}
			})
		}
	}
}

//nolint:gocognit // Exercise the excluded inline-complex shape at live and both omission boundaries.
func TestDirectAllInlineComplexRejectsBeforeZeroOmission(t *testing.T) {
	bodies := []struct {
		name, inline string
	}{
		{"empty", `<xs:complexType/>`},
		{"unresolved simpleContent base", `<xs:complexType><xs:simpleContent><xs:extension base="r:Missing"/></xs:simpleContent></xs:complexType>`},
		{"wrong-kind simpleContent base", `<xs:complexType><xs:simpleContent><xs:extension base="r:Record"/></xs:simpleContent></xs:complexType>`},
		{"unresolved complexContent base", `<xs:complexType><xs:complexContent><xs:extension base="r:Missing"/></xs:complexContent></xs:complexType>`},
		{"policy-gated simpleContent base", `<xs:complexType><xs:simpleContent><xs:extension base="xs:precisionDecimal"/></xs:simpleContent></xs:complexType>`},
	}
	shapes := []struct {
		name, allOpen, elementOpen string
		policy                     LanguagePolicy
		version                    XSDVersion
		live                       bool
	}{
		{"live", `<xs:all>`, `<xs:element name="v">`, Strict11, XSDVersion11, true},
		{"zero member XSD 1.0", `<xs:all>`, `<xs:element name="v" minOccurs="0" maxOccurs="0">`, Strict10, XSDVersion10, false},
		{"zero member XSD 1.1", `<xs:all>`, `<xs:element name="v" minOccurs="0" maxOccurs="0">`, Strict11, XSDVersion11, false},
		{"zero owner XSD 1.1", `<xs:all minOccurs="0" maxOccurs="0">`, `<xs:element name="v">`, Strict11, XSDVersion11, false},
		{"zero owner compatibility", `<xs:all minOccurs="0" maxOccurs="0">`, `<xs:element name="v">`, Compatibility, XSDVersion11, false},
	}
	for _, body := range bodies {
		for _, shape := range shapes {
			t.Run(body.name+"/"+shape.name, func(t *testing.T) {
				root := allParticleTestRoot(shape.allOpen+shape.elementOpen+body.inline+`</xs:element></xs:all>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, shape.policy)
				if err == nil {
					t.Fatal("inline complex all member returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaAllLimitedSpecRef(shape.version)
				if shape.live {
					wantSpec = newSchemaSyntaxUnsupportedForVersion(Loc{}, "", shape.version).SpecRef()
				}
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:complexType`, 2)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("inline complex diagnostic = %s, want unsupported at %s with spec %q", diagnostic, wantLoc, wantSpec)
				}
				if !shape.live && !errors.Is(err, errSchemaAllMemberInlineComplex) {
					t.Fatalf("zero inline complex diagnostic lost cause: %v", err)
				}
			})
		}
	}
}

func TestDirectAllZeroInlineComplexKeepsInvalidSyntax(t *testing.T) {
	for _, model := range []string{
		`<xs:all><xs:element name="v" minOccurs="0" maxOccurs="0"><xs:complexType name="Bad"/></xs:element></xs:all>`,
		`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v"><xs:complexType name="Bad"/></xs:element></xs:all>`,
	} {
		root := allParticleTestRoot(model, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err == nil {
			t.Fatal("invalid inline complex all member returned a schema")
		}
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `name="Bad"`, 1)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc {
			t.Fatalf("invalid inline complex diagnostic = %s, want invalid composition at %s", diagnostic, wantLoc)
		}
	}
}

func TestDirectAllZeroOccurrencePreservesReferenceTypeAndPolicyFailures(t *testing.T) {
	tests := []struct {
		name, model, primary, code string
		policy                     LanguagePolicy
		class                      FailureClass
		cause                      error
	}{
		{"zero member unresolved ref", `<xs:all><xs:element ref="r:missing" minOccurs="0" maxOccurs="0"/></xs:all>`, `ref="r:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, Strict11, FailureInvalid, errSchemaElementReferenceUnresolved},
		{"zero owner unresolved ref", `<xs:all minOccurs="0" maxOccurs="0"><xs:element ref="r:missing"/></xs:all>`, `ref="r:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, Strict11, FailureInvalid, errSchemaElementReferenceUnresolved},
		{"zero member inline unresolved base", `<xs:all><xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="r:Missing"/></xs:simpleType></xs:element></xs:all>`, `base="r:Missing"`, diagnosticSchemaSimpleTypeUnresolvedCode, Strict11, FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
		{"zero owner inline unresolved base", `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v"><xs:simpleType><xs:restriction base="r:Missing"/></xs:simpleType></xs:element></xs:all>`, `base="r:Missing"`, diagnosticSchemaSimpleTypeUnresolvedCode, Strict11, FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
		{"strict10 zero precisionDecimal", `<xs:all><xs:element name="v" type="xs:precisionDecimal" minOccurs="0" maxOccurs="0"/></xs:all>`, `type="xs:precisionDecimal"`, diagnosticSchemaPrecisionDecimalVersionCode, Strict10, FailureUnsupported, errLanguagePolicyMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := allParticleTestRoot(test.model, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("zero occurrence masked a semantic failure")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) || !errors.Is(err, test.cause) {
				t.Fatalf("zero occurrence diagnostic = %s, want %s/%s at %q with cause %v", diagnostic, test.class, test.code, test.primary, test.cause)
			}
		})
	}
	root := allParticleTestRoot(`<xs:all><xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:all>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil || len(directAllFromSchema(t, schema).Members()) != 0 {
		t.Fatalf("valid zero anonymous term = %v/%v, want omitted member", schema, err)
	}
}

func TestAllOtherOwnerShapesRemainLocatedUnsupported(t *testing.T) {
	tests := []struct {
		name, body, primary string
	}{
		{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`},
		{"extension owner", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`},
		{"named model group", `<xs:group name="Group"><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:group>`, `<xs:all>`},
	}
	for _, test := range tests {
		for _, policy := range []LanguagePolicy{Strict10, Strict11} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded all owner returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 1) {
					t.Fatalf("excluded owner diagnostic = %s, want unsupported at %q", diagnostic, test.primary)
				}
			})
		}
	}
}

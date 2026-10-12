package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func xsdNamespaceExtensionRoot(version, base, suffix string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="` + testXSDNamespace + `" version="` + version + `">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="xs:` + base + `"><xs:sequence><xs:element name="term" type="xs:integer" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  ` + suffix + `
</xs:schema>`
}

func xsdNamespaceEmptyBase() string {
	return `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
}

//nolint:gocognit // The public facts share one forward-resolution fixture.
func TestDeclaredXSDNamespaceComplexExtensionKeepsNamedAndRefFacts(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.0"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="` + testXSDNamespace + `" version="` + profile.version + `">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="xs:Base"><xs:sequence minOccurs="0" maxOccurs="2"><xs:element name="term" type="xs:integer" minOccurs="0" maxOccurs="unbounded"/><xs:element ref="xs:target" minOccurs="2" maxOccurs="3"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="target" type="xs:decimal"/>
  ` + xsdNamespaceEmptyBase() + `
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("declared XSD base: %v", err)
			}
			components := schema.Components()
			if len(components) != 3 || components[0].Name().Local() != "Derived" || components[1].Name().Local() != "target" || components[2].Name().Local() != "Base" {
				t.Fatalf("component order = %v, want Derived/target/Base", components)
			}
			derived, ok := components[0].ComplexType()
			if !ok || derived.Base() != mustTestQName(t, testXSDNamespace, "Base") || derived.BaseLoc() != complexContentTestLoc(t, root, `base="xs:Base"`) || derived.Derivation() != ComplexTypeDerivationExtension || derived.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) {
				t.Fatalf("derived base and provenance = %v/%t", derived, ok)
			}
			baseRef, ok := derived.BaseReference()
			baseID, hasID := baseRef.ComponentID()
			if !ok || !baseRef.IsNamed() || !hasID || baseID != components[2].ID() || baseRef.Loc() != derived.BaseLoc() {
				t.Fatalf("resolved base reference = %v/%t", baseRef, ok)
			}
			wildcard, ok := derived.AnyAttribute()
			if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
				t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
			}
			sequence, ok := derived.Particle().(SequenceParticle)
			if !ok || sequence.Occurrences().String() != "0/2" || len(sequence.Particles()) != 2 || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs`) {
				t.Fatalf("derived sequence = %v/%t", derived.Particle(), ok)
			}
			local, localOK := sequence.Particles()[0].(ElementParticle)
			reference, refOK := sequence.Particles()[1].(ElementReferenceParticle)
			if !localOK || local.Name().Local() != "term" || local.Occurrences().String() != "0/unbounded" || local.Loc() != complexContentTestLoc(t, root, `<xs:element name="term"`) || !refOK || reference.Ref() != mustTestQName(t, testXSDNamespace, "target") || reference.TargetID() != components[1].ID() || reference.Occurrences().String() != "2/3" || reference.RefLoc() != complexContentTestLoc(t, root, `ref="xs:target"`) {
				t.Fatalf("local/ref facts = %v/%v", sequence.Particles()[0], sequence.Particles()[1])
			}
			particles := sequence.Particles()
			particles[0] = nil
			if len(sequence.Particles()) != 2 || sequence.Particles()[0] == nil {
				t.Fatal("mutating particle query changed the schema")
			}
			first := schema.Components()
			components[0] = Component{}
			if !reflect.DeepEqual(first, schema.Components()) {
				t.Fatal("mutating component query changed the schema")
			}
			for iteration := 0; iteration < 2; iteration++ {
				repeated, repeatErr := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if repeatErr != nil || !reflect.DeepEqual(first, repeated.Components()) {
					t.Fatalf("repeat %d changed facts: %v", iteration, repeatErr)
				}
			}
		})
	}
}

//nolint:gocognit // Classify each alternate exit at the same extension-base boundary.
func TestDeclaredXSDNamespaceExtensionBaseExits(t *testing.T) {
	cases := []struct {
		name           string
		base           string
		suffix         string
		class          FailureClass
		code           string
		cause          error
		relatedMarker  []string
		otherNamespace bool
	}{
		{name: "builtin anyType", base: "anyType", class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaComplexTypeBaseUnsupported},
		{name: "builtin simple", base: "string", class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind},
		{name: "version-gated builtin simple", base: "precisionDecimal", class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind},
		{name: "unimplemented builtin simple", base: "duration", class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind},
		{name: "absent XSD name", base: "Missing", class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind},
		{name: "unresolved other namespace", base: "Missing", otherNamespace: true, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseUnresolved},
		{name: "declared simple", base: "Simple", suffix: `<xs:simpleType name="Simple"><xs:restriction base="xs:string"/></xs:simpleType>`, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind, relatedMarker: []string{`<xs:simpleType name="Simple"`}},
		{name: "declared element", base: "Element", suffix: `<xs:element name="Element" type="xs:integer"/>`, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind, relatedMarker: []string{`<xs:element name="Element"`}},
		{name: "prohibited final", base: "Base", suffix: `<xs:complexType name="Base" final="extension"/>`, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseUnsupported, relatedMarker: []string{`final="extension"`}},
		{name: "nonempty base", base: "Base", suffix: `<xs:complexType name="Base"><xs:sequence><xs:element name="prior" type="xs:integer"/></xs:sequence></xs:complexType>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaComplexTypeBaseUnsupported, relatedMarker: []string{`<xs:complexType name="Base"`, `<xs:sequence><xs:element name="prior"`}},
		{name: "self cycle", base: "Derived", class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseCycle},
	}
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "strict10", policy: Strict10, version: XSDVersion10},
		{name: "strict11", policy: Strict11, version: XSDVersion11},
	} {
		for _, test := range cases {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := xsdNamespaceExtensionRoot(string(profile.version), test.base, test.suffix)
				baseQName := "xs:" + test.base
				if test.otherNamespace {
					baseQName = "u:" + test.base
					root = strings.Replace(root, `targetNamespace=`, `xmlns:u="urn:missing" targetNamespace=`, 1)
					root = strings.Replace(root, `base="xs:`+test.base+`"`, `base="`+baseQName+`"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("extension-base exit unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != complexContentTestLoc(t, root, `base="`+baseQName+`"`) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, test.cause) {
					t.Fatalf("extension-base diagnostic = %v, want %s/%s/%s with cause %v", err, test.class, test.code, schemaComplexTypeExtensionSpecRef(profile.version), test.cause)
				}
				var wantRelated []Loc
				for _, marker := range test.relatedMarker {
					wantRelated = append(wantRelated, complexContentTestLoc(t, root, marker))
				}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("related locations = %v, want %v", diagnostic.Related(), wantRelated)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported base lost ErrUnsupported: %v", err)
				}
			})
		}
	}
}

func xsdNamespaceImportedBase(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="` + testXSDNamespace + `" version="` + version + `">` + xsdNamespaceEmptyBase() + `</xs:schema>`
}

//nolint:gocognit // Pair imported visibility and transitive invisibility at one lookup boundary.
func TestDeclaredXSDNamespaceExtensionBaseGraphVisibility(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "strict10", policy: Strict10, version: XSDVersion10},
		{name: "strict11", policy: Strict11, version: XSDVersion11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			base := xsdNamespaceImportedBase(string(profile.version))
			owner := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="xs:Base"><xs:sequence><xs:element name="term" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:import namespace="` + testXSDNamespace + `" schemaLocation="base.xsd"/>` + owner + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: base}}, profile.policy)
			if err != nil {
				t.Fatalf("visible imported XSD base: %v", err)
			}
			if len(schema.Documents()) != 2 || len(schema.Components()) != 2 || schema.Components()[0].Name().Local() != "Derived" || schema.Components()[1].Name().Local() != "Base" {
				t.Fatalf("discovery order = %v/%v", schema.Documents(), schema.Components())
			}
			derived, ok := schema.Components()[0].ComplexType()
			if !ok {
				t.Fatal("derived complex view absent")
			}
			ref, ok := derived.BaseReference()
			id, hasID := ref.ComponentID()
			if !ok || !hasID || id != schema.Components()[1].ID() || ref.Loc() != complexContentTestLoc(t, root, `base="xs:Base"`) {
				t.Fatalf("imported base identity/location = %v/%t", ref, ok)
			}

			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="` + string(profile.version) + `"><xs:import namespace="` + testXSDNamespace + `" schemaLocation="base.xsd"/></xs:schema>`
			invisible := strings.Replace(root, `<xs:import namespace="`+testXSDNamespace+`" schemaLocation="base.xsd"/>`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, 1)
			invisibleSchema, invisibleErr := discoverTestSchemaWithPolicy(t, invisible, map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: bridge},
				"base.xsd":   {id: "base.xsd", contents: base},
			}, profile.policy)
			if invisibleErr == nil {
				t.Fatal("transitive imported XSD base unexpectedly visible")
			}
			assertZeroSchema(t, invisibleSchema)
			diagnostic := requireDiagnostic(t, invisibleErr)
			wantRelated := []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != complexContentTestLoc(t, invisible, `base="xs:Base"`) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(invisibleErr, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("invisible base diagnostic = %v; related = %v", invisibleErr, diagnostic.Related())
			}
		})
	}
}

func TestDeclaredXSDNamespaceExtensionBaseAmbiguityAtLookup(t *testing.T) {
	name := mustTestQName(t, testXSDNamespace, "Base")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindComplexTypeDefinition, name: mustTestQName(t, "urn:root", "Derived")}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindComplexTypeDefinition, name: name, loc: mustTestLoc(t, "one.xsd", 2, 3), complexType: &schemaComplexTypeInput{body: &schemaComplexTypeEmptyBodyInput{}}}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindComplexTypeDefinition, name: name, loc: mustTestLoc(t, "two.xsd", 3, 3), complexType: &schemaComplexTypeInput{body: &schemaComplexTypeEmptyBodyInput{}}}
	useLoc := mustTestLoc(t, "root.xsd", 4, 26)
	for _, version := range []XSDVersion{XSDVersion10, XSDVersion11} {
		_, _, err := resolveSchemaComplexTypeExtensionReference(
			schemaComplexTypeReferenceInput{kind: schemaComplexTypeQNameReferenceInput, name: name, loc: useLoc},
			owner,
			[]schemaComponentRecord{owner, first, second},
			map[QName][]int{name: {1, 2}},
			map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
			version,
		)
		if err == nil {
			t.Fatal("ambiguous XSD base unexpectedly resolved")
		}
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != useLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || !errors.Is(err, errSchemaComplexTypeBaseAmbiguous) {
			t.Fatalf("ambiguous XSD base diagnostic = %v", err)
		}
	}
}

//nolint:gocognit // Each admitted owner and term form must keep both consumer exits.
func TestDeclaredXSDNamespaceExtensionConsumersStayUnsupported(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "strict10", policy: Strict10, version: XSDVersion10},
		{name: "strict11", policy: Strict11, version: XSDVersion11},
	} {
		for _, shape := range []struct {
			name   string
			inline bool
			ref    bool
		}{
			{name: "named/local"},
			{name: "named/ref", ref: true},
			{name: "inline/local", inline: true},
			{name: "inline/ref", inline: true, ref: true},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				term := `<xs:element name="term" type="xs:integer"/>`
				if shape.ref {
					term = `<xs:element ref="r:target"/>`
				}
				body := `<xs:complexContent><xs:extension base="xs:Base"><xs:sequence>` + term + `</xs:sequence></xs:extension></xs:complexContent>`
				owner := `<xs:element name="root" type="r:Derived"/><xs:complexType name="Derived">` + body + `</xs:complexType>`
				if shape.inline {
					owner = `<xs:element name="root"><xs:complexType>` + body + `</xs:complexType></xs:element>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:import namespace="` + testXSDNamespace + `" schemaLocation="base.xsd"/>` + owner + `<xs:element name="target" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: xsdNamespaceImportedBase(string(profile.version))}}, profile.policy)
				if err != nil {
					t.Fatalf("admit extension owner: %v", err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				if validationErr == nil {
					t.Fatal("validation accepted extension")
				}
				vd := requireDiagnostic(t, validationErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation diagnostic = %v", validationErr)
				}
				if !slices.Contains(vd.Related(), complexContentTestLoc(t, root, `<xs:extension`)) || !slices.Contains(vd.Related(), complexContentTestLoc(t, root, `base="xs:Base"`)) {
					t.Fatalf("validation related = %v, want extension/base", vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				if generationErr == nil || output != nil {
					t.Fatalf("generation = %q/%v, want no output", output, generationErr)
				}
				gd := requireDiagnostic(t, generationErr)
				generationLoc := complexContentTestLoc(t, root, `<xs:extension`)
				generationSpec := codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference)
				if shape.inline {
					generationLoc = complexContentTestLoc(t, root, `<xs:element name="root"`)
					generationSpec = schemaElementTypeSpecRef(profile.version)
				}
				if gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != generationLoc || gd.SpecRef() != generationSpec || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %v", generationErr)
				}
				if !slices.Contains(gd.Related(), complexContentTestLoc(t, root, `<xs:extension`)) || !slices.Contains(gd.Related(), complexContentTestLoc(t, root, `base="xs:Base"`)) {
					t.Fatalf("generation related = %v, want extension/base", gd.Related())
				}
			})
		}
	}
}

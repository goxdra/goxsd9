package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

var emptyBlockProfiles = []struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}{
	{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
	{name: "Strict10", policy: Strict10, version: XSDVersion10},
	{name: "Strict11", policy: Strict11, version: XSDVersion11},
}

//nolint:gocognit // Keep policy, body, default, and authored-location cases together.
func TestEmptyNamedComplexTypeAdmitsEffectiveEmptyBlock(t *testing.T) {
	cases := []struct {
		name         string
		defaultBlock string
		block        string
		locNeedle    string
	}{
		{name: "omitted"},
		{name: "empty default", defaultBlock: ` blockDefault=""`, locNeedle: `blockDefault=""`},
		{name: "whitespace default", defaultBlock: ` blockDefault=" &#x9;&#xA;&#xD; "`, locNeedle: `blockDefault=`},
		{name: "projected empty default", defaultBlock: ` blockDefault="substitution"`, locNeedle: `blockDefault="substitution"`},
		{name: "explicit empty", block: ` block=""`, locNeedle: `block=""`},
		{name: "explicit whitespace", block: ` block=" &#x9;&#xA;&#xD; "`, locNeedle: `block=`},
		{name: "override extension", defaultBlock: ` blockDefault="extension"`, block: ` block=""`, locNeedle: `block=""`},
		{name: "override all", defaultBlock: ` blockDefault="#all"`, block: ` block=" &#x9;&#xA;&#xD; "`, locNeedle: `block=`},
	}
	for _, profile := range emptyBlockProfiles {
		for _, body := range []struct {
			name string
			text string
		}{{name: "zero length"}, {name: "XML whitespace", text: " \t\n "}} {
			for _, test := range cases {
				t.Run(profile.name+"/"+body.name+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + test.defaultBlock + `><xs:complexType name="Empty"` + test.block + `>` + body.text + `</xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("discoverSchema: %v", err)
					}
					if schema.LanguagePolicy() != profile.policy {
						t.Fatalf("language policy = %q, want %q", schema.LanguagePolicy(), profile.policy)
					}
					components := schema.Components()
					if len(components) != 1 || components[0].Name() != mustTestQName(t, "", "Empty") || components[0].ID().Ordinal() != 1 {
						t.Fatalf("components = %#v, want one Empty complex type", components)
					}
					definition, ok := components[0].ComplexTypeDefinition()
					if !ok {
						t.Fatal("Empty has no complex type view")
					}
					if definition.Particle() != nil || len(definition.AttributeUses()) != 0 || definition.Derivation() != "" {
						t.Fatalf("empty body changed: particle=%T uses=%d derivation=%q", definition.Particle(), len(definition.AttributeUses()), definition.Derivation())
					}
					if got := definition.ProhibitedSubstitutions(); got != nil {
						t.Fatalf("effective block = %#v, want nil", got)
					}
					wantLoc := Loc{}
					if test.locNeedle != "" {
						wantLoc = schemaBlockTestAttributeLoc(t, "root.xsd", root, test.locNeedle)
					}
					if got := definition.ProhibitedSubstitutionsLoc(); got != wantLoc {
						t.Fatalf("block location = %s, want %s", got, wantLoc)
					}
					if documents := schema.Documents(); len(documents) != 1 || len(documents[0].Components()) != 1 || documents[0].Components()[0].ID() != components[0].ID() {
						t.Fatalf("document component projection differs from schema: %#v", documents)
					}
					components[0] = Component{}
					if fresh := schema.Components()[0]; fresh.Name() != mustTestQName(t, "", "Empty") {
						t.Fatal("mutating Components changed schema facts")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep graph order, namespace adoption, and authored policy locations in one assertion.
func TestEmptyNamedComplexTypeBlockKeepsGraphProvenance(t *testing.T) {
	for _, profile := range emptyBlockProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" blockDefault="#all"><xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/><xs:include schemaLocation="child.xsd"/><xs:complexType name="Root" block=""/></xs:schema>`
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="extension"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Child" block=" &#x9; "/></xs:schema>`
			foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" blockDefault="#all"><xs:complexType name="Foreign" block=""/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"root.xsd":    {id: "root.xsd", contents: root},
				"child.xsd":   {id: "child.xsd", contents: child},
				"foreign.xsd": {id: "foreign.xsd", contents: foreign},
			}, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			documents := schema.Documents()
			components := schema.Components()
			if len(documents) != 3 || len(components) != 3 {
				t.Fatalf("document/component counts = %d/%d, want 3/3", len(documents), len(components))
			}
			want := []struct {
				source SourceID
				name   QName
				root   string
				marker string
			}{
				{source: "root.xsd", name: mustTestQName(t, "urn:root", "Root"), root: root, marker: `block=""`},
				{source: "child.xsd", name: mustTestQName(t, "urn:root", "Child"), root: child, marker: `block=" &#x9; "`},
				{source: "foreign.xsd", name: mustTestQName(t, "urn:foreign", "Foreign"), root: foreign, marker: `block=""`},
			}
			for index, expected := range want {
				component := components[index]
				if documents[index].Source() != expected.source || documents[index].TargetNamespace() != expected.name.Namespace() || component.Document() != expected.source || component.Name() != expected.name || component.ID().Ordinal() != 1 {
					t.Fatalf("component %d order/identity/namespace = %q/%q/%q/%q/%d", index, documents[index].Source(), documents[index].TargetNamespace(), component.Document(), component.Name(), component.ID().Ordinal())
				}
				definition, ok := component.ComplexTypeDefinition()
				if !ok || definition.Particle() != nil || definition.ProhibitedSubstitutions() != nil {
					t.Fatalf("component %d lacks empty complex facts", index)
				}
				wantLoc := schemaBlockTestAttributeLoc(t, expected.source, expected.root, expected.marker)
				if definition.ProhibitedSubstitutionsLoc() != wantLoc {
					t.Fatalf("component %d policy Loc = %s, want %s", index, definition.ProhibitedSubstitutionsLoc(), wantLoc)
				}
				if perDocument := documents[index].Components(); len(perDocument) != 1 || perDocument[0].ID() != component.ID() {
					t.Fatalf("document %d components = %#v", index, perDocument)
				}
			}
		})
	}
}

//nolint:gocognit // Assert each alternate exit at the empty-body admission boundary.
func TestEmptyNamedComplexTypeBlockAlternateExits(t *testing.T) {
	cases := []struct {
		name         string
		defaultBlock string
		block        string
		marker       string
		class        FailureClass
		code         string
		cause        error
		blockScope   schemaBlockPolicyScope
	}{
		{name: "explicit extension", block: ` block="extension"`, marker: `block="extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaEmptyComplexBlockUnsupported},
		{name: "explicit restriction", block: ` block="restriction"`, marker: `block="restriction"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaEmptyComplexBlockUnsupported},
		{name: "explicit all", block: ` block="#all"`, marker: `block="#all"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaEmptyComplexBlockUnsupported},
		{name: "inherited extension", defaultBlock: ` blockDefault="extension"`, marker: `blockDefault="extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaEmptyComplexBlockUnsupported},
		{name: "inherited all", defaultBlock: ` blockDefault="#all"`, marker: `blockDefault="#all"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaEmptyComplexBlockUnsupported},
		{name: "invalid explicit token", block: ` block="unknown"`, marker: `block="unknown"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, blockScope: schemaBlockComplex},
		{name: "invalid complex substitution", block: ` block="substitution"`, marker: `block="substitution"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, blockScope: schemaBlockComplex},
		{name: "invalid explicit combination", block: ` block="#all extension"`, marker: `block="#all extension"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, blockScope: schemaBlockComplex},
		{name: "invalid inherited token", defaultBlock: ` blockDefault="unknown"`, marker: `blockDefault="unknown"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, blockScope: schemaBlockDocumentDefault},
		{name: "invalid inherited combination", defaultBlock: ` blockDefault="#all extension"`, marker: `blockDefault="#all extension"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, blockScope: schemaBlockDocumentDefault},
	}
	for _, profile := range emptyBlockProfiles {
		for _, test := range cases {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + test.defaultBlock + `><xs:complexType name="Empty"` + test.block + `/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("result = (%#v, %v), want nil schema and diagnostic", schema, err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, test.marker)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s (cause %v), want %s/%s at %s with %v", d, err, test.class, test.code, wantLoc, test.cause)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported diagnostic lost sentinel: %v", err)
				}
				wantSpec := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef()
				if test.class == FailureInvalid {
					wantSpec = schemaBlockSpecRef(profile.version, test.blockScope)
				}
				if d.SpecRef() != wantSpec || len(d.Related()) != 0 || d.Unwrap() == nil {
					t.Fatalf("diagnostic metadata = spec %q related %v cause %v, want %q/no related/preserved cause", d.SpecRef(), d.Related(), d.Unwrap(), wantSpec)
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Check excluded owner shapes and consumers independently of the named empty-body change.
func TestEmptyBlockOtherOwnersAndConsumersStayAtTheirBoundaries(t *testing.T) {
	for _, profile := range emptyBlockProfiles {
		t.Run(profile.name, func(t *testing.T) {
			invalidOwners := []struct {
				name   string
				body   string
				marker string
				class  FailureClass
				code   string
				cause  error
			}{
				{name: "inline complex", body: `<xs:element name="root"><xs:complexType block=""/></xs:element>`, marker: `block=""`, class: FailureInvalid, code: invalidSchemaCompositionCode},
				{name: "untyped global element", body: `<xs:element name="root" block=""/>`, marker: `block=""`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: ErrUnsupported},
				{name: "named reference to empty", body: `<xs:element name="root" type="r:Empty"/><xs:complexType name="Empty" block=""/>`, marker: `type="r:Empty"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: ErrUnsupported},
				{name: "direct attribute wildcard", body: `<xs:complexType name="Empty" block=""><xs:anyAttribute namespace="##other"/></xs:complexType>`, marker: `<xs:anyAttribute`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyAttributeUnsupported},
			}
			for _, test := range invalidOwners {
				t.Run(test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">` + test.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
						t.Fatalf("owner result = (%#v, %v), want no schema", schema, err)
					}
					d := requireDiagnostic(t, err)
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, test.marker) {
						t.Fatalf("owner diagnostic = %s, want %s/%s at %s", d, test.class, test.code, test.marker)
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("owner diagnostic lost cause: %v", err)
					}
					if len(d.Related()) != 0 {
						t.Fatalf("owner diagnostic related locations = %v, want none", d.Related())
					}
					wantSpec := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef()
					if test.name == "direct attribute wildcard" {
						wantSpec = schemaAnyAttributeSpecRef(profile.version)
					}
					if test.class == FailureUnsupported && d.SpecRef() != wantSpec {
						t.Fatalf("owner SpecRef = %q, want selected edition", d.SpecRef())
					}
				})
			}
			for _, owner := range []struct {
				name string
				body string
			}{
				{name: "direct modeled", body: `<xs:complexType name="Other" block=""><xs:choice><xs:element name="child" type="xs:integer"/></xs:choice></xs:complexType>`},
				{name: "attribute only", body: `<xs:complexType name="Other" block=""><xs:attribute name="value" type="xs:integer"/></xs:complexType>`},
			} {
				t.Run(owner.name, func(t *testing.T) {
					input := `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="#all">` + owner.body + `</xs:schema>`
					otherSchema, otherErr := discoverTestSchemaWithPolicy(t, input, nil, profile.policy)
					if otherErr != nil {
						t.Fatalf("other owner: %v", otherErr)
					}
					components := otherSchema.Components()
					if len(components) != 1 {
						t.Fatalf("component count = %d, want 1", len(components))
					}
					definition, ok := components[0].ComplexTypeDefinition()
					if !ok || definition.ProhibitedSubstitutions() != nil || definition.ProhibitedSubstitutionsLoc() != schemaBlockTestAttributeLoc(t, "root.xsd", input, `block=""`) {
						t.Fatalf("other owner policy = %#v/%s", definition.ProhibitedSubstitutions(), definition.ProhibitedSubstitutionsLoc())
					}
					if owner.name == "direct modeled" && definition.Particle() == nil {
						t.Fatal("modeled owner lost particle")
					}
					if owner.name == "attribute only" && len(definition.AttributeUses()) != 1 {
						t.Fatal("attribute-only owner lost use")
					}
				})
			}

			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" blockDefault="#all"><xs:element name="root" type="xs:integer"/><xs:complexType name="Empty" block=""/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("named reference schema: %v", err)
			}
			if refs := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Empty")); len(refs) != 1 {
				t.Fatalf("named reference target matches = %d, want 1", len(refs))
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root">7</root>`)))
			if validationErr != nil {
				t.Fatalf("unrelated integer validation: %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("generation result = (%q, %v), want nil output and error", output, generationErr)
			}
			generation := requireDiagnostic(t, generationErr)
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, `<xs:complexType`) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic = %s (%v), want located unsupported", generation, generationErr)
			}
			if generation.SpecRef() != codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference) || !reflect.DeepEqual(generation.Related(), []Loc(nil)) {
				t.Fatalf("generation diagnostic metadata = spec %q, related %v", generation.SpecRef(), generation.Related())
			}
		})
	}
}

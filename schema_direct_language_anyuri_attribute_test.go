package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type directBuiltinUseCase struct {
	name     string
	policy   LanguagePolicy
	particle string
}

func directBuiltinUseCases() []directBuiltinUseCase {
	cases := make([]directBuiltinUseCase, 0, 12)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, particle := range []string{"choice", "sequence"} {
			for _, name := range []string{"language", "anyURI"} {
				cases = append(cases, directBuiltinUseCase{name: name, policy: policy, particle: particle})
			}
		}
	}
	return cases
}

func directBuiltinUseBody(particle, attributes string) string {
	return `<xs:complexType name="Record"><xs:` + particle + ` minOccurs="0" maxOccurs="2"><xs:element name="item" type="xs:integer" minOccurs="0" maxOccurs="3"/></xs:` + particle + `>` + attributes + `</xs:complexType>`
}

//nolint:gocognit,funlen // Public copies and exact lexical facts are one admission contract.
func TestDirectLanguageAnyURIAttributeFactsAcrossPolicies(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			attributes := `<xs:attribute name="before" type="xs:integer"/>` +
				`<xs:attribute name="wanted" type="xs:` + test.name + `" use="required" form="unqualified"/>` +
				`<xs:attribute name="gone" type="xs:` + test.name + `" use="prohibited"/>` +
				`<xs:attribute name="after" type="xs:boolean"/>`
			root := languageUseRoot(directBuiltinUseBody(test.particle, attributes))
			schema, err := parseIDUseSchema(t, root, nil, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			if schema.LanguagePolicy() != test.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
				t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 || found[0].ID().Source() != "root.xsd" {
				t.Fatalf("owner = %v", found)
			}
			owner := requireTestComplexTypeDefinition(t, found[0], "Record")
			if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) {
				t.Fatalf("owner Loc = %s", owner.Loc())
			}
			particleLoc := elementReferenceTestAttributeLoc(t, root, `<xs:`+test.particle+` minOccurs=`)
			var child ElementParticle
			switch particle := owner.Particle().(type) {
			case ChoiceParticle:
				if test.particle != "choice" || particle.Loc() != particleLoc || particle.Occurrences().String() != "0/2" || len(particle.Alternatives()) != 1 {
					t.Fatalf("choice = %#v", particle)
				}
				child = normalizedStringChoiceElement(t, particle.Alternatives()[0])
				alternatives := particle.Alternatives()
				alternatives[0] = nil
				if particle.Alternatives()[0] == nil {
					t.Fatal("mutating choice alternatives changed schema")
				}
			case SequenceParticle:
				if test.particle != "sequence" || particle.Loc() != particleLoc || particle.Occurrences().String() != "0/2" || len(particle.Elements()) != 1 {
					t.Fatalf("sequence = %#v", particle)
				}
				child = particle.Elements()[0]
				elements := particle.Elements()
				elements[0] = ElementParticle{}
				if particle.Elements()[0].Name().IsZero() {
					t.Fatal("mutating sequence elements changed schema")
				}
			default:
				t.Fatalf("particle = %T", owner.Particle())
			}
			if child.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="item"`) || child.Occurrences().String() != "0/3" || child.Name() != mustTestQName(t, "", "item") {
				t.Fatalf("child facts = %s/%s/%s", child.Loc(), child.Occurrences(), child.Name())
			}
			uses := owner.AttributeUses()
			if len(uses) != 3 {
				t.Fatalf("effective uses = %v", uses)
			}
			for index, name := range []string{"before", "wanted", "after"} {
				wantType := []string{"integer", test.name, "boolean"}[index]
				local, localOK := uses[index].(LocalAttributeUse)
				attributeLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`)
				attributeTypeLoc := mustTestLoc(t, "root.xsd", attributeLoc.Line(), attributeLoc.Column()+len(`<xs:attribute name="`+name+`" `))
				if !localOK || uses[index].Name().Local() != name || uses[index].Loc() != attributeLoc || local.DeclaredType() != mustTestQName(t, testXSDNamespace, wantType) || local.TypeLoc() != attributeTypeLoc {
					t.Fatalf("use %d = %s/%s", index, uses[index].Name(), uses[index].Loc())
				}
			}
			use, ok := uses[1].(LocalAttributeUse)
			if !ok {
				t.Fatalf("wanted use = %T", uses[1])
			}
			nameLoc := elementReferenceTestAttributeLoc(t, root, `name="wanted"`)
			typeLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:`+test.name+`"`)
			useLoc := elementReferenceTestAttributeLoc(t, root, `use="required"`)
			formLoc := elementReferenceTestAttributeLoc(t, root, `form="unqualified"`)
			if use.Name() != mustTestQName(t, "", "wanted") || use.NameLoc() != nameLoc || use.TypeLoc() != typeLoc || use.UseLoc() != useLoc || use.FormLoc() != formLoc || use.Use() != AttributeUseRequired || use.DeclaredType() != mustTestQName(t, testXSDNamespace, test.name) {
				t.Fatalf("use facts = %#v", use)
			}
			if id, hasID := use.TypeID(); hasID || !id.IsZero() {
				t.Fatalf("built-in type ID = %v/%t", id, hasID)
			}
			ref, ok := use.TypeReference()
			if !ok || !ref.IsBuiltin() || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
				t.Fatalf("type reference = %#v/%t", ref, ok)
			}
			if id, hasID := ref.ComponentID(); hasID || !id.IsZero() {
				t.Fatalf("built-in reference ID = %v/%t", id, hasID)
			}
			uses[0] = nil
			if owner.AttributeUses()[0] == nil {
				t.Fatal("mutating AttributeUses changed schema")
			}
			components := schema.Components()
			components[0] = Component{}
			if schema.Components()[0].Name().Local() != "Record" {
				t.Fatal("mutating Components changed schema")
			}
			var first, second []QName
			for _, walked := range []*[]QName{&first, &second} {
				if walkErr := schema.Walk(func(component Component) error {
					*walked = append(*walked, component.Name())
					return nil
				}); walkErr != nil {
					t.Fatal(walkErr)
				}
			}
			if !reflect.DeepEqual(first, []QName{mustTestQName(t, "urn:root", "Record")}) || !reflect.DeepEqual(first, second) {
				t.Fatalf("walks = %v / %v", first, second)
			}
		})
	}
}

//nolint:gocognit // Discovery and owner provenance are checked through the public schema.
func TestDirectLanguageAnyURIAttributeGraphProvenance(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			owner := func(name, termType string) string {
				return `<xs:complexType name="` + name + `"><xs:` + test.particle + `><xs:element name="item" type="` + termType + `"/></xs:` + test.particle + `><xs:attribute name="value" type="xs:` + test.name + `"/></xs:complexType>`
			}
			root := languageUseRoot(`<xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + owner("Root", "r:Forward") + `<xs:simpleType name="Forward"><xs:restriction base="xs:integer"/></xs:simpleType>`)
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"include.xsd":   {id: "include.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/>` + owner("Included", "xs:integer") + `</xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + owner("Adopted", "xs:integer") + `</xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + owner("Imported", "xs:integer") + `</xs:schema>`},
			}
			schema, err := parseIDUseSchema(t, root, fixtures, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			wantSources := []SourceID{"root.xsd", "include.xsd", "chameleon.xsd", "other.xsd"}
			documents := schema.Documents()
			if len(documents) != len(wantSources) {
				t.Fatalf("documents = %v", documents)
			}
			for i, source := range wantSources {
				if documents[i].Source() != source {
					t.Fatalf("document %d = %s", i, documents[i].Source())
				}
			}
			wantWalk := []string{"root.xsd:Root", "root.xsd:Forward", "include.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Imported"}
			for replay := 0; replay < 2; replay++ {
				var walked []string
				if walkErr := schema.Walk(func(component Component) error {
					walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
					return nil
				}); walkErr != nil {
					t.Fatal(walkErr)
				}
				if !reflect.DeepEqual(walked, wantWalk) {
					t.Fatalf("walk %d = %v", replay, walked)
				}
			}
			for _, want := range []struct {
				name, namespace, attributeNamespace string
				source                              SourceID
				contents                            string
			}{
				{"Root", "urn:root", "urn:root", "root.xsd", root},
				{"Included", "urn:root", "", "include.xsd", fixtures["include.xsd"].contents},
				{"Adopted", "urn:root", "", "chameleon.xsd", fixtures["chameleon.xsd"].contents},
				{"Imported", "urn:other", "", "other.xsd", fixtures["other.xsd"].contents},
			} {
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, want.namespace, want.name))
				if len(found) != 1 || found[0].ID().Source() != want.source || found[0].Loc() != complexContentTestSourceLoc(t, want.source, want.contents, `<xs:complexType name="`+want.name+`"`) {
					t.Fatalf("%s owner = %v", want.name, found)
				}
				uses := requireTestComplexTypeDefinition(t, found[0], want.name).AttributeUses()
				if len(uses) != 1 || uses[0].Loc() != complexContentTestSourceLoc(t, want.source, want.contents, `<xs:attribute name="value"`) || uses[0].Name() != mustTestQName(t, want.attributeNamespace, "value") {
					t.Fatalf("%s uses = %v", want.name, uses)
				}
				local, ok := uses[0].(LocalAttributeUse)
				if !ok || local.DeclaredType() != mustTestQName(t, testXSDNamespace, test.name) || local.TypeLoc() != complexContentTestSourceLoc(t, want.source, want.contents, `type="xs:`+test.name+`"`) {
					t.Fatalf("%s use type = %v", want.name, uses[0])
				}
				if want.name == "Root" {
					definition := requireTestComplexTypeDefinition(t, found[0], want.name)
					var term ElementParticle
					switch particle := definition.Particle().(type) {
					case ChoiceParticle:
						term = normalizedStringChoiceElement(t, particle.Alternatives()[0])
					case SequenceParticle:
						term = particle.Elements()[0]
					default:
						t.Fatalf("root particle = %T", definition.Particle())
					}
					id, hasID := term.TypeID()
					ref, hasRef := term.TypeReference()
					if !hasID || id.Source() != "root.xsd" || term.DeclaredType() != mustTestQName(t, "urn:root", "Forward") || !hasRef || ref.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Forward"`) {
						t.Fatalf("forward term = %#v, type ID %v/%t", term, id, hasID)
					}
				}
			}
		})
	}
}

type directBuiltinExcludedUse struct {
	name, attribute, extra, marker, related string
	cause                                   error
	useSpec                                 bool
}

func directBuiltinExcludedUses(name string) []directBuiltinExcludedUse {
	value := "en"
	if name == "anyURI" {
		value = "../private.xsd"
	}
	alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
	inline := `<xs:simpleType><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
	return []directBuiltinExcludedUse{
		{"inline restriction", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
		{"ref built-in target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:` + name + `"/>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref named target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/>` + alias, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref inline target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"direct default", `<xs:attribute name="a" type="xs:` + name + `" default="` + value + `"/>`, "", `default="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"direct fixed", `<xs:attribute name="a" type="xs:` + name + `" fixed="` + value + `"/>`, "", `fixed="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"named default", `<xs:attribute name="a" type="r:Alias" default="` + value + `"/>`, alias, `default="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"named fixed", `<xs:attribute name="a" type="r:Alias" fixed="` + value + `"/>`, alias, `fixed="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline default", `<xs:attribute name="a" default="` + value + `">` + inline + `</xs:attribute>`, "", `default="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline fixed", `<xs:attribute name="a" fixed="` + value + `">` + inline + `</xs:attribute>`, "", `fixed="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
		{"broader QName", `<xs:attribute name="a" type="xs:QName"/>`, "", `type="xs:QName"`, "", errSchemaAttributeUseUnsupported, false},
	}
}

//nolint:gocognit // Each direct form retains its own located unsupported boundary.
func TestDirectLanguageAnyURIAttributeFormsRemainExcluded(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		for _, excluded := range directBuiltinExcludedUses(test.name) {
			t.Run(string(test.policy)+"/"+test.particle+"/"+test.name+"/"+excluded.name, func(t *testing.T) {
				root := languageUseRoot(directBuiltinUseBody(test.particle, excluded.attribute) + excluded.extra)
				schema, err := parseIDUseSchema(t, root, nil, test.policy)
				if err == nil {
					t.Fatal("excluded local form was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(test.policy))
				if excluded.useSpec {
					wantSpec = schemaAttributeUseSpecRef(languageUseVersion(test.policy))
				}
				var related []Loc
				if excluded.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, excluded.related)}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, excluded.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, excluded.cause) {
					t.Fatalf("%s diagnostic = %v; related = %v", excluded.name, err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Owner gates are distinct from the direct attribute form gates.
func TestDirectLanguageAnyURIOtherOwnersRemainExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			attribute := `<xs:attribute name="a" type="xs:` + name + `"/>`
			for _, test := range []struct{ name, body, extra string }{
				{"direct group", `<xs:complexType name="Record"><xs:group ref="r:Fields"/>` + attribute + `</xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`},
				{"inline choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="x" type="xs:integer"/></xs:choice>` + attribute + `</xs:complexType></xs:element>`, ""},
				{"inline sequence", `<xs:element name="root"><xs:complexType><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>` + attribute + `</xs:complexType></xs:element>`, ""},
				{"simpleContent", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string">` + attribute + `</xs:extension></xs:simpleContent></xs:complexType>`, ""},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(test.body + test.extra)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					if err == nil {
						t.Fatal("excluded owner was admitted")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+name+`"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
						t.Fatalf("owner diagnostic = %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each newly queryable use must stop both public consumers at its own Loc.
func TestDirectLanguageAnyURIAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			root := languageUseRoot(directBuiltinUseBody(test.particle, `<xs:attribute name="value" type="xs:`+test.name+`"/>`) + `<xs:element name="root" type="r:Record"/>`)
			schema, err := parseIDUseSchema(t, root, nil, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
			ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
			elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
			value := "en"
			if test.name == "anyURI" {
				value = "../private.xsd"
			}
			instance := `<r:root xmlns:r="urn:root" value="` + value + `"/>`
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
			vd := requireDiagnostic(t, validationErr)
			validationRelated := []Loc{elementLoc, ownerLoc}
			if test.particle == "sequence" {
				validationRelated = append(validationRelated, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`))
			}
			validationRelated = append(validationRelated, useLoc)
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(test.policy)) || !reflect.DeepEqual(vd.Related(), validationRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("validation diagnostic = %v; related = %v", validationErr, vd.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			wantSpec := codegenDirectSequenceSpecReference(languageUseVersion(test.policy), codegenDirectSequenceParticlesReference)
			if test.particle == "choice" {
				wantSpec = codegenDirectChoiceSpecReference(languageUseVersion(test.policy), codegenDirectChoiceParticlesReference)
			}
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != wantSpec || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v; related = %v", output, generationErr, gd.Related())
			}
		})
	}
}

func TestDirectLanguageAnyURIAttributeGraphAcquisitionRetainsCause(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/>` + directBuiltinUseBody(test.particle, `<xs:attribute name="a" type="xs:`+test.name+`"/>`))
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("direct built-in owner source unavailable")
			schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, test.policy)
			if err == nil {
				t.Fatal("missing included source was admitted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
				t.Fatalf("acquisition diagnostic = %v; related = %v", err, d.Related())
			}
		})
	}
}

type directBuiltinBoundaryCase struct {
	name, attributes, extra, marker, related string
	code, spec                               string
	class                                    FailureClass
	cause                                    error
}

func directBuiltinBoundaryCases(name string) []directBuiltinBoundaryCase {
	return []directBuiltinBoundaryCase{
		{"duplicate use", `<xs:attribute name="a" type="xs:` + name + `"/><xs:attribute name="a" type="xs:` + name + `" use="required"/>`, "", `<xs:attribute name="a" type="xs:` + name + `" use="required"`, `<xs:attribute name="a" type="xs:` + name + `"/>`, diagnosticSchemaAttributeUseDuplicateCode, "use", FailureInvalid, errSchemaAttributeUseDuplicate},
		{"unresolved type", `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, "type", FailureInvalid, errSchemaAttributeTypeUnresolved},
		{"prohibited unresolved type", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, "type", FailureInvalid, errSchemaAttributeTypeUnresolved},
		{"wrong kind type", `<xs:attribute name="a" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, "type", FailureInvalid, errSchemaAttributeTypeWrongKind},
		{"invalid use", `<xs:attribute name="a" type="xs:` + name + `" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
		{"invalid name", `<xs:attribute name="a:b" type="xs:` + name + `"/>`, "", `name="a:b"`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
		{"missing name and ref", `<xs:attribute type="xs:` + name + `"/>`, "", `<xs:attribute type=`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
		{"both name and ref", `<xs:attribute name="a" ref="r:b" type="xs:` + name + `"/>`, "", `ref="r:b"`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
		{"type and inline", `<xs:attribute name="a" type="xs:` + name + `"><xs:simpleType><xs:restriction base="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
		{"model after attribute", `<xs:attribute name="a" type="xs:` + name + `"/><xs:sequence><xs:element name="later" type="xs:integer"/></xs:sequence>`, "", `<xs:sequence><xs:element name="later"`, "", invalidSchemaCompositionCode, "composition", FailureInvalid, nil},
		{"attribute after wildcard", `<xs:anyAttribute namespace="##other"/><xs:attribute name="a" type="xs:` + name + `"/>`, "", `<xs:attribute name="a"`, "", invalidSchemaCompositionCode, "composition", FailureInvalid, nil},
		{"duplicate target before type resolution", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`, `<xs:simpleType name="Alias" id="second"`, `<xs:simpleType name="Alias"><xs:restriction`, diagnosticSchemaGlobalDuplicateCode, "global", FailureInvalid, errSchemaGlobalDeclarationDuplicate},
	}
}

//nolint:gocognit // Alternate exits at the admitted owner retain their own class and provenance.
func TestDirectLanguageAnyURIAttributeBoundaryDiagnostics(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		for _, boundary := range directBuiltinBoundaryCases(test.name) {
			t.Run(string(test.policy)+"/"+test.particle+"/"+test.name+"/"+boundary.name, func(t *testing.T) {
				root := languageUseRoot(directBuiltinUseBody(test.particle, boundary.attributes) + boundary.extra)
				schema, err := parseIDUseSchema(t, root, nil, test.policy)
				if err == nil {
					t.Fatal("boundary error was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(test.policy))
				if boundary.spec == "use" {
					wantSpec = schemaAttributeUseSpecRef(languageUseVersion(test.policy))
				}
				if boundary.spec == "composition" {
					wantSpec = ""
				}
				if boundary.spec == "global" {
					wantSpec = schemaGlobalDuplicateSpecRef(languageUseVersion(test.policy))
				}
				var related []Loc
				if boundary.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, boundary.related)}
				}
				if d.Class() != boundary.class || d.Code() != boundary.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, boundary.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("%s diagnostic = %v; related = %v", boundary.name, err, d.Related())
				}
				if boundary.cause != nil && !errors.Is(err, boundary.cause) {
					t.Fatalf("%s lost cause: %v", boundary.name, err)
				}
				if boundary.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s lost unsupported cause: %v", boundary.name, err)
				}
			})
		}
	}
}

//nolint:gocognit // The omitted particle and its invalid child share one admission boundary.
func TestDirectLanguageAnyURIAttributeZeroParticleRetainsAuthoredOwnerGate(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			body := `<xs:complexType name="Record"><xs:` + test.particle + ` minOccurs="0" maxOccurs="0"><xs:element name="item" type="xs:integer"/></xs:` + test.particle + `><xs:attribute name="kept" type="xs:` + test.name + `"/><xs:attribute name="gone" type="xs:` + test.name + `" use="prohibited"/></xs:complexType>`
			root := languageUseRoot(body)
			schema, err := parseIDUseSchema(t, root, nil, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("owner = %v", found)
			}
			owner := requireTestComplexTypeDefinition(t, found[0], "Record")
			uses := owner.AttributeUses()
			if owner.Particle() != nil || len(uses) != 1 || uses[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="kept"`) || uses[0].Name().Local() != "kept" {
				t.Fatalf("zero particle/use = %T/%v", owner.Particle(), uses)
			}
			bad := strings.Replace(root, `type="xs:integer"`, `type="r:Missing"`, 1)
			badSchema, badErr := parseIDUseSchema(t, bad, nil, test.policy)
			if badErr == nil {
				t.Fatal("zero particle concealed unresolved child type")
			}
			assertZeroSchema(t, badSchema)
			d := requireDiagnostic(t, badErr)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, bad, `type="r:Missing"`) || d.SpecRef() != schemaElementTypeSpecRef(languageUseVersion(test.policy)) || len(d.Related()) != 0 || !errors.Is(badErr, errSchemaElementTypeUnresolved) {
				t.Fatalf("zero particle diagnostic = %v; related = %v", badErr, d.Related())
			}
		})
	}
}

//nolint:gocognit // Verify namespace binding without a resolver call for each direct owner.
func TestDirectLanguageAnyURIAttributeTypePrefixBinding(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			root := languageUseRoot(directBuiltinUseBody(test.particle, `<xs:attribute name="value" type="builtin:`+test.name+`"/>`))
			root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:builtin="`+testXSDNamespace+`"`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			resolver := &discoveryResolver{}
			schema, err := ParseSchemaWithPolicy(source, resolver, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(resolver.calls) != 0 {
				t.Fatalf("built-in type triggered resource resolution: %v", resolver.calls)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("owner = %v", found)
			}
			uses := requireTestComplexTypeDefinition(t, found[0], "Record").AttributeUses()
			if len(uses) != 1 {
				t.Fatalf("uses = %v", uses)
			}
			use, ok := uses[0].(LocalAttributeUse)
			if !ok {
				t.Fatalf("use = %T", uses[0])
			}
			ref, refOK := use.TypeReference()
			wantType := mustTestQName(t, testXSDNamespace, test.name)
			typeLoc := elementReferenceTestAttributeLoc(t, root, `type="builtin:`+test.name+`"`)
			if !refOK || use.DeclaredType() != wantType || ref.QName() != wantType || use.TypeLoc() != typeLoc || ref.Loc() != typeLoc {
				t.Fatalf("bound type = %#v/%#v", use, ref)
			}
		})
	}
}

func TestDirectLanguageAnyURIAttributeInvisibleTypeRetainsResolutionExit(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			t.Run(string(test.policy)+"/"+test.particle+"/"+test.name+"/"+relation, func(t *testing.T) {
				root, fixtures := typeVisibilityTestHiddenGraph(t, string(languageUseVersion(test.policy)), relation, false, true, false, "")
				owner := `<xs:complexType name="Record"><xs:` + test.particle + `><xs:element name="item" type="xs:integer"/></xs:` + test.particle + `><xs:attribute name="first" type="xs:` + test.name + `"/><xs:attribute name="hidden" type="f:Hidden" use="prohibited"/></xs:complexType>`
				root = strings.Replace(root, `<xs:element name="item" type="f:Hidden"/>`, owner, 1)
				schema, err := parseIDUseSchema(t, root, fixtures, test.policy)
				if err == nil {
					t.Fatal("invisible prohibited type was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="f:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(test.policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
					t.Fatalf("visibility diagnostic = %v; related = %v", err, d.Related())
				}
			})
		}
	}
}

func TestDirectLanguageAnyURIAttributeCycleRetainsTypeDiagnostic(t *testing.T) {
	for _, test := range directBuiltinUseCases() {
		t.Run(string(test.policy)+"/"+test.particle+"/"+test.name, func(t *testing.T) {
			root := languageUseRoot(directBuiltinUseBody(test.particle, `<xs:attribute name="first" type="xs:`+test.name+`"/><xs:attribute name="cycle" type="r:First"/>`) + `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`)
			schema, err := parseIDUseSchema(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("cyclic type was admitted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			wantRelated := []Loc{
				elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="cycle"`),
				elementReferenceTestAttributeLoc(t, root, `base="r:Second"`),
				elementReferenceTestAttributeLoc(t, root, `base="r:First"`),
				elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`),
				elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`),
			}
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeCycleCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:First"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(test.policy)) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("cyclic type diagnostic = %v; related = %v", err, d.Related())
			}
		})
	}
}

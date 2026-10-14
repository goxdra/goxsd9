package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

type localNamespaceTerm struct {
	name      goxsd9.QName
	typeName  goxsd9.QName
	loc       goxsd9.Loc
	typeLoc   goxsd9.Loc
	occurs    string
	typeID    goxsd9.ComponentID
	hasTypeID bool
}

func localNamespaceLoc(t *testing.T, source goxsd9.SourceID, contents, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.LastIndex(contents, marker)
	if index < 0 {
		t.Fatalf("%s lacks %q", source, marker)
	}
	line, column := 1, 1
	for _, r := range contents[:index] {
		if r == '\n' {
			line, column = line+1, 1
			continue
		}
		column++
	}
	loc, err := goxsd9.NewLoc(source, line, column)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

//nolint:gocognit // Check ordered copies and public term facts in one view helper.
func localNamespaceOwnerTerms(t *testing.T, schema goxsd9.Schema, namespace, owner, model string) []localNamespaceTerm {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, owner)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s: %d owner components, want one", name, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex type view", name)
	}
	var elements []goxsd9.ElementParticle
	switch model {
	case "choice":
		choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
		if !ok {
			t.Fatalf("%s particle = %T, want choice", name, definition.Particle())
		}
		alternatives := choice.Alternatives()
		for _, alternative := range alternatives {
			element, ok := alternative.(goxsd9.ElementParticle)
			if !ok {
				t.Fatalf("%s alternative = %T, want element", name, alternative)
			}
			elements = append(elements, element)
		}
		if len(alternatives) > 0 {
			alternatives[0] = nil
			if len(choice.Alternatives()) != len(elements) || choice.Alternatives()[0] == nil {
				t.Fatal("choice alternatives alias schema storage")
			}
		}
	case "sequence":
		sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
		if !ok {
			t.Fatalf("%s particle = %T, want sequence", name, definition.Particle())
		}
		elements = sequence.Elements()
		if len(elements) > 0 {
			elements[0] = goxsd9.ElementParticle{}
			if sequence.Elements()[0].Name().Local() == "" {
				t.Fatal("sequence elements alias schema storage")
			}
			elements = sequence.Elements()
		}
	default:
		t.Fatalf("unknown model %q", model)
	}
	result := make([]localNamespaceTerm, 0, len(elements))
	for _, element := range elements {
		reference, ok := element.TypeReference()
		if !ok || reference.Loc().IsZero() {
			t.Fatalf("%s missing located type reference", element.Name())
		}
		id, hasID := element.TypeID()
		result = append(result, localNamespaceTerm{
			name: element.Name(), typeName: element.DeclaredType(), loc: element.Loc(),
			typeLoc: reference.Loc(), occurs: element.Occurrences().String(), typeID: id, hasTypeID: hasID,
		})
	}
	return result
}

func localNamespaceGraph(explicit bool) (string, map[string]string) {
	xsd := `http://www.w3.org/2001/XMLSchema`
	local := func(namespace, name, model string) string {
		attribute := ` form="qualified"`
		if explicit {
			attribute = ` targetNamespace="  ` + namespace + `  "`
		}
		return `<xs:complexType name="` + name + `"><xs:` + model + `><xs:element name="before" type="xs:integer"` + attribute + ` minOccurs="0" maxOccurs="3"/><xs:element name="omitted" type="xs:integer"` + attribute + ` minOccurs="0" maxOccurs="0"/><xs:element name="after" type="xs:integer"` + attribute + `/></xs:` + model + `></xs:complexType>`
	}
	root := `<xs:schema xmlns:xs="` + xsd + `" targetNamespace="urn:root" version="1.0"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + local("urn:root", "Root", "choice") + `</xs:schema>`
	documents := map[string]string{
		"root.xsd":      root,
		"included.xsd":  `<xs:schema xmlns:xs="` + xsd + `" targetNamespace="urn:root" version="1.1"><xs:include schemaLocation="root.xsd"/>` + local("urn:root", "Included", "sequence") + `</xs:schema>`,
		"chameleon.xsd": `<xs:schema xmlns:xs="` + xsd + `" version="1.0">` + local("urn:root", "Chameleon", "choice") + `</xs:schema>`,
		"other.xsd":     `<xs:schema xmlns:xs="` + xsd + `" targetNamespace="urn:other" version="1.1"><xs:import namespace="urn:root" schemaLocation="root.xsd"/>` + local("urn:other", "Imported", "sequence") + `</xs:schema>`,
	}
	return root, documents
}

//nolint:gocognit // Compare all graph owners and their ordered public views together.
func TestMatchingLocalElementNamespaceGraphEquivalence(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			explicitRoot, explicitDocuments := localNamespaceGraph(true)
			qualifiedRoot, qualifiedDocuments := localNamespaceGraph(false)
			explicit := parseElementNamespaceSchema(t, policy, explicitRoot, explicitDocuments)
			qualified := parseElementNamespaceSchema(t, policy, qualifiedRoot, qualifiedDocuments)
			wantSources := []goxsd9.SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
			for _, schema := range []goxsd9.Schema{explicit, qualified} {
				documents := schema.Documents()
				if len(documents) != len(wantSources) {
					t.Fatalf("document count = %d, want %d", len(documents), len(wantSources))
				}
				for index, source := range wantSources {
					if documents[index].Source() != source {
						t.Fatalf("document %d source = %q, want %q", index, documents[index].Source(), source)
					}
				}
			}
			for _, owner := range []struct{ namespace, name, model, source string }{
				{"urn:root", "Root", "choice", "root.xsd"},
				{"urn:root", "Included", "sequence", "included.xsd"},
				{"urn:root", "Chameleon", "choice", "chameleon.xsd"},
				{"urn:other", "Imported", "sequence", "other.xsd"},
			} {
				t.Run(owner.name, func(t *testing.T) {
					got := localNamespaceOwnerTerms(t, explicit, owner.namespace, owner.name, owner.model)
					want := localNamespaceOwnerTerms(t, qualified, owner.namespace, owner.name, owner.model)
					for _, variant := range []struct {
						terms []localNamespaceTerm
						root  string
						docs  map[string]string
					}{{got, explicitRoot, explicitDocuments}, {want, qualifiedRoot, qualifiedDocuments}} {
						contents := variant.root
						if owner.source != "root.xsd" {
							contents = variant.docs[owner.source]
						}
						for index, local := range []string{"before", "after"} {
							if len(variant.terms) != 2 {
								t.Fatalf("%s term count = %d, want 2", owner.name, len(variant.terms))
							}
							marker := `<xs:element name="` + local + `"`
							if variant.terms[index].loc != localNamespaceLoc(t, goxsd9.SourceID(owner.source), contents, marker) || variant.terms[index].typeLoc.IsZero() || variant.terms[index].typeLoc.Source() != goxsd9.SourceID(owner.source) {
								t.Fatalf("%s provenance = %#v", owner.name, variant.terms[index])
							}
						}
					}
					for index := range got {
						got[index].loc = goxsd9.Loc{}
						got[index].typeLoc = goxsd9.Loc{}
					}
					for index := range want {
						want[index].loc = goxsd9.Loc{}
						want[index].typeLoc = goxsd9.Loc{}
					}
					if !reflect.DeepEqual(got, want) || len(got) != 2 {
						t.Fatalf("explicit terms = %#v; qualified terms = %#v", got, want)
					}
					if got[0].name.String() != "{"+owner.namespace+"}before" || got[0].occurs != "0/3" || got[1].name.String() != "{"+owner.namespace+"}after" || got[1].occurs != "1/1" {
						t.Fatalf("owner %s ordered facts = %#v", owner.name, got)
					}
					for _, term := range got {
						if term.typeName.String() != "{http://www.w3.org/2001/XMLSchema}integer" || term.hasTypeID {
							t.Fatalf("unexpected type facts: %#v", term)
						}
						if matches := explicit.FindKind(goxsd9.ComponentKindElementDeclaration, term.name); len(matches) != 0 {
							t.Fatalf("local %s became global: %v", term.name, matches)
						}
					}
				})
			}
		})
	}
}

//nolint:gocognit // The two equivalent forms share the same bounds assertions.
func TestMatchingLocalElementNamespaceKeepsNamedTypeBounds(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(string(policy)+"/"+model, func(t *testing.T) {
				ids := make([]goxsd9.ComponentID, 0, 2)
				for _, attribute := range []string{`targetNamespace="urn:root"`, `form="qualified"`} {
					root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:r="urn:root" targetNamespace="urn:root"><xs:complexType name="Owner"><xs:` + model + `><xs:element name="value" type="r:Bound" ` + attribute + ` minOccurs="0" maxOccurs="4"/></xs:` + model + `></xs:complexType><xs:simpleType name="Bound"><xs:restriction base="xs:positiveInteger"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType></xs:schema>`
					schema := parseElementNamespaceSchema(t, policy, root, nil)
					name, err := goxsd9.NewQName("urn:root", "Owner")
					if err != nil {
						t.Fatal(err)
					}
					owners := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
					if len(owners) != 1 {
						t.Fatalf("owners = %v", owners)
					}
					definition, ok := owners[0].ComplexTypeDefinition()
					if !ok {
						t.Fatal("owner has no complex type view")
					}
					var element goxsd9.ElementParticle
					switch model {
					case "choice":
						choice, choiceOK := definition.Particle().(goxsd9.ChoiceParticle)
						if !choiceOK || len(choice.Alternatives()) != 1 {
							t.Fatalf("particle = %T, want one-element choice", definition.Particle())
						}
						element, choiceOK = choice.Alternatives()[0].(goxsd9.ElementParticle)
						if !choiceOK {
							t.Fatalf("alternative = %T, want element", choice.Alternatives()[0])
						}
					case "sequence":
						sequence, sequenceOK := definition.Particle().(goxsd9.SequenceParticle)
						if !sequenceOK || len(sequence.Elements()) != 1 {
							t.Fatalf("particle = %T, want one-element sequence", definition.Particle())
						}
						element = sequence.Elements()[0]
					}
					if element.Name().String() != "{urn:root}value" || element.Occurrences().String() != "0/4" || element.Loc() != localNamespaceLoc(t, "root.xsd", root, `<xs:element name="value"`) {
						t.Fatalf("element facts = %s/%s/%s", element.Name(), element.Occurrences(), element.Loc())
					}
					id, hasID := element.TypeID()
					reference, hasReference := element.TypeReference()
					refID, hasRefID := reference.ComponentID()
					boundName, err := goxsd9.NewQName("urn:root", "Bound")
					if err != nil {
						t.Fatal(err)
					}
					targets := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, boundName)
					if !hasID || !hasReference || !hasRefID || len(targets) != 1 || id != refID || id != targets[0].ID() || reference.Loc() != localNamespaceLoc(t, "root.xsd", root, `type="r:Bound"`) {
						t.Fatalf("named type reference = %v/%t %v/%t", id, hasID, refID, hasRefID)
					}
					bounds, ok := reference.IntegerBounds()
					minimum, hasMin := bounds.MinInclusiveFacet()
					if !ok || !hasMin || len(bounds.Bounds()) != 1 || minimum.Value().Canonical() != "2" || minimum.Loc() != localNamespaceLoc(t, "root.xsd", root, `value="2"`) {
						t.Fatalf("bounds = %#v/%t", minimum, hasMin)
					}
					copyBounds := bounds.Bounds()
					copyBounds[0] = goxsd9.IntegerBoundFacet{}
					repeated, repeatedOK := element.TypeReference()
					repeatedBounds, repeatedBoundsOK := repeated.IntegerBounds()
					repeatedMin, repeatedMinOK := repeatedBounds.MinInclusiveFacet()
					if !repeatedOK || !repeatedBoundsOK || !repeatedMinOK || repeatedMin.Value().Canonical() != "2" {
						t.Fatal("copied bound changed schema")
					}
					ids = append(ids, id)
				}
				if ids[0] != ids[1] {
					t.Fatalf("type identity changed between equivalent forms: %v", ids)
				}
			})
		}
	}
}

//nolint:gocognit // Check both consumers for each policy and direct model.
func TestMatchingLocalElementNamespaceConsumersStayBounded(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(string(policy)+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:r="urn:root" targetNamespace="urn:root"><xs:element name="root" type="r:Owner"/><xs:complexType name="Owner"><xs:` + model + `><xs:element name="value" type="xs:normalizedString" targetNamespace="urn:root"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema := parseElementNamespaceSchema(t, policy, root, nil)
				terms := localNamespaceOwnerTerms(t, schema, "urn:root", "Owner", model)
				if len(terms) != 1 || terms[0].name.String() != "{urn:root}value" {
					t.Fatalf("local terms = %#v", terms)
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = %q/%v", output, err)
				}
				generated := elementNamespaceDiagnostic(t, err)
				generationSpec := "xsd11-structures#element-" + model
				if generated.Code() != "GOXSD9029" || generated.Class() != goxsd9.FailureUnsupported || generated.Loc() != terms[0].loc || generated.SpecRef() != generationSpec || len(generated.Related()) != 0 || generated.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("generation diagnostic = %v", err)
				}
				err = goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>a b</value></root>`)))
				if err == nil {
					t.Fatal("ValidateInstance accepted query-only normalizedString local")
				}
				validated := elementNamespaceDiagnostic(t, err)
				wantRelated := []goxsd9.Loc{
					localNamespaceLoc(t, "root.xsd", root, `<xs:element name="root"`),
					localNamespaceLoc(t, "root.xsd", root, `<xs:complexType name="Owner"`),
					localNamespaceLoc(t, "root.xsd", root, `<xs:`+model),
					terms[0].loc,
				}
				wantValidationLoc := terms[0].loc
				if model == "sequence" {
					var locErr error
					wantValidationLoc, locErr = goxsd9.NewLoc("instance.xml", 1, 1)
					if locErr != nil {
						t.Fatal(locErr)
					}
				}
				if validated.Code() != goxsd9.UnsupportedInstanceValidationCode || validated.Class() != goxsd9.FailureUnsupported || validated.Loc() != wantValidationLoc || validated.SpecRef() != "xsd11-structures#cvc-elt" || !reflect.DeepEqual(validated.Related(), wantRelated) || validated.Unwrap() == nil || !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("validation diagnostic = %v", err)
				}
			})
		}
	}
}

// The policy, local grammar, and namespace comparison each have a separate exit.
//
//nolint:gocognit // The grammar exits share one located diagnostic contract.
func TestMatchingLocalElementNamespaceBoundaryDiagnostics(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		for _, test := range []struct {
			name, schemaTarget, element, tail, marker, related, code, spec, cause string
			policy                                                                goxsd9.LanguagePolicy
			class                                                                 goxsd9.FailureClass
		}{
			{"different/direct", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="urn:other"/>`, "", `targetNamespace="urn:other"`, `targetNamespace="urn:root"`, "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"different/named", ` targetNamespace="urn:root"`, `<xs:element name="value" type="r:Named" targetNamespace="urn:other"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`, `targetNamespace="urn:other"`, `targetNamespace="urn:root"`, "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"different/inline", ` targetNamespace="urn:root"`, `<xs:element name="value" targetNamespace="urn:other"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, "", `targetNamespace="urn:other"`, `targetNamespace="urn:root"`, "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"different/zero", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="urn:other" minOccurs="0" maxOccurs="0"/>`, "", `targetNamespace="urn:other"`, `targetNamespace="urn:root"`, "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"absent/direct", "", `<xs:element name="value" type="xs:integer" targetNamespace="urn:root"/>`, "", `targetNamespace="urn:root"`, "", "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Compatibility, goxsd9.FailureInvalid},
			{"absent/named", "", `<xs:element name="value" type="r:Named" targetNamespace="urn:root"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`, `targetNamespace="urn:root"`, "", "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"absent/inline", "", `<xs:element name="value" targetNamespace="urn:root"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, "", `targetNamespace="urn:root"`, "", "XSD3010", "xsd11-structures#dcl.elt.local", "no matching containing schema targetNamespace", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"form conflict/direct", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="urn:root" form="qualified"/>`, "", `form="qualified"`, "", "XSD3010", "", "", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"form conflict/named", ` targetNamespace="urn:root"`, `<xs:element name="value" type="r:Named" targetNamespace="urn:root" form="qualified"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`, `form="qualified"`, "", "XSD3010", "", "", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"form conflict/inline", ` targetNamespace="urn:root"`, `<xs:element name="value" targetNamespace="urn:root" form="qualified"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, "", `form="qualified"`, "", "XSD3010", "", "", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"ref conflict", ` targetNamespace="urn:root"`, `<xs:element ref="r:value" targetNamespace="urn:root"/>`, "", `targetNamespace="urn:root"`, "", "XSD3010", "", "", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"malformed URI", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="bad%ZZ"/>`, "", `targetNamespace="bad%ZZ"`, "", "XSD3010", "", "", goxsd9.Strict11, goxsd9.FailureInvalid},
			{"Strict10 mismatch", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="urn:root"/>`, "", `targetNamespace="urn:root"`, "", goxsd9.UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", "outside the selected XSD 1.0 policy", goxsd9.Strict10, goxsd9.FailureUnsupported},
			{"Strict10/zero", ` targetNamespace="urn:root"`, `<xs:element name="value" type="xs:integer" targetNamespace="urn:root" minOccurs="0" maxOccurs="0"/>`, "", `targetNamespace="urn:root"`, "", goxsd9.UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", "outside the selected XSD 1.0 policy", goxsd9.Strict10, goxsd9.FailureUnsupported},
			{"unresolved/zero", ` targetNamespace="urn:root"`, `<xs:element name="value" type="r:Missing" targetNamespace="urn:root" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", "XSD3019", "xsd11-structures#Element_Declaration_details", "element type is unresolved", goxsd9.Strict11, goxsd9.FailureInvalid},
		} {
			t.Run(model+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:r="urn:root"` + test.schemaTarget + `><xs:complexType name="Owner"><xs:` + model + `>` + test.element + `</xs:` + model + `></xs:complexType>` + test.tail + `</xs:schema>`
				schema, err := parseElementNamespaceSchemaResult(t, test.policy, root, nil)
				if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatalf("parse result = %v/%v, want error and nil schema", schema, err)
				}
				diagnostic := elementNamespaceDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != localNamespaceLoc(t, "root.xsd", root, test.marker) {
					t.Fatalf("diagnostic = %v, want %s/%s at %s", err, test.class, test.code, test.marker)
				}
				if diagnostic.SpecRef() != test.spec {
					t.Fatalf("SpecRef = %q, want %q", diagnostic.SpecRef(), test.spec)
				}
				var related []goxsd9.Loc
				if test.related != "" {
					related = []goxsd9.Loc{localNamespaceLoc(t, "root.xsd", root, test.related)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
				if test.cause != "" && (diagnostic.Unwrap() == nil || !strings.Contains(diagnostic.Unwrap().Error(), test.cause)) {
					t.Fatalf("cause = %v, want %q", diagnostic.Unwrap(), test.cause)
				}
				if test.cause == "" && diagnostic.Unwrap() != nil {
					t.Fatalf("unexpected cause = %v", diagnostic.Unwrap())
				}
				if test.class == goxsd9.FailureUnsupported && !errors.Is(err, goxsd9.ErrUnsupported) {
					t.Fatalf("unsupported cause lost: %v", err)
				}
			})
		}
	}
}

func TestMatchingLocalElementNamespaceDoesNotExpandOwnerShapes(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"inline choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="value" type="xs:integer" targetNamespace="urn:root"/></xs:choice></xs:complexType></xs:element>`},
		{"named group", `<xs:group name="Fields"><xs:sequence><xs:element name="value" type="xs:integer" targetNamespace="urn:root"/></xs:sequence></xs:group>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root">` + test.body + `</xs:schema>`
			schema, err := parseElementNamespaceSchemaResult(t, goxsd9.Strict11, root, nil)
			if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatalf("owner shape result = %v/%v", schema, err)
			}
			diagnostic := elementNamespaceDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || diagnostic.Loc() != localNamespaceLoc(t, "root.xsd", root, `targetNamespace="urn:root"`) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("%s diagnostic = %v", test.name, err)
			}
		})
	}
}

package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func negativeAllAttributeSchema(model, uses, attributes string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` +
		`<xs:element name="root" type="r:Record"/><xs:complexType name="Record">` + model + uses +
		`<xs:anyAttribute` + attributes + `/></xs:complexType><xs:element name="forward" type="xs:string"/></xs:schema>`
}

func negativeAllDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record definitions = %d", len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record is not a complex type")
	}
	return definition
}

//nolint:gocognit,funlen // Keep public wildcard, all, and use facts in the admission matrix.
func TestNegativeAnyAttributeNamedDirectAllPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []struct{ name, lexical, want string }{
			{"default", "", "strict"},
			{"strict", ` processContents="strict"`, "strict"},
			{"lax", ` processContents="lax"`, "lax"},
			{"skip", ` processContents="skip"`, "skip"},
		} {
			for _, qname := range []struct{ name, lexical, want string }{
				{"absent", "", ""},
				{"empty", ` notQName=""`, ""},
				{"xml-space", ` notQName="&#x9;&#xA;&#xD; "`, ""},
			} {
				for _, shape := range []struct{ name, model, memberRange string }{
					{"empty", `<xs:all/>`, ""},
					{"members", `<xs:all minOccurs="0"><xs:element name="local" type="xs:string" minOccurs="0"/><xs:element ref="r:forward"/></xs:all>`, "0/1"},
				} {
					for _, use := range []struct{ name, lexical string }{{"zero", ""}, {"one", `<xs:attribute name="flag" type="xs:ID" use="optional"/>`}} {
						t.Run(string(policy)+"/"+mode.name+"/"+qname.name+"/"+shape.name+"/"+use.name, func(t *testing.T) {
							attributes := ` notNamespace=" urn:z  ##targetNamespace urn:a ##local urn:z "` + mode.lexical + qname.lexical
							root := negativeAllAttributeSchema(shape.model, use.lexical, attributes)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
							if err != nil {
								t.Fatalf("parse: %v", err)
							}
							definition := negativeAllDefinition(t, schema)
							wildcard, ok := definition.AnyAttribute()
							if !ok {
								t.Fatal("anyAttribute missing")
							}
							constraint := wildcard.NamespaceConstraint()
							wantNamespaces := []string{"", "urn:a", "urn:all", "urn:z"}
							if constraint.Variety() != WildcardNamespaceConstraintNot || !reflect.DeepEqual(constraint.Namespaces(), wantNamespaces) ||
								constraint.LexicalForm() != "urn:z ##targetNamespace urn:a ##local urn:z" || wildcard.Namespace() != constraint.LexicalForm() ||
								wildcard.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || constraint.Loc() != anyAttributeTestLoc(root, `notNamespace=`) ||
								wildcard.NamespaceLoc() != constraint.Loc() || wildcard.ProcessContents() != mode.want ||
								wildcard.ProcessContentsLoc() != anyAttributeTestLocOptional(root, strings.TrimSpace(mode.lexical)) {
								t.Fatalf("wildcard facts = %#v, namespaces %v", wildcard, constraint.Namespaces())
							}
							qfacts := wildcard.QNameConstraint()
							if qfacts.Present() != (qname.lexical != "") || qfacts.LexicalForm() != qname.want ||
								qfacts.Loc() != anyAttributeTestLocOptional(root, strings.TrimSpace(qname.lexical)) ||
								len(qfacts.Tokens()) != 0 || len(qfacts.Names()) != 0 {
								t.Fatalf("QName facts = %#v", qfacts)
							}
							if qname.lexical != "" {
								bindings := qfacts.NamespaceBindings()
								if len(bindings) == 0 {
									t.Fatal("explicit notQName lost namespace bindings")
								}
								bindings[0].Namespace = "changed"
								if definitionAnyAttribute(t, definition).QNameConstraint().NamespaceBindings()[0].Namespace == "changed" {
									t.Fatal("QName bindings leaked backing storage")
								}
							}
							wantUses := 0
							if use.lexical != "" {
								wantUses = 1
							}
							if got := len(definition.AttributeUses()); got != wantUses {
								t.Fatalf("attribute uses = %d", got)
							}
							if use.lexical != "" {
								attribute := requireBoundedAllUse(t, definition, 0)
								if attribute.Name().Local() != "flag" || attribute.Loc() != anyAttributeTestLoc(root, `<xs:attribute name="flag"`) {
									t.Fatalf("attribute use = %#v", attribute)
								}
							}
							all, ok := definition.Particle().(AllParticle)
							if !ok || all.Loc() != anyAttributeTestLoc(root, `<xs:all`) || all.Occurrences().String() != map[bool]string{true: "0/1", false: "1/1"}[shape.memberRange != ""] {
								t.Fatalf("all particle = %#v", definition.Particle())
							}
							members := all.Members()
							if shape.memberRange == "" && len(members) != 0 || shape.memberRange != "" && len(members) != 2 {
								t.Fatalf("all members = %#v", members)
							}
							if shape.memberRange != "" {
								local, localOK := members[0].(ElementParticle)
								ref, refOK := members[1].(ElementReferenceParticle)
								if !localOK || !refOK || local.Name().Local() != "local" || ref.Name().Local() != "forward" ||
									local.Occurrences().String() != "0/1" || ref.Occurrences().String() != "1/1" ||
									local.Loc() != anyAttributeTestLoc(root, `<xs:element name="local"`) || ref.Loc() != anyAttributeTestLoc(root, `<xs:element ref=`) {
									t.Fatalf("member order/bounds/locations = %#v", members)
								}
								members[0] = nil
								if all.Members()[0] == nil {
									t.Fatal("all members leaked backing storage")
								}
								minimum := local.Occurrences().Minimum()
								minimum.value.SetInt64(9)
								if all.Members()[0].Occurrences().String() != "0/1" {
									t.Fatal("member occurrence bound leaked mutable integer")
								}
							}
							copied := constraint.Namespaces()
							copied[0] = "changed"
							if !reflect.DeepEqual(definitionAnyAttribute(t, definition).NamespaceConstraint().Namespaces(), wantNamespaces) {
								t.Fatal("wildcard namespaces leaked backing storage")
							}
						})
					}
				}
			}
		}
	}
}

func TestNegativeAnyAttributeNamedDirectAllOmissionAndConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, shape := range []string{"empty", "member"} {
			model := `<xs:all minOccurs="0" maxOccurs="0"/>`
			if shape == "member" {
				model = `<xs:all minOccurs="0" maxOccurs="0"><xs:element name="local" type="xs:string" minOccurs="0" maxOccurs="0"/></xs:all>`
			}
			root := negativeAllAttributeSchema(model, "", ` notNamespace="##local" notQName="" processContents="skip"`)
			root = strings.Replace(root, `<xs:element name="root" type="r:Record"/>`, "", 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s parse: %v", policy, shape, err)
			}
			definition := negativeAllDefinition(t, schema)
			if definition.Particle() != nil || definitionAnyAttribute(t, definition).ProcessContents() != "skip" {
				t.Fatalf("%s/%s omitted all lost wildcard", policy, shape)
			}
		}
	}
}

func TestNegativeAnyAttributeNamedDirectAllConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{`<xs:all/>`, `<xs:all><xs:element name="local" type="xs:string"/></xs:all>`} {
			root := negativeAllAttributeSchema(model, "", ` notNamespace="##local" processContents="lax"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse consumer fixture: %v", err)
			}
			all := directAllFromSchema(t, schema)
			assertDirectAllConsumerRejection(t, schema, all, XSDVersion11)
		}
	}
}

func assertNegativeAllDiagnostic(t *testing.T, root string, schema Schema, err error, class FailureClass, code, marker, spec string, related []Loc, cause error) {
	t.Helper()
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != class || d.Code() != code || d.Loc() != anyAttributeTestLoc(root, marker) ||
		!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != spec || cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic = %s related=%v cause=%v; want %s %s %s %q %v", d, d.Related(), err, class, code, marker, spec, related)
	}
}

func TestNegativeAnyAttributeNamedDirectAllExclusions(t *testing.T) {
	for _, form := range []struct{ name, attributes string }{
		{"negative-qname", ` notNamespace="##local" notQName="r:name"`},
		{"negative-defined", ` notNamespace="##local" notQName="##defined"`},
		{"negative-qname-skip", ` notNamespace="##local" notQName="r:name" processContents="skip"`},
		{"positive", ` namespace="##any"`},
		{"positive-empty-qname", ` namespace="" notQName=""`},
		{"no-negative", ` processContents="skip"`},
	} {
		root := negativeAllAttributeSchema(`<xs:all/>`, "", form.attributes)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertNegativeAllDiagnostic(t, root, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, nil, errSchemaAnyAttributeUnsupported)
	}
	for _, shape := range []struct {
		name, marker, spec string
		cause              error
	}{
		{"no-model", `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
		{"inline-all", `<xs:all`, "xsd10-structures#schema-document", ErrUnsupported},
		{"group-ref", `<xs:all`, "xsd11-structures#cSchemaDocument", ErrUnsupported},
		{"extension-all", `<xs:anyAttribute`, schemaComplexTypeExtensionSpecRef(XSDVersion11), errSchemaAnyAttributeUnsupported},
		{"restriction-all", `<xs:all`, "xsd11-structures#cSchemaDocument", ErrUnsupported},
	} {
		for _, qname := range []struct{ name, attribute string }{
			{"absent", ""},
			{"empty", ` notQName=""`},
			{"explicit", ` notQName="r:name"`},
			{"symbolic", ` notQName="##defined"`},
		} {
			t.Run(shape.name+"/"+qname.name, func(t *testing.T) {
				wildcard := `<xs:anyAttribute notNamespace="##local"` + qname.attribute + ` processContents="skip"/>`
				body := `<xs:complexType name="Record">` + wildcard + `</xs:complexType>`
				switch shape.name {
				case "inline-all":
					body = `<xs:element name="root"><xs:complexType><xs:all/>` + wildcard + `</xs:complexType></xs:element>`
				case "group-ref":
					body = `<xs:group name="G"><xs:all/></xs:group><xs:complexType name="Record"><xs:group ref="r:G"/>` + wildcard + `</xs:complexType>`
				case "extension-all":
					body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:all/>` + wildcard + `</xs:extension></xs:complexContent></xs:complexType>`
				case "restriction-all":
					body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:restriction base="r:Base"><xs:all/>` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all">` + body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				marker, cause := shape.marker, shape.cause
				if shape.name == "extension-all" && (qname.name == "absent" || qname.name == "explicit") {
					marker, cause = `<xs:extension`, ErrUnsupported
				}
				assertNegativeAllDiagnostic(t, root, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, marker, shape.spec, nil, cause)
			})
		}
	}
}

func TestNegativeAnyAttributeNamedDirectAllStrict10Policy(t *testing.T) {
	for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`, ` processContents="skip"`} {
		for _, qname := range []string{"", ` notQName=""`, ` notQName="&#x9;&#xA;"`} {
			root := negativeAllAttributeSchema(`<xs:all><xs:element name="item" type="xs:string"/></xs:all>`, "", ` notNamespace="##local"`+mode+qname)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertNegativeAllDiagnostic(t, root, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, `notNamespace=`, "xsd11-structures#cSchemaDocument", nil, errLanguagePolicyMismatch)
		}
	}
}

func TestNegativeAnyAttributeNamedDirectAllEditionAndMemberRanges(t *testing.T) {
	for _, edition := range []string{"1.0", "1.1"} {
		root := negativeAllAttributeSchema(`<xs:all minOccurs="0"><xs:element name="repeat" type="xs:string" minOccurs="0" maxOccurs="3"/><xs:element name="omitted" type="xs:string" minOccurs="0" maxOccurs="0"/><xs:element ref="r:forward"/></xs:all>`, "", ` notNamespace="##targetNamespace" notQName="&#x9;" processContents="skip"`)
		root = strings.Replace(root, `targetNamespace="urn:all"`, `targetNamespace="urn:all" version="`+edition+`"`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
		if err != nil {
			t.Fatalf("Compatibility edition %s: %v", edition, err)
		}
		definition := negativeAllDefinition(t, schema)
		all, ok := definition.Particle().(AllParticle)
		if !ok || all.Occurrences().String() != "0/1" || len(all.Members()) != 2 ||
			all.Members()[0].Occurrences().String() != "0/3" || all.Members()[1].Occurrences().String() != "1/1" ||
			!reflect.DeepEqual(definitionAnyAttribute(t, definition).NamespaceConstraint().Namespaces(), []string{"urn:all"}) {
			t.Fatalf("edition %s all/wildcard = %#v/%#v", edition, definition.Particle(), definitionAnyAttribute(t, definition))
		}
	}
	root := negativeAllAttributeSchema(`<xs:all><xs:element name="repeat" type="xs:string" maxOccurs="3"/></xs:all>`, "", ` notNamespace="##local"`)
	root = strings.Replace(root, `targetNamespace="urn:all"`, `targetNamespace="urn:all" version="1.0"`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureUnsupported || d.Code() != diagnosticSchemaAllOccurrenceVersionCode ||
		d.Loc() != anyAttributeTestLoc(root, `maxOccurs="3"`) || len(d.Related()) != 0 ||
		d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("Strict10 member range = %s related=%v cause=%v", d, d.Related(), err)
	}
}

func TestNegativeAnyAttributeNamedDirectAllBoundaryPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, model, uses, attributes, extra, primary, related, code, spec string
		primaryIndex, relatedIndex                                         int
		class                                                              FailureClass
		cause                                                              error
	}{
		{"missing-ref-before-omission", `<xs:all minOccurs="0" maxOccurs="0"><xs:element ref="r:missing"/></xs:all>`, "", ` notNamespace="##local"`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, "xsd11-structures#src-resolve", 1, 0, FailureInvalid, errSchemaElementReferenceUnresolved},
		{"wrong-kind-ref", `<xs:all><xs:element ref="r:Wrong"/></xs:all>`, "", ` notNamespace="##local"`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:string"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, "xsd11-structures#src-resolve", 1, 1, FailureInvalid, errSchemaElementReferenceWrongKind},
		{"duplicate-member", `<xs:all><xs:element name="same" type="xs:string"/><xs:element name="same" type="xs:string"/></xs:all>`, "", ` notNamespace="##local"`, "", `<xs:element name="same"`, `<xs:element name="same"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(XSDVersion11), 2, 1, FailureInvalid, errSchemaAllMemberDuplicate},
		{"missing-use-before-omission", `<xs:all minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="a" type="r:Missing"/>`, ` notNamespace="##local"`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), 1, 0, FailureInvalid, errSchemaAttributeTypeUnresolved},
		{"duplicate-use", `<xs:all/>`, `<xs:attribute name="a" type="xs:ID"/><xs:attribute name="a" type="xs:ID"/>`, ` notNamespace="##local"`, "", `<xs:attribute name="a"`, `<xs:attribute name="a"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), 2, 1, FailureInvalid, errSchemaAttributeUseDuplicate},
		{"malformed-negative", `<xs:all/>`, "", ` notNamespace="##bad"`, "", `notNamespace=`, "", invalidSchemaCompositionCode, "", 1, 0, FailureInvalid, nil},
		{"malformed-process", `<xs:all/>`, "", ` notNamespace="##local" processContents="bad"`, "", `processContents=`, "", invalidSchemaCompositionCode, "", 1, 0, FailureInvalid, nil},
		{"conflicting-namespace", `<xs:all/>`, "", ` notNamespace="##local" namespace="##any"`, "", `namespace=`, "", invalidSchemaCompositionCode, "", 1, 0, FailureInvalid, nil},
		{"invalid-membership", `<xs:all/>`, "", ` notNamespace="##targetNamespace" notQName="r:name"`, "", `notQName=`, `notNamespace=`, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, 1, 1, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"invalid-outer-bound", `<xs:all maxOccurs="2"/>`, "", ` notNamespace="##local"`, "", `maxOccurs="2"`, "", invalidSchemaCompositionCode, schemaAllLimitedSpecRef(XSDVersion11), 1, 0, FailureInvalid, nil},
		{"unsupported-use-type", `<xs:all/>`, `<xs:attribute name="a" type="xs:string"/>`, ` notNamespace="##local"`, "", `type="xs:string"`, "", UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(XSDVersion11), 1, 0, FailureUnsupported, errSchemaAttributeUseUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := negativeAllAttributeSchema(tc.model, tc.uses, tc.attributes)
			root = strings.Replace(root, `</xs:schema>`, tc.extra+`</xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			primary := allParticleTestTokenLoc(t, "root.xsd", root, tc.primary, tc.primaryIndex)
			var related []Loc
			if tc.related != "" {
				related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, tc.related, tc.relatedIndex)}
			}
			if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != primary ||
				!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != tc.spec || tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("%s diagnostic = %s related=%v cause=%v", tc.name, d, d.Related(), err)
			}
		})
	}
}

//nolint:gocognit // One graph checks adopted and imported namespaces, cycle handling, and ordered public facts.
func TestNegativeAnyAttributeNamedDirectAllGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root"><xs:complexType name="Included"><xs:all minOccurs="0"><xs:element ref="r:forward"/></xs:all><xs:anyAttribute notNamespace="urn:z ##targetNamespace ##local" notQName="" processContents="lax"/></xs:complexType><xs:element name="forward" type="xs:string"/></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:all><xs:element ref="o:forward"/></xs:all><xs:anyAttribute notNamespace="##targetNamespace urn:z" notQName="&#x9;&#xA;" processContents="skip"/></xs:complexType><xs:element name="forward" type="xs:string"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph %s run %d: %v", policy, run, err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("document discovery order = %v", documents)
			}
			components := schema.Components()
			if len(components) != 4 {
				t.Fatalf("components = %v", components)
			}
			for i, want := range []struct {
				name, source, content, namespaceMarker, qnameMarker, processMarker string
				namespaces                                                         []string
			}{
				{"Included", "included.xsd", included, `notNamespace="urn:z ##targetNamespace ##local"`, `notQName=""`, `processContents="lax"`, []string{"", "urn:root", "urn:z"}},
				{"Imported", "other.xsd", other, `notNamespace="##targetNamespace urn:z"`, `notQName="&#x9;&#xA;"`, `processContents="skip"`, []string{"urn:other", "urn:z"}},
			} {
				index := i * 2
				component := components[index]
				namespace := "urn:root"
				if i != 0 {
					namespace = "urn:other"
				}
				if component.Name() != mustTestQName(t, namespace, want.name) || component.ID().Source() != SourceID(want.source) {
					t.Fatalf("component %d = %v", index, component)
				}
				definition, ok := component.ComplexType()
				if !ok {
					t.Fatalf("component %d is not complex type", index)
				}
				wildcard := definitionAnyAttribute(t, definition)
				if wildcard.Loc() != anyAttributeTestLocForSource(SourceID(want.source), want.content, `<xs:anyAttribute`) ||
					wildcard.NamespaceLoc() != anyAttributeTestLocForSource(SourceID(want.source), want.content, want.namespaceMarker) ||
					wildcard.QNameConstraint().Loc() != anyAttributeTestLocForSource(SourceID(want.source), want.content, want.qnameMarker) ||
					wildcard.ProcessContentsLoc() != anyAttributeTestLocForSource(SourceID(want.source), want.content, want.processMarker) ||
					!reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), want.namespaces) {
					t.Fatalf("component %d wildcard = %#v", index, wildcard)
				}
				all, ok := definition.Particle().(AllParticle)
				if !ok || len(all.Members()) != 1 {
					t.Fatalf("component %d all = %#v", index, definition.Particle())
				}
				ref, ok := all.Members()[0].(ElementReferenceParticle)
				if !ok || ref.TargetID() != components[index+1].ID() {
					t.Fatalf("component %d reference = %#v", index, all.Members()[0])
				}
			}
			walked := make([]ComponentID, 0, len(components))
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			for index, component := range components {
				if walked[index] != component.ID() {
					t.Fatalf("walk %d = %v, want %v", index, walked[index], component.ID())
				}
			}
			if run == 0 {
				previous = components
				continue
			}
			if !reflect.DeepEqual(previous, components) {
				t.Fatal("repeated graph parse changed public facts")
			}
		}
	}
}

func TestNegativeAnyAttributeNamedDirectAllInvisibleReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:import namespace="urn:b" schemaLocation="b.xsd"/></xs:schema>`
	a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:a"><xs:complexType name="Record"><xs:all><xs:element ref="b:target"/></xs:all><xs:anyAttribute notNamespace="##local"/></xs:complexType></xs:schema>`
	b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="target" type="xs:string"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"a.xsd": {id: "a.xsd", contents: a}, "b.xsd": {id: "b.xsd", contents: b}}, Strict11)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceNamespaceCode ||
		d.Loc() != allParticleTestTokenLoc(t, "a.xsd", a, `ref="b:target"`, 1) ||
		!reflect.DeepEqual(d.Related(), []Loc{allParticleTestTokenLoc(t, "b.xsd", b, `<xs:element name="target"`, 1)}) ||
		d.SpecRef() != "xsd11-structures#composition-importLicenseReferences" || !errors.Is(err, errSchemaElementReferenceNamespace) {
		t.Fatalf("invisible member reference = %s related=%v spec=%q cause=%v", d, d.Related(), d.SpecRef(), err)
	}
}

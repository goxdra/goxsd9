package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func inlineDirectGroupSchema(version, use, target string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" targetNamespace="urn:inline-direct-group" version="` + version + `">
<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + use + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>
<xs:group name="Target">` + target + `</xs:group><xs:element name="a" type="xs:integer"/><xs:element name="b" type="xs:integer"/>
</xs:schema>`
}

func TestInlineDirectGroupPublicFacts(t *testing.T) { //nolint:gocognit,funlen // Public copies, order, provenance, and consumers share one fixture.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, use, bounds string }{
			{"default", `<xs:group ref="t:Target"/>`, "1/1"},
			{"unbounded", `<xs:group ref="t:Target" minOccurs="0" maxOccurs="unbounded"/>`, "0/unbounded"},
			{"finite", `<xs:group ref="t:Target" minOccurs="2" maxOccurs="18446744073709551616"/>`, "2/18446744073709551616"},
			{"zero", `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`, ""},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, test.use, `<xs:choice><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:choice>`)
				var first []Component
				for repeat := 0; repeat < 2; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if repeat == 0 {
						first = schema.Components()
					}
					if !reflect.DeepEqual(first, schema.Components()) || len(first) != 5 {
						t.Fatalf("component order = %v", schema.Components())
					}
					var walked []ComponentID
					if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
					for index, component := range first {
						if walked[index] != component.ID() {
							t.Fatalf("walk = %v", walked)
						}
					}
					inline := inlineExtensionChoiceOwner(t, schema)
					if !inline.ID().IsZero() || !inline.Name().IsZero() || inline.Derivation() != ComplexTypeDerivationExtension || inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType><xs:complexContent>`) || inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
						t.Fatalf("inline owner = %v", inline)
					}
					base, ok := inline.BaseReference()
					baseID, hasID := base.ComponentID()
					if !ok || !hasID || baseID != first[0].ID() || base.Loc() != inline.BaseLoc() {
						t.Fatalf("base reference = %v", base)
					}
					wildcard, ok := inline.AnyAttribute()
					if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
						t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
					}
					outer, ok := inline.Particle().(SequenceParticle)
					if !ok || outer.Loc() != complexContentTestLoc(t, root, `<xs:sequence><xs:group`) || outer.Occurrences().String() != "1/1" {
						t.Fatalf("outer = %v", inline.Particle())
					}
					terms := outer.Particles()
					if test.name == "zero" {
						if len(terms) != 0 {
							t.Fatalf("zero group survived = %v", terms)
						}
					}
					if test.name != "zero" {
						if len(terms) != 1 {
							t.Fatalf("direct group terms = %v", terms)
						}
						group, ok := terms[0].(ModelGroupReferenceParticle)
						if !ok || group.Ref() != mustTestQName(t, "urn:inline-direct-group", "Target") || group.TargetID() != first[2].ID() || group.Occurrences().String() != test.bounds || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Target"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Target"`) {
							t.Fatalf("direct group = %v", terms)
						}
						terms[0] = nil
						if outer.Particles()[0] == nil {
							t.Fatal("particle slice was mutable")
						}
						if test.name == "finite" {
							maximum, finite := group.Occurrences().Maximum().Finite()
							if !finite {
								t.Fatal("finite maximum became unbounded")
							}
							maximum.value.SetInt64(0)
							if group.Occurrences().String() != test.bounds {
								t.Fatal("occurrence maximum was mutable")
							}
						}
					}
					if repeat == 0 {
						validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:inline-direct-group"/>`)))
						validation := requireDiagnostic(t, validationErr)
						if validation.Code() != UnsupportedInstanceValidationCode || validation.Class() != FailureUnsupported || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
							t.Fatalf("validation = %v", validationErr)
						}
						output, generationErr := GenerateGo(schema, "generated")
						generation := requireDiagnostic(t, generationErr)
						if output != nil || generation.Code() != diagnosticCodegenUnsupported || generation.Class() != FailureUnsupported || generation.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Base"`) {
							t.Fatalf("generation = %q/%v", output, generationErr)
						}
					}
				}
			})
		}
	}
}

func TestInlineDirectGroupBoundaryExits(t *testing.T) { //nolint:gocognit // Invalid, resolution, proof, and unsupported exits share the direct use boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, use, target, extra, marker, code, spec string
			class                                        FailureClass
			cause                                        error
			related                                      []string
		}{
			{"missing under zero", `<xs:group ref="t:Missing" minOccurs="0" maxOccurs="0"/>`, `<xs:sequence/>`, "", `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"wrong kind under zero", `<xs:group ref="t:a" minOccurs="0" maxOccurs="0"/>`, `<xs:sequence/>`, "", `ref="t:a"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="a"`}},
			{"malformed min", `<xs:group ref="t:Target" minOccurs="bad"/>`, `<xs:sequence/>`, "", `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
			{"reversed bounds", `<xs:group ref="t:Target" minOccurs="2" maxOccurs="1"/>`, `<xs:sequence/>`, "", `<xs:group ref="t:Target"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"missing ref", `<xs:group minOccurs="0" maxOccurs="0"/>`, `<xs:sequence/>`, "", `<xs:group minOccurs`, invalidSchemaCompositionCode, "", FailureInvalid, nil, nil},
			{"choice duplicate", `<xs:group ref="t:Target"/>`, `<xs:choice><xs:element ref="t:a"/><xs:element ref="t:a"/></xs:choice>`, "", `ref="t:a"`, diagnosticSchemaElementReferenceDuplicateCode, schemaElementReferenceDuplicateSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceDuplicate, []string{`ref="t:a"`}},
			{"unproved repeated name", `<xs:group ref="t:Target"/>`, `<xs:sequence><xs:group ref="t:G1"/><xs:group ref="t:G2"/></xs:sequence>`, `<xs:group name="G1"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group><xs:group name="G2"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group>`, `ref="t:Target"`, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, []string{`ref="t:G1"`, `ref="t:G2"`}},
			{"choice overlap", `<xs:group ref="t:Target"/>`, `<xs:choice><xs:group ref="t:G1"/><xs:group ref="t:G2"/></xs:choice>`, `<xs:group name="G1"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group><xs:group name="G2"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group>`, `ref="t:G2"`, "XSD3064", schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureInvalid, errSchemaInlineDirectGroupAmbiguous, []string{`ref="t:G1"`, `ref="t:Target"`}},
			{"inconsistent declarations", `<xs:group ref="t:Target"/>`, `<xs:sequence><xs:element name="a" form="qualified" type="xs:string"/><xs:group ref="t:G1"/></xs:sequence>`, `<xs:group name="G1"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group>`, `ref="t:G1"`, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), FailureInvalid, errSchemaGroupElementConsistency, []string{`<xs:element name="a" form="qualified"`}},
			{"repeated optional tail", `<xs:group ref="t:Target" maxOccurs="2"/>`, `<xs:sequence><xs:element ref="t:a"/><xs:element ref="t:a" minOccurs="0"/></xs:sequence>`, "", `ref="t:a" minOccurs="0"`, diagnosticSchemaElementReferenceDuplicateCode, schemaElementReferenceDuplicateSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceDuplicate, []string{`ref="t:a"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, test.use, test.target)
				root = strings.Replace(root, `</xs:group><xs:element name="a" type="xs:integer"`, `</xs:group>`+test.extra+`<xs:element name="a" type="xs:integer"`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := complexContentTestLoc(t, root, test.marker)
				if test.name == "choice duplicate" || test.name == "repeated optional tail" {
					loc = namedGroupLastLoc(t, root, test.marker)
				}
				var related []Loc
				for _, marker := range test.related {
					related = append(related, complexContentTestLoc(t, root, marker))
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || (test.cause != nil && !errors.Is(err, test.cause)) {
					t.Fatalf("boundary exit = %v related=%v; want %s %s at %s related=%v", err, d.Related(), test.class, test.code, loc, related)
				}
				if test.name == "malformed min" && errors.Unwrap(d) == nil {
					t.Fatal("malformed bound lost cause")
				}
			})
		}
	}
}

func TestInlineDirectGroupExcludedShapes(t *testing.T) {
	group := `<xs:group ref="t:Target"/>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, owner, marker string }{
			{"named extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, `ref="t:Target"`},
			{"named direct", `<xs:complexType name="Derived"><xs:sequence>` + group + `</xs:sequence></xs:complexType>`, `ref="t:Target"`},
			{"inline direct", `<xs:element name="root"><xs:complexType><xs:sequence>` + group + `</xs:sequence></xs:complexType></xs:element>`, `ref="t:Target"`},
			{"local inline owner", `<xs:complexType name="Container"><xs:sequence><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:sequence></xs:complexType>`, `ref="t:Target"`},
			{"sibling element ref", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `<xs:element ref="t:a"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:element ref="t:a"`},
			{"sibling local declaration", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `<xs:element name="local" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:element name="local"`},
			{"sibling wildcard", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `<xs:any/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:any`},
			{"second group", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + group + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:group ref="t:Target"`},
			{"nested group", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:sequence>` + group + `</xs:sequence></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:sequence><xs:group`},
			{"all model", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:all><xs:element ref="t:a"/></xs:all></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:extension base="t:Base"`},
			{"attribute", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `</xs:sequence><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `ref="t:Target"`},
			{"attribute group", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `</xs:sequence><xs:attributeGroup ref="t:Attrs"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:attributeGroup name="Attrs"><xs:attribute name="flag" type="xs:boolean"/></xs:attributeGroup>`, `<xs:attributeGroup ref="t:Attrs"`},
			{"direct anyAttribute", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + group + `</xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:anyAttribute`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" targetNamespace="urn:inline-direct-group" version="` + profile.version + `"><xs:complexType name="Base"/>` + test.owner + `<xs:group name="Target"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group><xs:element name="a" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := complexContentTestLoc(t, root, test.marker)
				if test.name == "second group" {
					loc = namedGroupLastLoc(t, root, test.marker)
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != loc || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded %s = %v", test.name, err)
				}
			})
		}
	}
}

func TestInlineDirectGroupGraphVisibility(t *testing.T) { //nolint:gocognit // Check each graph edge against one public target view.
	owner := `<xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:group ref="t:Target"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`
	target := `<xs:group name="Target"><xs:sequence/></xs:group>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, root, child, source, namespace, marker string
		}{
			{"include", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" targetNamespace="urn:inline-direct-group"><xs:include schemaLocation="child.xsd"/><xs:include schemaLocation="child.xsd"/>` + owner + `</xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-direct-group">` + target + `</xs:schema>`, "child.xsd", "urn:inline-direct-group", `ref="t:Target"`},
			{"import", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" xmlns:o="urn:other" targetNamespace="urn:inline-direct-group"><xs:import namespace="urn:other" schemaLocation="child.xsd"/>` + strings.Replace(owner, `ref="t:Target"`, `ref="o:Target"`, 1) + `</xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + target + `</xs:schema>`, "child.xsd", "urn:other", `ref="o:Target"`},
			{"chameleon", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" targetNamespace="urn:inline-direct-group"><xs:include schemaLocation="child.xsd"/>` + owner + `</xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + target + `</xs:schema>`, "child.xsd", "urn:inline-direct-group", `ref="t:Target"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := strings.Replace(test.root, `<xs:schema `, `<xs:schema version="`+profile.version+`" `, 1)
				fixtures := map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: test.child}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 1 {
					t.Fatalf("graph outer = %v", inlineExtensionChoiceOwner(t, schema).Particle())
				}
				group, ok := outer.Particles()[0].(ModelGroupReferenceParticle)
				targets := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, test.namespace, "Target"))
				if !ok || len(targets) != 1 || group.TargetID() != targets[0].ID() || group.TargetID().Source() != SourceID(test.source) || group.Ref() != targets[0].Name() || group.RefLoc() != complexContentTestLoc(t, root, test.marker) {
					t.Fatalf("graph target = %v/%v", group, targets)
				}
			})
		}
	}
}

func TestInlineDirectGroupCycleBeforeZeroOmission(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`, `<xs:sequence><xs:group ref="t:Target"/></xs:sequence>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceCycleCode || d.Loc() != namedGroupLastLoc(t, root, `ref="t:Target"`) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="Target"`)}) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
				t.Fatalf("cycle = %v related=%v", err, d.Related())
			}
		})
	}
}

func TestInlineDirectGroupInvisibleTarget(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, withAttribute := range []bool{false, true} {
			name := profile.name + "/plain"
			if withAttribute {
				name = profile.name + "/attribute"
			}
			t.Run(name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" xmlns:o="urn:other" targetNamespace="urn:inline-direct-group" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:group ref="o:Target" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:schema>`
				if withAttribute {
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence><xs:attribute name="flag" type="xs:boolean"/></xs:extension>`, 1)
				}
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`
				fixtures := map[string]discoveryFixture{
					"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-direct-group"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
					"other.xsd": {id: "other.xsd", contents: other},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{complexContentTestSourceLoc(t, "other.xsd", other, `<xs:group name="Target"`)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || d.Loc() != complexContentTestLoc(t, root, `ref="o:Target"`) || d.SpecRef() != schemaModelGroupReferenceImportSpecRef(profile.factVersion) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
					t.Fatalf("invisible target = %v related=%v", err, d.Related())
				}
			})
		}
	}
}

func TestInlineDirectGroupNonemptyBaseExcludedAfterReference(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`, `<xs:sequence/>`)
			root = strings.Replace(root, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:complexType>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), complexContentTestLoc(t, root, `<xs:sequence><xs:element ref="t:a"`)}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `base="t:Base"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.factVersion) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) {
				t.Fatalf("nonempty base = %v related=%v", err, d.Related())
			}
		})
	}
}

func TestInlineDirectGroupChoiceSharedSuffix(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			target := `<xs:choice><xs:group ref="t:G1"/><xs:group ref="t:G2"/></xs:choice>`
			root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, target)
			root = strings.Replace(root, `</xs:schema>`, `<xs:group name="G1"><xs:sequence><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:sequence></xs:group><xs:group name="G2"><xs:sequence><xs:element ref="t:c"/><xs:element ref="t:b"/></xs:sequence></xs:group><xs:element name="c" type="xs:integer"/></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := inlineExtensionChoiceOwner(t, schema)
			outer, ok := owner.Particle().(SequenceParticle)
			if !ok || len(outer.Particles()) != 1 {
				t.Fatalf("outer = %v", owner.Particle())
			}
			group, ok := outer.Particles()[0].(ModelGroupReferenceParticle)
			if !ok || group.Ref() != mustTestQName(t, "urn:inline-direct-group", "Target") || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Target"`) {
				t.Fatalf("group = %v", outer.Particles()[0])
			}
			if len(schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:inline-direct-group", "Target"))) != 1 {
				t.Fatal("named groups lost from public schema")
			}
		})
	}
}

func TestInlineDirectGroupChoiceSharedFirstInvalid(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, `<xs:choice><xs:group ref="t:G1"/><xs:group ref="t:G2"/></xs:choice>`)
			root = strings.Replace(root, `</xs:schema>`, `<xs:group name="G1"><xs:sequence><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:sequence></xs:group><xs:group name="G2"><xs:sequence><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:sequence></xs:group></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			wantRelated := []Loc{complexContentTestLoc(t, root, `ref="t:G1"`), complexContentTestLoc(t, root, `ref="t:Target"`)}
			if d.Class() != FailureInvalid || d.Code() != "XSD3064" || d.Loc() != complexContentTestLoc(t, root, `ref="t:G2"`) || d.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaInlineDirectGroupAmbiguous) {
				t.Fatalf("shared first = %v related=%v", err, d.Related())
			}
		})
	}
}

func TestInlineDirectGroupSharedReferenceDAGBounded(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, `<xs:sequence><xs:group ref="t:G0"/></xs:sequence>`)
			var definitions strings.Builder
			for index := 0; index < 40; index++ {
				_, err := io.WriteString(&definitions, `<xs:group name="G`+strconv.Itoa(index)+`"><xs:sequence><xs:group ref="t:G`+strconv.Itoa(index+1)+`"/><xs:group ref="t:G`+strconv.Itoa(index+1)+`"/></xs:sequence></xs:group>`)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := io.WriteString(&definitions, `<xs:group name="G40"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group>`)
			if err != nil {
				t.Fatal(err)
			}
			root = strings.Replace(root, `</xs:schema>`, definitions.String()+`</xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `ref="t:Target"`) || d.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || len(d.Related()) != 2 || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("shared DAG = %v related=%v", err, d.Related())
			}
		})
	}
}

func TestInlineDirectGroupAttributeReferencePrecedence(t *testing.T) { //nolint:gocognit // Each excluded-use exit proves the reference gate before refusal.
	attribute := `<xs:attribute name="flag" type="xs:boolean"/>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, use, marker, code, spec string
			class                         FailureClass
			cause                         error
			related                       []string
		}{
			{"valid", `<xs:group ref="t:Target"/>`, `ref="t:Target"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, []string{attribute}},
			{"valid zero", `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`, `ref="t:Target"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, []string{attribute}},
			{"missing", `<xs:group ref="t:Missing"/>`, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"missing zero", `<xs:group ref="t:Missing" minOccurs="0" maxOccurs="0"/>`, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"wrong kind", `<xs:group ref="t:a"/>`, `ref="t:a"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="a"`}},
			{"wrong kind zero", `<xs:group ref="t:a" minOccurs="0" maxOccurs="0"/>`, `ref="t:a"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="a"`}},
			{"malformed occurrence", `<xs:group ref="t:Target" minOccurs="bad"/>`, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
			{"reversed occurrence", `<xs:group ref="t:Target" minOccurs="2" maxOccurs="1"/>`, `<xs:group ref="t:Target"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, test.use, `<xs:sequence/>`)
				root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence>`+attribute+`</xs:extension>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var wantRelated []Loc
				for _, marker := range test.related {
					wantRelated = append(wantRelated, complexContentTestLoc(t, root, marker))
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), wantRelated) || (test.cause != nil && !errors.Is(err, test.cause)) {
					t.Fatalf("attribute reference gate = %v related=%v; want %s %s at %s related=%v", err, d.Related(), test.class, test.code, test.marker, wantRelated)
				}
				if test.name == "malformed occurrence" && errors.Unwrap(d) == nil {
					t.Fatal("malformed occurrence lost cause")
				}
			})
		}
	}
}

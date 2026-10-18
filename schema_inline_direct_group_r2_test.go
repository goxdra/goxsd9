package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestInlineDirectGroupOuterSequenceRepetitionProof(t *testing.T) { //nolint:gocognit // Both repetition bounds share ambiguous and singleton public controls.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, maximum := range []string{"2", "unbounded"} {
			t.Run(profile.name+"/ambiguous/"+maximum, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, `<xs:choice><xs:group ref="t:A"/><xs:group ref="t:B"/></xs:choice>`)
				root = strings.Replace(root, `<xs:sequence><xs:group ref="t:Target"/>`, `<xs:sequence maxOccurs="`+maximum+`"><xs:group ref="t:Target"/>`, 1)
				root = strings.Replace(root, `</xs:schema>`, `<xs:group name="A"><xs:sequence><xs:element ref="t:a"/></xs:sequence></xs:group><xs:group name="B"><xs:sequence><xs:element ref="t:b"/><xs:element ref="t:a" minOccurs="0"/></xs:sequence></xs:group></xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `ref="t:Target"`) || d.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:sequence maxOccurs=`)}) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("repeated outer sequence = %v related=%v", err, d.Related())
				}
			})
			t.Run(profile.name+"/singleton/"+maximum, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, `<xs:choice><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:choice>`)
				root = strings.Replace(root, `<xs:sequence><xs:group ref="t:Target"/>`, `<xs:sequence maxOccurs="`+maximum+`"><xs:group ref="t:Target"/>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
				if !ok || outer.Occurrences().String() != "1/"+maximum || len(outer.Particles()) != 1 {
					t.Fatalf("safe outer sequence = %v", outer)
				}
			})
		}
	}
}

func TestInlineDirectGroupSharedSubstitutionMember(t *testing.T) { //nolint:gocognit // Edition-specific shared, transitive, and disjoint sets share one diagnostic assertion.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, member, marker string
			invalid              bool
		}{
			{"disjoint", `<xs:element name="c" type="xs:integer" substitutionGroup="t:a"/>`, "", false},
			{"blocked", `<xs:element name="c" type="xs:integer" substitutionGroup="t:a t:b"/>`, "", false},
			{"shared", `<xs:element name="c" type="xs:integer" substitutionGroup="t:a t:b"/>`, `<xs:element name="c"`, true},
			{"transitive", `<xs:element name="c" type="xs:integer" substitutionGroup="t:b"/>`, `<xs:element name="c"`, true},
		} {
			if profile.policy != Strict10 && test.name == "transitive" {
				continue
			}
			if profile.policy == Strict10 && test.name == "blocked" {
				continue
			}
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineDirectGroupSchema(profile.version, `<xs:group ref="t:Target"/>`, `<xs:choice><xs:element ref="t:a"/><xs:element ref="t:b"/></xs:choice>`)
				if test.name == "transitive" {
					root = strings.Replace(root, `<xs:element name="b" type="xs:integer"/>`, `<xs:element name="b" type="xs:integer" abstract="true" substitutionGroup="t:a"/>`, 1)
				}
				if test.name == "blocked" {
					root = strings.Replace(root, `<xs:element name="a" type="xs:integer"/>`, `<xs:element name="a" type="xs:integer" block="substitution"/>`, 1)
				}
				root = strings.Replace(root, `</xs:schema>`, test.member+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 && test.name == "shared" {
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `substitutionGroup="t:a t:b"`) || d.SpecRef() != schemaSubstitutionAffiliationSpecRef(XSDVersion10) || len(d.Related()) != 0 || !errors.Is(err, errSchemaSubstitutionCardinality) {
						t.Fatalf("Strict10 two-head affiliation = %v related=%v", err, d.Related())
					}
					return
				}
				if !test.invalid {
					if err != nil || len(schema.Components()) == 0 {
						t.Fatalf("disjoint substitution sets = %v / %v", schema, err)
					}
					return
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{
					complexContentTestLoc(t, root, `ref="t:a"`),
					complexContentTestLoc(t, root, `ref="t:Target"`),
					complexContentTestLoc(t, root, test.marker),
				}
				if d.Class() != FailureInvalid || d.Code() != "XSD3064" || d.Loc() != complexContentTestLoc(t, root, `ref="t:b"`) || d.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaInlineDirectGroupAmbiguous) {
					t.Fatalf("shared substitution member = %v related=%v", err, d.Related())
				}
			})
		}
	}
}

func TestInlineDirectGroupSubstitutionDAGBounded(t *testing.T) {
	root := inlineDirectGroupSchema("1.1", `<xs:group ref="t:Target"/>`, `<xs:choice><xs:element ref="t:X"/><xs:element ref="t:Y"/></xs:choice>`)
	var declarations strings.Builder
	if _, err := io.WriteString(&declarations, `<xs:element name="X" type="xs:integer"/><xs:element name="Y" type="xs:integer"/><xs:element name="A0" type="xs:integer" abstract="true"/><xs:element name="B0" type="xs:integer" abstract="true"/>`); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 14; index++ {
		current := strconv.Itoa(index)
		previous := strconv.Itoa(index - 1)
		for _, prefix := range []string{"A", "B"} {
			if _, err := io.WriteString(&declarations, `<xs:element name="`+prefix+current+`" type="xs:integer" abstract="true" substitutionGroup="t:A`+previous+` t:B`+previous+`"/>`); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := io.WriteString(&declarations, `<xs:element name="C" type="xs:integer" substitutionGroup="t:A14 t:B14"/>`); err != nil {
		t.Fatal(err)
	}
	root = strings.Replace(root, `</xs:schema>`, declarations.String()+`</xs:schema>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `ref="t:Target"`) || d.SpecRef() != schemaGroupNonAmbiguousSpecRef(XSDVersion11) || len(d.Related()) != 1 || d.Related()[0].IsZero() || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("bounded substitution DAG = %v related=%v", err, d.Related())
	}
}

func TestInlineDirectGroupAttributeUseResolutionBeforeExclusion(t *testing.T) { //nolint:gocognit // Each attribute-use exit is asserted at the same excluded owner boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, zero := range []bool{false, true} {
			use := `<xs:group ref="t:Target"/>`
			boundName := "default"
			if zero {
				use = `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`
				boundName = "zero"
			}
			for _, test := range []struct {
				name, attributes, marker, code, spec string
				class                                FailureClass
				cause                                error
				related                              []string
			}{
				{"valid", `<xs:attribute name="flag" type="xs:boolean"/>`, `ref="t:Target"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, []string{`<xs:attribute name="flag"`}},
				{"missing ref", `<xs:attribute ref="t:Missing"/>`, `ref="t:Missing"`, diagnosticSchemaAttributeReferenceUnresolvedCode, schemaAttributeUseResolveSpecRef(profile.factVersion), FailureInvalid, errSchemaAttributeReferenceUnresolved, nil},
				{"wrong kind ref", `<xs:attribute ref="t:a"/>`, `ref="t:a"`, diagnosticSchemaAttributeReferenceWrongKindCode, schemaAttributeUseResolveSpecRef(profile.factVersion), FailureInvalid, errSchemaAttributeReferenceWrongKind, []string{`<xs:element name="a"`}},
				{"missing type", `<xs:attribute name="flag" type="t:Missing"/>`, `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaAttributeTypeUnresolved, nil},
				{"wrong kind type", `<xs:attribute name="flag" type="t:a"/>`, `type="t:a"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaAttributeTypeWrongKind, []string{`<xs:element name="a"`}},
				{"duplicate uses", `<xs:attribute name="flag" type="xs:boolean"/><xs:attribute name="flag" type="xs:boolean" use="required"/>`, `<xs:attribute name="flag" type="xs:boolean" use="required"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(profile.factVersion), FailureInvalid, errSchemaAttributeUseDuplicate, []string{`<xs:attribute name="flag" type="xs:boolean"/>`}},
			} {
				t.Run(profile.name+"/"+boundName+"/"+test.name, func(t *testing.T) {
					root := inlineDirectGroupSchema(profile.version, use, `<xs:sequence/>`)
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence>`+test.attributes+`</xs:extension>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					for _, marker := range test.related {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("attribute-use precedence = %v related=%v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestInlineDirectGroupSiblingReferenceResolutionBeforeExclusion(t *testing.T) { //nolint:gocognit // Each ordered sibling reference exit is asserted under both group bounds.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, zero := range []bool{false, true} {
			bound := ""
			boundName := "default"
			if zero {
				bound = ` minOccurs="0" maxOccurs="0"`
				boundName = "zero"
			}
			group := `<xs:group ref="t:Target"` + bound + `/>`
			for _, test := range []struct {
				name, use, marker, code, spec string
				class                         FailureClass
				cause                         error
				related                       []string
			}{
				{"valid sibling", group + `<xs:element ref="t:a"/>`, `<xs:element ref="t:a"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, nil},
				{"valid sibling first", `<xs:element ref="t:a"/>` + group, `<xs:element ref="t:a"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, nil},
				{"valid second group", group + group, `<xs:group ref="t:Target"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported, nil},
				{"missing group", `<xs:group ref="t:Missing"` + bound + `/><xs:element ref="t:a"/>`, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
				{"wrong kind group", `<xs:group ref="t:a"` + bound + `/><xs:element ref="t:a"/>`, `ref="t:a"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="a"`}},
				{"missing sibling", group + `<xs:element ref="t:Missing"/>`, `ref="t:Missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceUnresolved, nil},
				{"wrong kind sibling", group + `<xs:element ref="t:Target"/>`, `ref="t:Target"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceWrongKind, []string{`<xs:group name="Target"`}},
				{"lexical group first", `<xs:group ref="t:Missing"` + bound + `/><xs:element ref="t:Absent"/>`, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
				{"lexical sibling first", `<xs:element ref="t:Absent"/>` + `<xs:group ref="t:Missing"` + bound + `/>`, `ref="t:Absent"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceUnresolved, nil},
			} {
				t.Run(profile.name+"/"+boundName+"/"+test.name, func(t *testing.T) {
					root := inlineDirectGroupSchema(profile.version, test.use, `<xs:sequence/>`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantLoc := complexContentTestLoc(t, root, test.marker)
					if test.name == "wrong kind sibling" || test.name == "valid second group" {
						wantLoc = namedGroupLastLoc(t, root, test.marker)
					}
					var related []Loc
					for _, marker := range test.related {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("sibling reference precedence = %v related=%v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestInlineDirectGroupExcludedReferenceVisibility(t *testing.T) { //nolint:gocognit // Indirectly discovered targets exercise each excluded reference gate.
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="ForeignGroup"><xs:sequence/></xs:group><xs:element name="foreign" type="xs:integer"/><xs:attribute name="foreignAttr" type="xs:boolean"/><xs:simpleType name="ForeignType"><xs:restriction base="xs:boolean"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-direct-group"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	for _, profile := range nestedGroupChoiceProfiles {
		for _, zero := range []bool{false, true} {
			bound := ""
			boundName := "default"
			if zero {
				bound = ` minOccurs="0" maxOccurs="0"`
				boundName = "zero"
			}
			for _, test := range []struct {
				name, use, attribute, marker, related, code, spec string
				cause                                             error
			}{
				{"group", `<xs:group ref="o:ForeignGroup"` + bound + `/><xs:element ref="t:a"/>`, "", `ref="o:ForeignGroup"`, `<xs:group name="ForeignGroup"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
				{"sibling", `<xs:group ref="t:Target"` + bound + `/><xs:element ref="o:foreign"/>`, "", `ref="o:foreign"`, `<xs:element name="foreign"`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
				{"attribute ref", `<xs:group ref="t:Target"` + bound + `/>`, `<xs:attribute ref="o:foreignAttr"/>`, `ref="o:foreignAttr"`, `<xs:attribute name="foreignAttr"`, diagnosticSchemaAttributeReferenceNamespaceCode, schemaAttributeUseResolveSpecRef(profile.factVersion), errSchemaAttributeReferenceNamespace},
				{"attribute type", `<xs:group ref="t:Target"` + bound + `/>`, `<xs:attribute name="flag" type="o:ForeignType"/>`, `type="o:ForeignType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.factVersion), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+boundName+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-direct-group" xmlns:o="urn:other" targetNamespace="urn:inline-direct-group" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + test.use + `</xs:sequence>` + test.attribute + `</xs:extension></xs:complexContent></xs:complexType></xs:element><xs:group name="Target"><xs:sequence/></xs:group><xs:element name="a" type="xs:integer"/></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestSourceLoc(t, "other.xsd", other, test.related)}
					}
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("excluded visibility = %v related=%v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestInlineDirectGroupExcludedTargetCycleBeforeRefusal(t *testing.T) { //nolint:gocognit // All policies and zero bounds share exact cycle provenance at both excluded shapes.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, zero := range []bool{false, true} {
			group := `<xs:group ref="t:Target"/>`
			boundName := "default"
			if zero {
				group = `<xs:group ref="t:Target" minOccurs="0" maxOccurs="0"/>`
				boundName = "zero"
			}
			for _, test := range []struct{ name, use, attribute string }{
				{"attribute", group, `<xs:attribute name="flag" type="xs:boolean"/>`},
				{"sibling", group + `<xs:element ref="t:a"/>`, ""},
			} {
				t.Run(profile.name+"/"+boundName+"/"+test.name, func(t *testing.T) {
					root := inlineDirectGroupSchema(profile.version, test.use, `<xs:sequence><xs:group ref="t:Target"/></xs:sequence>`)
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence>`+test.attribute+`</xs:extension>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantSpec := "xsd11-structures#mg-props-correct"
					if profile.factVersion == XSDVersion10 {
						wantSpec = "xsd10-structures#mg-props-correct"
					}
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceCycleCode || d.Loc() != namedGroupLastLoc(t, root, `ref="t:Target"`) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="Target"`)}) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
						t.Fatalf("excluded cycle = %v related=%v", err, d.Related())
					}
				})
			}
		}
	}
}

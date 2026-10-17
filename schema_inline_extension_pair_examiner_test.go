package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlinePairExaminerSchema(version, middle, groups, elements, form string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:pair-review" targetNamespace="urn:pair-review" version="` + version + `"` + form + `>
<xs:complexType name="Empty"/>
<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Empty"><xs:sequence>` + middle + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>
` + groups + elements + `</xs:schema>`
}

func parseInlinePairPublic(t *testing.T, root string, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	return ParseSchemaWithPolicy(source, nil, policy)
}

func inlinePairNthLoc(t *testing.T, root, marker string, occurrence int) Loc {
	t.Helper()
	index := -1
	for range occurrence {
		next := strings.Index(root[index+1:], marker)
		if next < 0 {
			t.Fatalf("missing occurrence %d of %q", occurrence, marker)
		}
		index += next + 1
	}
	line, column := 1, 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}

func assertInlinePairPublicFailure(t *testing.T, root string, policy LanguagePolicy, class FailureClass, code, spec string, primary, related Loc, cause error) {
	t.Helper()
	schema, err := parseInlinePairPublic(t, root, policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.SpecRef() != spec || diagnostic.Loc() != primary || !reflect.DeepEqual(diagnostic.Related(), []Loc{related}) || !errors.Is(err, cause) {
		t.Fatalf("failure = %v; primary %v, related %v; want %v, %v", err, diagnostic.Loc(), diagnostic.Related(), primary, related)
	}
}

func TestInlineExtensionPairPublicWithinSiblingConflicts(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, groupG, form, code, spec string
			class                          FailureClass
			cause                          error
		}{
			{"optional group ref", `<xs:element ref="t:x" minOccurs="0"/>`, "", diagnosticSchemaInlineExtensionPairAmbiguousCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureInvalid, errSchemaInlineExtensionPairAmbiguous},
			{"inconsistent local type", `<xs:element name="x" type="xs:token"/>`, ` elementFormDefault="qualified"`, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), FailureInvalid, errSchemaGroupElementConsistency},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				middle := `<xs:sequence><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence><xs:sequence><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
				groups := `<xs:group name="G"><xs:sequence>` + test.groupG + `</xs:sequence></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
				root := inlinePairExaminerSchema(profile.version, middle, groups, `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/>`, test.form)
				assertInlinePairPublicFailure(t, root, profile.policy, test.class, test.code, test.spec, inlinePairNthLoc(t, root, `ref="t:x"`, 1), inlinePairNthLoc(t, root, `ref="t:G"`, 1), test.cause)
			})
		}
	}
}

func TestInlineExtensionPairPublicSharedSubstitutionMember(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		if profile.policy == Strict10 {
			continue
		}
		for _, test := range []struct{ name, extras string }{
			{"direct", `<xs:element name="member" type="xs:integer" substitutionGroup="t:h1 t:h2"/>`},
			{"transitive", `<xs:element name="via1" type="xs:integer" substitutionGroup="t:h1"/><xs:element name="via2" type="xs:integer" substitutionGroup="t:h2"/><xs:element name="member" type="xs:integer" substitutionGroup="t:via1 t:via2"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				middle := `<xs:sequence minOccurs="0"><xs:group ref="t:G"/><xs:element ref="t:h1"/></xs:sequence><xs:sequence minOccurs="0"><xs:group ref="t:H"/><xs:element ref="t:h2"/></xs:sequence>`
				groups := `<xs:group name="G"><xs:sequence/></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
				elements := `<xs:element name="h1" type="xs:integer"/><xs:element name="h2" type="xs:integer"/>` + test.extras
				root := inlinePairExaminerSchema(profile.version, middle, groups, elements, "")
				assertInlinePairPublicFailure(t, root, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), inlinePairNthLoc(t, root, `ref="t:h2"`, 1), inlinePairNthLoc(t, root, `ref="t:h1"`, 1), ErrUnsupported)
			})
		}
	}
}

func TestInlineExtensionPairPublicSeparatedAndAdjacentRepeat(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, separated := range []bool{false, true} {
			name := "adjacent"
			middle := `<xs:sequence minOccurs="0"><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence>`
			if separated {
				name = "separated"
				middle += `<xs:element ref="t:y"/>`
			}
			middle += `<xs:sequence minOccurs="0"><xs:group ref="t:H"/><xs:element ref="t:x"/></xs:sequence>`
			t.Run(profile.name+"/"+name, func(t *testing.T) {
				groups := `<xs:group name="G"><xs:sequence/></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
				root := inlinePairExaminerSchema(profile.version, middle, groups, `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/>`, "")
				class, code, cause := FailureInvalid, diagnosticSchemaInlineExtensionPairAmbiguousCode, errSchemaInlineExtensionPairAmbiguous
				if separated {
					class, code, cause = FailureUnsupported, UnsupportedSchemaSyntaxCode, ErrUnsupported
				}
				assertInlinePairPublicFailure(t, root, profile.policy, class, code, schemaGroupNonAmbiguousSpecRef(profile.factVersion), inlinePairNthLoc(t, root, `ref="t:x"`, 2), inlinePairNthLoc(t, root, `ref="t:x"`, 1), cause)
			})
		}
	}
}

func TestInlineExtensionPairPublicOmittedTrailingReference(t *testing.T) { //nolint:gocognit // Both sibling positions and public resolved terms share one bounded assertion.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, omitted := range []string{"x", "y"} {
			t.Run(profile.name+"/"+omitted, func(t *testing.T) {
				first := `<xs:sequence><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence>`
				second := `<xs:sequence><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
				if omitted == "x" {
					first = strings.Replace(first, `<xs:element ref="t:x"/>`, `<xs:element ref="t:x" minOccurs="0" maxOccurs="0"/>`, 1)
				}
				if omitted == "y" {
					second = strings.Replace(second, `<xs:element ref="t:y"/>`, `<xs:element ref="t:y" minOccurs="0" maxOccurs="0"/>`, 1)
				}
				groups := `<xs:group name="G"><xs:sequence/></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
				root := inlinePairExaminerSchema(profile.version, first+second, groups, `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/>`, "")
				schema, err := parseInlinePairPublic(t, root, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 2 {
					t.Fatalf("outer terms = %T/%v", outer, outer.Particles())
				}
				for index, term := range outer.Particles() {
					nested, nestedOK := term.(SequenceParticle)
					if !nestedOK {
						t.Fatalf("nested %d = %T", index, term)
					}
					wantCount := 2
					wantGroup := "G"
					if index == 1 {
						wantGroup = "H"
					}
					if index == 0 && omitted == "x" || index == 1 && omitted == "y" {
						wantCount = 1
					}
					parts := nested.Particles()
					if len(parts) != wantCount {
						t.Fatalf("nested %d = %v", index, parts)
					}
					group, groupOK := parts[0].(ModelGroupReferenceParticle)
					if !groupOK || group.RefLoc() != inlinePairNthLoc(t, root, `ref="t:`+wantGroup+`"`, 1) || group.Occurrences().String() != "1/1" {
						t.Fatalf("nested %d = %v", index, parts)
					}
				}
			})
		}
	}
}

func TestInlineExtensionPairPublicOmittedSiblingKeepsProof(t *testing.T) { //nolint:gocognit // Omission position and both diagnostic classes form one public matrix.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, omitted := range []string{"first", "second"} {
			for _, conflict := range []string{"ambiguity", "consistency"} {
				t.Run(profile.name+"/"+omitted+"/"+conflict, func(t *testing.T) {
					first := `<xs:sequence><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence>`
					second := `<xs:sequence><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
					groupG, groupH := "", ""
					liveGroup := "G"
					if omitted == "first" {
						first = `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:G"/><xs:element ref="t:y"/></xs:sequence>`
						second = `<xs:sequence><xs:group ref="t:H"/><xs:element ref="t:x"/></xs:sequence>`
						liveGroup = "H"
					}
					if omitted == "second" {
						second = `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
					}
					form := ""
					member := `<xs:element ref="t:x" minOccurs="0"/>`
					class, code, spec, cause := FailureInvalid, diagnosticSchemaInlineExtensionPairAmbiguousCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), errSchemaInlineExtensionPairAmbiguous
					if conflict == "consistency" {
						member = `<xs:element name="x" type="xs:token"/>`
						form = ` elementFormDefault="qualified"`
						class, code, spec, cause = FailureInvalid, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), errSchemaGroupElementConsistency
					}
					if liveGroup == "G" {
						groupG = member
					}
					if liveGroup == "H" {
						groupH = member
					}
					groups := `<xs:group name="G"><xs:sequence>` + groupG + `</xs:sequence></xs:group><xs:group name="H"><xs:sequence>` + groupH + `</xs:sequence></xs:group>`
					root := inlinePairExaminerSchema(profile.version, first+second, groups, `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/>`, form)
					assertInlinePairPublicFailure(t, root, profile.policy, class, code, spec, inlinePairNthLoc(t, root, `ref="t:x"`, 1), inlinePairNthLoc(t, root, `ref="t:`+liveGroup+`"`, 1), cause)
				})
			}
		}
	}
}

func TestInlineExtensionPairPublicGroupInternalSubstitution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, trailingOmitted := range []bool{false, true} {
			name := profile.name + "/trailing-live"
			if trailingOmitted {
				name = profile.name + "/trailing-omitted"
			}
			t.Run(name, func(t *testing.T) {
				middle := `<xs:sequence><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence><xs:sequence><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
				if trailingOmitted {
					middle = strings.Replace(middle, `<xs:element ref="t:x"/>`, `<xs:element ref="t:x" minOccurs="0" maxOccurs="0"/>`, 1)
				}
				groups := `<xs:group name="G"><xs:sequence><xs:element ref="t:a" minOccurs="0"/><xs:element ref="t:b"/></xs:sequence></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
				elements := `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/><xs:element name="a" type="xs:integer"/><xs:element name="b" type="xs:integer" substitutionGroup="t:a"/>`
				root := inlinePairExaminerSchema(profile.version, middle, groups, elements, "")
				schema, err := parseInlinePairPublic(t, root, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				related := []Loc{inlinePairNthLoc(t, root, `ref="t:a"`, 1), inlinePairNthLoc(t, root, `ref="t:G"`, 1)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaInlineExtensionPairAmbiguousCode || diagnostic.Loc() != inlinePairNthLoc(t, root, `ref="t:b"`, 1) || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !errors.Is(err, errSchemaInlineExtensionPairAmbiguous) {
					t.Fatalf("group overlap = %v; related %v", err, diagnostic.Related())
				}
			})
		}
	}
}

func TestInlineExtensionPairPublicAbstractHeadOverlapIsUnproved(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			middle := `<xs:sequence minOccurs="0"><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence><xs:sequence minOccurs="0"><xs:group ref="t:H"/><xs:element ref="t:x"/></xs:sequence>`
			groups := `<xs:group name="G"><xs:sequence/></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
			root := inlinePairExaminerSchema(profile.version, middle, groups, `<xs:element name="x" type="xs:integer" abstract="true"/>`, "")
			assertInlinePairPublicFailure(t, root, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), inlinePairNthLoc(t, root, `ref="t:x"`, 2), inlinePairNthLoc(t, root, `ref="t:x"`, 1), ErrUnsupported)
		})
	}
}

func TestInlineExtensionPairPublicOmittedOuterHasNoUPA(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			middle := `<xs:sequence><xs:group ref="t:G"/><xs:element ref="t:x"/></xs:sequence><xs:sequence><xs:group ref="t:H"/><xs:element ref="t:y"/></xs:sequence>`
			groups := `<xs:group name="G"><xs:sequence><xs:element ref="t:a" minOccurs="0"/><xs:element ref="t:b"/></xs:sequence></xs:group><xs:group name="H"><xs:sequence/></xs:group>`
			elements := `<xs:element name="x" type="xs:integer"/><xs:element name="y" type="xs:integer"/><xs:element name="a" type="xs:integer"/><xs:element name="b" type="xs:integer" substitutionGroup="t:a"/>`
			root := inlinePairExaminerSchema(profile.version, middle, groups, elements, "")
			root = strings.Replace(root, `<xs:extension base="t:Empty"><xs:sequence>`, `<xs:extension base="t:Empty"><xs:sequence minOccurs="0" maxOccurs="0">`, 1)
			schema, err := parseInlinePairPublic(t, root, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := inlineExtensionChoiceOwner(t, schema)
			if owner.Particle() != nil || len(schema.Components()) != 8 {
				t.Fatalf("omitted content = %v; components %d", owner.Particle(), len(schema.Components()))
			}
		})
	}
}

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

package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const inheritedRestrictionWildcard = `<xs:anyAttribute namespace="##other" processContents="lax"/>`
const inheritedRestrictionOptionalGroup = `<xs:group ref="t:Derivation" minOccurs="0" maxOccurs="0"/>`
const inheritedRestrictionPresentGroup = `<xs:group ref="t:Derivation"/>`
const inheritedRestrictionOwnUse = `<xs:attribute name="own" type="xs:boolean"/>`
const inheritedRestrictionProhibitedUse = `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`

const inheritedRestrictionExaminerXML = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:t="urn:owner" targetNamespace="urn:owner">
<xs:complexType name="Open"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
<xs:complexType name="Annotated"><xs:complexContent><xs:extension base="t:Open"><xs:sequence><xs:element ref="t:annotation" minOccurs="0"/></xs:sequence><xs:attribute name="id" type="xs:ID"/></xs:extension></xs:complexContent></xs:complexType>
<xs:group name="Derivation"><xs:choice><xs:element ref="t:annotation"/></xs:choice></xs:group>
<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Annotated"><xs:group ref="t:Derivation" minOccurs="0" maxOccurs="0"/></xs:extension></xs:complexContent></xs:complexType>
<xs:complexType name="Restricted"><xs:complexContent><xs:restriction base="t:Derived"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
<xs:element name="annotation" type="xs:string"/>
</xs:schema>`

func TestInheritedUsesRestrictionExaminerXML(t *testing.T) {
	for attempt := range 2 {
		schema, err := discoverTestSchemaWithPolicy(t, inheritedRestrictionExaminerXML, nil, Compatibility)
		if err == nil {
			t.Fatalf("attempt %d published the examiner restriction leak", attempt)
		}
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantRelated := []Loc{
			complexContentTestLoc(t, inheritedRestrictionExaminerXML, `<xs:complexType name="Derived"`),
			complexContentTestLoc(t, inheritedRestrictionExaminerXML, `<xs:sequence><xs:element ref="t:annotation" minOccurs="0"`),
			complexContentTestLoc(t, inheritedRestrictionExaminerXML, `<xs:attribute name="id" type="xs:ID"`),
		}
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
			diagnostic.Loc() != complexContentTestLoc(t, inheritedRestrictionExaminerXML, `base="t:Derived"`) ||
			!reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) ||
			!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) {
			t.Fatalf("attempt %d examiner restriction diagnostic = %v, related %v", attempt, err, diagnostic.Related())
		}
	}
}

func inheritedRestrictionExtension(base, group, uses string) string {
	return `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:` + base + `">` + group + uses + `</xs:extension></xs:complexContent></xs:complexType>`
}

func inheritedRestrictionOwnerSource(base string, inline bool) string {
	content := `<xs:complexContent><xs:restriction base="t:` + base + `">` + inheritedRestrictionWildcard + `</xs:restriction></xs:complexContent>`
	if inline {
		return `<xs:element name="root"><xs:complexType>` + content + `</xs:complexType></xs:element>`
	}
	return `<xs:complexType name="Restricted">` + content + `</xs:complexType>`
}

type inheritedRestrictionCase struct {
	name, derived, base string
	useFree, supported  bool
	related             []string
}

func inheritedRestrictionCases() []inheritedRestrictionCase {
	sequence := `<xs:sequence><xs:element ref="t:annotation" minOccurs="0"/>`
	id := `<xs:attribute name="id" type="xs:ID"/>`
	derived := `<xs:complexType name="Derived"`
	annotated := `<xs:complexType name="Annotated"`
	group := `<xs:group ref="t:Derivation"`
	return []inheritedRestrictionCase{
		{"direct annotated", "", "Annotated", false, false, []string{annotated, sequence, id}},
		{"omitted group no own use", inheritedRestrictionExtension("Annotated", inheritedRestrictionOptionalGroup, ""), "Derived", false, false, []string{derived, sequence, id}},
		{"omitted group prohibited use", inheritedRestrictionExtension("Annotated", inheritedRestrictionOptionalGroup, inheritedRestrictionProhibitedUse), "Derived", false, false, []string{derived, sequence, id}},
		{"omitted group authored use", inheritedRestrictionExtension("Annotated", inheritedRestrictionOptionalGroup, inheritedRestrictionOwnUse), "Derived", false, false, []string{derived, sequence, id, inheritedRestrictionOwnUse}},
		{"present group no own use", inheritedRestrictionExtension("Annotated", inheritedRestrictionPresentGroup, ""), "Derived", false, false, []string{derived, sequence, id, group}},
		{"present group authored use", inheritedRestrictionExtension("Annotated", inheritedRestrictionPresentGroup, inheritedRestrictionOwnUse), "Derived", false, false, []string{derived, sequence, id, group, inheritedRestrictionOwnUse}},
		{"use-free optional sequence", inheritedRestrictionExtension("Annotated", inheritedRestrictionOptionalGroup, ""), "Derived", true, false, []string{derived, sequence}},
		{"empty wildcard extension", inheritedRestrictionExtension("Open", "", ""), "Derived", false, true, nil},
	}
}

//nolint:gocognit // The public matrix checks restriction admission across owner, policy, and inherited shape.
func TestInheritedUsesRestrictionAdmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, inline := range []bool{false, true} {
			owner := "named"
			if inline {
				owner = "inline"
			}
			for _, probe := range inheritedRestrictionCases() {
				t.Run(string(policy)+"/"+owner+"/"+probe.name, func(t *testing.T) {
					annotated := syntheticInheritedAnnotated
					if probe.useFree {
						annotated = strings.Replace(annotated, inheritedRestrictionIDUse(), "", 1)
					}
					declarations := syntheticInheritedOpen + annotated + syntheticInheritedGroup + probe.derived +
						inheritedRestrictionOwnerSource(probe.base, inline) + syntheticInheritedTargets
					source := syntheticInheritedSchema("urn:owner", "", declarations)
					for attempt := range 2 {
						schema, err := discoverTestSchemaWithPolicy(t, source, nil, policy)
						if probe.supported {
							assertInheritedEmptyRestriction(t, schema, err, source, inline)
							continue
						}
						if err == nil {
							t.Fatalf("attempt %d published restriction over inherited content", attempt)
						}
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						primary := complexContentTestLoc(t, source, `base="t:`+probe.base+`"`)
						related := make([]Loc, 0, len(probe.related))
						for _, marker := range probe.related {
							related = append(related, complexContentTestLoc(t, source, marker))
						}
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != primary ||
							!reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(version) ||
							!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) {
							t.Fatalf("attempt %d restriction diagnostic = %v, related %v", attempt, err, diagnostic.Related())
						}
					}
				})
			}
		}
	}
}

func inheritedRestrictionIDUse() string {
	return `<xs:attribute name="id" type="xs:ID"/>`
}

func TestInheritedUsesRestrictionOmissionResolvesDependencies(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, inline := range []bool{false, true} {
			owner := "named"
			if inline {
				owner = "inline"
			}
			t.Run(string(policy)+"/"+owner, func(t *testing.T) {
				assertInheritedRestrictionOmission(t, policy, version, inline)
			})
		}
	}
}

func assertInheritedRestrictionOmission(t *testing.T, policy LanguagePolicy, version XSDVersion, inline bool) {
	t.Helper()
	missing := strings.Replace(inheritedRestrictionOptionalGroup, `ref="t:Derivation"`, `ref="t:Missing"`, 1)
	declarations := syntheticInheritedOpen + syntheticInheritedAnnotated + syntheticInheritedGroup +
		inheritedRestrictionExtension("Annotated", missing, inheritedRestrictionProhibitedUse) +
		inheritedRestrictionOwnerSource("Derived", inline) + syntheticInheritedTargets
	source := syntheticInheritedSchema("urn:owner", "", declarations)
	for attempt := range 2 {
		schema, err := discoverTestSchemaWithPolicy(t, source, nil, policy)
		if err == nil {
			t.Fatalf("attempt %d omitted unresolved group published", attempt)
		}
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceUnresolvedCode ||
			diagnostic.Loc() != complexContentTestLoc(t, source, `ref="t:Missing"`) || len(diagnostic.Related()) != 0 ||
			diagnostic.SpecRef() != schemaModelGroupReferenceSpecRef(version) || !errors.Is(err, errSchemaModelGroupReferenceUnresolved) || errors.Is(err, ErrUnsupported) {
			t.Fatalf("attempt %d dependency diagnostic = %v, related %v", attempt, err, diagnostic.Related())
		}
	}
}

func assertInheritedEmptyRestriction(t *testing.T, schema Schema, err error, source string, inline bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("empty wildcard restriction rejected: %v", err)
	}
	definition := inheritedEmptyRestrictionDefinition(t, schema, inline)
	reference, ok := definition.BaseReference()
	if !ok || reference.Name().Local() != "Derived" || reference.Loc() != complexContentTestLoc(t, source, `base="t:Derived"`) || definition.Particle() != nil || len(definition.AttributeUses()) != 0 {
		t.Fatalf("empty restriction facts = %#v/%t", definition, ok)
	}
	wildcard, present := definition.AnyAttribute()
	if !present || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != namedRestrictionLastLoc(t, source, inheritedRestrictionWildcard) {
		t.Fatalf("empty restriction wildcard = %#v/%t", wildcard, present)
	}
}

func inheritedEmptyRestrictionDefinition(t *testing.T, schema Schema, inline bool) ComplexTypeDefinition {
	t.Helper()
	if inline {
		found := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:owner", "root"))
		if len(found) != 1 {
			t.Fatalf("inline root count = %d", len(found))
		}
		declaration, ok := found[0].ElementDeclaration()
		if !ok {
			t.Fatal("inline root has no declaration")
		}
		definition, ok := declaration.InlineComplexType()
		if !ok {
			t.Fatal("inline root has no complex type")
		}
		return definition
	}
	found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:owner", "Restricted"))
	if len(found) != 1 {
		t.Fatalf("named restriction count = %d", len(found))
	}
	definition, ok := found[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("named restriction has no complex type")
	}
	return definition
}

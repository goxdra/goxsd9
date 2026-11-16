package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func parseReferenceChoicePublic(t *testing.T, source string, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return ParseSchemaWithPolicy(root, nil, policy)
}

func referenceChoiceRoot(version, owner, choice, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:test" targetNamespace="urn:test" elementFormDefault="qualified" version="` + version + `">
  ` + owner + `
  <xs:group name="Types"><xs:sequence><xs:element name="a" type="xs:integer"/><xs:element name="b" type="xs:integer"/></xs:sequence></xs:group>
  <xs:group name="G"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
  <xs:group name="Bad"><xs:choice>` + choice + `</xs:choice></xs:group>
  ` + declarations + `
</xs:schema>`
}

func TestReferenceOnlyNamedGroupChoiceSubstitutionUPA(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []struct {
			name, owner string
		}{
			{"top-level ref", `<xs:complexType name="Owner"><xs:group ref="g:Bad"/></xs:complexType>`},
			{"unused group", `<xs:complexType name="Owner"/>`},
			{"zero-use ref", `<xs:complexType name="Owner"><xs:group ref="g:Bad" minOccurs="0" maxOccurs="0"/></xs:complexType>`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				choice := `<xs:element ref="g:h"/><xs:element ref="g:m"/>`
				declarations := `<xs:element name="h" type="xs:integer" abstract="true"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`
				root := referenceChoiceRoot(profile.version, shape.owner, choice, declarations)
				schema, err := parseReferenceChoicePublic(t, root, profile.policy)
				assertReferenceChoiceUPA(t, schema, err, root, `ref="g:m"`, []string{`ref="g:h"/></xs:choice></xs:group>`, `<xs:element name="h" type="xs:integer" abstract="true"`, `<xs:element name="m" type="xs:integer"`, `substitutionGroup="g:h"`}, profile.factVersion)
			})
		}
	}
}

func TestReferenceOnlyNamedGroupChoiceTransitiveAndSharedMember(t *testing.T) { //nolint:gocognit // Each policy checks the same public witness and positive control.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, choice, declarations, later string
			related                           []string
		}{
			{"transitive", `<xs:element ref="g:h"/><xs:element ref="g:m"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`, `ref="g:m"`, []string{`ref="g:h"/></xs:choice></xs:group>`, `<xs:element name="h" type="xs:integer"`, `<xs:element name="m" type="xs:integer"`, `substitutionGroup="g:mid"`, `substitutionGroup="g:h"`}},
			{"separate members", `<xs:element ref="g:h"/><xs:element ref="g:k"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="k" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/><xs:element name="n" type="xs:integer" substitutionGroup="g:k"/>`, `ref="g:k"`, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := referenceChoiceRoot(profile.version, ``, test.choice, test.declarations)
				schema, err := parseReferenceChoicePublic(t, root, profile.policy)
				if test.related == nil {
					if err != nil {
						t.Fatal(err)
					}
					if len(schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:test", "Bad"))) != 1 {
						t.Fatal("missing group")
					}
					return
				}
				assertReferenceChoiceUPA(t, schema, err, root, test.later, test.related, profile.factVersion)
			})
		}
	}
}

func TestReferenceOnlyNamedGroupChoiceSharedMemberStrict11(t *testing.T) {
	root := referenceChoiceRoot("1.1", ``, `<xs:element ref="g:h"/><xs:element ref="g:k"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="k" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h g:k"/>`)
	schema, err := parseReferenceChoicePublic(t, root, Strict11)
	assertReferenceChoiceUPA(t, schema, err, root, `ref="g:k"`, []string{`ref="g:h"`, `<xs:element name="h" type="xs:integer"`, `<xs:element name="k" type="xs:integer"`, `<xs:element name="m" type="xs:integer"`, `substitutionGroup="g:h g:k"`}, XSDVersion11)
}

func TestReferenceOnlyNamedGroupChoiceDiagnosticOrder(t *testing.T) {
	root := referenceChoiceRoot("1.1", ``, `<xs:element ref="g:h"/><xs:element ref="g:m"/><xs:element ref="g:n"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="n" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
	for repeat := 0; repeat < 2; repeat++ {
		schema, err := parseReferenceChoicePublic(t, root, Strict11)
		assertReferenceChoiceUPA(t, schema, err, root, `ref="g:m"`, []string{`ref="g:h"`, `<xs:element name="h" type="xs:integer"`, `<xs:element name="m" type="xs:integer"`, `substitutionGroup="g:h"#2`}, XSDVersion11)
	}
}

func TestReferenceOnlyNamedGroupNestedReferenceChoiceUPA(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := referenceChoiceRoot(profile.version, ``, `<xs:element ref="g:h"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
			root = strings.Replace(root, `<xs:group name="Bad"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>`, `<xs:group name="Bad"><xs:sequence><xs:choice><xs:element ref="g:h"/><xs:element ref="g:m"/></xs:choice></xs:sequence></xs:group>`, 1)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			assertReferenceChoiceUPA(t, schema, err, root, `ref="g:m"`, []string{`ref="g:h"`, `<xs:element name="h" type="xs:integer"`, `<xs:element name="m" type="xs:integer"`, `substitutionGroup="g:h"`}, profile.factVersion)
		})
	}
}

func TestReferenceOnlyNamedGroupChoiceBlockedDerivationNeedsProof(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := referenceChoiceRoot(profile.version, ``, `<xs:element ref="g:h"/><xs:element ref="g:m"/>`, `<xs:simpleType name="Narrow"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="h" type="xs:integer" block="restriction"/><xs:element name="m" type="g:Narrow" substitutionGroup="g:h"/>`)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{referenceChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `<xs:element name="m" type="g:Narrow"`)}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:m"`) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaGroupReferenceChoiceDerivationProof) {
				t.Fatalf("blocked derivation diagnostic = %s related %v cause %v", diagnostic, diagnostic.Related(), err)
			}
		})
	}
}

func TestReferenceOnlyNamedGroupChoiceIncludedGraphLocation(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, graph := range []struct{ name, rootNamespace, edge, childNamespace string }{
			{"include", "urn:test", `<xs:include schemaLocation="child.xsd"/>`, ` targetNamespace="urn:test"`},
			{"import", "urn:root", `<xs:import namespace="urn:test" schemaLocation="child.xsd"/>`, ` targetNamespace="urn:test"`},
			{"chameleon", "urn:test", `<xs:include schemaLocation="child.xsd"/>`, ``},
		} {
			t.Run(profile.name+"/"+graph.name, func(t *testing.T) {
				rootText := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:test" targetNamespace="` + graph.rootNamespace + `" version="` + profile.version + `">` + graph.edge + `<xs:complexType name="Owner"><xs:group ref="g:Bad"/></xs:complexType></xs:schema>`
				child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:test"` + graph.childNamespace + `><xs:group name="Bad"><xs:choice><xs:element ref="g:h"/><xs:element ref="g:m"/></xs:choice></xs:group><xs:element name="h" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/></xs:schema>`
				root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(rootText)})
				if err != nil {
					t.Fatal(err)
				}
				schema, err := ParseSchemaWithPolicy(root, &discoveryResolver{fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}}}, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{
					allParticleTestTokenLoc(t, "child.xsd", child, `ref="g:h"`, 1),
					allParticleTestTokenLoc(t, "child.xsd", child, `<xs:element name="h"`, 1),
					allParticleTestTokenLoc(t, "child.xsd", child, `<xs:element name="m"`, 1),
					allParticleTestTokenLoc(t, "child.xsd", child, `substitutionGroup="g:h"`, 1),
				}
				if diagnostic.Code() != diagnosticSchemaGroupReferenceChoiceUPACode || diagnostic.Class() != FailureInvalid || diagnostic.Loc() != allParticleTestTokenLoc(t, "child.xsd", child, `ref="g:m"`, 1) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.factVersion) || !errors.Is(err, errSchemaGroupReferenceChoiceUPA) {
					t.Fatalf("included group diagnostic = %s related %v cause %v", diagnostic, diagnostic.Related(), err)
				}
			})
		}
	}
}

func TestReferenceOnlyNamedGroupChoiceEarlierReferenceExits(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name+"/duplicate", func(t *testing.T) {
			root := referenceChoiceRoot(profile.version, ``, `<xs:element ref="g:h"/><xs:element ref="g:h"/>`, `<xs:element name="h" type="xs:integer"/>`)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			assertNamedGroupLocalFailure(t, schema, err, diagnosticSchemaElementReferenceDuplicateCode, FailureInvalid, allParticleTestTokenLoc(t, "root.xsd", root, `ref="g:h"`, 3), []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `ref="g:h"`, 2)}, schemaElementReferenceDuplicateSpecRef(profile.factVersion), errSchemaElementReferenceDuplicate)
		})
		t.Run(profile.name+"/unresolved", func(t *testing.T) {
			root := referenceChoiceRoot(profile.version, ``, `<xs:element ref="g:h"/><xs:element ref="g:missing"/>`, `<xs:element name="h" type="xs:integer"/>`)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			assertNamedGroupLocalFailure(t, schema, err, diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, namedGroupChoiceLoc(t, root, `ref="g:missing"`), nil, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceUnresolved)
		})
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			root := referenceChoiceRoot(profile.version, ``, `<xs:group ref="g:Bad"/>`, `<xs:element name="h" type="xs:integer"/>`)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:Bad"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{namedGroupChoiceLoc(t, root, `<xs:group name="Bad"`)}) || diagnostic.SpecRef() != directGroupCycleSpecRef(profile.factVersion) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
				t.Fatalf("group cycle diagnostic = %s related %v cause %v", diagnostic, diagnostic.Related(), err)
			}
		})
	}
}

func TestReferenceOnlyNamedGroupChoicePositiveControls(t *testing.T) { //nolint:gocognit // Public references verify IDs, QNames, bounds, and locations together.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, choice, declarations string }{
			{"distinct names", `<xs:element ref="g:h" minOccurs="2" maxOccurs="5"/><xs:element ref="g:m"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="m" type="xs:integer"/>`},
			{"blocked substitution", `<xs:element ref="g:h"/><xs:element ref="g:m"/>`, `<xs:element name="h" type="xs:integer" block="substitution"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := referenceChoiceRoot(profile.version, `<xs:complexType name="Owner"><xs:group ref="g:Bad"/></xs:complexType>`, test.choice, test.declarations)
				schema, err := parseReferenceChoicePublic(t, root, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				group := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:test", "Bad"))
				if len(group) != 1 {
					t.Fatal("missing group")
				}
				definition, ok := group[0].ModelGroupDefinition()
				if !ok {
					t.Fatal("missing group definition")
				}
				choice, ok := definition.Particle().(ChoiceParticle)
				if !ok || len(choice.Alternatives()) != 2 {
					t.Fatalf("choice = %T", definition.Particle())
				}
				for index, term := range choice.Alternatives() {
					ref, ok := term.(ElementReferenceParticle)
					if !ok || ref.TargetID().IsZero() || ref.RefLoc() != referenceChoiceLoc(t, root, []string{`ref="g:h"`, `ref="g:m"`}[index]) || ref.Ref() != mustTestQName(t, "urn:test", []string{"h", "m"}[index]) {
						t.Fatalf("reference %d lost identity, QName, or location: %v", index, term)
					}
				}
				if test.name == "distinct names" {
					first, ok := choice.Alternatives()[0].(ElementReferenceParticle)
					if !ok || first.Occurrences().String() != "2/5" {
						t.Fatal("reference bounds changed")
					}
				}
			})
		}
	}
}

func TestReferenceOnlyNamedGroupChoiceSequenceKeepsValidOverlap(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := strings.Replace(referenceChoiceRoot(profile.version, ``, `<xs:element ref="g:h"/><xs:element ref="g:m"/>`, `<xs:element name="h" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`), `<xs:group name="Bad"><xs:choice>`, `<xs:group name="Bad"><xs:sequence>`, 1)
			root = strings.Replace(root, `</xs:choice></xs:group>`+"\n  "+`<xs:element name="h"`, `</xs:sequence></xs:group>`+"\n  "+`<xs:element name="h"`, 1)
			schema, err := parseReferenceChoicePublic(t, root, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:test", "Bad"))) != 1 {
				t.Fatal("missing sequence group")
			}
		})
	}
}

func TestReferenceOnlyNamedGroupChoiceConsumersStayUnsupported(t *testing.T) {
	root := referenceChoiceRoot("1.1", `<xs:element name="root" type="g:Owner"/><xs:complexType name="Owner"><xs:group ref="g:Bad"/></xs:complexType>`, `<xs:element ref="g:h"/><xs:element ref="g:m"/>`, `<xs:element name="h" type="xs:integer" block="substitution"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
	schema, err := parseReferenceChoicePublic(t, root, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	useLoc := namedGroupChoiceLoc(t, root, `ref="g:Bad"`)
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><m>1</m></root>`)))
	validation := requireDiagnostic(t, validationErr)
	if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != useLoc || validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceModelGroupReference) {
		t.Fatalf("validation = %s; %v", validation, validationErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	generation := requireDiagnostic(t, generationErr)
	if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != useLoc || generation.SpecRef() != codegenDirectSequenceSpecReference(XSDVersion11, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
		t.Fatalf("generation = %s; output %q; %v", generation, output, generationErr)
	}
}

func assertReferenceChoiceUPA(t *testing.T, schema Schema, err error, root, primary string, related []string, version XSDVersion) {
	t.Helper()
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantRelated := make([]Loc, 0, len(related))
	for _, marker := range related {
		wantRelated = append(wantRelated, referenceChoiceLoc(t, root, marker))
	}
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGroupReferenceChoiceUPACode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, primary) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(version) || !errors.Is(err, errSchemaGroupReferenceChoiceUPA) {
		t.Fatalf("UPA diagnostic = %s related %v cause %v; want %v", diagnostic, diagnostic.Related(), err, wantRelated)
	}
	if !strings.Contains(diagnostic.Message(), "match") {
		t.Fatalf("UPA diagnostic lost match: %s", diagnostic.Message())
	}
}

func referenceChoiceLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	if strings.HasSuffix(marker, "#2") {
		return allParticleTestTokenLoc(t, "root.xsd", root, strings.TrimSuffix(marker, "#2"), 2)
	}
	if strings.HasPrefix(marker, `ref="g:h"`) {
		return allParticleTestTokenLoc(t, "root.xsd", root, `ref="g:h"`, 2)
	}
	return namedGroupChoiceLoc(t, root, marker)
}

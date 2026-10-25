package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func directGroupProofRoot(version, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" elementFormDefault="qualified" version="` + version + `">` + body + `</xs:schema>`
}

func directGroupProofProfiles() []struct {
	name, version string
	policy        LanguagePolicy
	facts         XSDVersion
} {
	return []struct {
		name, version string
		policy        LanguagePolicy
		facts         XSDVersion
	}{
		{"Compatibility", "1.0", Compatibility, XSDVersion11},
		{"Strict10", "1.0", Strict10, XSDVersion10},
		{"Strict11", "1.1", Strict11, XSDVersion11},
	}
}

func directGroupRepeatTargets() string {
	return `<xs:group name="C"><xs:choice><xs:element ref="g:a"/><xs:group ref="g:S"/></xs:choice></xs:group>
<xs:group name="S"><xs:sequence><xs:element ref="g:b"/><xs:element ref="g:a" minOccurs="0"/></xs:sequence></xs:group>
<xs:element name="a" type="xs:integer"/><xs:element name="b" type="xs:integer"/>`
}

func TestDirectNamedGroupRepeatedBoundaryFailsClosed(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		for _, test := range []struct {
			name, model, primary, related, leaf string
		}{
			{"group finite", `<xs:sequence><xs:group ref="g:C" maxOccurs="2"/></xs:sequence>`, `ref="g:C"`, `<xs:group ref="g:C"`, `ref="g:a"`},
			{"group unbounded", `<xs:sequence><xs:group ref="g:C" maxOccurs="unbounded"/></xs:sequence>`, `ref="g:C"`, `<xs:group ref="g:C"`, `ref="g:a"`},
			{"owner sequence finite", `<xs:sequence maxOccurs="2"><xs:group ref="g:C"/></xs:sequence>`, `<xs:sequence maxOccurs="2"`, `<xs:sequence maxOccurs="2"`, `ref="g:a"`},
			{"owner sequence unbounded", `<xs:sequence maxOccurs="unbounded"><xs:group ref="g:C"/></xs:sequence>`, `<xs:sequence maxOccurs="unbounded"`, `<xs:sequence maxOccurs="unbounded"`, `ref="g:a"`},
			{"owner choice finite", `<xs:choice maxOccurs="2"><xs:group ref="g:C"/></xs:choice>`, `<xs:choice maxOccurs="2"`, `<xs:choice maxOccurs="2"`, `ref="g:a"`},
			{"owner choice unbounded", `<xs:choice maxOccurs="unbounded"><xs:group ref="g:C"/></xs:choice>`, `<xs:choice maxOccurs="unbounded"`, `<xs:choice maxOccurs="unbounded"`, `ref="g:a"`},
			{"nested group finite", `<xs:sequence><xs:group ref="g:Nested"/></xs:sequence>`, `ref="g:Nested"`, `<xs:group ref="g:S" maxOccurs="2"`, `ref="g:b"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				targets := directGroupRepeatTargets()
				if test.name == "nested group finite" {
					targets += `<xs:group name="Nested"><xs:choice><xs:group ref="g:S" maxOccurs="2"/></xs:choice></xs:group>`
				}
				root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner">`+test.model+`</xs:complexType>`+targets)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantPrimary := namedGroupChoiceLoc(t, root, test.primary)
				wantRelated := []Loc{namedGroupChoiceLoc(t, root, test.related), namedGroupChoiceLoc(t, root, test.leaf)}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantPrimary || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.facts) || !errors.Is(err, errSchemaDirectGroupProofUnsupported) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("repeat proof = %v; related %v, want primary %s related %s", err, diagnostic.Related(), wantPrimary, wantRelated)
				}
			})
		}
	}
}

func TestDirectNamedGroupSharedSubstitutionMembersFailUPA(t *testing.T) { //nolint:gocognit // Direct and transitive shared members have distinct edition gates.
	for _, profile := range directGroupProofProfiles() {
		for _, test := range []struct {
			name, middle, member string
		}{
			{"direct", ``, `<xs:element name="m" type="xs:integer" substitutionGroup="g:h1 g:h2"/>`},
			{"transitive", `<xs:element name="mid1" type="xs:integer" substitutionGroup="g:h1"/><xs:element name="mid2" type="xs:integer" substitutionGroup="g:h2"/>`, `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid1 g:mid2"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:h2"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h1"/></xs:choice></xs:group>
<xs:element name="h1" type="xs:integer"/><xs:element name="h2" type="xs:integer"/>`+test.middle+test.member)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if profile.policy == Strict10 {
					marker := `substitutionGroup="g:h1 g:h2"`
					if test.name == "transitive" {
						marker = `substitutionGroup="g:mid1 g:mid2"`
					}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, marker) || diagnostic.SpecRef() != schemaSubstitutionAffiliationXSD10SpecRef || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaSubstitutionCardinality) {
						t.Fatalf("strict10 substitution gate = %v", err)
					}
					return
				}
				wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h1"`), namedGroupChoiceLoc(t, root, `ref="g:h2"`), namedGroupChoiceLoc(t, root, `<xs:element name="m"`)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaDirectGroupUPACode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:h2"`) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaDirectGroupUPA) {
					t.Fatalf("shared member = %v; related %v want %v", err, diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestDirectNamedGroupSubstitutionMemberElementConsistency(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		for _, chain := range []struct {
			name, members string
		}{
			{"direct", `<xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`},
			{"transitive", `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`},
			{"abstract head", `<xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`},
		} {
			t.Run(profile.name+"/"+chain.name, func(t *testing.T) {
				head := `<xs:element name="h" type="xs:integer"/>`
				if chain.name == "abstract head" {
					head = `<xs:element name="h" type="xs:integer" abstract="true"/>`
				}
				root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:sequence><xs:group ref="g:C"/><xs:element name="m" type="xs:string"/></xs:sequence></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>`+head+chain.members)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `<xs:element name="m" type="xs:string"`), namedGroupChoiceLoc(t, root, `<xs:element name="m" type="xs:integer"`)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGroupElementConsistencyCode || diagnostic.Loc() != wantRelated[1] || diagnostic.SpecRef() != schemaGroupElementConsistencySpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaGroupElementConsistency) {
					t.Fatalf("substitution EDC = %v; related %v want %v", err, diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestDirectNamedGroupSubstitutionBlockAndAbstract(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		t.Run(profile.name+"/blocked member", func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer" block="substitution"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
			choice, ok := owner.Particle().(ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 2 {
				t.Fatalf("blocked member owner = %T", owner.Particle())
			}
		})
		t.Run(profile.name+"/abstract head still substitutes", func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer" abstract="true"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `ref="g:m"`), namedGroupChoiceLoc(t, root, `<xs:element name="m" type="xs:integer"`)}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaDirectGroupUPACode || diagnostic.Loc() != wantRelated[1] || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaDirectGroupUPA) {
				t.Fatalf("abstract substitution = %v; related %v", err, diagnostic.Related())
			}
		})
	}
}

func TestDirectNamedGroupDisjointSubstitutionStillAdmits(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:other"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/><xs:element name="other" type="xs:integer"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
			choice, ok := owner.Particle().(ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 2 {
				t.Fatalf("owner model = %T", owner.Particle())
			}
			reference, ok := choice.Alternatives()[0].(ModelGroupReferenceParticle)
			if !ok || reference.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:C"`) || reference.TargetID() != namedSequenceGroup(t, schema, "urn:direct-group", "C").ID() {
				t.Fatalf("disjoint reference = %T", choice.Alternatives()[0])
			}
		})
	}
}

func TestDirectNamedGroupHeadRestrictionExcludesDerivedMember(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:simpleType name="Small"><xs:restriction base="xs:integer"/></xs:simpleType>
<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer" block="restriction"/><xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="g:Small" substitutionGroup="g:mid"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
			choice, ok := owner.Particle().(ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 2 {
				t.Fatalf("blocked transitive member owner = %T", owner.Particle())
			}
		})
	}
}

func TestDirectNamedGroupOriginalHeadSubstitutionProof(t *testing.T) { //nolint:gocognit // Each row isolates a normative block or declaration exit.
	for _, profile := range directGroupProofProfiles() {
		for _, test := range []struct {
			name, model, head, mid, member    string
			code, primary, related, memberLoc string
		}{
			{
				name:   "intermediate substitution block UPA",
				model:  `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`,
				head:   `<xs:element name="h" type="xs:integer"/>`,
				mid:    `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h" block="substitution"/>`,
				member: `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`,
				code:   diagnosticSchemaDirectGroupUPACode, primary: `ref="g:m"`, related: `ref="g:h"`, memberLoc: `<xs:element name="m"`,
			},
			{
				name:   "intermediate substitution block EDC",
				model:  `<xs:sequence><xs:group ref="g:C"/><xs:element name="m" type="xs:string"/></xs:sequence>`,
				head:   `<xs:element name="h" type="xs:integer"/>`,
				mid:    `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h" block="substitution"/>`,
				member: `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`,
				code:   diagnosticSchemaGroupElementConsistencyCode, primary: `<xs:element name="m" type="xs:string"`, related: `ref="g:h"`, memberLoc: `<xs:element name="m" type="xs:integer"`,
			},
			{
				name:   "abstract referenced declaration EDC",
				model:  `<xs:sequence><xs:group ref="g:C"/><xs:element name="h" type="xs:string"/></xs:sequence>`,
				head:   `<xs:element name="h" type="xs:integer" abstract="true"/>`,
				mid:    `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/>`,
				member: `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`,
				code:   diagnosticSchemaGroupElementConsistencyCode, primary: `<xs:element name="h" type="xs:string"`, related: `ref="g:h"`,
			},
			{
				name:   "head restriction same type UPA",
				model:  `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`,
				head:   `<xs:element name="h" type="xs:integer" block="restriction"/>`,
				mid:    `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/>`,
				member: `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`,
				code:   diagnosticSchemaDirectGroupUPACode, primary: `ref="g:m"`, related: `ref="g:h"`, memberLoc: `<xs:element name="m"`,
			},
			{
				name:   "head extension permits restriction UPA",
				model:  `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`,
				head:   `<xs:element name="h" type="xs:integer" block="extension"/>`,
				mid:    `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/>`,
				member: `<xs:simpleType name="Small"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="m" type="g:Small" substitutionGroup="g:mid"/>`,
				code:   diagnosticSchemaDirectGroupUPACode, primary: `ref="g:m"`, related: `ref="g:h"`, memberLoc: `<xs:element name="m"`,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner">`+test.model+`</xs:complexType><xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>`+test.head+test.mid+test.member)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{namedGroupChoiceLoc(t, root, test.related), namedGroupChoiceLoc(t, root, test.primary)}
				if test.memberLoc != "" {
					wantRelated = append(wantRelated, namedGroupChoiceLoc(t, root, test.memberLoc))
				}
				wantCause := errSchemaDirectGroupUPA
				wantRef := schemaGroupNonAmbiguousSpecRef(profile.facts)
				if test.code == diagnosticSchemaGroupElementConsistencyCode {
					wantCause = errSchemaGroupElementConsistency
					wantRef = schemaGroupElementConsistencySpecRef(profile.facts)
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.primary) || diagnostic.SpecRef() != wantRef || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, wantCause) {
					t.Fatalf("substitution proof = %v; related %v want %v", err, diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestDirectNamedGroupSubstitutionBlockBoundaries(t *testing.T) { //nolint:gocognit // Keep the three block-position controls together.
	for _, profile := range directGroupProofProfiles() {
		for _, test := range []struct {
			name, groupHead, headBlock, mid, member, model string
		}{
			{"original head substitution block", "h", "substitution", `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/>`, `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`, `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`},
			{"intermediate substitution block at intermediate use", "mid", "", `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h" block="substitution"/>`, `<xs:element name="m" type="xs:integer" substitutionGroup="g:mid"/>`, `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`},
			{"restriction through two named bases", "h", "restriction", `<xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/>`, `<xs:simpleType name="Small"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="Tiny"><xs:restriction base="g:Small"/></xs:simpleType><xs:element name="m" type="g:Tiny" substitutionGroup="g:mid"/>`, `<xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				head := `<xs:element name="h" type="xs:integer"`
				if test.headBlock != "" {
					head += ` block="` + test.headBlock + `"`
				}
				head += `/>`
				root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner">`+test.model+`</xs:complexType><xs:group name="C"><xs:choice><xs:element ref="g:`+test.groupHead+`"/></xs:choice></xs:group>`+head+test.mid+test.member)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
				choice, ok := owner.Particle().(ChoiceParticle)
				if !ok || len(choice.Alternatives()) != 2 {
					t.Fatalf("blocked substitution owner = %T", owner.Particle())
				}
			})
		}
	}
}

func TestDirectNamedGroupUnknownFullDerivationIsLocated(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		for _, headBlock := range []string{"", ` block="restriction"`} {
			name := "no block"
			if headBlock != "" {
				name = "restriction block"
			}
			t.Run(profile.name+"/"+name, func(t *testing.T) {
				root := directGroupProofRoot(profile.version, `<xs:simpleType name="Small"><xs:restriction base="xs:long"/></xs:simpleType>
<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element ref="g:m"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer"`+headBlock+`/><xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="g:Small" substitutionGroup="g:mid"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `<xs:element name="m"`)}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:C"`) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaDirectGroupProofUnsupported) {
					t.Fatalf("unknown full derivation = %v; related %v", err, diagnostic.Related())
				}
			})
		}
	}
}

func TestDirectNamedGroupAbstractHeadConsistencyPrecedesUnprovedMembership(t *testing.T) {
	for _, profile := range directGroupProofProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:simpleType name="Small"><xs:restriction base="xs:long"/></xs:simpleType>
<xs:complexType name="Owner"><xs:sequence><xs:group ref="g:C"/><xs:element name="h" type="xs:string"/></xs:sequence></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer" abstract="true" block="restriction"/><xs:element name="mid" type="xs:integer" substitutionGroup="g:h"/><xs:element name="m" type="g:Small" substitutionGroup="g:mid"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `<xs:element name="h" type="xs:string"`)}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGroupElementConsistencyCode || diagnostic.Loc() != wantRelated[1] || diagnostic.SpecRef() != schemaGroupElementConsistencySpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaGroupElementConsistency) {
				t.Fatalf("known abstract EDC = %v; related %v", err, diagnostic.Related())
			}
		})
	}
}

func TestDirectNamedGroupAbstractAttributionEditionBoundary(t *testing.T) { //nolint:gocognit // Compare the edition-specific public result in one fixture.
	for _, profile := range directGroupProofProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := directGroupProofRoot(profile.version, `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:C"/><xs:element name="h" type="xs:integer"/></xs:choice></xs:complexType>
<xs:group name="C"><xs:choice><xs:element ref="g:h"/></xs:choice></xs:group>
<xs:element name="h" type="xs:integer" abstract="true"/><xs:element name="m" type="xs:integer" substitutionGroup="g:h"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
				if _, ok := owner.Particle().(ChoiceParticle); !ok {
					t.Fatalf("strict10 abstract choice = %T", owner.Particle())
				}
				return
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{namedGroupChoiceLoc(t, root, `ref="g:h"`), namedGroupChoiceLoc(t, root, `<xs:element name="h" type="xs:integer"/>`)}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantRelated[1] || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(profile.facts) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaDirectGroupProofUnsupported) {
				t.Fatalf("abstract attribution = %v; related %v", err, diagnostic.Related())
			}
		})
	}
}

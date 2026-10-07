package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit // Verify the owned public facts and consumers under each policy.
func TestDirectAllBuiltinNonNegativeIntegerFacts(t *testing.T) {
	for _, profile := range []struct {
		policy                 LanguagePolicy
		version                XSDVersion
		memberRange, wantRange string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0"><xs:element name="first" type="xs:integer"/><xs:element name="count" type="xs:nonNegativeInteger" form="qualified" `+profile.memberRange+`/><xs:element name="last" type="xs:boolean"/></xs:all>`, `<xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("nondeterministic components: %v", err)
			}
			all := directAllFromSchema(t, schema)
			members := all.Members()
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" || len(members) != 3 {
				t.Fatalf("all facts = %s/%s/%d", all.Loc(), all.Occurrences(), len(members))
			}
			for index, want := range []string{"first", "count", "last"} {
				member, hasMember := members[index].(ElementParticle)
				if !hasMember || member.Name().Local() != want {
					t.Fatalf("member order at %d = %#v", index, members[index])
				}
			}
			count, hasCount := members[1].(ElementParticle)
			if !hasCount {
				t.Fatalf("count member = %T", members[1])
			}
			if count.Name() != mustTestQName(t, "urn:all", "count") || count.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="count"`, 1) || count.Occurrences().String() != profile.wantRange || count.DeclaredType() != mustTestQName(t, testXSDNamespace, "nonNegativeInteger") {
				t.Fatalf("count facts = %q/%s/%s/%q", count.Name(), count.Loc(), count.Occurrences(), count.DeclaredType())
			}
			ref, ok := count.TypeReference()
			if !ok || !ref.IsBuiltin() || ref.Name() != count.DeclaredType() || ref.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:nonNegativeInteger"`, 1) || ref.VarietyLoc() != ref.Loc() {
				t.Fatalf("type reference = %#v/%t", ref, ok)
			}
			if id, hasID := count.TypeID(); hasID || !id.IsZero() {
				t.Fatalf("built-in type ID = %v/%t", id, hasID)
			}
			if id, hasID := ref.ComponentID(); hasID || !id.IsZero() {
				t.Fatalf("built-in reference ID = %v/%t", id, hasID)
			}
			bounds, hasBounds := ref.IntegerBounds()
			minimumBound, hasMin := bounds.MinInclusiveFacet()
			if !hasBounds || !hasMin || bounds.Version() != profile.version || minimumBound.Kind() != BoundMinInclusive || minimumBound.Value().Canonical() != "0" || !minimumBound.Loc().IsZero() {
				t.Fatalf("intrinsic bound = %v/%t/%v", minimumBound, hasMin, bounds.Version())
			}
			copyOfMin := minimumBound.Value()
			copyOfMin.value.SetInt64(9)
			copyOfOccurs := count.Occurrences().Minimum()
			copyOfOccurs.value.SetInt64(9)
			members[1] = nil
			copied, hasCopy := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !hasCopy {
				t.Fatal("copied member lost element shape")
			}
			copiedRef, _ := copied.TypeReference()
			copiedBounds, _ := copiedRef.IntegerBounds()
			copiedMin, _ := copiedBounds.MinInclusiveFacet()
			if copied.Occurrences().String() != profile.wantRange || copiedMin.Value().Canonical() != "0" || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatal("caller mutation changed schema facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Check omission and reference shape across each policy.
func TestDirectAllNonNegativeIntegerOmissionAndReference(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="omit" type="xs:nonNegativeInteger" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:nonNegativeInteger"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("zero member omitted incorrectly: %#v", members)
			}
			kept, hasKept := members[0].(ElementParticle)
			if !hasKept || kept.Name().Local() != "keep" {
				t.Fatalf("zero member omitted incorrectly: %#v", members[0])
			}
			root = allParticleTestRoot(`<xs:all><xs:element ref="r:global"/><xs:element name="count" type="xs:nonNegativeInteger"/></xs:all>`, `<xs:element name="global" type="xs:nonNegativeInteger"/><xs:element name="root" type="r:Record"/>`)
			schema, err = discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			all := directAllFromSchema(t, schema)
			members = all.Members()
			ref, ok := members[0].(ElementReferenceParticle)
			if !ok || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
				t.Fatalf("reference shape = %#v", members[0])
			}
			if declaration, ok := members[1].(ElementParticle); !ok || declaration.DeclaredType().Local() != "nonNegativeInteger" {
				t.Fatalf("local shape = %#v", members[1])
			}
			version := XSDVersion11
			if policy == Strict10 {
				version = XSDVersion10
			}
			assertDirectAllConsumerRejection(t, schema, all, version)
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="count" type="xs:nonNegativeInteger"/></xs:all>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		facts, _ := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0].ComplexTypeDefinition()
		if facts.Particle() != nil {
			t.Fatalf("zero all owner published %T", facts.Particle())
		}
	}
}

//nolint:gocognit // Keep each unsupported and invalid exit's diagnostic contract explicit.
func TestDirectAllNonNegativeIntegerExclusionsAndFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		cases := []struct {
			name, member, extra, primary, code, spec string
			cause                                    error
			related                                  []string
		}{
			{"named excluded", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:string"/></xs:simpleType>`, `type="r:Named"`, UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), errSchemaAllMemberScalar, nil},
			{"inline", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported, nil},
			{"bad lexical", `<xs:element name="v" type="xs:nonNegativeInteger" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), nil, nil},
			{"bad range", `<xs:element name="v" type="xs:nonNegativeInteger" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="v"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"duplicate", `<xs:element name="v" type="xs:nonNegativeInteger"/><xs:element name="v" type="xs:nonNegativeInteger"/>`, "", `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), errSchemaAllMemberDuplicate, []string{`<xs:element name="v"`}},
		}
		for _, test := range cases {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("excluded form returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				occurrence := 1
				if test.name == "duplicate" {
					occurrence = 2
				}
				if diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.primary, occurrence) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) || test.name != "inline" && diagnostic.Unwrap() == nil {
					t.Fatalf("diagnostic = %s, want %s at %s with cause %v", diagnostic, test.code, test.primary, test.cause)
				}
				wantClass := FailureInvalid
				if test.name == "named excluded" || test.name == "inline" {
					wantClass = FailureUnsupported
				}
				if diagnostic.Class() != wantClass {
					t.Fatalf("class = %s, want %s", diagnostic.Class(), wantClass)
				}
				var related []Loc
				for _, token := range test.related {
					related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, token, 1))
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
			})
		}
		for _, excluded := range []struct {
			name, member, extra string
		}{
			{"named zero", `<xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`},
			{"inline zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:element>`, ""},
		} {
			t.Run(string(profile.policy)+"/"+excluded.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+excluded.member+`</xs:all>`, excluded.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil || len(directAllFromSchema(t, schema).Members()) != 0 {
					t.Fatalf("zero term was not omitted: %v", err)
				}
			})
		}
	}
}

func TestDirectAllNonNegativeIntegerStrict10RepeatedMemberRejected(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:element name="count" type="xs:nonNegativeInteger" maxOccurs="2"/></xs:all>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="2"`, 1)
	wantSpec := newXSD11FeatureMismatch(FeatureSchemaSyntax, diagnosticSchemaAllOccurrenceVersionCode, wantLoc, "").SpecRef()
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, errLanguagePolicyMismatch) || len(diagnostic.Related()) != 0 {
		t.Fatalf("policy diagnostic = %s, want unsupported at %s", diagnostic, wantLoc)
	}
}

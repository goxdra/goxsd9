package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit // Check the complete copied member view under each graph policy.
func TestDirectAllBuiltinNegativeIntegerFacts(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
		bounds  string
		want    string
	}{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all minOccurs="0"><xs:element name="before" type="xs:integer"/><xs:element name="debt" type="xs:negativeInteger" form="qualified" `+profile.bounds+`/><xs:element name="after" type="xs:boolean"/></xs:all>`, `<xs:element name="root" type="r:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse all negativeInteger: %v", err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated facts/order changed: %v", err)
			}
			all := directAllFromSchema(t, schema)
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" {
				t.Fatalf("all location/range = %s/%s", all.Loc(), all.Occurrences())
			}
			members := all.Members()
			if len(members) != 3 {
				t.Fatalf("members = %d, want three", len(members))
			}
			for index, name := range []string{"before", "debt", "after"} {
				member, ok := members[index].(ElementParticle)
				if !ok || member.Name().Local() != name {
					t.Fatalf("member %d = %#v, want %s", index, members[index], name)
				}
			}
			debt, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("debt member = %T", members[1])
			}
			wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="debt"`, 1)
			wantTypeLoc := allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:negativeInteger"`, 1)
			if debt.Name() != mustTestQName(t, "urn:all", "debt") || debt.Loc() != wantLoc || debt.Occurrences().String() != profile.want || debt.DeclaredType() != mustTestQName(t, testXSDNamespace, "negativeInteger") {
				t.Fatalf("debt facts = %q/%s/%s/%q", debt.Name(), debt.Loc(), debt.Occurrences(), debt.DeclaredType())
			}
			if id, hasID := debt.TypeID(); hasID || !id.IsZero() {
				t.Fatalf("built-in type ID = %v/%t", id, hasID)
			}
			reference, ok := debt.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name() != debt.DeclaredType() || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantTypeLoc {
				t.Fatalf("type reference = %#v/%t", reference, ok)
			}
			if id, hasID := reference.ComponentID(); hasID || !id.IsZero() {
				t.Fatalf("built-in reference ID = %v/%t", id, hasID)
			}
			bounds, ok := reference.IntegerBounds()
			maximum, hasMaximum := bounds.MaxInclusiveFacet()
			if !ok || !hasMaximum || bounds.Version() != profile.version || maximum.Kind() != BoundMaxInclusive || maximum.Value().Canonical() != "-1" || !maximum.Loc().IsZero() {
				t.Fatalf("intrinsic bound = %v/%t, want -1 with no facet Loc", maximum, hasMaximum)
			}
			minimum := debt.Occurrences().Minimum()
			minimum.value.SetInt64(99)
			if finite, hasFinite := debt.Occurrences().Maximum().Finite(); hasFinite {
				finite.value.SetInt64(99)
			}
			maximumValue := maximum.Value()
			maximumValue.value.SetInt64(99)
			members[1] = nil
			copied, ok := directAllFromSchema(t, schema).Members()[1].(ElementParticle)
			if !ok {
				t.Fatal("copied debt member lost element shape")
			}
			copiedReference, _ := copied.TypeReference()
			copiedBounds, _ := copiedReference.IntegerBounds()
			copiedMaximum, _ := copiedBounds.MaxInclusiveFacet()
			if copied.Occurrences().String() != profile.want || copiedMaximum.Value().Canonical() != "-1" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller mutation changed copied schema facts")
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Check both omission boundaries and reference identity across policies.
func TestDirectAllNegativeIntegerOmissionAndReferenceShape(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="omit" type="xs:negativeInteger" minOccurs="0" maxOccurs="0"/><xs:element name="keep" type="xs:negativeInteger"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members := directAllFromSchema(t, schema).Members()
			if len(members) != 1 {
				t.Fatalf("omitted member = %#v", members)
			}
			kept, ok := members[0].(ElementParticle)
			if !ok || kept.Name().Local() != "keep" {
				t.Fatalf("omitted member = %#v", members)
			}
			root = allParticleTestRoot(`<xs:all><xs:element ref="r:global"/><xs:element name="local" type="xs:negativeInteger"/></xs:all>`, `<xs:element name="global" type="xs:negativeInteger"/>`)
			schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			members = directAllFromSchema(t, schema).Members()
			reference, ok := members[0].(ElementReferenceParticle)
			if !ok || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
				t.Fatalf("reference shape = %#v", members[0])
			}
			if _, ok := members[1].(ElementParticle); !ok {
				t.Fatalf("local shape = %T", members[1])
			}
		})
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:negativeInteger"/></xs:all>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		definition := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))[0]
		facts, _ := definition.ComplexTypeDefinition()
		if facts.Particle() != nil {
			t.Fatalf("zero owner published %T", facts.Particle())
		}
	}
}

//nolint:gocognit // Preserve each alternate exit at the direct-all admission boundary.
func TestDirectAllNegativeIntegerExclusionsAndFailures(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Compatibility, XSDVersion11}, {Strict10, XSDVersion10}, {Strict11, XSDVersion11}} {
		for _, test := range []struct {
			name, member, extra, primary, code, spec string
			class                                    FailureClass
			cause                                    error
			related                                  string
		}{
			{"named", `<xs:element name="v" type="r:Named"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, `type="r:Named"`, UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), FailureUnsupported, errSchemaAllMemberScalar, ""},
			{"inline", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, schemaAllLimitedSpecRef(profile.version), FailureUnsupported, errSchemaAllMemberScalar, ""},
			{"named zero", `<xs:element name="v" type="r:Named" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, `type="r:Named"`, "", "", "", nil, ""},
			{"inline zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", "", "", nil, ""},
			{"invalid lexical", `<xs:element name="v" type="xs:negativeInteger" maxOccurs="maybe"/>`, "", `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil, ""},
			{"invalid range", `<xs:element name="v" type="xs:negativeInteger" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="v"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, `minOccurs="2"`},
			{"duplicate", `<xs:element name="v" type="xs:negativeInteger"/><xs:element name="v" type="xs:negativeInteger"/>`, "", `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), FailureInvalid, errSchemaAllMemberDuplicate, `<xs:element name="v"`},
		} {
			t.Run(string(profile.policy)+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if test.code == "" {
					if err != nil || len(directAllFromSchema(t, schema).Members()) != 0 {
						t.Fatalf("zero excluded member = %v/%v", schema, err)
					}
					return
				}
				if err == nil {
					t.Fatal("failure returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				primaryOccurrence := 1
				if test.name == "duplicate" {
					primaryOccurrence = 2
				}
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, primaryOccurrence)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s/%s at %s with %v", diagnostic, test.class, test.code, wantLoc, test.cause)
				}
				if test.spec != "" && diagnostic.SpecRef() != test.spec {
					t.Fatalf("spec = %q, want %q", diagnostic.SpecRef(), test.spec)
				}
				if test.name == "invalid lexical" && diagnostic.Unwrap() == nil {
					t.Fatal("invalid occurrence lost lexical cause")
				}
				var related []Loc
				if test.name == "duplicate" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, 1)}
				}
				if test.name == "invalid range" {
					related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `minOccurs="2"`, 1), allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="1"`, 1)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
			})
		}
	}
}

func TestDirectAllNegativeIntegerStrict10RepeatedMemberRemainsUnsupported(t *testing.T) {
	root := allParticleTestRoot(`<xs:all><xs:element name="v" type="xs:negativeInteger" maxOccurs="2"/></xs:all>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 accepted a repeated all member")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="2"`, 1)
	wantSpec := newXSD11FeatureMismatch(FeatureSchemaSyntax, diagnosticSchemaAllOccurrenceVersionCode, wantLoc, "").SpecRef()
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic = %s, want policy mismatch at %s", diagnostic, wantLoc)
	}
}

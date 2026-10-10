package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type allSignProfile struct {
	policy   LanguagePolicy
	version  XSDVersion
	rangeIn  string
	rangeOut string
}

func allSignProfiles() []allSignProfile {
	return []allSignProfile{
		{Compatibility, XSDVersion11, `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{Strict10, XSDVersion10, `minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, XSDVersion11, `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	}
}

//nolint:gocognit // Verify every public built-in fact together.
func assertAllSignBuiltin(t *testing.T, member Particle, root, local, scalar, occurs string, version XSDVersion) {
	t.Helper()
	element, ok := member.(ElementParticle)
	if !ok {
		t.Fatalf("%s member = %T", local, member)
	}
	wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="`+local+`"`, 1)
	wantName := mustTestQName(t, "urn:all", local)
	if local == "negative" || local == "keep" {
		wantName = mustTestQName(t, "", local)
	}
	wantType := mustTestQName(t, testXSDNamespace, scalar)
	if element.Name() != wantName || element.Loc() != wantLoc || element.DeclaredType() != wantType || element.Occurrences().String() != occurs {
		t.Fatalf("%s facts = %s at %s, %s, %s", local, element.Name(), element.Loc(), element.DeclaredType(), element.Occurrences())
	}
	typeOccurrence := 1
	if local == "keep" {
		typeOccurrence = 2
	}
	ref, ok := element.TypeReference()
	if !ok || !ref.IsBuiltin() || ref.Name() != wantType || ref.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="xs:`+scalar+`"`, typeOccurrence) || ref.VarietyLoc() != ref.Loc() {
		t.Fatalf("%s type reference = %#v", local, ref)
	}
	if id, hasID := element.TypeID(); hasID || !id.IsZero() {
		t.Fatalf("%s synthetic element type ID = %v/%t", local, id, hasID)
	}
	if id, hasID := ref.ComponentID(); hasID || !id.IsZero() {
		t.Fatalf("%s synthetic reference ID = %v/%t", local, id, hasID)
	}
	bounds, ok := ref.IntegerBounds()
	if !ok || bounds.Version() != version {
		t.Fatalf("%s bounds = %v/%t", local, bounds, ok)
	}
	minimum, hasMinimum := bounds.MinInclusiveFacet()
	maximum, hasMaximum := bounds.MaxInclusiveFacet()
	if scalar == "positiveInteger" && (!hasMinimum || hasMaximum || minimum.Kind() != BoundMinInclusive || minimum.Value().Canonical() != "1" || !minimum.Loc().IsZero()) {
		t.Fatalf("positive intrinsic bounds = %v/%v", minimum, maximum)
	}
	if scalar == "nonPositiveInteger" && (hasMinimum || !hasMaximum || maximum.Kind() != BoundMaxInclusive || maximum.Value().Canonical() != "0" || !maximum.Loc().IsZero()) {
		t.Fatalf("nonPositive intrinsic bounds = %v/%v", minimum, maximum)
	}
}

//nolint:gocognit // One public fixture checks copied facts, order, and consumer boundaries across policies.
func TestDirectAllSignIntegerBuiltinFactsAndConsumers(t *testing.T) {
	for _, profile := range allSignProfiles() {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" elementFormDefault="qualified" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:all minOccurs="0"><xs:element name="first" type="xs:integer"/><xs:element name="positive" type="xs:positiveInteger" ` + profile.rangeIn + `/><xs:element name="omit" type="xs:positiveInteger" minOccurs="0" maxOccurs="0"/><xs:element ref="r:forward"/><xs:element name="negative" form="unqualified" type="xs:nonPositiveInteger"/><xs:element name="last" type="xs:boolean"/></xs:all></xs:complexType><xs:element name="forward" type="xs:nonPositiveInteger"/><xs:element name="root" type="r:Record"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated parse differs: %v", err)
			}
			for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
				if found := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, scalar)); len(found) != 0 {
					t.Fatalf("%s gained synthetic component", scalar)
				}
			}
			all := directAllFromSchema(t, schema)
			if all.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `<xs:all minOccurs="0"`, 1) || all.Occurrences().String() != "0/1" {
				t.Fatalf("all location/range = %s/%s", all.Loc(), all.Occurrences())
			}
			members := all.Members()
			if len(members) != 5 {
				t.Fatalf("members = %d, want 5", len(members))
			}
			for index, want := range []string{"first", "positive", "forward", "negative", "last"} {
				name, _ := schemaAllMemberNameAndLoc(members[index])
				if name.Local() != want {
					t.Fatalf("member %d = %s, want %s", index, name, want)
				}
			}
			assertAllSignBuiltin(t, members[1], root, "positive", "positiveInteger", profile.rangeOut, profile.version)
			assertAllSignBuiltin(t, members[3], root, "negative", "nonPositiveInteger", "1/1", profile.version)
			reference, ok := members[2].(ElementReferenceParticle)
			if !ok || reference.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:forward"`, 1) || reference.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "forward"))[0].ID() {
				t.Fatalf("reference member = %#v", members[2])
			}
			positive, ok := members[1].(ElementParticle)
			if !ok {
				t.Fatalf("positive member = %T", members[1])
			}
			positiveRef, _ := positive.TypeReference()
			positiveBounds, _ := positiveRef.IntegerBounds()
			minimum, _ := positiveBounds.MinInclusiveFacet()
			minimumCopy := minimum.Value()
			minimumCopy.value.SetInt64(99)
			occurrenceCopy := positive.Occurrences().Minimum()
			occurrenceCopy.value.SetInt64(99)
			negative, ok := members[3].(ElementParticle)
			if !ok {
				t.Fatalf("negative member = %T", members[3])
			}
			negativeRef, _ := negative.TypeReference()
			negativeBounds, _ := negativeRef.IntegerBounds()
			maximum, _ := negativeBounds.MaxInclusiveFacet()
			maximumCopy := maximum.Value()
			maximumCopy.value.SetInt64(99)
			members[1] = nil
			fresh := directAllFromSchema(t, schema).Members()
			assertAllSignBuiltin(t, fresh[1], root, "positive", "positiveInteger", profile.rangeOut, profile.version)
			assertAllSignBuiltin(t, fresh[3], root, "negative", "nonPositiveInteger", "1/1", profile.version)
			if !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller mutation changed component facts")
			}
			for run := 0; run < 2; run++ {
				var walked []string
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.Name().Local()); return nil }); err != nil || !reflect.DeepEqual(walked, []string{"Record", "forward", "root"}) {
					t.Fatalf("walk %d = %v/%v", run, walked, err)
				}
			}
			assertDirectAllConsumerRejection(t, schema, all, profile.version)
		})
	}
}

//nolint:gocognit // Check source order, ownership, and lexical provenance in one graph.
func TestDirectAllSignIntegerGraphSources(t *testing.T) {
	for _, profile := range allSignProfiles() {
		t.Run(string(profile.policy), func(t *testing.T) {
			includedVersion := profile.version
			if profile.policy == Compatibility {
				includedVersion = XSDVersion10
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + string(profile.version) + `"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="rootPositive" type="xs:positiveInteger"/><xs:element name="rootNegative" type="xs:nonPositiveInteger"/></xs:all></xs:complexType><xs:element name="forward" type="r:Forward"/><xs:complexType name="Forward"><xs:all><xs:element name="forwardPositive" type="xs:positiveInteger"/></xs:all></xs:complexType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:all" version="` + string(includedVersion) + `"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Included"><xs:all><xs:element name="includedNegative" type="xs:nonPositiveInteger"/></xs:all></xs:complexType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:complexType name="Chameleon"><xs:all><xs:element name="adoptedPositive" type="xs:positiveInteger"/></xs:all></xs:complexType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(profile.version) + `"><xs:complexType name="Imported"><xs:all><xs:element name="importedNegative" type="xs:nonPositiveInteger"/></xs:all></xs:complexType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			var sources []SourceID
			for _, component := range schema.Components() {
				source := component.ID().Source()
				if len(sources) == 0 || sources[len(sources)-1] != source {
					sources = append(sources, source)
				}
			}
			if !reflect.DeepEqual(sources, []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}) {
				t.Fatalf("source order = %v", sources)
			}
			for _, want := range []struct{ source, namespace, owner, local, scalar string }{
				{"root.xsd", "urn:all", "Record", "rootPositive", "positiveInteger"},
				{"root.xsd", "urn:all", "Forward", "forwardPositive", "positiveInteger"},
				{"included.xsd", "urn:all", "Included", "includedNegative", "nonPositiveInteger"},
				{"chameleon.xsd", "urn:all", "Chameleon", "adoptedPositive", "positiveInteger"},
				{"other.xsd", "urn:other", "Imported", "importedNegative", "nonPositiveInteger"},
			} {
				component := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, want.namespace, want.owner))
				if len(component) != 1 || component[0].ID().Source() != SourceID(want.source) {
					t.Fatalf("%s owner identity = %v", want.owner, component)
				}
				definition, _ := component[0].ComplexTypeDefinition()
				all, ok := definition.Particle().(AllParticle)
				if !ok || len(all.Members()) == 0 {
					t.Fatalf("%s all = %T", want.owner, definition.Particle())
				}
				member, ok := all.Members()[0].(ElementParticle)
				if !ok {
					t.Fatalf("%s member = %T", want.owner, all.Members()[0])
				}
				declaration := root
				if want.source != "root.xsd" {
					declaration = fixtures[want.source].contents
				}
				ref, _ := member.TypeReference()
				bounds, hasBounds := ref.IntegerBounds()
				if !hasBounds || bounds.Version() != profile.version {
					t.Fatalf("%s bound edition = %v/%t, want %s", want.owner, bounds.Version(), hasBounds, profile.version)
				}
				typeOccurrence := 1
				if want.owner == "Forward" {
					typeOccurrence = 2
				}
				if member.Name().Local() != want.local || member.DeclaredType() != mustTestQName(t, testXSDNamespace, want.scalar) || member.Loc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, `<xs:element name="`+want.local+`"`, 1) || ref.Loc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, `type="xs:`+want.scalar+`"`, typeOccurrence) {
					t.Fatalf("%s provenance = %#v", want.owner, member)
				}
			}
		})
	}
}

func TestDirectAllSignIntegerConsumerOutputsRemainNil(t *testing.T) {
	for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
		root := allParticleTestRoot(`<xs:all><xs:element name="v" type="xs:`+scalar+`"/></xs:all>`, `<xs:element name="root" type="r:Record"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatal(err)
		}
		all := directAllFromSchema(t, schema)
		instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:all"/>`)))
		instanceDiagnostic := requireDiagnostic(t, instanceErr)
		if instanceDiagnostic.Code() != UnsupportedInstanceValidationCode || instanceDiagnostic.Loc() != all.Loc() || instanceDiagnostic.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(instanceErr, errInstanceChoiceParticle) {
			t.Fatalf("%s validation = %s", scalar, instanceDiagnostic)
		}
		generated, generationErr := GenerateGo(schema, "generated")
		generationDiagnostic := requireDiagnostic(t, generationErr)
		if generated != nil || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != all.Loc() || generationDiagnostic.SpecRef() != codegenDirectSequenceSpecReference(XSDVersion11, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
			t.Fatalf("%s generation = %q/%s", scalar, generated, generationDiagnostic)
		}
	}
}

//nolint:gocognit // Named, inline, ref, and omitted forms share the all-member boundary.
func TestDirectAllSignIntegerExcludedShapesAndReferences(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
			for _, test := range []struct {
				name, member, extra, marker, spec string
				cause                             error
			}{
				{"named derivative", `<xs:element name="v" type="r:Derived"/>`, `<xs:simpleType name="Derived"><xs:restriction base="xs:` + scalar + `"/></xs:simpleType>`, `type="r:Derived"`, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
				{"anonymous derivative", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:` + scalar + `"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef(), ErrUnsupported},
			} {
				t.Run(string(profile.policy)+"/"+scalar+"/"+test.name, func(t *testing.T) {
					root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || !errors.Is(err, test.cause) {
						t.Fatalf("%s exclusion = %s, cause %v", test.name, diagnostic, err)
					}
				})
			}
			t.Run(string(profile.policy)+"/"+scalar+"/zero and ref", func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all><xs:element name="directOmit" type="xs:`+scalar+`" minOccurs="0" maxOccurs="0"/><xs:element name="namedOmit" type="r:Derived" minOccurs="0" maxOccurs="0"/><xs:element name="inlineOmit" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:`+scalar+`"/></xs:simpleType></xs:element><xs:element ref="r:global"/><xs:element name="keep" type="xs:`+scalar+`"/></xs:all>`, `<xs:simpleType name="Derived"><xs:restriction base="xs:`+scalar+`"/></xs:simpleType><xs:element name="global" type="xs:`+scalar+`"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				members := directAllFromSchema(t, schema).Members()
				if len(members) != 2 {
					t.Fatalf("zero terms persisted: %v", members)
				}
				ref, ok := members[0].(ElementReferenceParticle)
				if !ok || ref.RefLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="r:global"`, 1) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:all", "global"))[0].ID() {
					t.Fatalf("reference = %#v", members[0])
				}
				assertAllSignBuiltin(t, members[1], root, "keep", scalar, "1/1", profile.version)
			})
		}
	}
}

func TestDirectAllSignIntegerOtherOwnersRemainExcluded(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
			extensionSpec := "xsd11-structures#cos-ct-extends"
			if profile.version == XSDVersion10 {
				extensionSpec = "xsd10-structures#cos-ct-extends"
			}
			for _, test := range []struct{ name, body, marker, spec string }{
				{"inline complex", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="v" type="xs:` + scalar + `"/></xs:all></xs:complexType></xs:element>`, `<xs:all>`, "xsd10-structures#schema-document"},
				{"extension", `<xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="v" type="xs:` + scalar + `"/></xs:all></xs:extension></xs:complexContent></xs:complexType>`, `<xs:extension`, extensionSpec},
				{"named group", `<xs:group name="Group"><xs:all><xs:element name="v" type="xs:` + scalar + `"/></xs:all></xs:group>`, `<xs:all>`, newSchemaSyntaxUnsupportedForVersion(Loc{}, "", profile.version).SpecRef()},
			} {
				t.Run(string(profile.policy)+"/"+scalar+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1) || diagnostic.SpecRef() != test.spec || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s = %s, cause %v", test.name, diagnostic, err)
					}
				})
			}
		}
	}
}

type allSignFailure struct {
	name, member, extra, marker, related, code, spec string
	markerOccurrence, relatedOccurrence              int
	class                                            FailureClass
	cause                                            error
	lexicalCause                                     bool
}

func assertAllSignFailure(t *testing.T, profile allSignProfile, test allSignFailure) {
	t.Helper()
	root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, test.extra)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.markerOccurrence)
	var related []Loc
	if test.related != "" {
		related = []Loc{allParticleTestTokenLoc(t, "root.xsd", root, test.related, test.relatedOccurrence)}
	}
	if test.name == "reversed occurrence" {
		related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="1"`, 1))
	}
	if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) || test.cause != nil && !errors.Is(err, test.cause) {
		t.Fatalf("%s = %s related %v, cause %v; want %s at %s related %v spec %s", test.name, diagnostic, diagnostic.Related(), err, test.code, wantLoc, related, test.spec)
	}
	if test.lexicalCause {
		var inner Diagnostic
		if !errors.As(errors.Unwrap(diagnostic), &inner) || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != wantLoc {
			t.Fatalf("%s lost located lexical cause: %v", test.name, errors.Unwrap(diagnostic))
		}
	}
}

func TestDirectAllSignIntegerFailureExits(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
			invalidFacet := `<xs:maxInclusive value="0"/>`
			facetMarker := `value="0"`
			facetSpec := boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule)
			if scalar == "nonPositiveInteger" {
				invalidFacet = `<xs:minInclusive value="1"/>`
				facetMarker = `value="1"`
				facetSpec = boundSpecRef(profile.version, BoundMinInclusive, boundRestrictionRule)
			}
			for _, test := range []allSignFailure{
				{"malformed occurrence", `<xs:element name="v" type="xs:` + scalar + `" maxOccurs="many"/>`, "", `maxOccurs="many"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), 1, 0, FailureInvalid, nil, true},
				{"reversed occurrence", `<xs:element name="v" type="xs:` + scalar + `" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="v"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), 1, 1, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, false},
				{"unresolved type before zero", `<xs:element name="v" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), 1, 0, FailureInvalid, errSchemaElementTypeUnresolved, false},
				{"wrong kind before zero", `<xs:element name="v" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:` + scalar + `"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), 1, 1, FailureInvalid, errSchemaElementTypeWrongKind, false},
				{"invalid named facet before zero", `<xs:element name="v" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:` + scalar + `">` + invalidFacet + `</xs:restriction></xs:simpleType>`, facetMarker, "", InvalidBoundRestrictionCode, facetSpec, 1, 0, FailureInvalid, errInvalidBoundRestriction, false},
				{"invalid inline facet before zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:` + scalar + `">` + invalidFacet + `</xs:restriction></xs:simpleType></xs:element>`, "", facetMarker, "", InvalidBoundRestrictionCode, facetSpec, 1, 0, FailureInvalid, errInvalidBoundRestriction, false},
				{"unresolved ref before zero", `<xs:element ref="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), 1, 0, FailureInvalid, errSchemaElementReferenceUnresolved, false},
				{"duplicate", `<xs:element name="v" type="xs:` + scalar + `"/><xs:element name="v" type="xs:` + scalar + `"/>`, "", `<xs:element name="v"`, `<xs:element name="v"`, diagnosticSchemaElementReferenceDuplicateCode, schemaAllLimitedSpecRef(profile.version), 2, 1, FailureInvalid, errSchemaAllMemberDuplicate, false},
			} {
				t.Run(string(profile.policy)+"/"+scalar+"/"+test.name, func(t *testing.T) {
					assertAllSignFailure(t, profile, test)
				})
			}
		}
	}
}

func TestDirectAllSignIntegerStrict10RepeatGate(t *testing.T) {
	for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
		t.Run(scalar, func(t *testing.T) {
			root := allParticleTestRoot(`<xs:all><xs:element name="v" type="xs:`+scalar+`" maxOccurs="2"/></xs:all>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="2"`, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 repeat = %s, cause %v", diagnostic, err)
			}
		})
	}
}

func TestDirectAllSignIntegerOuterZeroAndVersionGate(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="positive" type="xs:positiveInteger"/><xs:element name="negative" type="xs:nonPositiveInteger"/></xs:all>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s outer zero: %v", policy, err)
		}
		definition := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:all", "Record"))
		if len(definition) != 1 {
			t.Fatalf("%s Record count = %d", policy, len(definition))
		}
		complexType, ok := definition[0].ComplexTypeDefinition()
		if !ok || complexType.Particle() != nil {
			t.Fatalf("%s zero outer all published %T", policy, complexType.Particle())
		}
	}
	root := allParticleTestRoot(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="positive" type="xs:positiveInteger"/></xs:all>`, "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `maxOccurs="0"`, 1) || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("Strict10 outer zero = %s, cause %v", diagnostic, err)
	}
}

func TestDirectAllSignIntegerHiddenAndCyclicGraphs(t *testing.T) {
	for _, profile := range allSignProfiles() {
		for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
			t.Run(string(profile.policy)+"/"+scalar+"/hidden", func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:all"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:all><xs:element name="v" type="h:Hidden" minOccurs="0" maxOccurs="0"/></xs:all></xs:complexType></xs:schema>`
				fixtures := map[string]discoveryFixture{
					"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
					"hidden.xsd": {id: "hidden.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + scalar + `"/></xs:simpleType></xs:schema>`},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="h:Hidden"`, 1) || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
					t.Fatalf("hidden type = %s, cause %v", diagnostic, err)
				}
			})
			t.Run(string(profile.policy)+"/"+scalar+"/cycle", func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all><xs:element name="v" type="r:One" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:`+scalar+`"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `base="r:Two"`, 1) || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !reflect.DeepEqual(diagnostic.Related(), []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `base="r:One"`, 1)}) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
					t.Fatalf("type cycle = %s, cause %v", diagnostic, err)
				}
			})
		}
	}
}

func TestDirectAllSignIntegerSyntaxBeforeZero(t *testing.T) {
	for _, scalar := range []string{"positiveInteger", "nonPositiveInteger"} {
		for _, test := range []struct {
			name, member, marker, code string
			occurrence                 int
		}{
			{"duplicate type", `<xs:element name="v" type="xs:` + scalar + `" type="xs:` + scalar + `" minOccurs="0" maxOccurs="0"/>`, `type="xs:` + scalar + `"`, InvalidXMLSyntaxCode, 2},
			{"invalid name", `<xs:element name="1v" type="xs:` + scalar + `" minOccurs="0" maxOccurs="0"/>`, `name="1v"`, invalidSchemaDeclarationNameCode, 1},
			{"invalid form", `<xs:element name="v" form="other" type="xs:` + scalar + `" minOccurs="0" maxOccurs="0"/>`, `form="other"`, invalidSchemaCompositionCode, 1},
		} {
			t.Run(scalar+"/"+test.name, func(t *testing.T) {
				root := allParticleTestRoot(`<xs:all>`+test.member+`</xs:all>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.occurrence) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "" {
					t.Fatalf("%s syntax = %s, cause %v", test.name, diagnostic, err)
				}
			})
		}
	}
}

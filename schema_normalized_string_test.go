package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + body + `</xs:schema>`
}

func normalizedStringDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	t.Helper()
	return schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:test", local)
}

func assertNormalizedStringReference(t *testing.T, reference SimpleTypeReference, loc Loc) {
	t.Helper()
	name := mustTestQName(t, testXSDNamespace, "normalizedString")
	if !reference.IsBuiltin() || reference.Name() != name || reference.QName() != name || reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.Loc() != loc || reference.VarietyLoc() != loc {
		t.Fatalf("normalizedString reference = %#v, want built-in atomic reference at %s", reference, loc)
	}
	if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicNormalizedString {
		t.Fatalf("normalizedString atomic facts = %#v", reference.facts)
	}
	whiteSpace, ok := reference.StringWhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "replace" || whiteSpace.Fixed() || !whiteSpace.Loc().IsZero() {
		t.Fatalf("normalizedString whitespace = %#v/%t, want unlocated nonfixed replace", whiteSpace, ok)
	}
	if id, hasID := reference.ComponentID(); hasID || !id.IsZero() {
		t.Fatalf("built-in ID = %v/%t, want no component ID", id, hasID)
	}
	if id, hasID := reference.AnonymousID(); hasID || !id.IsZero() {
		t.Fatalf("built-in anonymous ID = %v/%t, want no anonymous ID", id, hasID)
	}
}

//nolint:gocognit,funlen // Public references, copied facts, and ordered variety links form one admission contract.
func TestNormalizedStringReferencesAndFacetsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:element name="direct" type="xs:normalizedString"/>
  <xs:element name="named" type="t:Child"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType></xs:element>
  <xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:enumeration value="a b"/><xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value="a&#x9;b"/><xs:enumeration value="a&#xD;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:normalizedString"/></xs:simpleType>
  <xs:simpleType name="NamedList"><xs:list itemType="t:Base"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:normalizedString t:Base"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:union></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			if schema.LanguagePolicy() != profile.policy || len(schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "normalizedString"))) != 0 {
				t.Fatal("policy or built-in component identity is wrong")
			}
			direct := tokenElementDefinition(t, schema, "direct")
			reference, ok := direct.TypeReference()
			if !ok {
				t.Fatal("direct type reference is missing")
			}
			assertNormalizedStringReference(t, reference, mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="xs:normalizedString"`))
			if id, hasID := direct.TypeID(); hasID || !id.IsZero() {
				t.Fatalf("direct built-in type ID = %v/%t", id, hasID)
			}
			base := normalizedStringDefinition(t, schema, "Base")
			if !base.IsString() || base.facts.atomicKind != schemaSimpleTypeAtomicNormalizedString {
				t.Fatal("Base did not retain distinct string-derived kind")
			}
			baseRef, ok := base.BaseReference()
			if !ok {
				t.Fatal("Base reference missing")
			}
			assertNormalizedStringReference(t, baseRef, mustSchemaTokenLoc(t, "root.xsd", root, 6, `base="xs:normalizedString"`))
			assertStringEnumerationFacts(t, base.StringEnumerationFacets(), profile.version, []string{"a\nb", "a\tb", "a\rb", " a  b "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 6, `value="a&#xA;b"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 6, `value="a&#x9;b"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 6, `value="a&#xD;b"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 6, `value=" a  b "`),
			})
			whiteSpace, ok := base.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "replace" || whiteSpace.Fixed() || !whiteSpace.Loc().IsZero() {
				t.Fatalf("Base whitespace = %#v/%t", whiteSpace, ok)
			}
			inherited := normalizedStringDefinition(t, schema, "Inherited")
			assertStringEnumerationFacts(t, inherited.StringEnumerationFacets(), profile.version, []string{"a\nb", "a\tb", "a\rb", " a  b "}, base.StringEnumerationFacets().Locations())
			child := normalizedStringDefinition(t, schema, "Child")
			if !child.IsString() || child.facts.atomicKind != schemaSimpleTypeAtomicNormalizedString {
				t.Fatal("Child lost normalizedString identity")
			}
			assertStringEnumerationFacts(t, child.StringEnumerationFacets(), profile.version, []string{"a b"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 5, `value="a b"`)})
			whiteSpace, ok = child.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "collapse" || whiteSpace.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 5, `value="collapse"`) {
				t.Fatalf("Child whitespace = %#v/%t", whiteSpace, ok)
			}
			childBase, ok := child.BaseReference()
			if !ok || !childBase.IsNamed() || childBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 5, `base="t:Base"`) {
				t.Fatalf("Child base = %#v/%t", childBase, ok)
			}
			if id, hasID := childBase.ComponentID(); !hasID || id != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Base")) {
				t.Fatalf("Child base ID = %v/%t", id, hasID)
			}
			inlineRef, ok := tokenElementDefinition(t, schema, "inline").TypeReference()
			if !ok || !inlineRef.IsAnonymous() {
				t.Fatal("inline element has no anonymous reference")
			}
			if id, hasID := inlineRef.AnonymousID(); !hasID || id.IsZero() {
				t.Fatalf("inline ID = %v/%t", id, hasID)
			}
			inline, ok := inlineRef.AnonymousType()
			if !ok || !inline.IsString() || inline.facts.atomicKind != schemaSimpleTypeAtomicNormalizedString {
				t.Fatal("inline type lost normalizedString identity")
			}
			inlineBase, ok := inline.BaseReference()
			if !ok {
				t.Fatal("inline base missing")
			}
			assertNormalizedStringReference(t, inlineBase, mustSchemaTokenLoc(t, "root.xsd", root, 4, `base="xs:normalizedString"`))
			list := normalizedStringDefinition(t, schema, "List")
			item, ok := list.ItemType()
			if !ok || list.Variety() != SimpleTypeVarietyList {
				t.Fatal("list item missing")
			}
			assertNormalizedStringReference(t, item, mustSchemaTokenLoc(t, "root.xsd", root, 8, `itemType="xs:normalizedString"`))
			namedItem, ok := normalizedStringDefinition(t, schema, "NamedList").ItemType()
			if !ok || !namedItem.IsNamed() || namedItem.Name() != mustTestQName(t, "urn:test", "Base") {
				t.Fatalf("named list item = %#v/%t", namedItem, ok)
			}
			if id, hasID := namedItem.ComponentID(); !hasID || id != componentIDForName(t, schema, mustTestQName(t, "urn:test", "Base")) {
				t.Fatalf("named list item ID = %v/%t", id, hasID)
			}
			union := normalizedStringDefinition(t, schema, "Union")
			members := union.MemberTypes()
			if union.Variety() != SimpleTypeVarietyUnion || len(members) != 3 {
				t.Fatalf("union members = %#v", members)
			}
			assertNormalizedStringReference(t, members[0], mustSchemaTokenLoc(t, "root.xsd", root, 10, `memberTypes="`))
			if !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Base") || !members[2].IsAnonymous() {
				t.Fatalf("union member order = %#v", members)
			}
			members[0] = SimpleTypeReference{}
			if union.MemberTypes()[0].Kind() != SimpleTypeReferenceBuiltin {
				t.Fatal("union member slice changed schema")
			}
			facets := base.StringEnumerationFacets()
			values := facets.Values()
			values[0] = "changed"
			if base.StringEnumerationFacets().Values()[0] != "a\nb" {
				t.Fatal("enumeration copy changed schema")
			}
			changed := whiteSpace.WithFixed(true)
			if !changed.Fixed() || whiteSpace.Fixed() {
				t.Fatal("whitespace copy changed schema")
			}
		})
	}
}

//nolint:gocognit // Keep all alternate facet exits under the same policy matrix.
func TestNormalizedStringRestrictionDiagnosticsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, code, mark, spec string
			cause                        error
			related                      string
		}{
			{"invalid whitespace", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="unknown"/></xs:restriction></xs:simpleType>`, InvalidStringWhiteSpaceCode, `value="unknown"`, stringWhiteSpaceSpecRef(profile.version), errInvalidStringWhiteSpaceValue, ""},
			{"less restrictive whitespace", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, InvalidStringWhiteSpaceRestrictionCode, `value="preserve"`, stringWhiteSpaceSpecRef(profile.version), errInvalidStringWhiteSpaceRestriction, ""},
			{"outside enumeration", `<xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:enumeration value="c"/></xs:restriction></xs:simpleType>`, InvalidEnumerationRestrictionCode, `value="c"`, versionedEnumerationSpecRef(profile.version, "enumeration-valid-restriction"), errInvalidEnumerationRestriction, `value="a&#xA;b"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("invalid restriction returned a schema or no error")
				}
				d := requireDiagnostic(t, err)
				want := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != want || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, want %s at %s with %s and %v", err, test.code, want, test.spec, test.cause)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 2, test.related)}
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("related = %v, want %v", d.Related(), related)
				}
			})
		}
	}
}

//nolint:gocognit // Each consumer and element shape needs independent diagnostics.
func TestNormalizedStringConsumerExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, body string }{
			{"direct", `<xs:element name="item" type="xs:normalizedString"/>`},
			{"named", `<xs:element name="item" type="t:Text"/><xs:simpleType name="Text"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
			{"inline", `<xs:element name="item"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:element>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatal("Go generation admitted normalizedString")
				}
				d := requireDiagnostic(t, err)
				wantGenSpec := schemaElementTypeSpecRef(profile.version)
				if test.name == "named" {
					wantGenSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				wantElementLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != wantElementLoc || d.SpecRef() != wantGenSpec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation diagnostic = %v", err)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:test">a b</item>`)))
				if err == nil {
					t.Fatal("instance validation admitted normalizedString")
				}
				d = requireDiagnostic(t, err)
				wantCause := errInstanceUnsupportedType
				if test.name == "inline" {
					wantCause = errInstanceNoDeclaredType
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != (Loc{source: "instance.xml", line: 1, column: 1}) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, wantCause) {
					t.Fatalf("validation diagnostic = %v; loc=%s spec=%s cause=%t", err, d.Loc(), d.SpecRef(), errors.Is(err, errInstanceUnsupportedType))
				}
				wantRelated := []Loc{wantElementLoc}
				if test.name == "named" {
					wantRelated = append(wantRelated, mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:simpleType`))
				}
				if !reflect.DeepEqual(d.Related(), wantRelated) {
					t.Fatalf("validation related = %v, want %v", d.Related(), wantRelated)
				}
			})
		}
	}
}

//nolint:gocognit // Graph discovery, chameleon adoption, and cycles share one reference boundary.
func TestNormalizedStringGraphReferences(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="Forward"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
  <xs:simpleType name="FromIncluded"><xs:restriction base="t:Included"/></xs:simpleType>
  <xs:simpleType name="FromImported"><xs:restriction base="o:Imported"/></xs:simpleType>
</xs:schema>`
			included := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"included.xsd": {id: "included.xsd", contents: included},
				"other.xsd":    {id: "other.xsd", contents: other},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			wantNames := []QName{
				mustTestQName(t, "urn:test", "Forward"), mustTestQName(t, "urn:test", "Later"),
				mustTestQName(t, "urn:test", "FromIncluded"), mustTestQName(t, "urn:test", "FromImported"),
				mustTestQName(t, "urn:test", "Included"), mustTestQName(t, "urn:other", "Imported"),
			}
			components := schema.Components()
			if len(components) != len(wantNames) {
				t.Fatalf("component count = %d, want %d", len(components), len(wantNames))
			}
			for i, want := range wantNames {
				if components[i].Name() != want {
					t.Fatalf("component %d = %q, want %q", i, components[i].Name(), want)
				}
			}
			for _, test := range []struct{ name, target, source string }{
				{"Forward", "Later", "root.xsd"},
				{"FromIncluded", "Included", "included.xsd"},
				{"FromImported", "Imported", "other.xsd"},
			} {
				definition := normalizedStringDefinition(t, schema, test.name)
				base, ok := definition.BaseReference()
				if !ok || !base.IsNamed() || base.Name().Local() != test.target || base.Name().Namespace() == "" {
					t.Fatalf("%s base = %#v/%t", test.name, base, ok)
				}
				id, hasID := base.ComponentID()
				if !hasID || id.Source() != SourceID(test.source) || id != componentIDForName(t, schema, base.Name()) {
					t.Fatalf("%s base ID = %v/%t", test.name, id, hasID)
				}
				whiteSpace, ok := base.StringWhiteSpaceFacet()
				if !ok || whiteSpace.Value() != "replace" || !whiteSpace.Loc().IsZero() {
					t.Fatalf("%s base whitespace = %#v/%t", test.name, whiteSpace, ok)
				}
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("repeated graph changed order or facts: %v", err)
			}
		})
	}
}

func TestNormalizedStringExcludedComponentShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark, code, spec string
		}{
			{"global attribute inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version)},
			{"local direct", `<xs:complexType name="Box"><xs:sequence><xs:element name="a" type="xs:normalizedString"/></xs:sequence></xs:complexType>`, `type="xs:normalizedString"`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"local named", `<xs:complexType name="Box"><xs:sequence><xs:element name="a" type="t:T"/></xs:sequence></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"local inline", `<xs:complexType name="Box"><xs:sequence><xs:element name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:element></xs:sequence></xs:complexType>`, `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"length facet", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal")},
			{"pattern facet", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal")},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded component returned a schema or no error")
				}
				d := requireDiagnostic(t, err)
				want := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != want || d.SpecRef() != test.spec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %v (spec %s); want %s at %s, spec %s", err, d.SpecRef(), test.code, want, test.spec)
				}
			})
		}
	}
}

func TestNormalizedStringValueSpaceAndFixedFacetDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, code, mark, related, spec string
			cause                                 error
		}{
			{"replace preserves runs", `<xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>`, InvalidEnumerationRestrictionCode, `value="a b"`, `value="a  b"`, versionedEnumerationSpecRef(profile.version, "enumeration-valid-restriction"), errInvalidEnumerationRestriction},
			{"fixed replace rejects collapse", `<xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="replace" fixed="true"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType>`, InvalidStringWhiteSpaceRestrictionCode, `value="collapse"`, `value="replace"`, stringWhiteSpaceSpecRef(profile.version), errInvalidStringWhiteSpaceRestriction},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("invalid restriction returned a schema or no error")
				}
				d := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 2, test.related)}
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, spec=%s related=%v", err, d.SpecRef(), d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Resolution and cycles share the reference boundary.
func TestNormalizedStringReferenceResolutionFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, code, mark string
			class                  FailureClass
			cause                  error
		}{
			{"unresolved", `<xs:simpleType name="T"><xs:restriction base="t:Missing"/></xs:simpleType><xs:simpleType name="Later"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, diagnosticSchemaSimpleTypeUnresolvedCode, `base="t:Missing"`, FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
			{"cycle", `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, diagnosticSchemaSimpleTypeCycleCode, `base="t:B"`, FailureInvalid, errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("bad reference returned a schema or no error")
				}
				d := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, actual class=%s code=%s loc=%s spec=%s, want class=%s code=%s loc=%s spec=%s cause=%t; related=%v", err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), test.class, test.code, wantLoc, schemaSimpleTypeSpecRef(profile.version), errors.Is(err, test.cause), d.Related())
				}
				var related []Loc
				if test.name == "cycle" {
					related = []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="t:A"`)}
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("related = %v, want %v", d.Related(), related)
				}
			})
		}
	}
}

//nolint:gocognit // Preserve independent generation and validation diagnostics.
func TestNormalizedStringElementRefConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringRoot(`
  <xs:complexType name="Box"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:complexType>
  <xs:element name="box" type="t:Box"/>
  <xs:element name="item" type="xs:normalizedString"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatal("Go generation admitted normalizedString reference target")
			}
			d := requireDiagnostic(t, err)
			wantRefLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `ref="t:item"`)
			wantGenSpec := "xsd11-structures#element-choice"
			if profile.version == XSDVersion10 {
				wantGenSpec = "xsd10-structures#element-choice"
			}
			wantGenRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element`),
			}
			if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != wantRefLoc || d.SpecRef() != wantGenSpec || !reflect.DeepEqual(d.Related(), wantGenRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %v", err)
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<box xmlns="urn:test"><item>a b</item></box>`)))
			if err == nil {
				t.Fatal("validation admitted normalizedString reference target")
			}
			d = requireDiagnostic(t, err)
			wantValidationRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:choice`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element`),
			}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != (Loc{source: "instance.xml", line: 1, column: 1}) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), wantValidationRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %v", err)
			}
		})
	}
}

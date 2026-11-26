package goxsd9

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func parseAnyURIPublic(t *testing.T, contents string, resolver *discoveryResolver, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	root, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(contents)})
	if err != nil {
		t.Fatal(err)
	}
	return ParseSchemaWithPolicy(root, resolver, policy)
}

//nolint:gocognit // Each lexical form has an edition-specific public outcome.
func TestSchemaAnyURINotationLexicalAdmission(t *testing.T) {
	for _, test := range []struct {
		name, value string
		strict10    bool
	}{
		{"malformed percent", "##%ZZ", false},
		{"URI-like brackets", "http://[bad", false},
		{"brackets in path", "a[b]", false},
		{"multiple fragments", "a#b#c", false},
		{"empty scheme payload", "foo:", false},
		{"whitespace collapse empties scheme", "foo: ", false},
		{"opaque question mark", "foo:?q", true},
		{"empty scheme with fragment", "foo:#frag", false},
		{"query without path", "?q", false},
		{"query without path and fragment", "?#frag", false},
		{"opaque starts with bracket", "foo:[part]", false},
		{"later IPv6 zone syntax", "http://[fe80::1%25eth0]/", false},
		{"stray closing authority bracket", "http://abc]/", false},
		{"misplaced opening authority bracket", "http://abc[::1]/", false},
		{"IPv6 host suffix", "http://[::1]extra/", false},
		{"IPv6 host nondecimal port", "http://[::1]:abc/", false},
		{"IPv6 host repeated userinfo", "http://user@[::1]@foo/", false},
		{"IPv6 double compression", "http://[1::2::3]/", false},
		{"IPv6 IPv4 octet overflow", "http://[::192.256.5.5]/", false},
		{"IPv6 IPv4 octet too long", "http://[::192.0009.5.5]/", false},
		{"raw space", "a b", true},
		{"XML DEL", "a\u007fb", true},
		{"Unicode", "caf\u00e9", true},
		{"fragment", "#part", true},
		{"brackets in query", "a?q=[part]", true},
		{"brackets in fragment", "#frag[ment]", true},
		{"registered name escaped host", "http://%41.example/", true},
		{"registered name colon", "http://foo:bar/", true},
		{"registered name XLink escape", "http://caf\u00e9.example/", true},
		{"scheme with absolute path", "foo:/", true},
		{"opaque bracket after first character", "foo:a[part]", true},
		{"IPv6 host", "http://[::1]/", true},
		{"IPv6 embedded IPv4", "http://[::192.9.5.5]/", true},
		{"IPv6 embedded IPv4 leading zero", "http://[::192.009.5.5]/", true},
		{"IPv6 userinfo and port", "http://user@[::1]:80/", true},
		{"empty", "", true},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" public="p" system="` + test.value + `"/></xs:schema>`
				resolver := &discoveryResolver{}
				schema, err := parseAnyURIPublic(t, root, resolver, policy)
				loc := mustSchemaTokenLoc(t, "root.xsd", root, 1, `system=`)
				if len(resolver.calls) != 0 {
					t.Fatalf("notation unexpectedly resolved a source: %#v", resolver.calls)
				}
				if policy != Strict10 || test.strict10 {
					if err != nil {
						t.Fatalf("ParseSchemaWithPolicy: %v", err)
					}
					if len(schema.Documents()) != 1 || len(schema.Components()) != 1 {
						t.Fatalf("schema documents/components = %d/%d", len(schema.Documents()), len(schema.Components()))
					}
					notation, ok := schema.Components()[0].Notation()
					if !ok {
						t.Fatal("notation view absent")
					}
					if value, present := notation.System(); !present || value != collapseXMLWhitespace(test.value) {
						t.Fatalf("notation system = %q/%t, want %q", value, present, collapseXMLWhitespace(test.value))
					}
					if got, present := notation.SystemLoc(); !present || got != loc {
						t.Fatalf("notation system location = %s/%t, want %s", got, present, loc)
					}
					return
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaNotationCode || diagnostic.Loc() != loc || diagnostic.SpecRef() != schemaNotationXSD10SpecRef || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaNotationSystem) {
					t.Fatalf("Strict10 notation diagnostic = %v", err)
				}
				var lexical Diagnostic
				if !errors.As(diagnostic.Unwrap(), &lexical) || lexical.Class() != FailureInvalid || lexical.Code() != invalidSchemaCompositionCode || lexical.Loc() != loc || lexical.SpecRef() != "xsd10-datatypes#anyURI-lexical-representation" || len(lexical.Related()) != 0 || lexical.Unwrap() != nil {
					t.Fatalf("Strict10 lexical cause = %v", diagnostic.Unwrap())
				}
			})
		}
	}
}

func TestSchemaAnyURILegacyAuthorityMarks(t *testing.T) {
	for _, mark := range "-_.!~*'()" {
		for _, authority := range []struct{ name, value string }{
			{"registered name", "http://a" + string(mark) + "b/"},
			{"IPv6 userinfo", "http://u" + string(mark) + "v@[::1]/"},
		} {
			for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
				t.Run(authority.name+"/"+string(mark)+"/"+string(policy), func(t *testing.T) {
					assertAnyURIAuthorityMarkNotation(t, authority.value, policy)
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="` + authority.value + `"/>`
					assertAnyURIAlternateCase(t, anyURIAlternateCase{"XML Base mark", root, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported}, policy)
				})
			}
		}
	}
}

func assertAnyURIAuthorityMarkNotation(t *testing.T, value string, policy LanguagePolicy) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" public="p" system="` + value + `"/></xs:schema>`
	resolver := &discoveryResolver{}
	schema, err := parseAnyURIPublic(t, root, resolver, policy)
	if err != nil {
		t.Fatalf("notation system %q: %v", value, err)
	}
	if len(resolver.calls) != 0 || len(schema.Documents()) != 1 || len(schema.Components()) != 1 {
		t.Fatalf("notation discovery = documents %d, components %d, calls %#v", len(schema.Documents()), len(schema.Components()), resolver.calls)
	}
	notation, ok := schema.Components()[0].Notation()
	if !ok {
		t.Fatal("notation view absent")
	}
	if got, present := notation.System(); !present || got != value {
		t.Fatalf("notation system = %q/%t, want %q", got, present, value)
	}
	if got, present := notation.SystemLoc(); !present || got != mustSchemaTokenLoc(t, "root.xsd", root, 1, `system=`) {
		t.Fatalf("notation system location = %s/%t", got, present)
	}
}

func TestSchemaAnyURILegacyAuthorityLocationReachesResolverUnchanged(t *testing.T) {
	for _, location := range []string{
		"http://[::192.009.5.5]/",
		"http://a(b)/",
		"http://u(v)@[::1]/",
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(location+"/"+string(policy), func(t *testing.T) {
				assertAnyURIAuthorityLocationReachesResolver(t, location, policy)
			})
		}
	}
}

func assertAnyURIAuthorityLocationReachesResolver(t *testing.T, location string, policy LanguagePolicy) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="` + location + `"/><xs:element name="root"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="child"/></xs:schema>`
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{location: {id: "child.xsd", contents: child}}}
	schema, err := parseAnyURIPublic(t, root, resolver, policy)
	if err != nil {
		t.Fatalf("schemaLocation %q: %v", location, err)
	}
	if len(schema.Documents()) != 2 || len(schema.Components()) != 2 || schema.Components()[1].Name().Local() != "child" {
		t.Fatalf("discovered schema = %#v, %#v", schema.Documents(), schema.Components())
	}
	if want := []discoveryCall{{location: location}}; !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
	}
}

//nolint:gocognit // Resolver calls, publication, and lexical failure are one phase boundary.
func TestSchemaAnyURIDiscoveryKeepsLexicalLocationsAndResolverInput(t *testing.T) {
	for _, test := range []struct {
		name, directive, child      string
		wantNamespace, wantLocation string
	}{
		{"include", `<xs:include schemaLocation=" part%ZZ.xsd "/>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="child"/></xs:schema>`, "", " part%ZZ.xsd "},
		{"import namespace", `<xs:import namespace="urn:%ZZ" schemaLocation="child.xsd"/>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:%ZZ"><xs:element name="child"/></xs:schema>`, "urn:%ZZ", "child.xsd"},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + test.directive + `<xs:element name="root"/></xs:schema>`
				resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{test.wantLocation: {id: "child.xsd", contents: test.child}}}
				schema, err := parseAnyURIPublic(t, root, resolver, policy)
				if policy == Strict10 {
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					attribute := `schemaLocation=`
					if test.name == "import namespace" {
						attribute = `namespace=`
					}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, attribute) || diagnostic.SpecRef() != "xsd10-datatypes#anyURI-lexical-representation" || len(resolver.calls) != 0 {
						t.Fatalf("Strict10 discovery exit = %v, calls = %#v", err, resolver.calls)
					}
					return
				}
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				if len(schema.Documents()) != 2 || len(schema.Components()) != 2 || schema.Components()[0].Document() != "root.xsd" || schema.Components()[1].Document() != "child.xsd" {
					t.Fatalf("discovery facts = %#v, %#v", schema.Documents(), schema.Components())
				}
				if want := []discoveryCall{{namespaceURN: test.wantNamespace, location: test.wantLocation}}; !reflect.DeepEqual(resolver.calls, want) {
					t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
				}
			})
		}
	}
}

func TestSchemaAnyURIResolverFailureAfterLexicalAdmission(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="%ZZ"/></xs:schema>`
	cause := errors.New("fixture resolver rejected opaque location")
	resolver := &discoveryResolver{failures: map[string]error{"%ZZ": cause}}
	schema, err := parseAnyURIPublic(t, root, resolver, Strict11)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureResolution || diagnostic.Code() != SourceResolveCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, `<xs:include`) || !errors.Is(err, cause) || !reflect.DeepEqual(resolver.calls, []discoveryCall{{location: "%ZZ"}}) {
		t.Fatalf("resolver diagnostic = %v, calls = %#v", err, resolver.calls)
	}
}

func TestSchemaAnyURIImportNamespaceMismatchAfterResolution(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:%ZZ" schemaLocation="child.xsd"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"/>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}}}
			schema, err := parseAnyURIPublic(t, root, resolver, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, `<xs:import`) || diagnostic.SpecRef() != "" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil {
				t.Fatalf("post-resolution mismatch = %v", err)
			}
			if want := []discoveryCall{{namespaceURN: "urn:%ZZ", location: "child.xsd"}}; !reflect.DeepEqual(resolver.calls, want) {
				t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
			}
		})
	}
}

func TestSchemaAnyURIIdentityXPathPolicyPrecedence(t *testing.T) {
	root := identityRoot(`<xs:element name="item" type="xs:integer"><xs:key name="k"><xs:selector xpathDefaultNamespace="%ZZ" xpath="."/><xs:field xpath="@id"/></xs:key></xs:element>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		t.Run(string(policy), func(t *testing.T) {
			assertAnyURIIdentityXPathPolicy(t, root, policy)
		})
	}
}

func assertAnyURIIdentityXPathPolicy(t *testing.T, root string, policy LanguagePolicy) {
	t.Helper()
	schema, err := parseAnyURIPublic(t, root, &discoveryResolver{}, policy)
	if policy == Strict10 {
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, `xpathDefaultNamespace=`) || diagnostic.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint") || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("Strict10 identity policy precedence = %v", err)
		}
		return
	}
	if err != nil || len(schema.Components()) != 1 {
		t.Fatalf("identity admission = %v, components = %d", err, len(schema.Components()))
	}
	element, ok := schema.Components()[0].ElementDeclaration()
	if !ok || len(element.IdentityConstraints()) != 1 || element.IdentityConstraints()[0].Selector().DefaultNamespace() != "%ZZ" {
		t.Fatalf("selector default namespace was not retained: %#v", schema.Components())
	}
}

func TestSchemaAnyURIDiscoveryCyclePreservesOrderAndChameleonNamespace(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="%ZZ"/><xs:element name="root"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="root.xsd"/><xs:element name="child"/></xs:schema>`
	resolver := &discoveryResolver{fixtures: map[string]discoveryFixture{
		"%ZZ":      {id: "child.xsd", contents: child},
		"root.xsd": {id: "root.xsd", contents: root},
	}}
	schema, err := parseAnyURIPublic(t, root, resolver, Strict11)
	if err != nil {
		t.Fatalf("cyclic chameleon graph: %v", err)
	}
	if len(schema.Documents()) != 2 || len(schema.Components()) != 2 || schema.Components()[0].Name() != mustTestQName(t, "urn:root", "root") || schema.Components()[1].Name() != mustTestQName(t, "urn:root", "child") {
		t.Fatalf("graph order/names = %#v, %#v", schema.Documents(), schema.Components())
	}
	want := []discoveryCall{{location: "%ZZ"}, {contextSource: "child.xsd", location: "root.xsd"}}
	if !reflect.DeepEqual(resolver.calls, want) {
		t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
	}
}

type anyURISyntaxCase struct {
	name, root, attribute string
	components            int
}

func TestSchemaAnyURISharedSyntaxCallers(t *testing.T) {
	for _, test := range []anyURISyntaxCase{
		{"root namespace", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="%ZZ"><xs:element name="e"/></xs:schema>`, `targetNamespace=`, 1},
		{"documentation source", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:documentation source="%ZZ"/></xs:annotation></xs:schema>`, `source=`, 0},
		{"appinfo source", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:annotation><xs:appinfo source="%ZZ"/></xs:annotation></xs:schema>`, `source=`, 0},
		{"XPath default namespace", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="%ZZ"/>`, `xpathDefaultNamespace=`, 0},
		{"element wildcard namespace", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="T"><xs:sequence><xs:any namespace="%ZZ"/></xs:sequence></xs:complexType></xs:schema>`, `namespace=`, 1},
		{"element wildcard notNamespace", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="T"><xs:sequence><xs:any notNamespace="%ZZ"/></xs:sequence></xs:complexType></xs:schema>`, `notNamespace=`, 1},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				assertAnyURISyntaxCase(t, test, policy)
			})
		}
	}
}

func assertAnyURISyntaxCase(t *testing.T, test anyURISyntaxCase, policy LanguagePolicy) {
	t.Helper()
	resolver := &discoveryResolver{}
	schema, err := parseAnyURIPublic(t, test.root, resolver, policy)
	loc := mustSchemaTokenLoc(t, "root.xsd", test.root, 1, test.attribute)
	if policy == Strict10 {
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != loc || diagnostic.SpecRef() != "xsd10-datatypes#anyURI-lexical-representation" || len(diagnostic.Related()) != 0 {
			t.Fatalf("Strict10 lexical diagnostic = %v", err)
		}
		return
	}
	if err != nil || len(schema.Documents()) != 1 || len(schema.Components()) != test.components || len(resolver.calls) != 0 {
		t.Fatalf("admission = %v, documents/components/calls = %d/%d/%d", err, len(schema.Documents()), len(schema.Components()), len(resolver.calls))
	}
	if test.name == "root namespace" && schema.Documents()[0].TargetNamespace() != "%ZZ" {
		t.Fatalf("copied target namespace = %q", schema.Documents()[0].TargetNamespace())
	}
	if test.name == "element wildcard namespace" {
		assertAnyURIWildcardFacts(t, schema, loc)
	}
}

func assertAnyURIWildcardFacts(t *testing.T, schema Schema, loc Loc) {
	t.Helper()
	definition, ok := schema.Components()[0].ComplexType()
	if !ok {
		t.Fatal("complex type absent")
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok || len(sequence.Particles()) != 1 {
		t.Fatalf("sequence particle = %#v", definition.Particle())
	}
	wildcard, ok := sequence.Particles()[0].(WildcardParticle)
	if !ok {
		t.Fatalf("wildcard particle = %#v", sequence.Particles()[0])
	}
	constraint := wildcard.NamespaceConstraint()
	if !reflect.DeepEqual(constraint.Namespaces(), []string{"%ZZ"}) || constraint.LexicalForm() != "%ZZ" || constraint.Loc() != loc || wildcard.NamespaceLoc() != loc || wildcard.MinOccurs() != 1 || wildcard.MaxOccurs() != 1 {
		t.Fatalf("wildcard copied facts = %#v, %#v", wildcard, constraint)
	}
}

func TestSchemaAnyURIAttributeWildcardUnsupportedAfterLexicalAdmission(t *testing.T) {
	for _, attribute := range []string{`namespace="%ZZ"`, `notNamespace="%ZZ"`} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
			t.Run(attribute+"/"+string(policy), func(t *testing.T) {
				assertAnyURIAttributeWildcardExit(t, attribute, policy)
			})
		}
	}
}

func assertAnyURIAttributeWildcardExit(t *testing.T, attribute string, policy LanguagePolicy) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="T"><xs:anyAttribute ` + attribute + `/></xs:complexType></xs:schema>`
	schema, err := parseAnyURIPublic(t, root, &discoveryResolver{}, policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if policy == Strict10 {
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, attribute) || diagnostic.SpecRef() != "xsd10-datatypes#anyURI-lexical-representation" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil {
			t.Fatalf("Strict10 attribute wildcard lexical error = %v", err)
		}
		return
	}
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, `<xs:anyAttribute`) || diagnostic.SpecRef() != schemaAnyAttributeSpecRef(XSDVersion11) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaAnyAttributeUnsupported) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("attribute wildcard unsupported exit = %v", err)
	}
}

type anyURIAlternateCase struct {
	name, root, marker, code, specRef string
	class                             FailureClass
}

func TestSchemaAnyURIAlternateSyntaxExits(t *testing.T) {
	for _, test := range []anyURIAlternateCase{
		{"redefine unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:redefine schemaLocation="%ZZ"/></xs:schema>`, `<xs:redefine`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", FailureUnsupported},
		{"override unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:override schemaLocation="%ZZ"/></xs:schema>`, `<xs:override`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", FailureUnsupported},
		{"XML Base supported lexeme unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="base path"/>`, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported},
		{"XML Base preserves trailing space before escaping", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="foo: "/>`, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported},
		{"XML Base registered name unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://%41.example/"/>`, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported},
		{"XML Base IPv6 embedded IPv4 unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://[::192.9.5.5]/"/>`, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported},
		{"XML Base IPv6 leading zero unsupported", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://[::192.009.5.5]/"/>`, `xml:base=`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported},
		{"XML Base malformed percent invalid", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="%ZZ"/>`, `xml:base=`, invalidSchemaCompositionCode, "xml-base-2001#syntax", FailureInvalid},
		{"XML Base stray authority bracket invalid", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://abc]/"/>`, `xml:base=`, invalidSchemaCompositionCode, "xml-base-2001#syntax", FailureInvalid},
		{"XML Base IPv6 host suffix invalid", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://[::1]extra/"/>`, `xml:base=`, invalidSchemaCompositionCode, "xml-base-2001#syntax", FailureInvalid},
		{"XML Base invalid IPv6 invalid", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="http://[1::2::3]/"/>`, `xml:base=`, invalidSchemaCompositionCode, "xml-base-2001#syntax", FailureInvalid},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				assertAnyURIAlternateCase(t, test, policy)
			})
		}
	}
}

func assertAnyURIAlternateCase(t *testing.T, test anyURIAlternateCase, policy LanguagePolicy) {
	t.Helper()
	resolver := &discoveryResolver{}
	schema, err := parseAnyURIPublic(t, test.root, resolver, policy)
	assertZeroSchema(t, schema)
	if len(resolver.calls) != 0 {
		t.Fatalf("invalid or unsupported XML Base unexpectedly resolved: %#v", resolver.calls)
	}
	diagnostic := requireDiagnostic(t, err)
	marker := test.marker
	code, specRef, class := test.code, test.specRef, test.class
	if policy == Strict10 && (test.name == "redefine unsupported" || test.name == "override unsupported") {
		marker = `schemaLocation=`
		code, specRef, class = invalidSchemaCompositionCode, "xsd10-datatypes#anyURI-lexical-representation", FailureInvalid
	}
	if diagnostic.Code() != code || diagnostic.Class() != class || diagnostic.SpecRef() != specRef || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", test.root, 1, marker) || len(diagnostic.Related()) != 0 {
		t.Fatalf("caller exit = %v", err)
	}
	if class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported cause lost: %v", err)
	}
	if class == FailureInvalid && errors.Is(err, ErrUnsupported) {
		t.Fatalf("invalid classified as unsupported: %v", err)
	}
	if class == FailureInvalid && diagnostic.Unwrap() != nil {
		t.Fatalf("invalid lexical diagnostic has unexpected cause: %v", diagnostic.Unwrap())
	}
}

func TestSchemaAnyURICallerAndXMLBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, root, token, code, specRef string
		class                            FailureClass
	}{
		{"empty target namespace", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace=""/>`, `targetNamespace=`, invalidSchemaTargetNamespaceCode, "", FailureInvalid},
		{"wildcard keyword", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="T"><xs:sequence><xs:any namespace="##%ZZ"/></xs:sequence></xs:complexType></xs:schema>`, `namespace=`, invalidSchemaCompositionCode, "", FailureInvalid},
		{"XML Base legacy URI", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xml:base="%ZZ"/>`, `xml:base=`, invalidSchemaCompositionCode, "xml-base-2001#syntax", FailureInvalid},
		{"invalid XML character", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" public="p" system="&#0;"/></xs:schema>`, `&#0;`, "XSD3001", "", FailureInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := parseAnyURIPublic(t, test.root, &discoveryResolver{}, Strict11)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic = %v", err)
			}
			want := mustSchemaTokenLoc(t, "root.xsd", test.root, 1, test.token)
			if test.name == "invalid XML character" {
				want = mustTestLoc(t, "root.xsd", 1, 56)
			}
			if diagnostic.Loc() != want {
				t.Fatalf("location = %s, want %s", diagnostic.Loc(), want)
			}
		})
	}
}

//nolint:gocognit // The edition and local-use shapes share one diagnostic boundary.
func TestSchemaAnyURILocalAttributeShapesAndCallerRules(t *testing.T) {
	for _, test := range []struct {
		name, attribute, tail, marker string
		ref                           bool
	}{
		{"direct", `<xs:attribute name="a" targetNamespace="%ZZ" type="xs:integer"/>`, "", `targetNamespace="%ZZ"`, false},
		{"named", `<xs:attribute name="a" targetNamespace="%ZZ" type="r:Int"/>`, `<xs:simpleType name="Int"><xs:restriction base="xs:integer"/></xs:simpleType>`, `targetNamespace="%ZZ"`, false},
		{"inline", `<xs:attribute name="a" targetNamespace="%ZZ"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>`, "", `targetNamespace="%ZZ"`, false},
		{"ref", `<xs:attribute ref="r:a" targetNamespace="%ZZ"/>`, "", `ref="r:a"`, true},
	} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
			t.Run(test.name+"/"+string(policy), func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:complexType name="T">` + test.attribute + `</xs:complexType>` + test.tail + `</xs:schema>`
				schema, err := parseAnyURIPublic(t, root, &discoveryResolver{}, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("local attribute diagnostic = %v", err)
				}
				if policy == Strict10 {
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, `targetNamespace="%ZZ"`) || diagnostic.SpecRef() != "xsd10-datatypes#anyURI-lexical-representation" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil {
						t.Fatalf("Strict10 local attribute lexical precedence = %v", err)
					}
					return
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, test.marker) {
					t.Fatalf("local attribute location = %s", diagnostic.Loc())
				}
				if test.ref {
					if diagnostic.SpecRef() != "xsd11-structures#AU_details" || len(diagnostic.Related()) != 0 || diagnostic.Unwrap() != nil {
						t.Fatalf("reference combination diagnostic = %v, spec = %q, related = %#v, cause = %v", err, diagnostic.SpecRef(), diagnostic.Related(), diagnostic.Unwrap())
					}
					return
				}
				wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 1, `targetNamespace="urn:root"`)}
				if diagnostic.SpecRef() != "xsd11-structures#AU_details" || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaAttributeTargetNamespace) {
					t.Fatalf("local attribute namespace mismatch = %v, related = %#v", err, diagnostic.Related())
				}
			})
		}
	}
}

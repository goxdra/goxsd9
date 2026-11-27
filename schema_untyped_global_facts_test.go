package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // The full authored Boolean matrix shares one public declaration contract.
func TestUntypedGlobalBooleanFactsAndLocations(t *testing.T) {
	values := []struct {
		name, lexical string
		value         bool
	}{
		{name: "omitted"},
		{name: "false", lexical: "0"},
		{name: "true", lexical: "true", value: true},
	}
	for _, policy := range untypedGlobalPolicies() {
		for _, abstract := range values {
			for _, nillable := range values {
				t.Run(policy.name+"/"+abstract.name+"/"+nillable.name, func(t *testing.T) {
					attributes := ""
					if abstract.lexical != "" {
						attributes += ` abstract="` + abstract.lexical + `"`
					}
					if nillable.lexical != "" {
						attributes += ` nillable="` + nillable.lexical + `"`
					}
					root := untypedGlobalRoot(attributes)
					first, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("first parse: %v", err)
					}
					second, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("second parse: %v", err)
					}
					if !reflect.DeepEqual(untypedGlobalFacts(t, first), untypedGlobalFacts(t, second)) {
						t.Fatal("repeated builds changed component identities or walk order")
					}
					components := first.Components()
					if len(components) != 3 || components[1].ID().Ordinal() != 2 || components[1].Name() != mustTestQName(t, "urn:root", "item") {
						t.Fatalf("global component identity/order = %v", components)
					}
					declaration, ok := components[1].ElementDeclaration()
					alias, aliasOK := components[1].Element()
					if !ok || !aliasOK || alias.ID() != declaration.ID() || !declaration.IsUntyped() || declaration.Component().ID() != components[1].ID() || declaration.Loc() != components[1].Loc() {
						t.Fatalf("untyped declaration identity = %#v/%t, alias = %#v/%t", declaration, ok, alias, aliasOK)
					}
					if declaration.IsAbstract() != abstract.value || declaration.IsNillable() != nillable.value {
						t.Fatalf("effective Boolean facts = %t/%t", declaration.IsAbstract(), declaration.IsNillable())
					}
					wantAbstractLoc, wantNillableLoc := Loc{}, Loc{}
					if abstract.lexical != "" {
						wantAbstractLoc = schemaMixedComplexLoc(t, root, `abstract="`+abstract.lexical+`"`)
					}
					if nillable.lexical != "" {
						wantNillableLoc = schemaMixedComplexLoc(t, root, `nillable="`+nillable.lexical+`"`)
					}
					if declaration.AbstractLoc() != wantAbstractLoc || declaration.NillableLoc() != wantNillableLoc {
						t.Fatalf("Boolean locations = %v/%v, want %v/%v", declaration.AbstractLoc(), declaration.NillableLoc(), wantAbstractLoc, wantNillableLoc)
					}
					if !declaration.DeclaredType().IsZero() || !declaration.DisallowedSubstitutionsLoc().IsZero() || len(declaration.DisallowedSubstitutions()) != 0 || len(declaration.SubstitutionGroupAffiliations()) != 0 || len(declaration.SubstitutionGroupAffiliationLocations()) != 0 {
						t.Fatal("untyped declaration acquired authored type, block, or affiliation facts")
					}
					if id, hasID := declaration.TypeID(); hasID || !id.IsZero() {
						t.Fatalf("synthetic type identity = %v/%t", id, hasID)
					}
					if _, hasRef := declaration.TypeReference(); hasRef {
						t.Fatal("untyped declaration acquired a type reference")
					}
					if _, hasInline := declaration.InlineSimpleType(); hasInline {
						t.Fatal("untyped declaration acquired an inline simple type")
					}
					if _, hasInline := declaration.InlineComplexType(); hasInline {
						t.Fatal("untyped declaration acquired an inline complex type")
					}
					var zero ElementDeclaration
					zeroID, zeroHasID := zero.TypeID()
					_, zeroHasRef := zero.TypeReference()
					_, zeroHasSimple := zero.InlineSimpleType()
					_, zeroHasComplex := zero.InlineComplexType()
					if zero.IsUntyped() || zero.IsAbstract() || zero.IsNillable() || !zero.AbstractLoc().IsZero() || !zero.NillableLoc().IsZero() || !zero.DeclaredType().IsZero() || zeroHasID || !zeroID.IsZero() || zeroHasRef || zeroHasSimple || zeroHasComplex || len(zero.DisallowedSubstitutions()) != 0 || !zero.DisallowedSubstitutionsLoc().IsZero() || len(zero.SubstitutionGroupAffiliations()) != 0 || len(zero.SubstitutionGroupAffiliationLocations()) != 0 || len(zero.IdentityConstraints()) != 0 {
						t.Fatal("zero declaration query is not defensive")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Repeated graph builds, adoption, and exact source locations share one contract.
func TestUntypedGlobalTrueFlagsRemainVisibleInGraphs(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root, fixtures := untypedGlobalGraph(` abstract="1" nillable="true"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
			if err != nil {
				t.Fatalf("graph parse: %v", err)
			}
			facts := untypedGlobalFacts(t, schema)
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
			if err != nil || !reflect.DeepEqual(facts, untypedGlobalFacts(t, again)) {
				t.Fatalf("repeated graph changed identity or order: %v", err)
			}
			if len(facts.documents) != 4 || facts.documents[2].namespace != "urn:root" || facts.documents[3].namespace != "urn:other" {
				t.Fatalf("graph discovery/adoption = %#v", facts.documents)
			}
			for _, name := range []struct{ namespace, local string }{
				{"urn:root", "item"}, {"urn:root", "included"}, {"urn:root", "chameleon"}, {"urn:other", "imported"},
			} {
				matches := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, name.namespace, name.local))
				if len(matches) != 1 {
					t.Fatalf("FindKind(%s:%s) = %v", name.namespace, name.local, matches)
				}
				declaration, ok := matches[0].ElementDeclaration()
				if !ok || !declaration.IsUntyped() || !declaration.IsAbstract() || !declaration.IsNillable() || declaration.AbstractLoc().IsZero() || declaration.NillableLoc().IsZero() {
					t.Fatalf("graph declaration %s = %#v/%t", name.local, declaration, ok)
				}
				written := fixtures[string(declaration.ID().Source())].contents
				if declaration.AbstractLoc() != schemaBlockTestAttributeLoc(t, declaration.ID().Source(), written, `abstract="1"`) ||
					declaration.NillableLoc() != schemaBlockTestAttributeLoc(t, declaration.ID().Source(), written, `nillable="true"`) {
					t.Fatalf("graph %s Boolean locations = %v/%v", name.local, declaration.AbstractLoc(), declaration.NillableLoc())
				}
			}
		})
	}
}

func TestUntypedGlobalTrueConsumerRootDiagnostics(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		root := untypedGlobalRoot(` abstract="true" nillable="1"`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
		if err != nil {
			t.Fatalf("%s parse: %v", policy.name, err)
		}
		declaration, ok := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "item"))[0].ElementDeclaration()
		if !ok {
			t.Fatal("missing untyped view")
		}
		instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:root"/>`)))
		diagnostic := requireDiagnostic(t, instanceErr)
		version := instanceSchemaValidationVersion(schema)
		wantInstanceSpec := requireDiagnostic(t, newInstanceValidationUnsupported(Loc{}, "", nil, version, errInstanceNoDeclaredType)).SpecRef()
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || diagnostic.SpecRef() != wantInstanceSpec || !reflect.DeepEqual(diagnostic.Related(), []Loc{declaration.Loc()}) || !errors.Is(instanceErr, errInstanceNoDeclaredType) {
			t.Fatalf("%s validation = %s", policy.name, diagnostic)
		}
		output, generationErr := GenerateGo(schema, "generated")
		generation := requireDiagnostic(t, generationErr)
		if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != declaration.Loc() || generation.SpecRef() != schemaElementTypeSpecRef(version) || !errors.Is(generationErr, errCodegenUnsupported) {
			t.Fatalf("%s generation = %q/%s", policy.name, output, generation)
		}
	}
}

func TestUntypedGlobalTrueFlagsPreserveOwnerExclusions(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		version := XSDVersion11
		if policy.policy == Strict10 {
			version = XSDVersion10
		}
		cases := []struct {
			name, element, marker, code, spec string
			cause                             error
		}{
			{
				name: "substitution", element: `<xs:element name="item" abstract="true" nillable="1" substitutionGroup="t:before"/>`,
				marker: `substitutionGroup="t:before"`, code: UnsupportedSchemaSyntaxCode,
				spec: newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef(), cause: ErrUnsupported,
			},
			{
				name: "nonempty block", element: `<xs:element name="item" abstract="true" nillable="1" block="extension"/>`,
				marker: `block="extension"`, code: UnsupportedSchemaSyntaxCode,
				spec: newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef(), cause: ErrUnsupported,
			},
			{
				name: "identity constraint", element: `<xs:element name="item" abstract="true" nillable="1"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element>`,
				marker: `<xs:key`, code: diagnosticSchemaIdentityUnsupportedCode,
				spec: schemaIdentitySpecRef(version, "src-identity-constraint"), cause: errSchemaIdentityUnsupported,
			},
		}
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:element name="before" type="xs:string"/>` + test.element + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("excluded owner returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != schemaMixedComplexLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("excluded owner diagnostic = %s, cause %v", d, err)
				}
			})
		}
	}
}

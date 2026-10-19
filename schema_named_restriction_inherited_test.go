package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func inheritedRestrictionGraph(version XSDVersion, baseNamespace, ownerNamespace, derivedNamespace string, throughExtension, inline bool) (string, string) {
	baseQName := "Base"
	if baseNamespace != "" {
		baseQName = "b:Base"
	}
	derivedBase := baseQName
	middle := ""
	if throughExtension {
		derivedBase = "t:Middle"
		if ownerNamespace == "" {
			derivedBase = "Middle"
		}
		middle = `<xs:complexType name="Middle"><xs:complexContent><xs:extension base="` + baseQName + `"/></xs:complexContent></xs:complexType>`
	}
	wildcard := namedRestrictionWildcard(derivedNamespace, "lax")
	derived := namedRestrictionOwner(derivedBase, wildcard)
	rootType := "Derived"
	if ownerNamespace != "" {
		rootType = "t:Derived"
	}
	element := `<xs:element name="root" type="` + rootType + `"/>`
	if inline {
		element = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="` + derivedBase + `">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
		derived = ""
	}
	importNamespace := ""
	if baseNamespace != "" {
		importNamespace = ` namespace="` + baseNamespace + `"`
	}
	ownerTarget := ""
	if ownerNamespace != "" {
		ownerTarget = ` targetNamespace="` + ownerNamespace + `"`
	}
	baseTarget := ""
	if baseNamespace != "" {
		baseTarget = ` targetNamespace="` + baseNamespace + `"`
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" xmlns:b="urn:base"` + ownerTarget + ` version="` + string(version) + `">` +
		`<xs:import` + importNamespace + ` schemaLocation="base.xsd"/>` + element + derived + middle + `</xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + baseTarget + ` version="` + string(version) + `">` + namedRestrictionBase() + `</xs:schema>`
	return root, base
}

func parseInheritedRestrictionGraph(t *testing.T, root, base string, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	return discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"base.xsd": {id: "base.xsd", contents: base},
	}, policy)
}

func inheritedRestrictionDefinition(t *testing.T, schema Schema, inline bool) ComplexTypeDefinition {
	t.Helper()
	for _, component := range schema.Components() {
		if !inline && component.Name().Local() == "Derived" {
			definition, ok := component.ComplexType()
			if !ok {
				t.Fatal("named Derived is not a complex type")
			}
			return definition
		}
		if !inline || component.Name().Local() != "root" {
			continue
		}
		declaration, ok := component.ElementDeclaration()
		if !ok {
			t.Fatal("root is not an element")
		}
		definition, ok := declaration.InlineComplexType()
		if !ok {
			t.Fatal("root has no inline complex type")
		}
		return definition
	}
	t.Fatal("Derived is absent")
	return ComplexTypeDefinition{}
}

func inheritedRestrictionComponent(t *testing.T, schema Schema, local string) Component {
	t.Helper()
	for _, component := range schema.Components() {
		if component.Name().Local() == local {
			return component
		}
	}
	t.Fatalf("component %q is absent", local)
	return Component{}
}

func restrictionFactVersion(profile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}) XSDVersion {
	if profile.policy == Compatibility {
		return XSDVersion11
	}
	return profile.version
}

func assertInheritedRestrictionSubsetDiagnostic(t *testing.T, root, base, derivedNamespace string, profile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}, schema Schema, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("broader inherited wildcard parsed")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	primary := namedRestrictionLastLoc(t, root, `namespace="`+derivedNamespace+`"`)
	related := []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `namespace="##other"`)}
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
		diagnostic.Loc() != primary || !reflect.DeepEqual(diagnostic.Related(), related) ||
		diagnostic.SpecRef() != schemaComplexTypeDerivationSpecRef(restrictionFactVersion(profile)) ||
		!errors.Is(err, errSchemaComplexTypeWildcardRestriction) {
		t.Fatalf("inherited subset diagnostic = %v", err)
	}
}

//nolint:gocognit // Policy, owner shape, and accepted/rejected namespace sets form one public provenance matrix.
func TestInheritedOtherWildcardRestrictionKeepsOriginalNamespace(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		for _, inline := range []bool{false, true} {
			for _, tc := range []struct {
				name, namespace string
				valid           bool
			}{
				{"other broadens", "##other", false},
				{"original namespace excluded", "urn:base", false},
				{"mixed original namespace excluded", "urn:n urn:base", false},
				{"absent namespace excluded", "##local", false},
				{"middle namespace allowed", "urn:n", true},
				{"middle and outside allowed", "urn:n urn:outside", true},
			} {
				shape := "named"
				if inline {
					shape = "inline"
				}
				t.Run(profile.name+"/"+shape+"/"+tc.name, func(t *testing.T) {
					root, base := inheritedRestrictionGraph(profile.version, "urn:base", "urn:n", tc.namespace, true, inline)
					schema, err := parseInheritedRestrictionGraph(t, root, base, profile.policy)
					if !tc.valid {
						assertInheritedRestrictionSubsetDiagnostic(t, root, base, tc.namespace, profile, schema, err)
						return
					}
					if err != nil {
						t.Fatalf("valid inherited subset: %v", err)
					}
					derived := inheritedRestrictionDefinition(t, schema, inline)
					middleComponent := inheritedRestrictionComponent(t, schema, "Middle")
					middle, ok := middleComponent.ComplexType()
					if !ok || middle.Particle() != nil || len(middle.AttributeUses()) != 0 {
						t.Fatalf("completed Middle = %v/%v", middle, ok)
					}
					baseComponent := inheritedRestrictionComponent(t, schema, "Base")
					baseReference, hasBase := middle.BaseReference()
					baseID, hasBaseID := baseReference.ComponentID()
					if !hasBase || !hasBaseID || baseID != baseComponent.ID() {
						t.Fatalf("Middle base = %v/%v", baseReference, hasBase)
					}
					middleWildcard, hasWildcard := middle.AnyAttribute()
					baseWildcardLoc := complexContentTestSourceLoc(t, "base.xsd", base, `<xs:anyAttribute`)
					baseNamespaceLoc := complexContentTestSourceLoc(t, "base.xsd", base, `namespace="##other"`)
					if !hasWildcard || middleWildcard.Loc() != baseWildcardLoc || middleWildcard.NamespaceLoc() != baseNamespaceLoc ||
						middleWildcard.Namespace() != "##other" || middleWildcard.ProcessContents() != "lax" {
						t.Fatalf("inherited wildcard = %v/%v", middleWildcard, hasWildcard)
					}
					middleReference, hasMiddle := derived.BaseReference()
					middleID, hasMiddleID := middleReference.ComponentID()
					derivedWildcard, hasDerivedWildcard := derived.AnyAttribute()
					if !hasMiddle || !hasMiddleID || middleID != middleComponent.ID() ||
						middleReference.Loc() != complexContentTestLoc(t, root, `base="t:Middle"`) ||
						derived.Particle() != nil || len(derived.AttributeUses()) != 0 ||
						!hasDerivedWildcard || derivedWildcard.Namespace() != tc.namespace ||
						derivedWildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="`+tc.namespace+`"`) ||
						!reflect.DeepEqual(derivedWildcard.NamespaceConstraint().Namespaces(), strings.Split(tc.namespace, " ")) {
						t.Fatalf("derived facts = %v/%v", derived, derivedWildcard)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Both directions are checked against all policy/owner combinations.
func TestOtherWildcardRestrictionNoNamespaceImportPolicy(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		for _, inline := range []bool{false, true} {
			for _, throughExtension := range []bool{false, true} {
				for _, tc := range []struct {
					name, baseNamespace, ownerNamespace string
					valid11                             bool
				}{
					{"absent base to named", "", "urn:n", true},
					{"named base to absent", "urn:base", "", false},
				} {
					shape := "named"
					if inline {
						shape = "inline"
					}
					path := "direct"
					if throughExtension {
						path = "inherited"
					}
					t.Run(profile.name+"/"+shape+"/"+path+"/"+tc.name, func(t *testing.T) {
						root, base := inheritedRestrictionGraph(profile.version, tc.baseNamespace, tc.ownerNamespace, "##other", throughExtension, inline)
						schema, err := parseInheritedRestrictionGraph(t, root, base, profile.policy)
						if !tc.valid11 || profile.policy == Strict10 {
							assertInheritedRestrictionSubsetDiagnostic(t, root, base, "##other", profile, schema, err)
							return
						}
						if err != nil {
							t.Fatalf("XSD 1.1 exclusion-set subset: %v", err)
						}
						derived := inheritedRestrictionDefinition(t, schema, inline)
						baseComponent := inheritedRestrictionComponent(t, schema, "Base")
						baseReferenceName := "Base"
						if throughExtension {
							baseComponent = inheritedRestrictionComponent(t, schema, "Middle")
							baseReferenceName = "t:Middle"
						}
						reference, hasReference := derived.BaseReference()
						id, hasID := reference.ComponentID()
						wildcard, hasWildcard := derived.AnyAttribute()
						if !hasReference || !hasID || id != baseComponent.ID() ||
							reference.Loc() != complexContentTestLoc(t, root, `base="`+baseReferenceName+`"`) ||
							!hasWildcard || wildcard.Namespace() != "##other" ||
							wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) {
							t.Fatalf("published no-namespace base = %v/%v", reference, wildcard)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // This admitted extension/restriction/restriction chain proves source order and copied identity.
func TestInheritedOtherWildcardRestrictionTransitiveAdmittedChain(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		for _, inline := range []bool{false, true} {
			shape := "named"
			if inline {
				shape = "inline"
			}
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				root, base := inheritedRestrictionGraph(profile.version, "urn:base", "urn:n", "urn:n", true, false)
				root = strings.Replace(root, `<xs:element name="root" type="t:Derived"/>`, `<xs:element name="root" type="t:Final"/>`, 1)
				final := namedRestrictionOwner("t:Derived", namedRestrictionWildcard("urn:n", "lax"))
				final = strings.Replace(final, `name="Derived"`, `name="Final"`, 1)
				if inline {
					final = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Derived">` + namedRestrictionWildcard("urn:n", "lax") + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
					root = strings.Replace(root, `<xs:element name="root" type="t:Final"/>`, "", 1)
				}
				root = strings.Replace(root, `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`, `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`+final, 1)
				schema, err := parseInheritedRestrictionGraph(t, root, base, profile.policy)
				if err != nil {
					t.Fatalf("transitive admitted chain: %v", err)
				}
				derivedComponent := inheritedRestrictionComponent(t, schema, "Derived")
				var finalDefinition ComplexTypeDefinition
				if inline {
					finalDefinition = inheritedRestrictionDefinition(t, schema, true)
				}
				if !inline {
					finalComponent := inheritedRestrictionComponent(t, schema, "Final")
					var ok bool
					finalDefinition, ok = finalComponent.ComplexType()
					if !ok {
						t.Fatal("Final is not a complex type")
					}
				}
				reference, hasReference := finalDefinition.BaseReference()
				id, hasID := reference.ComponentID()
				wildcard, hasWildcard := finalDefinition.AnyAttribute()
				if !hasReference || !hasID || id != derivedComponent.ID() ||
					reference.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) ||
					finalDefinition.Particle() != nil || len(finalDefinition.AttributeUses()) != 0 ||
					!hasWildcard || wildcard.Namespace() != "urn:n" {
					t.Fatalf("Final chain facts = %v/%v", reference, wildcard)
				}
			})
		}
	}
}

func TestInheritedOtherWildcardExtensionOfExtensionRemainsUnsupported(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		root, base := inheritedRestrictionGraph(profile.version, "urn:base", "urn:n", "urn:n", true, false)
		root = strings.Replace(root, `base="t:Middle"`, `base="t:Upper"`, 1)
		upper := `<xs:complexType name="Upper"><xs:complexContent><xs:extension base="t:Middle"/></xs:complexContent></xs:complexType>`
		root = strings.Replace(root, `</xs:schema>`, upper+`</xs:schema>`, 1)
		t.Run(profile.name, func(t *testing.T) {
			schema, err := parseInheritedRestrictionGraph(t, root, base, profile.policy)
			if err == nil {
				t.Fatal("extension of extension parsed")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != namedRestrictionLastLoc(t, root, `base="t:Middle"`) ||
				!reflect.DeepEqual(diagnostic.Related(), []Loc{
					complexContentTestLoc(t, root, `<xs:complexType name="Middle"`),
					complexContentTestLoc(t, root, `<xs:extension base="b:Base"`),
				}) ||
				diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(restrictionFactVersion(profile)) ||
				!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("extension-chain exclusion = %v", err)
			}
		})
	}
}

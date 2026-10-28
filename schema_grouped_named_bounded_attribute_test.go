package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedNamedBoundedGraph(f boundedLocalFamily, version XSDVersion) (string, map[string]discoveryFixture) {
	root, fixtures := boundedLocalGraph(f, version)
	root = strings.Replace(root, `<xs:complexType name="Box"><xs:attribute`, `<xs:complexType name="Box"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="2" maxOccurs="3"/><xs:attribute`, 1)
	root = strings.Replace(root, `</xs:complexType><xs:simpleType name="Child">`, `</xs:extension></xs:complexContent></xs:complexType><xs:group name="Fields"><xs:sequence/></xs:group><xs:simpleType name="Child">`, 1)
	other := fixtures["other.xsd"]
	other.contents = strings.Replace(other.contents, `</xs:schema>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType></xs:schema>`, 1)
	fixtures["other.xsd"] = other
	primary := fixtures["root.xsd"]
	primary.contents = root
	fixtures["root.xsd"] = primary
	return root, fixtures
}

//nolint:gocognit,funlen // The public graph checks named chains, copied facts, identity, order, and provenance together.
func TestGroupedNamedBoundedGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root, fixtures := groupedNamedBoundedGraph(family, profile.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeated components = %v", err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 4 {
					t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
				}
				for index, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
					if schema.Documents()[index].Source() != source {
						t.Fatalf("document %d = %s", index, schema.Documents()[index].Source())
					}
				}
				var walks [2][]ComponentID
				for index := range walks {
					if err := schema.Walk(func(component Component) error {
						walks[index] = append(walks[index], component.ID())
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
					t.Fatalf("walk order = %v/%v", walks[0], walks[1])
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Box"))[0], "Box")
				if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Box"`) || owner.DerivationLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:extension`) || owner.BaseLoc() != elementReferenceTestAttributeLoc(t, root, `base="o:Base"`) {
					t.Fatalf("owner provenance = %#v", owner)
				}
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:other", "Base"))
				if !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base = %#v", base)
				}
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:bounded-local", "Fields"))
				if !hasGroup || group.Ref() != mustTestQName(t, "urn:bounded-local", "Fields") || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "2/3" || group.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) {
					t.Fatalf("group = %#v", owner.Particle())
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, "other.xsd", fixtures["other.xsd"].contents, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
					t.Fatalf("inherited wildcard = %#v", wildcard)
				}
				uses := owner.AttributeUses()
				if len(uses) != 6 {
					t.Fatalf("ordered uses = %v", uses)
				}
				for index, expected := range []struct{ local, lexical, namespace, source, minimum, maximum, minNeedle, maxNeedle string }{
					{"builtin", "xs:" + family.name, testXSDNamespace, "", family.minimum, family.maximum, "", ""},
					{"forward", "r:Child", "urn:bounded-local", "root.xsd", family.childMinimum, family.childMaximum, `value="` + family.childMinimum + `"`, `value="` + family.childMaximum + `"`},
					{"inherited", "r:Parent", "urn:bounded-local", "root.xsd", family.parentMinimum, family.parentMaximum, `value="` + family.parentMinimum + `"`, `value="` + family.parentMaximum + `"`},
					{"included", "r:Included", "urn:bounded-local", "included.xsd", family.minimum, family.maximum, "", ""},
					{"adopted", "r:Adopted", "urn:bounded-local", "chameleon.xsd", family.minimum, family.maximum, "", ""},
					{"foreign", "o:Foreign", "urn:other", "other.xsd", family.minimum, family.maximum, "", ""},
				} {
					use, ok := uses[index].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", index, uses[index])
					}
					declared := mustTestQName(t, expected.namespace, strings.SplitN(expected.lexical, ":", 2)[1])
					nameNS := "urn:bounded-local"
					if expected.local == "included" {
						nameNS = ""
					}
					if use.Name() != mustTestQName(t, nameNS, expected.local) || use.DeclaredType() != declared || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+expected.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+expected.local+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) {
						t.Fatalf("use %d provenance = %#v", index, use)
					}
					ref, ok := use.TypeReference()
					id, hasID := use.TypeID()
					refID, refHasID := ref.ComponentID()
					if !ok || ref.QName() != declared || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || id != refID || hasID != refHasID {
						t.Fatalf("use %d type = %#v", index, ref)
					}
					if index == 0 && (!ref.IsBuiltin() || hasID || !id.IsZero() || ref.VarietyLoc() != use.TypeLoc()) {
						t.Fatalf("built-in type = %#v", ref)
					}
					if index > 0 {
						declaration := fixtures[expected.source].contents
						variety := `<xs:restriction base="xs:` + family.name + `"`
						if index == 1 {
							variety = `<xs:restriction base="r:Parent"`
						}
						if !ref.IsNamed() || !hasID || id != componentIDForName(t, schema, declared) || id.Source() != SourceID(expected.source) || ref.VarietyLoc() != mustSchemaTokenLoc(t, SourceID(expected.source), declaration, 1, variety) {
							t.Fatalf("named type %d = %#v", index, ref)
						}
					}
					var minLoc, maxLoc Loc
					if expected.minNeedle != "" {
						minLoc = elementReferenceTestAttributeLoc(t, root, expected.minNeedle)
					}
					if expected.maxNeedle != "" {
						maxLoc = elementReferenceTestAttributeLoc(t, root, expected.maxNeedle)
					}
					assertAllIntegerBounds(t, ref, profile.version, expected.minimum, expected.maximum, minLoc, maxLoc)
				}
				assertBoundedLocalFacetProvenance(t, schema, root, profile.version)
				uses[1] = nil
				if owner.AttributeUses()[1].Name().Local() != "forward" {
					t.Fatal("use slice mutated owner")
				}
				freshUse, ok := owner.AttributeUses()[1].(LocalAttributeUse)
				if !ok {
					t.Fatalf("fresh use = %T", owner.AttributeUses()[1])
				}
				ref, ok := freshUse.TypeReference()
				if !ok {
					t.Fatal("fresh use lost type reference")
				}
				bounds, ok := ref.IntegerBounds()
				if !ok {
					t.Fatal("fresh type lost integer bounds")
				}
				for _, bound := range bounds.Bounds() {
					bound.Value().value.SetInt64(0)
				}
				assertAllIntegerBounds(t, ref, profile.version, family.childMinimum, family.childMaximum, elementReferenceTestAttributeLoc(t, root, `value="`+family.childMinimum+`"`), elementReferenceTestAttributeLoc(t, root, `value="`+family.childMaximum+`"`))
			})
		}
	}
}

//nolint:gocognit // Each newly admitted named use has independent consumer exits.
func TestGroupedNamedBoundedConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range []string{"direct", "chain"} {
				t.Run(profile.name+"/"+family.name+"/"+shape, func(t *testing.T) {
					declaration := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
					if shape == "chain" {
						declaration = `<xs:simpleType name="Alias"><xs:restriction base="t:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
					}
					root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Alias"/>`)
					root = strings.Replace(root, `</xs:schema>`, declaration+`</xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := complexContentTestLoc(t, root, `<xs:attribute name="named"`)
					ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
					rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
					instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
					vd := requireDiagnostic(t, instanceErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, errInstanceAttributes) {
						t.Fatalf("validation = %v", instanceErr)
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v", output, generationErr)
					}
				})
			}
		}
	}
}

func TestGroupedNamedBoundedUnsignedLongUpperPrecision(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="large" type="t:Large"/>`)
		root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Large"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="9223372036854775808"/><xs:maxInclusive value="18446744073709551615"/><xs:totalDigits value="20"/><xs:enumeration value="18446744073709551615"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
		if !ok {
			t.Fatalf("use = %T", owner.AttributeUses()[0])
		}
		ref, ok := use.TypeReference()
		if !ok || !ref.IsNamed() || use.DeclaredType() != mustTestQName(t, "urn:root", "Large") {
			t.Fatalf("reference = %#v", ref)
		}
		assertAllIntegerBounds(t, ref, profile.version, "9223372036854775808", "18446744073709551615", complexContentTestLoc(t, root, `value="9223372036854775808"`), complexContentTestLoc(t, root, `value="18446744073709551615"`))
		definition, ok := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Large"))[0].SimpleTypeDefinition()
		if !ok {
			t.Fatal("Large definition missing")
		}
		values := definition.IntegerEnumerationFacets().Values()
		locations := definition.IntegerEnumerationFacets().Locations()
		if len(values) != 1 || values[0].Canonical() != "18446744073709551615" || !reflect.DeepEqual(locations, []Loc{complexContentTestLoc(t, root, `<xs:enumeration value="18446744073709551615"`)}) {
			t.Fatalf("enumeration = %v/%v", values, locations)
		}
		values[0].value.SetInt64(0)
		locations[0] = Loc{}
		if definition.IntegerEnumerationFacets().Values()[0].Canonical() != "18446744073709551615" {
			t.Fatal("enumeration copy mutated schema")
		}
	}
}

func TestGroupedNamedBoundedOmissionAndDependencies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			root := groupedExtensionSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="gone" type="t:Alias" use="prohibited"/><xs:attribute name="kept" type="t:Alias"/>`)
			root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "kept" {
				t.Fatalf("omitted facts = %#v/%#v", owner.Particle(), owner.AttributeUses())
			}
			for _, tc := range []groupedLongExit{
				{"missing group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
				{"wrong group kind", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, diagnosticSchemaModelGroupReferenceWrongKindCode, FailureInvalid, errSchemaModelGroupReferenceWrongKind, schemaModelGroupReferenceSpecRef},
				{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
				{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
				{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
				{"prohibited missing type", `type="t:Alias" use="prohibited"`, `type="t:Missing" use="prohibited"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
			} {
				testGroupedLongExits(t, profile, root, []groupedLongExit{tc})
			}
		}
	}
}

func groupedNamedBoundedInheritedSchema(f boundedLocalFamily, chain, prohibited bool) string {
	use := `<xs:attribute name="bounded" type="t:Alias"/>`
	if prohibited {
		use = `<xs:attribute name="bounded" type="t:Alias" use="prohibited"/>`
	}
	derived := strings.Replace(syntheticInheritedDerived, `</xs:extension>`, use+`</xs:extension>`, 1)
	alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType>`
	if chain {
		alias = `<xs:simpleType name="Alias"><xs:restriction base="t:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:` + f.name + `"/></xs:simpleType>`
	}
	return syntheticInheritedSchema("urn:owner", "", derived+syntheticInheritedGroup+syntheticInheritedAnnotated+syntheticInheritedOpen+syntheticInheritedTargets+alias)
}

func TestGroupedNamedBoundedInheritedBaseExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, chain := range []bool{false, true} {
				name := "direct"
				if chain {
					name = "chain"
				}
				t.Run(profile.name+"/"+family.name+"/"+name, func(t *testing.T) {
					root := groupedNamedBoundedInheritedSchema(family, chain, false)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, `base="t:Annotated"`), []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Annotated"`), complexContentTestLoc(t, root, `<xs:attribute name="bounded"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnsupported)
					zero := strings.Replace(root, `ref="t:Derivation"`, `ref="t:Derivation" minOccurs="0" maxOccurs="0"`, 1)
					schema, err = discoverTestSchemaWithPolicy(t, zero, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, zero, `base="t:Annotated"`), []Loc{complexContentTestLoc(t, zero, `<xs:complexType name="Annotated"`), complexContentTestLoc(t, zero, `<xs:attribute name="bounded"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnsupported)
				})
			}
		}
	}
}

//nolint:gocognit // The existing built-in G1 use and omitted named use remain public controls.
func TestGroupedNamedBoundedInheritedBaseControls(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			root := groupedNamedBoundedInheritedSchema(family, false, false)
			builtIn := strings.Replace(root, `type="t:Alias"`, `type="xs:`+family.name+`"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, builtIn, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:owner", "Derived")
			uses := owner.AttributeUses()
			if len(uses) != 3 {
				t.Fatalf("built-in G1 uses = %v", uses)
			}
			use, ok := uses[2].(LocalAttributeUse)
			if !ok {
				t.Fatalf("built-in G1 use = %T", uses[2])
			}
			ref, ok := use.TypeReference()
			if !ok || !ref.IsBuiltin() || use.TypeLoc() != complexContentTestLoc(t, builtIn, `type="xs:`+family.name+`"`) {
				t.Fatalf("built-in G1 reference = %#v", ref)
			}
			assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
			prohibited := groupedNamedBoundedInheritedSchema(family, false, true)
			schema, err = discoverTestSchemaWithPolicy(t, prohibited, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner = modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:owner", "Derived")
			if len(owner.AttributeUses()) != 2 || owner.AttributeUses()[0].Name().Local() != "final" || owner.AttributeUses()[1].Name().Local() != "name" {
				t.Fatalf("prohibited named use was published: %v", owner.AttributeUses())
			}
		}
	}
}

//nolint:gocognit // Dependency and base diagnostics must precede the new G1 unsupported exit.
func TestGroupedNamedBoundedInheritedBasePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			root := groupedNamedBoundedInheritedSchema(family, false, false)
			for _, tc := range []struct {
				name, source, marker, code string
				class                      FailureClass
				related                    []Loc
				spec                       func(XSDVersion) string
				cause                      error
			}{
				{"missing group", strings.Replace(root, `ref="t:Derivation"`, `ref="t:Missing"`, 1), `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, nil, schemaModelGroupReferenceSpecRef, errSchemaModelGroupReferenceUnresolved},
				{"missing group and type", strings.Replace(strings.Replace(root, `ref="t:Derivation"`, `ref="t:Missing"`, 1), `type="t:Alias"`, `type="t:Missing"`, 1), `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, nil, schemaModelGroupReferenceSpecRef, errSchemaModelGroupReferenceUnresolved},
				{"missing type", strings.Replace(root, `type="t:Alias"`, `type="t:Missing"`, 1), `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, nil, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
				{"prohibited missing type", strings.Replace(groupedNamedBoundedInheritedSchema(family, false, true), `type="t:Alias"`, `type="t:Missing"`, 1), `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, nil, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
				{"missing type and final base", strings.Replace(strings.Replace(root, `type="t:Alias"`, `type="t:Missing"`, 1), `<xs:complexType name="Annotated">`, `<xs:complexType name="Annotated" final="extension">`, 1), `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, nil, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
				{"final base", strings.Replace(root, `<xs:complexType name="Annotated">`, `<xs:complexType name="Annotated" final="extension">`, 1), `base="t:Annotated"`, invalidSchemaCompositionCode, FailureInvalid, nil, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnsupported},
				{"duplicate inherited name", strings.Replace(root, `name="bounded"`, `name="id"`, 1), `<xs:attribute name="id"`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, nil, schemaAttributeUseSpecRef, errSchemaAttributeUseDuplicate},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, tc.source, nil, profile.policy)
					related := tc.related
					if tc.name == "final base" {
						related = []Loc{complexContentTestLoc(t, tc.source, `final="extension"`)}
					}
					if tc.name == "duplicate inherited name" {
						related = []Loc{allParticleTestTokenLoc(t, "root.xsd", tc.source, `<xs:attribute name="id"`, 2)}
					}
					assertNCNameDiagnostic(t, schema, err, tc.class, tc.code, complexContentTestLoc(t, tc.source, tc.marker), related, tc.spec(profile.version), tc.cause)
				})
			}
		}
	}
}

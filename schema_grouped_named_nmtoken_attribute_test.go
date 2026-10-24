package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const groupedNMTOKENTypes = `<xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:NMTOKEN"><xs:whiteSpace value="collapse"/><xs:enumeration value="one"/><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType>` //nolint:gosec // Schema fixture has no credential.

func groupedNMTOKENSchema(version, group, uses string) string {
	root := groupedExtensionSchema(version, group, uses)
	return strings.Replace(root, `</xs:schema>`, groupedNMTOKENTypes+`</xs:schema>`, 1)
}

//nolint:gocognit,funlen // One public result ties copied facets to the ordered use and its owner.
func TestGroupedNamedNMTOKENPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, edition := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+edition, func(t *testing.T) {
				const uses = `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="built" type="xs:NMTOKEN"/><xs:attribute name="named" type="t:Child" use="required" form="unqualified"/><xs:attribute name="gone" type="t:Child" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
				root := groupedNMTOKENSchema(edition, ` minOccurs="2" maxOccurs="3"`, uses)
				root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) || owner.Base() != mustTestQName(t, "urn:root", "Base") {
					t.Fatalf("owner = %v", owner)
				}
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() || !hasGroup || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("base/group = %v/%v", base, group)
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				attributes := owner.AttributeUses()
				var names []string
				for _, attribute := range attributes {
					names = append(names, attribute.Name().Local())
				}
				if !reflect.DeepEqual(names, []string{"before", "built", "named", "after"}) {
					t.Fatalf("lexical use order = %v", names)
				}
				built, builtOK := attributes[1].(LocalAttributeUse)
				builtRef, hasBuiltRef := built.TypeReference()
				_, builtHasID := built.TypeID()
				if !builtOK || !hasBuiltRef || !builtRef.IsBuiltin() || builtHasID || built.DeclaredType() != mustTestQName(t, testXSDNamespace, "NMTOKEN") {
					t.Fatalf("built-in control = %v/%v", built, builtRef)
				}
				use, local := attributes[2].(LocalAttributeUse)
				qname := mustTestQName(t, "urn:root", "Child")
				if !local || use.Name() != mustTestQName(t, "", "named") || use.DeclaredType() != qname || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="named"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="named"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="t:Child"`) || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
					t.Fatalf("named use = %v", use)
				}
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				refID, hasRefID := ref.ComponentID()
				white, hasWhite := ref.StringWhiteSpaceFacet()
				if !hasRef || !ref.IsNamed() || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != qname || ref.QName() != qname || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != complexContentTestLoc(t, root, `<xs:restriction base="t:Parent"`) || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, qname) || !hasWhite || white.Value() != "collapse" || !white.Fixed() || white.Loc() != complexContentTestLoc(t, root, `value="collapse"`) {
					t.Fatalf("named type facts = %v/%v/%v", ref, id, white)
				}
				assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two", "three"}, []Loc{complexContentTestLoc(t, root, `value="two"`), complexContentTestLoc(t, root, `value="three"`)})
				values, locs := ref.StringEnumerationFacets().Values(), ref.StringEnumerationFacets().Locations()
				values[0], locs[0], attributes[2] = "changed", Loc{}, nil
				components := schema.Components()
				components[0] = Component{}
				fresh, hasFresh := owner.AttributeUses()[2].(LocalAttributeUse)
				freshRef, hasFreshRef := fresh.TypeReference()
				if !hasFresh || !hasFreshRef || freshRef.StringEnumerationFacets().Values()[0] != "two" || freshRef.StringEnumerationFacets().Locations()[0].IsZero() || schema.Components()[0].Name().IsZero() {
					t.Fatal("caller changed published facts")
				}
				var walks [2][]ComponentID
				for index := range walks {
					if walkErr := schema.Walk(func(component Component) error { walks[index] = append(walks[index], component.ID()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
				}
				if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
					t.Fatalf("walk order = %v/%v", walks[0], walks[1])
				}
				controlRoot := strings.Replace(root, `<xs:attribute name="named" type="t:Child" use="required" form="unqualified"/>`, "", 1)
				controlRoot = strings.Replace(controlRoot, `<xs:attribute name="gone" type="t:Child" use="prohibited"/>`, "", 1)
				control, controlErr := discoverTestSchemaWithPolicy(t, controlRoot, nil, profile.policy)
				if controlErr != nil {
					t.Fatal(controlErr)
				}
				controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
				controlGroup, hasControlGroup := controlOwner.Particle().(ModelGroupReferenceParticle)
				controlBase, hasControlBase := controlOwner.BaseReference()
				controlBaseID, hasControlBaseID := controlBase.ComponentID()
				if !hasControlGroup || !hasControlBase || !hasControlBaseID || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != group.Occurrences().String() || controlBaseID != baseID || len(controlOwner.AttributeUses()) != 3 {
					t.Fatalf("absent-use control = %v", controlOwner)
				}
				again, repeatErr := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if repeatErr != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeat = %v", repeatErr)
				}
			})
		}
	}
}

func TestGroupedNamedNMTOKENConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedNMTOKENSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		testGroupedNamedTokenFamilyConsumers(t, root, profile)
	}
}

//nolint:gocognit // Source arrangements independently expose document and type provenance.
func TestGroupedNamedNMTOKENGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon", "repeated"} {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), ` minOccurs="1" maxOccurs="2"`, `<xs:attribute name="named" type="t:Alias"/>`)
				typeName, namespace, sourceID := "t:Alias", "urn:root", SourceID("root.xsd")
				declaration := `<xs:simpleType name="Alias"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="allow"/></xs:restriction></xs:simpleType>`
				fixtures := map[string]discoveryFixture{}
				if graph == "forward" {
					root = strings.Replace(root, `</xs:schema>`, declaration+`</xs:schema>`, 1)
				}
				if graph != "forward" {
					directive := `<xs:include schemaLocation="dependency.xsd"/>`
					target := ` targetNamespace="urn:root"`
					if graph == "imported" {
						directive = `<xs:import namespace="urn:other" schemaLocation="dependency.xsd"/>`
						target, typeName, namespace = ` targetNamespace="urn:other"`, "o:Alias", "urn:other"
						root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
						root = strings.Replace(root, `type="t:Alias"`, `type="o:Alias"`, 1)
					}
					if graph == "chameleon" {
						target = ""
					}
					if graph == "repeated" {
						directive += `<xs:include schemaLocation="again.xsd"/>`
					}
					root = strings.Replace(root, `<xs:element name="root"`, directive+`<xs:element name="root"`, 1)
					dependency := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + target + ` version="` + string(profile.version) + `">` + declaration + `</xs:schema>`
					sourceID = "dependency.xsd"
					fixtures["dependency.xsd"] = discoveryFixture{id: sourceID, contents: dependency}
					if graph == "repeated" {
						fixtures["again.xsd"] = discoveryFixture{id: sourceID, contents: dependency}
					}
				}
				sourceContents := root
				if graph != "forward" {
					sourceContents = fixtures["dependency.xsd"].contents
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				group, groupOK := owner.Particle().(ModelGroupReferenceParticle)
				base, baseOK := owner.BaseReference()
				baseID, baseIDOK := base.ComponentID()
				uses := owner.AttributeUses()
				if !groupOK || group.Occurrences().String() != "1/2" || !baseOK || !baseIDOK || baseID.Source() != "root.xsd" || len(uses) != 1 {
					t.Fatalf("owner graph = %v", owner)
				}
				use, local := uses[0].(LocalAttributeUse)
				ref, refOK := use.TypeReference()
				typeID, idOK := use.TypeID()
				if !local || !refOK || !idOK || typeID.Source() != sourceID || typeID != componentIDForName(t, schema, mustTestQName(t, namespace, "Alias")) || use.DeclaredType() != mustTestQName(t, namespace, "Alias") || ref.Loc() != complexContentTestLoc(t, root, `type="`+typeName+`"`) || ref.Kind() != SimpleTypeReferenceNamed || ref.VarietyLoc() != complexContentTestSourceLoc(t, sourceID, sourceContents, `<xs:restriction base="xs:NMTOKEN"`) {
					t.Fatalf("type provenance = %v/%v", use, ref)
				}
				if len(schema.Documents()) != 1 && graph == "forward" || len(schema.Documents()) != 2 && graph != "forward" {
					t.Fatalf("documents = %v", schema.Documents())
				}
				locations := ref.StringEnumerationFacets().Locations()
				if !reflect.DeepEqual(ref.StringEnumerationFacets().Values(), []string{"allow"}) || len(locations) != 1 || locations[0] != complexContentTestSourceLoc(t, sourceID, sourceContents, `value="allow"`) {
					t.Fatalf("copied facets = %v/%v", ref.StringEnumerationFacets().Values(), locations)
				}
			})
		}
	}
}

func TestGroupedNamedNMTOKENOmissionAndBoundaryExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedNMTOKENSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="named" type="t:Child"/><xs:attribute name="gone" type="t:Child" use="prohibited"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "named" {
			t.Fatalf("zero particle/prohibited use = %v", owner)
		}
		testGroupedLongExits(t, profile, root, []groupedLongExit{
			{"group before named use", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
			{"prohibited type before base", `type="t:Child" use="prohibited"`, `type="t:Missing" use="prohibited"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
			{"missing base", `base="t:Base"`, `base="t:Missing"`, `base="t:Missing"`, "", invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnresolved, schemaComplexTypeExtensionSpecRef},
			{"wrong base kind", `base="t:Base"`, `base="t:Child"`, `base="t:Child"`, `<xs:simpleType name="Child"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseWrongKind, schemaComplexTypeExtensionSpecRef},
			{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
		})
	}
}

func TestGroupedNamedNMTOKENTypeExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, marker, related, code string
			class                                              FailureClass
			spec                                               func(XSDVersion) string
			cause                                              error
		}{
			{"missing", "t:Missing", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
			{"wrong kind", "t:Base", "", `type="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeWrongKind},
			{"named list", "t:List", `<xs:simpleType name="List"><xs:list itemType="xs:NMTOKEN"/></xs:simpleType>`, `type="t:List"`, "", UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"named union", "t:Union", `<xs:simpleType name="Union"><xs:union memberTypes="xs:NMTOKEN xs:NMTOKEN"/></xs:simpleType>`, `type="t:Union"`, "", UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"unsupported facet", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, FailureUnsupported, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
			{"invalid narrowing", "t:Bad", `<xs:simpleType name="Parent"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="t:Parent"><xs:enumeration value="outside"/></xs:restriction></xs:simpleType>`, `value="outside"`, `value="one"`, InvalidEnumerationRestrictionCode, FailureInvalid, func(v XSDVersion) string { return enumerationSpecRef(v, enumerationRestrictionRule) }, errInvalidEnumerationRestriction},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="`+test.typeName+`"/>`)
				root = strings.Replace(root, `</xs:schema>`, test.declaration+`</xs:schema>`, 1)
				failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				d := requireDiagnostic(t, err)
				assertZeroSchema(t, failed)
				var related []Loc
				if test.related != "" {
					related = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != test.spec(profile.version) || !errors.Is(err, test.cause) {
					t.Fatalf("%s = %v", test.name, err)
				}
			})
		}
	}
}

func TestGroupedNamedNMTOKENGraphFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="o:Hidden"/>`)
		root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
		foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:schema>`
		fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
		failed, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)

		cycle := groupedNMTOKENSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		cycle = strings.Replace(cycle, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, cycle, nil, profile.policy)
		cycleSpec := "xsd11-structures#mg-props-correct"
		if profile.version == XSDVersion10 {
			cycleSpec = "xsd10-structures#mg-props-correct"
		}
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, allParticleTestTokenLoc(t, "root.xsd", cycle, `ref="t:Fields"`, 2), []Loc{complexContentTestLoc(t, cycle, `<xs:group name="Fields"`)}, cycleSpec, errSchemaModelGroupReferenceCycle)

		cycle = groupedNMTOKENSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		cycle = strings.Replace(cycle, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, cycle, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, cycle, `base="t:Derived"`), []Loc{complexContentTestLoc(t, cycle, `base="t:Base"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseCycle)

		duplicate := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Alias"/>`)
		duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:schema>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias" id="second"`), []Loc{complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias"><xs:restriction`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
	}
}

func TestGroupedNamedNMTOKENPolicyAndSyntaxPrecedence(t *testing.T) {
	root := groupedNMTOKENSchema("1.1", "", `<xs:attribute name="named" type="t:Child" inheritable="false"/>`)
	failed, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, `inheritable="false"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, parseErr := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if parseErr != nil || len(modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived").AttributeUses()) != 1 {
			t.Fatalf("policy %s = %v", policy, parseErr)
		}
	}
	for _, profile := range tokenPolicyProfiles() {
		bad := groupedNMTOKENSchema(string(profile.version), ` minOccurs="3" maxOccurs="2"`, `<xs:attribute name="named" type="t:Child"/>`)
		failed, err = discoverTestSchemaWithPolicy(t, bad, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, bad, `<xs:group ref="t:Fields"`), []Loc{complexContentTestLoc(t, bad, `minOccurs="3"`), complexContentTestLoc(t, bad, `maxOccurs="2"`)}, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum)
		bad = groupedNMTOKENSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child" use="sometimes"/>`)
		failed, err = discoverTestSchemaWithPolicy(t, bad, nil, profile.policy)
		d := requireDiagnostic(t, err)
		assertZeroSchema(t, failed)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, bad, `use="sometimes"`) || len(d.Related()) != 0 || d.SpecRef() != schemaAttributeUseSpecRef(profile.version) {
			t.Fatalf("invalid use = %v", err)
		}
	}
}

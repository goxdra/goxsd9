package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const groupedNamedTokenTypes = `<xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:token"><xs:whiteSpace value="collapse"/><xs:enumeration value="one"/><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType>` //nolint:gosec // Schema fixture has no credential.

func groupedNamedTokenSchema(version, group, uses string) string {
	root := groupedExtensionSchema(version, group, uses)
	return strings.Replace(root, `</xs:schema>`, groupedNamedTokenTypes+`</xs:schema>`, 1)
}

//nolint:gocognit,funlen // One public result ties group, base, use, copied facets, and order together.
func TestGroupedNamedTokenPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, edition := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+edition, func(t *testing.T) {
				uses := `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="builtin" type="xs:token"/><xs:attribute name="named" type="t:Child" use="required" form="unqualified"/><xs:attribute name="gone" type="t:Child" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
				root := groupedNamedTokenSchema(edition, ` minOccurs="2" maxOccurs="3"`, uses)
				root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				owners := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
				if len(owners) != 1 || owner.ID() != owners[0].ID() || owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner facts = %v", owner)
				}
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
				if !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base = %v", base)
				}
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if !hasGroup || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("group = %v", owner.Particle())
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				attributes := owner.AttributeUses()
				if len(attributes) != 4 || attributes[0].Name().Local() != "before" || attributes[1].Name().Local() != "builtin" || attributes[2].Name().Local() != "named" || attributes[3].Name().Local() != "after" {
					t.Fatalf("ordered uses = %v", attributes)
				}
				built, isBuilt := attributes[1].(LocalAttributeUse)
				builtRef, hasBuiltRef := built.TypeReference()
				builtID, hasBuiltID := built.TypeID()
				if !isBuilt || !hasBuiltRef || !builtRef.IsBuiltin() || hasBuiltID || !builtID.IsZero() || built.DeclaredType() != mustTestQName(t, testXSDNamespace, "token") {
					t.Fatalf("built-in control = %v/%v", built, builtRef)
				}
				use, isLocal := attributes[2].(LocalAttributeUse)
				qname := mustTestQName(t, "urn:root", "Child")
				if !isLocal || use.Name() != mustTestQName(t, "", "named") || use.DeclaredType() != qname || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="named"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="named"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="t:Child"`) || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
					t.Fatalf("named use = %v", use)
				}
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				refID, hasRefID := ref.ComponentID()
				if !hasRef || !ref.IsNamed() || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != qname || ref.QName() != qname || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != complexContentTestLoc(t, root, `<xs:restriction base="t:Parent"`) || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, qname) {
					t.Fatalf("named reference = %v/%v", ref, id)
				}
				white, hasWhite := ref.StringWhiteSpaceFacet()
				if !hasWhite || white.Value() != "collapse" || !white.Fixed() || white.Loc() != complexContentTestLoc(t, root, `value="collapse"`) {
					t.Fatalf("effective whitespace = %v", white)
				}
				assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two", "three"}, []Loc{complexContentTestLoc(t, root, `value="two"`), complexContentTestLoc(t, root, `value="three"`)})
				values := ref.StringEnumerationFacets().Values()
				locs := ref.StringEnumerationFacets().Locations()
				values[0], locs[0] = "changed", Loc{}
				attributes[2] = nil
				components := schema.Components()
				components[0] = Component{}
				fresh, isFresh := owner.AttributeUses()[2].(LocalAttributeUse)
				freshRef, hasFreshRef := fresh.TypeReference()
				if !isFresh || !hasFreshRef || freshRef.StringEnumerationFacets().Values()[0] != "two" || freshRef.StringEnumerationFacets().Locations()[0].IsZero() || schema.Components()[0].Name().IsZero() {
					t.Fatal("caller mutated completed facts")
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
				controlGroup, controlHasGroup := controlOwner.Particle().(ModelGroupReferenceParticle)
				controlBase, controlHasBase := controlOwner.BaseReference()
				controlBaseID, controlHasBaseID := controlBase.ComponentID()
				if !controlHasGroup || !controlHasBase || !controlHasBaseID || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != "2/3" || controlBaseID != baseID || len(controlOwner.AttributeUses()) != 3 {
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

func TestGroupedNamedTokenConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedNamedTokenSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		testGroupedNamedTokenFamilyConsumers(t, root, profile)
	}
}

func testGroupedNamedTokenFamilyConsumers(t *testing.T, root string, profile tokenPolicyProfile) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatal(err)
	}
	useLoc := complexContentTestLoc(t, root, `<xs:attribute name="named"`)
	ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
	rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
	instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	vd := requireDiagnostic(t, instanceErr)
	if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
		t.Fatalf("validation = %v", instanceErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	gd := requireDiagnostic(t, generationErr)
	if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("generation = %v/%v", output, generationErr)
	}
}

//nolint:gocognit // Graph provenance is checked through one public grouped owner per source arrangement.
func TestGroupedNamedTokenGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				lexical, typeNS, sourceID := "r:Alias", "urn:root", SourceID("root.xsd")
				baseLexical := "r:Base"
				directive, target := "", ` targetNamespace="urn:root"`
				if graph == "included" || graph == "chameleon" {
					directive, sourceID = `<xs:include schemaLocation="dependency.xsd"/>`, "dependency.xsd"
				}
				if graph == "imported" {
					directive = `<xs:import namespace="urn:other" schemaLocation="dependency.xsd"/>`
					lexical, typeNS, sourceID, target = "o:Alias", "urn:other", "dependency.xsd", ` targetNamespace="urn:other"`
					baseLexical = "o:Base"
				}
				if graph == "chameleon" {
					target = ""
				}
				declarations := `<xs:complexType name="Base"/><xs:simpleType name="Alias"><xs:restriction base="xs:token"><xs:enumeration value=" allow "/></xs:restriction></xs:simpleType>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(profile.version) + `">` + directive + `<xs:element name="root" type="r:Derived"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + baseLexical + `"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/><xs:attribute name="named" type="` + lexical + `"/></xs:extension></xs:complexContent></xs:complexType><xs:group name="Fields"><xs:sequence/></xs:group>`
				fixtures := map[string]discoveryFixture{}
				contents := root + declarations + `</xs:schema>`
				if graph == "forward" {
					root = contents
				}
				if graph != "forward" {
					root += `</xs:schema>`
					contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + target + ` version="` + string(profile.version) + `">` + declarations + `</xs:schema>`
					fixtures["dependency.xsd"] = discoveryFixture{id: sourceID, contents: contents}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				uses := owner.AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("uses = %v", uses)
				}
				use, isLocal := uses[0].(LocalAttributeUse)
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				if !hasGroup || group.Occurrences().String() != "1/2" || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || !hasBase || !hasBaseID || baseID.Source() != sourceID || !isLocal || !hasRef || !hasID || id.Source() != sourceID || id != componentIDForName(t, schema, mustTestQName(t, typeNS, "Alias")) || use.DeclaredType() != mustTestQName(t, typeNS, "Alias") || ref.Loc() != complexContentTestLoc(t, root, `type="`+lexical+`"`) || ref.VarietyLoc() != complexContentTestSourceLoc(t, sourceID, contents, `<xs:restriction base="xs:token"`) {
					t.Fatalf("graph facts = %v/%v/%v", group, base, use)
				}
				assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{" allow "}, []Loc{complexContentTestSourceLoc(t, sourceID, contents, `value=" allow "`)})
			})
		}
	}
}

func TestGroupedNamedTokenAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		zero := groupedNamedTokenSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="named" type="t:Child"/><xs:attribute name="gone" type="t:Child" use="prohibited"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, zero, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "named" {
			t.Fatalf("omission facts = %v/%v", owner.Particle(), owner.AttributeUses())
		}
		testGroupedLongExits(t, profile, zero, []groupedLongExit{
			{"group before named use", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
			{"prohibited named use before base", `type="t:Child" use="prohibited"`, `type="t:Missing" use="prohibited"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
			{"missing base", `base="t:Base"`, `base="t:Missing"`, `base="t:Missing"`, "", invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnresolved, schemaComplexTypeExtensionSpecRef},
			{"wrong base kind", `base="t:Base"`, `base="t:Child"`, `base="t:Child"`, `<xs:simpleType name="Child"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseWrongKind, schemaComplexTypeExtensionSpecRef},
			{"base final", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"base uses", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
		})
	}
}

func TestGroupedNamedTokenTypeDependencyExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, marker, related, code string
			class                                              FailureClass
			spec                                               func(XSDVersion) string
			cause                                              error
		}{
			{"missing", "t:Missing", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
			{"wrong kind", "t:Base", "", `type="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeWrongKind},
			{"cycle", "t:First", `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, `type="t:First"`, `<xs:attribute name="named"|base="t:Second"|base="t:First"|<xs:simpleType name="First"|<xs:simpleType name="Second"`, diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaSimpleTypeBaseCycle},
			{"simple final", "t:Bad", `<xs:simpleType name="Parent" final="restriction"><xs:restriction base="xs:token"/></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="t:Parent"/></xs:simpleType>`, `base="t:Parent"`, `final="restriction"`, diagnosticSchemaSimpleTypeBaseCode, FailureInvalid, schemaSimpleTypeRestrictionSpecRef, errSchemaSimpleTypeInvalidDerivation},
			{"whitespace widening", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:token"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, FailureInvalid, stringWhiteSpaceSpecRef, errInvalidStringWhiteSpaceRestriction},
			{"unsupported facet", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:token"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, FailureUnsupported, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
			{"named list", "t:List", `<xs:simpleType name="List"><xs:list itemType="xs:token"/></xs:simpleType>`, `type="t:List"`, "", UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"named union", "t:Union", `<xs:simpleType name="Union"><xs:union memberTypes="xs:token xs:token"/></xs:simpleType>`, `type="t:Union"`, "", UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="`+test.typeName+`"/>`)
				root = strings.Replace(root, `</xs:schema>`, test.declaration+`</xs:schema>`, 1)
				var related []Loc
				if test.related != "" {
					for _, marker := range strings.Split(test.related, "|") {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

func TestGroupedNamedTokenVisibilityCyclesAndDuplicates(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="o:Hidden"/>`)
		root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
		foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`
		fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
		failed, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)

		duplicate := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Alias"/>`)
		duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:token"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias" id="second"`), []Loc{complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias"><xs:restriction`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)

		groupCycle := groupedNamedTokenSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		groupCycle = strings.Replace(groupCycle, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, groupCycle, nil, profile.policy)
		cycleSpec := "xsd11-structures#mg-props-correct"
		if profile.version == XSDVersion10 {
			cycleSpec = "xsd10-structures#mg-props-correct"
		}
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, allParticleTestTokenLoc(t, "root.xsd", groupCycle, `ref="t:Fields"`, 2), []Loc{complexContentTestLoc(t, groupCycle, `<xs:group name="Fields"`)}, cycleSpec, errSchemaModelGroupReferenceCycle)

		baseCycle := groupedNamedTokenSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		baseCycle = strings.Replace(baseCycle, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, baseCycle, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, baseCycle, `base="t:Derived"`), []Loc{complexContentTestLoc(t, baseCycle, `base="t:Base"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseCycle)
	}
}

//nolint:gocognit // Each precedence case has its own exact located diagnostic.
func TestGroupedNamedTokenNarrowingAndSyntaxPrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, from, to, marker, related, code string
			class                                 FailureClass
			spec                                  func(XSDVersion) string
			cause                                 error
		}{
			{"invalid narrowing", `<xs:enumeration value="two"/>`, `<xs:enumeration value="outside"/>`, `value="outside"`, `value="one"`, InvalidEnumerationRestrictionCode, FailureInvalid, func(v XSDVersion) string { return enumerationSpecRef(v, enumerationRestrictionRule) }, errInvalidEnumerationRestriction},
			{"invalid occurrence", ` minOccurs="2" maxOccurs="3"`, ` minOccurs="3" maxOccurs="2"`, `<xs:group ref="t:Fields"`, `minOccurs="3"|maxOccurs="2"`, invalidSchemaCompositionCode, FailureInvalid, schemaParticleCorrectSpecRef, errParticleOccurrenceMinimumExceedsMaximum},
			{"missing group before unsupported type", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, schemaModelGroupReferenceSpecRef, errSchemaModelGroupReferenceUnresolved},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedNamedTokenSchema(string(profile.version), ` minOccurs="2" maxOccurs="3"`, `<xs:attribute name="named" type="t:Child"/>`)
				root = strings.Replace(root, test.from, test.to, 1)
				if test.name == "missing group before unsupported type" {
					root = strings.Replace(root, `type="t:Child"`, `type="t:MissingType"`, 1)
				}
				var related []Loc
				if test.name == "invalid narrowing" {
					related = []Loc{
						complexContentTestLoc(t, root, `value="one"`),
						complexContentTestLoc(t, root, `value="two"`),
						allParticleTestTokenLoc(t, "root.xsd", root, `value="three"`, 2),
					}
				}
				if test.name == "invalid occurrence" {
					for _, marker := range strings.Split(test.related, "|") {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
				}
				failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

func TestGroupedNamedTokenSourceResolution(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedNamedTokenSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/>`)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
		source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		cause := errors.New("named grouped token dependency unavailable")
		failed, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureResolution, SourceResolveCode, complexContentTestLoc(t, root, `<xs:include`), nil, "", cause)
	}
}

func TestGroupedNamedTokenUseAndGroupFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedNamedTokenSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Child"/><xs:attribute name="named" type="xs:boolean"/>`)
		failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeUseDuplicateCode, complexContentTestLoc(t, root, `<xs:attribute name="named" type="xs:boolean"`), []Loc{complexContentTestLoc(t, root, `<xs:attribute name="named" type="t:Child"`)}, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseDuplicate)

		root = groupedNamedTokenSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="named" type="t:Child"/>`)
		root = strings.Replace(root, `ref="t:Fields"`, `ref="t:root"`, 1)
		failed, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceWrongKindCode, complexContentTestLoc(t, root, `ref="t:root"`), []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`)}, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind)
	}
}

func TestGroupedNamedTokenStrict10PolicyPrecedesOwnerGate(t *testing.T) {
	root := groupedNamedTokenSchema("1.1", "", `<xs:attribute name="named" type="t:Child" inheritable="false"/>`)
	failed, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, `inheritable="false"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, parseErr := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "named" {
			t.Fatalf("policy %s uses = %v", policy, owner.AttributeUses())
		}
	}
}

func TestGroupedNamedTokenAnonymousOwnerExcluded(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, use, types, marker, related string
			spec                              func(XSDVersion) string
			cause                             error
		}{
			{"built-in direct", `<xs:attribute name="value" type="xs:token"/>`, "", `type="xs:token"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"named direct", `<xs:attribute name="value" type="t:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:token"/></xs:simpleType>`, `type="t:Alias"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"inline atomic", `<xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"attribute ref", `<xs:attribute ref="t:alias"/>`, `<xs:attribute name="alias" type="xs:token"/>`, `ref="t:alias"`, `<xs:attribute name="alias"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema(string(profile.version), "", test.use)
				root = strings.Replace(root, `</xs:schema>`, test.types+`</xs:schema>`, 1)
				failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					related = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

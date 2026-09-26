package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedExtensionSchema(version, group, attributes string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `" attributeFormDefault="qualified">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"` + group + `/>` + attributes + `</xs:extension></xs:complexContent></xs:complexType>
  <xs:group name="Fields"><xs:sequence/></xs:group>
  <xs:attribute name="global" type="xs:decimal"/>
  <xs:complexType name="Base"/>
</xs:schema>`
}

//nolint:gocognit // Keep the paired policy and immutable fact assertions together.
func TestGroupedExtensionFactsAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{"compatibility10", Compatibility, "1.0"},
		{"compatibility11", Compatibility, "1.1"},
		{"strict10-label10", Strict10, "1.0"},
		{"strict10-label11", Strict10, "1.1"},
		{"strict11-label10", Strict11, "1.0"},
		{"strict11-label11", Strict11, "1.1"},
	}
	attributes := `<xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute name="local" type="xs:integer" form="unqualified"/><xs:attribute ref="t:global"/><xs:attribute name="excluded" type="xs:integer" use="prohibited"/>`
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := groupedExtensionSchema(profile.version, ` minOccurs="2" maxOccurs="18446744073709551616"`, attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover grouped extension: %v", err)
			}
			components := schema.Components()
			if len(components) != 5 {
				t.Fatalf("components = %d, want 5", len(components))
			}
			derived := requireTestComplexTypeDefinition(t, components[1], "Derived")
			if derived.Derivation() != ComplexTypeDerivationExtension || derived.Base() != mustTestQName(t, "urn:root", "Base") || derived.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("derivation/base facts = %q/%q/%s", derived.Derivation(), derived.Base(), derived.BaseLoc())
			}
			base, ok := derived.BaseReference()
			if !ok {
				t.Fatal("base reference absent")
			}
			baseID, ok := base.ComponentID()
			if !ok || baseID != components[4].ID() {
				t.Fatalf("base ID = %v/%v, want %v", baseID, ok, components[4].ID())
			}
			group, ok := derived.Particle().(ModelGroupReferenceParticle)
			if !ok || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != components[2].ID() || group.Occurrences().String() != "2/18446744073709551616" {
				t.Fatalf("group facts = %#v", derived.Particle())
			}
			if group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) {
				t.Fatalf("group locations = %s/%s", group.Loc(), group.RefLoc())
			}
			uses := derived.AttributeUses()
			if len(uses) != 3 || uses[0].Name() != mustTestQName(t, "urn:root", "flag") || uses[0].Use() != AttributeUseRequired || uses[1].Name() != mustTestQName(t, "", "local") || uses[2].Name() != mustTestQName(t, "urn:root", "global") {
				t.Fatalf("ordered uses = %#v", uses)
			}
			local, ok := uses[0].(LocalAttributeUse)
			if !ok || local.NameLoc() != complexContentTestLoc(t, root, `name="flag"`) || local.TypeLoc() != complexContentTestLoc(t, root, `type="xs:boolean"`) || local.UseLoc() != complexContentTestLoc(t, root, `use="required"`) {
				t.Fatalf("local use locations = %#v", uses[0])
			}
			ref, ok := uses[2].(AttributeReferenceUse)
			if !ok || ref.TargetID() != components[3].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:global"`) {
				t.Fatalf("attribute ref = %#v", uses[2])
			}
			before := schema.Components()
			uses[0] = nil
			if len(derived.AttributeUses()) != 3 || !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("attribute-use query changed immutable schema")
			}
		})
	}
}

func TestGroupedExtensionZeroGroupRetainsBaseAndRejectsConsumers(t *testing.T) {
	root := groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="excluded" type="xs:integer" use="prohibited"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover zero group: %v", err)
	}
	derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
	if derived.Particle() != nil || len(derived.AttributeUses()) != 0 || derived.Derivation() != ComplexTypeDerivationExtension || derived.Base() != mustTestQName(t, "urn:root", "Base") {
		t.Fatalf("zero group facts = %T/%d/%q/%q", derived.Particle(), len(derived.AttributeUses()), derived.Derivation(), derived.Base())
	}
	instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if instanceErr == nil || !errors.Is(instanceErr, ErrUnsupported) {
		t.Fatalf("validation error = %v, want unsupported", instanceErr)
	}
	_, generateErr := GenerateGo(schema, "generated")
	if generateErr == nil || !errors.Is(generateErr, ErrUnsupported) {
		t.Fatalf("generation error = %v, want unsupported", generateErr)
	}
}

//nolint:gocognit // Keep the paired validation and generation precedence matrix together.
func TestGroupedExtensionConsumerDiagnosticPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		group           string
		attributes      string
		primary         string
		validationCause error
	}{
		{"group with uses", "", `<xs:attribute name="first" type="xs:boolean"/><xs:attribute name="second" type="xs:integer"/>`, `<xs:attribute name="first"`, errInstanceAttributes},
		{"zero group with uses", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="first" type="xs:boolean"/><xs:attribute name="second" type="xs:integer"/>`, `<xs:attribute name="first"`, errInstanceAttributes},
		{"group with prohibited use", "", `<xs:attribute name="excluded" type="xs:integer" use="prohibited"/>`, `ref="t:Fields"`, errInstanceModelGroupReference},
		{"zero group with prohibited use", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="excluded" type="xs:integer" use="prohibited"/>`, `<xs:extension`, errInstanceComplexContentExtension},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := groupedExtensionSchema("1.1", test.group, test.attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("discover grouped extension: %v", err)
			}
			want := complexContentTestLoc(t, root, test.primary)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			if validationErr == nil {
				t.Fatal("validation accepted grouped extension")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Loc() != want || !errors.Is(validationErr, test.validationCause) || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %v, want %s and cause %v", validationErr, want, test.validationCause)
			}
			generated, generationErr := GenerateGo(schema, "generated")
			if generationErr == nil || generated != nil {
				t.Fatalf("generation = %q/%v, want nil/unsupported", generated, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Loc() != want || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %v, want unsupported at %s", generationErr, want)
			}
		})
	}
}

func TestGroupedExtensionRetainsBoundedInheritedWildcard(t *testing.T) {
	root := groupedExtensionSchema("1.1", "", `<xs:attribute name="flag" type="xs:boolean"/>`)
	root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover inherited wildcard: %v", err)
	}
	derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
	wildcard, ok := derived.AnyAttribute()
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
		t.Fatalf("wildcard = %#v, want inherited ##other/lax", wildcard)
	}
}

func TestGroupedExtensionValidatesGroupBeforeZeroOmissionAndAttributes(t *testing.T) {
	root := strings.Replace(groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="bad" type="xs:ID"/>`), `ref="t:Fields"`, `ref="t:Missing"`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("unresolved zero group was accepted")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceUnresolvedCode || diagnostic.Loc() != complexContentTestLoc(t, root, `ref="t:Missing"`) || !errors.Is(err, errSchemaModelGroupReferenceUnresolved) {
		t.Fatalf("group diagnostic = %v", err)
	}
}

func TestGroupedExtensionResolvesChameleonAndImportedGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:a="urn:attributes" targetNamespace="urn:root">
  <xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:attributes" schemaLocation="attributes.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute ref="a:global" use="required"/></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="root.xsd"/><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/></xs:schema>`
	attributes := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:attributes"><xs:attribute name="global" type="xs:boolean"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"shared.xsd":     {id: "shared.xsd", contents: shared},
		"again.xsd":      {id: "shared.xsd", contents: shared},
		"root.xsd":       {id: "root.xsd", contents: root},
		"attributes.xsd": {id: "attributes.xsd", contents: attributes},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err != nil {
		t.Fatalf("discover graph: %v", err)
	}
	documents := schema.Documents()
	if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "shared.xsd" || documents[2].Source() != "attributes.xsd" {
		t.Fatalf("documents = %v, want root/shared/attributes", documents)
	}
	derived := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
	groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
	if len(groups) != 1 {
		t.Fatalf("chameleon groups = %d, want 1", len(groups))
	}
	group, ok := derived.Particle().(ModelGroupReferenceParticle)
	if !ok || group.TargetID() != groups[0].ID() {
		t.Fatalf("group = %#v, want chameleon target", derived.Particle())
	}
	base := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
	baseRef, baseOK := derived.BaseReference()
	baseID, idOK := baseRef.ComponentID()
	if len(base) != 1 || !baseOK || !idOK || baseID != base[0].ID() {
		t.Fatalf("base = %#v, want chameleon target", baseRef)
	}
	uses := derived.AttributeUses()
	global := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:attributes", "global"))
	if len(uses) != 1 || len(global) != 1 {
		t.Fatalf("attribute use/global target counts = %d/%d, want 1/1", len(uses), len(global))
	}
	attributeRef, refOK := uses[0].(AttributeReferenceUse)
	if !refOK || attributeRef.TargetID() != global[0].ID() || attributeRef.Use() != AttributeUseRequired {
		t.Fatalf("attribute use = %#v, want imported target", uses)
	}
}

func TestGroupedExtensionRejectsDuplicateEffectiveAttributeNames(t *testing.T) {
	root := groupedExtensionSchema("1.1", "", `<xs:attribute name="same" type="xs:boolean"/><xs:attribute name="same" type="xs:integer"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("duplicate attribute names were accepted")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeUseDuplicateCode || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="same" type="xs:integer"`) || !errors.Is(err, errSchemaAttributeUseDuplicate) {
		t.Fatalf("duplicate diagnostic = %v", err)
	}
	if !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, `<xs:attribute name="same" type="xs:boolean"`)}) {
		t.Fatalf("related = %v, want first use", diagnostic.Related())
	}
}

func TestGroupedExtensionRetainsAnonymousLocalAttributeIdentity(t *testing.T) {
	attributes := `<xs:attribute name="inline"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>`
	root := groupedExtensionSchema("1.1", "", attributes)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover anonymous local attribute: %v", err)
	}
	derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
	uses := derived.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("uses = %d, want 1", len(uses))
	}
	local, ok := uses[0].(LocalAttributeUse)
	if !ok || local.Name() != mustTestQName(t, "urn:root", "inline") || local.TypeLoc() != complexContentTestLoc(t, root, `<xs:simpleType>`) {
		t.Fatalf("local anonymous use = %#v", uses[0])
	}
	reference, ok := local.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("anonymous type reference = %#v", reference)
	}
	id, ok := reference.AnonymousID()
	if !ok || id.Source() != "root.xsd" || id.Ordinal() == 0 {
		t.Fatalf("anonymous ID = %v/%v", id, ok)
	}
}

func TestGroupedExtensionResolvesAttributesBeforeBase(t *testing.T) {
	root := strings.Replace(groupedExtensionSchema("1.1", "", `<xs:attribute ref="t:missing"/>`), `base="t:Base"`, `base="t:MissingBase"`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("unresolved attribute and base were accepted")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != complexContentTestLoc(t, root, `ref="t:missing"`) {
		t.Fatalf("diagnostic = %v, want attribute reference first", err)
	}
}

func TestGroupedExtensionTargetAndUnsupportedBoundaries(t *testing.T) {
	base := groupedExtensionSchema("1.1", "", `<xs:attribute name="flag" type="xs:boolean"/>`)
	cases := []struct {
		name   string
		root   string
		class  FailureClass
		code   string
		cause  error
		marker string
	}{
		{
			name:   "wrong kind group target",
			root:   strings.Replace(base, `ref="t:Fields"`, `ref="t:root"`, 1),
			class:  FailureInvalid,
			code:   diagnosticSchemaModelGroupReferenceWrongKindCode,
			cause:  errSchemaModelGroupReferenceWrongKind,
			marker: `ref="t:root"`,
		},
		{
			name:   "unresolved attribute target",
			root:   groupedExtensionSchema("1.1", "", `<xs:attribute ref="t:missing"/>`),
			class:  FailureInvalid,
			code:   diagnosticSchemaAttributeReferenceUnresolvedCode,
			cause:  errSchemaAttributeReferenceUnresolved,
			marker: `ref="t:missing"`,
		},
		{
			name:   "wrong kind base target",
			root:   strings.Replace(base, `base="t:Base"`, `base="t:Fields"`, 1),
			class:  FailureInvalid,
			code:   invalidSchemaCompositionCode,
			cause:  errSchemaComplexTypeBaseWrongKind,
			marker: `base="t:Fields"`,
		},
		{
			name:   "identity-only local attribute type",
			root:   groupedExtensionSchema("1.1", "", `<xs:attribute name="flag" type="xs:ID"/>`),
			class:  FailureUnsupported,
			code:   UnsupportedSchemaSyntaxCode,
			cause:  ErrUnsupported,
			marker: `type="xs:ID"`,
		},
		{
			name:   "nonempty named base",
			root:   strings.Replace(base, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, 1),
			class:  FailureUnsupported,
			code:   UnsupportedSchemaSyntaxCode,
			cause:  errSchemaComplexTypeBaseNonEmpty,
			marker: `base="t:Base"`,
		},
		{
			name:   "extension wildcard",
			root:   strings.Replace(base, `</xs:extension>`, `<xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension>`, 1),
			class:  FailureUnsupported,
			code:   UnsupportedSchemaSyntaxCode,
			cause:  ErrUnsupported,
			marker: `<xs:anyAttribute`,
		},
		{
			name:   "extension open content",
			root:   strings.Replace(base, `<xs:group ref="t:Fields"`, `<xs:openContent mode="none"/><xs:group ref="t:Fields"`, 1),
			class:  FailureUnsupported,
			code:   UnsupportedSchemaSyntaxCode,
			cause:  ErrUnsupported,
			marker: `<xs:openContent`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("bounded extension failure was accepted")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != complexContentTestLoc(t, test.root, test.marker) || diagnostic.SpecRef() == "" || !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic = %v, want %s/%s at %q with cause %v", err, test.class, test.code, test.marker, test.cause)
			}
		})
	}
}

func TestGroupedExtensionBaseCycleRetainsReferenceLocations(t *testing.T) {
	root := groupedExtensionSchema("1.1", "", `<xs:attribute name="flag" type="xs:boolean"/>`)
	root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("cyclic grouped bases were accepted")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
		t.Fatalf("cycle diagnostic = %v", err)
	}
	if !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, `base="t:Base"`)}) {
		t.Fatalf("cycle related = %v, want first base reference", diagnostic.Related())
	}
}

func TestGroupedExtensionSourceAcquisitionFailureIsResolution(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:include schemaLocation="missing.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`
	source, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(root)})
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	cause := errors.New("source unavailable")
	resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}
	schema, err := discoverSchemaWithPolicy(source, resolver, Strict11)
	if err == nil {
		t.Fatal("missing included source was accepted")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureResolution || diagnostic.Code() != SourceResolveCode || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:include`) || !errors.Is(err, cause) {
		t.Fatalf("resolution diagnostic = %v", err)
	}
}

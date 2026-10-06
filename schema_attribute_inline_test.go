package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // One fixture verifies ordered public ownership and copied facts for every inline variety.
func TestGlobalAttributeInlineSimpleTypeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" finalDefault="restriction">
  <xs:attribute name="atomic"><xs:simpleType><xs:restriction base="xs:integer"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType></xs:attribute>
  <xs:attribute name="listed"><xs:simpleType><xs:list itemType="r:Later"/></xs:simpleType></xs:attribute>
  <xs:attribute name="united"><xs:simpleType><xs:union memberTypes="xs:language r:Later"><xs:simpleType><xs:restriction base="xs:string"><xs:enumeration value=""/></xs:restriction></xs:simpleType></xs:union></xs:simpleType></xs:attribute>
  <xs:simpleType name="Later"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			components := schema.Components()
			if len(components) != 4 {
				t.Fatalf("components = %d, want 4", len(components))
			}
			varieties := []SimpleTypeVariety{SimpleTypeVarietyAtomicRestriction, SimpleTypeVarietyList, SimpleTypeVarietyUnion}
			varietyTokens := []string{"<xs:restriction", "<xs:list", "<xs:union"}
			ids := make([]SimpleTypeID, 3)
			for index := range varieties {
				component := components[index]
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.ID() != component.ID() || declaration.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, index+2, "<xs:attribute") {
					t.Fatalf("attribute %d identity/location = %v/%s", index, declaration.ID(), declaration.Loc())
				}
				if declaration.DeclaredType() != (QName{}) {
					t.Fatalf("inline attribute %d has declared QName %q", index, declaration.DeclaredType())
				}
				if id, hasID := declaration.TypeID(); hasID || !id.IsZero() {
					t.Fatalf("inline attribute %d has named type ID %v/%t", index, id, hasID)
				}
				reference, ok := declaration.TypeReference()
				inline, hasInline := declaration.InlineSimpleType()
				id, hasID := reference.AnonymousID()
				if !ok || !hasInline || !hasID || id.IsZero() || !reference.IsAnonymous() || reference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, index+2, "<xs:simpleType") || reference.Variety() != varieties[index] || reference.VarietyLoc() != mustSchemaTokenLoc(t, "root.xsd", root, index+2, varietyTokens[index]) {
					t.Fatalf("inline attribute %d type facts = %#v/%t/%v/%s/%s", index, reference, hasInline, id, reference.Loc(), reference.VarietyLoc())
				}
				modelID, hasModelID := inline.NodeID()
				if !hasModelID || modelID != id || !inline.IsAnonymous() || inline.Loc() != reference.Loc() || inline.VarietyLoc() != reference.VarietyLoc() || inline.ID() != (ComponentID{}) || !reflect.DeepEqual(inline.Final(), []string{"restriction"}) || inline.FinalLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault") {
					t.Fatalf("inline attribute %d model ownership = %#v", index, inline)
				}
				ids[index] = id
			}
			firstInline, _ := components[0].AttributeDeclaration()
			firstType, _ := firstInline.InlineSimpleType()
			finalCopy := firstType.Final()
			finalCopy[0] = "changed"
			if !reflect.DeepEqual(firstType.Final(), []string{"restriction"}) {
				t.Fatal("mutating copied inline final controls changed Schema")
			}
			if ids[0].Ordinal() >= ids[1].Ordinal() || ids[1].Ordinal() >= ids[2].Ordinal() {
				t.Fatalf("inline IDs not in declaration order: %v", ids)
			}
			atomic, _ := components[0].AttributeDeclaration()
			atomicType, _ := atomic.InlineSimpleType()
			bounds, ok := atomicType.IntegerBounds()
			if !ok {
				t.Fatal("atomic inline restriction lost effective bounds")
			}
			minimum, hasMinimum := bounds.MinInclusive()
			minimumLoc, hasMinimumLoc := bounds.MinInclusiveLoc()
			if !hasMinimum || minimum.Canonical() != "2" || !hasMinimumLoc || minimumLoc != mustSchemaTokenLoc(t, "root.xsd", root, 2, `value="2"`) || bounds.HasMaxInclusive() {
				t.Fatalf("inline restriction bounds = %#v", bounds)
			}
			boundDeclarations := bounds.Declarations()
			if len(boundDeclarations) != 1 {
				t.Fatalf("inline restriction bound declarations = %d, want 1", len(boundDeclarations))
			}
			boundDeclarations[0] = IntegerBoundFacet{}
			if fresh, hasFresh := atomicType.IntegerBounds(); !hasFresh || len(fresh.Declarations()) != 1 || fresh.Declarations()[0].Loc() != minimumLoc {
				t.Fatal("mutating copied bound declarations changed Schema")
			}
			base, ok := atomicType.BaseReference()
			if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "integer") || base.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="xs:integer"`) {
				t.Fatalf("inline atomic base = %#v/%t", base, ok)
			}
			listed, _ := components[1].AttributeDeclaration()
			listType, _ := listed.InlineSimpleType()
			item, ok := listType.ItemType()
			itemID, hasItemID := item.ComponentID()
			if !ok || !hasItemID || !item.IsNamed() || itemID != components[3].ID() || item.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `itemType="r:Later"`) {
				t.Fatalf("inline list item = %#v/%t", item, ok)
			}
			united, _ := components[2].AttributeDeclaration()
			unionType, _ := united.InlineSimpleType()
			members := unionType.MemberTypes()
			if len(members) != 3 || !members[0].IsBuiltin() || members[0].Name().Local() != "language" || !members[1].IsNamed() || !members[2].IsAnonymous() {
				t.Fatalf("inline union members = %#v", members)
			}
			memberID, ok := members[1].ComponentID()
			if !ok || memberID != components[3].ID() {
				t.Fatalf("forward union member ID = %v/%t", memberID, ok)
			}
			nested, ok := members[2].AnonymousType()
			nestedID, hasNestedID := members[2].AnonymousID()
			if !ok || !hasNestedID || nestedID.Ordinal() <= ids[2].Ordinal() || nested.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType><xs:restriction`) || !reflect.DeepEqual(nested.Final(), []string{"restriction"}) || nested.FinalLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault") || len(nested.StringEnumerationFacets().Declarations()) != 1 || nested.StringEnumerationFacets().Declarations()[0].Value() != "" {
				t.Fatalf("nested union member facts = %#v/%v", nested, nestedID)
			}
			members[0] = SimpleTypeReference{}
			if again := unionType.MemberTypes(); len(again) != 3 || !again[0].IsBuiltin() {
				t.Fatal("union member slice mutation changed Schema")
			}
			walked := make([]ComponentID, 0, 4)
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			wantWalk := []ComponentID{components[0].ID(), components[1].ID(), components[2].ID(), components[3].ID()}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("Walk IDs = %v, want %v", walked, wantWalk)
			}
		})
	}
}

func TestGlobalAttributeInlineIntegratedIntegerDerivatives(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, test := range []struct {
			name, bound string
			minimum     bool
		}{
			{name: "positiveInteger", bound: "1", minimum: true},
			{name: "nonPositiveInteger", bound: "0"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + test.name + `"/></xs:simpleType></xs:attribute></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("inline %s attribute: %v", test.name, err)
				}
				if len(schema.Components()) != 1 {
					t.Fatalf("inline %s components = %d, want 1", test.name, len(schema.Components()))
				}
				declaration, ok := schema.Components()[0].AttributeDeclaration()
				inline, hasInline := declaration.InlineSimpleType()
				base, hasBase := inline.BaseReference()
				bounds, hasBounds := inline.IntegerBounds()
				if !ok || !hasInline || !hasBase || !hasBounds || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, test.name) || base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:`+test.name+`"`) {
					t.Fatalf("inline %s facts = %#v/%#v/%#v", test.name, declaration, base, bounds)
				}
				if test.minimum {
					minimum, hasMinimum := bounds.MinInclusive()
					if !hasMinimum || minimum.Canonical() != test.bound || bounds.HasMaxInclusive() {
						t.Fatalf("inline %s bounds = %#v", test.name, bounds)
					}
					return
				}
				maximum, hasMaximum := bounds.MaxInclusive()
				if !hasMaximum || maximum.Canonical() != test.bound || bounds.HasMinInclusive() {
					t.Fatalf("inline %s bounds = %#v", test.name, bounds)
				}
			})
		}
	}
}

//nolint:gocognit // Each diagnostic exit asserts public code, location, cause, and edition reference.
func TestGlobalAttributeInlineTypeExitsAcrossPolicies(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, code, spec, related string
			class                                   FailureClass
			cause                                   error
		}{
			{name: "type conflict", body: `<xs:attribute name="a" type="xs:integer"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>`, marker: `<xs:simpleType`, related: `type="xs:integer"`, code: invalidSchemaCompositionCode, spec: schemaAttributeTypeSpecRef(profile.version), class: FailureInvalid, cause: errSchemaAttributeInlineTypeConflict},
			{name: "duplicate child", body: `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>`, marker: `<xs:simpleType`, related: `<xs:simpleType`, code: invalidSchemaCompositionCode, spec: schemaAttributeTypeSpecRef(profile.version), class: FailureInvalid, cause: errSchemaAttributeInlineTypeDuplicate},
			{name: "unsupported value", body: `<xs:attribute name="a" fixed="2"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>`, marker: `fixed="2"`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeValueConstraintSpecRef(profile.version), class: FailureUnsupported, cause: errSchemaAttributeValueConstraintUnsupported},
			{name: "excluded builtin", body: `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:attribute>`, marker: `<xs:simpleType`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(profile.version), class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
			{name: "excluded list member", body: `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:nonNegativeInteger"/></xs:simpleType></xs:attribute>`, marker: `itemType="xs:nonNegativeInteger"`, related: `<xs:simpleType`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(profile.version), class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
			{name: "excluded union member", body: `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:QName"/></xs:simpleType></xs:attribute>`, marker: `memberTypes="xs:QName"`, related: `<xs:simpleType`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(profile.version), class: FailureUnsupported, cause: errSchemaAttributeTypeUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("failure returned a partial schema or no diagnostic")
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				if test.name == "duplicate child" {
					wantLoc = mustTestLoc(t, "root.xsd", 1, wantLoc.Column()+len(`<xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType>`))
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc {
					t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, wantLoc)
				}
				if test.spec != "" && diagnostic.SpecRef() != test.spec {
					t.Fatalf("SpecRef = %q, want %q", diagnostic.SpecRef(), test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.related != "" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
					t.Fatalf("related locations = %v, want inline owner", diagnostic.Related())
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations = %v", diagnostic.Related())
				}
			})
		}
	}
}

func TestGlobalAttributeInlineMalformedChildrenAcrossPolicies(t *testing.T) {
	tests := []struct {
		name, body, marker string
	}{
		{
			name:   "missing model",
			body:   `<xs:attribute name="a"><xs:simpleType/></xs:attribute>`,
			marker: `<xs:simpleType/>`,
		},
		{
			name:   "restriction without base",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:restriction/></xs:simpleType></xs:attribute>`,
			marker: `<xs:restriction/>`,
		},
		{
			name:   "list without item",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:list/></xs:simpleType></xs:attribute>`,
			marker: `<xs:list/>`,
		},
		{
			name:   "union without members",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:union/></xs:simpleType></xs:attribute>`,
			marker: `<xs:union/>`,
		},
		{
			name:   "restriction with two bases",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"><xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType></xs:restriction></xs:simpleType></xs:attribute>`,
			marker: `<xs:simpleType><xs:list`,
		},
		{
			name:   "list with two item sources",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:integer"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:list></xs:simpleType></xs:attribute>`,
			marker: `itemType="xs:integer"`,
		},
		{
			name:   "inline final",
			body:   `<xs:attribute name="a"><xs:simpleType final="restriction"><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>`,
			marker: `final="restriction"`,
		},
		{
			name:   "duplicate model",
			body:   `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"/><xs:list itemType="xs:integer"/></xs:simpleType></xs:attribute>`,
			marker: `<xs:list itemType=`,
		},
	}
	for _, profile := range curatorPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("malformed inline child returned a schema or no error")
				}
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeInlineTypeMalformed) {
					t.Fatalf("malformed inline diagnostic = %s related %v, want invalid composition at %s with edition reference and cause", diagnostic, diagnostic.Related(), wantLoc)
				}
				var original Diagnostic
				if !errors.As(diagnostic.Unwrap(), &original) || original.Code() != diagnostic.Code() || original.Loc() != wantLoc || original.Message() != diagnostic.Message() || original.SpecRef() != "" {
					t.Fatalf("original located composition error was not preserved: %v", diagnostic.Unwrap())
				}
			})
		}
	}
}

//nolint:gocognit // The graph fixture checks both imported and chameleon type identity at public use sites.
func TestGlobalAttributeInlineTypesResolveGraphMembers(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" xmlns:o="urn:o" targetNamespace="urn:r" finalDefault="restriction">
  <xs:include schemaLocation="child.xsd"/>
  <xs:import namespace="urn:o" schemaLocation="other.xsd"/>
  <xs:attribute name="imported"><xs:simpleType><xs:list itemType="o:Imported"/></xs:simpleType></xs:attribute>
  <xs:simpleType name="Forward"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" finalDefault="union">
  <xs:attribute name="chameleon"><xs:simpleType><xs:union memberTypes="r:Forward"/></xs:simpleType></xs:attribute>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:o" finalDefault="union"><xs:simpleType name="Imported"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: child},
				"other.xsd": {id: "other.xsd", contents: other},
			}
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("graph schema: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated graph build = %v, ordered facts changed", err)
			}
			if len(first.Documents()) != 3 {
				t.Fatalf("graph documents = %d, want 3", len(first.Documents()))
			}
			imported := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "imported"))
			chameleon := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "chameleon"))
			forward := first.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:r", "Forward"))
			named := first.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:o", "Imported"))
			if len(imported) != 1 || len(chameleon) != 1 || len(forward) != 1 || len(named) != 1 {
				t.Fatalf("graph component lookup = %d/%d/%d/%d", len(imported), len(chameleon), len(forward), len(named))
			}
			importedDecl, _ := imported[0].AttributeDeclaration()
			importedType, ok := importedDecl.InlineSimpleType()
			if !ok || importedDecl.ID().Source() != "root.xsd" || importedType.Variety() != SimpleTypeVarietyList || !reflect.DeepEqual(importedType.Final(), []string{"restriction"}) || importedType.FinalLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault") {
				t.Fatalf("imported inline attribute facts = %#v/%#v", importedDecl, importedType)
			}
			item, ok := importedType.ItemType()
			itemID, hasID := item.ComponentID()
			if !ok || !hasID || itemID != named[0].ID() || item.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `itemType="o:Imported"`) {
				t.Fatalf("imported list item = %#v/%t", item, ok)
			}
			chameleonDecl, _ := chameleon[0].AttributeDeclaration()
			chameleonType, ok := chameleonDecl.InlineSimpleType()
			if !ok || chameleonDecl.ID().Source() != "child.xsd" || chameleonType.Variety() != SimpleTypeVarietyUnion || !reflect.DeepEqual(chameleonType.Final(), []string{"union"}) || chameleonType.FinalLoc() != mustSchemaTokenLoc(t, "child.xsd", child, 1, "finalDefault") {
				t.Fatalf("chameleon inline attribute facts = %#v/%#v", chameleonDecl, chameleonType)
			}
			members := chameleonType.MemberTypes()
			if len(members) != 1 {
				t.Fatalf("chameleon union members = %d", len(members))
			}
			memberID, hasID := members[0].ComponentID()
			if !hasID || memberID != forward[0].ID() || members[0].Loc() != mustSchemaTokenLoc(t, "child.xsd", child, 2, `memberTypes="r:Forward"`) {
				t.Fatalf("chameleon forward member = %#v", members[0])
			}
		})
	}
}

//nolint:gocognit // Exercise both list and union member resolution exits across policies.
func TestGlobalAttributeInlineMemberResolutionDiagnostics(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, code string
			cause                    error
			related                  string
		}{
			{name: "unresolved list", body: `<xs:attribute name="a"><xs:simpleType><xs:list itemType="r:Missing"/></xs:simpleType></xs:attribute>`, marker: `itemType="r:Missing"`, code: diagnosticSchemaSimpleTypeUnresolvedCode, cause: errSchemaSimpleTypeBaseUnresolved},
			{name: "wrong kind list", body: `<xs:attribute name="a"><xs:simpleType><xs:list itemType="r:Thing"/></xs:simpleType></xs:attribute><xs:element name="Thing" type="xs:integer"/>`, marker: `itemType="r:Thing"`, code: diagnosticSchemaSimpleTypeWrongKindCode, cause: errSchemaSimpleTypeBaseWrongKind, related: `<xs:element name="Thing"`},
			{name: "unresolved union", body: `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="r:Missing"/></xs:simpleType></xs:attribute>`, marker: `memberTypes="r:Missing"`, code: diagnosticSchemaSimpleTypeUnresolvedCode, cause: errSchemaSimpleTypeBaseUnresolved},
			{name: "wrong kind union", body: `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="r:Thing"/></xs:simpleType></xs:attribute><xs:element name="Thing" type="xs:integer"/>`, marker: `memberTypes="r:Thing"`, code: diagnosticSchemaSimpleTypeWrongKindCode, cause: errSchemaSimpleTypeBaseWrongKind, related: `<xs:element name="Thing"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("member resolution error returned a schema or no error")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, test.cause) {
					t.Fatalf("member diagnostic = %s, want %s at %s with %v", diagnostic, test.code, test.marker, test.cause)
				}
				if test.related != "" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
					t.Fatalf("member related = %v, want target declaration", diagnostic.Related())
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected member related locations = %v", diagnostic.Related())
				}
			})
		}
	}
}

func TestGlobalAttributeInlineMembersRespectGraphVisibility(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, model := range []struct {
			name, body, marker string
		}{
			{"list", `<xs:list itemType="f:Hidden"/>`, `itemType="f:Hidden"`},
			{"union", `<xs:union memberTypes="f:Hidden"/>`, `memberTypes="f:Hidden"`},
		} {
			t.Run(profile.name+"/"+model.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/><xs:attribute name="a"><xs:simpleType>` + model.body + `</xs:simpleType></xs:attribute></xs:schema>`
				fixtures := map[string]discoveryFixture{
					"child.xsd":   {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`},
					"foreign.xsd": {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("invisible inline member published schema: %v", err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeUnresolvedCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, model.marker) || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
					t.Fatalf("invisible inline member diagnostic = %s related %v", diagnostic, diagnostic.Related())
				}
			})
		}
	}
}

func TestGlobalAttributeInlineInheritablePolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" inheritable="true"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("inline inheritable attribute: %v", err)
			}
			attributes := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "", "value"))
			if len(attributes) != 1 {
				t.Fatalf("attribute count = %d, want 1", len(attributes))
			}
			declaration, ok := attributes[0].AttributeDeclaration()
			if !ok || !declaration.IsInheritable() {
				t.Fatal("inline inheritable fact was lost")
			}
		})
	}
	assertStrict10GlobalAttributeInheritableMismatch(t, root)
	malformed := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" inheritable="maybe"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy)+" malformed", func(t *testing.T) {
			assertMalformedGlobalAttributeInheritable(t, malformed, policy)
		})
	}
}

//nolint:gocognit // Verify each identity-only built-in is retained under every policy.
func TestGlobalAttributeInlineIdentityOnlyBuiltins(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, builtin := range []string{"language", "NCName", "anyURI", "ID"} {
			t.Run(profile.name+"/"+builtin, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + builtin + `"/></xs:simpleType></xs:attribute></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("identity-only inline type: %v", err)
				}
				declaration, ok := schema.Components()[0].AttributeDeclaration()
				if !ok {
					t.Fatal("inline attribute declaration missing")
				}
				inline, ok := declaration.InlineSimpleType()
				base, hasBase := inline.BaseReference()
				if !ok || !hasBase || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, builtin) || base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:`+builtin+`"`) {
					t.Fatalf("identity-only base = %#v/%t", base, hasBase)
				}
			})
		}
	}
}

func TestGlobalAttributeInlineListAndUnionReferencesStayUnsupported(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, model := range []struct{ name, body string }{
			{"list", `<xs:list itemType="xs:integer"/>`},
			{"union", `<xs:union memberTypes="xs:integer"/>`},
		} {
			t.Run(profile.name+"/"+model.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a"><xs:simpleType>` + model.body + `</xs:simpleType></xs:attribute><xs:complexType name="Box"><xs:attribute ref="r:a"/></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("local reference to inline list/union returned Schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:a"`) || diagnostic.SpecRef() != schemaAttributeUseSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}) {
					t.Fatalf("inline %s ref diagnostic = %s related %v", model.name, diagnostic, diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Check generation and validation as separate public consumers.
func TestGlobalAttributeInlineConsumerBoundary(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">
  <xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute>
  <xs:complexType name="Box"><xs:attribute ref="r:a"/></xs:complexType>
  <xs:element name="root" type="r:Box"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("inline attribute use schema: %v", err)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatal("GenerateGo accepted inline global attribute or returned output")
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:attribute ref="r:a"`) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic = %s", generationDiagnostic)
			}
			generationRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:r"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute><xs:element name="root" type="xs:integer"/></xs:schema>`
			generationSchema, err := discoverTestSchemaWithPolicy(t, generationRoot, nil, profile.policy)
			if err != nil {
				t.Fatalf("direct global inline consumer schema: %v", err)
			}
			output, generationErr = GenerateGo(generationSchema, "generated")
			if output != nil || generationErr == nil {
				t.Fatal("GenerateGo returned output for a global inline attribute")
			}
			generationDiagnostic = requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != elementReferenceTestAttributeLoc(t, generationRoot, `<xs:attribute name="a"`) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("direct global generation diagnostic = %s", generationDiagnostic)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:r"/>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted an attribute-bearing type")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:attribute ref="r:a"`) || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("validation diagnostic = %s related %v, cause %v", validationDiagnostic, validationDiagnostic.Related(), validationErr)
			}
		})
	}
}

func TestGlobalAttributeInlineListOwnsNestedAnonymousItem(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="items"><xs:simpleType><xs:list><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:list></xs:simpleType></xs:attribute></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("nested list item: %v", err)
			}
			declaration, ok := schema.Components()[0].AttributeDeclaration()
			if !ok {
				t.Fatal("global attribute declaration missing")
			}
			inline, ok := declaration.InlineSimpleType()
			outerID, hasOuterID := inline.NodeID()
			item, hasItem := inline.ItemType()
			itemID, hasItemID := item.AnonymousID()
			itemType, hasItemType := item.AnonymousType()
			if !ok || !hasOuterID || !hasItem || !hasItemID || !hasItemType || !item.IsAnonymous() || outerID.IsZero() || itemID.Ordinal() <= outerID.Ordinal() || item.Loc() != itemType.Loc() || itemType.Variety() != SimpleTypeVarietyAtomicRestriction {
				t.Fatalf("nested list identities = %v/%v, item %#v", outerID, itemID, item)
			}
			base, hasBase := itemType.BaseReference()
			if !hasBase || base.Name() != mustTestQName(t, testXSDNamespace, "integer") || base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="xs:integer"`) {
				t.Fatalf("nested list base = %#v/%t", base, hasBase)
			}
		})
	}
}

func TestGlobalAttributeInlineCyclePreservesAttributeContext(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a"><xs:simpleType><xs:restriction base="r:First"/></xs:simpleType></xs:attribute><xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("cyclic inline member returned a schema or no error")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeTypeCycleCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType><xs:restriction base="r:First"`) || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || len(diagnostic.Related()) == 0 || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("inline cycle diagnostic = %s related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

//nolint:gocognit // Compare the edition-selected default and the named base override at the public boundary.
func TestGlobalAttributeInlineFinalDefaultAndNamedOverride(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			allRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" finalDefault="#all"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:attribute></xs:schema>`
			allSchema, err := discoverTestSchemaWithPolicy(t, allRoot, nil, profile.policy)
			if err != nil {
				t.Fatalf("inline #all default: %v", err)
			}
			allAttribute, ok := allSchema.Components()[0].AttributeDeclaration()
			allType, hasType := allAttribute.InlineSimpleType()
			wantFinal := issue459ProjectFinalDefault([]string{"extension", "restriction", "list", "union"}, profile.version)
			if !ok || !hasType || !reflect.DeepEqual(allType.Final(), wantFinal) || allType.FinalLoc() != elementReferenceTestAttributeLoc(t, allRoot, `finalDefault="#all"`) {
				t.Fatalf("inline final = %#v/%s, want %#v at document default", allType.Final(), allType.FinalLoc(), wantFinal)
			}

			rootPrefix := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" finalDefault="restriction"><xs:attribute name="a"><xs:simpleType><xs:restriction base="r:Base"/></xs:simpleType></xs:attribute>`
			overriddenRoot := rootPrefix + `<xs:simpleType name="Base" final=""><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
			overridden, err := discoverTestSchemaWithPolicy(t, overriddenRoot, nil, profile.policy)
			if err != nil {
				t.Fatalf("named base explicit final override: %v", err)
			}
			attribute := overridden.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "a"))
			base := overridden.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:r", "Base"))
			if len(attribute) != 1 || len(base) != 1 {
				t.Fatalf("attribute/base counts = %d/%d", len(attribute), len(base))
			}
			declaration, _ := attribute[0].AttributeDeclaration()
			inline, ok := declaration.InlineSimpleType()
			baseType, hasBase := base[0].SimpleTypeDefinition()
			if !ok || !hasBase || !reflect.DeepEqual(inline.Final(), []string{"restriction"}) || inline.FinalLoc() != elementReferenceTestAttributeLoc(t, overriddenRoot, `finalDefault="restriction"`) || len(baseType.Final()) != 0 || !baseType.FinalLoc().IsZero() {
				t.Fatalf("effective final/override = %#v/%s, %#v/%s", inline.Final(), inline.FinalLoc(), baseType.Final(), baseType.FinalLoc())
			}

			blockedRoot := rootPrefix + `<xs:simpleType name="Base"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
			blocked, err := discoverTestSchemaWithPolicy(t, blockedRoot, nil, profile.policy)
			if err == nil || blocked.storage != nil {
				t.Fatal("inherited named final allowed inline restriction or returned Schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, blockedRoot, `base="r:Base"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, blockedRoot, `finalDefault="restriction"`)}) || diagnostic.SpecRef() != schemaSimpleTypeRestrictionSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("inherited named final diagnostic = %s related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestGlobalAttributeNestedAnonymousFinalDefaultBlocksUnion(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" finalDefault="union"><xs:attribute name="a"><xs:simpleType><xs:union><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:union></xs:simpleType></xs:attribute></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("nested anonymous final allowed prohibited union or returned Schema")
			}
			diagnostic := requireDiagnostic(t, err)
			wantLoc := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType><xs:restriction`)
			wantRelated := elementReferenceTestAttributeLoc(t, root, `finalDefault="union"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != wantLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || diagnostic.SpecRef() != schemaSimpleTypeRestrictionSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("nested final diagnostic = %s related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

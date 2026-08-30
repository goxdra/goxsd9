package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaDirectSubstitutionGroupXSD10PreservesHeadIDAndLocation(t *testing.T) {
	memberLine := `  <xs:element name="member" type="xs:integer" substitutionGroup="r:head"/>`
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
` + memberLine + `
	  <xs:element name="head" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	member, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	head, ok := components[1].ElementDeclaration()
	if !ok {
		t.Fatal("head has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{head.ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member affiliations = %#v, want %#v", got, want)
	}
	wantLoc := mustTestLoc(t, "root.xsd", 2, strings.Index(memberLine, "substitutionGroup")+1)
	if got, want := member.SubstitutionGroupAffiliationLocations(), []Loc{wantLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member affiliation locations = %#v, want %#v", got, want)
	}
}

func TestSchemaDirectSubstitutionGroupXSD10RejectsImportedUntypedHead(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:c="urn:child" targetNamespace="urn:root" version="1.0">
  <xs:import namespace="urn:child" schemaLocation="child.xsd"/>
  <xs:element name="member" type="xs:boolean" substitutionGroup="c:head"/>
</xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child" version="1.0">
  <xs:element name="head"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: child},
	}, Strict10)
	assertSchemaSubstitutionFailure(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, "", 0)
}

func TestSchemaDirectSubstitutionGroupXSD10ResolvesBackwardDeclaration(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
	  <xs:element name="head" type="xs:integer"/>
	  <xs:element name="member" type="xs:integer" substitutionGroup="r:head"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	member, ok := schema.Components()[1].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{schema.Components()[0].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member affiliations = %#v, want %#v", got, want)
	}
}

func TestSchemaDirectSubstitutionGroupXSD11PreservesOrderedHeadsAndEmptyAbsence(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="xs:integer" substitutionGroup="r:second r:first"/>
	  <xs:element name="second" type="xs:integer"/>
	  <xs:element name="first" type="xs:integer"/>
  <xs:element name="empty" type="xs:boolean" substitutionGroup="   "/>
  <xs:element name="absent" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 5; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	member, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID(), components[2].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member affiliations = %#v, want %#v", got, want)
	}
	locations := member.SubstitutionGroupAffiliationLocations()
	if len(locations) != 2 || locations[0] != locations[1] || locations[0].IsZero() {
		t.Fatalf("member affiliation locations = %#v, want two copies of the attribute location", locations)
	}
	empty, ok := components[3].ElementDeclaration()
	if !ok {
		t.Fatal("empty-list declaration has no element view")
	}
	if got := empty.SubstitutionGroupAffiliations(); got != nil {
		t.Fatalf("empty-list affiliations = %#v, want nil", got)
	}
	absent, ok := components[4].ElementDeclaration()
	if !ok {
		t.Fatal("absent declaration has no element view")
	}
	if got := absent.SubstitutionGroupAffiliations(); got != nil {
		t.Fatalf("absent affiliations = %#v, want nil", got)
	}

	ids := member.SubstitutionGroupAffiliations()
	ids[0] = ComponentID{}
	locations[0] = Loc{}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID(), components[2].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("member affiliations changed after slice mutation = %#v, want %#v", got, want)
	}
	if got := member.SubstitutionGroupAffiliationLocations(); got[0].IsZero() {
		t.Fatal("member affiliation location changed after slice mutation")
	}
}

func TestSchemaDirectSubstitutionGroupXSD11DeduplicatesExpandedQNames(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:alias="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="xs:integer" substitutionGroup="r:head alias:head r:head"/>
  <xs:element name="head" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	member, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deduplicated affiliations = %#v, want %#v", got, want)
	}
	if got := member.SubstitutionGroupAffiliationLocations(); len(got) != 1 || got[0].IsZero() {
		t.Fatalf("deduplicated affiliation locations = %#v, want one located first occurrence", got)
	}
}

func TestSchemaDirectSubstitutionGroupAcceptsProvenIntegerToDecimal(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
		ver    string
	}{
		{name: "XSD 1.0", policy: Strict10, ver: "1.0"},
		{name: "XSD 1.1", policy: Strict11, ver: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + test.ver + `">
  <xs:element name="member" type="xs:integer" substitutionGroup="r:head"/>
  <xs:element name="head" type="xs:decimal"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			member, ok := components[0].ElementDeclaration()
			if !ok {
				t.Fatal("member has no element view")
			}
			if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID()}; !reflect.DeepEqual(got, want) {
				t.Fatalf("integer-to-decimal affiliations = %#v, want %#v", got, want)
			}
		})
	}
}

func TestSchemaDirectSubstitutionGroupAcceptsNamedRestrictionToBuiltinBase(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="r:Boolean" substitutionGroup="r:head"/>
  <xs:element name="head" type="xs:boolean"/>
  <xs:simpleType name="Boolean"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	member, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("named restriction affiliations = %#v, want %#v", got, want)
	}
}

func TestSchemaDirectSubstitutionGroupRejectsProvenDecimalToInteger(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="xs:decimal" substitutionGroup="r:head"/>
  <xs:element name="head" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, diagnosticSchemaSubstitutionTypeCode, schemaSubstitutionConstraintSpecRef(XSDVersion11), 1)
	if !errors.Is(err, errSchemaSubstitutionTypeInvalid) {
		t.Fatalf("decimal-to-integer diagnostic cause = %v, want errSchemaSubstitutionTypeInvalid", err)
	}
	if diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("decimal-to-integer diagnostic location = %s, want root.xsd", diagnostic.Loc())
	}
}

func TestSchemaDirectSubstitutionGroupRejectsCrossFamilyTypes(t *testing.T) {
	for _, test := range []struct {
		name       string
		memberType string
		headType   string
	}{
		{name: "integer member and boolean head", memberType: "integer", headType: "boolean"},
		{name: "boolean member and integer head", memberType: "boolean", headType: "integer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="xs:` + test.memberType + `" substitutionGroup="r:head"/>
  <xs:element name="head" type="xs:` + test.headType + `"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, diagnosticSchemaSubstitutionTypeCode, schemaSubstitutionConstraintSpecRef(XSDVersion11), 1)
			if !errors.Is(err, errSchemaSubstitutionTypeInvalid) {
				t.Fatalf("cross-family diagnostic cause = %v, want errSchemaSubstitutionTypeInvalid", err)
			}
			if diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("cross-family diagnostic location = %s, want root.xsd", diagnostic.Loc())
			}
		})
	}
}

func TestSchemaDirectSubstitutionGroupKeepsUnknownNumericDerivationUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" type="r:Integer" substitutionGroup="r:head"/>
  <xs:element name="head" type="r:Decimal"/>
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Decimal"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, "", 0)
	if diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSubstitutionTypeUnsupported) {
		t.Fatalf("unknown numeric derivation diagnostic = %s/%v, want schema syntax unsupported with cause", diagnostic, err)
	}
}

func TestSchemaDirectSubstitutionGroupRejectsMultiNodeCycle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="first" type="xs:integer" substitutionGroup="r:second"/>
  <xs:element name="second" type="xs:integer" substitutionGroup="r:third"/>
  <xs:element name="third" type="xs:integer" substitutionGroup="r:first"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, diagnosticSchemaSubstitutionCycleCode, schemaSubstitutionConstraintSpecRef(XSDVersion11), 2)
	if !errors.Is(err, errSchemaSubstitutionCycle) {
		t.Fatalf("cycle diagnostic cause = %v, want errSchemaSubstitutionCycle", err)
	}
	if diagnostic.Loc() != mustTestLoc(t, "root.xsd", 4, strings.Index(`  <xs:element name="third" type="xs:integer" substitutionGroup="r:first"/>`, "substitutionGroup")+1) {
		t.Fatalf("cycle diagnostic location = %s, want third affiliation", diagnostic.Loc())
	}
}

func TestSchemaDirectSubstitutionGroupRejectsInlineSource(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="member" substitutionGroup="r:head"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>
  <xs:element name="head" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, "", 0)
	if diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("inline-source diagnostic = %s/%v, want schema syntax unsupported", diagnostic, err)
	}
}

func TestSchemaDirectSubstitutionGroupRejectsXSD10InvalidCardinality(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty", value: "   "},
		{name: "multiple", value: "r:first r:second"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0"><xs:element name="member" type="xs:integer" substitutionGroup="` + test.value + `"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, schemaSubstitutionAffiliationXSD10SpecRef, 0)
			if diagnostic.Unwrap() == nil {
				t.Fatal("cardinality diagnostic has no cause")
			}
		})
	}
}

func TestSchemaDirectSubstitutionGroupRejectsMalformedAndUnboundQNames(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "malformed", value: "r:bad:head"},
		{name: "unbound", value: "u:head"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1"><xs:element name="member" type="xs:integer" substitutionGroup="` + test.value + `"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, invalidSchemaConditionalCode, schemaSubstitutionQNameXSD11SpecRef, 0)
			if diagnostic.Unwrap() == nil {
				t.Fatal("QName diagnostic has no cause")
			}
		})
	}
}

//nolint:gocognit // Keep the edition and diagnostic precedence matrix together.
func TestSchemaDirectSubstitutionGroupReportsDeterministicHeadErrors(t *testing.T) {
	for _, edition := range []struct {
		name    string
		policy  LanguagePolicy
		version string
		ref     string
	}{
		{name: "XSD 1.0", policy: Strict10, version: "1.0", ref: schemaSubstitutionResolveXSD10SpecRef},
		{name: "XSD 1.1", policy: Strict11, version: "1.1", ref: schemaSubstitutionResolveXSD11SpecRef},
	} {
		t.Run(edition.name, func(t *testing.T) {
			cases := []struct {
				name          string
				declarations  string
				code          string
				related       int
				constraintRef bool
			}{
				{
					name:         "missing",
					declarations: `<xs:element name="member" type="xs:integer" substitutionGroup="r:missing"/>`,
					code:         diagnosticSchemaSubstitutionUnresolvedCode,
				},
				{
					name:         "wrong kind",
					declarations: `<xs:simpleType name="head"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="member" type="xs:integer" substitutionGroup="r:head"/>`,
					code:         diagnosticSchemaSubstitutionWrongKindCode,
					related:      1,
				},
				{
					name:          "self",
					declarations:  `<xs:element name="member" type="xs:integer" substitutionGroup="r:member"/>`,
					code:          diagnosticSchemaSubstitutionSelfCode,
					related:       1,
					constraintRef: true,
				},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + edition.version + `">` + test.declarations + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, edition.policy)
					wantRef := edition.ref
					if test.constraintRef {
						wantRef = schemaSubstitutionConstraintSpecRef(XSDVersion(edition.version))
					}
					diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, test.code, wantRef, test.related)
					if diagnostic.Unwrap() == nil {
						t.Fatal("head diagnostic has no cause")
					}
				})
			}
		})
	}
}

func TestSchemaDirectSubstitutionGroupReportsAmbiguousHeadAtResolution(t *testing.T) {
	headName := mustTestQName(t, "urn:root", "head")
	sourceName := mustTestQName(t, "urn:root", "member")
	sourceLoc := mustTestLoc(t, "root.xsd", 2, 3)
	firstHeadLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondHeadLoc := mustTestLoc(t, "second.xsd", 2, 3)
	source := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 1},
		kind: ComponentKindElementDeclaration,
		name: sourceName,
		loc:  sourceLoc,
		element: &schemaElementInput{
			substitutionGroup: []schemaElementSubstitutionGroupInput{{name: headName, loc: sourceLoc}},
		},
	}
	records := []schemaComponentRecord{
		source,
		{id: ComponentID{source: "first.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: headName, loc: firstHeadLoc},
		{id: ComponentID{source: "second.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: headName, loc: secondHeadLoc},
	}
	_, err := resolveSchemaElementSubstitutionGroup(
		0,
		records[0],
		records,
		map[QName][]int{headName: {1, 2}},
		nil,
		map[SourceID]string{"root.xsd": "urn:root"},
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous head unexpectedly resolved")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSubstitutionAmbiguousCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSubstitutionAmbiguousCode)
	}
	if got, want := diagnostic.Related(), []Loc{firstHeadLoc, secondHeadLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ambiguous head related locations = %#v, want %#v", got, want)
	}
	if diagnostic.SpecRef() != schemaSubstitutionResolveXSD11SpecRef || errors.Unwrap(err) == nil {
		t.Fatalf("ambiguous head diagnostic metadata/cause = %q/%v", diagnostic.SpecRef(), errors.Unwrap(err))
	}
}

func TestSchemaDirectSubstitutionGroupRequiresDirectForeignImport(t *testing.T) {
	for _, edition := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "XSD 1.0", policy: Strict10, version: "1.0"},
		{name: "XSD 1.1", policy: Strict11, version: "1.1"},
	} {
		t.Run(edition.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root" version="` + edition.version + `">
  <xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>
  <xs:element name="member" type="xs:integer" substitutionGroup="f:head"/>
</xs:schema>`
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="` + edition.version + `">
  <xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/>
</xs:schema>`
			foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" version="` + edition.version + `"><xs:element name="head"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"bridge.xsd":  {id: "bridge.xsd", contents: bridge},
				"foreign.xsd": {id: "foreign.xsd", contents: foreign},
			}, edition.policy)
			diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureInvalid, diagnosticSchemaSubstitutionImportCode, schemaSubstitutionConstraintSpecRef(XSDVersion(edition.version)), 1)
			if diagnostic.Unwrap() == nil {
				t.Fatal("foreign-import diagnostic has no cause")
			}
		})
	}
}

func TestSchemaDirectSubstitutionGroupResolvesDirectForeignImport(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root" version="1.1">
  <xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/>
  <xs:element name="member" type="xs:integer" substitutionGroup="f:head"/>
</xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" version="1.1">
  <xs:element name="head" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"foreign.xsd": {id: "foreign.xsd", contents: foreign},
	}, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	member, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("member has no element view")
	}
	if got, want := member.SubstitutionGroupAffiliations(), []ComponentID{components[1].ID()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("foreign affiliations = %#v, want %#v", got, want)
	}
}

func TestSchemaDirectSubstitutionGroupKeepsExcludedSourcesUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="Record"><xs:choice/></xs:complexType>
  <xs:element name="member" type="r:Record" substitutionGroup="r:head"/>
  <xs:element name="head"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	diagnostic := assertSchemaSubstitutionFailure(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, "", 0)
	if diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.SpecRef() == "" {
		t.Fatalf("unsupported diagnostic feature/specification = %q/%q, want schema syntax and a reference", diagnostic.Feature(), diagnostic.SpecRef())
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported diagnostic does not match ErrUnsupported: %v", err)
	}
}

func assertSchemaSubstitutionFailure(t *testing.T, schema Schema, err error, class FailureClass, code, specRef string, related int) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("discoverSchema accepted an invalid substitutionGroup affiliation")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, class, code)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want located root.xsd diagnostic", diagnostic.Loc())
	}
	if specRef != "" && diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic specification reference = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if got := len(diagnostic.Related()); got != related {
		t.Fatalf("diagnostic related location count = %d, want %d", got, related)
	}
	return diagnostic
}

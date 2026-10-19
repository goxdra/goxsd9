package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedRestrictionBase() string {
	return `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
}

//nolint:gocognit // Each public consumer is checked independently for each admitted owner shape.
func TestNamedWildcardRestrictionConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		for _, shape := range []struct{ name, body string }{
			{"named type", `<xs:element name="root" type="t:Derived"/>` + namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax"))},
			{"inline type", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + namedRestrictionWildcard("##other", "lax") + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := namedRestrictionSchema(string(profile.version), "urn:n", shape.body+namedRestrictionBase())
				factVersion := profile.version
				if profile.policy == Compatibility {
					factVersion = XSDVersion11
				}
				declarationLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
				definitionLoc := complexContentTestLoc(t, root, `<xs:complexType`)
				if shape.name == "named type" {
					definitionLoc = complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				}
				contentLoc := complexContentTestLoc(t, root, `<xs:complexContent>`)
				restrictionLoc := complexContentTestLoc(t, root, `<xs:restriction base="t:Base"`)
				baseLoc := complexContentTestLoc(t, root, `base="t:Base"`)
				wildcardLoc := complexContentTestLoc(t, root, `<xs:anyAttribute`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if shape.name == "inline type" {
					components := schema.Components()
					declaration, ok := components[0].ElementDeclaration()
					if !ok {
						t.Fatal("inline owner has no element declaration")
					}
					inline, ok := declaration.InlineComplexType()
					if !ok || inline.Particle() != nil || len(inline.AttributeUses()) != 0 ||
						inline.Derivation() != ComplexTypeDerivationRestriction || inline.DerivationLoc() != restrictionLoc || inline.BaseLoc() != baseLoc {
						t.Fatalf("inline restriction facts = %v/%v", inline, ok)
					}
					baseRef, ok := inline.BaseReference()
					id, hasID := baseRef.ComponentID()
					wildcard, hasWildcard := inline.AnyAttribute()
					if !ok || !hasID || id != components[1].ID() || baseRef.Loc() != baseLoc || !hasWildcard || wildcard.Loc() != wildcardLoc {
						t.Fatalf("inline base/wildcard = %v/%v", baseRef, wildcard)
					}
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:n"/>`)))
				if validationErr == nil {
					t.Fatal("validation accepted restriction")
				}
				validation := requireDiagnostic(t, validationErr)
				wantValidationRelated := []Loc{declarationLoc, definitionLoc, contentLoc, restrictionLoc, baseLoc, wildcardLoc}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode ||
					validation.Feature() != FeatureInstanceValidation || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) ||
					!reflect.DeepEqual(validation.Related(), wantValidationRelated) || validation.SpecRef() != instanceValidationSpecRef(factVersion) ||
					!errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceOpenAttrsType) {
					t.Fatalf("validation diagnostic = %v", validationErr)
				}
				generated, generationErr := GenerateGo(schema, "generated")
				if generationErr == nil || generated != nil {
					t.Fatalf("generation = %q/%v", generated, generationErr)
				}
				generation := requireDiagnostic(t, generationErr)
				wantGenerationRelated := []Loc{definitionLoc, contentLoc, restrictionLoc, baseLoc, wildcardLoc}
				wantGenerationSpec := codegenDirectSequenceSpecReference(factVersion, codegenDirectSequenceParticlesReference)
				wantGenerationPrimary := definitionLoc
				if shape.name == "inline type" {
					wantGenerationRelated = []Loc{definitionLoc, restrictionLoc, baseLoc, wildcardLoc}
					wantGenerationSpec = schemaElementTypeSpecRef(factVersion)
					wantGenerationPrimary = declarationLoc
				}
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
					generation.Feature() != FeatureCodegen || generation.Loc() != wantGenerationPrimary ||
					!reflect.DeepEqual(generation.Related(), wantGenerationRelated) || generation.SpecRef() != wantGenerationSpec ||
					!errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %v", generationErr)
				}
			})
		}
	}
}

func namedRestrictionOwner(base, wildcard string) string {
	return `<xs:complexType name="Derived"><xs:complexContent><xs:restriction base="` + base + `">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType>`
}

func namedRestrictionWildcard(namespace, process string) string {
	return `<xs:anyAttribute namespace="` + namespace + `" processContents="` + process + `"/>`
}

func namedRestrictionSchema(version, namespace, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="` + namespace + `" targetNamespace="` + namespace + `" version="` + version + `">` + body + `</xs:schema>`
}

func namedRestrictionProfiles() []struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
} {
	return []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{"strict10", Strict10, XSDVersion10},
		{"strict11", Strict11, XSDVersion11},
		{"compat10", Compatibility, XSDVersion10},
		{"compat11", Compatibility, XSDVersion11},
	}
}

//nolint:gocognit // The positive contract includes identity, source order, copies, and exact locations.
func TestNamedWildcardRestrictionPublishesCompletedBaseFacts(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			wildcard := namedRestrictionWildcard("##other", "lax")
			root := namedRestrictionSchema(string(profile.version), "urn:n",
				namedRestrictionOwner("t:Base", wildcard)+namedRestrictionBase())
			var first []Component
			for attempt := range 2 {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("parse attempt %d: %v", attempt, err)
				}
				components := schema.Components()
				if len(components) != 2 || components[0].Name().Local() != "Derived" || components[1].Name().Local() != "Base" {
					t.Fatalf("source order = %v", components)
				}
				derived, ok := components[0].ComplexType()
				if !ok {
					t.Fatal("derived complex type is missing")
				}
				if derived.ID() != components[0].ID() || derived.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) ||
					derived.Derivation() != ComplexTypeDerivationRestriction ||
					derived.DerivationLoc() != complexContentTestLoc(t, root, `<xs:restriction base="t:Base"`) ||
					derived.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) ||
					derived.Base().Namespace() != "urn:n" || derived.Base().Local() != "Base" ||
					derived.Particle() != nil || len(derived.AttributeUses()) != 0 {
					t.Fatalf("derived facts = %+v", derived)
				}
				baseRef, ok := derived.BaseReference()
				baseID, hasID := baseRef.ComponentID()
				if !ok || !baseRef.IsNamed() || !hasID || baseID != components[1].ID() || baseRef.Loc() != derived.BaseLoc() || baseRef.Name() != derived.Base() {
					t.Fatalf("base reference = %v/%v", baseRef, ok)
				}
				attribute, ok := derived.AnyAttribute()
				if !ok || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" ||
					attribute.Loc() != complexContentTestLoc(t, root, wildcard) ||
					attribute.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) ||
					attribute.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("wildcard = %+v/%v", attribute, ok)
				}
				if _, found := schema.Lookup(baseID); !found {
					t.Fatal("base ID was not published")
				}
				components[0] = Component{}
				if schema.Components()[0].Name().Local() != "Derived" {
					t.Fatal("Components returned mutable slice")
				}
				if attempt == 0 {
					first = schema.Components()
					continue
				}
				if !reflect.DeepEqual(first, schema.Components()) {
					t.Fatal("repeated component facts changed")
				}
			}
		})
	}
}

func TestNamedWildcardRestrictionSubsetAndEffectiveEmptyBase(t *testing.T) { //nolint:gocognit // Subset and effective-body facts are one positive matrix.
	for _, profile := range namedRestrictionProfiles() {
		for _, tc := range []struct{ name, base, wildcard, baseName string }{
			{"stronger process", namedRestrictionBase(), namedRestrictionWildcard("##other", "strict"), "Base"},
			{"namespace subset", namedRestrictionBase(), namedRestrictionWildcard("urn:outside", "lax"), "Base"},
			{"empty extension chain", namedRestrictionBase() + `<xs:complexType name="Middle"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, namedRestrictionWildcard("##other", "lax"), "Middle"},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := namedRestrictionSchema(string(profile.version), "urn:n", namedRestrictionOwner("t:"+tc.baseName, tc.wildcard)+tc.base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("valid subset or empty chain: %v", err)
				}
				components := schema.Components()
				derived, ok := components[0].ComplexType()
				reference, hasRef := derived.BaseReference()
				id, hasID := reference.ComponentID()
				if !ok || !hasRef || !hasID || derived.Particle() != nil || len(derived.AttributeUses()) != 0 ||
					derived.Derivation() != ComplexTypeDerivationRestriction || reference.Name().Local() != tc.baseName {
					t.Fatalf("derived facts = %v/%v/%v", derived, reference, components)
				}
				base, found := schema.Lookup(id)
				if !found || base.Name().Local() != tc.baseName {
					t.Fatalf("base ID = %v/%v", base, found)
				}
				wildcard, hasWildcard := derived.AnyAttribute()
				if !hasWildcard || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) ||
					wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="`+wildcard.Namespace()+`"`) {
					t.Fatalf("derived wildcard = %v/%v", wildcard, hasWildcard)
				}
				if tc.name == "namespace subset" {
					namespaces := wildcard.NamespaceConstraint().Namespaces()
					if !reflect.DeepEqual(namespaces, []string{"urn:outside"}) {
						t.Fatalf("expanded namespace set = %v", namespaces)
					}
					namespaces[0] = "mutated"
					if !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), []string{"urn:outside"}) {
						t.Fatal("wildcard namespace slice aliases schema")
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each alternate exit checks its complete public diagnostic.
func TestNamedWildcardRestrictionPreflightAndSubsetDiagnostics(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		cases := []struct {
			name, base, owner, marker, related, related2 string
			class                                        FailureClass
			code                                         string
			cause                                        error
		}{
			{"missing", "", namedRestrictionOwner("t:Missing", namedRestrictionWildcard("##other", "lax")), `base="t:Missing"`, "", "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnresolved},
			{"wrong kind", `<xs:simpleType name="Base"><xs:restriction base="xs:string"/></xs:simpleType>`, namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax")), `base="t:Base"`, `<xs:simpleType name="Base"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseWrongKind},
			{"builtin simple", "", namedRestrictionOwner("xs:string", namedRestrictionWildcard("##other", "lax")), `base="xs:string"`, "", "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseWrongKind},
			{"final", strings.Replace(namedRestrictionBase(), `name="Base"`, `name="Base" final="restriction"`, 1), namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax")), `base="t:Base"`, `final="restriction"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnsupported},
			{"no base wildcard", `<xs:complexType name="Base"/>`, namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax")), `<xs:anyAttribute`, `<xs:complexType name="Base"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeWildcardRestriction},
			{"broader namespace", namedRestrictionBase(), namedRestrictionOwner("t:Base", namedRestrictionWildcard("##any", "lax")), `namespace="##any"`, `namespace="##other"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeWildcardRestriction},
			{"weaker process", namedRestrictionBase(), namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "skip")), `processContents="skip"`, `processContents="lax"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeWildcardRestriction},
			{"nonempty base", `<xs:complexType name="Base"><xs:choice><xs:element name="v" type="xs:integer"/></xs:choice></xs:complexType>`, namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax")), `base="t:Base"`, `<xs:complexType name="Base"`, `<xs:choice>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseUnsupported},
			{"base attribute uses", `<xs:complexType name="Base"><xs:attribute name="a" type="xs:boolean"/></xs:complexType>`, namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax")), `base="t:Base"`, `<xs:complexType name="Base"`, `<xs:attribute name="a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseUnsupported},
		}
		for _, tc := range cases {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := namedRestrictionSchema(string(profile.version), "urn:n", tc.owner+tc.base)
				factVersion := profile.version
				if profile.policy == Compatibility {
					factVersion = XSDVersion11
				}
				var first Diagnostic
				for attempt := range 2 {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("restriction unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if tc.related != "" {
						relatedLoc := complexContentTestLoc(t, root, tc.related)
						if tc.name == "broader namespace" || tc.name == "weaker process" {
							relatedLoc = namedRestrictionLastLoc(t, root, tc.related)
						}
						related = []Loc{relatedLoc}
					}
					if tc.related2 != "" {
						related = append(related, complexContentTestLoc(t, root, tc.related2))
					}
					if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, root, tc.marker) ||
						!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaComplexTypeDerivationSpecRef(factVersion) ||
						!errors.Is(err, tc.cause) {
						t.Fatalf("diagnostic = %s/%s/%s/%v/%s/%v", d.Class(), d.Code(), d.Loc(), d.Related(), d.SpecRef(), err)
					}
					if tc.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported sentinel lost: %v", err)
					}
					if attempt == 0 {
						first = d
						continue
					}
					if first.Class() != d.Class() || first.Code() != d.Code() || first.Loc() != d.Loc() ||
						!reflect.DeepEqual(first.Related(), d.Related()) || first.SpecRef() != d.SpecRef() || first.Message() != d.Message() {
						t.Fatal("repeated diagnostic changed")
					}
				}
			})
		}
	}
}

func TestNamedWildcardRestrictionFinalDefaultProvenance(t *testing.T) {
	for _, profile := range namedRestrictionProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := namedRestrictionSchema(string(profile.version), "urn:n",
				namedRestrictionOwner("t:Base", namedRestrictionWildcard("##other", "lax"))+namedRestrictionBase())
			root = strings.Replace(root, `version="`+string(profile.version)+`"`, `version="`+string(profile.version)+`" finalDefault="restriction"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("finalDefault restriction permitted derivation")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode ||
				d.Loc() != complexContentTestLoc(t, root, `base="t:Base"`) ||
				!reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `finalDefault="restriction"`)}) ||
				d.SpecRef() != schemaComplexTypeDerivationSpecRef(factVersion) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("finalDefault diagnostic = %v", err)
			}
		})
	}
}

func namedRestrictionLastLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.LastIndex(root, marker)
	if index < 0 {
		t.Fatalf("missing last marker %q", marker)
	}
	before := root[:index]
	line := strings.Count(before, "\n") + 1
	column := len([]rune(before[strings.LastIndex(before, "\n")+1:])) + 1
	return mustTestLoc(t, "root.xsd", line, column)
}

func TestNamedWildcardRestrictionGraphVisibilityAndCycles(t *testing.T) { //nolint:gocognit // One graph matrix keeps discovery and diagnostic order explicit.
	for _, profile := range namedRestrictionProfiles() {
		for _, graph := range []struct {
			name, directive, prefix, wildcard, baseNamespace, source string
		}{
			{"include", `<xs:include schemaLocation="base.xsd"/>`, "t:Base", "##other", "urn:n", "base.xsd"},
			{"chameleon", `<xs:include schemaLocation="base.xsd"/>`, "t:Base", "##other", "", "base.xsd"},
			{"import", `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`, "b:Base", "urn:outside", "urn:base", "base.xsd"},
		} {
			t.Run(profile.name+"/"+graph.name, func(t *testing.T) {
				baseHeader := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"`
				if graph.baseNamespace != "" {
					baseHeader += ` targetNamespace="` + graph.baseNamespace + `"`
				}
				base := baseHeader + `>` + namedRestrictionBase() + `</xs:schema>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" xmlns:b="urn:base" targetNamespace="urn:n" version="` + string(profile.version) + `">` +
					graph.directive + namedRestrictionOwner(graph.prefix, namedRestrictionWildcard(graph.wildcard, "lax")) + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
					graph.source: {id: SourceID(graph.source), contents: base},
				}, profile.policy)
				if err != nil {
					t.Fatalf("graph parse: %v", err)
				}
				components := schema.Components()
				if len(components) != 2 || components[0].Name().Local() != "Derived" || components[1].Name().Local() != "Base" {
					t.Fatalf("graph component order = %v", components)
				}
				definition, _ := components[0].ComplexType()
				reference, ok := definition.BaseReference()
				id, hasID := reference.ComponentID()
				if !ok || !hasID || id != components[1].ID() || reference.Loc() != complexContentTestLoc(t, root, `base="`+graph.prefix+`"`) ||
					components[1].Loc() != complexContentTestSourceLoc(t, SourceID(graph.source), base, `<xs:complexType name="Base"`) {
					t.Fatalf("graph identity/locations = %v/%v", reference, components)
				}
			})
		}
		t.Run(profile.name+"/invisible", func(t *testing.T) {
			base := namedRestrictionSchema(string(profile.version), "urn:base", namedRestrictionBase())
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="` + string(profile.version) + `"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:n" version="` + string(profile.version) + `"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>` + namedRestrictionOwner("b:Base", namedRestrictionWildcard("##other", "lax")) + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: bridge},
				"base.xsd":   {id: "base.xsd", contents: base},
			}, profile.policy)
			if err == nil {
				t.Fatal("invisible base resolved")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) ||
				!reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}) ||
				d.SpecRef() != schemaComplexTypeDerivationSpecRef(factVersion) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("invisible-base diagnostic = %v", err)
			}
		})
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			wildcard := namedRestrictionWildcard("##other", "lax")
			root := namedRestrictionSchema(string(profile.version), "urn:n",
				strings.Replace(namedRestrictionOwner("t:B", wildcard), `name="Derived"`, `name="A"`, 1)+
					strings.Replace(namedRestrictionOwner("t:A", wildcard), `name="Derived"`, `name="B"`, 1))
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("cyclic restrictions resolved")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="t:A"`) ||
				!reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `base="t:B"`)}) ||
				d.SpecRef() != schemaComplexTypeDerivationSpecRef(factVersion) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
				t.Fatalf("cycle diagnostic = %v", err)
			}
		})
	}
}

func TestNamedWildcardRestrictionRicherDerivedShapesStayUnsupported(t *testing.T) { //nolint:gocognit // Each excluded shape and policy has an independent parser assertion.
	for _, profile := range namedRestrictionProfiles() {
		for _, shape := range []string{"named", "inline"} {
			for _, child := range []struct{ name, xml, marker string }{
				{"particle", `<xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>`, `<xs:sequence>`},
				{"attribute use", `<xs:attribute name="v" type="xs:boolean"/>`, `<xs:attribute name="v"`},
				{"attribute group ref", `<xs:attributeGroup ref="t:Uses"/>`, `<xs:attributeGroup`},
				{"wildcard notQName", `<xs:anyAttribute namespace="##other" processContents="lax" notQName="xs:string"/>`, `notQName="xs:string"`},
			} {
				t.Run(profile.name+"/"+shape+"/"+child.name, func(t *testing.T) {
					wildcard := namedRestrictionWildcard("##other", "lax")
					body := child.xml + wildcard
					if child.name == "wildcard notQName" {
						body = child.xml
					}
					owner := namedRestrictionOwner("t:Base", body)
					if shape == "inline" {
						owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + body + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
					}
					root := namedRestrictionSchema(string(profile.version), "urn:n", owner+namedRestrictionBase())
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("richer derived form parsed")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					marker := child.marker
					if child.name == "wildcard notQName" && profile.policy != Strict10 {
						marker = `<xs:anyAttribute`
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, marker) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("richer derived diagnostic = %v", err)
					}
				})
			}
		}
	}
}

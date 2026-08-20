package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func TestCodegenIdentifierCorpus(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		kind      codegenNameKind
		anonymous bool
		path      []uint32
		want      string
		wantError bool
	}{
		{name: "pascal words", raw: "purchase-order", kind: codegenNameKindComponent, want: "PurchaseOrder"},
		{name: "underscore punctuation", raw: "purchase_order.item", kind: codegenNameKindComponent, want: "PurchaseOrderItem"},
		{name: "lower upper boundary", raw: "purchaseOrder", kind: codegenNameKindComponent, want: "PurchaseOrder"},
		{name: "acronym boundary", raw: "HTTPServer", kind: codegenNameKindComponent, want: "HttpServer"},
		{name: "digit boundary", raw: "part2value", kind: codegenNameKindComponent, want: "Part2Value"},
		{name: "leading digit", raw: "123-lives", kind: codegenNameKindComponent, want: "N123Lives"},
		{name: "unicode letters", raw: "café", kind: codegenNameKindComponent, want: "Café"},
		{name: "unicode simple fold", raw: "Kelvin", kind: codegenNameKindComponent, want: "Kelvin"},
		{name: "uncased unicode export", raw: "東京", kind: codegenNameKindComponent, want: "X東京"},
		{name: "keyword", raw: "TYPE", kind: codegenNameKindComponent, want: "XType"},
		{name: "predeclared", raw: "INT", kind: codegenNameKindComponent, want: "XInt"},
		{name: "case collision base", raw: "Foo", kind: codegenNameKindComponent, want: "Foo"},
		{name: "empty normalized", raw: "---", kind: codegenNameKindComponent, wantError: true},
		{name: "combining mark only", raw: "\u0301", kind: codegenNameKindComponent, wantError: true},
		{name: "invalid UTF-8", raw: string([]byte{0xff}), kind: codegenNameKindComponent, wantError: true},
		{name: "anonymous field", kind: codegenNameKindField, anonymous: true, path: []uint32{1, 12}, want: "FieldAtP1P12"},
		{name: "anonymous variant", kind: codegenNameKindVariant, anonymous: true, path: []uint32{2, 3}, want: "VariantAtP2P3"},
		{name: "anonymous global rejected", kind: codegenNameKindComponent, anonymous: true, path: []uint32{1}, wantError: true},
		{name: "anonymous path required", kind: codegenNameKindField, anonymous: true, wantError: true},
		{name: "anonymous path one based", kind: codegenNameKindField, anonymous: true, path: []uint32{0}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := codegenIdentifier(test.raw, test.kind, test.anonymous, test.path, Loc{})
			if test.wantError {
				requireCodegenInvalidName(t, err)
				if got != "" {
					t.Fatalf("identifier = %q after error, want empty", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("codegenIdentifier: %v", err)
			}
			if got != test.want {
				t.Fatalf("identifier = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCodegenPackageValidationIsStableAndPrecedesOutput(t *testing.T) {
	valid := []string{"generated", "generated_2", "Σchema", "_generated", "Type"}
	for _, name := range valid {
		assertValidCodegenPackageName(t, name)
	}

	invalid := []string{"", "_", "1generated", "generated-name", "type", "int", string([]byte{0xff})}
	for _, name := range invalid {
		name := name
		t.Run("invalid/"+name, func(t *testing.T) {
			assertInvalidCodegenPackageName(t, name)
		})
	}
}

func TestCodegenNamingAllocatesOrderedScopesAndReservations(t *testing.T) {
	sharedFirst := mustTestQName(t, "urn:first", "shared")
	sharedSecond := mustTestQName(t, "urn:second", "shared")
	sharedCase := mustTestQName(t, "urn:second", "Shared")
	digitName := mustTestQName(t, "urn:second", "9-lives")
	rootLoc := mustTestLoc(t, "first.xsd", 1, 1)
	firstLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondRootLoc := mustTestLoc(t, "second.xsd", 1, 1)
	secondLoc := mustTestLoc(t, "second.xsd", 2, 3)
	schema := mustTestSchema(t, []schemaDocumentInput{
		{
			source:  "first.xsd",
			rootLoc: rootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: sharedFirst, loc: firstLoc},
				{kind: ComponentKindComplexTypeDefinition, name: sharedSecond, loc: firstLoc},
			},
		},
		{
			source:  "second.xsd",
			rootLoc: secondRootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindAttributeDeclaration, name: sharedCase, loc: secondLoc},
				{kind: ComponentKindNotationDeclaration, name: digitName, loc: secondLoc},
			},
		},
	})
	components := schema.Components()
	ownerFirst := components[0].ID()
	ownerSecond := components[2].ID()
	input := codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		localParticles: []codegenLocalParticleRequest{
			{owner: ownerFirst, path: []uint32{1}, name: mustTestQName(t, "", "line-item")},
			{owner: ownerFirst, path: []uint32{2}, name: mustTestQName(t, "", "line_item")},
			{owner: ownerSecond, path: []uint32{1}, name: mustTestQName(t, "urn:other", "line.item")},
			{owner: ownerFirst, path: []uint32{3}, anonymous: true},
		},
		variants: []codegenVariantRequest{
			{owner: ownerFirst, path: []uint32{1}, name: mustTestQName(t, "", "choice-a")},
			{owner: ownerFirst, path: []uint32{2}, name: mustTestQName(t, "", "choice_a")},
			{owner: ownerSecond, path: []uint32{1}, name: mustTestQName(t, "", "choice-a")},
			{owner: ownerFirst, path: []uint32{3}, anonymous: true},
		},
		importAliases: []codegenImportAliasRequest{
			{identity: "urn:billing", alias: "billing-client"},
			{identity: "urn:billing-v2", alias: "billing_client"},
			{identity: "urn:json", alias: "json"},
			{identity: "urn:shared", alias: "shared"},
		},
	}

	names, err := newCodegenNaming(input)
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}
	assertCodegenComponentIdentifiers(t, names, []string{"Shared", "Shared2", "Shared3", "N9Lives"})
	if got, ok := names.componentName(ownerFirst); !ok || got != "Shared" {
		t.Fatalf("componentName(ownerFirst) = %q, %t, want Shared, true", got, ok)
	}
	if got, ok := names.componentName(ComponentID{source: "missing", ordinal: 1}); ok || got != "" {
		t.Fatalf("componentName(missing) = %q, %t, want empty, false", got, ok)
	}

	assertCodegenFieldNames(t, names, []codegenExpectedScopedName{
		{ownerFirst, []uint32{1}, "LineItem"},
		{ownerFirst, []uint32{2}, "LineItem2"},
		{ownerSecond, []uint32{1}, "LineItem"},
		{ownerFirst, []uint32{3}, "FieldAtP3"},
	})
	assertCodegenVariantNames(t, names, []codegenExpectedScopedName{
		{ownerFirst, []uint32{1}, "ChoiceA"},
		{ownerFirst, []uint32{2}, "ChoiceA2"},
		{ownerSecond, []uint32{1}, "ChoiceA3"},
		{ownerFirst, []uint32{3}, "VariantAtP3"},
	})
	assertCodegenImportNames(t, names, []codegenExpectedImportName{
		{"urn:billing", "BillingClient"},
		{"urn:billing-v2", "BillingClient2"},
		{"urn:json", "Json"},
		{"urn:shared", "Shared4"},
	})
}

func TestCodegenNamingAllocatesCaseFoldCollisionsInOrder(t *testing.T) {
	tests := []struct {
		name     string
		rawNames []string
		want     []string
	}{
		{
			name:     "HTTPServer and httpserver",
			rawNames: []string{"HTTPServer", "httpserver", "HTTPServer2"},
			want:     []string{"HttpServer", "Httpserver2", "HttpServer22"},
		},
		{
			name:     "fooBAR and FOObar",
			rawNames: []string{"fooBAR", "FOObar", "fooBAR2"},
			want:     []string{"FooBar", "FoObar2", "FooBar22"},
		},
		{
			name:     "first use determines spelling",
			rawNames: []string{"httpserver", "HTTPServer"},
			want:     []string{"Httpserver", "HttpServer2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := mustCodegenNamingForComponents(t, test.rawNames)
			second := mustCodegenNamingForComponents(t, test.rawNames)
			assertCodegenComponentNamesInOrder(t, first, test.want)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("repeated naming differs:\nfirst: %#v\nsecond: %#v", first, second)
			}
		})
	}
}

func mustCodegenNamingForComponents(t *testing.T, rawNames []string) codegenNaming {
	t.Helper()
	declarations := make([]schemaComponentInput, len(rawNames))
	for index, rawName := range rawNames {
		declarations[index] = schemaComponentInput{
			kind: ComponentKindElementDeclaration,
			name: mustTestQName(t, "urn:test", rawName),
			loc:  mustTestLoc(t, "names.xsd", index+2, 1),
		}
	}
	input := codegenNamingInput{
		packageName: "generated",
		schema: mustTestSchema(t, []schemaDocumentInput{{
			source:       "names.xsd",
			rootLoc:      mustTestLoc(t, "names.xsd", 1, 1),
			declarations: declarations,
		}}),
	}
	names, err := newCodegenNaming(input)
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}
	return names
}

func assertCodegenComponentNamesInOrder(t *testing.T, names codegenNaming, want []string) {
	t.Helper()
	got := names.componentNames()
	if len(got) != len(want) {
		t.Fatalf("component count = %d, want %d", len(got), len(want))
	}
	for index, expected := range want {
		if got[index].identifier != expected {
			t.Errorf("component %d identifier = %q, want %q", index, got[index].identifier, expected)
		}
	}
}

type codegenExpectedScopedName struct {
	owner ComponentID
	path  []uint32
	name  string
}

type codegenExpectedImportName struct {
	identity string
	name     string
}

func assertValidCodegenPackageName(t *testing.T, name string) {
	t.Helper()
	if err := validateCodegenPackageName(name); err != nil {
		t.Errorf("validateCodegenPackageName(%q): %v", name, err)
	}
}

func assertInvalidCodegenPackageName(t *testing.T, name string) {
	t.Helper()
	names, err := newCodegenNaming(codegenNamingInput{packageName: name})
	if err == nil {
		t.Fatal("newCodegenNaming accepted an invalid package name")
	}
	if !reflect.DeepEqual(names, codegenNaming{}) {
		t.Fatalf("naming result after package error = %#v, want zero result", names)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidCodegenPackageNameCode {
		t.Fatalf("diagnostic = %s, want invalid package diagnostic", diagnostic)
	}
	if !errors.Is(err, errInvalidCodegenPackageName) {
		t.Fatalf("package cause was not preserved: %v", err)
	}
	if !diagnostic.Loc().IsZero() {
		t.Fatalf("package diagnostic location = %s, want zero location", diagnostic.Loc())
	}
}

func assertCodegenComponentIdentifiers(t *testing.T, names codegenNaming, want []string) {
	t.Helper()
	for index, identifier := range want {
		if got := names.components[index].identifier; got != identifier {
			t.Errorf("component %d identifier = %q, want %q", index, got, identifier)
		}
	}
}

func assertCodegenFieldNames(t *testing.T, names codegenNaming, want []codegenExpectedScopedName) {
	t.Helper()
	for _, expected := range want {
		if got, ok := names.fieldName(expected.owner, expected.path); !ok || got != expected.name {
			t.Errorf("fieldName(%v, %v) = %q, %t, want %q, true", expected.owner, expected.path, got, ok, expected.name)
		}
	}
}

func assertCodegenVariantNames(t *testing.T, names codegenNaming, want []codegenExpectedScopedName) {
	t.Helper()
	for _, expected := range want {
		if got, ok := names.variantName(expected.owner, expected.path); !ok || got != expected.name {
			t.Errorf("variantName(%v, %v) = %q, %t, want %q, true", expected.owner, expected.path, got, ok, expected.name)
		}
	}
}

func assertCodegenImportNames(t *testing.T, names codegenNaming, want []codegenExpectedImportName) {
	t.Helper()
	for _, expected := range want {
		if got, ok := names.importAlias(expected.identity); !ok || got != expected.name {
			t.Errorf("importAlias(%q) = %q, %t, want %q, true", expected.identity, got, ok, expected.name)
		}
	}
}

func TestCodegenNamingVariantsSharePackageTypeScope(t *testing.T) {
	componentName := mustTestQName(t, "urn:component", "choice-a")
	ownerName := mustTestQName(t, "urn:owner", "owner")
	schema := mustTestSchema(t, []schemaDocumentInput{
		{
			source:  "components.xsd",
			rootLoc: mustTestLoc(t, "components.xsd", 1, 1),
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: componentName, loc: mustTestLoc(t, "components.xsd", 2, 1)},
				{kind: ComponentKindComplexTypeDefinition, name: ownerName, loc: mustTestLoc(t, "components.xsd", 3, 1)},
			},
		},
	})
	components := schema.Components()
	firstOwner := components[0].ID()
	secondOwner := components[1].ID()
	names, err := newCodegenNaming(codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		variants: []codegenVariantRequest{
			{owner: firstOwner, path: []uint32{1}, name: mustTestQName(t, "urn:first", "choice-a")},
			{owner: secondOwner, path: []uint32{1}, name: mustTestQName(t, "urn:second", "choice_a")},
		},
	})
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}

	if got, ok := names.componentName(firstOwner); !ok || got != "ChoiceA" {
		t.Fatalf("componentName(firstOwner) = %q, %t, want ChoiceA, true", got, ok)
	}
	for _, want := range []struct {
		owner ComponentID
		path  []uint32
		name  string
	}{
		{firstOwner, []uint32{1}, "ChoiceA2"},
		{secondOwner, []uint32{1}, "ChoiceA3"},
	} {
		if got, ok := names.variantName(want.owner, want.path); !ok || got != want.name {
			t.Errorf("variantName(%v, %v) = %q, %t, want %q, true", want.owner, want.path, got, ok, want.name)
		}
	}
	if got, ok := names.variantName(firstOwner, []uint32{2}); ok || got != "" {
		t.Fatalf("variantName(firstOwner, /2) = %q, %t, want empty, false", got, ok)
	}
}

func TestCodegenNamingImportAliasesReservePackageTypeNames(t *testing.T) {
	name := mustTestQName(t, "urn:type", "shared")
	schema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "schema.xsd",
		rootLoc: mustTestLoc(t, "schema.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindComplexTypeDefinition,
			name: name,
			loc:  mustTestLoc(t, "schema.xsd", 2, 1),
		}},
	}})
	owner := schema.Components()[0].ID()
	names, err := newCodegenNaming(codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		variants: []codegenVariantRequest{{
			owner: owner,
			path:  []uint32{1},
			name:  mustTestQName(t, "urn:variant", "shared"),
		}},
		importAliases: []codegenImportAliasRequest{{identity: "urn:import", alias: "shared"}},
	})
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}
	if got, _ := names.componentName(owner); got != "Shared" {
		t.Fatalf("componentName(owner) = %q, want Shared", got)
	}
	if got, _ := names.variantName(owner, []uint32{1}); got != "Shared2" {
		t.Fatalf("variantName(owner, /1) = %q, want Shared2", got)
	}
	if got, _ := names.importAlias("urn:import"); got != "Shared3" {
		t.Fatalf("importAlias(urn:import) = %q, want Shared3", got)
	}
}

func TestCodegenNamingIsDeterministicForOrderedInput(t *testing.T) {
	nameOne := mustTestQName(t, "urn:one", "same-name")
	nameTwo := mustTestQName(t, "urn:two", "SAME_NAME")
	schema := mustTestSchema(t, []schemaDocumentInput{
		{
			source:  "one.xsd",
			rootLoc: mustTestLoc(t, "one.xsd", 1, 1),
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: nameOne, loc: mustTestLoc(t, "one.xsd", 2, 1)},
			},
		},
		{
			source:  "two.xsd",
			rootLoc: mustTestLoc(t, "two.xsd", 1, 1),
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: nameTwo, loc: mustTestLoc(t, "two.xsd", 2, 1)},
			},
		},
	})
	components := schema.Components()
	input := codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		localParticles: []codegenLocalParticleRequest{
			{owner: components[0].ID(), path: []uint32{1, 2}, anonymous: true},
		},
		variants: []codegenVariantRequest{
			{owner: components[0].ID(), path: []uint32{3}, name: mustTestQName(t, "", "choice")},
		},
		importAliases: []codegenImportAliasRequest{{identity: "urn:one", alias: "same-name"}},
	}
	first, err := newCodegenNaming(input)
	if err != nil {
		t.Fatalf("first newCodegenNaming: %v", err)
	}
	second, err := newCodegenNaming(input)
	if err != nil {
		t.Fatalf("second newCodegenNaming: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated naming differs:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if first.packageIdentifier() != "generated" {
		t.Fatalf("packageIdentifier() = %q, want generated", first.packageIdentifier())
	}
}

func TestCodegenNamingAccessorsOwnMutableTables(t *testing.T) {
	name := mustTestQName(t, "urn:test", "item")
	schema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "schema.xsd",
		rootLoc: mustTestLoc(t, "schema.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindElementDeclaration,
			name: name,
			loc:  mustTestLoc(t, "schema.xsd", 2, 1),
		}},
	}})
	owner := schema.Components()[0].ID()
	names, err := newCodegenNaming(codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		localParticles: []codegenLocalParticleRequest{{
			owner: owner,
			path:  []uint32{1, 2},
			name:  mustTestQName(t, "", "field"),
		}},
		variants: []codegenVariantRequest{{
			owner: owner,
			path:  []uint32{3},
			name:  mustTestQName(t, "", "variant"),
		}},
		importAliases: []codegenImportAliasRequest{{identity: "urn:import", alias: "imported"}},
	})
	if err != nil {
		t.Fatalf("newCodegenNaming: %v", err)
	}

	components := names.componentNames()
	components[0].identifier = "Mutated"
	fields := names.fieldNames()
	fields[0].path[0] = 99
	variants := names.variantNames()
	variants[0].path[0] = 99
	imports := names.importAliasNames()
	imports[0].identifier = "Mutated"
	componentIDs := names.componentIdentifiers()
	componentIDs[owner] = "Mutated"
	fieldIDs := names.fieldIdentifiers()
	fieldIDs[codegenScopedPathKey{owner: owner, path: "/1/2/"}] = "Mutated"
	variantIDs := names.variantIdentifiers()
	variantIDs[codegenScopedPathKey{owner: owner, path: "/3/"}] = "Mutated"
	importIDs := names.importIdentifiers()
	importIDs["urn:import"] = "Mutated"

	if got, _ := names.componentName(owner); got != "Item" {
		t.Fatalf("componentName after accessor mutation = %q, want Item", got)
	}
	if got, _ := names.fieldName(owner, []uint32{1, 2}); got != "Field" {
		t.Fatalf("fieldName after accessor mutation = %q, want Field", got)
	}
	if got, _ := names.variantName(owner, []uint32{3}); got != "Variant" {
		t.Fatalf("variantName after accessor mutation = %q, want Variant", got)
	}
	if got, _ := names.importAlias("urn:import"); got != "Imported" {
		t.Fatalf("importAlias after accessor mutation = %q, want Imported", got)
	}

	clone := names.clone()
	cloneFields := clone.fieldNames()
	cloneFields[0].path[0] = 77
	if got, _ := names.fieldName(owner, []uint32{1, 2}); got != "Field" {
		t.Fatalf("fieldName after clone mutation = %q, want Field", got)
	}
}

func TestCodegenNamingRejectsInvalidRequestsWithoutPartialOutput(t *testing.T) {
	validName := mustTestQName(t, "urn:test", "valid")
	invalidName := mustTestQName(t, "urn:test", "---")
	validLoc := mustTestLoc(t, "schema.xsd", 2, 1)
	invalidLoc := mustTestLoc(t, "schema.xsd", 3, 1)
	schema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "schema.xsd",
		rootLoc: mustTestLoc(t, "schema.xsd", 1, 1),
		declarations: []schemaComponentInput{
			{kind: ComponentKindElementDeclaration, name: validName, loc: validLoc},
			{kind: ComponentKindElementDeclaration, name: invalidName, loc: invalidLoc},
		},
	}})
	names, err := newCodegenNaming(codegenNamingInput{packageName: "generated", schema: schema})
	if err == nil {
		t.Fatal("newCodegenNaming accepted a name that normalizes to nothing")
	}
	if !reflect.DeepEqual(names, codegenNaming{}) {
		t.Fatalf("naming result after name error = %#v, want zero result", names)
	}
	diagnostic := assertCodegenInvalidName(t, err)
	if diagnostic.Loc() != invalidLoc {
		t.Fatalf("invalid name location = %s, want %s", diagnostic.Loc(), invalidLoc)
	}

	owner := schema.Components()[0].ID()
	duplicatePath := codegenNamingInput{
		packageName: "generated",
		schema:      schema,
		localParticles: []codegenLocalParticleRequest{
			{owner: owner, path: []uint32{1}, name: validName},
			{owner: owner, path: []uint32{1}, name: validName},
		},
	}
	_, err = newCodegenNaming(duplicatePath)
	requireCodegenInvalidName(t, err)

	duplicateImport := codegenNamingInput{
		packageName:   "generated",
		schema:        schema,
		importAliases: []codegenImportAliasRequest{{identity: "urn:test", alias: "one"}, {identity: "urn:test", alias: "two"}},
	}
	_, err = newCodegenNaming(duplicateImport)
	requireCodegenInvalidName(t, err)

	invalidOwner := codegenNamingInput{
		packageName:    "generated",
		schema:         schema,
		localParticles: []codegenLocalParticleRequest{{owner: ComponentID{ordinal: 1}, path: []uint32{1}, name: validName}},
	}
	_, err = newCodegenNaming(invalidOwner)
	requireCodegenInvalidName(t, err)
}

func TestCodegenNamingPreservesInvalidNameCauseAndNamespaceInput(t *testing.T) {
	loc := mustTestLoc(t, "schema.xsd", 4, 2)
	_, err := codegenQNameIdentifier(QName{namespace: string([]byte{0xff}), local: "item"}, codegenNameKindComponent, false, nil, loc)
	if err == nil {
		t.Fatal("codegenQNameIdentifier accepted an invalid namespace")
	}
	diagnostic := assertCodegenInvalidName(t, err)
	if diagnostic.Loc() != loc {
		t.Fatalf("namespace diagnostic location = %s, want %s", diagnostic.Loc(), loc)
	}
}

func assertCodegenInvalidName(t *testing.T, err error) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected invalid codegen name error")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidCodegenNameCode {
		t.Fatalf("diagnostic = %s, want invalid codegen name diagnostic", diagnostic)
	}
	if !errors.Is(err, errInvalidCodegenName) {
		t.Fatalf("invalid name cause was not preserved: %v", err)
	}
	return diagnostic
}

func requireCodegenInvalidName(t *testing.T, err error) {
	t.Helper()
	diagnostic := assertCodegenInvalidName(t, err)
	if diagnostic.Code() == "" {
		t.Fatal("invalid code-generation name diagnostic has no code")
	}
}

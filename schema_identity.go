package goxsd9

import (
	"errors"
	"fmt"
)

const (
	diagnosticSchemaIdentitySyntaxCode      = "XSD3053"
	diagnosticSchemaIdentityDuplicateCode   = "XSD3054"
	diagnosticSchemaIdentityUnresolvedCode  = "XSD3055"
	diagnosticSchemaIdentityWrongKindCode   = "XSD3056"
	diagnosticSchemaIdentityAmbiguousCode   = "XSD3057"
	diagnosticSchemaIdentityFieldCountCode  = "XSD3058"
	diagnosticSchemaIdentityUnsupportedCode = "XSD3059"
	diagnosticSchemaIdentityInvisibleCode   = "XSD3060"
)

var (
	errSchemaIdentitySyntax      = errors.New("identity constraint syntax is invalid")
	errSchemaIdentityDuplicate   = errors.New("identity constraint name is duplicated")
	errSchemaIdentityUnresolved  = errors.New("identity constraint reference is unresolved")
	errSchemaIdentityInvisible   = errors.New("identity constraint reference is not visible")
	errSchemaIdentityWrongKind   = errors.New("identity constraint reference has the wrong target kind")
	errSchemaIdentityAmbiguous   = errors.New("identity constraint reference is ambiguous")
	errSchemaIdentityFieldCount  = errors.New("identity constraint field counts differ")
	errSchemaIdentityUnsupported = errors.New("identity constraint form is not implemented")
)

// IdentityConstraintID identifies a nested identity constraint in its source.
type IdentityConstraintID struct {
	source  SourceID
	ordinal uint64
}

// Source returns the declaring source identity.
func (id IdentityConstraintID) Source() SourceID { return id.source }

// Ordinal returns the one-based lexical identity-constraint ordinal.
func (id IdentityConstraintID) Ordinal() uint64 { return id.ordinal }

// IsZero reports whether the ID identifies no constraint.
func (id IdentityConstraintID) IsZero() bool { return id == IdentityConstraintID{} }

// IdentityConstraintKind identifies the three supported declaration forms.
type IdentityConstraintKind string

const (
	// IdentityConstraintUnique identifies an xs:unique declaration.
	IdentityConstraintUnique IdentityConstraintKind = "unique"
	// IdentityConstraintKey identifies an xs:key declaration.
	IdentityConstraintKey IdentityConstraintKind = "key"
	// IdentityConstraintKeyref identifies an xs:keyref declaration.
	IdentityConstraintKeyref IdentityConstraintKind = "keyref"
)

// IdentityNamespaceBinding is a copied in-scope prefix binding.
type IdentityNamespaceBinding struct {
	Prefix    string
	Namespace string
}

// IdentityXPath retains the XML-decoded expression and its namespace context.
type IdentityXPath struct{ facts schemaIdentityXPath }

// LexicalForm returns the unnormalized XML-decoded XPath attribute value.
func (expression IdentityXPath) LexicalForm() string { return expression.facts.lexical }

// Loc returns the XPath attribute location.
func (expression IdentityXPath) Loc() Loc { return expression.facts.loc }

// DefaultNamespace returns the effective default element namespace for XPath.
func (expression IdentityXPath) DefaultNamespace() string { return expression.facts.defaultNamespace }

// NamespaceBindings returns independent ordered in-scope prefix bindings.
func (expression IdentityXPath) NamespaceBindings() []IdentityNamespaceBinding {
	return append([]IdentityNamespaceBinding(nil), expression.facts.bindings...)
}

// IdentityConstraint is an immutable nested declaration on a global element.
type IdentityConstraint struct {
	facts *schemaIdentityConstraintComponent
}

// ID returns the nested model identity.
func (constraint IdentityConstraint) ID() IdentityConstraintID {
	if constraint.facts == nil {
		return IdentityConstraintID{}
	}
	return constraint.facts.id
}

// Kind returns unique, key, or keyref.
func (constraint IdentityConstraint) Kind() IdentityConstraintKind {
	if constraint.facts == nil {
		return ""
	}
	return constraint.facts.kind
}

// Name returns the expanded declaration name.
func (constraint IdentityConstraint) Name() QName {
	if constraint.facts == nil {
		return QName{}
	}
	return constraint.facts.name
}

// Loc returns the declaration location.
func (constraint IdentityConstraint) Loc() Loc {
	if constraint.facts == nil {
		return Loc{}
	}
	return constraint.facts.loc
}

// Selector returns the ordered selector expression.
func (constraint IdentityConstraint) Selector() IdentityXPath {
	if constraint.facts == nil {
		return IdentityXPath{}
	}
	return IdentityXPath{facts: constraint.facts.selector}
}

// Fields returns independent field expressions in lexical order.
func (constraint IdentityConstraint) Fields() []IdentityXPath {
	if constraint.facts == nil {
		return nil
	}
	fields := make([]IdentityXPath, len(constraint.facts.fields))
	for index, field := range constraint.facts.fields {
		fields[index] = IdentityXPath{facts: field}
	}
	return fields
}

// Refer returns the expanded keyref QName and its attribute location.
func (constraint IdentityConstraint) Refer() (QName, Loc, bool) {
	if constraint.facts == nil || constraint.facts.kind != IdentityConstraintKeyref {
		return QName{}, Loc{}, false
	}
	return constraint.facts.refer, constraint.facts.referLoc, true
}

// TargetID returns the resolved key or unique identity of a keyref.
func (constraint IdentityConstraint) TargetID() (IdentityConstraintID, bool) {
	if constraint.facts == nil || constraint.facts.kind != IdentityConstraintKeyref {
		return IdentityConstraintID{}, false
	}
	return constraint.facts.targetID, true
}

// IdentityConstraints returns independent views in declaration order.
func (declaration ElementDeclaration) IdentityConstraints() []IdentityConstraint {
	if declaration.facts == nil {
		return nil
	}
	constraints := make([]IdentityConstraint, len(declaration.facts.identityConstraints))
	for index := range declaration.facts.identityConstraints {
		constraints[index] = IdentityConstraint{facts: &declaration.facts.identityConstraints[index]}
	}
	return constraints
}

type schemaIdentityXPath struct {
	lexical          string
	loc              Loc
	defaultNamespace string
	bindings         []IdentityNamespaceBinding
}

type schemaIdentityConstraintInput struct {
	kind     IdentityConstraintKind
	name     QName
	loc      Loc
	selector schemaIdentityXPath
	fields   []schemaIdentityXPath
	refer    QName
	referLoc Loc
}

type schemaIdentityConstraintComponent struct {
	schemaIdentityConstraintInput
	id       IdentityConstraintID
	targetID IdentityConstraintID
}

func cloneSchemaIdentityXPath(input schemaIdentityXPath) schemaIdentityXPath {
	input.bindings = append([]IdentityNamespaceBinding(nil), input.bindings...)
	return input
}

func cloneSchemaIdentityConstraintInputs(inputs []schemaIdentityConstraintInput) []schemaIdentityConstraintInput {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]schemaIdentityConstraintInput, len(inputs))
	for index, input := range inputs {
		out[index] = input
		out[index].selector = cloneSchemaIdentityXPath(input.selector)
		out[index].fields = make([]schemaIdentityXPath, len(input.fields))
		for fieldIndex, field := range input.fields {
			out[index].fields[fieldIndex] = cloneSchemaIdentityXPath(field)
		}
	}
	return out
}

func cloneSchemaIdentityConstraintComponents(inputs []schemaIdentityConstraintComponent) []schemaIdentityConstraintComponent {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]schemaIdentityConstraintComponent, len(inputs))
	for index, input := range inputs {
		out[index] = input
		out[index].selector = cloneSchemaIdentityXPath(input.selector)
		out[index].fields = make([]schemaIdentityXPath, len(input.fields))
		for fieldIndex, field := range input.fields {
			out[index].fields[fieldIndex] = cloneSchemaIdentityXPath(field)
		}
	}
	return out
}

func schemaIdentitySpecRef(version XSDVersion, anchor string) string {
	if version == XSDVersion10 {
		return "xsd10-structures#" + anchor
	}
	return "xsd11-structures#" + anchor
}

func newSchemaIdentityDiagnostic(class FailureClass, code string, loc Loc, message string, related []Loc, version XSDVersion, anchor string, cause error) Diagnostic {
	return Diagnostic{class: class, code: code, loc: loc, message: message, related: related, specRef: schemaIdentitySpecRef(version, anchor), cause: cause}
}

func newSchemaIdentityInvalid(loc Loc, message string, version XSDVersion) Diagnostic {
	return newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentitySyntaxCode, loc, message, nil, version, "src-identity-constraint", errSchemaIdentitySyntax)
}

func newSchemaIdentityUnsupported(loc Loc, message string, version XSDVersion) Diagnostic {
	diagnostic := newSchemaIdentityDiagnostic(FailureUnsupported, diagnosticSchemaIdentityUnsupportedCode, loc, message, nil, version, "src-identity-constraint", errSchemaIdentityUnsupported)
	diagnostic.feature = FeatureSchemaSyntax
	return diagnostic
}

func schemaIdentityUniqueAttributes(element *syntaxElement, version XSDVersion, locals ...string) error {
	for _, local := range locals {
		attributes := syntaxAttributesByLocal(element, local)
		if len(attributes) > 1 {
			return newSchemaIdentityInvalid(attributes[1].loc, fmt.Sprintf("identity attribute %q must be unique", local), version)
		}
	}
	return nil
}

func schemaRootXPathDefaultNamespace(root *syntaxElement) string {
	attributes := syntaxAttributesByLocal(root, "xpathDefaultNamespace")
	if len(attributes) == 0 {
		return ""
	}
	return collapseXMLWhitespace(attributes[0].value)
}

func schemaIdentityXPathDefaultNamespace(element *syntaxElement, inherited, targetNamespace string) string {
	attributes := syntaxAttributesByLocal(element, "xpathDefaultNamespace")
	value := inherited
	if len(attributes) > 0 {
		value = collapseXMLWhitespace(attributes[0].value)
	}
	switch value {
	case "##local":
		return ""
	case "##targetNamespace":
		return targetNamespace
	case "##defaultNamespace":
		namespace, _ := element.scope.lookup("")
		return namespace
	default:
		return value
	}
}

func schemaIdentityBindings(scope *syntaxScope) []IdentityNamespaceBinding {
	if scope == nil {
		return []IdentityNamespaceBinding{{Prefix: "xml", Namespace: xmlNamespaceURI}}
	}
	bindings := schemaIdentityBindings(scope.parent)
	for _, binding := range scope.bindings {
		found := false
		for index := range bindings {
			if bindings[index].Prefix == binding.prefix {
				bindings[index].Namespace = binding.namespace
				found = true
				break
			}
		}
		if !found {
			bindings = append(bindings, IdentityNamespaceBinding{Prefix: binding.prefix, Namespace: binding.namespace})
		}
	}
	return bindings
}

func schemaIdentityXPathInput(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (schemaIdentityXPath, error) {
	xpath, err := schemaIdentityXPathAttribute(element, version)
	if err != nil {
		return schemaIdentityXPath{}, err
	}
	if err := validateSchemaIdentityXPathChildren(element, version); err != nil {
		return schemaIdentityXPath{}, err
	}
	return schemaIdentityXPath{lexical: xpath.value, loc: xpath.loc, defaultNamespace: schemaIdentityXPathDefaultNamespace(element, facts.xpathDefaultNamespace, facts.targetNamespace.value), bindings: schemaIdentityBindings(element.scope)}, nil
}

func schemaIdentityXPathAttribute(element *syntaxElement, version XSDVersion) (syntaxAttribute, error) {
	if err := schemaIdentityUniqueAttributes(element, version, "xpath", "xpathDefaultNamespace", "id"); err != nil {
		return syntaxAttribute{}, err
	}
	for _, attribute := range element.attrs {
		if err := validateSchemaIdentityXPathAttribute(attribute, version); err != nil {
			return syntaxAttribute{}, err
		}
	}
	xpaths := syntaxAttributesByLocal(element, "xpath")
	if len(xpaths) == 0 {
		return syntaxAttribute{}, newSchemaIdentityInvalid(element.loc, "identity XPath requires an xpath attribute", version)
	}
	xpath := xpaths[0]
	if collapseXMLWhitespace(xpath.value) == "" {
		return syntaxAttribute{}, newSchemaIdentityInvalid(xpath.loc, "identity XPath must not be empty", version)
	}
	return xpath, nil
}

func validateSchemaIdentityXPathAttribute(attribute syntaxAttribute, version XSDVersion) error {
	if attribute.name.namespace == xsdNamespaceURI {
		return newSchemaIdentityInvalid(attribute.loc, "identity XPath has a forbidden XSD-qualified attribute", version)
	}
	if attribute.name.namespace != "" {
		return nil
	}
	switch attribute.name.local {
	case "xpath", "id":
		return nil
	case "xpathDefaultNamespace":
		if version == XSDVersion10 {
			return newXSD11FeatureMismatchAtReference(FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode, attribute.loc, "identity XPath default namespace is an XSD 1.1-only construct", schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint"), nil)
		}
		if err := validateSchemaXPathDefaultNamespace(attribute); err != nil {
			return newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentitySyntaxCode, attribute.loc, "identity XPath default namespace is invalid", nil, version, "src-identity-constraint", errors.Join(errSchemaIdentitySyntax, err))
		}
		return nil
	default:
		return newSchemaIdentityInvalid(attribute.loc, "identity XPath has a forbidden attribute", version)
	}
}

func validateSchemaIdentityXPathChildren(element *syntaxElement, version XSDVersion) error {
	annotationSeen := false
	for _, node := range element.children {
		if text, ok := node.(syntaxText); ok {
			if !xmlWhitespace([]byte(text.data)) {
				return newSchemaIdentityInvalid(text.loc, "identity XPath cannot contain character data", version)
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(element.loc, "identity XPath has an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI || child.name.local != "annotation" {
			return newSchemaIdentityInvalid(child.loc, "identity XPath permits only annotation children", version)
		}
		if annotationSeen {
			return newSchemaIdentityInvalid(child.loc, "identity XPath annotation must be unique", version)
		}
		if err := validateSchemaAnnotationElement(child); err != nil {
			return err
		}
		annotationSeen = true
	}
	return nil
}

func schemaIdentityConstraintInputs(owner *syntaxElement, facts schemaDocumentFacts, version XSDVersion) ([]schemaIdentityConstraintInput, error) {
	var inputs []schemaIdentityConstraintInput
	for _, node := range owner.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.namespace != xsdNamespaceURI {
			continue
		}
		if child.name.local != "unique" && child.name.local != "key" && child.name.local != "keyref" {
			continue
		}
		input, err := schemaIdentityConstraintInputFromElement(child, facts, version)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func schemaIdentityConstraintInputFromElement(element *syntaxElement, facts schemaDocumentFacts, version XSDVersion) (schemaIdentityConstraintInput, error) {
	input := schemaIdentityConstraintInput{kind: IdentityConstraintKind(element.name.local), loc: element.loc}
	if err := validateSchemaIdentityConstraintAttributes(element, version); err != nil {
		return input, err
	}
	name, err := schemaIdentityConstraintName(element, facts.targetNamespace.value, version)
	if err != nil {
		return input, err
	}
	input.name = name
	if err := schemaIdentityConstraintRefer(element, &input, facts, version); err != nil {
		return input, err
	}
	if err := schemaIdentityConstraintChildren(element, &input, facts, version); err != nil {
		return input, err
	}
	return input, nil
}

func validateSchemaIdentityConstraintAttributes(element *syntaxElement, version XSDVersion) error {
	if err := schemaIdentityUniqueAttributes(element, version, "name", "refer", "ref", "id"); err != nil {
		return err
	}
	var reuse syntaxAttribute
	hasReuse := false
	for _, attribute := range element.attrs {
		if attribute.name.namespace == xsdNamespaceURI {
			return newSchemaIdentityInvalid(attribute.loc, "identity constraint has a forbidden XSD-qualified attribute", version)
		}
		if attribute.name.namespace != "" {
			continue
		}
		switch attribute.name.local {
		case "name", "refer", "id":
		case "ref":
			reuse = attribute
			hasReuse = true
		default:
			return newSchemaIdentityInvalid(attribute.loc, "identity constraint has a forbidden attribute", version)
		}
	}
	if hasReuse {
		return validateSchemaIdentityRefReuse(element, reuse, version)
	}
	return nil
}

func validateSchemaIdentityRefReuse(element *syntaxElement, ref syntaxAttribute, version XSDVersion) error {
	for _, attribute := range element.attrs {
		if attribute.name.namespace == "" && (attribute.name.local == "name" || attribute.name.local == "refer") {
			return newSchemaIdentityInvalid(ref.loc, "identity constraint ref cannot be combined with name or refer", version)
		}
	}
	if _, err := expandSchemaQName(element, ref); err != nil {
		return newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentitySyntaxCode, ref.loc, "identity constraint ref is not a valid QName", nil, version, "src-identity-constraint", errors.Join(errSchemaIdentitySyntax, err))
	}
	if err := validateSchemaIdentityRefReuseChildren(element, version); err != nil {
		return err
	}
	if version == XSDVersion10 {
		return newXSD11FeatureMismatchAtReference(FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode, ref.loc, "identity constraint ref reuse is an XSD 1.1-only construct", schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint"), nil)
	}
	return newSchemaIdentityUnsupported(ref.loc, "XSD 1.1 identity constraint ref reuse is not implemented", version)
}

func validateSchemaIdentityRefReuseChildren(element *syntaxElement, version XSDVersion) error {
	annotationSeen := false
	for _, node := range element.children {
		if text, ok := node.(syntaxText); ok {
			if !xmlWhitespace([]byte(text.data)) {
				return newSchemaIdentityInvalid(text.loc, "identity ref cannot contain character data", version)
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(element.loc, "identity ref has an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI || child.name.local != "annotation" || annotationSeen {
			return newSchemaIdentityInvalid(child.loc, "identity ref has a forbidden child", version)
		}
		if err := validateSchemaAnnotationElement(child); err != nil {
			return err
		}
		annotationSeen = true
	}
	return nil
}

func schemaIdentityConstraintName(element *syntaxElement, targetNamespace string, version XSDVersion) (QName, error) {
	names := syntaxAttributesByLocal(element, "name")
	if len(names) == 0 {
		return QName{}, newSchemaIdentityInvalid(element.loc, "identity constraint requires a name", version)
	}
	name := collapseXMLWhitespace(names[0].value)
	if !validNCName(name) {
		return QName{}, newSchemaIdentityInvalid(names[0].loc, "identity constraint name must be an NCName", version)
	}
	expanded, err := NewQName(targetNamespace, name)
	if err != nil {
		return QName{}, newSchemaBridgeInvariant(names[0].loc, "construct identity constraint name")
	}
	return expanded, nil
}

func schemaIdentityConstraintRefer(element *syntaxElement, input *schemaIdentityConstraintInput, facts schemaDocumentFacts, version XSDVersion) error {
	references := syntaxAttributesByLocal(element, "refer")
	if input.kind != IdentityConstraintKeyref {
		if len(references) > 0 {
			return newSchemaIdentityInvalid(references[0].loc, "only keyref permits refer", version)
		}
		return nil
	}
	if len(references) == 0 {
		return newSchemaIdentityInvalid(element.loc, "keyref requires refer", version)
	}
	refer, err := expandSchemaQName(element, references[0])
	if err != nil {
		return newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentitySyntaxCode, references[0].loc, "keyref refer is not a valid QName", nil, version, "src-identity-constraint", errors.Join(errSchemaIdentitySyntax, err))
	}
	if facts.chameleon && facts.targetNamespace.present && refer.Namespace() == "" {
		refer, err = NewQName(facts.targetNamespace.value, refer.Local())
		if err != nil {
			return newSchemaBridgeInvariant(references[0].loc, "construct chameleon identity reference QName")
		}
	}
	input.refer = refer
	input.referLoc = references[0].loc
	return nil
}

//nolint:gocognit // The ordered annotation, selector, and field grammar is one boundary.
func schemaIdentityConstraintChildren(element *syntaxElement, input *schemaIdentityConstraintInput, facts schemaDocumentFacts, version XSDVersion) error {
	seenSelector := false
	seenField := false
	annotationSeen := false
	for _, node := range element.children {
		if text, ok := node.(syntaxText); ok {
			if !xmlWhitespace([]byte(text.data)) {
				return newSchemaIdentityInvalid(text.loc, "identity constraint cannot contain character data", version)
			}
			continue
		}
		child, ok := node.(*syntaxElement)
		if !ok {
			return newSchemaBridgeInvariant(element.loc, "identity constraint has an unknown syntax node")
		}
		if child.name.namespace != xsdNamespaceURI {
			return newSchemaIdentityInvalid(child.loc, "identity constraint has a foreign child", version)
		}
		switch child.name.local {
		case "annotation":
			if annotationSeen || seenSelector || seenField {
				return newSchemaIdentityInvalid(child.loc, "identity annotation must be first", version)
			}
			if err := validateSchemaAnnotationElement(child); err != nil {
				return err
			}
			annotationSeen = true
		case "selector":
			if seenSelector || seenField {
				return newSchemaIdentityInvalid(child.loc, "identity selector must occur once before fields", version)
			}
			expression, err := schemaIdentityXPathInput(child, facts, version)
			if err != nil {
				return err
			}
			input.selector = expression
			seenSelector = true
		case "field":
			if !seenSelector {
				return newSchemaIdentityInvalid(child.loc, "identity field requires a preceding selector", version)
			}
			expression, err := schemaIdentityXPathInput(child, facts, version)
			if err != nil {
				return err
			}
			input.fields = append(input.fields, expression)
			seenField = true
		default:
			return newSchemaIdentityInvalid(child.loc, "identity constraint has a forbidden child", version)
		}
	}
	if !seenSelector {
		return newSchemaIdentityInvalid(element.loc, "identity constraint requires one selector", version)
	}
	if !seenField {
		return newSchemaIdentityInvalid(element.loc, "identity constraint requires at least one field", version)
	}
	return nil
}

func resolveSchemaIdentityConstraints(records []schemaComponentRecord, visibleSources map[SourceID][]SourceID, version XSDVersion) ([][]schemaIdentityConstraintComponent, error) {
	results, byName, err := allocateSchemaIdentityConstraints(records)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateSchemaIdentityConstraints(results, version); err != nil {
		return nil, err
	}
	if err := resolveSchemaIdentityKeyrefs(results, byName, visibleSources, version); err != nil {
		return nil, err
	}
	return results, nil
}

func allocateSchemaIdentityConstraints(records []schemaComponentRecord) ([][]schemaIdentityConstraintComponent, map[QName][]schemaIdentityConstraintComponent, error) {
	results := make([][]schemaIdentityConstraintComponent, len(records))
	byName := make(map[QName][]schemaIdentityConstraintComponent)
	nextBySource := make(map[SourceID]uint64)
	for index, record := range records {
		if record.element == nil {
			continue
		}
		for _, input := range record.element.identityConstraints {
			next := nextBySource[record.id.Source()] + 1
			if next == 0 {
				return nil, nil, newSchemaBridgeInvariant(input.loc, "identity constraint ordinal overflows uint64")
			}
			nextBySource[record.id.Source()] = next
			component := schemaIdentityConstraintComponent{schemaIdentityConstraintInput: input, id: IdentityConstraintID{source: record.id.Source(), ordinal: next}}
			results[index] = append(results[index], component)
			byName[input.name] = append(byName[input.name], component)
		}
	}
	return results, byName, nil
}

func rejectDuplicateSchemaIdentityConstraints(results [][]schemaIdentityConstraintComponent, version XSDVersion) error {
	first := make(map[QName]schemaIdentityConstraintComponent)
	for _, owned := range results {
		for _, component := range owned {
			if earliest, ok := first[component.name]; ok {
				return newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentityDuplicateCode, component.loc, fmt.Sprintf("identity constraint %q is duplicated", component.name), []Loc{earliest.loc}, version, "coss-identity-constraint", errSchemaIdentityDuplicate)
			}
			first[component.name] = component
		}
	}
	return nil
}

func resolveSchemaIdentityKeyrefs(results [][]schemaIdentityConstraintComponent, byName map[QName][]schemaIdentityConstraintComponent, visibleSources map[SourceID][]SourceID, version XSDVersion) error {
	for index, owned := range results {
		for childIndex, component := range owned {
			if component.kind != IdentityConstraintKeyref {
				continue
			}
			available, ok := visibleSources[component.id.source]
			if !ok {
				return newSchemaBridgeInvariant(component.referLoc, "identity constraint source has no visibility entry")
			}
			target, err := resolveSchemaIdentityTarget(component, byName[component.refer], available, version)
			if err != nil {
				return err
			}
			results[index][childIndex].targetID = target.id
		}
	}
	return nil
}

func resolveSchemaIdentityTarget(component schemaIdentityConstraintComponent, candidates []schemaIdentityConstraintComponent, available []SourceID, version XSDVersion) (schemaIdentityConstraintComponent, error) {
	if len(candidates) == 0 {
		return schemaIdentityConstraintComponent{}, newSchemaIdentityDiagnostic(FailureResolution, diagnosticSchemaIdentityUnresolvedCode, component.referLoc, fmt.Sprintf("identity constraint %q is unresolved", component.refer), nil, version, "src-identity-constraint", errSchemaIdentityUnresolved)
	}
	var visible []schemaIdentityConstraintComponent
	for _, candidate := range candidates {
		if sourceIDInList(available, candidate.id.source) {
			visible = append(visible, candidate)
		}
	}
	if len(visible) == 0 {
		related := make([]Loc, len(candidates))
		for index, candidate := range candidates {
			related[index] = candidate.loc
		}
		return schemaIdentityConstraintComponent{}, newSchemaIdentityDiagnostic(FailureResolution, diagnosticSchemaIdentityInvisibleCode, component.referLoc, fmt.Sprintf("identity constraint %q is not visible from this document", component.refer), related, version, "src-identity-constraint", errSchemaIdentityInvisible)
	}
	if len(visible) > 1 {
		related := make([]Loc, len(visible))
		for index, candidate := range visible {
			related[index] = candidate.loc
		}
		return schemaIdentityConstraintComponent{}, newSchemaIdentityDiagnostic(FailureResolution, diagnosticSchemaIdentityAmbiguousCode, component.referLoc, fmt.Sprintf("identity constraint %q is ambiguous", component.refer), related, version, "src-identity-constraint", errSchemaIdentityAmbiguous)
	}
	target := visible[0]
	if target.kind == IdentityConstraintKeyref {
		return schemaIdentityConstraintComponent{}, newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentityWrongKindCode, component.referLoc, "keyref must refer to a key or unique", []Loc{target.loc}, version, "src-identity-constraint", errSchemaIdentityWrongKind)
	}
	if len(component.fields) != len(target.fields) {
		return schemaIdentityConstraintComponent{}, newSchemaIdentityDiagnostic(FailureInvalid, diagnosticSchemaIdentityFieldCountCode, component.referLoc, "keyref field count differs from target", []Loc{target.loc}, version, "coss-identity-constraint", errSchemaIdentityFieldCount)
	}
	return target, nil
}

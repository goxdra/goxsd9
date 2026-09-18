package goxsd9

// AttributeValueConstraintKind identifies the kind of a global attribute
// value constraint.
type AttributeValueConstraintKind string

const (
	// AttributeValueConstraintDefault identifies a default value constraint.
	AttributeValueConstraintDefault AttributeValueConstraintKind = "default"
	// AttributeValueConstraintFixed identifies a fixed value constraint.
	AttributeValueConstraintFixed AttributeValueConstraintKind = "fixed"
)

// AttributeValueConstraint is an immutable typed value constraint on a global
// attribute declaration. Lexical returns the normalized lexical form; the
// typed accessors retain the exact integer or decimal value.
type AttributeValueConstraint struct {
	kind       AttributeValueConstraintKind
	lexical    string
	loc        Loc
	integer    StrictInteger
	hasInteger bool
	decimal    StrictDecimal
	hasDecimal bool
}

// Kind returns whether the constraint supplies a default or fixed value.
func (constraint AttributeValueConstraint) Kind() AttributeValueConstraintKind {
	return constraint.kind
}

// IsDefault reports whether the constraint supplies a default value.
func (constraint AttributeValueConstraint) IsDefault() bool {
	return constraint.kind == AttributeValueConstraintDefault
}

// IsFixed reports whether the constraint supplies a fixed value.
func (constraint AttributeValueConstraint) IsFixed() bool {
	return constraint.kind == AttributeValueConstraintFixed
}

// Lexical returns the normalized lexical spelling retained for the value.
func (constraint AttributeValueConstraint) Lexical() string {
	return constraint.lexical
}

// Loc returns the source location of the default or fixed attribute.
func (constraint AttributeValueConstraint) Loc() Loc {
	return constraint.loc
}

// IntegerValue returns the exact integer value when the constraint is typed as
// an integer.
func (constraint AttributeValueConstraint) IntegerValue() (StrictInteger, bool) {
	if !constraint.hasInteger {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(constraint.integer), true
}

// DecimalValue returns the exact decimal value when the constraint is typed as
// a decimal.
func (constraint AttributeValueConstraint) DecimalValue() (StrictDecimal, bool) {
	if !constraint.hasDecimal {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(constraint.decimal), true
}

// Integer is a concise alias for IntegerValue.
func (constraint AttributeValueConstraint) Integer() (StrictInteger, bool) {
	return constraint.IntegerValue()
}

// Decimal is a concise alias for DecimalValue.
func (constraint AttributeValueConstraint) Decimal() (StrictDecimal, bool) {
	return constraint.DecimalValue()
}

func cloneAttributeValueConstraint(constraint *AttributeValueConstraint) *AttributeValueConstraint {
	if constraint == nil {
		return nil
	}
	clone := *constraint
	if constraint.hasInteger {
		clone.integer = cloneStrictInteger(constraint.integer)
	}
	if constraint.hasDecimal {
		clone.decimal = cloneStrictDecimal(constraint.decimal)
	}
	return &clone
}

type schemaAttributeValueConstraintInput struct {
	kind    AttributeValueConstraintKind
	lexical string
	loc     Loc
}

func cloneSchemaAttributeValueConstraintInput(input *schemaAttributeValueConstraintInput) *schemaAttributeValueConstraintInput {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

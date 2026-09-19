package goxsd9

// ValueConstraintKind identifies the kind of a global declaration value
// constraint.
type ValueConstraintKind string

const (
	// ValueConstraintDefault identifies a default value constraint.
	ValueConstraintDefault ValueConstraintKind = "default"
	// ValueConstraintFixed identifies a fixed value constraint.
	ValueConstraintFixed ValueConstraintKind = "fixed"
)

// AttributeValueConstraintKind identifies the kind of a global attribute
// value constraint.
type AttributeValueConstraintKind = ValueConstraintKind

const (
	// AttributeValueConstraintDefault identifies a default value constraint.
	AttributeValueConstraintDefault = ValueConstraintDefault
	// AttributeValueConstraintFixed identifies a fixed value constraint.
	AttributeValueConstraintFixed = ValueConstraintFixed
)

// ElementValueConstraintKind identifies the kind of a global element value
// constraint.
type ElementValueConstraintKind = ValueConstraintKind

const (
	// ElementValueConstraintDefault identifies a default value constraint.
	ElementValueConstraintDefault = ValueConstraintDefault
	// ElementValueConstraintFixed identifies a fixed value constraint.
	ElementValueConstraintFixed = ValueConstraintFixed
)

// ValueConstraint is an immutable typed value constraint on a supported
// global declaration. Lexical returns the normalized lexical form; the typed
// accessors retain the exact value.
type ValueConstraint struct {
	kind                ValueConstraintKind
	lexical             string
	loc                 Loc
	boolean             StrictBoolean
	hasBoolean          bool
	integer             StrictInteger
	hasInteger          bool
	decimal             StrictDecimal
	hasDecimal          bool
	precisionDecimal    StrictPrecisionDecimal
	hasPrecisionDecimal bool
	stringValue         string
	hasString           bool
}

// AttributeValueConstraint is retained as the attribute-specific name for
// the shared declaration value-constraint representation.
type AttributeValueConstraint = ValueConstraint

// ElementValueConstraint is the element-specific name for the shared
// declaration value-constraint representation.
type ElementValueConstraint = ValueConstraint

// Kind returns whether the constraint supplies a default or fixed value.
func (constraint ValueConstraint) Kind() ValueConstraintKind {
	return constraint.kind
}

// IsDefault reports whether the constraint supplies a default value.
func (constraint ValueConstraint) IsDefault() bool {
	return constraint.kind == ValueConstraintDefault
}

// IsFixed reports whether the constraint supplies a fixed value.
func (constraint ValueConstraint) IsFixed() bool {
	return constraint.kind == ValueConstraintFixed
}

// Lexical returns the normalized lexical spelling retained for the value.
func (constraint ValueConstraint) Lexical() string {
	return constraint.lexical
}

// Loc returns the source location of the default or fixed attribute.
func (constraint ValueConstraint) Loc() Loc {
	return constraint.loc
}

// BooleanValue returns the exact boolean value when the constraint is typed as
// a boolean.
func (constraint ValueConstraint) BooleanValue() (StrictBoolean, bool) {
	if !constraint.hasBoolean {
		return StrictBoolean{}, false
	}
	return constraint.boolean, true
}

// IntegerValue returns the exact integer value when the constraint is typed as
// an integer.
func (constraint ValueConstraint) IntegerValue() (StrictInteger, bool) {
	if !constraint.hasInteger {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(constraint.integer), true
}

// DecimalValue returns the exact decimal value when the constraint is typed as
// a decimal.
func (constraint ValueConstraint) DecimalValue() (StrictDecimal, bool) {
	if !constraint.hasDecimal {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(constraint.decimal), true
}

// Integer is a concise alias for IntegerValue.
func (constraint ValueConstraint) Integer() (StrictInteger, bool) {
	return constraint.IntegerValue()
}

// Decimal is a concise alias for DecimalValue.
func (constraint ValueConstraint) Decimal() (StrictDecimal, bool) {
	return constraint.DecimalValue()
}

// PrecisionDecimalValue returns the exact precisionDecimal value when the
// constraint is typed as precisionDecimal.
func (constraint ValueConstraint) PrecisionDecimalValue() (StrictPrecisionDecimal, bool) {
	if !constraint.hasPrecisionDecimal {
		return StrictPrecisionDecimal{}, false
	}
	return StrictPrecisionDecimal{value: clonePrecisionDecimalValue(constraint.precisionDecimal.privateValue())}, true
}

// PrecisionDecimal is a concise alias for PrecisionDecimalValue.
func (constraint ValueConstraint) PrecisionDecimal() (StrictPrecisionDecimal, bool) {
	return constraint.PrecisionDecimalValue()
}

// StringValue returns the value-space string when the constraint is typed as
// a supported string-family datatype.
func (constraint ValueConstraint) StringValue() (string, bool) {
	if !constraint.hasString {
		return "", false
	}
	return constraint.stringValue, true
}

// String returns a concise alias for StringValue.
func (constraint ValueConstraint) String() (string, bool) {
	return constraint.StringValue()
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
	if constraint.hasPrecisionDecimal {
		clone.precisionDecimal = StrictPrecisionDecimal{value: clonePrecisionDecimalValue(constraint.precisionDecimal.privateValue())}
	}
	return &clone
}

type schemaValueConstraintInput struct {
	kind    ValueConstraintKind
	lexical string
	loc     Loc
}

type schemaAttributeValueConstraintInput = schemaValueConstraintInput

func cloneSchemaAttributeValueConstraintInput(input *schemaValueConstraintInput) *schemaValueConstraintInput {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

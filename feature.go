package goxsd9

import (
	"fmt"
	"sort"

	"github.com/goxdra/goxsd9/internal/feature"
)

// FeatureID is a stable identifier for a specification feature.
type FeatureID = feature.ID

const (
	// FeatureInstanceSyntax identifies XML instance syntax outside the first
	// decoder slice, including DTDs and non-UTF-8 encodings.
	FeatureInstanceSyntax FeatureID = "xsd.instance.syntax"
	// FeatureInstanceValidation identifies XML instance semantic validation
	// outside the supported scalar element slice.
	FeatureInstanceValidation FeatureID = "xsd.instance.validation"
	// FeatureDatatypeFacets identifies the not-yet-implemented datatype facet set.
	FeatureDatatypeFacets FeatureID = "xsd.datatype.facets"
	// FeaturePrecisionDecimal identifies the not-yet-implemented XSD 1.1 type.
	FeaturePrecisionDecimal FeatureID = "xsd.datatype.precision-decimal"
	// FeatureSchemaSyntax identifies XSD syntax outside the bootstrap kernel.
	FeatureSchemaSyntax FeatureID = "xsd.schema.syntax"
)

// UnsupportedFeature is an opaque handle to a registered unsupported
// specification capability.
type UnsupportedFeature = feature.Feature

// UnsupportedFeatureReference identifies a pinned specification section for
// one specification version.
type UnsupportedFeatureReference = feature.Reference

// LookupUnsupportedFeature finds a registered unsupported feature by exact ID.
func LookupUnsupportedFeature(id FeatureID) (UnsupportedFeature, bool) {
	return feature.Lookup(id)
}

// UnsupportedFeatures returns registered unsupported features in stable order.
func UnsupportedFeatures() []UnsupportedFeature {
	return feature.All()
}

// ValidateFeatureRegistry validates the repository's unsupported-feature
// registry.
func ValidateFeatureRegistry() error {
	return feature.ValidateRegistry()
}

// UnsupportedFeatureReport ranks unsupported features by observed diagnostic
// count and breaks ties by feature ID.
type UnsupportedFeatureReport struct {
	feature UnsupportedFeature
	count   int
}

// Feature returns the registered feature represented by the report row.
func (report UnsupportedFeatureReport) Feature() UnsupportedFeature {
	return report.feature
}

// Count returns the number of unsupported diagnostics for the feature.
func (report UnsupportedFeatureReport) Count() int {
	return report.count
}

// ReportUnsupportedFeatures aggregates unsupported diagnostics into a
// deterministic unlock ranking. Non-unsupported diagnostics are ignored.
func ReportUnsupportedFeatures(diagnostics Diagnostics) ([]UnsupportedFeatureReport, error) {
	counts := make(map[FeatureID]int)
	for _, diagnostic := range diagnostics.items {
		if diagnostic.Class() != FailureUnsupported {
			continue
		}
		feature, ok := LookupUnsupportedFeature(diagnostic.Feature())
		if !ok {
			return nil, fmt.Errorf("unsupported diagnostic references unknown feature %q", diagnostic.Feature())
		}
		counts[feature.ID()]++
	}

	report := make([]UnsupportedFeatureReport, 0, len(counts))
	for _, feature := range UnsupportedFeatures() {
		count := counts[feature.ID()]
		if count == 0 {
			continue
		}
		report = append(report, UnsupportedFeatureReport{feature: feature, count: count})
	}
	sort.Slice(report, func(left, right int) bool {
		if report[left].count != report[right].count {
			return report[left].count > report[right].count
		}
		return report[left].feature.ID() < report[right].feature.ID()
	})
	return report, nil
}

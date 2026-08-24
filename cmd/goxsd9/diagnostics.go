package main

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goxdra/goxsd9"
)

type diagnosticFormat string

const (
	diagnosticsHuman diagnosticFormat = "human"
	diagnosticsJSON  diagnosticFormat = "json"
)

const (
	cliUsageCode      = "CLI1001"
	cliPathPolicyCode = "CLI1002"
	cliResourceCode   = "CLI1003"
	cliLimitCode      = "CLI1004"
	cliOutputCode     = "CLI1005"
	cliInternalCode   = "CLI1006"
)

const (
	cliUsageKind      = "usage"
	cliPathPolicyKind = "path-policy"
	cliResourceKind   = "resource"
	cliLimitKind      = "limit"
	cliOutputKind     = "output"
	cliInternalKind   = "internal"
)

type cliError struct {
	code     string
	kind     string
	sourceID goxsd9.SourceID
	message  string
	cause    error
}

func newCLIError(code, kind string, sourceID goxsd9.SourceID, message string, cause error) *cliError {
	return &cliError{code: code, kind: kind, sourceID: sourceID, message: message, cause: cause}
}

func (err *cliError) Error() string {
	return err.message
}

func (err *cliError) Unwrap() error {
	return err.cause
}

type renderedRelated struct {
	SourceID string             `json:"source_id"`
	Location renderedLineColumn `json:"location"`
}

type renderedLineColumn struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type renderedDiagnostic struct {
	Class    *string            `json:"class"`
	Kind     string             `json:"kind"`
	Code     string             `json:"code"`
	SourceID string             `json:"source_id"`
	Location renderedLineColumn `json:"location"`
	Related  []renderedRelated  `json:"related"`
	Feature  string             `json:"feature"`
	SpecRef  string             `json:"spec_ref"`
	Message  string             `json:"message"`
}

type diagnosticEnvelope struct {
	Format      string               `json:"format"`
	Command     string               `json:"command"`
	Stage       string               `json:"stage"`
	ExitStatus  int                  `json:"exit_status"`
	Diagnostics []renderedDiagnostic `json:"diagnostics"`
}

func reportUsage(writer io.Writer, command, message string, format diagnosticFormat, sourceID goxsd9.SourceID) int {
	diagnostic := newCLIError(cliUsageCode, cliUsageKind, sourceID, message, nil)
	if err := writeDiagnostics(writer, format, command, "usage", 2, []error{diagnostic}); err != nil {
		return 1
	}
	return 2
}

func reportError(writer io.Writer, command string, format diagnosticFormat, stage string, err error) int {
	rendered := renderError(err)
	if writeErr := writeDiagnostics(writer, format, command, stage, 1, rendered); writeErr != nil {
		return 1
	}
	return 1
}

func renderError(err error) []error {
	var boundary *cliError
	if errors.As(err, &boundary) {
		return []error{boundary}
	}
	var limitErr *sourceLimitError
	if errors.As(err, &limitErr) {
		return []error{newCLIError(cliLimitCode, cliLimitKind, limitErr.sourceID, limitErr.Error(), limitErr)}
	}
	var diagnostics goxsd9.Diagnostics
	if errors.As(err, &diagnostics) {
		return diagnosticErrors(diagnostics.All())
	}
	var diagnosticsPointer *goxsd9.Diagnostics
	if errors.As(err, &diagnosticsPointer) && diagnosticsPointer != nil {
		return diagnosticErrors(diagnosticsPointer.All())
	}
	var diagnostic goxsd9.Diagnostic
	if errors.As(err, &diagnostic) {
		return []error{diagnostic}
	}
	var diagnosticPointer *goxsd9.Diagnostic
	if errors.As(err, &diagnosticPointer) && diagnosticPointer != nil {
		return []error{*diagnosticPointer}
	}
	return []error{newCLIError(cliInternalCode, cliInternalKind, "-", "unclassified command failure", err)}
}

func diagnosticErrors(items []goxsd9.Diagnostic) []error {
	result := make([]error, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}

func writeDiagnostics(writer io.Writer, format diagnosticFormat, command, stage string, status int, errorsToRender []error) error {
	if writer == nil {
		return errors.New("diagnostic writer is nil")
	}
	rendered := make([]renderedDiagnostic, 0, len(errorsToRender))
	for _, err := range errorsToRender {
		rendered = append(rendered, renderDiagnostic(err))
	}
	if format == diagnosticsJSON {
		return json.NewEncoder(writer).Encode(diagnosticEnvelope{
			Format:      "goxsd9-diagnostics/v1",
			Command:     command,
			Stage:       stage,
			ExitStatus:  status,
			Diagnostics: rendered,
		})
	}
	for _, diagnostic := range rendered {
		line := diagnostic.human(command, stage)
		count, err := io.WriteString(writer, line+"\n")
		if err != nil {
			return err
		}
		if count != len(line)+1 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func renderDiagnostic(err error) renderedDiagnostic {
	var cli *cliError
	if errors.As(err, &cli) {
		return renderedDiagnostic{
			Kind:     cli.kind,
			Code:     cli.code,
			SourceID: sourceIDString(cli.sourceID),
			Location: renderedLineColumn{},
			Related:  make([]renderedRelated, 0),
			Message:  cli.message,
		}
	}
	var diagnostic goxsd9.Diagnostic
	if errors.As(err, &diagnostic) {
		return renderLibraryDiagnostic(diagnostic)
	}
	var diagnosticPointer *goxsd9.Diagnostic
	if errors.As(err, &diagnosticPointer) && diagnosticPointer != nil {
		return renderLibraryDiagnostic(*diagnosticPointer)
	}
	return renderedDiagnostic{
		Kind:     cliInternalKind,
		Code:     cliInternalCode,
		SourceID: "-",
		Location: renderedLineColumn{},
		Related:  make([]renderedRelated, 0),
		Message:  "unclassified command failure",
	}
}

func renderLibraryDiagnostic(diagnostic goxsd9.Diagnostic) renderedDiagnostic {
	class := string(diagnostic.Class())
	location := diagnostic.Loc()
	related := diagnostic.Related()
	renderedRelatedLocations := make([]renderedRelated, 0, len(related))
	for _, relatedLocation := range related {
		renderedRelatedLocations = append(renderedRelatedLocations, renderedRelated{
			SourceID: sourceIDString(relatedLocation.Source()),
			Location: renderedLineColumn{Line: relatedLocation.Line(), Column: relatedLocation.Column()},
		})
	}
	return renderedDiagnostic{
		Class:    &class,
		Kind:     "processing",
		Code:     diagnostic.Code(),
		SourceID: sourceIDString(location.Source()),
		Location: renderedLineColumn{Line: location.Line(), Column: location.Column()},
		Related:  renderedRelatedLocations,
		Feature:  string(diagnostic.Feature()),
		SpecRef:  diagnostic.SpecRef(),
		Message:  diagnostic.Message(),
	}
}

func sourceIDString(sourceID goxsd9.SourceID) string {
	if sourceID == "" {
		return "-"
	}
	return string(sourceID)
}

func (diagnostic renderedDiagnostic) human(command, stage string) string {
	class := "-"
	if diagnostic.Class != nil {
		class = *diagnostic.Class
	}
	fields := []string{
		command,
		"stage=" + stage,
		"class=" + class,
		"kind=" + diagnostic.Kind,
		"source_id=" + escapeHumanSourceID(diagnostic.SourceID),
		"location=" + strconv.Itoa(diagnostic.Location.Line) + ":" + strconv.Itoa(diagnostic.Location.Column),
		"code=" + diagnostic.Code,
	}
	if len(diagnostic.Related) > 0 {
		related := make([]string, 0, len(diagnostic.Related))
		for _, location := range diagnostic.Related {
			related = append(related, escapeHumanSourceID(location.SourceID)+":"+strconv.Itoa(location.Location.Line)+":"+strconv.Itoa(location.Location.Column))
		}
		fields = append(fields, "related="+strings.Join(related, ","))
	}
	if diagnostic.Feature != "" {
		fields = append(fields, "feature="+diagnostic.Feature)
	}
	if diagnostic.SpecRef != "" {
		fields = append(fields, "spec_ref="+diagnostic.SpecRef)
	}
	return strings.Join(fields, " ") + " " + strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(diagnostic.Message)
}

func escapeHumanSourceID(sourceID string) string {
	for index := 0; index < len(sourceID); {
		value, size := utf8.DecodeRuneInString(sourceID[index:])
		if isHumanSourceIDSeparator(value) {
			var escaped strings.Builder
			escaped.Grow(len(sourceID) + 2)
			escaped.WriteString(sourceID[:index])
			for remaining := index; remaining < len(sourceID); {
				value, size = utf8.DecodeRuneInString(sourceID[remaining:])
				if isHumanSourceIDSeparator(value) {
					quoted := strconv.QuoteRune(value)
					escaped.WriteString(quoted[1 : len(quoted)-1])
					remaining += size
					continue
				}
				escaped.WriteString(sourceID[remaining : remaining+size])
				remaining += size
			}
			return escaped.String()
		}
		index += size
	}
	return sourceID
}

func isHumanSourceIDSeparator(value rune) bool {
	return unicode.IsControl(value) || unicode.In(value, unicode.Cf, unicode.Zl, unicode.Zp)
}

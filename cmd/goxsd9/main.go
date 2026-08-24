// Command goxsd9 provides the first product command over the public schema API.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/goxdra/goxsd9"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithInput(args, os.Stdin, stdout, stderr)
}

func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return reportUsage(stderr, "parse", "missing command", diagnosticsHuman, "-")
	}
	switch args[0] {
	case "parse":
		return runParseCommand(args[1:], stdin, stdout, stderr)
	case "validate":
		return runValidateCommand(args[1:], stdin, stdout, stderr)
	case "generate":
		return runGenerateCommand(args[1:], stdin, stdout, stderr)
	default:
		return reportUsage(stderr, "parse", fmt.Sprintf("unknown command %q", args[0]), diagnosticsHuman, "-")
	}
}

func runParseCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, err := parseParseOptions(args)
	if err != nil {
		return reportUsage(stderr, "parse", err.Error(), options.diagnostics, usageSourceID(options.schema))
	}
	if options.schema == "-" && !options.schemaRootSet {
		return reportUsage(stderr, "parse", "schema stdin requires --schema-root", options.diagnostics, "schema/stdin")
	}

	return runParse(options, stdin, stdout, stderr)
}

func runValidateCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, err := parseValidateOptions(args)
	if err != nil {
		return reportUsage(stderr, "validate", err.Error(), options.diagnostics, validateUsageSourceID(options))
	}
	if options.schema == "-" && !options.schemaRootSet {
		return reportUsage(stderr, "validate", "schema stdin requires --schema-root", options.diagnostics, "schema/stdin")
	}

	return runValidate(options, stdin, stdout, stderr)
}

func runGenerateCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, err := parseGenerateOptions(args)
	if err != nil {
		return reportUsage(stderr, "generate", err.Error(), options.diagnostics, usageSourceID(options.schema))
	}
	if options.schema == "-" && !options.schemaRootSet {
		return reportUsage(stderr, "generate", "schema stdin requires --schema-root", options.diagnostics, "schema/stdin")
	}
	if options.force && (!options.outputSet || options.output == "-") {
		return reportUsage(stderr, "generate", "--force requires an explicit file destination", options.diagnostics, usageSourceID(options.schema))
	}

	return runGenerate(options, stdin, stdout, stderr)
}

type commandOptions struct {
	schema         string
	instance       string
	schemaRoot     string
	diagnostics    diagnosticFormat
	schemaRootSet  bool
	diagnosticsSet bool
}

type parseOptions = commandOptions
type validateOptions = commandOptions

type generateOptions struct {
	commandOptions
	packageName string
	packageSet  bool
	output      string
	outputSet   bool
	force       bool
	forceSet    bool
}

func parseParseOptions(args []string) (parseOptions, error) {
	options := parseOptions{diagnostics: diagnosticsHuman}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if options.schema != "" {
			if isFlag(argument) {
				return options, errors.New("flags must precede the schema operand")
			}
			return options, errors.New("parse accepts exactly one schema operand")
		}
		if argument == "" {
			return options, errors.New("schema operand is empty")
		}
		if argument == "-" || !isFlag(argument) {
			options.schema = argument
			continue
		}

		next, err := parseFlag(args, index, &options)
		if err != nil {
			return options, err
		}
		index = next
	}
	if options.schema == "" {
		return options, errors.New("parse requires one schema operand")
	}
	return options, nil
}

func parseValidateOptions(args []string) (validateOptions, error) {
	options := validateOptions{diagnostics: diagnosticsHuman}
	for index := 0; index < len(args); index++ {
		next, err := parseValidateArgument(args, index, &options)
		if err != nil {
			return options, err
		}
		index = next
	}
	if options.schema == "" {
		return options, errors.New("validate requires two operands")
	}
	if options.instance == "" {
		return options, errors.New("validate requires two operands")
	}
	if options.schema == "-" && options.instance == "-" {
		return options, errors.New("validate cannot read schema and instance from stdin")
	}
	return options, nil
}

func parseValidateArgument(args []string, index int, options *validateOptions) (int, error) {
	argument := args[index]
	if options.instance != "" {
		if argument == "" {
			return index, errors.New("operand is empty")
		}
		if isFlag(argument) {
			return index, errors.New("flags must precede operands")
		}
		return index, errors.New("validate accepts exactly two operands")
	}
	if options.schema != "" {
		if argument == "" {
			return index, errors.New("instance operand is empty")
		}
		if argument == "-" || !isFlag(argument) {
			options.instance = argument
			return index, nil
		}
		return index, errors.New("flags must precede operands")
	}
	if argument == "" {
		return index, errors.New("schema operand is empty")
	}
	if argument == "-" || !isFlag(argument) {
		options.schema = argument
		return index, nil
	}
	return parseFlag(args, index, options)
}

func parseGenerateOptions(args []string) (generateOptions, error) {
	options := generateOptions{commandOptions: commandOptions{diagnostics: diagnosticsHuman}}
	for index := 0; index < len(args); index++ {
		next, err := parseGenerateArgument(args, index, &options)
		if err != nil {
			return options, err
		}
		index = next
	}
	if options.schema == "" {
		return options, errors.New("generate requires one schema operand")
	}
	if !options.packageSet {
		return options, errors.New("generate requires --package")
	}
	if options.force && (!options.outputSet || options.output == "-") {
		return options, errors.New("--force requires an explicit file destination")
	}
	return options, nil
}

func parseGenerateArgument(args []string, index int, options *generateOptions) (int, error) {
	argument := args[index]
	if options.schema != "" {
		if isFlag(argument) {
			return index, errors.New("flags must precede the schema operand")
		}
		return index, errors.New("generate accepts exactly one schema operand")
	}
	if argument == "" {
		return index, errors.New("schema operand is empty")
	}
	if argument == "-" || !isFlag(argument) {
		options.schema = argument
		return index, nil
	}
	return parseGenerateFlag(args, index, options)
}

func parseGenerateFlag(args []string, index int, options *generateOptions) (int, error) {
	argument := args[index]
	switch {
	case argument == "--package" || hasFlagValue(argument, "--package"):
		return parsePackageFlag(args, index, options)
	case argument == "--output" || hasFlagValue(argument, "--output"):
		return parseOutputFlag(args, index, options)
	case argument == "--force":
		if options.forceSet {
			return index, errors.New("duplicate --force flag")
		}
		options.force = true
		options.forceSet = true
		return index, nil
	default:
		return parseFlag(args, index, &options.commandOptions)
	}
}

func parsePackageFlag(args []string, index int, options *generateOptions) (int, error) {
	if options.packageSet {
		return index, errors.New("duplicate --package flag")
	}
	value, next, err := flagValue(args, index, "--package")
	if err != nil {
		return index, err
	}
	if value == "" {
		return index, errors.New("--package requires a name")
	}
	options.packageName = value
	options.packageSet = true
	return next, nil
}

func parseOutputFlag(args []string, index int, options *generateOptions) (int, error) {
	if options.outputSet {
		return index, errors.New("duplicate --output flag")
	}
	value, next, err := flagValueAllowDash(args, index, "--output")
	if err != nil {
		return index, err
	}
	if value == "" {
		return index, errors.New("--output requires a file or -")
	}
	options.output = value
	options.outputSet = true
	return next, nil
}

func flagValueAllowDash(args []string, index int, name string) (string, int, error) {
	argument := args[index]
	if hasFlagValue(argument, name) {
		return argument[len(name)+1:], index, nil
	}
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", name)
	}
	if isFlag(args[index+1]) && args[index+1] != "-" {
		return "", index, fmt.Errorf("%s requires a value", name)
	}
	return args[index+1], index + 1, nil
}

func parseFlag(args []string, index int, options *commandOptions) (int, error) {
	argument := args[index]
	switch {
	case argument == "--schema-root" || hasFlagValue(argument, "--schema-root"):
		return parseSchemaRootFlag(args, index, options)
	case argument == "--diagnostics" || hasFlagValue(argument, "--diagnostics"):
		return parseDiagnosticsFlag(args, index, options)
	default:
		return index, fmt.Errorf("unknown option %q", argument)
	}
}

func parseSchemaRootFlag(args []string, index int, options *parseOptions) (int, error) {
	if options.schemaRootSet {
		return index, errors.New("duplicate --schema-root flag")
	}
	value, next, err := flagValue(args, index, "--schema-root")
	if err != nil {
		return index, err
	}
	if value == "" {
		return index, errors.New("--schema-root requires a directory")
	}
	options.schemaRoot = value
	options.schemaRootSet = true
	return next, nil
}

func parseDiagnosticsFlag(args []string, index int, options *parseOptions) (int, error) {
	if options.diagnosticsSet {
		return index, errors.New("duplicate --diagnostics flag")
	}
	value, next, err := flagValue(args, index, "--diagnostics")
	if err != nil {
		return index, err
	}
	if value != string(diagnosticsHuman) && value != string(diagnosticsJSON) {
		return index, errors.New("--diagnostics must be human or json")
	}
	options.diagnostics = diagnosticFormat(value)
	options.diagnosticsSet = true
	return next, nil
}

func isFlag(argument string) bool {
	return len(argument) > 0 && argument[0] == '-'
}

func hasFlagValue(argument, name string) bool {
	return len(argument) > len(name) && argument[:len(name)] == name && argument[len(name)] == '='
}

func flagValue(args []string, index int, name string) (string, int, error) {
	argument := args[index]
	if hasFlagValue(argument, name) {
		return argument[len(name)+1:], index, nil
	}
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", name)
	}
	if isFlag(args[index+1]) {
		return "", index, fmt.Errorf("%s requires a value", name)
	}
	return args[index+1], index + 1, nil
}

func usageSourceID(schema string) goxsd9.SourceID {
	if schema == "-" {
		return "schema/stdin"
	}
	return "-"
}

func validateUsageSourceID(options validateOptions) goxsd9.SourceID {
	if options.instance == "-" {
		return "instance/stdin"
	}
	if options.schema == "-" {
		return "schema/stdin"
	}
	return "-"
}

func runParse(options parseOptions, stdin io.Reader, stdout, stderr io.Writer) int {
	plan, err := prepareSchemaPlan(options)
	if err != nil {
		return reportError(stderr, "parse", options.diagnostics, "parse", err)
	}

	budget := &schemaBudget{}
	root, err := plan.openRoot(stdin, budget)
	if err != nil {
		return reportError(stderr, "parse", options.diagnostics, "parse", err)
	}

	schema, err := goxsd9.ParseSchema(root, plan.resolver(budget))
	if err != nil {
		return reportError(stderr, "parse", options.diagnostics, "parse", err)
	}

	output := fmt.Sprintf("documents=%d components=%d\n", len(schema.Documents()), len(schema.Components()))
	if err := writeOutput(stdout, output); err != nil {
		return reportError(stderr, "parse", options.diagnostics, "output", newCLIError(cliOutputCode, cliOutputKind, "output/stdout", "failed to write parse summary", err))
	}
	return 0
}

func runValidate(options validateOptions, stdin io.Reader, _, stderr io.Writer) int {
	plan, err := prepareSchemaPlan(options)
	if err != nil {
		return reportError(stderr, "validate", options.diagnostics, "parse", err)
	}

	budget := &schemaBudget{}
	root, err := plan.openRoot(stdin, budget)
	if err != nil {
		return reportError(stderr, "validate", options.diagnostics, "parse", err)
	}

	schema, err := goxsd9.ParseSchema(root, plan.resolver(budget))
	if err != nil {
		return reportError(stderr, "validate", options.diagnostics, "parse", err)
	}

	instance, err := prepareInstancePlan(options.instance)
	if err != nil {
		return reportError(stderr, "validate", options.diagnostics, "validate", err)
	}
	reader, err := instance.open(stdin)
	if err != nil {
		return reportError(stderr, "validate", options.diagnostics, "validate", err)
	}
	bounded := newInstanceSource(reader, instance.sourceID)
	if err := goxsd9.ValidateInstance(schema, instance.sourceID, bounded); err != nil {
		return reportError(stderr, "validate", options.diagnostics, "validate", err)
	}
	return 0
}

func writeOutput(writer io.Writer, output string) error {
	if writer == nil {
		return errors.New("stdout writer is nil")
	}
	count, err := io.WriteString(writer, output)
	if err != nil {
		return err
	}
	if count != len(output) {
		return io.ErrShortWrite
	}
	return nil
}

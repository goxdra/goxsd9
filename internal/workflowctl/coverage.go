package workflowctl

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var coverageGoEnvironment = []string{
	"GOPROXY=off",
	"GOSUMDB=off",
	"GOTOOLCHAIN=local",
	"GOFLAGS=-mod=readonly",
}

type coverageGoPackage struct {
	path        string
	relativeDir string
	hasTests    bool
}

type coverageSnapshot struct {
	packages map[string]coverageGoPackage
	paths    []string
	counts   map[string]coverageCounts
}

type coverageCounts struct {
	statements int
	covered    int
}

type coverageReport struct {
	Base       string                  `json:"base"`
	Head       string                  `json:"head"`
	Packages   []coveragePackageReport `json:"packages"`
	Affected   coverageTotalsReport    `json:"affected"`
	Repository coverageTotalsReport    `json:"repository"`
}

type coveragePackageReport struct {
	Package  string              `json:"package"`
	Status   string              `json:"status"`
	Affected bool                `json:"affected"`
	Base     coverageSideReport  `json:"base"`
	Head     coverageSideReport  `json:"head"`
	Delta    coverageDeltaReport `json:"delta"`
}

type coverageSideReport struct {
	Present    bool    `json:"present"`
	HasTests   bool    `json:"hasTests"`
	Statements int     `json:"statements"`
	Covered    int     `json:"covered"`
	Percent    float64 `json:"percent"`
}

type coverageDeltaReport struct {
	Statements int     `json:"statements"`
	Covered    int     `json:"covered"`
	Percent    float64 `json:"percent"`
}

type coverageTotalsReport struct {
	Base  coverageAggregate      `json:"base"`
	Head  coverageAggregate      `json:"head"`
	Delta coverageAggregateDelta `json:"delta"`
}

type coverageAggregate struct {
	Packages       int     `json:"packages"`
	TestedPackages int     `json:"testedPackages"`
	Statements     int     `json:"statements"`
	Covered        int     `json:"covered"`
	Percent        float64 `json:"percent"`
}

type coverageAggregateDelta struct {
	Packages       int     `json:"packages"`
	TestedPackages int     `json:"testedPackages"`
	Statements     int     `json:"statements"`
	Covered        int     `json:"covered"`
	Percent        float64 `json:"percent"`
}

type goListCoveragePackage struct {
	Dir          string   `json:"Dir"`
	ImportPath   string   `json:"ImportPath"`
	TestGoFiles  []string `json:"TestGoFiles"`
	XTestGoFiles []string `json:"XTestGoFiles"`
}

func (a app) runCoverage(args []string) error {
	flags := flag.NewFlagSet("coverage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("base", "", "Git base reference")
	format := flags.String("format", "text", "output format: text or json")
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return usageError("coverage: %v", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*base) == "" {
		return usageError("usage: workflowctl coverage --base REF [--format text|json]")
	}
	if *jsonOutput {
		*format = "json"
	}
	if *format != "text" && *format != "json" {
		return usageError("coverage: unsupported output format %q", *format)
	}

	root, err := a.root()
	if err != nil {
		return err
	}
	report, err := a.buildCoverageReport(root, *base)
	if err != nil {
		return err
	}
	if *format == "json" {
		return a.writeCoverageJSON(report)
	}
	return a.writeCoverageText(report)
}

func (a app) buildCoverageReport(root, baseRef string) (report coverageReport, err error) {
	clean, err := a.command(root, "git", "status", "--porcelain")
	if err != nil {
		return coverageReport{}, fmt.Errorf("read coverage worktree status: %w", err)
	}
	if strings.TrimSpace(clean) != "" {
		return coverageReport{}, stateError("coverage requires a clean worktree")
	}
	base, err := a.resolveCommit(root, baseRef)
	if err != nil {
		return coverageReport{}, fmt.Errorf("resolve coverage base %q: %w", baseRef, err)
	}
	head, err := a.resolveCommit(root, "HEAD")
	if err != nil {
		return coverageReport{}, fmt.Errorf("resolve coverage head: %w", err)
	}
	changed, err := a.coverageChangedPaths(root, base, head)
	if err != nil {
		return coverageReport{}, err
	}

	temporaryRoot, err := os.MkdirTemp("", "goxsd9-coverage-")
	if err != nil {
		return coverageReport{}, fmt.Errorf("create coverage temporary directory: %w", err)
	}
	baseRoot := filepath.Join(temporaryRoot, "base")
	worktreeAdded := false
	defer func() {
		if worktreeAdded {
			cleanupErr := a.removeCoverageWorktree(root, baseRoot)
			err = joinCoverageErrors(err, cleanupErr)
		}
		cleanupErr := os.RemoveAll(temporaryRoot)
		err = joinCoverageErrors(err, cleanupErr)
	}()

	if err := a.addCoverageWorktree(root, baseRoot, base); err != nil {
		return coverageReport{}, err
	}
	worktreeAdded = true
	baseSnapshot, err := a.coverageSnapshot(baseRoot, filepath.Join(temporaryRoot, "base.cover"), "base")
	if err != nil {
		return coverageReport{}, err
	}
	headSnapshot, err := a.coverageSnapshot(root, filepath.Join(temporaryRoot, "head.cover"), "head")
	if err != nil {
		return coverageReport{}, err
	}
	return assembleCoverageReport(base, head, changed, baseSnapshot, headSnapshot), nil
}

func joinCoverageErrors(current, cleanup error) error {
	if cleanup == nil {
		return current
	}
	if current == nil {
		return cleanup
	}
	return errors.Join(current, cleanup)
}

func (a app) addCoverageWorktree(root, directory, revision string) error {
	if _, err := a.command(root, "git", "worktree", "add", "--detach", "--quiet", directory, revision); err != nil {
		return fmt.Errorf("create coverage base worktree: %w", err)
	}
	return nil
}

func (a app) removeCoverageWorktree(root, directory string) error {
	if _, err := a.command(root, "git", "worktree", "remove", "--force", directory); err != nil {
		return fmt.Errorf("remove coverage base worktree: %w", err)
	}
	return nil
}

func (a app) coverageChangedPaths(root, base, head string) (map[string]bool, error) {
	output, err := a.gitRaw(root, "diff", "--name-only", "-z", "--no-renames", base, head, "--")
	if err != nil {
		return nil, fmt.Errorf("read coverage changed paths: %w", err)
	}
	paths, err := parseCoveragePaths(output)
	if err != nil {
		return nil, fmt.Errorf("parse coverage changed paths: %w", err)
	}
	changed := make(map[string]bool)
	for _, filePath := range paths {
		if strings.HasSuffix(filePath, ".go") {
			changed[filePath] = true
		}
	}
	return changed, nil
}

func parseCoveragePaths(output string) ([]string, error) {
	if output == "" {
		return nil, nil
	}
	fields := strings.Split(output, "\x00")
	paths := make([]string, 0, len(fields))
	for index, field := range fields {
		if field == "" {
			if index == len(fields)-1 {
				continue
			}
			return nil, fmt.Errorf("empty path at field %d", index+1)
		}
		if filepath.IsAbs(filepath.FromSlash(field)) || path.Clean(field) != field || path.Clean(field) == "." {
			return nil, fmt.Errorf("unsafe path %q", field)
		}
		paths = append(paths, field)
	}
	return paths, nil
}

func (a app) coverageSnapshot(directory, profilePath, side string) (coverageSnapshot, error) {
	packages, err := a.listCoveragePackages(directory, side)
	if err != nil {
		return coverageSnapshot{}, err
	}
	if _, err := a.commandWithEnv(directory, coverageGoEnvironment, "go", "test", "-count=1", "-covermode=set",
		"-coverpkg=./...", "-coverprofile="+profilePath, "./..."); err != nil {
		return coverageSnapshot{}, fmt.Errorf("run %s coverage tests: %w", side, err)
	}
	counts, err := readCoverageProfile(profilePath, packages.paths)
	if err != nil {
		return coverageSnapshot{}, fmt.Errorf("read %s coverage profile: %w", side, err)
	}
	return coverageSnapshot{packages: packages.packages, paths: packages.paths, counts: counts}, nil
}

func (a app) listCoveragePackages(directory, side string) (coverageSnapshot, error) {
	output, err := a.commandWithEnv(directory, coverageGoEnvironment, "go", "list", "-json", "./...")
	if err != nil {
		return coverageSnapshot{}, fmt.Errorf("list %s Go packages: %w", side, err)
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	packages := make(map[string]coverageGoPackage)
	for {
		var listed goListCoveragePackage
		decodeErr := decoder.Decode(&listed)
		if errors.Is(decodeErr, io.EOF) {
			break
		}
		if decodeErr != nil {
			return coverageSnapshot{}, fmt.Errorf("decode %s Go package list: %w", side, decodeErr)
		}
		if listed.ImportPath == "" || listed.Dir == "" {
			return coverageSnapshot{}, fmt.Errorf("decode %s Go package list: missing package path or directory", side)
		}
		if _, exists := packages[listed.ImportPath]; exists {
			return coverageSnapshot{}, fmt.Errorf("decode %s Go package list: duplicate package %q", side, listed.ImportPath)
		}
		relativeDir, relErr := filepath.Rel(directory, listed.Dir)
		if relErr != nil {
			return coverageSnapshot{}, fmt.Errorf("resolve %s package %q directory: %w", side, listed.ImportPath, relErr)
		}
		if relativeDir == ".." || strings.HasPrefix(relativeDir, ".."+string(filepath.Separator)) {
			return coverageSnapshot{}, fmt.Errorf("%s package %q is outside its checkout", side, listed.ImportPath)
		}
		packages[listed.ImportPath] = coverageGoPackage{
			path: listed.ImportPath, relativeDir: filepath.ToSlash(relativeDir),
			hasTests: len(listed.TestGoFiles) != 0 || len(listed.XTestGoFiles) != 0,
		}
	}
	if len(packages) == 0 {
		return coverageSnapshot{}, fmt.Errorf("list %s Go packages: no packages found", side)
	}
	paths := make([]string, 0, len(packages))
	for packagePath := range packages {
		paths = append(paths, packagePath)
	}
	sort.Strings(paths)
	return coverageSnapshot{packages: packages, paths: paths}, nil
}

func readCoverageProfile(profilePath string, packagePaths []string) (counts map[string]coverageCounts, err error) {
	file, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", profilePath, err)
	}
	defer func() {
		closeErr := file.Close()
		err = joinCoverageErrors(err, closeErr)
	}()

	counts = make(map[string]coverageCounts)
	seenBlocks := make(map[string]coverageBlock)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	lineNumber := 0
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return nil, fmt.Errorf("scan profile: %w", scanErr)
		}
		return nil, errors.New("coverage profile is empty")
	}
	lineNumber++
	mode := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "mode:"))
	if !strings.HasPrefix(scanner.Text(), "mode:") || mode == "" {
		return nil, fmt.Errorf("line %d: missing coverage mode", lineNumber)
	}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		packagePath, block, parseErr := parseCoverageBlock(line, packagePaths)
		if parseErr != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, parseErr)
		}
		previous, seen := seenBlocks[block.key]
		if seen {
			if previous.covered != 0 || block.covered == 0 {
				continue
			}
			seenBlocks[block.key] = block
			current := counts[packagePath]
			if block.statements > maxCoverageInt()-current.covered {
				return nil, fmt.Errorf("line %d: covered count overflows int", lineNumber)
			}
			current.covered += block.statements
			counts[packagePath] = current
			continue
		}
		seenBlocks[block.key] = block
		current := counts[packagePath]
		if block.statements > maxCoverageInt()-current.statements {
			return nil, fmt.Errorf("line %d: statement count overflows int", lineNumber)
		}
		current.statements += block.statements
		if block.covered != 0 {
			if block.statements > maxCoverageInt()-current.covered {
				return nil, fmt.Errorf("line %d: covered count overflows int", lineNumber)
			}
			current.covered += block.statements
		}
		counts[packagePath] = current
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan profile: %w", scanErr)
	}
	for _, packagePath := range packagePaths {
		current := counts[packagePath]
		if current.covered > current.statements {
			return nil, fmt.Errorf("package %q has more covered than total statements", packagePath)
		}
	}
	return counts, nil
}

type coverageBlock struct {
	key        string
	statements int
	covered    int
}

func parseCoverageBlock(line string, packagePaths []string) (string, coverageBlock, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", coverageBlock{}, errors.New("coverage block must contain a path, range, statement count, and execution count")
	}
	separator := strings.LastIndex(fields[0], ":")
	if separator < 1 || separator == len(fields[0])-1 {
		return "", coverageBlock{}, errors.New("coverage block has an invalid source range")
	}
	if err := validateCoverageRange(fields[0][separator+1:]); err != nil {
		return "", coverageBlock{}, err
	}
	statements, err := strconv.Atoi(fields[1])
	if err != nil || statements < 0 {
		return "", coverageBlock{}, fmt.Errorf("invalid statement count %q", fields[1])
	}
	executions, err := strconv.Atoi(fields[2])
	if err != nil || executions < 0 {
		return "", coverageBlock{}, fmt.Errorf("invalid execution count %q", fields[2])
	}
	sourcePath := fields[0][:separator]
	packagePath := coveragePackageForSource(sourcePath, packagePaths)
	if packagePath == "" {
		return "", coverageBlock{}, fmt.Errorf("source %q does not belong to a listed package", sourcePath)
	}
	return packagePath, coverageBlock{key: fields[0] + " " + fields[1], statements: statements, covered: executions}, nil
}

func validateCoverageRange(text string) error {
	parts := strings.Split(text, ",")
	if len(parts) != 2 {
		return fmt.Errorf("invalid source range %q", text)
	}
	for _, part := range parts {
		position := strings.Index(part, ".")
		if position < 1 || position == len(part)-1 {
			return fmt.Errorf("invalid source range %q", text)
		}
		line, lineErr := strconv.Atoi(part[:position])
		column, columnErr := strconv.Atoi(part[position+1:])
		if lineErr != nil || columnErr != nil || line < 1 || column < 1 {
			return fmt.Errorf("invalid source range %q", text)
		}
	}
	return nil
}

func coveragePackageForSource(sourcePath string, packagePaths []string) string {
	matched := ""
	for _, packagePath := range packagePaths {
		if strings.HasPrefix(sourcePath, packagePath+"/") && len(packagePath) > len(matched) {
			matched = packagePath
		}
	}
	return matched
}

func maxCoverageInt() int {
	return int(^uint(0) >> 1)
}

func assembleCoverageReport(base, head string, changed map[string]bool, baseSnapshot, headSnapshot coverageSnapshot) coverageReport {
	allPaths := make(map[string]bool, len(baseSnapshot.paths)+len(headSnapshot.paths))
	for _, packagePath := range baseSnapshot.paths {
		allPaths[packagePath] = true
	}
	for _, packagePath := range headSnapshot.paths {
		allPaths[packagePath] = true
	}
	paths := make([]string, 0, len(allPaths))
	for packagePath := range allPaths {
		paths = append(paths, packagePath)
	}
	sort.Strings(paths)

	reports := make([]coveragePackageReport, 0, len(paths))
	for _, packagePath := range paths {
		basePackage, basePresent := baseSnapshot.packages[packagePath]
		headPackage, headPresent := headSnapshot.packages[packagePath]
		affected := coveragePackageAffected(changed, basePackage, basePresent, headPackage, headPresent)
		status := coveragePackageStatus(basePresent, headPresent, affected)
		baseSide := makeCoverageSide(basePresent, basePackage, baseSnapshot.counts[packagePath])
		headSide := makeCoverageSide(headPresent, headPackage, headSnapshot.counts[packagePath])
		reports = append(reports, coveragePackageReport{
			Package: packagePath, Status: status, Affected: affected,
			Base: baseSide, Head: headSide, Delta: coverageDelta(baseSide, headSide),
		})
	}
	return coverageReport{
		Base: base, Head: head, Packages: reports,
		Affected: coverageTotals(reports, true), Repository: coverageTotals(reports, false),
	}
}

func coveragePackageAffected(changed map[string]bool, base coverageGoPackage, basePresent bool,
	head coverageGoPackage, headPresent bool,
) bool {
	if !basePresent || !headPresent {
		return true
	}
	for filePath := range changed {
		fileDir := path.Dir(filePath)
		if fileDir == base.relativeDir || fileDir == head.relativeDir {
			return true
		}
	}
	return false
}

func coveragePackageStatus(basePresent, headPresent, affected bool) string {
	if !basePresent {
		return "added"
	}
	if !headPresent {
		return "removed"
	}
	if affected {
		return "changed"
	}
	return "unchanged"
}

func makeCoverageSide(present bool, packageInfo coverageGoPackage, counts coverageCounts) coverageSideReport {
	if !present {
		return coverageSideReport{}
	}
	return coverageSideReport{
		Present: true, HasTests: packageInfo.hasTests, Statements: counts.statements,
		Covered: counts.covered, Percent: coveragePercent(counts.covered, counts.statements),
	}
}

func coverageDelta(base, head coverageSideReport) coverageDeltaReport {
	return coverageDeltaReport{
		Statements: head.Statements - base.Statements,
		Covered:    head.Covered - base.Covered,
		Percent:    coveragePercent(head.Covered, head.Statements) - coveragePercent(base.Covered, base.Statements),
	}
}

func coverageTotals(packages []coveragePackageReport, affectedOnly bool) coverageTotalsReport {
	base := coverageAggregateFor(packages, affectedOnly, true)
	head := coverageAggregateFor(packages, affectedOnly, false)
	return coverageTotalsReport{
		Base: base, Head: head,
		Delta: coverageAggregateDelta{Packages: head.Packages - base.Packages,
			TestedPackages: head.TestedPackages - base.TestedPackages,
			Statements:     head.Statements - base.Statements, Covered: head.Covered - base.Covered,
			Percent: coveragePercent(head.Covered, head.Statements) - coveragePercent(base.Covered, base.Statements)},
	}
}

func coverageAggregateFor(packages []coveragePackageReport, affectedOnly, baseSide bool) coverageAggregate {
	aggregate := coverageAggregate{}
	for _, packageReport := range packages {
		if affectedOnly && !packageReport.Affected {
			continue
		}
		side := packageReport.Head
		if baseSide {
			side = packageReport.Base
		}
		if !side.Present {
			continue
		}
		aggregate.Packages++
		if side.HasTests {
			aggregate.TestedPackages++
		}
		aggregate.Statements += side.Statements
		aggregate.Covered += side.Covered
	}
	aggregate.Percent = coveragePercent(aggregate.Covered, aggregate.Statements)
	return aggregate
}

func coveragePercent(covered, statements int) float64 {
	if statements == 0 {
		return 0
	}
	return math.Round(float64(covered)*1000/float64(statements)) / 10
}

func (a app) writeCoverageJSON(report coverageReport) error {
	encoder := json.NewEncoder(a.stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode coverage report: %w", err)
	}
	return nil
}

func (a app) writeCoverageText(report coverageReport) error {
	var output strings.Builder
	if err := writeCoverageTextLine(&output, "Coverage delta"); err != nil {
		return err
	}
	if err := writeCoverageTextLine(&output, "Base: %s", report.Base); err != nil {
		return err
	}
	if err := writeCoverageTextLine(&output, "Head: %s", report.Head); err != nil {
		return err
	}
	if err := writeCoverageTextLine(&output, "\n## Packages"); err != nil {
		return err
	}
	nameWidth := len("PACKAGE")
	for _, packageReport := range report.Packages {
		if len(packageReport.Package) > nameWidth {
			nameWidth = len(packageReport.Package)
		}
	}
	if _, err := fmt.Fprintf(&output, "%-*s  %-9s  %-9s  %-20s  %-20s  %s\n", nameWidth, "PACKAGE",
		"STATUS", "TESTS", "BASE", "HEAD", "DELTA"); err != nil {
		return err
	}
	for _, packageReport := range report.Packages {
		if _, err := fmt.Fprintf(&output, "%-*s  %-9s  %-9s  %-20s  %-20s  %s\n", nameWidth, packageReport.Package,
			packageReport.Status, coverageTestKind(packageReport), formatCoverageSide(packageReport.Base),
			formatCoverageSide(packageReport.Head), formatCoverageDelta(packageReport.Delta)); err != nil {
			return err
		}
	}
	if err := writeCoverageTotalsText(&output, "Affected packages", report.Affected); err != nil {
		return err
	}
	if err := writeCoverageTotalsText(&output, "Repository totals", report.Repository); err != nil {
		return err
	}
	if _, err := io.WriteString(a.stdout, output.String()); err != nil {
		return fmt.Errorf("write coverage report: %w", err)
	}
	return nil
}

func writeCoverageTextLine(output *strings.Builder, format string, args ...any) error {
	_, err := fmt.Fprintf(output, format+"\n", args...)
	return err
}

func writeCoverageTotalsText(output *strings.Builder, title string, totals coverageTotalsReport) error {
	if err := writeCoverageTextLine(output, "\n## %s", title); err != nil {
		return err
	}
	if err := writeCoverageTextLine(output, "Base: %s across %d packages (%d with tests)",
		formatCoverageAggregate(totals.Base), totals.Base.Packages, totals.Base.TestedPackages); err != nil {
		return err
	}
	if err := writeCoverageTextLine(output, "Head: %s across %d packages (%d with tests)",
		formatCoverageAggregate(totals.Head), totals.Head.Packages, totals.Head.TestedPackages); err != nil {
		return err
	}
	return writeCoverageTextLine(output, "Delta: %s statements, %s covered, %s percentage points",
		formatSignedInt(totals.Delta.Statements), formatSignedInt(totals.Delta.Covered),
		formatSignedPercent(totals.Delta.Percent))
}

func coverageTestKind(packageReport coveragePackageReport) string {
	if packageReport.Base.HasTests && packageReport.Head.HasTests {
		return "both"
	}
	if packageReport.Base.HasTests {
		return "base-only"
	}
	if packageReport.Head.HasTests {
		return "head-only"
	}
	return "none"
}

func formatCoverageSide(side coverageSideReport) string {
	if !side.Present {
		return "absent"
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", side.Percent, side.Covered, side.Statements)
}

func formatCoverageDelta(delta coverageDeltaReport) string {
	return fmt.Sprintf("%s (%s/%s)", formatSignedPercent(delta.Percent), formatSignedInt(delta.Covered),
		formatSignedInt(delta.Statements))
}

func formatCoverageAggregate(aggregate coverageAggregate) string {
	return fmt.Sprintf("%.1f%% (%d/%d)", aggregate.Percent, aggregate.Covered, aggregate.Statements)
}

func formatSignedInt(value int) string {
	if value >= 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func formatSignedPercent(value float64) string {
	return fmt.Sprintf("%+.1f%%", value)
}

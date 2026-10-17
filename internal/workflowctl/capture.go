package workflowctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	captureSchema = "goxsd9/workflow-capture/v1"
	captureLimit  = 8 << 20
)

// A capture preserves a previously completed command independently of later reads.
// Byte slices use JSON base64 encoding; the capture command never runs that command.
type captureCommandResult struct {
	Schema     string `json:"schema"`
	Operation  string `json:"operation"`
	Stdout     []byte `json:"stdout"`
	Stderr     []byte `json:"stderr"`
	ExitStatus *int   `json:"exitStatus"`
}

type captureComment struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Body      []byte `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type captureReceipt struct {
	CommentID     int64             `json:"commentID"`
	Receipt       evaluationReceipt `json:"receipt"`
	ReceiptMarker []byte            `json:"receiptMarker"`
	Attestation   []byte            `json:"attestation"`
	CommentBody   []byte            `json:"commentBody"`
}

type capturePRFacts struct {
	Number         int              `json:"number"`
	URL            string           `json:"url"`
	BaseRef        string           `json:"baseRef"`
	Base           string           `json:"base"`
	HeadRef        string           `json:"headRef"`
	Head           string           `json:"head"`
	Body           []byte           `json:"body"`
	BodySHA256     string           `json:"bodySHA256"`
	Comments       []captureComment `json:"comments"`
	Evidence       *prEvidence      `json:"evidence,omitempty"`
	EvidenceBlock  []byte           `json:"evidenceBlock,omitempty"`
	EvidenceSHA256 string           `json:"evidenceSHA256,omitempty"`
	Receipts       []captureReceipt `json:"receipts"`
}

type captureGitObject struct {
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	OID    string `json:"oid"`
	Kind   string `json:"kind"`
	Bytes  []byte `json:"bytes"`
}

type captureEnvelope struct {
	Schema       string                `json:"schema"`
	Command      *captureCommandResult `json:"command,omitempty"`
	PR           *capturePRFacts       `json:"pr,omitempty"`
	Artifact     any                   `json:"artifact,omitempty"`
	GitObject    *captureGitObject     `json:"gitObject,omitempty"`
	CaptureError string                `json:"captureError,omitempty"`
}

func (a app) runCapture(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl capture pr PR [--expected-head SHA] [--command-result-file FILE] | artifact NAME --file FILE --base SHA --head SHA [--audit-file FILE] | git-object --commit SHA --path PATH")
	}
	switch args[0] {
	case "pr":
		return a.runCapturePR(args[1:])
	case "artifact":
		return a.runCaptureArtifact(args[1:])
	case "git-object":
		return a.runCaptureGitObject(args[1:])
	default:
		return usageError("unknown capture command %q", args[0])
	}
}

func (a app) writeCapture(envelope captureEnvelope, captureErr error) error {
	envelope.Schema = captureSchema
	if captureErr != nil {
		envelope.CaptureError = captureErr.Error()
	}
	if err := json.NewEncoder(a.stdout).Encode(envelope); err != nil {
		return fmt.Errorf("write capture: %w", err)
	}
	if captureErr != nil {
		return fmt.Errorf("capture: %w", captureErr)
	}
	return nil
}

func (a app) runCapturePR(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl capture pr PR [--expected-head SHA] [--command-result-file FILE]")
	}
	number, err := positiveNumber(args[0])
	if err != nil {
		return usageError("capture pr: %v", err)
	}
	flags := flag.NewFlagSet("capture pr", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expectedHead := flags.String("expected-head", "", "exact REST head SHA")
	commandPath := flags.String("command-result-file", "", "previously recorded command result")
	if parseErr := flags.Parse(args[1:]); parseErr != nil || flags.NArg() != 0 {
		return usageError("usage: workflowctl capture pr PR [--expected-head SHA] [--command-result-file FILE]")
	}
	envelope := captureEnvelope{}
	if *commandPath != "" {
		var result captureCommandResult
		if readErr := readCaptureJSONFile(*commandPath, "command result", &result); readErr != nil {
			return a.writeCapture(envelope, readErr)
		}
		if result.Schema != "goxsd9/command-result/v1" || strings.TrimSpace(result.Operation) == "" ||
			result.Stdout == nil || result.Stderr == nil || result.ExitStatus == nil || *result.ExitStatus < 0 {
			return a.writeCapture(envelope, errors.New("command result requires schema goxsd9/command-result/v1, operation, stdout, stderr, and nonnegative exitStatus"))
		}
		envelope.Command = &result
	}
	root, err := a.root()
	if err != nil {
		return a.writeCapture(envelope, fmt.Errorf("find repository: %w", err))
	}
	facts, err := a.capturePR(root, number, *expectedHead)
	if err != nil {
		return a.writeCapture(envelope, err)
	}
	envelope.PR = &facts
	return a.writeCapture(envelope, nil)
}

//nolint:gocognit // Exact PR, evidence, receipt, and source-drift checks form one ordered read.
func (a app) capturePR(root string, number int, expectedHead string) (capturePRFacts, error) {
	if expectedHead != "" && !validExactCommitSHA(expectedHead) {
		return capturePRFacts{}, fmt.Errorf("expected head %q is not an exact commit SHA", expectedHead)
	}
	view, err := a.readPullRequestForResume(root, number)
	if err != nil {
		return capturePRFacts{}, fmt.Errorf("read exact PR: %w", err)
	}
	if expectedHead != "" && view.HeadRefOID != expectedHead {
		return capturePRFacts{}, fmt.Errorf("PR #%d head changed: got %q, want %q", number, view.HeadRefOID, expectedHead)
	}
	if strings.TrimSpace(view.URL) == "" {
		return capturePRFacts{}, fmt.Errorf("PR #%d has no canonical URL", number)
	}
	if len(view.Body) > captureLimit || len(view.Comments) > 10000 {
		return capturePRFacts{}, errors.New("PR body or comment list exceeds capture bound")
	}
	facts := capturePRFacts{
		Number: number, URL: view.URL, BaseRef: view.BaseRefName, Base: view.BaseRefOID,
		HeadRef: view.HeadRefName, Head: view.HeadRefOID, Body: []byte(view.Body),
		BodySHA256: sha256Hex([]byte(view.Body)), Comments: make([]captureComment, 0, len(view.Comments)),
		Receipts: make([]captureReceipt, 0),
	}
	totalCommentBytes := 0
	for _, comment := range view.Comments {
		totalCommentBytes += len(comment.Body)
		if totalCommentBytes > captureLimit {
			return capturePRFacts{}, fmt.Errorf("PR comments exceed capture bound at comment %d", comment.ID)
		}
		facts.Comments = append(facts.Comments, captureComment{
			ID: comment.ID, Author: comment.Author.Login, Body: []byte(comment.Body),
			CreatedAt: comment.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		})
	}
	if strings.Contains(view.Body, evidenceMarkerToken(prEvidenceStartMarker)) ||
		strings.Contains(view.Body, evidenceMarkerToken(prEvidenceEndMarker)) {
		parsed, parseErr := validatePREvidenceForView(view)
		if parseErr != nil {
			return capturePRFacts{}, fmt.Errorf("PR #%d evidence: %w", number, parseErr)
		}
		_, digest := currentPREvidenceDigest(view, parsed)
		facts.Evidence, facts.EvidenceBlock, facts.EvidenceSHA256 = &parsed.evidence, parsed.block, digest
	}
	history, err := readEvaluationMutationHistory(number, view.Comments)
	if err != nil {
		return capturePRFacts{}, fmt.Errorf("PR #%d evaluation history: %w", number, err)
	}
	records, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return capturePRFacts{}, fmt.Errorf("PR #%d receipts: %w", number, err)
	}
	for _, record := range records {
		if record.receipt.PR != number {
			return capturePRFacts{}, fmt.Errorf("receipt comment %d belongs to PR #%d", record.comment.ID, record.receipt.PR)
		}
		if record.receipt.Head == view.HeadRefOID {
			if bindErr := evaluationReceiptMatchesCurrentPR(record.receipt, view); bindErr != nil {
				return capturePRFacts{}, fmt.Errorf("receipt comment %d PR binding: %w", record.comment.ID, bindErr)
			}
			if bindErr := evaluationReceiptMatchesCurrentEvidence(record.receipt, view); bindErr != nil {
				return capturePRFacts{}, fmt.Errorf("receipt comment %d evidence binding: %w", record.comment.ID, bindErr)
			}
		}
		_, attestation, _, ok := receiptAttestation(record)
		if !ok || len(attestation) == 0 {
			return capturePRFacts{}, fmt.Errorf("receipt comment %d has invalid attestation transport", record.comment.ID)
		}
		facts.Receipts = append(facts.Receipts, captureReceipt{
			CommentID: record.comment.ID, Receipt: record.receipt, ReceiptMarker: record.marker,
			Attestation: attestation, CommentBody: []byte(record.comment.Body),
		})
	}
	latest, err := a.readPullRequestForResume(root, number)
	if err != nil {
		return capturePRFacts{}, fmt.Errorf("re-read exact PR: %w", err)
	}
	if latest.BaseRefOID != view.BaseRefOID || latest.HeadRefOID != view.HeadRefOID ||
		latest.BaseRefName != view.BaseRefName || latest.HeadRefName != view.HeadRefName ||
		latest.URL != view.URL || latest.Body != view.Body || !sameCaptureComments(latest.Comments, view.Comments) {
		return capturePRFacts{}, fmt.Errorf("PR #%d source changed during capture", number)
	}
	return facts, nil
}

func readCaptureJSONFile(path, label string, target any) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: stat file: %w", label, err)
	}
	if info.Size() > captureLimit {
		return fmt.Errorf("%s exceeds capture bound", label)
	}
	return readStructuredJSONFile(path, label, target)
}

func sameCaptureComments(a, b []pullRequestComment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

//nolint:gocognit,funlen // Artifact names select existing shared validators without alternate schemas.
func (a app) runCaptureArtifact(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl capture artifact NAME --file FILE --base SHA --head SHA [--audit-file FILE]")
	}
	name := args[0]
	flags := flag.NewFlagSet("capture artifact", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("file", "", "artifact JSON file")
	base := flags.String("base", "", "exact REST PR base SHA")
	head := flags.String("head", "", "exact REST PR head SHA")
	auditPath := flags.String("audit-file", "", "matching documentation audit JSON for curator-result")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *path == "" ||
		!validExactCommitSHA(*base) || !validExactCommitSHA(*head) {
		return usageError("usage: workflowctl capture artifact NAME --file FILE --base SHA --head SHA [--audit-file FILE]")
	}
	envelope := captureEnvelope{}
	view := pullRequestView{BaseRefOID: *base, HeadRefOID: *head}
	switch name {
	case "development-signals":
		var report developmentSignalsReport
		if err := readCaptureJSONFile(*path, name, &report); err != nil {
			return a.writeCapture(envelope, err)
		}
		if err := validateDevelopmentSignals(report, *base, *head); err != nil {
			return a.writeCapture(envelope, fmt.Errorf("%s: %w", name, err))
		}
		envelope.Artifact = report
	case "documentation-audit":
		var report documentationAuditReport
		if err := readCaptureJSONFile(*path, name, &report); err != nil {
			return a.writeCapture(envelope, err)
		}
		if err := validateDocumentationAuditReport(report, *base, *head); err != nil {
			return a.writeCapture(envelope, fmt.Errorf("%s: %w", name, err))
		}
		envelope.Artifact = report
	case "pr-evidence":
		var report prEvidence
		if err := readCaptureJSONFile(*path, name, &report); err != nil {
			return a.writeCapture(envelope, err)
		}
		if err := validatePREvidence(report, view); err != nil {
			return a.writeCapture(envelope, fmt.Errorf("%s: %w", name, err))
		}
		envelope.Artifact = report
	case "curator-result":
		if *auditPath == "" {
			return usageError("capture artifact curator-result requires --audit-file")
		}
		var audit documentationAuditReport
		if err := readCaptureJSONFile(*auditPath, "documentation audit", &audit); err != nil {
			return a.writeCapture(envelope, err)
		}
		if err := validateDocumentationAuditReport(audit, *base, *head); err != nil {
			return a.writeCapture(envelope, fmt.Errorf("documentation audit: %w", err))
		}
		var report curatorResult
		if err := readCaptureJSONFile(*path, name, &report); err != nil {
			return a.writeCapture(envelope, err)
		}
		if err := validateCuratorResult(report, audit, *head); err != nil {
			return a.writeCapture(envelope, fmt.Errorf("curator-result: %w", err))
		}
		envelope.Artifact = report
	default:
		return usageError("unknown capture artifact %q", name)
	}
	return a.writeCapture(envelope, nil)
}

func (a app) runCaptureGitObject(args []string) error {
	flags := flag.NewFlagSet("capture git-object", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	commit := flags.String("commit", "", "exact commit SHA")
	path := flags.String("path", "", "tracked path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !validExactCommitSHA(*commit) ||
		validateGitPath(*path) != nil {
		return usageError("usage: workflowctl capture git-object --commit SHA --path PATH")
	}
	envelope := captureEnvelope{}
	root, err := a.root()
	if err != nil {
		return a.writeCapture(envelope, err)
	}
	object, err := a.captureGitObject(root, *commit, *path)
	if err != nil {
		return a.writeCapture(envelope, err)
	}
	envelope.GitObject = &object
	return a.writeCapture(envelope, nil)
}

//nolint:gocognit // Tree mode and object-size checks precede the one blob read.
func (a app) captureGitObject(root, commit, path string) (captureGitObject, error) {
	listing, err := a.gitRaw(root, "ls-tree", "-z", commit, "--", path)
	if err != nil {
		return captureGitObject{}, fmt.Errorf("list %s at %s: %w", path, commit, err)
	}
	if !strings.HasSuffix(listing, "\x00") || strings.Count(listing, "\x00") != 1 {
		return captureGitObject{}, fmt.Errorf("tracked path %q has no unique tree entry", path)
	}
	entry := strings.TrimSuffix(listing, "\x00")
	meta, listedPath, found := strings.Cut(entry, "\t")
	fields := strings.Fields(meta)
	if !found || listedPath != path || len(fields) != 3 || !validExactCommitSHA(fields[2]) {
		return captureGitObject{}, fmt.Errorf("tracked path %q has malformed tree entry", path)
	}
	result := captureGitObject{Commit: commit, Path: path, Mode: fields[0], OID: fields[2]}
	if fields[0] == "160000" && fields[1] == "commit" {
		result.Kind = "gitlink"
		return result, nil
	}
	if (fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000") || fields[1] != "blob" {
		return captureGitObject{}, fmt.Errorf("tracked path %q has unsupported mode/type %s/%s", path, fields[0], fields[1])
	}
	sizeText, err := a.command(root, "git", "cat-file", "-s", fields[2])
	if err != nil {
		return captureGitObject{}, fmt.Errorf("size tracked blob %s: %w", fields[2], err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 10, 64)
	if err != nil || size < 0 || size > captureLimit {
		return captureGitObject{}, fmt.Errorf("tracked blob %s has invalid or excessive size %q", fields[2], sizeText)
	}
	content, err := a.gitRaw(root, "cat-file", "blob", fields[2])
	if err != nil {
		return captureGitObject{}, fmt.Errorf("read tracked blob %s: %w", fields[2], err)
	}
	if int64(len(content)) != size {
		return captureGitObject{}, fmt.Errorf("tracked blob %s size changed: got %d, want %d", fields[2], len(content), size)
	}
	result.Kind, result.Bytes = "blob", []byte(content)
	verify, err := a.gitRaw(root, "ls-tree", "-z", commit, "--", path)
	if err != nil {
		return captureGitObject{}, fmt.Errorf("re-read tracked path %q: %w", path, err)
	}
	if !bytes.Equal([]byte(verify), []byte(listing)) {
		return captureGitObject{}, fmt.Errorf("tracked path %q changed during capture", path)
	}
	return result, nil
}

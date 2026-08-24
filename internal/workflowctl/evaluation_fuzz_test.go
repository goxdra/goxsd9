package workflowctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	evaluationEvidenceTimestampLayout = "2006-01-02T15:04:05Z"
	evaluationEvidenceMaxFrameSize    = 16 << 10
	evaluationEvidenceHeaderSize      = 4 + 1 + 1 + 1 + len(evaluationEvidenceTimestampLayout)
	evaluationEvidenceEntryHeader     = 1 + 5
	evaluationEvidenceFrameMagic      = "WFE!"
	evaluationEvidenceFrameVersion    = byte('1')
)

// The scenario byte is opaque corpus metadata and never supplies expected behavior.
const evaluationEvidenceScenarioCount = 10

type evaluationEvidenceReservedSequence struct {
	value           string
	receiptEvidence bool
}

// These literals intentionally mirror the production wire format without
// sharing its validation implementation or sequence table.
var evaluationEvidenceReservedSequences = [...]evaluationEvidenceReservedSequence{
	{value: "<!-- workflowctl-evaluation ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-attestation-base64 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-attestation ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-report-base64-v1 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-repair-v1 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-challenge ", receiptEvidence: false},
	{value: "<!-- workflowctl-evaluation-resolution-v1 ", receiptEvidence: true},
	{value: "## Examiner evaluation — round receipt\n\n", receiptEvidence: true},
	{value: "## Examiner evaluation — no-verdict resolution\n\n", receiptEvidence: true},
}

const (
	evaluationEvidenceReceiptHeading = "## Examiner evaluation — round receipt\n\n"
	evaluationEvidenceRepairHeading  = "## Examiner evaluation transport repair\n\n"
)

type evaluationEvidenceFrame struct {
	timestamp  time.Time
	authorTags []byte
	bodies     [][]byte
}

type evaluationEvidenceClassification byte

const (
	evaluationEvidenceFrameRejected evaluationEvidenceClassification = iota
	evaluationEvidenceMarkerRejected
	evaluationEvidenceAttestationRejected
	evaluationEvidenceHistoryRejected
	evaluationEvidenceLatestRejected
	evaluationEvidenceUnauthorized
	evaluationEvidenceAuthorized
	evaluationEvidencePanicked
)

type evaluationEvidenceResult struct {
	classification evaluationEvidenceClassification
	authorized     bool
	recovered      [][]byte
	panicked       bool
}

type evaluationEvidenceExpectation struct {
	classification evaluationEvidenceClassification
	authorized     bool
	recovered      [][]byte
}

type evaluationEvidenceSemantics struct {
	expectation evaluationEvidenceExpectation
}

type evaluationEvidenceClaimProofFixture struct {
	Issue  int    `json:"issue"`
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

type evaluationEvidenceReceiptFixture struct {
	AttestationSHA256 string                                `json:"attestationSHA256,omitempty"`
	BaseRefName       string                                `json:"baseRefName,omitempty"`
	Challenge         string                                `json:"challenge,omitempty"`
	ClaimProofs       []evaluationEvidenceClaimProofFixture `json:"claimProofs,omitempty"`
	ClosingIssues     []int                                 `json:"closingIssues,omitempty"`
	Evaluator         string                                `json:"evaluator"`
	EvaluatorRunID    string                                `json:"evaluatorRunID,omitempty"`
	Head              string                                `json:"head"`
	HeadRefName       string                                `json:"headRefName,omitempty"`
	BodySHA256        string                                `json:"bodySHA256,omitempty"`
	PR                int                                   `json:"pullRequest,omitempty"`
	RecordedAt        time.Time                             `json:"recordedAt"`
	ReportSHA256      string                                `json:"reportSHA256"`
	ReportTransport   string                                `json:"reportTransport,omitempty"`
	Round             int                                   `json:"round"`
	Verdict           string                                `json:"verdict"`
}

type evaluationEvidenceReceiptFormat byte

const (
	evaluationEvidenceLegacyReceipt evaluationEvidenceReceiptFormat = iota
	evaluationEvidenceCurrentReceipt
)

type evaluationEvidenceRepairFixture struct {
	AttestationSHA256     string `json:"attestationSHA256"`
	Challenge             string `json:"challenge"`
	Evaluator             string `json:"evaluator"`
	EvaluatorRunID        string `json:"evaluatorRunID"`
	Head                  string `json:"head"`
	OriginalCommentSHA256 string `json:"originalCommentSHA256"`
	ReceiptMarkerSHA256   string `json:"receiptMarkerSHA256"`
	PR                    int    `json:"pullRequest"`
	ReportSHA256          string `json:"reportSHA256"`
	Round                 int    `json:"round"`
	Schema                string `json:"schema"`
	Verdict               string `json:"verdict"`
}

type evaluationEvidenceChallengeFixture struct {
	Challenge   string    `json:"challenge"`
	Head        string    `json:"head"`
	PR          int       `json:"pullRequest"`
	RequestedAt time.Time `json:"requestedAt"`
}

type evaluationEvidenceFindingFixture struct {
	Impact             string `json:"impact"`
	Location           string `json:"location"`
	RequiredCorrection string `json:"requiredCorrection"`
}

type evaluationEvidenceAttestationFixture struct {
	Challenge string                             `json:"challenge"`
	Evaluator string                             `json:"evaluator"`
	Findings  []evaluationEvidenceFindingFixture `json:"findings"`
	Head      string                             `json:"head"`
	PR        int                                `json:"pullRequest"`
	RunID     string                             `json:"runID"`
	Schema    string                             `json:"schema"`
	Summary   string                             `json:"summary"`
	Verdict   string                             `json:"verdict"`
}

type evaluationEvidenceChallengeRecord struct {
	comment      pullRequestComment
	commentIndex int
	challenge    evaluationEvidenceChallengeFixture
}

type evaluationEvidenceRepairRecord struct {
	comment      pullRequestComment
	commentIndex int
	repair       evaluationEvidenceRepairFixture
}

type evaluationEvidenceReceipt struct {
	commentIndex int
	commentTime  time.Time
	commentBody  string
	marker       []byte
	format       evaluationEvidenceReceiptFormat
	receipt      evaluationEvidenceReceiptFixture
}

type evaluationEvidenceCommentSemantics struct {
	challenge      *evaluationEvidenceChallengeRecord
	receipt        *evaluationEvidenceReceipt
	repair         *evaluationEvidenceRepairRecord
	classification evaluationEvidenceClassification
}

func FuzzEvaluationEvidence(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		expected, hasExpected := expectedEvaluationEvidence(input)
		first := runEvaluationEvidence(input)
		second := runEvaluationEvidence(input)
		if first.classification != second.classification || first.authorized != second.authorized ||
			!evaluationEvidenceRawEqual(first.recovered, second.recovered) {
			t.Fatal("evaluation evidence classification, authorization, or raw recovery was nondeterministic")
		}
		if hasExpected {
			if first.classification != expected.classification || first.authorized != expected.authorized {
				t.Fatalf("evaluation evidence scenario classified as %d, authorized=%t; want %d, authorized=%t",
					first.classification, first.authorized, expected.classification, expected.authorized)
			}
			if !evaluationEvidenceRawEqual(first.recovered, expected.recovered) {
				t.Fatalf("evaluation evidence recovered raw attestations %#v; want %#v",
					first.recovered, expected.recovered)
			}
		}
		if first.panicked || second.panicked {
			t.Fatal("evaluation evidence boundary panicked")
		}
	})
}

func expectedEvaluationEvidence(input []byte) (evaluationEvidenceExpectation, bool) {
	frame, ok := decodeEvaluationEvidenceFrame(input)
	if !ok {
		_, _, headerOK := decodeEvaluationEvidenceHeader(input)
		if !headerOK {
			return evaluationEvidenceExpectation{}, false
		}
		return evaluationEvidenceExpectation{classification: evaluationEvidenceFrameRejected}, true
	}
	semantics := independentlyValidateEvaluationEvidence(frame)
	return semantics.expectation, true
}

func independentlyValidateEvaluationEvidence(frame evaluationEvidenceFrame) evaluationEvidenceSemantics {
	challenges, receipts, repairs, classification := expectedEvaluationEvidenceRecords(frame)
	if classification != 0 {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: classification},
		}
	}

	recovered, recoveryValid := expectedEvaluationEvidenceRecovery(receipts)
	if !recoveryValid {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{
				classification: evaluationEvidenceAttestationRejected,
				recovered:      recovered,
			},
		}
	}

	if !expectedEvaluationEvidenceHistoryValid(challenges, receipts, repairs) {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{
				classification: evaluationEvidenceHistoryRejected,
				recovered:      recovered,
			},
		}
	}

	if len(receipts) == 0 {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: evaluationEvidenceUnauthorized},
		}
	}

	latest := receipts[len(receipts)-1].receipt
	if latest.AttestationSHA256 == "" || latest.Head != "head" || latest.PR != 47 || latest.Verdict != "pass" {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{
				classification: evaluationEvidenceUnauthorized,
				recovered:      recovered,
			},
		}
	}
	if expectedEvaluationEvidenceReceiptHasMetadata(latest) {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{
				classification: evaluationEvidenceLatestRejected,
				recovered:      recovered,
			},
		}
	}
	return evaluationEvidenceSemantics{
		expectation: evaluationEvidenceExpectation{
			classification: evaluationEvidenceAuthorized,
			authorized:     true,
			recovered:      recovered,
		},
	}
}

func expectedEvaluationEvidenceRecords(frame evaluationEvidenceFrame) (
	[]evaluationEvidenceChallengeRecord, []evaluationEvidenceReceipt, []evaluationEvidenceRepairRecord,
	evaluationEvidenceClassification) {
	var challenges []evaluationEvidenceChallengeRecord
	var receipts []evaluationEvidenceReceipt
	var repairs []evaluationEvidenceRepairRecord
	for index := range frame.bodies {
		comment := expectedEvaluationEvidenceCommentSemantics(frame, index)
		if comment.classification != 0 {
			return challenges, receipts, repairs, comment.classification
		}
		if comment.challenge != nil {
			challenges = append(challenges, *comment.challenge)
		}
		if comment.receipt != nil {
			receipts = append(receipts, *comment.receipt)
		}
		if comment.repair != nil {
			repairs = append(repairs, *comment.repair)
		}
	}
	return challenges, receipts, repairs, 0
}

func expectedEvaluationEvidenceCommentSemantics(frame evaluationEvidenceFrame, index int) evaluationEvidenceCommentSemantics {
	body := frame.bodies[index]
	if frame.authorTags[index] != 'T' {
		return expectedEvaluationEvidenceUntrustedCommentSemantics(body)
	}
	if expectedEvaluationEvidenceHasMarker(body, evaluationChallengeMarker) {
		return expectedEvaluationEvidenceChallengeSemantics(frame, index)
	}
	if expectedEvaluationEvidenceHasMarker(body, evaluationRepairMarker) {
		return expectedEvaluationEvidenceRepairSemantics(frame, index)
	}
	return expectedEvaluationEvidenceTrustedCommentSemantics(frame, index)
}

func expectedEvaluationEvidenceUntrustedCommentSemantics(body []byte) evaluationEvidenceCommentSemantics {
	if expectedEvaluationEvidenceHasMarker(body, evaluationChallengeMarker) {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	return evaluationEvidenceCommentSemantics{}
}

func expectedEvaluationEvidenceChallengeSemantics(frame evaluationEvidenceFrame, index int) evaluationEvidenceCommentSemantics {
	body := frame.bodies[index]
	if expectedEvaluationEvidenceHasReceiptEvidence(body) {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	challenge, ok := expectedEvaluationEvidenceChallenge(body)
	if !ok {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	return evaluationEvidenceCommentSemantics{
		challenge: &evaluationEvidenceChallengeRecord{
			comment:      evaluationEvidenceComment(frame, index),
			commentIndex: index,
			challenge:    challenge,
		},
	}
}

func expectedEvaluationEvidenceRepairSemantics(frame evaluationEvidenceFrame, index int) evaluationEvidenceCommentSemantics {
	body := frame.bodies[index]
	if expectedEvaluationEvidenceHasReceipt(body) ||
		expectedEvaluationEvidenceHasAttestationEvidence(body) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationChallengeMarker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationResolutionMarker) ||
		bytes.Contains(body, []byte(evaluationResolutionHeading)) {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	repair, ok := expectedEvaluationEvidenceRepair(body)
	if !ok {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	return evaluationEvidenceCommentSemantics{
		repair: &evaluationEvidenceRepairRecord{
			comment:      evaluationEvidenceComment(frame, index),
			commentIndex: index,
			repair:       repair,
		},
	}
}

func expectedEvaluationEvidenceTrustedCommentSemantics(frame evaluationEvidenceFrame, index int) evaluationEvidenceCommentSemantics {
	body := frame.bodies[index]
	if expectedEvaluationEvidenceHasMarker(body, evaluationResolutionMarker) ||
		bytes.Contains(body, []byte(evaluationResolutionHeading)) {
		return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
	}
	if !expectedEvaluationEvidenceHasReceipt(body) {
		if expectedEvaluationEvidenceHasAttestationEvidence(body) {
			return evaluationEvidenceCommentSemantics{classification: evaluationEvidenceMarkerRejected}
		}
		return evaluationEvidenceCommentSemantics{}
	}
	receipt, classification := expectedEvaluationEvidenceReceipt(frame, index)
	if classification != 0 {
		return evaluationEvidenceCommentSemantics{classification: classification}
	}
	return evaluationEvidenceCommentSemantics{receipt: &receipt}
}

func expectedEvaluationEvidenceHasMarker(body []byte, marker string) bool {
	return bytes.Contains(body, []byte("<!-- "+marker))
}

func expectedEvaluationEvidenceHasReceipt(body []byte) bool {
	return expectedEvaluationEvidenceHasMarker(body, evaluationMarker) ||
		bytes.Contains(body, []byte(evaluationEvidenceReceiptHeading))
}

func expectedEvaluationEvidenceHasReceiptEvidence(body []byte) bool {
	for _, sequence := range evaluationEvidenceReservedSequences {
		if !sequence.receiptEvidence {
			continue
		}
		if bytes.Contains(body, []byte(sequence.value)) {
			return true
		}
	}
	return false
}

func expectedEvaluationEvidenceHasAttestationEvidence(body []byte) bool {
	return expectedEvaluationEvidenceHasMarker(body, evaluationReportBase64Marker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationAttestationBase64Marker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationAttestationMarker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationResolutionMarker) ||
		bytes.Contains(body, []byte(evaluationResolutionHeading))
}

func expectedEvaluationEvidenceHasRawAttestation(body []byte) bool {
	return expectedEvaluationEvidenceHasMarker(body, evaluationAttestationBase64Marker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationAttestationMarker)
}

func evaluationEvidenceComment(frame evaluationEvidenceFrame, index int) pullRequestComment {
	comment := pullRequestComment{
		Body:      string(frame.bodies[index]),
		CreatedAt: frame.timestamp.Add(time.Duration(index) * time.Minute),
	}
	comment.Author.Login = evaluationEvidenceAuthor(frame.authorTags[index])
	return comment
}

func expectedEvaluationEvidenceChallenge(body []byte) (evaluationEvidenceChallengeFixture, bool) {
	value, found, valid := expectedEvaluationEvidenceMarker(body, evaluationChallengeMarker)
	if !valid || !found {
		return evaluationEvidenceChallengeFixture{}, false
	}
	var challenge evaluationEvidenceChallengeFixture
	if !expectedEvaluationEvidenceJSON(value, &challenge,
		[]string{"challenge", "head", "pullRequest", "requestedAt"}, nil) {
		return evaluationEvidenceChallengeFixture{}, false
	}
	if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
		return evaluationEvidenceChallengeFixture{}, false
	}
	return challenge, true
}

func expectedEvaluationEvidenceReceipt(frame evaluationEvidenceFrame, index int) (
	evaluationEvidenceReceipt, evaluationEvidenceClassification) {
	body := frame.bodies[index]
	receiptMarker, found, valid := expectedEvaluationEvidenceMarker(body, evaluationMarker)
	if !valid || !found {
		return evaluationEvidenceReceipt{}, evaluationEvidenceMarkerRejected
	}
	var receipt evaluationEvidenceReceiptFixture
	if !expectedEvaluationEvidenceJSON(receiptMarker, &receipt, []string{
		"attestationSHA256", "baseRefName", "challenge", "claimProofs", "closingIssues", "evaluator",
		"evaluatorRunID", "head", "headRefName", "bodySHA256", "pullRequest", "recordedAt", "reportSHA256",
		"reportTransport", "round", "verdict",
	}, nil) || !expectedEvaluationEvidenceReceiptFieldsValid(receipt) {
		return evaluationEvidenceReceipt{}, evaluationEvidenceMarkerRejected
	}
	return evaluationEvidenceReceipt{
		commentIndex: index,
		commentTime:  frame.timestamp.Add(time.Duration(index) * time.Minute),
		commentBody:  string(body),
		marker:       append([]byte(nil), receiptMarker...),
		format:       expectedEvaluationEvidenceReceiptFormat(receipt),
		receipt:      receipt,
	}, 0
}

func expectedEvaluationEvidenceReceiptFormat(receipt evaluationEvidenceReceiptFixture) evaluationEvidenceReceiptFormat {
	if receipt.ReportTransport == "base64-v1" {
		return evaluationEvidenceCurrentReceipt
	}
	return evaluationEvidenceLegacyReceipt
}

func expectedEvaluationEvidenceReceiptFieldsValid(receipt evaluationEvidenceReceiptFixture) bool {
	if receipt.Evaluator != "Examiner" || receipt.Round < 1 || receipt.RecordedAt.IsZero() ||
		(receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" ||
		!expectedEvaluationEvidenceValidSHA256(receipt.ReportSHA256) {
		return false
	}
	if receipt.ReportTransport != "" && receipt.ReportTransport != "base64-v1" {
		return false
	}
	if receipt.AttestationSHA256 == "" {
		return expectedEvaluationEvidenceReceiptMetadataValid(receipt)
	}
	return expectedEvaluationEvidenceValidSHA256(receipt.AttestationSHA256) && receipt.Challenge != "" &&
		receipt.EvaluatorRunID != "" && receipt.PR >= 1 && expectedEvaluationEvidenceReceiptMetadataValid(receipt)
}

func expectedEvaluationEvidenceReceiptHasMetadata(receipt evaluationEvidenceReceiptFixture) bool {
	return receipt.BaseRefName != "" || len(receipt.ClosingIssues) != 0 || receipt.HeadRefName != "" ||
		receipt.BodySHA256 != "" || receipt.ClaimProofs != nil
}

func expectedEvaluationEvidenceReceiptMetadataValid(receipt evaluationEvidenceReceiptFixture) bool {
	if !expectedEvaluationEvidenceReceiptHasMetadata(receipt) {
		return true
	}
	if receipt.BaseRefName == "" || receipt.HeadRefName == "" ||
		!expectedEvaluationEvidenceValidSHA256(receipt.BodySHA256) || len(receipt.ClosingIssues) == 0 {
		return false
	}
	if !expectedEvaluationEvidenceIssueListValid(receipt.ClosingIssues) {
		return false
	}
	if receipt.ClaimProofs == nil {
		return len(receipt.ClosingIssues) == 1
	}
	return expectedEvaluationEvidenceClaimProofsValid(receipt.ClosingIssues, receipt.ClaimProofs)
}

func expectedEvaluationEvidenceIssueListValid(issues []int) bool {
	for index, issue := range issues {
		if issue < 1 {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if issues[prior] == issue {
				return false
			}
		}
	}
	return true
}

func expectedEvaluationEvidenceClaimProofsValid(issues []int,
	proofs []evaluationEvidenceClaimProofFixture) bool {
	if len(proofs) != len(issues) {
		return false
	}
	if !expectedEvaluationEvidenceClaimProofValuesValid(proofs) {
		return false
	}
	return expectedEvaluationEvidenceClaimProofsCoverIssues(issues, proofs)
}

func expectedEvaluationEvidenceClaimProofValuesValid(proofs []evaluationEvidenceClaimProofFixture) bool {
	for index, proof := range proofs {
		if proof.Issue < 1 || proof.Branch == "" || proof.SHA == "" {
			return false
		}
		branchIssue, ok := expectedEvaluationEvidenceIssueFromBranch(proof.Branch)
		if !ok || branchIssue != proof.Issue {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if proofs[prior].Issue == proof.Issue {
				return false
			}
		}
	}
	return true
}

func expectedEvaluationEvidenceClaimProofsCoverIssues(issues []int,
	proofs []evaluationEvidenceClaimProofFixture) bool {
	for _, issue := range issues {
		found := false
		for _, proof := range proofs {
			if proof.Issue == issue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceIssueFromBranch(branch string) (int, bool) {
	value := strings.TrimPrefix(branch, "agent/issue-")
	if value == branch || value == "" {
		return 0, false
	}
	if separator := strings.IndexByte(value, '-'); separator >= 0 {
		value = value[:separator]
	}
	issue, err := strconv.Atoi(value)
	return issue, err == nil && issue > 0
}

func expectedEvaluationEvidenceValidatedAttestation(body []byte) (
	evaluationEvidenceAttestationFixture, []byte, bool) {
	raw, found, valid := expectedEvaluationEvidenceAttestation(body)
	if !valid || !found {
		return evaluationEvidenceAttestationFixture{}, nil, false
	}
	value := raw
	var attestation evaluationEvidenceAttestationFixture
	if !expectedEvaluationEvidenceJSON(value, &attestation, []string{
		"challenge", "evaluator", "findings", "head", "pullRequest", "runID", "schema", "summary", "verdict",
	}, []string{"location", "impact", "requiredCorrection"}) {
		return evaluationEvidenceAttestationFixture{}, nil, false
	}
	if !expectedEvaluationEvidenceAttestationFieldsValid(attestation) ||
		!expectedEvaluationEvidenceFindingsValid(attestation) ||
		!expectedEvaluationEvidenceAttestationTextValid(attestation) {
		return evaluationEvidenceAttestationFixture{}, nil, false
	}
	return attestation, raw, true
}

func expectedEvaluationEvidenceAttestationFieldsValid(attestation evaluationEvidenceAttestationFixture) bool {
	return attestation.Evaluator == "Examiner" && attestation.Challenge != "" && attestation.Head != "" &&
		attestation.PR >= 1 && attestation.RunID != "" &&
		attestation.Schema == "goxsd9/examiner-attestation/v1" &&
		strings.TrimSpace(attestation.Summary) != "" &&
		(attestation.Verdict == "pass" || attestation.Verdict == "fail") && attestation.Findings != nil
}

func expectedEvaluationEvidenceFindingsValid(attestation evaluationEvidenceAttestationFixture) bool {
	if attestation.Verdict == "pass" && len(attestation.Findings) != 0 {
		return false
	}
	if attestation.Verdict == "fail" && len(attestation.Findings) == 0 {
		return false
	}
	for _, finding := range attestation.Findings {
		if strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Impact) == "" ||
			strings.TrimSpace(finding.RequiredCorrection) == "" {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceAttestationTextValid(attestation evaluationEvidenceAttestationFixture) bool {
	fields := make([]string, 0, 1+3*len(attestation.Findings))
	fields = append(fields, attestation.Summary)
	for _, finding := range attestation.Findings {
		fields = append(fields, finding.Location, finding.Impact, finding.RequiredCorrection)
	}
	for _, field := range fields {
		for _, sequence := range evaluationEvidenceReservedSequences {
			if strings.Contains(field, sequence.value) {
				return false
			}
		}
	}
	return true
}

func expectedEvaluationEvidenceAttestationMatches(receipt evaluationEvidenceReceiptFixture,
	attestation evaluationEvidenceAttestationFixture, raw []byte) bool {
	return expectedEvaluationEvidenceSHA256(raw) == receipt.AttestationSHA256 &&
		attestation.Challenge == receipt.Challenge && attestation.Evaluator == receipt.Evaluator &&
		attestation.RunID == receipt.EvaluatorRunID && attestation.Head == receipt.Head &&
		attestation.PR == receipt.PR && attestation.Verdict == receipt.Verdict
}

func expectedEvaluationEvidenceReport(attestation evaluationEvidenceAttestationFixture) []byte {
	parts := make([]string, 0, 1+len(attestation.Findings))
	parts = append(parts, "**"+strings.ToUpper(attestation.Verdict)+"**\n\n"+strings.TrimSpace(attestation.Summary))
	for index, finding := range attestation.Findings {
		parts = append(parts, fmt.Sprintf("%d. `%s` — %s Required correction: %s", index+1,
			strings.TrimSpace(finding.Location), strings.TrimSpace(finding.Impact),
			strings.TrimSpace(finding.RequiredCorrection)))
	}
	return []byte(strings.Join(parts, "\n\n"))
}

func expectedEvaluationEvidenceRepair(body []byte) (evaluationEvidenceRepairFixture, bool) {
	value, found, valid := expectedEvaluationEvidenceMarker(body, evaluationRepairMarker)
	if !valid || !found {
		return evaluationEvidenceRepairFixture{}, false
	}
	var repair evaluationEvidenceRepairFixture
	if !expectedEvaluationEvidenceJSON(value, &repair, []string{
		"attestationSHA256", "challenge", "evaluator", "evaluatorRunID", "head", "originalCommentSHA256",
		"receiptMarkerSHA256", "pullRequest", "reportSHA256", "round", "schema", "verdict",
	}, nil) {
		return evaluationEvidenceRepairFixture{}, false
	}
	if repair.Schema != "goxsd9/examiner-evaluation-repair/v1" || repair.Evaluator != "Examiner" ||
		repair.PR < 1 || repair.Round < 1 || repair.Head == "" || repair.Challenge == "" ||
		repair.EvaluatorRunID == "" || (repair.Verdict != "pass" && repair.Verdict != "fail") ||
		!expectedEvaluationEvidenceValidSHA256(repair.AttestationSHA256) ||
		!expectedEvaluationEvidenceValidSHA256(repair.OriginalCommentSHA256) ||
		!expectedEvaluationEvidenceValidSHA256(repair.ReceiptMarkerSHA256) ||
		!expectedEvaluationEvidenceValidSHA256(repair.ReportSHA256) {
		return evaluationEvidenceRepairFixture{}, false
	}
	return repair, true
}

func expectedEvaluationEvidenceRecovery(receipts []evaluationEvidenceReceipt) ([][]byte, bool) {
	var recovered [][]byte
	for _, record := range receipts {
		if record.receipt.AttestationSHA256 == "" {
			continue
		}
		attestation, raw, ok := expectedEvaluationEvidenceValidatedAttestation([]byte(record.commentBody))
		if !ok || !expectedEvaluationEvidenceAttestationMatches(record.receipt, attestation, raw) {
			return recovered, false
		}
		recovered = append(recovered, append([]byte(nil), raw...))
	}
	return recovered, true
}

func expectedEvaluationEvidenceHistoryValid(challenges []evaluationEvidenceChallengeRecord,
	receipts []evaluationEvidenceReceipt, repairs []evaluationEvidenceRepairRecord) bool {
	if !expectedEvaluationEvidenceReceiptsValid(challenges, receipts, repairs) {
		return false
	}
	return expectedEvaluationEvidenceRepairsValid(receipts, repairs)
}

func expectedEvaluationEvidenceReceiptsValid(challenges []evaluationEvidenceChallengeRecord,
	receipts []evaluationEvidenceReceipt, repairs []evaluationEvidenceRepairRecord) bool {
	for index, record := range receipts {
		if !expectedEvaluationEvidenceReceiptIdentifiersUnique(receipts, index) ||
			!expectedEvaluationEvidenceReceiptMatchesRecord(record, repairs) {
			return false
		}
		if record.receipt.AttestationSHA256 != "" &&
			expectedEvaluationEvidenceMatchingChallenges(challenges, record) != 1 {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceRepairsValid(receipts []evaluationEvidenceReceipt,
	repairs []evaluationEvidenceRepairRecord) bool {
	for _, repair := range repairs {
		if !expectedEvaluationEvidenceRepairCommentIsValid(repair) {
			return false
		}
		matches := 0
		for _, record := range receipts {
			if expectedEvaluationEvidenceRepairMatchesRecord(record, repair) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceReceiptIdentifiersUnique(receipts []evaluationEvidenceReceipt, index int) bool {
	current := receipts[index].receipt
	for prior := 0; prior < index; prior++ {
		previous := receipts[prior].receipt
		if previous.Round == current.Round || (previous.Challenge != "" && previous.Challenge == current.Challenge) ||
			(previous.EvaluatorRunID != "" && previous.EvaluatorRunID == current.EvaluatorRunID) {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceReceiptMatchesRecord(record evaluationEvidenceReceipt,
	repairs []evaluationEvidenceRepairRecord) bool {
	if !expectedEvaluationEvidenceCommentTimeMatches(record.commentTime, record.receipt.RecordedAt) {
		return false
	}
	_, _, canonicalReport, attestationOK := expectedEvaluationEvidenceReceiptAttestation(record)
	if !attestationOK {
		return false
	}
	reportMarker, hasReportMarker, reportMarkerOK := expectedEvaluationEvidenceReportMarker(record.commentBody)
	if hasReportMarker && !reportMarkerOK {
		return false
	}
	if hasReportMarker {
		return expectedEvaluationEvidenceReportMatchesRecord(record, nil, canonicalReport, reportMarker,
			hasReportMarker, reportMarkerOK, repairs)
	}
	visibleReport, reportOK := expectedEvaluationEvidenceVisibleReport(record.commentBody)
	if !reportOK {
		return false
	}
	return expectedEvaluationEvidenceReportMatchesRecord(record, visibleReport, canonicalReport, nil,
		hasReportMarker, reportMarkerOK, repairs)
}

func expectedEvaluationEvidenceReceiptAttestation(record evaluationEvidenceReceipt) (
	evaluationEvidenceAttestationFixture, []byte, []byte, bool) {
	if record.receipt.AttestationSHA256 == "" {
		if expectedEvaluationEvidenceHasRawAttestation([]byte(record.commentBody)) {
			return evaluationEvidenceAttestationFixture{}, nil, nil, false
		}
		return evaluationEvidenceAttestationFixture{}, nil, nil, true
	}
	attestation, raw, ok := expectedEvaluationEvidenceValidatedAttestation([]byte(record.commentBody))
	if !ok || !expectedEvaluationEvidenceAttestationMatches(record.receipt, attestation, raw) {
		return evaluationEvidenceAttestationFixture{}, nil, nil, false
	}
	return attestation, raw, expectedEvaluationEvidenceReport(attestation), true
}

func expectedEvaluationEvidenceReportMatchesRecord(record evaluationEvidenceReceipt, visibleReport,
	canonicalReport, reportMarker []byte, hasReportMarker, reportMarkerOK bool,
	repairs []evaluationEvidenceRepairRecord) bool {
	if record.format == evaluationEvidenceCurrentReceipt {
		return expectedEvaluationEvidenceCurrentReportMatches(record, canonicalReport, reportMarker,
			hasReportMarker, reportMarkerOK)
	}
	return expectedEvaluationEvidenceLegacyReportMatches(record, visibleReport, canonicalReport, reportMarker,
		hasReportMarker, reportMarkerOK, repairs)
}

func expectedEvaluationEvidenceCurrentReportMatches(record evaluationEvidenceReceipt, canonicalReport,
	reportMarker []byte, hasReportMarker, reportMarkerOK bool) bool {
	if !hasReportMarker || !reportMarkerOK || expectedEvaluationEvidenceSHA256(reportMarker) != record.receipt.ReportSHA256 {
		return false
	}
	return record.receipt.AttestationSHA256 == "" || bytes.Equal(reportMarker, canonicalReport)
}

func expectedEvaluationEvidenceLegacyReportMatches(record evaluationEvidenceReceipt, visibleReport,
	canonicalReport, reportMarker []byte, hasReportMarker, reportMarkerOK bool,
	repairs []evaluationEvidenceRepairRecord) bool {
	receipt := record.receipt
	if hasReportMarker {
		if !reportMarkerOK || expectedEvaluationEvidenceSHA256(reportMarker) != receipt.ReportSHA256 {
			return false
		}
		return receipt.AttestationSHA256 == "" || bytes.Equal(reportMarker, canonicalReport)
	}
	if expectedEvaluationEvidenceSHA256(visibleReport) == receipt.ReportSHA256 {
		return receipt.AttestationSHA256 == "" || bytes.Equal(visibleReport, canonicalReport)
	}
	if receipt.AttestationSHA256 == "" || expectedEvaluationEvidenceSHA256(canonicalReport) != receipt.ReportSHA256 {
		return false
	}
	if len(repairs) == 0 {
		return false
	}
	matches := 0
	for _, repair := range repairs {
		if expectedEvaluationEvidenceRepairMatchesRecord(record, repair) {
			matches++
		}
	}
	return matches == 1
}

func expectedEvaluationEvidenceRepairCommentIsValid(record evaluationEvidenceRepairRecord) bool {
	marker, found, valid := expectedEvaluationEvidenceMarker([]byte(record.comment.Body), evaluationRepairMarker)
	if !valid || !found {
		return false
	}
	expected := fmt.Sprintf("<!-- workflowctl-evaluation-repair-v1 %s -->\n%sRound %d is superseded only for its visible report projection; the original receipt and Examiner evidence remain authoritative.\n",
		marker, evaluationEvidenceRepairHeading, record.repair.Round)
	return record.comment.Body == expected
}

func expectedEvaluationEvidenceRepairMatchesRecord(record evaluationEvidenceReceipt,
	repair evaluationEvidenceRepairRecord) bool {
	repairValue := repair.repair
	if record.format != evaluationEvidenceLegacyReceipt || record.receipt.AttestationSHA256 == "" ||
		expectedEvaluationEvidenceHasMarker([]byte(record.commentBody), evaluationReportBase64Marker) {
		return false
	}
	if !expectedEvaluationEvidenceRepairFollowsReceipt(record, repair) {
		return false
	}
	attestation, rawAttestation, canonicalReport, ok := expectedEvaluationEvidenceReceiptAttestation(record)
	if !ok {
		return false
	}
	visibleReport, reportOK := expectedEvaluationEvidenceVisibleReport(record.commentBody)
	if !reportOK || bytes.Equal(visibleReport, canonicalReport) ||
		expectedEvaluationEvidenceSHA256(canonicalReport) != record.receipt.ReportSHA256 {
		return false
	}
	receipt := record.receipt
	return repairValue.AttestationSHA256 == expectedEvaluationEvidenceSHA256(rawAttestation) &&
		repairValue.Challenge == receipt.Challenge && repairValue.Evaluator == receipt.Evaluator &&
		repairValue.EvaluatorRunID == receipt.EvaluatorRunID && repairValue.Head == receipt.Head &&
		repairValue.PR == receipt.PR && repairValue.ReportSHA256 == expectedEvaluationEvidenceSHA256(canonicalReport) &&
		repairValue.Round == receipt.Round && repairValue.Verdict == receipt.Verdict &&
		repairValue.OriginalCommentSHA256 == expectedEvaluationEvidenceSHA256([]byte(record.commentBody)) &&
		repairValue.ReceiptMarkerSHA256 == expectedEvaluationEvidenceSHA256(record.marker) &&
		attestation.PR == repairValue.PR && attestation.Head == repairValue.Head
}

func expectedEvaluationEvidenceRepairFollowsReceipt(record evaluationEvidenceReceipt,
	repair evaluationEvidenceRepairRecord) bool {
	if repair.commentIndex <= record.commentIndex {
		if repair.commentIndex != 0 || record.commentIndex != 0 {
			return false
		}
		return repair.comment.CreatedAt.After(record.commentTime)
	}
	return !repair.comment.CreatedAt.Before(record.commentTime)
}

func expectedEvaluationEvidenceReportMarker(body string) ([]byte, bool, bool) {
	if !expectedEvaluationEvidenceHasMarker([]byte(body), evaluationReportBase64Marker) {
		return nil, false, false
	}
	value, found, valid := expectedEvaluationEvidenceMarker([]byte(body), evaluationReportBase64Marker)
	if !found || !valid {
		return nil, true, false
	}
	report, ok := decodeEvaluationEvidenceBase64(value)
	if !ok {
		return nil, true, false
	}
	return report, true, true
}

func expectedEvaluationEvidenceVisibleReport(body string) ([]byte, bool) {
	_, report, found := strings.Cut(body, evaluationEvidenceReceiptHeading)
	if !found {
		return nil, false
	}
	report = strings.TrimSpace(report)
	if report == "" {
		return nil, false
	}
	return []byte(report), true
}

func expectedEvaluationEvidenceMatchingChallenges(challenges []evaluationEvidenceChallengeRecord,
	receipt evaluationEvidenceReceipt) int {
	matches := 0
	for _, challenge := range challenges {
		if challenge.challenge.Challenge != receipt.receipt.Challenge ||
			challenge.challenge.PR != receipt.receipt.PR || challenge.challenge.Head != receipt.receipt.Head ||
			challenge.commentIndex >= receipt.commentIndex ||
			challenge.comment.CreatedAt.After(receipt.receipt.RecordedAt) ||
			!expectedEvaluationEvidenceCommentTimeMatches(challenge.comment.CreatedAt, challenge.challenge.RequestedAt) ||
			challenge.challenge.RequestedAt.After(receipt.receipt.RecordedAt) ||
			!receipt.receipt.RecordedAt.Before(challenge.challenge.RequestedAt.Add(2*time.Hour)) {
			continue
		}
		matches++
	}
	return matches
}

func expectedEvaluationEvidenceCommentTimeMatches(commentTime, markerTime time.Time) bool {
	if commentTime.IsZero() || markerTime.IsZero() {
		return false
	}
	difference := commentTime.Sub(markerTime)
	return difference >= -5*time.Minute && difference <= 5*time.Minute
}

func expectedEvaluationEvidenceValidSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func expectedEvaluationEvidenceSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func expectedEvaluationEvidenceJSON(data []byte, target any, fields, findingFields []string) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if !expectedEvaluationEvidenceJSONObject(decoder, fields, findingFields) {
		return false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

func expectedEvaluationEvidenceJSONObject(decoder *json.Decoder, fields, findingFields []string) bool {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	var keys []string
	for decoder.More() {
		key, ok := expectedEvaluationEvidenceJSONKey(decoder, fields)
		if !ok || !expectedEvaluationEvidenceJSONKeyUnique(keys, key) {
			return false
		}
		keys = append(keys, key)
		if !expectedEvaluationEvidenceJSONObjectField(decoder, key, findingFields) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim('}'))
}

func expectedEvaluationEvidenceJSONArray(decoder *json.Decoder, fields []string) bool {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return false
	}
	for decoder.More() {
		if !expectedEvaluationEvidenceJSONObject(decoder, fields, nil) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim(']'))
}

func expectedEvaluationEvidenceNullableJSONArray(decoder *json.Decoder, fields []string) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	if token == nil {
		return true
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != json.Delim('[') {
		return false
	}
	for decoder.More() {
		if !expectedEvaluationEvidenceJSONObject(decoder, fields, nil) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim(']'))
}

func expectedEvaluationEvidenceJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		return expectedEvaluationEvidenceJSONObjectValue(decoder)
	case '[':
		return expectedEvaluationEvidenceJSONArrayValue(decoder)
	default:
		return false
	}
}

func expectedEvaluationEvidenceJSONKey(decoder *json.Decoder, fields []string) (string, bool) {
	token, err := decoder.Token()
	key, ok := token.(string)
	if err != nil || !ok || !expectedEvaluationEvidenceKeyAllowed(key, fields) {
		return "", false
	}
	return key, true
}

func expectedEvaluationEvidenceJSONKeyUnique(keys []string, key string) bool {
	for _, seen := range keys {
		if strings.EqualFold(seen, key) {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceJSONObjectField(decoder *json.Decoder, key string, findingFields []string) bool {
	if key == "findings" && findingFields != nil {
		return expectedEvaluationEvidenceJSONArray(decoder, findingFields)
	}
	if key == "claimProofs" {
		return expectedEvaluationEvidenceNullableJSONArray(decoder, []string{"issue", "branch", "sha"})
	}
	return expectedEvaluationEvidenceJSONValue(decoder)
}

func expectedEvaluationEvidenceJSONObjectValue(decoder *json.Decoder) bool {
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		if _, ok := key.(string); !ok || !expectedEvaluationEvidenceJSONValue(decoder) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim('}'))
}

func expectedEvaluationEvidenceJSONArrayValue(decoder *json.Decoder) bool {
	for decoder.More() {
		if !expectedEvaluationEvidenceJSONValue(decoder) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim(']'))
}

func expectedEvaluationEvidenceJSONClosing(decoder *json.Decoder, want json.Delim) bool {
	closing, err := decoder.Token()
	return err == nil && closing == want
}

func expectedEvaluationEvidenceKeyAllowed(key string, fields []string) bool {
	for _, field := range fields {
		if key == field {
			return true
		}
	}
	return false
}

func decodeEvaluationEvidenceBase64(value []byte) ([]byte, bool) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(value) {
		return nil, false
	}
	return decoded, true
}

func expectedEvaluationEvidenceAttestation(body []byte) ([]byte, bool, bool) {
	base64Value, hasBase64, valid := expectedEvaluationEvidenceMarker(body, evaluationAttestationBase64Marker)
	if !valid {
		return nil, false, false
	}
	plainValue, hasPlain, valid := expectedEvaluationEvidenceMarker(body, evaluationAttestationMarker)
	if !valid || hasBase64 == hasPlain {
		return nil, false, false
	}
	if hasPlain {
		return append([]byte(nil), plainValue...), true, true
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(base64Value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(base64Value) {
		return nil, false, false
	}
	return decoded, true, true
}

func expectedEvaluationEvidenceMarker(body []byte, marker string) ([]byte, bool, bool) {
	prefix := []byte("<!-- " + marker)
	start := bytes.Index(body, prefix)
	if start < 0 {
		return nil, false, true
	}
	valueStart := start + len(prefix)
	closing := []byte(" -->")
	valueEnd := bytes.Index(body[valueStart:], closing)
	if valueEnd < 0 {
		return nil, true, false
	}
	valueEnd += valueStart
	if bytes.Contains(body[valueEnd+len(closing):], prefix) {
		return nil, true, false
	}
	return body[valueStart:valueEnd], true, true
}

func runEvaluationEvidence(input []byte) (result evaluationEvidenceResult) {
	defer func() {
		if recover() != nil {
			result = evaluationEvidenceResult{
				classification: evaluationEvidencePanicked,
				panicked:       true,
			}
		}
	}()
	return runEvaluationEvidencePhases(input)
}

func runEvaluationEvidencePhases(input []byte) evaluationEvidenceResult {
	frame, ok := decodeEvaluationEvidenceFrame(input)
	if !ok {
		return evaluationEvidenceResult{classification: evaluationEvidenceFrameRejected}
	}
	comments := evaluationEvidenceComments(frame)
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return evaluationEvidenceResult{classification: evaluationEvidenceMarkerRejected}
	}
	return evaluateEvaluationEvidenceHistory(comments, history)
}

func evaluateEvaluationEvidenceHistory(comments []pullRequestComment, history evaluationHistory) evaluationEvidenceResult {
	recovered, ok := recoverEvaluationEvidenceAttestations(history)
	if !ok {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceAttestationRejected,
			recovered:      recovered,
		}
	}
	if historyErr := validateEvaluationHistory(history); historyErr != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceHistoryRejected,
			recovered:      recovered,
		}
	}
	return evaluateEvaluationEvidenceLatest(comments, recovered)
}

func recoverEvaluationEvidenceAttestations(history evaluationHistory) ([][]byte, bool) {
	var recovered [][]byte
	for _, record := range history.receipts {
		if record.receipt.AttestationSHA256 == "" {
			continue
		}
		_, raw, ok := parseCommentAttestation(record.comment.Body)
		if !ok {
			return recovered, false
		}
		_, checkedRaw, _, ok := receiptAttestation(record)
		if !ok || !bytes.Equal(raw, checkedRaw) {
			return recovered, false
		}
		recovered = append(recovered, append([]byte(nil), raw...))
	}
	return recovered, true
}

func evaluateEvaluationEvidenceLatest(comments []pullRequestComment, recovered [][]byte) evaluationEvidenceResult {
	passes, err := latestEvaluationPasses(pullRequestView{
		Comments:   comments,
		HeadRefOID: "head",
	}, 47)
	if err != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceLatestRejected,
			recovered:      recovered,
		}
	}
	if !passes {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceUnauthorized,
			recovered:      recovered,
		}
	}
	return evaluationEvidenceResult{
		classification: evaluationEvidenceAuthorized,
		authorized:     true,
		recovered:      recovered,
	}
}

func decodeEvaluationEvidenceFrame(input []byte) (evaluationEvidenceFrame, bool) {
	commentCount, timestamp, ok := decodeEvaluationEvidenceHeader(input)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	authorTags, bodies, ok := decodeEvaluationEvidenceEntries(input, commentCount)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	return evaluationEvidenceFrame{
		timestamp:  timestamp,
		authorTags: authorTags,
		bodies:     bodies,
	}, true
}

func decodeEvaluationEvidenceHeader(input []byte) (int, time.Time, bool) {
	if len(input) > evaluationEvidenceMaxFrameSize || len(input) < evaluationEvidenceHeaderSize {
		return 0, time.Time{}, false
	}
	if string(input[:len(evaluationEvidenceFrameMagic)]) != evaluationEvidenceFrameMagic ||
		input[4] != evaluationEvidenceFrameVersion {
		return 0, time.Time{}, false
	}
	if input[5] < '0' || input[5] >= '0'+byte(evaluationEvidenceScenarioCount) {
		return 0, time.Time{}, false
	}
	if input[6] < '0' || input[6] > '8' {
		return 0, time.Time{}, false
	}
	timestamp, err := time.Parse(evaluationEvidenceTimestampLayout,
		string(input[7:7+len(evaluationEvidenceTimestampLayout)]))
	if err != nil {
		return 0, time.Time{}, false
	}
	return int(input[6] - '0'), timestamp.UTC(), true
}

func decodeEvaluationEvidenceEntries(input []byte, commentCount int) ([]byte, [][]byte, bool) {
	authorTags := make([]byte, 0, commentCount)
	bodies := make([][]byte, 0, commentCount)
	offset := evaluationEvidenceHeaderSize
	for index := 0; index < commentCount; index++ {
		authorTag, body, nextOffset, ok := decodeEvaluationEvidenceEntry(input, offset)
		if !ok {
			return nil, nil, false
		}
		authorTags = append(authorTags, authorTag)
		bodies = append(bodies, body)
		offset = nextOffset
	}
	if offset != len(input) {
		return nil, nil, false
	}
	return authorTags, bodies, true
}

func decodeEvaluationEvidenceEntry(input []byte, offset int) (byte, []byte, int, bool) {
	if len(input)-offset < evaluationEvidenceEntryHeader {
		return 0, nil, 0, false
	}
	authorTag := input[offset]
	if !evaluationEvidenceAuthorTagKnown(authorTag) {
		return 0, nil, 0, false
	}
	bodyLength, ok := evaluationEvidenceFixedDecimal(input[offset+1 : offset+evaluationEvidenceEntryHeader])
	if !ok || bodyLength > evaluationEvidenceMaxFrameSize || bodyLength > len(input)-offset-evaluationEvidenceEntryHeader {
		return 0, nil, 0, false
	}
	bodyStart := offset + evaluationEvidenceEntryHeader
	body := append([]byte(nil), input[bodyStart:bodyStart+bodyLength]...)
	return authorTag, body, bodyStart + bodyLength, true
}

func evaluationEvidenceComments(frame evaluationEvidenceFrame) []pullRequestComment {
	comments := make([]pullRequestComment, 0, len(frame.bodies))
	for index, body := range frame.bodies {
		comment := pullRequestComment{
			Body:      string(body),
			CreatedAt: frame.timestamp.Add(time.Duration(index) * time.Minute),
		}
		comment.Author.Login = evaluationEvidenceAuthor(frame.authorTags[index])
		comments = append(comments, comment)
	}
	return comments
}

func evaluationEvidenceAuthorTagKnown(tag byte) bool {
	switch tag {
	case 'T', 'O', 'U':
		return true
	default:
		return false
	}
}

func evaluationEvidenceAuthor(tag byte) string {
	switch tag {
	case 'T':
		return trustedActor
	case 'O':
		return owner
	case 'U':
		return "untrusted-evaluator"
	default:
		return ""
	}
}

func evaluationEvidenceFixedDecimal(input []byte) (int, bool) {
	value := 0
	for _, digit := range input {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		value = value*10 + int(digit-'0')
	}
	return value, true
}

func evaluationEvidenceRawEqual(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !bytes.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}

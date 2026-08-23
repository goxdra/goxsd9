package workflowctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	evaluationAttestationBase64Marker = "workflowctl-evaluation-attestation-base64 "
	evaluationAttestationMarker       = "workflowctl-evaluation-attestation "
	evaluationAttestationSchema       = "goxsd9/examiner-attestation/v1"
	evaluationChallengeMarker         = "workflowctl-evaluation-challenge "
	evaluationChallengeDuration       = 2 * time.Hour
	evaluationMarker                  = "workflowctl-evaluation "
	evaluationReportBase64Marker      = "workflowctl-evaluation-report-base64-v1 "
	evaluationReportTransportV1       = "base64-v1"
	evaluationReceiptHeading          = "## Examiner evaluation — round receipt\n\n"
	evaluationRepairMarker            = "workflowctl-evaluation-repair-v1 "
	evaluationRepairSchema            = "goxsd9/examiner-evaluation-repair/v1"
	evaluationRepairHeading           = "## Examiner evaluation transport repair\n\n"
)

var evaluationReservedTextSequences = [...]struct {
	name  string
	value string
}{
	{name: "receipt marker", value: "<!-- " + evaluationMarker},
	{name: "base64 attestation marker", value: "<!-- " + evaluationAttestationBase64Marker},
	{name: "plain attestation marker", value: "<!-- " + evaluationAttestationMarker},
	{name: "report marker", value: "<!-- " + evaluationReportBase64Marker},
	{name: "repair marker", value: "<!-- " + evaluationRepairMarker},
	{name: "challenge marker", value: "<!-- " + evaluationChallengeMarker},
	{name: "receipt heading", value: evaluationReceiptHeading},
}

type pullRequestView struct {
	BaseRefName             string `json:"baseRefName"`
	BaseRefOID              string `json:"baseRefOid"`
	Body                    string `json:"body"`
	ClosingIssuesReferences []struct {
		Number int `json:"number"`
	} `json:"closingIssuesReferences"`
	Comments       []pullRequestComment `json:"comments"`
	HeadRefName    string               `json:"headRefName"`
	HeadRefOID     string               `json:"headRefOid"`
	IsDraft        bool                 `json:"isDraft"`
	Merged         bool                 `json:"merged"`
	MergedAt       *time.Time           `json:"mergedAt"`
	MergeCommitSHA string               `json:"mergeCommitSha"`
	State          string               `json:"state"`
	Title          string               `json:"title"`
	URL            string               `json:"url"`
}

type pullRequestComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type pullRequestAPI struct {
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA string     `json:"merge_commit_sha"`
	Base           struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
	Head  struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	State string `json:"state"`
	Title string `json:"title"`
	URL   string `json:"html_url"`
}

type issueCommentAPI struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

type evaluationReceipt struct {
	AttestationSHA256 string                 `json:"attestationSHA256,omitempty"`
	BaseRefName       string                 `json:"baseRefName,omitempty"`
	Challenge         string                 `json:"challenge,omitempty"`
	ClaimProofs       []evaluationClaimProof `json:"claimProofs,omitempty"`
	ClosingIssues     []int                  `json:"closingIssues,omitempty"`
	Evaluator         string                 `json:"evaluator"`
	EvaluatorRunID    string                 `json:"evaluatorRunID,omitempty"`
	Head              string                 `json:"head"`
	HeadRefName       string                 `json:"headRefName,omitempty"`
	BodySHA256        string                 `json:"bodySHA256,omitempty"`
	EvidenceSHA256    string                 `json:"evidenceSHA256,omitempty"`
	PR                int                    `json:"pullRequest,omitempty"`
	RecordedAt        time.Time              `json:"recordedAt"`
	ReportSHA256      string                 `json:"reportSHA256"`
	ReportTransport   string                 `json:"reportTransport,omitempty"`
	Round             int                    `json:"round"`
	Verdict           string                 `json:"verdict"`
}

type evaluationClaimProof struct {
	Issue  int    `json:"issue"`
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

type evaluationRepair struct {
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

type evaluationChallenge struct {
	Challenge      string    `json:"challenge"`
	Head           string    `json:"head"`
	PR             int       `json:"pullRequest"`
	BodySHA256     string    `json:"bodySHA256,omitempty"`
	EvidenceSHA256 string    `json:"evidenceSHA256,omitempty"`
	RequestedAt    time.Time `json:"requestedAt"`
}

type evaluationFinding struct {
	Impact             string `json:"impact"`
	Location           string `json:"location"`
	RequiredCorrection string `json:"requiredCorrection"`
}

type evaluationFindings []evaluationFinding

func (findings *evaluationFindings) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("findings must be a JSON array, not null")
	}
	var values []evaluationFinding
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		values = make([]evaluationFinding, 0)
	}
	*findings = values
	return nil
}

type evaluationAttestation struct {
	Challenge string             `json:"challenge"`
	Evaluator string             `json:"evaluator"`
	Findings  evaluationFindings `json:"findings"`
	Head      string             `json:"head"`
	PR        int                `json:"pullRequest"`
	RunID     string             `json:"runID"`
	Schema    string             `json:"schema"`
	Summary   string             `json:"summary"`
	Verdict   string             `json:"verdict"`
}

type evaluationChallengeRecord struct {
	comment      pullRequestComment
	commentIndex int
	challenge    evaluationChallenge
}

type evaluationReceiptRecord struct {
	comment      pullRequestComment
	commentIndex int
	receipt      evaluationReceipt
	marker       []byte
}

type evaluationRepairRecord struct {
	comment      pullRequestComment
	commentIndex int
	repair       evaluationRepair
}

type evaluationHistory struct {
	challenges []evaluationChallengeRecord
	receipts   []evaluationReceiptRecord
	repairs    []evaluationRepairRecord
}

func (a app) runEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation challenge PR | record PR --attestation-file FILE | repair PR --round ROUND")
	}
	switch args[0] {
	case "challenge":
		if len(args) != 2 {
			return usageError("usage: workflowctl evaluation challenge PR")
		}
		pr, err := positiveNumber(args[1])
		if err != nil {
			return usageError("evaluation challenge: %v", err)
		}
		return a.requestEvaluation(pr)
	case "record":
		return a.recordEvaluation(args[1:])
	case "repair":
		return a.repairEvaluation(args[1:])
	default:
		return usageError("unknown evaluation command %q", args[0])
	}
}

func (a app) repairEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation repair PR --round ROUND")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation repair: %v", err)
	}
	flags := flag.NewFlagSet("evaluation repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	round := flags.Int("round", 0, "evaluation round to repair")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation repair: %v", parseErr)
	}
	if flags.NArg() != 0 || *round < 1 {
		return usageError("usage: workflowctl evaluation repair PR --round ROUND")
	}
	return a.repairEvaluationReceipt(pr, *round)
}

func (a app) recordEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation record PR --attestation-file FILE")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation record: %v", err)
	}
	flags := flag.NewFlagSet("evaluation record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	attestationFile := flags.String("attestation-file", "", "structured Examiner attestation")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation record: %v", parseErr)
	}
	if flags.NArg() != 0 || *attestationFile == "" {
		return usageError("usage: workflowctl evaluation record PR --attestation-file FILE")
	}
	if err := requireRegularFile(*attestationFile); err != nil {
		return err
	}
	return a.postEvaluation(pr, *attestationFile)
}

func (a app) repairEvaluationReceipt(number, round int) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if evidenceErr := rejectUntrustedEvaluationEvidence(view.Comments); evidenceErr != nil {
		return stateError("PR #%d has untrusted evaluation evidence: %v", number, evidenceErr)
	}
	history, historyErr := parseEvaluationHistory(view.Comments)
	if historyErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, historyErr)
	}
	candidate, candidateErr := repairCandidate(history, round)
	if candidateErr != nil {
		return stateError("PR #%d round %d: %v", number, round, candidateErr)
	}
	attestation, rawAttestation, canonicalReport, candidateErr :=
		validateRepairCandidate(number, history, candidate)
	if candidateErr != nil {
		return stateError("PR #%d round %d: %v", number, round, candidateErr)
	}
	if historyErr := validateRepairHistory(history, candidate); historyErr != nil {
		return stateError("PR #%d round %d: %v", number, round, historyErr)
	}
	if len(rawAttestation) == 0 || attestation.Schema != evaluationAttestationSchema {
		return stateError("PR #%d round %d raw attestation evidence is malformed", number, round)
	}
	repair := evaluationRepair{
		AttestationSHA256:     sha256Hex(rawAttestation),
		Challenge:             candidate.receipt.Challenge,
		Evaluator:             candidate.receipt.Evaluator,
		EvaluatorRunID:        candidate.receipt.EvaluatorRunID,
		Head:                  candidate.receipt.Head,
		OriginalCommentSHA256: sha256Hex([]byte(candidate.comment.Body)),
		ReceiptMarkerSHA256:   sha256Hex(candidate.marker),
		PR:                    candidate.receipt.PR,
		ReportSHA256:          sha256Hex(canonicalReport),
		Round:                 candidate.receipt.Round,
		Schema:                evaluationRepairSchema,
		Verdict:               candidate.receipt.Verdict,
	}
	marker, err := json.Marshal(repair)
	if err != nil {
		return fmt.Errorf("encode evaluation repair: %w", err)
	}
	body := evaluationRepairComment(marker, round)
	repaired := append(append([]pullRequestComment(nil), view.Comments...), pullRequestComment{
		Author: struct {
			Login string `json:"login"`
		}{Login: trustedActor},
		Body:      body,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	})
	if _, err := evaluationReceipts(repaired); err != nil {
		return stateError("PR #%d generated an invalid evaluation repair: %v", number, err)
	}
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d transport repair recorded", number, round)
}

func repairCandidate(history evaluationHistory, round int) (evaluationReceiptRecord, error) {
	var candidate evaluationReceiptRecord
	matches := 0
	for _, record := range history.receipts {
		if record.receipt.Round != round {
			continue
		}
		candidate = record
		matches++
	}
	if matches != 1 {
		return evaluationReceiptRecord{}, fmt.Errorf("has %d trusted structured receipts; want exactly one", matches)
	}
	return candidate, nil
}

func validateRepairCandidate(number int, history evaluationHistory, candidate evaluationReceiptRecord) (
	evaluationAttestation, []byte, []byte, error) {
	receipt := candidate.receipt
	if receipt.PR != number {
		return evaluationAttestation{}, nil, nil, fmt.Errorf("targets PR #%d, want PR #%d", receipt.PR, number)
	}
	if !commentTimeMatches(candidate.comment.CreatedAt, receipt.RecordedAt) {
		return evaluationAttestation{}, nil, nil, errors.New("receipt timestamp does not match its comment")
	}
	if receipt.ReportTransport != "" || hasMarker(candidate.comment.Body, evaluationReportBase64Marker) {
		return evaluationAttestation{}, nil, nil, errors.New("receipt is intact or uses versioned report transport")
	}
	attestation, rawAttestation, canonicalReport, ok := receiptAttestation(candidate)
	if !ok {
		return evaluationAttestation{}, nil, nil, errors.New("missing or invalid raw attestation evidence")
	}
	if challengeErr := validateEvaluationReceiptChallenge(history, candidate); challengeErr != nil {
		return evaluationAttestation{}, nil, nil, challengeErr
	}
	visibleReport, reportOK := visibleEvaluationReport(candidate.comment.Body)
	if !reportOK {
		return evaluationAttestation{}, nil, nil, errors.New("no valid visible report projection")
	}
	if bytes.Equal(visibleReport, canonicalReport) {
		return evaluationAttestation{}, nil, nil, errors.New("report is intact; no transport repair is needed")
	}
	if sha256Hex(canonicalReport) != receipt.ReportSHA256 {
		return evaluationAttestation{}, nil, nil, errors.New("canonical report does not match its receipt hash")
	}
	return attestation, rawAttestation, canonicalReport, nil
}

func validateRepairHistory(history evaluationHistory, candidate evaluationReceiptRecord) error {
	if err := validateEvaluationHistoryExcept(history, candidate.receipt.Round); err != nil {
		return err
	}
	for _, repair := range history.repairs {
		if evaluationRepairMatchesRecord(candidate, repair) {
			return errors.New("receipt already has a transport repair")
		}
	}
	return nil
}

func (a app) requestEvaluation(number int) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if stateErr := requirePRReviewStateReady(view.Body); stateErr != nil {
		return stateError("PR #%d review state is not evidence-ready: %v", number, stateErr)
	}
	parsedEvidence, err := a.validatePREvidenceForPR(root, number, view)
	if err != nil {
		return err
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	challengeID, err := randomRunID()
	if err != nil {
		return err
	}
	challenge := evaluationChallenge{
		Challenge:      challengeID,
		Head:           view.HeadRefOID,
		PR:             number,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    time.Now().UTC().Truncate(time.Second),
	}
	marker, err := json.Marshal(challenge)
	if err != nil {
		return fmt.Errorf("encode evaluation challenge: %w", err)
	}
	body := fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker,
		view.HeadRefOID)
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	return writeLine(a.stdout, "%s", marker)
}

func (a app) postEvaluation(number int, attestationFile string) error {
	root, view, primary, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	parsedEvidence, err := a.validatePREvidenceForPR(root, number, view)
	if err != nil {
		return err
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	receipts, receiptsErr := evaluationReceipts(view.Comments)
	if receiptsErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, receiptsErr)
	}
	attestation, attestationJSON, err := readEvaluationAttestation(attestationFile)
	if err != nil {
		return err
	}
	failedRounds := evaluationFailureCount(receipts)
	if failedRounds >= 3 {
		return a.reconcileRecordedNeedsHuman(root, number, primary, view, receipts, attestation, attestationJSON)
	}
	if validationErr := validateEvaluationAttestation(attestation, number, view, receipts,
		time.Now().UTC()); validationErr != nil {
		return stateError("reject Examiner attestation: %v", validationErr)
	}
	claimProofs, err := a.evaluationClaimProofs(root, view, primary)
	if err != nil {
		return err
	}
	report := renderEvaluationReport(attestation)
	canonicalReport := canonicalEvaluationReport(report)
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationJSON),
		BaseRefName:       view.BaseRefName,
		Challenge:         attestation.Challenge,
		ClaimProofs:       claimProofs,
		ClosingIssues:     closingIssueNumbers(view.Body),
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		HeadRefName:       view.HeadRefName,
		BodySHA256:        bodySHA256,
		EvidenceSHA256:    evidenceSHA256,
		PR:                attestation.PR,
		RecordedAt:        time.Now().UTC().Truncate(time.Second),
		ReportSHA256:      sha256Hex(canonicalReport),
		ReportTransport:   evaluationReportTransportV1,
		Round:             len(receipts) + 1,
		Verdict:           attestation.Verdict,
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode evaluation receipt: %w", err)
	}
	body := evaluationComment(marker, attestationJSON, string(canonicalReport))
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	if attestation.Verdict == "fail" && failedRounds+1 == 3 {
		if err := a.transitionIssueToNeedsHuman(root, primary); err != nil {
			return err
		}
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d: %s (%s)", number, receipt.Round,
		attestation.Verdict, view.HeadRefOID)
}

func (a app) reconcileRecordedNeedsHuman(root string, number, primary int, view pullRequestView,
	receipts []evaluationReceipt, attestation evaluationAttestation, attestationJSON []byte,
) error {
	receipt, ok := thirdFailureReceipt(receipts)
	if !ok {
		return stateError("PR #%d already has three failed evaluation rounds; exact third-failure receipt is missing",
			number)
	}
	if err := validateRecordedAttestation(attestation, attestationJSON, receipt, number, view); err != nil {
		return stateError("PR #%d already has three failed evaluation rounds; exact third-failure attestation required: %v",
			number, err)
	}
	if err := a.transitionIssueToNeedsHuman(root, primary); err != nil {
		return err
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d needs-human transition reconciled (%s)",
		number, receipt.Round, view.HeadRefOID)
}

func (a app) evaluationClaimProofs(root string, view pullRequestView, primary int) ([]evaluationClaimProof, error) {
	proofs := []evaluationClaimProof{{Issue: primary, Branch: view.HeadRefName, SHA: view.HeadRefOID}}
	if len(view.ClosingIssuesReferences) < 2 {
		return proofs, nil
	}
	claims, err := a.listRemoteClaims(root)
	if err != nil {
		return nil, err
	}
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		claim, err := a.findActiveCompanionClaim(root, issue.Number, view.HeadRefOID, claims)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, evaluationClaimProof{Issue: issue.Number, Branch: claim.branch, SHA: claim.sha})
	}
	return proofs, nil
}

func (a app) findActiveCompanionClaim(root string, issue int, head string, claims []remoteClaim) (remoteClaim, error) {
	candidates := make([]remoteClaim, 0, 1)
	for _, claim := range claims {
		if claim.number != issue || !claim.active {
			continue
		}
		if _, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head); err != nil {
			if isGitNonAncestor(err) {
				continue
			}
			return remoteClaim{}, fmt.Errorf("prove companion issue #%d claim %s is included in evaluated head: %w", issue, claim.branch, err)
		}
		candidates = append(candidates, claim)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return remoteClaim{}, stateError("companion issue #%d has ambiguous active claims in evaluated head %s", issue, head)
	}
	return remoteClaim{}, stateError("companion issue #%d has no active claim", issue)
}

func (a app) readEvaluationTarget(number int) (string, pullRequestView, int, error) {
	root, _, primary, err := a.currentClaim()
	if err != nil {
		return "", pullRequestView{}, 0, err
	}
	branch := claimBranch(primary)
	quiet := a
	quiet.stdout = io.Discard
	if verifyErr := quiet.verifyClaim(); verifyErr != nil {
		return "", pullRequestView{}, 0, verifyErr
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return "", pullRequestView{}, 0, err
	}
	if view.State != "OPEN" {
		return "", pullRequestView{}, 0, stateError("PR #%d is %s", number, view.State)
	}
	if view.HeadRefName != branch {
		return "", pullRequestView{}, 0, stateError("PR #%d uses branch %s, not claim branch %s", number,
			view.HeadRefName, branch)
	}
	if closingErr := a.validateClosingClaims(root, view, primary); closingErr != nil {
		return "", pullRequestView{}, 0, closingErr
	}
	local, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", pullRequestView{}, 0, fmt.Errorf("read evaluation head: %w", err)
	}
	if local != view.HeadRefOID {
		return "", pullRequestView{}, 0, stateError("PR #%d head is %s, but claim worktree is %s", number,
			view.HeadRefOID, local)
	}
	return root, view, primary, nil
}

func evaluationFailureCount(receipts []evaluationReceipt) int {
	count := 0
	for _, receipt := range receipts {
		if receipt.Verdict == "fail" {
			count++
		}
	}
	return count
}

func thirdFailureReceipt(receipts []evaluationReceipt) (evaluationReceipt, bool) {
	failures := 0
	for _, receipt := range receipts {
		if receipt.Verdict != "fail" {
			continue
		}
		failures++
		if failures == 3 {
			return receipt, true
		}
	}
	return evaluationReceipt{}, false
}

func validateRecordedAttestation(attestation evaluationAttestation, attestationJSON []byte,
	receipt evaluationReceipt, number int, view pullRequestView,
) error {
	if receipt.Verdict != "fail" {
		return errors.New("recorded third-failure receipt is not failing")
	}
	if receipt.AttestationSHA256 == "" || sha256Hex(attestationJSON) != receipt.AttestationSHA256 {
		return errors.New("attestation bytes do not match the recorded receipt")
	}
	if attestation.Schema != evaluationAttestationSchema || attestation.Evaluator != "Examiner" ||
		strings.TrimSpace(attestation.RunID) == "" {
		return errors.New("attestation identity is invalid")
	}
	if attestation.PR != number || attestation.Head != view.HeadRefOID || receipt.PR != number ||
		receipt.Head != view.HeadRefOID {
		return errors.New("attestation targets a different PR or head")
	}
	if attestation.Challenge != receipt.Challenge || attestation.RunID != receipt.EvaluatorRunID ||
		attestation.Evaluator != receipt.Evaluator || attestation.Verdict != receipt.Verdict {
		return errors.New("attestation identity differs from the recorded receipt")
	}
	if strings.TrimSpace(attestation.Summary) == "" {
		return errors.New("summary is empty")
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return err
	}
	return validateEvaluationAttestationText(attestation)
}

func readEvaluationAttestation(path string) (evaluationAttestation, []byte, error) {
	// #nosec G304 -- path is an explicit operator-supplied input.
	data, err := os.ReadFile(path)
	if err != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("read Examiner attestation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if duplicateErr := rejectDuplicateJSONKeys(data); duplicateErr != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("decode Examiner attestation: %w", duplicateErr)
	}
	decoder.DisallowUnknownFields()
	var attestation evaluationAttestation
	if decodeErr := decoder.Decode(&attestation); decodeErr != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("decode Examiner attestation: %w", decodeErr)
	}
	if trailingErr := requireAttestationJSONEnd(decoder); trailingErr != nil {
		return evaluationAttestation{}, nil, trailingErr
	}
	return attestation, data, nil
}

func requireAttestationJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode Examiner attestation trailer: %w", err)
	}
	return errors.New("examiner attestation contains more than one JSON value")
}

func validateEvaluationAttestation(attestation evaluationAttestation, number int, view pullRequestView,
	receipts []evaluationReceipt, now time.Time) error {
	if attestation.Schema != evaluationAttestationSchema {
		return fmt.Errorf("schema is %q, want %q", attestation.Schema, evaluationAttestationSchema)
	}
	if attestation.Evaluator != "Examiner" || strings.TrimSpace(attestation.RunID) == "" {
		return errors.New("evaluator must be Examiner with a nonempty fresh-context run ID")
	}
	if attestation.PR != number || attestation.Head != view.HeadRefOID {
		return fmt.Errorf("attestation targets PR #%d at %s, want PR #%d at %s", attestation.PR,
			attestation.Head, number, view.HeadRefOID)
	}
	if strings.TrimSpace(attestation.Summary) == "" {
		return errors.New("summary is empty")
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return err
	}
	if err := validateEvaluationAttestationText(attestation); err != nil {
		return err
	}
	challenge, ok := trustedEvaluationChallengeForView(view, attestation.Challenge, number, now)
	if !ok {
		return errors.New("challenge is missing, stale, untrusted, or for another head")
	}
	for _, receipt := range receipts {
		if receipt.Challenge == challenge.Challenge {
			return errors.New("challenge was already used")
		}
		if receipt.EvaluatorRunID == attestation.RunID {
			return errors.New("examiner run ID was already used")
		}
	}
	return nil
}

func validateEvaluationAttestationText(attestation evaluationAttestation) error {
	type evaluationTextField struct {
		name  string
		value string
	}
	fields := make([]evaluationTextField, 1, 1+3*len(attestation.Findings))
	fields[0] = evaluationTextField{name: "summary", value: attestation.Summary}
	for index, finding := range attestation.Findings {
		fields = append(fields,
			evaluationTextField{name: fmt.Sprintf("finding %d location", index+1), value: finding.Location},
			evaluationTextField{name: fmt.Sprintf("finding %d impact", index+1), value: finding.Impact},
			evaluationTextField{name: fmt.Sprintf("finding %d required correction", index+1),
				value: finding.RequiredCorrection},
		)
	}
	for _, field := range fields {
		for _, sequence := range evaluationReservedTextSequences {
			if strings.Contains(field.value, sequence.value) {
				return fmt.Errorf("%s contains reserved %s", field.name, sequence.name)
			}
		}
	}
	return nil
}

func validateEvaluationFindings(attestation evaluationAttestation) error {
	if attestation.Findings == nil {
		return errors.New("findings must be present as a JSON array")
	}
	if attestation.Verdict != "pass" && attestation.Verdict != "fail" {
		return fmt.Errorf("invalid verdict %q", attestation.Verdict)
	}
	if attestation.Verdict == "pass" && len(attestation.Findings) != 0 {
		return errors.New("passing attestation contains blocking findings")
	}
	if attestation.Verdict == "fail" && len(attestation.Findings) == 0 {
		return errors.New("failing attestation has no blocking findings")
	}
	for index, finding := range attestation.Findings {
		if strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Impact) == "" ||
			strings.TrimSpace(finding.RequiredCorrection) == "" {
			return fmt.Errorf("finding %d is missing location, impact, or required correction", index+1)
		}
	}
	return nil
}

func trustedEvaluationChallenge(comments []pullRequestComment, challengeID string, number int, head string,
	now time.Time) (evaluationChallenge, bool) {
	var trusted evaluationChallenge
	matches := 0
	for index, comment := range comments {
		if comment.Author.Login != trustedActor {
			continue
		}
		challenge, ok := parseEvaluationChallenge(comment.Body)
		if !ok || challenge.Challenge != challengeID || challenge.PR != number || challenge.Head != head {
			continue
		}
		matches++
		record := evaluationChallengeRecord{
			comment:      comment,
			commentIndex: index,
			challenge:    challenge,
		}
		if !evaluationChallengeRecordValidAt(record, now) {
			return evaluationChallenge{}, false
		}
		trusted = challenge
	}
	return trusted, matches == 1
}

func trustedEvaluationChallengeForView(view pullRequestView, challengeID string, number int,
	now time.Time,
) (evaluationChallenge, bool) {
	challenge, ok := trustedEvaluationChallenge(view.Comments, challengeID, number, view.HeadRefOID, now)
	if !ok {
		return evaluationChallenge{}, false
	}
	if !hasCurrentPREvidence(view) {
		return challenge, true
	}
	parsed, err := validatePREvidenceForView(view)
	if err != nil || challenge.BodySHA256 == "" || challenge.EvidenceSHA256 == "" {
		return evaluationChallenge{}, false
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsed)
	if challenge.BodySHA256 != bodySHA256 || challenge.EvidenceSHA256 != evidenceSHA256 {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func hasCurrentPREvidence(view pullRequestView) bool {
	return strings.Contains(view.Body, evidenceMarkerToken(prEvidenceStartMarker)) ||
		strings.Contains(view.Body, evidenceMarkerToken(prEvidenceEndMarker)) || view.BaseRefOID != ""
}

func parseEvaluationChallenge(body string) (evaluationChallenge, bool) {
	value, ok := markerJSON(body, evaluationChallengeMarker)
	if !ok {
		return evaluationChallenge{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationChallenge{}, false
	}
	decoder.DisallowUnknownFields()
	var challenge evaluationChallenge
	if err := decoder.Decode(&challenge); err != nil {
		return evaluationChallenge{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationChallenge{}, false
	}
	if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
		return evaluationChallenge{}, false
	}
	if (challenge.BodySHA256 == "") != (challenge.EvidenceSHA256 == "") ||
		(challenge.BodySHA256 != "" && (!validSHA256(challenge.BodySHA256) || !validSHA256(challenge.EvidenceSHA256))) {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func markerJSON(body, marker string) ([]byte, bool) {
	return markerBytes(body, marker)
}

func markerBytes(body, marker string) ([]byte, bool) {
	values, found := markerValues(body, marker)
	if !found || len(values) != 1 {
		return nil, false
	}
	return values[0], true
}

func markerValues(body, marker string) ([][]byte, bool) {
	prefix := "<!-- " + marker
	var values [][]byte
	searchFrom := 0
	for {
		relativeStart := strings.Index(body[searchFrom:], prefix)
		if relativeStart < 0 {
			return values, len(values) > 0
		}
		start := searchFrom + relativeStart + len(prefix)
		relativeEnd := strings.Index(body[start:], " -->")
		if relativeEnd < 0 {
			return nil, true
		}
		values = append(values, []byte(body[start:start+relativeEnd]))
		searchFrom = start + relativeEnd + len(" -->")
	}
}

func hasMarker(body, marker string) bool {
	return strings.Contains(body, "<!-- "+marker)
}

func commentTimeMatches(commentTime, markerTime time.Time) bool {
	return !commentTime.Before(markerTime.Add(-5*time.Minute)) && !commentTime.After(markerTime.Add(5*time.Minute))
}

func renderEvaluationReport(attestation evaluationAttestation) string {
	parts := make([]string, 0, 1+len(attestation.Findings))
	parts = append(parts, "**"+strings.ToUpper(attestation.Verdict)+"**\n\n"+strings.TrimSpace(attestation.Summary))
	for index, finding := range attestation.Findings {
		parts = append(parts, fmt.Sprintf("%d. `%s` — %s Required correction: %s", index+1,
			strings.TrimSpace(finding.Location), strings.TrimSpace(finding.Impact),
			strings.TrimSpace(finding.RequiredCorrection)))
	}
	return strings.Join(parts, "\n\n")
}

func evaluationComment(receiptMarker, attestationMarker []byte, report string) string {
	canonical := canonicalEvaluationReport(report)
	reportEncoded := base64.StdEncoding.EncodeToString(canonical)
	if len(attestationMarker) == 0 {
		return fmt.Sprintf("<!-- %s%s -->\n<!-- %s%s -->\n%s%s\n", evaluationMarker, receiptMarker,
			evaluationReportBase64Marker, reportEncoded, evaluationReceiptHeading, canonical)
	}
	encoded := base64.StdEncoding.EncodeToString(attestationMarker)
	return fmt.Sprintf("<!-- %s%s -->\n<!-- %s%s -->\n<!-- %s%s -->\n%s%s\n", evaluationMarker,
		receiptMarker, evaluationAttestationBase64Marker, encoded, evaluationReportBase64Marker,
		reportEncoded, evaluationReceiptHeading, canonical)
}

func canonicalEvaluationReport(report string) []byte {
	return []byte(strings.TrimSpace(report))
}

func evaluationRepairComment(marker []byte, round int) string {
	return fmt.Sprintf("<!-- %s%s -->\n%sRound %d is superseded only for its visible report projection; the original "+
		"receipt and Examiner evidence remain authoritative.\n", evaluationRepairMarker, marker,
		evaluationRepairHeading, round)
}

func (a app) postPullRequestComment(root string, number int, body string) error {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return fmt.Errorf("encode PR #%d comment: %w", number, err)
	}
	if _, err := a.commandInput(root, strings.NewReader(string(payload)), "gh", "api", "--method", "POST",
		"repos/"+repositoryKey+"/issues/"+strconv.Itoa(number)+"/comments", "--input", "-"); err != nil {
		return fmt.Errorf("comment on PR #%d: %w", number, err)
	}
	return nil
}

func (a app) readPullRequest(root string, number int) (pullRequestView, error) {
	output, err := a.command(root, "gh", "api", "repos/"+repositoryKey+"/pulls/"+strconv.Itoa(number))
	if err != nil {
		return pullRequestView{}, fmt.Errorf("read PR #%d: %w", number, err)
	}
	var response pullRequestAPI
	if decodeErr := json.Unmarshal([]byte(output), &response); decodeErr != nil {
		return pullRequestView{}, fmt.Errorf("decode PR #%d: %w", number, decodeErr)
	}
	comments, err := a.readPullRequestComments(root, number)
	if err != nil {
		return pullRequestView{}, err
	}
	view := pullRequestViewFromAPI(response)
	view.Comments = comments
	return view, nil
}

func pullRequestViewFromAPI(response pullRequestAPI) pullRequestView {
	view := pullRequestView{
		BaseRefName:    response.Base.Ref,
		BaseRefOID:     response.Base.SHA,
		Body:           response.Body,
		HeadRefName:    response.Head.Ref,
		HeadRefOID:     response.Head.SHA,
		IsDraft:        response.Draft,
		Merged:         response.Merged || response.MergedAt != nil,
		MergedAt:       response.MergedAt,
		MergeCommitSHA: response.MergeCommitSHA,
		State:          strings.ToUpper(response.State),
		Title:          response.Title,
		URL:            response.URL,
	}
	for _, issue := range closingIssueNumbers(response.Body) {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: issue})
	}
	return view
}

func (a app) readPullRequestComments(root string, number int) ([]pullRequestComment, error) {
	endpoint := "repos/" + repositoryKey + "/issues/" + strconv.Itoa(number) + "/comments?per_page=100"
	output, err := a.command(root, "gh", "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("read PR #%d comments: %w", number, err)
	}
	pages, err := decodeJSONDocuments[[]issueCommentAPI](output)
	if err != nil {
		return nil, fmt.Errorf("decode PR #%d comments: %w", number, err)
	}
	var comments []pullRequestComment
	for _, page := range pages {
		for _, response := range page {
			comment := pullRequestComment{Body: response.Body, CreatedAt: response.CreatedAt}
			comment.Author.Login = response.User.Login
			comments = append(comments, comment)
		}
	}
	return comments, nil
}

func decodeJSONDocuments[T any](output string) ([]T, error) {
	decoder := json.NewDecoder(strings.NewReader(output))
	var documents []T
	for {
		var document T
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			if len(documents) == 0 {
				return nil, errors.New("no JSON documents")
			}
			return documents, nil
		}
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
}

func containsNumber(numbers []int, target int) bool {
	for _, number := range numbers {
		if number == target {
			return true
		}
	}
	return false
}

func evaluationReceipts(comments []pullRequestComment) ([]evaluationReceipt, error) {
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return nil, err
	}
	if err := validateEvaluationHistory(history); err != nil {
		return nil, err
	}
	receipts := make([]evaluationReceipt, 0, len(history.receipts))
	for _, record := range history.receipts {
		receipts = append(receipts, record.receipt)
	}
	return receipts, nil
}

func parseEvaluationHistory(comments []pullRequestComment) (evaluationHistory, error) {
	var history evaluationHistory
	for commentIndex, comment := range comments {
		if err := appendEvaluationHistoryComment(&history, comment, commentIndex); err != nil {
			return evaluationHistory{}, err
		}
	}
	return history, nil
}

func appendEvaluationHistoryComment(history *evaluationHistory, comment pullRequestComment, commentIndex int) error {
	if comment.Author.Login != trustedActor {
		if hasMarker(comment.Body, evaluationChallengeMarker) {
			return errors.New("evaluation challenge marker has an untrusted author")
		}
		return nil
	}
	challenge, found, err := parseEvaluationChallengeRecord(comment, commentIndex)
	if err != nil {
		return err
	}
	if found {
		history.challenges = append(history.challenges, *challenge)
		return nil
	}
	receipt, repair, err := parseTrustedEvaluationComment(comment, commentIndex)
	if err != nil {
		return err
	}
	if receipt != nil {
		history.receipts = append(history.receipts, *receipt)
		return nil
	}
	if repair != nil {
		history.repairs = append(history.repairs, *repair)
	}
	return nil
}

func parseEvaluationChallengeRecord(comment pullRequestComment, commentIndex int) (
	*evaluationChallengeRecord, bool, error) {
	if !hasMarker(comment.Body, evaluationChallengeMarker) {
		return nil, false, nil
	}
	if evaluationChallengeContainsReceiptEvidence(comment.Body) {
		return nil, false, errors.New("trusted evaluation challenge also contains receipt evidence")
	}
	challenge, ok := parseEvaluationChallenge(comment.Body)
	if !ok {
		return nil, false, errors.New("trusted evaluation challenge marker is malformed")
	}
	return &evaluationChallengeRecord{
		comment:      comment,
		commentIndex: commentIndex,
		challenge:    challenge,
	}, true, nil
}

func evaluationChallengeContainsReceiptEvidence(body string) bool {
	return hasMarker(body, evaluationRepairMarker) || hasMarker(body, evaluationMarker) ||
		strings.Contains(body, evaluationReceiptHeading) || hasMarker(body, evaluationReportBase64Marker) ||
		hasMarker(body, evaluationAttestationBase64Marker) || hasMarker(body, evaluationAttestationMarker)
}

func parseTrustedEvaluationComment(comment pullRequestComment, commentIndex int) (*evaluationReceiptRecord, *evaluationRepairRecord, error) {
	hasRepairMarker := hasMarker(comment.Body, evaluationRepairMarker)
	hasReceiptMarker := hasMarker(comment.Body, evaluationMarker)
	hasReceiptHeading := strings.Contains(comment.Body, evaluationReceiptHeading)
	hasReceipt := hasReceiptMarker || hasReceiptHeading
	hasReportEvidence := hasMarker(comment.Body, evaluationReportBase64Marker) ||
		hasMarker(comment.Body, evaluationAttestationBase64Marker) ||
		hasMarker(comment.Body, evaluationAttestationMarker)
	if hasRepairMarker {
		if hasReceipt || hasReportEvidence || hasMarker(comment.Body, evaluationChallengeMarker) {
			return nil, nil, errors.New("trusted evaluation repair also contains a receipt")
		}
		repair, ok := parseEvaluationRepair(comment.Body)
		if !ok {
			return nil, nil, errors.New("trusted evaluation repair marker is malformed")
		}
		return nil, &evaluationRepairRecord{comment: comment, commentIndex: commentIndex, repair: repair}, nil
	}
	if !hasReceipt {
		if hasReportEvidence {
			return nil, nil, errors.New("trusted evaluation evidence has no receipt")
		}
		return nil, nil, nil
	}
	receipt, ok := parseEvaluationReceipt(comment.Body)
	if !ok {
		return nil, nil, errors.New("trusted automation evaluation receipt marker is malformed")
	}
	marker, ok := markerBytes(comment.Body, evaluationMarker)
	if !ok {
		return nil, nil, fmt.Errorf("evaluation round %d receipt marker is malformed", receipt.Round)
	}
	return &evaluationReceiptRecord{
		comment: comment, commentIndex: commentIndex, receipt: receipt, marker: marker,
	}, nil, nil
}

func rejectUntrustedEvaluationEvidence(comments []pullRequestComment) error {
	for _, comment := range comments {
		if comment.Author.Login == trustedActor {
			continue
		}
		if hasMarker(comment.Body, evaluationMarker) || strings.Contains(comment.Body, evaluationReceiptHeading) ||
			hasMarker(comment.Body, evaluationRepairMarker) ||
			hasMarker(comment.Body, evaluationReportBase64Marker) ||
			hasMarker(comment.Body, evaluationAttestationBase64Marker) ||
			hasMarker(comment.Body, evaluationAttestationMarker) ||
			hasMarker(comment.Body, evaluationChallengeMarker) {
			return errors.New("structured receipt, report, attestation, repair, or challenge marker has an untrusted author")
		}
	}
	return nil
}

func evaluationRepairCommentIsValid(comment pullRequestComment) bool {
	if comment.Author.Login != trustedActor {
		return false
	}
	marker, ok := markerBytes(comment.Body, evaluationRepairMarker)
	if !ok {
		return false
	}
	repair, ok := parseEvaluationRepair(comment.Body)
	if !ok {
		return false
	}
	return comment.Body == evaluationRepairComment(marker, repair.Round)
}

func validateEvaluationHistory(history evaluationHistory) error {
	return validateEvaluationHistoryExcept(history, 0)
}

func validateEvaluationHistoryExcept(history evaluationHistory, exceptRound int) error {
	if err := validateEvaluationReceiptsExcept(history, exceptRound); err != nil {
		return err
	}
	return validateEvaluationRepairs(history)
}

func validateEvaluationReceiptsExcept(history evaluationHistory, exceptRound int) error {
	seenRounds := make(map[int]struct{}, len(history.receipts))
	seenChallenges := make(map[string]struct{}, len(history.receipts))
	seenRunIDs := make(map[string]struct{}, len(history.receipts))
	for _, record := range history.receipts {
		receipt := record.receipt
		if _, seen := seenRounds[receipt.Round]; seen {
			return fmt.Errorf("evaluation round %d has duplicate trusted receipts", record.receipt.Round)
		}
		seenRounds[receipt.Round] = struct{}{}
		if err := recordEvaluationIdentifier(seenChallenges, "challenge", receipt.Challenge); err != nil {
			return err
		}
		if err := recordEvaluationIdentifier(seenRunIDs, "examiner run ID", receipt.EvaluatorRunID); err != nil {
			return err
		}
		if err := validateEvaluationReceiptChallenge(history, record); err != nil {
			return err
		}
		if receipt.Round == exceptRound {
			continue
		}
		if !evaluationReceiptMatchesRecord(record, history.repairs) {
			return fmt.Errorf("evaluation round %d receipt failed integrity validation", record.receipt.Round)
		}
	}
	return nil
}

func recordEvaluationIdentifier(seen map[string]struct{}, label, value string) error {
	if value == "" {
		return nil
	}
	if _, ok := seen[value]; ok {
		return fmt.Errorf("evaluation %s %q has duplicate trusted receipts", label, value)
	}
	seen[value] = struct{}{}
	return nil
}

func validateEvaluationReceiptChallenge(history evaluationHistory, record evaluationReceiptRecord) error {
	if record.receipt.AttestationSHA256 == "" {
		return nil
	}
	matches := 0
	for _, challenge := range history.challenges {
		if evaluationChallengeMatchesReceipt(challenge, record) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("evaluation round %d has %d matching trusted challenges", record.receipt.Round, matches)
	}
	return nil
}

func evaluationChallengeMatchesReceipt(challenge evaluationChallengeRecord, receipt evaluationReceiptRecord) bool {
	if challenge.challenge.Challenge != receipt.receipt.Challenge ||
		challenge.challenge.PR != receipt.receipt.PR || challenge.challenge.Head != receipt.receipt.Head {
		return false
	}
	if challenge.challenge.BodySHA256 != "" || challenge.challenge.EvidenceSHA256 != "" ||
		receipt.receipt.EvidenceSHA256 != "" {
		if challenge.challenge.BodySHA256 != receipt.receipt.BodySHA256 ||
			challenge.challenge.EvidenceSHA256 != receipt.receipt.EvidenceSHA256 {
			return false
		}
	}
	if challenge.commentIndex >= receipt.commentIndex ||
		challenge.comment.CreatedAt.After(receipt.comment.CreatedAt) {
		return false
	}
	return evaluationChallengeRecordValidAt(challenge, receipt.receipt.RecordedAt)
}

func evaluationChallengeRecordValidAt(record evaluationChallengeRecord, at time.Time) bool {
	challenge := record.challenge
	if record.comment.CreatedAt.IsZero() || record.comment.CreatedAt.After(at) ||
		!commentTimeMatches(record.comment.CreatedAt, challenge.RequestedAt) ||
		challenge.RequestedAt.After(at) || !at.Before(challenge.RequestedAt.Add(evaluationChallengeDuration)) {
		return false
	}
	return true
}

func validateEvaluationRepairs(history evaluationHistory) error {
	for _, repair := range history.repairs {
		if !evaluationRepairCommentIsValid(repair.comment) {
			return errors.New("evaluation repair comment is not machine-generated")
		}
		matches := 0
		for _, record := range history.receipts {
			if evaluationRepairMatchesRecord(record, repair) {
				matches++
			}
		}
		if matches != 1 {
			return errors.New("evaluation repair does not supersede exactly one receipt")
		}
	}
	return nil
}

func parseEvaluationReceipt(body string) (evaluationReceipt, bool) {
	value, ok := markerBytes(body, evaluationMarker)
	if !ok {
		return evaluationReceipt{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationReceipt{}, false
	}
	decoder.DisallowUnknownFields()
	var receipt evaluationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return evaluationReceipt{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationReceipt{}, false
	}
	if receipt.Evaluator != "Examiner" || receipt.Round < 1 || receipt.RecordedAt.IsZero() ||
		(receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" ||
		!validSHA256(receipt.ReportSHA256) {
		return evaluationReceipt{}, false
	}
	if receipt.ReportTransport != "" && receipt.ReportTransport != evaluationReportTransportV1 {
		return evaluationReceipt{}, false
	}
	if receipt.EvidenceSHA256 != "" && !validSHA256(receipt.EvidenceSHA256) {
		return evaluationReceipt{}, false
	}
	if receipt.AttestationSHA256 != "" && (!validSHA256(receipt.AttestationSHA256) || receipt.Challenge == "" ||
		receipt.EvaluatorRunID == "" || receipt.PR < 1) {
		return evaluationReceipt{}, false
	}
	if !validEvaluationReceiptMetadata(receipt) {
		return evaluationReceipt{}, false
	}
	return receipt, true
}

func validEvaluationReceiptMetadata(receipt evaluationReceipt) bool {
	hasMetadata := receipt.BaseRefName != "" || len(receipt.ClosingIssues) != 0 || receipt.HeadRefName != "" ||
		receipt.BodySHA256 != "" || receipt.EvidenceSHA256 != "" || receipt.ClaimProofs != nil
	if !hasMetadata {
		return true
	}
	if receipt.BaseRefName == "" || receipt.HeadRefName == "" || !validSHA256(receipt.BodySHA256) ||
		len(receipt.ClosingIssues) == 0 {
		return false
	}
	if receipt.EvidenceSHA256 != "" && !validSHA256(receipt.EvidenceSHA256) {
		return false
	}
	if !validEvaluationIssueList(receipt.ClosingIssues) {
		return false
	}
	if receipt.ClaimProofs == nil {
		return len(receipt.ClosingIssues) == 1
	}
	return validEvaluationClaimProofs(receipt.ClosingIssues, receipt.ClaimProofs)
}

func validEvaluationIssueList(issues []int) bool {
	seen := make(map[int]struct{}, len(issues))
	for _, issue := range issues {
		if issue < 1 {
			return false
		}
		if _, ok := seen[issue]; ok {
			return false
		}
		seen[issue] = struct{}{}
	}
	return true
}

func validEvaluationClaimProofs(issues []int, proofs []evaluationClaimProof) bool {
	if len(proofs) != len(issues) {
		return false
	}
	seen := make(map[int]struct{}, len(proofs))
	for _, proof := range proofs {
		if proof.Issue < 1 || proof.Branch == "" || proof.SHA == "" {
			return false
		}
		issue, ok := fixedClaimIssue(proof.Branch)
		if !ok || issue != proof.Issue {
			return false
		}
		if _, ok := seen[proof.Issue]; ok {
			return false
		}
		seen[proof.Issue] = struct{}{}
	}
	for _, issue := range issues {
		if _, ok := seen[issue]; !ok {
			return false
		}
	}
	return true
}

func parseEvaluationRepair(body string) (evaluationRepair, bool) {
	value, ok := markerBytes(body, evaluationRepairMarker)
	if !ok {
		return evaluationRepair{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationRepair{}, false
	}
	decoder.DisallowUnknownFields()
	var repair evaluationRepair
	if err := decoder.Decode(&repair); err != nil {
		return evaluationRepair{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationRepair{}, false
	}
	if repair.Schema != evaluationRepairSchema || repair.Evaluator != "Examiner" || repair.PR < 1 ||
		repair.Round < 1 || repair.Head == "" || repair.Challenge == "" || repair.EvaluatorRunID == "" ||
		(repair.Verdict != "pass" && repair.Verdict != "fail") || !validSHA256(repair.AttestationSHA256) ||
		!validSHA256(repair.OriginalCommentSHA256) || !validSHA256(repair.ReceiptMarkerSHA256) ||
		!validSHA256(repair.ReportSHA256) {
		return evaluationRepair{}, false
	}
	return repair, true
}

func receiptAttestation(record evaluationReceiptRecord) (evaluationAttestation, []byte, []byte, bool) {
	hasBase64 := hasMarker(record.comment.Body, evaluationAttestationBase64Marker)
	hasPlain := hasMarker(record.comment.Body, evaluationAttestationMarker)
	if record.receipt.AttestationSHA256 == "" {
		if hasBase64 || hasPlain {
			return evaluationAttestation{}, nil, nil, false
		}
		return evaluationAttestation{}, nil, nil, true
	}
	attestation, raw, ok := parseCommentAttestation(record.comment.Body)
	if !ok || sha256Hex(raw) != record.receipt.AttestationSHA256 ||
		attestation.Challenge != record.receipt.Challenge || attestation.Evaluator != record.receipt.Evaluator ||
		attestation.RunID != record.receipt.EvaluatorRunID || attestation.Head != record.receipt.Head ||
		attestation.PR != record.receipt.PR || attestation.Verdict != record.receipt.Verdict ||
		attestation.Schema != evaluationAttestationSchema || strings.TrimSpace(attestation.Summary) == "" {
		return evaluationAttestation{}, nil, nil, false
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return evaluationAttestation{}, nil, nil, false
	}
	if err := validateEvaluationAttestationText(attestation); err != nil {
		return evaluationAttestation{}, nil, nil, false
	}
	return attestation, raw, canonicalEvaluationReport(renderEvaluationReport(attestation)), true
}

func evaluationRepairMatchesRecord(record evaluationReceiptRecord, repairRecord evaluationRepairRecord) bool {
	repair := repairRecord.repair
	if record.receipt.ReportTransport != "" || record.receipt.AttestationSHA256 == "" ||
		hasMarker(record.comment.Body, evaluationReportBase64Marker) {
		return false
	}
	if !repairFollowsReceipt(record, repairRecord) {
		return false
	}
	attestation, rawAttestation, canonicalReport, ok := receiptAttestation(record)
	if !ok {
		return false
	}
	visibleReport, reportOK := visibleEvaluationReport(record.comment.Body)
	if !reportOK || bytes.Equal(visibleReport, canonicalReport) ||
		sha256Hex(canonicalReport) != record.receipt.ReportSHA256 {
		return false
	}
	receipt := record.receipt
	return repair.AttestationSHA256 == sha256Hex(rawAttestation) &&
		repair.Challenge == receipt.Challenge && repair.Evaluator == receipt.Evaluator &&
		repair.EvaluatorRunID == receipt.EvaluatorRunID && repair.Head == receipt.Head &&
		repair.PR == receipt.PR && repair.ReportSHA256 == sha256Hex(canonicalReport) &&
		repair.Round == receipt.Round && repair.Verdict == receipt.Verdict &&
		repair.OriginalCommentSHA256 == sha256Hex([]byte(record.comment.Body)) &&
		repair.ReceiptMarkerSHA256 == sha256Hex(record.marker) &&
		attestation.PR == repair.PR && attestation.Head == repair.Head
}

func repairFollowsReceipt(record evaluationReceiptRecord, repair evaluationRepairRecord) bool {
	if repair.commentIndex <= record.commentIndex {
		if repair.commentIndex != 0 || record.commentIndex != 0 {
			return false
		}
		return repair.comment.CreatedAt.After(record.comment.CreatedAt)
	}
	return !repair.comment.CreatedAt.Before(record.comment.CreatedAt)
}

func visibleEvaluationReport(body string) ([]byte, bool) {
	_, report, ok := strings.Cut(body, evaluationReceiptHeading)
	if !ok {
		return nil, false
	}
	report = string(canonicalEvaluationReport(report))
	if report == "" {
		return nil, false
	}
	return []byte(report), true
}

func parseEvaluationReport(body string) ([]byte, bool, bool) {
	if !hasMarker(body, evaluationReportBase64Marker) {
		return nil, false, false
	}
	value, ok := markerBytes(body, evaluationReportBase64Marker)
	if !ok {
		return nil, true, false
	}
	decoded, ok := decodeBase64Marker(value)
	if !ok || len(decoded) == 0 {
		return nil, true, false
	}
	return decoded, true, true
}

func decodeBase64Marker(value []byte) ([]byte, bool) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(value) {
		return nil, false
	}
	return decoded, true
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("JSON marker contains more than one value")
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanEvaluationJSONValue(decoder); err != nil {
		return fmt.Errorf("scan evaluation JSON: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return fmt.Errorf("scan evaluation JSON trailer: %w", err)
	}
	return nil
}

func scanEvaluationJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return scanEvaluationJSONObject(decoder)
	case '[':
		return scanEvaluationJSONArray(decoder)
	default:
		return fmt.Errorf("unexpected evaluation JSON delimiter %q", delimiter)
	}
}

func scanEvaluationJSONObject(decoder *json.Decoder) error {
	var keys []string
	for decoder.More() {
		key, err := scanEvaluationJSONKey(decoder)
		if err != nil {
			return err
		}
		for _, seen := range keys {
			if strings.EqualFold(seen, key) {
				return fmt.Errorf("evaluation JSON object key %q is duplicated", key)
			}
		}
		keys = append(keys, key)
		if err := scanEvaluationJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return errors.New("evaluation JSON object is not closed")
	}
	return nil
}

func scanEvaluationJSONArray(decoder *json.Decoder) error {
	for decoder.More() {
		if err := scanEvaluationJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim(']') {
		return errors.New("evaluation JSON array is not closed")
	}
	return nil
}

func scanEvaluationJSONKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	key, ok := token.(string)
	if !ok {
		return "", errors.New("evaluation JSON object key is not a string")
	}
	return key, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func latestEvaluationPasses(view pullRequestView, number int) (bool, error) {
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return false, err
	}
	if err := validateEvaluationHistory(history); err != nil {
		return false, err
	}
	if len(history.receipts) == 0 {
		return false, nil
	}
	latest := history.receipts[len(history.receipts)-1].receipt
	if latest.AttestationSHA256 == "" || latest.Head != view.HeadRefOID || latest.PR != number ||
		latest.Verdict != "pass" {
		return false, nil
	}
	if err := evaluationReceiptMatchesCurrentPR(latest, view); err != nil {
		return false, err
	}
	uses := 0
	runUses := 0
	for _, record := range history.receipts {
		receipt := record.receipt
		if receipt.Challenge == latest.Challenge {
			uses++
		}
		if receipt.EvaluatorRunID == latest.EvaluatorRunID {
			runUses++
		}
	}
	return uses == 1 && runUses == 1, nil
}

func latestPassingEvaluationReceipt(view pullRequestView, number int) (evaluationReceipt, error) {
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return evaluationReceipt{}, err
	}
	if err := validateEvaluationHistory(history); err != nil {
		return evaluationReceipt{}, err
	}
	if len(history.receipts) == 0 {
		return evaluationReceipt{}, errors.New("no trusted evaluation receipt")
	}
	latest := history.receipts[len(history.receipts)-1].receipt
	if latest.AttestationSHA256 == "" || latest.Head != view.HeadRefOID || latest.PR != number || latest.Verdict != "pass" {
		return evaluationReceipt{}, fmt.Errorf("latest trusted evaluation receipt is not a passing proof for the current head (receipt head=%q PR=%d verdict=%q, current head=%q PR=%d)", latest.Head, latest.PR, latest.Verdict, view.HeadRefOID, number)
	}
	if !hasEvaluationReceiptMetadata(latest) {
		return evaluationReceipt{}, errors.New("latest passing evaluation lacks immutable PR metadata; request a fresh challenge-bound Examiner attestation")
	}
	if len(latest.ClosingIssues) > 1 && latest.ClaimProofs == nil {
		return evaluationReceipt{}, errors.New("latest passing evaluation lacks immutable companion claim proof; request a fresh challenge-bound Examiner attestation")
	}
	if err := evaluationReceiptMatchesCurrentPR(latest, view); err != nil {
		return evaluationReceipt{}, err
	}
	if err := evaluationReceiptMatchesCurrentEvidence(latest, view); err != nil {
		return evaluationReceipt{}, err
	}
	return latest, nil
}

func evaluationReceiptMatchesCurrentPR(receipt evaluationReceipt, view pullRequestView) error {
	if !hasEvaluationReceiptMetadata(receipt) {
		return nil
	}
	if receipt.BaseRefName != view.BaseRefName {
		return errors.New("latest passing evaluation does not match current PR base; request a fresh challenge-bound Examiner attestation")
	}
	if receipt.HeadRefName != view.HeadRefName {
		return errors.New("latest passing evaluation does not match current PR head ref; request a fresh challenge-bound Examiner attestation")
	}
	if !sameIssueNumbers(receipt.ClosingIssues, closingIssueNumbers(view.Body)) {
		return errors.New("latest passing evaluation does not match current PR closure; request a fresh challenge-bound Examiner attestation")
	}
	if receipt.BodySHA256 != sha256Hex([]byte(view.Body)) {
		return errors.New("latest passing evaluation does not match current PR body; request a fresh challenge-bound Examiner attestation")
	}
	return nil
}

func evaluationReceiptMatchesCurrentEvidence(receipt evaluationReceipt, view pullRequestView) error {
	if !hasCurrentPREvidence(view) {
		return nil
	}
	parsed, err := validatePREvidenceForView(view)
	if err != nil {
		return fmt.Errorf("current PR evidence is invalid: %w", err)
	}
	if receipt.EvidenceSHA256 == "" {
		return errors.New("latest passing evaluation lacks evidence digest for the current PR; request a fresh challenge-bound Examiner attestation")
	}
	_, evidenceSHA256 := currentPREvidenceDigest(view, parsed)
	if receipt.EvidenceSHA256 != evidenceSHA256 {
		return errors.New("latest passing evaluation does not match current PR evidence; request a fresh challenge-bound Examiner attestation")
	}
	return nil
}

func evaluationReceiptMatchesRecord(record evaluationReceiptRecord, repairs []evaluationRepairRecord) bool {
	comment := record.comment
	receipt := record.receipt
	if !commentTimeMatches(comment.CreatedAt, receipt.RecordedAt) {
		return false
	}
	_, _, canonicalReport, attestationOK := receiptAttestation(record)
	if !attestationOK {
		return false
	}
	reportMarker, hasReportMarker, reportMarkerOK := parseEvaluationReport(comment.Body)
	if hasReportMarker && !reportMarkerOK {
		return false
	}
	if hasReportMarker {
		return evaluationReportMatchesRecord(record, nil, canonicalReport, reportMarker,
			hasReportMarker, reportMarkerOK, repairs)
	}
	visibleReport, reportOK := visibleEvaluationReport(comment.Body)
	if !reportOK {
		return false
	}
	return evaluationReportMatchesRecord(record, visibleReport, canonicalReport, reportMarker,
		hasReportMarker, reportMarkerOK, repairs)
}

func evaluationReportMatchesRecord(record evaluationReceiptRecord, visibleReport, canonicalReport,
	reportMarker []byte, hasReportMarker, reportMarkerOK bool, repairs []evaluationRepairRecord) bool {
	receipt := record.receipt
	if receipt.ReportTransport == evaluationReportTransportV1 && (!hasReportMarker || !reportMarkerOK) {
		return false
	}
	if hasReportMarker {
		return evaluationReportMarkerMatches(receipt, canonicalReport, reportMarker)
	}
	if receipt.ReportTransport == evaluationReportTransportV1 {
		return false
	}
	if sha256Hex(visibleReport) == receipt.ReportSHA256 {
		return receipt.AttestationSHA256 == "" || bytes.Equal(visibleReport, canonicalReport)
	}
	if receipt.AttestationSHA256 == "" || sha256Hex(canonicalReport) != receipt.ReportSHA256 {
		return false
	}
	if len(repairs) == 0 {
		return false
	}
	matchingRepairs := 0
	for _, repair := range repairs {
		if evaluationRepairMatchesRecord(record, repair) {
			matchingRepairs++
		}
	}
	return matchingRepairs == 1
}

func evaluationReportMarkerMatches(receipt evaluationReceipt, canonicalReport, reportMarker []byte) bool {
	if sha256Hex(reportMarker) != receipt.ReportSHA256 {
		return false
	}
	if receipt.AttestationSHA256 != "" && !bytes.Equal(reportMarker, canonicalReport) {
		return false
	}
	return true
}

func parseCommentAttestation(body string) (evaluationAttestation, []byte, bool) {
	base64Value, hasBase64 := markerBytes(body, evaluationAttestationBase64Marker)
	plainValue, hasPlain := markerBytes(body, evaluationAttestationMarker)
	if hasBase64 == hasPlain {
		return evaluationAttestation{}, nil, false
	}
	value := plainValue
	if hasBase64 {
		decoded, ok := decodeBase64Marker(base64Value)
		if !ok {
			return evaluationAttestation{}, nil, false
		}
		value = decoded
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationAttestation{}, nil, false
	}
	decoder.DisallowUnknownFields()
	var attestation evaluationAttestation
	if err := decoder.Decode(&attestation); err != nil {
		return evaluationAttestation{}, nil, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationAttestation{}, nil, false
	}
	return attestation, value, true
}

func (a app) transitionIssueToNeedsHuman(root string, number int) error {
	if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(number), "--repo", repositoryKey,
		"--add-label", "needs-human"); err != nil {
		return fmt.Errorf("needs-human label phase incomplete; retry: mark issue #%d needs-human: %w", number, err)
	}
	if err := a.setIssueProjectStatus(root, number, "Backlog"); err != nil {
		return fmt.Errorf("project Backlog phase incomplete after needs-human label; retry: %w", err)
	}
	return nil
}

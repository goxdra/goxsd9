package workflowctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func evaluationChallengeKeyFor(challenge evaluationChallenge) (evaluationChallengeKey, bool) {
	if challenge.Repository != repositoryKey || challenge.PR < 1 || challenge.Head == "" ||
		!validSHA256(challenge.BodySHA256) || !validSHA256(challenge.EvidenceSHA256) {
		return evaluationChallengeKey{}, false
	}
	return evaluationChallengeKey{
		repository:     challenge.Repository,
		pr:             challenge.PR,
		head:           challenge.Head,
		bodySHA256:     challenge.BodySHA256,
		evidenceSHA256: challenge.EvidenceSHA256,
	}, true
}

// evaluationChallengeHistoryPR identifies the single PR represented by an
// already authenticated challenge history. A history fetched for one PR is
// the only context in which a legacy marker may omit its repository field.
func evaluationChallengeHistoryPR(history evaluationHistory) (int, bool) {
	var number int
	for _, record := range history.challenges {
		challenge := record.challenge
		if record.comment.Author.Login != trustedActor || challenge.PR < 1 ||
			(challenge.Repository != "" && challenge.Repository != repositoryKey) {
			return 0, false
		}
		if number == 0 {
			number = challenge.PR
			continue
		}
		if challenge.PR != number {
			return 0, false
		}
	}
	return number, number > 0
}

// evaluationChallengeKeyForPR keeps the strict key primitive intact while
// accepting digest-bound legacy markers only in one known PR context.
func evaluationChallengeKeyForPR(challenge evaluationChallenge, number int, contextOK bool) (
	evaluationChallengeKey, bool) {
	if challenge.Repository == repositoryKey {
		return evaluationChallengeKeyFor(challenge)
	}
	if challenge.Repository != "" || !contextOK || challenge.PR != number {
		return evaluationChallengeKey{}, false
	}
	normalized := challenge
	normalized.Repository = repositoryKey
	return evaluationChallengeKeyFor(normalized)
}

func evaluationChallengeKeyForHistory(history evaluationHistory, challenge evaluationChallenge) (
	evaluationChallengeKey, bool) {
	number, contextOK := evaluationChallengeHistoryPR(history)
	return evaluationChallengeKeyForPR(challenge, number, contextOK)
}

// evaluationChallengeForHistory returns a temporary repository-bound copy.
// The source record and its original comment remain unchanged.
func evaluationChallengeForHistory(history evaluationHistory, challenge evaluationChallenge) (
	evaluationChallenge, bool) {
	key, complete := evaluationChallengeKeyForHistory(history, challenge)
	if !complete {
		return evaluationChallenge{}, false
	}
	normalized := challenge
	normalized.Repository = key.repository
	return normalized, true
}

func equalEvaluationChallengeKeys(left, right evaluationChallengeKey) bool {
	return left == right
}

func compareEvaluationChallengeHistoryOrder(left, right evaluationChallengeRecord) (int, error) {
	if left.commentIndex < right.commentIndex {
		return -1, nil
	}
	if left.commentIndex > right.commentIndex {
		return 1, nil
	}
	if left.comment.CreatedAt.Before(right.comment.CreatedAt) {
		return -1, nil
	}
	if left.comment.CreatedAt.After(right.comment.CreatedAt) {
		return 1, nil
	}
	if left.comment.ID > 0 && right.comment.ID > 0 && left.comment.ID != right.comment.ID {
		if left.comment.ID < right.comment.ID {
			return -1, nil
		}
		return 1, nil
	}
	if left.commentIndex == right.commentIndex &&
		(left.comment.ID == right.comment.ID || left.comment.ID == 0 || right.comment.ID == 0) {
		return 0, errors.New("equivalent evaluation challenges have ambiguous canonical comment ordering")
	}
	return 0, nil
}

func orderedEvaluationChallengeRecords(records []evaluationChallengeRecord) (
	[]evaluationChallengeRecord, error) {
	ordered := make([]evaluationChallengeRecord, 0, len(records))
	for _, record := range records {
		for _, current := range ordered {
			if record.comment.ID > 0 && record.comment.ID == current.comment.ID {
				return nil, errors.New("trusted evaluation challenges reuse a GitHub comment ID")
			}
		}
		insertAt := len(ordered)
		for index, current := range ordered {
			comparison, err := compareEvaluationChallengeHistoryOrder(record, current)
			if err != nil {
				return nil, err
			}
			if comparison < 0 {
				insertAt = index
				break
			}
		}
		ordered = append(ordered, evaluationChallengeRecord{})
		copy(ordered[insertAt+1:], ordered[insertAt:])
		ordered[insertAt] = record
	}
	return ordered, nil
}

func (projection evaluationLogicalProjection) challengeForID(challengeID string) (
	evaluationLogicalChallenge, bool) {
	var found evaluationLogicalChallenge
	matches := 0
	for _, challenge := range projection.challenges {
		for _, member := range challenge.members {
			if member.challenge.Challenge != challengeID {
				continue
			}
			found = challenge
			matches++
			break
		}
	}
	return found, matches == 1
}

// evaluationChallengeOnlyProjectionForHistory projects challenge identities and
// controller closures without requiring equivalent receipt groups to have
// converged. Terminal conflicts are still rejected before any mutation.
func evaluationChallengeOnlyProjectionForHistory(history evaluationHistory) (
	evaluationLogicalProjection, error) {
	if err := validateEvaluationTerminalConflicts(history); err != nil {
		return evaluationLogicalProjection{}, err
	}
	return evaluationLogicalProjectionForHistoryMode(history, false, true)
}

func evaluationLogicalChallengeHasID(challenge evaluationLogicalChallenge, challengeID string) bool {
	for _, member := range challenge.members {
		if member.challenge.Challenge == challengeID {
			return true
		}
	}
	return false
}

func hasChallengeClosureForID(challenge evaluationLogicalChallenge, challengeID string) bool {
	_, ok := evaluationChallengeClosureForID(challenge, challengeID)
	return ok
}

func evaluationChallengeClosureForID(challenge evaluationLogicalChallenge, challengeID string) (
	evaluationChallengeClosure, bool) {
	var found evaluationChallengeClosure
	matches := 0
	for _, closure := range challenge.closures {
		if closure.closure.DuplicateChallenge != challengeID {
			continue
		}
		found = closure.closure
		matches++
	}
	return found, matches == 1
}

func evaluationChallengeRecordForID(records []evaluationChallengeRecord, challengeID string) (
	evaluationChallengeRecord, bool) {
	var found evaluationChallengeRecord
	matches := 0
	for _, record := range records {
		if record.challenge.Challenge != challengeID {
			continue
		}
		found = record
		matches++
	}
	return found, matches == 1
}

func evaluationStatusChallengeForLogical(challenge evaluationChallengeRecord,
	logical evaluationLogicalChallenge) evaluationStatusChallenge {
	status := evaluationStatusChallenge{
		challenge:          challenge,
		logicalCanonical:   challenge.challenge.Challenge == logical.canonical.challenge.Challenge,
		logicalOutstanding: !logical.hasReceipt && !logical.hasResolution,
	}
	if logical.hasReceipt {
		status.resolvedReceipt = logical.receipt.receipt
	}
	if logical.hasResolution {
		status.resolvedResolution = logical.resolution.resolution
	}
	if !status.logicalCanonical {
		if closure, ok := evaluationChallengeClosureForID(logical, challenge.challenge.Challenge); ok {
			status.resolvedClosure = closure
			status.resolvedByClosure = true
			status.resolved = true
		}
	}
	if logical.hasReceipt || logical.hasResolution {
		status.resolved = true
		status.resolvedByResolution = logical.hasResolution && !status.resolvedByClosure
	}
	return status
}

func evaluationChallengeClosureMatchesRecord(record evaluationChallengeClosureRecord,
	closure evaluationChallengeClosure) bool {
	return record.closure.Schema == closure.Schema &&
		record.closure.Repository == closure.Repository &&
		record.closure.PR == closure.PR &&
		record.closure.Head == closure.Head &&
		record.closure.BodySHA256 == closure.BodySHA256 &&
		record.closure.EvidenceSHA256 == closure.EvidenceSHA256 &&
		record.closure.CanonicalChallenge == closure.CanonicalChallenge &&
		record.closure.DuplicateChallenge == closure.DuplicateChallenge &&
		record.closure.Controller == closure.Controller &&
		record.closure.ClosedAt.Equal(closure.ClosedAt)
}

//nolint:gocognit // Group complete identities while retaining chronological lifecycle boundaries.
func evaluationLogicalChallengeGroups(history evaluationHistory) (
	[]evaluationLogicalChallenge, error) {
	ordered, err := orderedEvaluationChallengeRecords(history.challenges)
	if err != nil {
		return nil, err
	}
	historyPR, contextOK := evaluationChallengeHistoryPR(history)
	groups := make([]evaluationLogicalChallenge, 0, len(ordered))
	for _, record := range ordered {
		key, complete := evaluationChallengeKeyForPR(record.challenge, historyPR, contextOK)
		groupIndex := -1
		if complete {
			for index := range groups {
				if !groups[index].keyComplete || !equalEvaluationChallengeKeys(groups[index].key, key) {
					continue
				}
				if evaluationChallengeGroupHasTerminalBefore(history, groups[index], record.commentIndex) {
					continue
				}
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			groups = append(groups, evaluationLogicalChallenge{
				key:         key,
				keyComplete: complete,
				canonical:   record,
				members:     []evaluationChallengeRecord{record},
			})
			continue
		}
		groups[groupIndex].members = append(groups[groupIndex].members, record)
	}
	return groups, nil
}

//nolint:gocognit // Inspect each terminal mutation in physical comment order.
func evaluationChallengeGroupHasTerminalBefore(history evaluationHistory,
	group evaluationLogicalChallenge, commentIndex int) bool {
	for _, member := range group.members {
		for _, receipt := range history.receipts {
			if receipt.receipt.AttestationSHA256 == "" || receipt.commentIndex >= commentIndex {
				continue
			}
			if evaluationChallengeMatchesReceiptForHistory(history, member, receipt) {
				return true
			}
		}
		for _, resolution := range history.resolutions {
			if resolution.commentIndex >= commentIndex {
				continue
			}
			if evaluationChallengeMatchesResolutionForHistory(history, member, resolution) {
				return true
			}
		}
	}
	return false
}

func evaluationChallengeMatchesReceiptForHistory(history evaluationHistory,
	challenge evaluationChallengeRecord, receipt evaluationReceiptRecord) bool {
	normalizedChallenge, complete := evaluationChallengeForHistory(history, challenge.challenge)
	if !complete {
		return evaluationChallengeMatchesReceipt(challenge, receipt)
	}
	normalizedReceipt := receipt.receipt
	if normalizedReceipt.Repository == "" {
		normalizedReceipt.Repository = repositoryKey
	}
	normalizedChallengeRecord := challenge
	normalizedChallengeRecord.challenge = normalizedChallenge
	normalizedReceiptRecord := receipt
	normalizedReceiptRecord.receipt = normalizedReceipt
	return evaluationChallengeMatchesReceipt(normalizedChallengeRecord, normalizedReceiptRecord)
}

func evaluationChallengeMatchesResolutionForHistory(history evaluationHistory,
	challenge evaluationChallengeRecord, resolution evaluationResolutionRecord) bool {
	normalizedChallenge, complete := evaluationChallengeForHistory(history, challenge.challenge)
	if !complete {
		return evaluationChallengeMatchesResolution(challenge, resolution)
	}
	normalizedResolution := resolution.resolution
	if normalizedResolution.Repository == "" {
		normalizedResolution.Repository = repositoryKey
	}
	normalizedChallengeRecord := challenge
	normalizedChallengeRecord.challenge = normalizedChallenge
	normalizedResolutionRecord := resolution
	normalizedResolutionRecord.resolution = normalizedResolution
	return evaluationChallengeMatchesResolution(normalizedChallengeRecord, normalizedResolutionRecord)
}

func evaluationLogicalProjectionForHistory(history evaluationHistory, requireClosures bool) (
	evaluationLogicalProjection, error) {
	return evaluationLogicalProjectionForHistoryMode(history, requireClosures, false)
}

//nolint:funlen,gocognit // Validate and project all authenticated challenge lifecycle records together.
func evaluationLogicalProjectionForHistoryMode(history evaluationHistory, requireClosures bool,
	challengeOnly bool,
) (evaluationLogicalProjection, error) {
	groups, err := evaluationLogicalChallengeGroups(history)
	if err != nil {
		return evaluationLogicalProjection{}, err
	}
	historyPR, contextOK := evaluationChallengeHistoryPR(history)
	for _, closure := range history.closures {
		canonical, canonicalOK := evaluationChallengeRecordForID(history.challenges,
			closure.closure.CanonicalChallenge)
		duplicate, duplicateOK := evaluationChallengeRecordForID(history.challenges,
			closure.closure.DuplicateChallenge)
		if !canonicalOK || !duplicateOK {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure references a non-unique trusted challenge")
		}
		canonicalKey, canonicalComplete := evaluationChallengeKeyForPR(canonical.challenge, historyPR, contextOK)
		duplicateKey, duplicateComplete := evaluationChallengeKeyForPR(duplicate.challenge, historyPR, contextOK)
		if !canonicalComplete || !duplicateComplete ||
			!equalEvaluationChallengeKeys(canonicalKey, duplicateKey) {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure references non-equivalent challenge identities")
		}
		groupIndex := -1
		for index := range groups {
			if !evaluationLogicalChallengeHasID(groups[index], canonical.challenge.Challenge) {
				continue
			}
			if groupIndex != -1 {
				return evaluationLogicalProjection{}, errors.New(
					"evaluation challenge belongs to more than one logical group")
			}
			groupIndex = index
		}
		if groupIndex < 0 || !evaluationLogicalChallengeHasID(groups[groupIndex],
			duplicate.challenge.Challenge) {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure does not bind one logical challenge group")
		}
		group := &groups[groupIndex]
		if group.canonical.challenge.Challenge != canonical.challenge.Challenge {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure does not identify the earliest canonical challenge")
		}
		if canonical.challenge.Challenge == duplicate.challenge.Challenge ||
			duplicate.commentIndex <= canonical.commentIndex ||
			closure.commentIndex <= duplicate.commentIndex ||
			closure.comment.CreatedAt.IsZero() ||
			closure.closure.ClosedAt.Before(duplicate.comment.CreatedAt) ||
			closure.closure.ClosedAt.Before(duplicate.challenge.RequestedAt) ||
			!commentTimeMatches(closure.comment.CreatedAt, closure.closure.ClosedAt) {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure has invalid deterministic comment order")
		}
		if closure.closure.Repository != canonicalKey.repository ||
			closure.closure.PR != canonicalKey.pr ||
			closure.closure.Head != canonicalKey.head ||
			closure.closure.BodySHA256 != canonicalKey.bodySHA256 ||
			closure.closure.EvidenceSHA256 != canonicalKey.evidenceSHA256 {
			return evaluationLogicalProjection{}, errors.New(
				"evaluation challenge closure does not bind the exact challenge identity")
		}
		for _, existing := range group.closures {
			if existing.closure.DuplicateChallenge == closure.closure.DuplicateChallenge {
				return evaluationLogicalProjection{}, fmt.Errorf(
					"evaluation challenge %q has multiple authenticated controller closures",
					duplicate.challenge.Challenge)
			}
		}
		group.closures = append(group.closures, closure)
	}
	for _, group := range groups {
		if !group.keyComplete || !requireClosures {
			continue
		}
		for _, member := range group.members[1:] {
			if !hasChallengeClosureForID(group, member.challenge.Challenge) {
				return evaluationLogicalProjection{}, fmt.Errorf(
					"equivalent evaluation challenge %q has no authenticated controller closure",
					member.challenge.Challenge)
			}
		}
	}
	if challengeOnly {
		return evaluationLogicalProjection{challenges: groups}, nil
	}
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return evaluationLogicalProjection{}, err
	}
	for index := range groups {
		for _, member := range groups[index].members {
			for _, receipt := range receipts {
				if receipt.receipt.AttestationSHA256 == "" ||
					!evaluationChallengeMatchesReceiptForHistory(history, member, receipt) {
					continue
				}
				if groups[index].hasReceipt {
					return evaluationLogicalProjection{}, errors.New(
						"logical evaluation challenge has multiple matching trusted receipts")
				}
				groups[index].hasReceipt = true
				groups[index].receipt = receipt
			}
			for _, resolution := range history.resolutions {
				if !evaluationChallengeMatchesResolutionForHistory(history, member, resolution) {
					continue
				}
				if groups[index].hasResolution {
					return evaluationLogicalProjection{}, errors.New(
						"logical evaluation challenge has multiple matching no-verdict resolutions")
				}
				groups[index].hasResolution = true
				groups[index].resolution = resolution
			}
		}
		if groups[index].hasReceipt && groups[index].hasResolution {
			return evaluationLogicalProjection{}, errors.New(
				"logical evaluation challenge has both an attested receipt and a no-verdict resolution")
		}
	}
	return evaluationLogicalProjection{challenges: groups}, nil
}

func readEvaluationMutationHistoryForConvergence(number int, comments []pullRequestComment) (
	evaluationHistory, error) {
	if err := rejectUntrustedEvaluationEvidence(comments); err != nil {
		return evaluationHistory{}, fmt.Errorf("untrusted evaluation evidence: %w", err)
	}
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return evaluationHistory{}, err
	}
	if err := validateEvaluationHistoryForConvergence(history); err != nil {
		return history, err
	}
	if err := validateEvaluationStatusHistoryForConvergence(number, history); err != nil {
		return history, err
	}
	return history, nil
}

func (a app) readEvaluationChallengeState(root string, number int, expected evaluationChallenge) (
	pullRequestView, evaluationHistory, error) {
	if expected.Repository != repositoryKey || expected.PR != number {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf(
			"PR #%d challenge has an invalid repository or PR identity", number)
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	if view.State != "OPEN" {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("PR #%d is %s", number, view.State)
	}
	history, err := readEvaluationMutationHistoryForConvergence(number, view.Comments)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf(
			"PR #%d has invalid evaluation history after challenge POST: %w", number, err)
	}
	record, ok := evaluationChallengeByID(history, expected.Challenge)
	if !ok {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf(
			"PR #%d challenge POST was not authenticated as exactly one trusted challenge %s",
			number, expected.Challenge)
	}
	if record.challenge.Challenge != expected.Challenge ||
		record.challenge.Repository != expected.Repository ||
		record.challenge.PR != expected.PR || record.challenge.Head != expected.Head ||
		record.challenge.BodySHA256 != expected.BodySHA256 ||
		record.challenge.EvidenceSHA256 != expected.EvidenceSHA256 ||
		!record.challenge.RequestedAt.Equal(expected.RequestedAt) {
		return pullRequestView{}, evaluationHistory{}, errors.New(
			"authenticated challenge differs from the exact posted challenge identity")
	}
	marker, err := jsonMarshalEvaluationChallenge(expected)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	expectedBody := "<!-- " + evaluationChallengeMarker + string(marker) + " -->\nExaminer challenge for " +
		string([]byte{96}) + expected.Head + string([]byte{96}) + ".\n"
	if record.comment.Body != expectedBody {
		return pullRequestView{}, evaluationHistory{}, errors.New(
			"authenticated challenge comment differs from the exact posted body")
	}
	if err := validateEvaluationChallengeView(view, number, expected); err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	return view, history, nil
}

func jsonMarshalEvaluationChallenge(challenge evaluationChallenge) ([]byte, error) {
	marker, err := json.Marshal(challenge)
	if err != nil {
		return nil, fmt.Errorf("encode evaluation challenge: %w", err)
	}
	return marker, nil
}

func validateEvaluationChallengeView(view pullRequestView, number int,
	challenge evaluationChallenge) error {
	if view.State != "OPEN" {
		return fmt.Errorf("PR #%d is %s", number, view.State)
	}
	if view.HeadRefOID != challenge.Head {
		return fmt.Errorf("PR #%d head changed from challenge %s to %s", number,
			challenge.Head, view.HeadRefOID)
	}
	parsedEvidence, err := validatePREvidenceForView(view)
	if err != nil {
		return fmt.Errorf("validate PR #%d evidence after challenge POST: %w", number, err)
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	if bodySHA256 != challenge.BodySHA256 || evidenceSHA256 != challenge.EvidenceSHA256 {
		return fmt.Errorf("PR #%d body or evidence changed after challenge POST", number)
	}
	return nil
}

func evaluationChallengeKeyForView(number int, view pullRequestView) (evaluationChallengeKey, bool) {
	parsedEvidence, err := validatePREvidenceForView(view)
	if err != nil {
		return evaluationChallengeKey{}, false
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	return evaluationChallengeKeyFor(evaluationChallenge{
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		Head:           view.HeadRefOID,
		PR:             number,
		Repository:     repositoryKey,
	})
}

func (a app) convergeEvaluationChallengeClosures(root string, number int,
	challenge evaluationChallenge, view pullRequestView, history evaluationHistory) error {
	return a.convergeEvaluationChallengeClosuresMode(root, number, challenge, view, history, true)
}

func (a app) convergeEvaluationChallengeClosuresByIdentity(root string, number int,
	challenge evaluationChallenge, view pullRequestView, history evaluationHistory) error {
	return a.convergeEvaluationChallengeClosuresMode(root, number, challenge,
		view, history, false)
}

//nolint:funlen,gocognit // Keep challenge convergence's read, mutation, and verification boundary explicit.
func (a app) convergeEvaluationChallengeClosuresMode(root string, number int,
	challenge evaluationChallenge, view pullRequestView, history evaluationHistory,
	validateCurrentView bool) error {
	if challenge.PR != number {
		return fmt.Errorf("PR #%d challenge has no authenticated PR identity", number)
	}
	if challenge.Repository != repositoryKey {
		normalized, ok := evaluationChallengeForHistory(history, challenge)
		if !ok {
			return fmt.Errorf("PR #%d challenge has no authenticated repository identity", number)
		}
		challenge = normalized
	}
	for attempt := 0; attempt < 100; attempt++ {
		if validateCurrentView {
			if err := validateEvaluationChallengeView(view, number, challenge); err != nil {
				return err
			}
		}
		projection, err := evaluationChallengeOnlyProjectionForHistory(history)
		if err != nil {
			return fmt.Errorf("PR #%d has invalid logical evaluation history: %w", number, err)
		}
		logical, ok := projection.challengeForID(challenge.Challenge)
		if !ok {
			return fmt.Errorf("PR #%d challenge %q is not in authenticated logical history",
				number, challenge.Challenge)
		}
		if !logical.keyComplete {
			return fmt.Errorf("PR #%d challenge %q lacks complete equivalence identity",
				number, challenge.Challenge)
		}
		var duplicate evaluationChallengeRecord
		foundDuplicate := false
		for _, member := range logical.members {
			if member.challenge.Challenge == logical.canonical.challenge.Challenge ||
				hasChallengeClosureForID(logical, member.challenge.Challenge) {
				continue
			}
			duplicate = member
			foundDuplicate = true
			break
		}
		if !foundDuplicate {
			return nil
		}
		closedAt := time.Now().UTC().Truncate(time.Second)
		if closedAt.Before(duplicate.comment.CreatedAt) {
			closedAt = duplicate.comment.CreatedAt
		}
		if closedAt.Before(duplicate.challenge.RequestedAt) {
			closedAt = duplicate.challenge.RequestedAt
		}
		closure := evaluationChallengeClosure{
			BodySHA256:         logical.key.bodySHA256,
			CanonicalChallenge: logical.canonical.challenge.Challenge,
			ClosedAt:           closedAt,
			Controller:         trustedActor,
			DuplicateChallenge: duplicate.challenge.Challenge,
			EvidenceSHA256:     logical.key.evidenceSHA256,
			Head:               logical.key.head,
			PR:                 logical.key.pr,
			Repository:         logical.key.repository,
			Schema:             evaluationChallengeClosureSchema,
		}
		marker, err := json.Marshal(closure)
		if err != nil {
			return fmt.Errorf("encode evaluation challenge closure: %w", err)
		}
		body := evaluationChallengeClosureComment(marker, closure.CanonicalChallenge,
			closure.DuplicateChallenge)
		generated := append(append([]pullRequestComment(nil), view.Comments...), pullRequestComment{
			Body:      body,
			CreatedAt: closedAt,
		})
		generated[len(generated)-1].Author.Login = trustedActor
		if _, err := readEvaluationMutationHistoryForConvergence(number, generated); err != nil {
			return fmt.Errorf("generated evaluation challenge closure is not authenticated: %w", err)
		}
		postErr := a.postPullRequestComment(root, number, body)
		verifiedView, readErr := a.readPullRequest(root, number)
		if readErr != nil {
			return fmt.Errorf("challenge closure POST could not be verified; preserve history and retry after inspection: %w",
				errors.Join(postErr, readErr))
		}
		verifiedHistory, historyErr := readEvaluationMutationHistoryForConvergence(number,
			verifiedView.Comments)
		if historyErr != nil {
			return fmt.Errorf("challenge closure POST produced unverifiable history; preserve history and retry after inspection: %w",
				errors.Join(postErr, historyErr))
		}
		verified := 0
		for _, record := range verifiedHistory.closures {
			if evaluationChallengeClosureMatchesRecord(record, closure) &&
				record.comment.Body == body {
				verified++
			}
		}
		if verified != 1 {
			err := fmt.Errorf("authenticated challenge closure count is %d; want exactly one", verified)
			if postErr != nil {
				return fmt.Errorf("challenge closure POST response was ambiguous; do not repost blindly: %w",
					errors.Join(postErr, err))
			}
			return fmt.Errorf("challenge closure POST was not authenticated exactly once in complete history: %w", err)
		}
		if postErr != nil {
			return fmt.Errorf("challenge closure POST response was ambiguous; do not repost blindly, retry the exact recording command after inspection: %w",
				postErr)
		}
		if validateCurrentView {
			if err := validateEvaluationChallengeView(verifiedView, number, challenge); err != nil {
				return fmt.Errorf("challenge closure POST changed the evaluated PR: %w",
					errors.Join(postErr, err))
			}
		}
		view = verifiedView
		history = verifiedHistory
	}
	return fmt.Errorf("PR #%d challenge convergence exceeded the bounded closure pass; preserve history and retry after inspection",
		number)
}

//nolint:gocognit // Select and converge each outstanding equivalent group in order.
func (a app) convergeEvaluationChallengeHistory(root string, number int,
	view pullRequestView, history evaluationHistory) (pullRequestView, evaluationHistory, error) {
	for attempt := 0; attempt < 100; attempt++ {
		projection, err := evaluationChallengeOnlyProjectionForHistory(history)
		if err != nil {
			return pullRequestView{}, evaluationHistory{}, err
		}
		var target evaluationChallenge
		var targetKey evaluationChallengeKey
		found := false
		for _, logical := range projection.challenges {
			if !logical.keyComplete {
				continue
			}
			for _, member := range logical.members[1:] {
				if hasChallengeClosureForID(logical, member.challenge.Challenge) {
					continue
				}
				target = logical.canonical.challenge
				targetKey = logical.key
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return view, history, nil
		}
		currentKey, currentKeyComplete := evaluationChallengeKeyForView(number, view)
		var convergenceErr error
		if currentKeyComplete && equalEvaluationChallengeKeys(currentKey, targetKey) {
			convergenceErr = a.convergeEvaluationChallengeClosures(root, number, target, view, history)
		}
		if !currentKeyComplete || !equalEvaluationChallengeKeys(currentKey, targetKey) {
			convergenceErr = a.convergeEvaluationChallengeClosuresByIdentity(root, number, target, view, history)
		}
		if convergenceErr != nil {
			return pullRequestView{}, evaluationHistory{}, convergenceErr
		}
		view, err = a.readPullRequest(root, number)
		if err != nil {
			return pullRequestView{}, evaluationHistory{}, fmt.Errorf("reread PR #%d after challenge convergence: %w",
				number, err)
		}
		history, err = readEvaluationMutationHistoryForConvergence(number, view.Comments)
		if err != nil {
			return pullRequestView{}, evaluationHistory{}, fmt.Errorf("reread PR #%d challenge history after convergence: %w",
				number, err)
		}
	}
	return pullRequestView{}, evaluationHistory{}, fmt.Errorf(
		"PR #%d challenge convergence exceeded the bounded history pass; preserve history and retry after inspection", number)
}

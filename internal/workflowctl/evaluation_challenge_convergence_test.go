package workflowctl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

//nolint:funlen,gocognit // Exercise projection, strict closure validation, and outstanding state together.
func TestEvaluationLogicalChallengeProjectionConvergesEquivalentHistory(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	bodySHA256 := strings.Repeat("a", 64)
	evidenceSHA256 := strings.Repeat("b", 64)
	first := evaluationChallenge{
		Challenge:      "canonical-challenge",
		Repository:     repositoryKey,
		Head:           "head-203",
		PR:             203,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    base,
	}
	duplicate := first
	duplicate.Challenge = "duplicate-challenge"
	duplicate.RequestedAt = base.Add(time.Second)
	distinctHead := duplicate
	distinctHead.Challenge = "different-head"
	distinctHead.Head = "other-head"
	distinctBody := duplicate
	distinctBody.Challenge = "different-body"
	distinctBody.BodySHA256 = strings.Repeat("c", 64)
	distinctEvidence := duplicate
	distinctEvidence.Challenge = "different-evidence"
	distinctEvidence.EvidenceSHA256 = strings.Repeat("d", 64)
	distinctPR := duplicate
	distinctPR.Challenge = "different-pr"
	distinctPR.PR = 204
	legacy := duplicate
	legacy.Challenge = "legacy-challenge"
	legacy.Repository = ""
	legacy.BodySHA256 = ""
	legacy.EvidenceSHA256 = ""
	comments := []pullRequestComment{
		evaluationChallengeProjectionComment(t, first, 101),
		evaluationChallengeProjectionComment(t, duplicate, 102),
		evaluationChallengeProjectionComment(t, distinctHead, 103),
		evaluationChallengeProjectionComment(t, distinctBody, 104),
		evaluationChallengeProjectionComment(t, distinctEvidence, 105),
		evaluationChallengeProjectionComment(t, distinctPR, 106),
		evaluationChallengeProjectionComment(t, legacy, 107),
	}
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		t.Fatalf("parse challenge history: %v", err)
	}
	projection, err := evaluationLogicalProjectionForHistory(history, false)
	if err != nil {
		t.Fatalf("project challenge history: %v", err)
	}
	if got, want := len(projection.challenges), 6; got != want {
		t.Fatalf("logical challenge groups = %d, want %d", got, want)
	}
	logical, ok := projection.challengeForID(first.Challenge)
	if !ok {
		t.Fatalf("canonical challenge is absent from projection")
	}
	if logical.canonical.challenge.Challenge != first.Challenge || len(logical.members) != 2 {
		t.Fatalf("equivalent logical challenge = %#v, want canonical plus duplicate", logical)
	}
	if _, legacyOK := projection.challengeForID(legacy.Challenge); !legacyOK {
		t.Fatalf("legacy digest-empty challenge is absent from projection")
	}
	if _, distinctOK := projection.challengeForID(distinctHead.Challenge); !distinctOK {
		t.Fatalf("non-equivalent challenge is absent from projection")
	}
	if _, strictErr := evaluationLogicalProjectionForHistory(history, true); strictErr == nil {
		t.Fatal("unclosed equivalent challenge history was accepted in strict mode")
	}

	closureAt := base.Add(2 * time.Minute)
	closure := evaluationChallengeClosure{
		BodySHA256:         bodySHA256,
		CanonicalChallenge: first.Challenge,
		ClosedAt:           closureAt,
		Controller:         trustedActor,
		DuplicateChallenge: duplicate.Challenge,
		EvidenceSHA256:     evidenceSHA256,
		Head:               first.Head,
		PR:                 first.PR,
		Repository:         repositoryKey,
		Schema:             evaluationChallengeClosureSchema,
	}
	marker, err := json.Marshal(closure)
	if err != nil {
		t.Fatalf("encode challenge closure: %v", err)
	}
	closureComment := pullRequestComment{
		Body:      evaluationChallengeClosureComment(marker, first.Challenge, duplicate.Challenge),
		CreatedAt: closureAt,
		ID:        108,
	}
	closureComment.Author.Login = trustedActor
	history, err = parseEvaluationHistory(append(comments, closureComment))
	if err != nil {
		t.Fatalf("parse closed challenge history: %v", err)
	}
	projection, err = evaluationLogicalProjectionForHistory(history, true)
	if err != nil {
		t.Fatalf("project closed challenge history: %v", err)
	}
	logical, logicalOK := projection.challengeForID(first.Challenge)
	if !logicalOK || len(logical.closures) != 1 || logical.hasReceipt || logical.hasResolution {
		t.Fatalf("closed logical challenge = %#v, want one closure and no verdict state", logical)
	}
	outstanding, err := outstandingEvaluationChallenges(history)
	if err != nil {
		t.Fatalf("find logical outstanding challenge: %v", err)
	}
	if len(outstanding) != 6 || outstanding[0].challenge.Challenge != first.Challenge {
		t.Fatalf("logical outstanding challenges = %#v, want canonical and non-equivalent challenges", outstanding)
	}
}

func TestEvaluationChallengeClosureRejectsUntrustedOrMismatchedHistory(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	first := evaluationChallenge{
		Challenge:      "canonical-challenge",
		Repository:     repositoryKey,
		Head:           "head-203",
		PR:             203,
		BodySHA256:     strings.Repeat("a", 64),
		EvidenceSHA256: strings.Repeat("b", 64),
		RequestedAt:    base,
	}
	duplicate := first
	duplicate.Challenge = "duplicate-challenge"
	comments := []pullRequestComment{
		evaluationChallengeProjectionComment(t, first, 101),
		evaluationChallengeProjectionComment(t, duplicate, 102),
	}
	closure := evaluationChallengeClosure{
		BodySHA256:         first.BodySHA256,
		CanonicalChallenge: first.Challenge,
		ClosedAt:           base.Add(time.Minute),
		Controller:         trustedActor,
		DuplicateChallenge: duplicate.Challenge,
		EvidenceSHA256:     first.EvidenceSHA256,
		Head:               first.Head,
		PR:                 first.PR,
		Repository:         repositoryKey,
		Schema:             evaluationChallengeClosureSchema,
	}
	marker, err := json.Marshal(closure)
	if err != nil {
		t.Fatalf("encode challenge closure: %v", err)
	}
	body := evaluationChallengeClosureComment(marker, first.Challenge, duplicate.Challenge)
	untrusted := pullRequestComment{Body: body, CreatedAt: closure.ClosedAt, ID: 109}
	untrusted.Author.Login = "other-user"
	if _, untrustedErr := parseEvaluationHistory(append(comments, untrusted)); untrustedErr == nil {
		t.Fatal("untrusted challenge closure was accepted")
	}
	wrong := closure
	wrong.Head = "other-head"
	marker, err = json.Marshal(wrong)
	if err != nil {
		t.Fatalf("encode mismatched challenge closure: %v", err)
	}
	mismatched := pullRequestComment{
		Body:      evaluationChallengeClosureComment(marker, first.Challenge, duplicate.Challenge),
		CreatedAt: closure.ClosedAt,
		ID:        110,
	}
	mismatched.Author.Login = trustedActor
	history, err := parseEvaluationHistory(append(comments, mismatched))
	if err != nil {
		t.Fatalf("parse mismatched closure history: %v", err)
	}
	if _, err := evaluationLogicalProjectionForHistory(history, false); err == nil {
		t.Fatal("mismatched challenge closure was accepted")
	}
}

func TestEvaluationChallengeReceiptDigestPresenceMustMatch(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	receipt := evaluationReceipt{
		Challenge:      "legacy-challenge",
		Repository:     "",
		PR:             203,
		Head:           "head-203",
		BodySHA256:     strings.Repeat("a", 64),
		EvidenceSHA256: strings.Repeat("b", 64),
		RecordedAt:     base.Add(time.Minute),
	}
	receiptRecord := evaluationReceiptRecord{
		comment:      pullRequestComment{ID: 201, CreatedAt: receipt.RecordedAt},
		commentIndex: 1,
		receipt:      receipt,
	}
	challenge := evaluationChallengeRecord{
		comment:      pullRequestComment{ID: 200, CreatedAt: base},
		commentIndex: 0,
		challenge: evaluationChallenge{
			Challenge:   receipt.Challenge,
			PR:          receipt.PR,
			Head:        receipt.Head,
			RequestedAt: base,
		},
	}
	if evaluationChallengeMatchesReceipt(challenge, receiptRecord) {
		t.Fatal("digest-bound receipt matched a digest-empty legacy challenge")
	}
	challenge.challenge.Repository = repositoryKey
	receiptRecord.receipt.Repository = repositoryKey
	if evaluationChallengeMatchesReceipt(challenge, receiptRecord) {
		t.Fatal("digest-bound receipt matched an incomplete digest-bound challenge")
	}
}

func TestEvaluationChallengeClosureTimeCannotPrecedeDuplicate(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	bodySHA256 := strings.Repeat("a", 64)
	evidenceSHA256 := strings.Repeat("b", 64)
	canonical := evaluationChallenge{
		Challenge:      "canonical-challenge",
		Repository:     repositoryKey,
		Head:           "head-203",
		PR:             203,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    base,
	}
	duplicate := canonical
	duplicate.Challenge = "duplicate-challenge"
	duplicate.RequestedAt = base.Add(time.Minute)
	comments := []pullRequestComment{
		evaluationChallengeProjectionComment(t, canonical, 301),
		evaluationChallengeProjectionComment(t, duplicate, 302),
	}
	closure := evaluationChallengeClosure{
		BodySHA256:         bodySHA256,
		CanonicalChallenge: canonical.Challenge,
		ClosedAt:           base.Add(30 * time.Second),
		Controller:         trustedActor,
		DuplicateChallenge: duplicate.Challenge,
		EvidenceSHA256:     evidenceSHA256,
		Head:               canonical.Head,
		PR:                 canonical.PR,
		Repository:         repositoryKey,
		Schema:             evaluationChallengeClosureSchema,
	}
	marker, err := json.Marshal(closure)
	if err != nil {
		t.Fatalf("encode early challenge closure: %v", err)
	}
	comment := pullRequestComment{
		ID:        303,
		Body:      evaluationChallengeClosureComment(marker, canonical.Challenge, duplicate.Challenge),
		CreatedAt: base.Add(2 * time.Minute),
	}
	comment.Author.Login = trustedActor
	history, err := parseEvaluationHistory(append(comments, comment))
	if err != nil {
		t.Fatalf("parse early challenge closure: %v", err)
	}
	if _, err := evaluationChallengeOnlyProjectionForHistory(history); err == nil {
		t.Fatal("challenge closure before duplicate challenge was accepted")
	}
}

//nolint:gocognit,funlen // Keep receipt and resolution race projections together.
func TestEvaluationTerminalOnClosedDuplicateProjectsToCanonical(t *testing.T) {
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	bodySHA256 := strings.Repeat("a", 64)
	evidenceSHA256 := strings.Repeat("b", 64)
	canonical := evaluationChallenge{
		Challenge:      "canonical-challenge",
		Repository:     repositoryKey,
		Head:           "head-203",
		PR:             203,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    base,
	}
	duplicate := canonical
	duplicate.Challenge = "duplicate-challenge"
	duplicate.RequestedAt = base.Add(time.Minute)
	canonicalRecord := evaluationChallengeRecord{
		comment:      pullRequestComment{ID: 401, CreatedAt: canonical.RequestedAt},
		commentIndex: 0,
		challenge:    canonical,
	}
	duplicateRecord := evaluationChallengeRecord{
		comment:      pullRequestComment{ID: 402, CreatedAt: duplicate.RequestedAt},
		commentIndex: 1,
		challenge:    duplicate,
	}
	closure := evaluationChallengeClosureRecord{
		comment:      pullRequestComment{ID: 403, CreatedAt: base.Add(2 * time.Minute)},
		commentIndex: 2,
		closure: evaluationChallengeClosure{
			BodySHA256:         bodySHA256,
			CanonicalChallenge: canonical.Challenge,
			ClosedAt:           base.Add(2 * time.Minute),
			Controller:         trustedActor,
			DuplicateChallenge: duplicate.Challenge,
			EvidenceSHA256:     evidenceSHA256,
			Head:               canonical.Head,
			PR:                 canonical.PR,
			Repository:         repositoryKey,
			Schema:             evaluationChallengeClosureSchema,
		},
	}
	baseHistory := evaluationHistory{
		challenges: []evaluationChallengeRecord{canonicalRecord, duplicateRecord},
		closures:   []evaluationChallengeClosureRecord{closure},
	}

	t.Run("receipt on duplicate", func(t *testing.T) {
		receipt := evaluationReceiptRecord{
			comment:      pullRequestComment{ID: 404, CreatedAt: base.Add(3 * time.Minute)},
			commentIndex: 3,
			receipt: evaluationReceipt{
				AttestationSHA256: strings.Repeat("c", 64),
				Challenge:         duplicate.Challenge,
				Repository:        repositoryKey,
				PR:                duplicate.PR,
				Head:              duplicate.Head,
				BodySHA256:        duplicate.BodySHA256,
				EvidenceSHA256:    duplicate.EvidenceSHA256,
				RecordedAt:        base.Add(3 * time.Minute),
			},
		}
		history := baseHistory
		history.receipts = []evaluationReceiptRecord{receipt}
		projection, err := evaluationLogicalProjectionForHistory(history, true)
		if err != nil {
			t.Fatalf("project receipt on closed duplicate: %v", err)
		}
		logical, ok := projection.challengeForID(canonical.Challenge)
		if !ok || !logical.hasReceipt || logical.receipt.receipt.Challenge != duplicate.Challenge {
			t.Fatalf("canonical logical receipt = %#v, want duplicate receipt projected", logical)
		}
		outstanding, err := outstandingEvaluationChallenges(history)
		if err != nil {
			t.Fatalf("outstanding receipt on closed duplicate: %v", err)
		}
		if len(outstanding) != 0 {
			t.Fatalf("outstanding receipt on closed duplicate = %#v, want none", outstanding)
		}
		if !latestEvaluationReceiptClosesLatestChallenge(history) {
			t.Fatal("latest receipt on closed duplicate did not close canonical challenge")
		}
	})

	t.Run("resolution on duplicate", func(t *testing.T) {
		resolution := evaluationResolutionRecord{
			comment:      pullRequestComment{ID: 405, CreatedAt: base.Add(3 * time.Hour)},
			commentIndex: 3,
			resolution: evaluationResolution{
				BodySHA256:     duplicate.BodySHA256,
				Challenge:      duplicate.Challenge,
				EvidenceSHA256: duplicate.EvidenceSHA256,
				Head:           duplicate.Head,
				Repository:     repositoryKey,
				PR:             duplicate.PR,
				Reason:         "expired",
				ResolvedAt:     base.Add(3 * time.Hour),
				Resolver:       trustedActor,
				Schema:         evaluationResolutionSchema,
			},
		}
		history := baseHistory
		history.resolutions = []evaluationResolutionRecord{resolution}
		projection, err := evaluationLogicalProjectionForHistory(history, true)
		if err != nil {
			t.Fatalf("project resolution on closed duplicate: %v", err)
		}
		logical, ok := projection.challengeForID(canonical.Challenge)
		if !ok || !logical.hasResolution || logical.resolution.resolution.Challenge != duplicate.Challenge {
			t.Fatalf("canonical logical resolution = %#v, want duplicate resolution projected", logical)
		}
		outstanding, err := outstandingEvaluationChallenges(history)
		if err != nil {
			t.Fatalf("outstanding resolution on closed duplicate: %v", err)
		}
		if len(outstanding) != 0 {
			t.Fatalf("outstanding resolution on closed duplicate = %#v, want none", outstanding)
		}
	})
}

func FuzzEvaluationChallengeClosureParser(f *testing.F) {
	f.Add("not a structured closure")
	f.Add("<!-- " + evaluationChallengeClosureMarker + "{} -->\n" + evaluationChallengeClosureHeading)
	f.Fuzz(func(_ *testing.T, body string) {
		comment := pullRequestComment{Body: body, CreatedAt: time.Now().UTC()}
		comment.Author.Login = trustedActor
		_, _ = parseEvaluationChallengeClosure(body)
		if _, err := parseEvaluationHistory([]pullRequestComment{comment}); err != nil {
			return
		}
	})
}

func evaluationChallengeProjectionComment(t *testing.T, challenge evaluationChallenge, id int64) pullRequestComment {
	t.Helper()
	marker, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("encode evaluation challenge: %v", err)
	}
	comment := pullRequestComment{
		Body:      "<!-- " + evaluationChallengeMarker + string(marker) + " -->\n",
		CreatedAt: challenge.RequestedAt,
		ID:        id,
	}
	comment.Author.Login = trustedActor
	return comment
}

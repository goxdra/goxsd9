package workflowctl

import (
	"strings"
	"time"
)

const (
	fakeClaimMarkerSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fakeClaimParentSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fakeClaimTreeSHA   = "cccccccccccccccccccccccccccccccccccccccc"
)

func fakeClaimMarkerGit(command string, input []byte, head string, issue int, runID string, lease time.Time) (string, bool) {
	for _, state := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "REBASE_HEAD", "rebase-apply", "rebase-merge", "sequencer"} {
		if command == "rev-parse --git-path "+state {
			return "/repo/.git/" + state, true
		}
	}
	message := claimMessage(issue, runID, lease)
	switch command {
	case "log --first-parent --format=%H%x00%B%x00 " + head:
		return fakeClaimMarkerSHA + "\x00" + message + "\x00", true
	case "cat-file --batch-check=%(objectname) %(objecttype)":
		sha := strings.TrimSpace(string(input))
		if sha == fakeClaimMarkerSHA || sha == fakeClaimParentSHA {
			return sha + " commit", true
		}
	case "cat-file commit " + fakeClaimMarkerSHA:
		return "tree " + fakeClaimTreeSHA + "\nparent " + fakeClaimParentSHA + "\n\n" + message, true
	case "rev-parse " + fakeClaimMarkerSHA + "^{tree}", "rev-parse " + fakeClaimParentSHA + "^{tree}":
		return fakeClaimTreeSHA + "\n", true
	}
	return "", false
}

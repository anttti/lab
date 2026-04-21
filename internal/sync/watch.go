package sync

import (
	"fmt"

	"lab/internal/db"
)

// mrSnapshot captures the state of one MR that is used to detect changes
// across syncs.
type mrSnapshot struct {
	IID            int
	Title          string
	WebURL         string
	PipelineStatus string
	Approved       bool
	NoteIDs        map[int]bool
	UnresolvedOpen int
}

// Update describes a change to a user's MR detected between two snapshots.
type Update struct {
	Title   string
	WebURL  string
	Message string
}

// snapshotUserMRs returns a map keyed by MR id of the current state of every
// MR authored by username. Returns an empty map if username is "".
func snapshotUserMRs(database *db.Database, username string) (map[int64]mrSnapshot, error) {
	if username == "" {
		return map[int64]mrSnapshot{}, nil
	}
	mrs, err := database.ListMRs(db.MRFilter{Author: &username})
	if err != nil {
		return nil, fmt.Errorf("snapshotUserMRs list: %w", err)
	}

	out := make(map[int64]mrSnapshot, len(mrs))
	for _, mr := range mrs {
		comments, err := database.ListComments(mr.ID)
		if err != nil {
			return nil, fmt.Errorf("snapshotUserMRs list comments: %w", err)
		}
		noteIDs := make(map[int]bool, len(comments))
		unresolved := 0
		for _, c := range comments {
			noteIDs[c.NoteID] = true
			if !c.Resolved {
				unresolved++
			}
		}
		ps := ""
		if mr.PipelineStatus != nil {
			ps = *mr.PipelineStatus
		}
		out[mr.ID] = mrSnapshot{
			IID:            mr.IID,
			Title:          mr.Title,
			WebURL:         mr.WebURL,
			PipelineStatus: ps,
			Approved:       mr.Approved,
			NoteIDs:        noteIDs,
			UnresolvedOpen: unresolved,
		}
	}
	return out, nil
}

// diffSnapshots returns one Update per MR that changed between pre and post.
// If an MR is missing from pre (newly seen but authored by the user) no update
// is reported; the user created it themselves. If an MR is missing from post
// (closed/merged) it is also ignored.
func diffSnapshots(pre, post map[int64]mrSnapshot) []Update {
	var updates []Update
	for id, after := range post {
		before, existed := pre[id]
		if !existed {
			continue
		}

		var reasons []string

		newComments := 0
		for noteID := range after.NoteIDs {
			if !before.NoteIDs[noteID] {
				newComments++
			}
		}
		if newComments > 0 {
			if newComments == 1 {
				reasons = append(reasons, "1 new comment")
			} else {
				reasons = append(reasons, fmt.Sprintf("%d new comments", newComments))
			}
		}

		if before.PipelineStatus != after.PipelineStatus && after.PipelineStatus != "" {
			reasons = append(reasons, fmt.Sprintf("pipeline %s", after.PipelineStatus))
		}

		if !before.Approved && after.Approved {
			reasons = append(reasons, "approved")
		}

		if len(reasons) == 0 {
			continue
		}

		message := fmt.Sprintf("!%d %s — %s", after.IID, after.Title, joinReasons(reasons))
		updates = append(updates, Update{
			Title:   "lab: MR updated",
			WebURL:  after.WebURL,
			Message: message,
		})
	}
	return updates
}

func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	out := reasons[0]
	for _, r := range reasons[1:] {
		out += ", " + r
	}
	return out
}

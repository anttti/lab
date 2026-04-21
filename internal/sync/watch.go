package sync

import (
	"fmt"
	"strings"

	"lab/internal/config"
	"lab/internal/db"
	"lab/internal/glab"
)

// pipelineFailStates is the set of terminal pipeline statuses we treat as
// "pipeline failed" for notification purposes.
var pipelineFailStates = map[string]bool{
	"failed":  true,
	"canceled": true,
}

// mrSnapshot captures the state of one MR used to detect changes across
// syncs. AuthoredByUser indicates whether this MR belongs to the user whose
// notifications we are generating.
type mrSnapshot struct {
	RepoID         int64
	RepoURL        string
	ProjectID      int64
	IID            int
	Title          string
	WebURL         string
	Author         string
	State          string
	PipelineStatus string
	Approved       bool
	NoteIDs        map[int]bool
	// Reviewers maps reviewer username → review state.
	Reviewers map[string]string
}

// Update describes a change detected between two snapshots that warrants
// a notification.
type Update struct {
	Title   string
	Message string
	WebURL  string
	// Kind is the notification trigger that caused this update. Useful
	// for test assertions.
	Kind string
}

// mrDetailFetcher looks up an MR's state after it has disappeared from the
// open-MR list. The sync.Engine satisfies this via its glab client.
type mrDetailFetcher interface {
	GetMRDetail(repoURL string, projectID int64, mrIID int) (glab.MRDetail, error)
}

// snapshotAll returns a snapshot keyed by MR id of every MR currently in the
// DB, together with its reviewer map. repos is needed to resolve the GitLab
// URL for disappeared-MR lookups.
func snapshotAll(database *db.Database) (map[int64]mrSnapshot, error) {
	repos, err := database.ListRepos()
	if err != nil {
		return nil, fmt.Errorf("snapshotAll list repos: %w", err)
	}
	repoByID := make(map[int64]*db.Repo, len(repos))
	for i := range repos {
		repoByID[repos[i].ID] = &repos[i]
	}

	mrs, err := database.ListMRs(db.MRFilter{})
	if err != nil {
		return nil, fmt.Errorf("snapshotAll list MRs: %w", err)
	}

	out := make(map[int64]mrSnapshot, len(mrs))
	for _, mr := range mrs {
		comments, err := database.ListComments(mr.ID)
		if err != nil {
			return nil, fmt.Errorf("snapshotAll list comments: %w", err)
		}
		noteIDs := make(map[int]bool, len(comments))
		for _, c := range comments {
			noteIDs[c.NoteID] = true
		}

		reviewers, err := database.GetMRReviewers(mr.ID)
		if err != nil {
			return nil, fmt.Errorf("snapshotAll get reviewers: %w", err)
		}
		revMap := make(map[string]string, len(reviewers))
		for _, r := range reviewers {
			revMap[r.Username] = r.State
		}

		ps := ""
		if mr.PipelineStatus != nil {
			ps = *mr.PipelineStatus
		}

		snap := mrSnapshot{
			RepoID:         mr.RepoID,
			IID:            mr.IID,
			Title:          mr.Title,
			WebURL:         mr.WebURL,
			Author:         mr.Author,
			State:          mr.State,
			PipelineStatus: ps,
			Approved:       mr.Approved,
			NoteIDs:        noteIDs,
			Reviewers:      revMap,
		}
		if repo, ok := repoByID[mr.RepoID]; ok {
			snap.RepoURL = repo.GitLabURL
			snap.ProjectID = repo.ProjectID
		}
		out[mr.ID] = snap
	}
	return out, nil
}

// diffSnapshots returns one Update per MR change that passes the
// notification filters in cfg for the given username. If username is empty
// no updates are produced. A fetcher may be nil, in which case the merged/
// closed lookup for disappeared MRs is skipped.
func diffSnapshots(pre, post map[int64]mrSnapshot, username string, cfg config.Notifications, fetcher mrDetailFetcher) []Update {
	if username == "" {
		return nil
	}
	var updates []Update

	// Changes on MRs that are still present in both snapshots.
	for id, after := range post {
		before, existed := pre[id]
		if !existed {
			if cfg.NewReviewRequest && after.Reviewers[username] != "" && after.Author != username {
				updates = append(updates, Update{
					Kind:    "new_review_request",
					Title:   "lab: review requested",
					Message: fmt.Sprintf("!%d %s — you were added as a reviewer", after.IID, after.Title),
					WebURL:  after.WebURL,
				})
			}
			continue
		}

		updates = append(updates, diffExistingMR(before, after, username, cfg)...)
	}

	// Changes on MRs that disappeared from the open list: could be merged
	// or closed. We only care about the user's own MRs here.
	if cfg.MRMerged && fetcher != nil {
		for id, before := range pre {
			if before.Author != username {
				continue
			}
			if _, still := post[id]; still {
				continue
			}
			if before.RepoURL == "" || before.ProjectID == 0 {
				continue
			}
			detail, err := fetcher.GetMRDetail(before.RepoURL, before.ProjectID, before.IID)
			if err != nil {
				continue
			}
			if detail.State == "merged" {
				updates = append(updates, Update{
					Kind:    "mr_merged",
					Title:   "lab: MR merged",
					Message: fmt.Sprintf("!%d %s — merged", before.IID, before.Title),
					WebURL:  before.WebURL,
				})
			}
		}
	}

	return updates
}

// diffExistingMR handles MRs present in both pre and post snapshots. Returns
// zero or more updates depending on the notification config.
func diffExistingMR(before, after mrSnapshot, username string, cfg config.Notifications) []Update {
	var updates []Update

	// new_comment only fires for MRs authored by the user (unchanged from v1).
	if cfg.NewComment && after.Author == username {
		newComments := 0
		for noteID := range after.NoteIDs {
			if !before.NoteIDs[noteID] {
				newComments++
			}
		}
		if newComments > 0 {
			updates = append(updates, Update{
				Kind:    "new_comment",
				Title:   "lab: MR updated",
				Message: fmt.Sprintf("!%d %s — %s", after.IID, after.Title, pluralComments(newComments)),
				WebURL:  after.WebURL,
			})
		}
	}

	// pipeline_failed: fires when the user's MR transitions to a failing
	// pipeline status.
	if cfg.PipelineFailed && after.Author == username {
		if before.PipelineStatus != after.PipelineStatus && pipelineFailStates[after.PipelineStatus] {
			updates = append(updates, Update{
				Kind:    "pipeline_failed",
				Title:   "lab: pipeline " + after.PipelineStatus,
				Message: fmt.Sprintf("!%d %s", after.IID, after.Title),
				WebURL:  after.WebURL,
			})
		}
	}

	// approved: user's MR got approved.
	if cfg.Approved && after.Author == username && !before.Approved && after.Approved {
		updates = append(updates, Update{
			Kind:    "approved",
			Title:   "lab: MR approved",
			Message: fmt.Sprintf("!%d %s", after.IID, after.Title),
			WebURL:  after.WebURL,
		})
	}

	// new_review_request: user became a reviewer on someone else's MR.
	if cfg.NewReviewRequest && after.Author != username {
		_, wasReviewer := before.Reviewers[username]
		_, isReviewer := after.Reviewers[username]
		if !wasReviewer && isReviewer {
			updates = append(updates, Update{
				Kind:    "new_review_request",
				Title:   "lab: review requested",
				Message: fmt.Sprintf("!%d %s — you were added as a reviewer", after.IID, after.Title),
				WebURL:  after.WebURL,
			})
		}
	}

	// rereview_request: user is already a reviewer and their review state
	// reset from a non-unreviewed value back to unreviewed.
	if cfg.RereviewRequest && after.Author != username {
		bs, wasReviewer := before.Reviewers[username]
		as, isReviewer := after.Reviewers[username]
		if wasReviewer && isReviewer && isReviewed(bs) && as == "unreviewed" {
			updates = append(updates, Update{
				Kind:    "rereview_request",
				Title:   "lab: re-review requested",
				Message: fmt.Sprintf("!%d %s", after.IID, after.Title),
				WebURL:  after.WebURL,
			})
		}
	}

	return updates
}

// isReviewed returns true when a GitLab reviewer_state indicates the user
// has already reviewed the MR (as opposed to "unreviewed" or empty).
func isReviewed(state string) bool {
	switch strings.ToLower(state) {
	case "reviewed", "approved", "requested_changes", "unapproved":
		return true
	}
	return false
}

func pluralComments(n int) string {
	if n == 1 {
		return "1 new comment"
	}
	return fmt.Sprintf("%d new comments", n)
}

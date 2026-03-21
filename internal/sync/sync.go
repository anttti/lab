package sync

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/anttimattila/lab/internal/db"
	"github.com/anttimattila/lab/internal/glab"
)

// GlabClient is the interface the Engine uses to talk to GitLab via glab.
// glab.Client satisfies this interface via Go structural typing.
type GlabClient interface {
	ListMRs(repoURL string) ([]glab.MRListItem, error)
	ListDiscussions(repoURL string, projectID int64, mrIID int) ([]glab.Discussion, error)
	GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error)
}

// Engine orchestrates syncing GitLab data into the local database.
type Engine struct {
	db     *db.Database
	client GlabClient
}

// New creates a new sync Engine.
func New(database *db.Database, client GlabClient) *Engine {
	return &Engine{db: database, client: client}
}

// SyncAll syncs all repos in the database.
func (e *Engine) SyncAll() error {
	repos, err := e.db.ListRepos()
	if err != nil {
		return fmt.Errorf("SyncAll list repos: %w", err)
	}
	var firstErr error
	for i := range repos {
		if err := e.SyncRepo(&repos[i]); err != nil {
			log.Printf("sync repo %q: %v", repos[i].Path, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// SyncRepo syncs all MRs (and their discussions) for the given repo.
func (e *Engine) SyncRepo(repo *db.Repo) error {
	// Check that the repo path exists on disk.
	if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
		log.Printf("SyncRepo: path %q does not exist on disk, skipping", repo.Path)
		return nil
	}

	glabMRs, err := e.client.ListMRs(repo.GitLabURL)
	if err != nil {
		return fmt.Errorf("SyncRepo list MRs: %w", err)
	}

	// Update project_id from the first MR if not yet set.
	if repo.ProjectID == 0 && len(glabMRs) > 0 {
		repo.ProjectID = glabMRs[0].ProjectID
		if err := e.db.UpdateRepoProjectID(repo.ID, repo.ProjectID); err != nil {
			return fmt.Errorf("SyncRepo update project_id: %w", err)
		}
	}

	keepIIDs := make([]int, 0, len(glabMRs))

	for _, glabMR := range glabMRs {
		keepIIDs = append(keepIIDs, glabMR.IID)

		// Capture the pre-upsert synced_at so we can decide whether discussions
		// need refreshing. We look up by (repo_id, iid) before mutating the row.
		var preSyncedAt *time.Time
		{
			existing, _ := e.db.ListMRs(db.MRFilter{RepoID: &repo.ID})
			for i := range existing {
				if existing[i].IID == glabMR.IID {
					preSyncedAt = existing[i].SyncedAt
					break
				}
			}
		}

		// Fetch pipeline status separately so we always have the freshest value.
		pipelineStatus, err := e.client.GetMRPipeline(repo.GitLabURL, repo.ProjectID, glabMR.IID)
		if err != nil {
			log.Printf("SyncRepo get pipeline for MR !%d: %v", glabMR.IID, err)
		}

		var ps *string
		if pipelineStatus != "" {
			ps = &pipelineStatus
		}

		mr := &db.MergeRequest{
			RepoID:         repo.ID,
			IID:            glabMR.IID,
			Title:          glabMR.Title,
			Author:         glabMR.Author.Username,
			State:          glabMR.State,
			SourceBranch:   glabMR.SourceBranch,
			TargetBranch:   glabMR.TargetBranch,
			WebURL:         glabMR.WebURL,
			PipelineStatus: ps,
			UpdatedAt:      glabMR.UpdatedAt,
		}

		if err := e.db.UpsertMR(mr); err != nil {
			return fmt.Errorf("SyncRepo upsert MR !%d: %w", glabMR.IID, err)
		}

		if err := e.db.SetMRLabels(mr.ID, glabMR.Labels); err != nil {
			return fmt.Errorf("SyncRepo set labels for MR !%d: %w", glabMR.IID, err)
		}

		// Only sync discussions if the MR has been updated since the last sync.
		// Use the pre-upsert synced_at value so we are not comparing against "now".
		if preSyncedAt == nil || glabMR.UpdatedAt.After(*preSyncedAt) {
			if err := e.syncDiscussions(repo, mr, glabMR.IID); err != nil {
				log.Printf("SyncRepo sync discussions for MR !%d: %v", glabMR.IID, err)
			}
		}
	}

	// Delete MRs that are no longer returned by glab.
	if err := e.db.DeleteStaleMRs(repo.ID, keepIIDs); err != nil {
		return fmt.Errorf("SyncRepo delete stale MRs: %w", err)
	}

	if err := e.db.UpdateRepoSyncTime(repo.ID); err != nil {
		return fmt.Errorf("SyncRepo update sync time: %w", err)
	}

	return nil
}

// SyncMR syncs discussions for a single MR identified by its IID.
func (e *Engine) SyncMR(repo *db.Repo, mrIID int) error {
	mrs, err := e.db.ListMRs(db.MRFilter{RepoID: &repo.ID})
	if err != nil {
		return fmt.Errorf("SyncMR list MRs: %w", err)
	}

	var target *db.MergeRequest
	for i := range mrs {
		if mrs[i].IID == mrIID {
			target = &mrs[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("SyncMR: MR !%d not found in repo %q", mrIID, repo.Path)
	}

	return e.syncDiscussions(repo, target, mrIID)
}

// syncDiscussions fetches discussions from GitLab and upserts them into the DB.
// System notes are filtered out. Position data is mapped to file_path/old_line/new_line.
func (e *Engine) syncDiscussions(repo *db.Repo, mr *db.MergeRequest, mrIID int) error {
	discussions, err := e.client.ListDiscussions(repo.GitLabURL, repo.ProjectID, mrIID)
	if err != nil {
		return fmt.Errorf("syncDiscussions list: %w", err)
	}

	keepNoteIDs := []int{}

	for _, disc := range discussions {
		for _, note := range disc.Notes {
			// Skip system-generated notes (e.g. "assigned to @user").
			if note.System {
				continue
			}

			keepNoteIDs = append(keepNoteIDs, note.ID)

			var filePath *string
			var oldLine, newLine *int

			if note.Position != nil {
				// Prefer new_path, fall back to old_path.
				p := note.Position.NewPath
				if p == "" {
					p = note.Position.OldPath
				}
				if p != "" {
					filePath = &p
				}
				oldLine = note.Position.OldLine
				newLine = note.Position.NewLine
			}

			c := &db.Comment{
				MRID:         mr.ID,
				DiscussionID: disc.ID,
				NoteID:       note.ID,
				Author:       note.Author.Username,
				Body:         note.Body,
				FilePath:     filePath,
				OldLine:      oldLine,
				NewLine:      newLine,
				Resolved:     note.Resolved,
				CreatedAt:    note.CreatedAt,
			}

			if err := e.db.UpsertComment(c); err != nil {
				return fmt.Errorf("syncDiscussions upsert note %d: %w", note.ID, err)
			}
		}
	}

	if err := e.db.DeleteStaleComments(mr.ID, keepNoteIDs); err != nil {
		return fmt.Errorf("syncDiscussions delete stale comments: %w", err)
	}

	return nil
}

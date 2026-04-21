package sync

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"lab/internal/config"
	"lab/internal/db"
	"lab/internal/glab"
	"lab/internal/notify"
)

const maxConcurrency = 4

// GlabClient is the interface the Engine uses to talk to GitLab via glab.
// glab.Client satisfies this interface via Go structural typing.
type GlabClient interface {
	ListMRs(repoURL string) ([]glab.MRListItem, error)
	ListDiscussions(repoURL string, projectID int64, mrIID int) ([]glab.Discussion, error)
	GetMRDetail(repoURL string, projectID int64, mrIID int) (glab.MRDetail, error)
	GetFileContent(repoURL string, projectID int64, filePath, ref string) (string, error)
}

// Engine orchestrates syncing GitLab data into the local database.
type Engine struct {
	db     *db.Database
	client GlabClient
	out    io.Writer
}

// New creates a new sync Engine.
func New(database *db.Database, client GlabClient) *Engine {
	return &Engine{db: database, client: client, out: os.Stdout}
}

// SetOutput sets the writer used for status messages.
func (e *Engine) SetOutput(w io.Writer) {
	e.out = w
}

// SyncAllWithWriter syncs all repos using the given writer for progress output.
func (e *Engine) SyncAllWithWriter(w io.Writer) error {
	old := e.out
	e.out = w
	defer func() { e.out = old }()
	return e.SyncAll()
}

// SyncAllWithNotifications runs SyncAll and emits a notification for each
// change detected between pre- and post-sync snapshots that matches the
// enabled triggers in cfg for username. If username is empty or notifier is
// nil it behaves like SyncAll.
func (e *Engine) SyncAllWithNotifications(username string, cfg config.Notifications, notifier notify.Notifier) error {
	if notifier == nil || username == "" {
		return e.SyncAll()
	}

	pre, err := snapshotAll(e.db)
	if err != nil {
		log.Printf("snapshot (pre): %v", err)
		pre = map[int64]mrSnapshot{}
	}

	syncErr := e.SyncAll()

	post, err := snapshotAll(e.db)
	if err != nil {
		log.Printf("snapshot (post): %v", err)
		return syncErr
	}

	for _, u := range diffSnapshots(pre, post, username, cfg, e.client) {
		if err := notifier.Notify(u.Title, u.Message, u.WebURL); err != nil {
			log.Printf("notify: %v", err)
		}
	}

	return syncErr
}

// SyncAll syncs all repos in the database.
func (e *Engine) SyncAll() error {
	repos, err := e.db.ListRepos()
	if err != nil {
		return fmt.Errorf("SyncAll list repos: %w", err)
	}
	var firstErr error
	for i := range repos {
		fmt.Fprintf(e.out, "Syncing %d/%d %s\n", i+1, len(repos), repos[i].Name)
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

	keepIIDs := make([]int, len(glabMRs))
	for i, glabMR := range glabMRs {
		keepIIDs[i] = glabMR.IID
	}

	// Delete stale MRs before upserting so we don't conflict.
	if err := e.db.DeleteStaleMRs(repo.ID, keepIIDs); err != nil {
		return fmt.Errorf("SyncRepo delete stale MRs: %w", err)
	}

	// Sync MRs concurrently with a bounded worker pool.
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, glabMR := range glabMRs {
		wg.Add(1)
		go func(idx int, glabMR glab.MRListItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Fprintf(e.out, "  MR !%d (%d/%d): %s\n", glabMR.IID, idx+1, len(glabMRs), glabMR.Title)

			if err := e.syncMRItem(repo, glabMR); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(i, glabMR)
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	if err := e.db.UpdateRepoSyncTime(repo.ID); err != nil {
		return fmt.Errorf("SyncRepo update sync time: %w", err)
	}

	return nil
}

// syncMRItem fetches pipeline/discussions and upserts a single MR into the DB.
func (e *Engine) syncMRItem(repo *db.Repo, glabMR glab.MRListItem) error {
	detail, err := e.client.GetMRDetail(repo.GitLabURL, repo.ProjectID, glabMR.IID)
	if err != nil {
		log.Printf("SyncRepo get detail for MR !%d: %v", glabMR.IID, err)
	}

	var ps *string
	if detail.PipelineStatus != "" {
		ps = &detail.PipelineStatus
	}

	mr := &db.MergeRequest{
		RepoID:         repo.ID,
		IID:            glabMR.IID,
		Title:          glabMR.Title,
		Author:         glabMR.Author.Username,
		State:          glabMR.State,
		Draft:          glabMR.Draft,
		SourceBranch:   glabMR.SourceBranch,
		TargetBranch:   glabMR.TargetBranch,
		WebURL:         glabMR.WebURL,
		PipelineStatus: ps,
		Approved:       detail.Approved,
		UpdatedAt:      glabMR.UpdatedAt,
	}

	if err := e.db.UpsertMR(mr); err != nil {
		return fmt.Errorf("SyncRepo upsert MR !%d: %w", glabMR.IID, err)
	}

	if err := e.db.SetMRLabels(mr.ID, glabMR.Labels); err != nil {
		return fmt.Errorf("SyncRepo set labels for MR !%d: %w", glabMR.IID, err)
	}

	if err := e.db.SetMRReviewers(mr.ID, mergeReviewers(glabMR.Reviewers, detail.Reviewers)); err != nil {
		return fmt.Errorf("SyncRepo set reviewers for MR !%d: %w", glabMR.IID, err)
	}

	if err := e.syncDiscussions(repo, mr, glabMR.IID); err != nil {
		log.Printf("SyncRepo sync discussions for MR !%d: %v", glabMR.IID, err)
	}

	return nil
}

// mergeReviewers prefers detail reviewers (which include ReviewState) over
// list reviewers, but falls back to the list if the detail call failed.
func mergeReviewers(list, detail []glab.Reviewer) []db.Reviewer {
	src := detail
	if len(src) == 0 {
		src = list
	}
	out := make([]db.Reviewer, 0, len(src))
	for _, r := range src {
		out = append(out, db.Reviewer{Username: r.Username, State: r.ReviewState})
	}
	return out
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
// For diff notes, a code snippet is fetched from GitLab and stored as diff_hunk.
func (e *Engine) syncDiscussions(repo *db.Repo, mr *db.MergeRequest, mrIID int) error {
	discussions, err := e.client.ListDiscussions(repo.GitLabURL, repo.ProjectID, mrIID)
	if err != nil {
		return fmt.Errorf("syncDiscussions list: %w", err)
	}

	// Cache fetched file contents to avoid redundant API calls.
	// Key: "ref:path"
	fileCache := map[string]string{}

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
			var diffHunk string

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

				// Fetch code snippet for the first non-system note with position data.
				if filePath != nil && note.Position.HeadSHA != "" {
					targetLine := 0
					if newLine != nil {
						targetLine = *newLine
					} else if oldLine != nil {
						targetLine = *oldLine
					}
					if targetLine > 0 {
						cacheKey := note.Position.HeadSHA + ":" + *filePath
						content, ok := fileCache[cacheKey]
						if !ok {
							content, err = e.client.GetFileContent(repo.GitLabURL, repo.ProjectID, *filePath, note.Position.HeadSHA)
							if err != nil {
								log.Printf("fetch file snippet for %s@%s: %v", *filePath, note.Position.HeadSHA[:8], err)
								content = ""
							}
							fileCache[cacheKey] = content
						}
						if content != "" {
							diffHunk = glab.ExtractSnippet(content, targetLine, 3)
						}
					}
				}
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
				DiffHunk:     diffHunk,
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

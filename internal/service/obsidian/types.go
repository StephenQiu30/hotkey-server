package obsidian

import (
	"context"
	"errors"

	domain "github.com/StephenQiu30/hotkey-server/internal/domain/obsidian"
)

var (
	ErrInvalidInput   = errors.New("invalid input")
	ErrNotConnected   = errors.New("not connected")
	ErrAlreadySynced  = errors.New("already synced")
	ErrConflict       = errors.New("conflict")
	ErrAuthFailed     = errors.New("auth failed")
	ErrRepoNotFound   = errors.New("repo not found")
	ErrBranchNotFound = errors.New("branch not found")
	ErrDirNotFound    = errors.New("dir not found")
	ErrInternal       = errors.New("internal error")
)

// GitProvider abstracts Git operations for testability.
type GitProvider interface {
	// ValidateRepo checks that the repo URL is reachable and the branch exists.
	ValidateRepo(ctx context.Context, repoURL, branch, accessToken string) (ValidateRepoResult, error)
	// CheckDirExists verifies a directory exists in the remote repo.
	CheckDirExists(ctx context.Context, repoURL, branch, dir, accessToken string) (bool, error)
	// CommitAndPush writes a file and pushes to the remote repo.
	// Returns the commit SHA and a commit URL (or empty if not available).
	CommitAndPush(ctx context.Context, input CommitInput) (CommitResult, error)
	// PullAndCheckConflict pulls latest and checks if the target file has conflicts.
	PullAndCheckConflict(ctx context.Context, repoURL, branch, filePath, accessToken string) (bool, error)
}

type ValidateRepoResult struct {
	DefaultBranch string
	Branches      []string
}

type CommitInput struct {
	RepoURL        string
	Branch         string
	Dir            string
	FilePath       string
	Content        []byte
	CommitMsg      string
	AccessToken    string
	IdempotencyKey string
}

type CommitResult struct {
	CommitSHA string
	CommitURL string
}

// SyncInput is the request to sync a piece of content.
type SyncInput struct {
	UserID      string
	ContentType string // "event_note" | "daily_report" | "weekly_report"
	ContentID   string
	Title       string
	Body        string
	Tags        []string
	Frontmatter map[string]string
}

// SyncOutput is the result of a sync operation.
type SyncOutput struct {
	RecordID      string
	FilePath      string
	CommitSHA     string
	CommitURL     string
	State         domain.SyncRecordState
	WasIdempotent bool
}

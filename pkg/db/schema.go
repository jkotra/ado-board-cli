package db

import (
	"time"
)

// TrackedRoot represents a work item tree root tracked by the user
type TrackedRoot struct {
	ID          int       `json:"id"`
	Org         string    `json:"org"`
	Project     string    `json:"project"`
	URL         string    `json:"url"`
	QueryID     string    `json:"query_id"`
	CustomTitle string    `json:"custom_title,omitempty"`
	TrackedAt   time.Time `json:"tracked_at"`
}

// WorkItemRecord represents a stored work item in SQLite
type WorkItemRecord struct {
	ID                 int       `json:"id"`
	Org                string    `json:"org"`
	Project            string    `json:"project"`
	WorkItemType       string    `json:"work_item_type"`
	Title              string    `json:"title"`
	State              string    `json:"state"`
	Reason             string    `json:"reason"`
	AssignedToName     string    `json:"assigned_to_name"`
	AssignedToEmail    string    `json:"assigned_to_email"`
	AssignedToAvatar   string    `json:"assigned_to_avatar"`
	CreatedDate        string    `json:"created_date"`
	CreatedByName      string    `json:"created_by_name"`
	CreatedByEmail     string    `json:"created_by_email"`
	ChangedDate        string    `json:"changed_date"`
	ChangedByName      string    `json:"changed_by_name"`
	StartDate          string    `json:"start_date"`
	TargetDate         string    `json:"target_date"`
	FinishDate         string    `json:"finish_date"`
	Description        string    `json:"description"`
	AcceptanceCriteria string    `json:"acceptance_criteria"`
	ReproSteps         string    `json:"repro_steps"`
	OriginalEstimate   float64   `json:"original_estimate"`
	RemainingWork      float64   `json:"remaining_work"`
	CompletedWork      float64   `json:"completed_work"`
	Effort             float64   `json:"effort"`
	StoryPoints        float64   `json:"story_points"`
	Priority           int       `json:"priority"`
	AreaPath           string    `json:"area_path"`
	IterationPath      string    `json:"iteration_path"`
	Tags               string    `json:"tags"`
	Activity           string    `json:"activity"`
	ParentID           int       `json:"parent_id"`
	URL                string    `json:"url"`
	LastSyncedAt       time.Time `json:"last_synced_at"`
}

// RelationRecord represents a relationship link between work items
type RelationRecord struct {
	SourceID int    `json:"source_id"`
	TargetID int    `json:"target_id"`
	RelType  string `json:"rel_type"`
}

// CommentRecord represents a comment stored in SQLite
type CommentRecord struct {
	ID          int       `json:"id"`
	WorkItemID  int       `json:"work_item_id"`
	CommentID   int       `json:"comment_id"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Text        string    `json:"text"`
	CreatedDate time.Time `json:"created_date"`
}

// PullRequestRecord represents a pull request stored in SQLite
type PullRequestRecord struct {
	ID             int       `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	RepositoryName string    `json:"repository_name"`
	Title          string    `json:"title"`
	Status         string    `json:"status"` // "active", "completed", "abandoned"
	CreatedByName  string    `json:"created_by_name"`
	CreatedByEmail string    `json:"created_by_email"`
	SourceBranch   string    `json:"source_branch"`
	TargetBranch   string    `json:"target_branch"`
	MergeStatus    string    `json:"merge_status"`
	URL            string    `json:"url"`
	LastSyncedAt   time.Time `json:"last_synced_at"`
}

// IdentityRecord represents a stored identity in SQLite
type IdentityRecord struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

// TrackedQuery represents an Azure DevOps query tracked in SQLite
type TrackedQuery struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Org       string    `json:"org"`
	Project   string    `json:"project"`
	URL       string    `json:"url"`
	QueryType string    `json:"query_type"`
	TrackedAt time.Time `json:"tracked_at"`
	ItemCount int       `json:"item_count"`
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tracked_queries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT,
    org TEXT NOT NULL,
    project TEXT NOT NULL,
    url TEXT,
    query_type TEXT,
    tracked_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS tracked_roots (
    id INTEGER PRIMARY KEY,
    org TEXT NOT NULL,
    project TEXT NOT NULL,
    url TEXT,
    query_id TEXT,
    custom_title TEXT DEFAULT '',
    tracked_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS work_items (
    id INTEGER PRIMARY KEY,
    org TEXT NOT NULL,
    project TEXT NOT NULL,
    work_item_type TEXT NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    reason TEXT,
    assigned_to_name TEXT,
    assigned_to_email TEXT,
    assigned_to_avatar TEXT,
    created_date TEXT,
    created_by_name TEXT,
    created_by_email TEXT,
    changed_date TEXT,
    changed_by_name TEXT,
    start_date TEXT,
    target_date TEXT,
    finish_date TEXT,
    description TEXT,
    acceptance_criteria TEXT,
    repro_steps TEXT,
    original_estimate REAL DEFAULT 0,
    remaining_work REAL DEFAULT 0,
    completed_work REAL DEFAULT 0,
    effort REAL DEFAULT 0,
    story_points REAL DEFAULT 0,
    priority INTEGER DEFAULT 2,
    area_path TEXT,
    iteration_path TEXT,
    tags TEXT,
    activity TEXT DEFAULT '',
    parent_id INTEGER DEFAULT 0,
    url TEXT,
    last_synced_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_work_items_parent_id ON work_items(parent_id);
CREATE INDEX IF NOT EXISTS idx_work_items_org_proj ON work_items(org, project);

CREATE TABLE IF NOT EXISTS relations (
    source_id INTEGER NOT NULL,
    target_id INTEGER NOT NULL,
    rel_type TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, rel_type)
);

CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_item_id INTEGER NOT NULL,
    comment_id INTEGER DEFAULT 0,
    author_name TEXT,
    author_email TEXT,
    text TEXT NOT NULL,
    created_date DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_comments_wi_id ON comments(work_item_id);

CREATE TABLE IF NOT EXISTS identities (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    email TEXT,
    avatar_url TEXT
);
CREATE INDEX IF NOT EXISTS idx_identities_email ON identities(email);

CREATE TABLE IF NOT EXISTS pull_requests (
    id INTEGER PRIMARY KEY,
    repository_id TEXT NOT NULL,
    repository_name TEXT,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    created_by_name TEXT,
    created_by_email TEXT,
    source_branch TEXT,
    target_branch TEXT,
    merge_status TEXT,
    url TEXT,
    last_synced_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS work_item_pull_requests (
    work_item_id INTEGER NOT NULL,
    pull_request_id INTEGER NOT NULL,
    PRIMARY KEY (work_item_id, pull_request_id)
);
CREATE INDEX IF NOT EXISTS idx_wi_prs_wi_id ON work_item_pull_requests(work_item_id);

CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS work_item_type_states (
    org TEXT NOT NULL,
    project TEXT NOT NULL,
    work_item_type TEXT NOT NULL,
    state_name TEXT NOT NULL,
    category TEXT,
    color TEXT,
    sort_order INTEGER DEFAULT 0,
    PRIMARY KEY (org, project, work_item_type, state_name)
);

CREATE TABLE IF NOT EXISTS work_item_type_fields (
    org TEXT NOT NULL,
    project TEXT NOT NULL,
    work_item_type TEXT NOT NULL,
    reference_name TEXT NOT NULL,
    name TEXT NOT NULL,
    always_required INTEGER DEFAULT 0,
    PRIMARY KEY (org, project, work_item_type, reference_name)
);
`

package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ado-board-cli/pkg/ado"

	_ "modernc.org/sqlite"
)

// DB manages SQLite database operations
type DB struct {
	conn *sql.DB

	typeFieldsMu    sync.RWMutex
	typeFieldsCache map[string]*typeFieldsCacheEntry

	typeStatesMu    sync.RWMutex
	typeStatesCache map[string]*typeStatesCacheEntry
}

type typeFieldsCacheEntry struct {
	hasFields       bool
	effortField     string
	hasEffort       bool
	estimateField   string
	hasEstimate     bool
	remainingField  string
	hasRemaining    bool
	completedField  string
	hasCompleted    bool
	startDateField  string
	hasStartDate    bool
	targetDateField string
	hasTargetDate   bool
	finishDateField string
	hasFinishDate   bool
}

type typeStatesCacheEntry struct {
	hasStates       bool
	states          []string
	stateCategories map[string]string // lowercase state_name -> category
}

func makeTypeCacheKey(org, project, itemType string) string {
	return strings.ToLower(org) + "|" + strings.ToLower(project) + "|" + strings.ToLower(itemType)
}

// InvalidateTypeSchemaCache clears the in-memory cache for a given work item type
func (d *DB) InvalidateTypeSchemaCache(itemType string) {
	if d == nil {
		return
	}
	suffix := "|" + strings.ToLower(itemType)

	d.typeFieldsMu.Lock()
	if d.typeFieldsCache != nil {
		for k := range d.typeFieldsCache {
			if strings.HasSuffix(k, suffix) {
				delete(d.typeFieldsCache, k)
			}
		}
	}
	d.typeFieldsMu.Unlock()

	d.typeStatesMu.Lock()
	if d.typeStatesCache != nil {
		for k := range d.typeStatesCache {
			if strings.HasSuffix(k, suffix) {
				delete(d.typeStatesCache, k)
			}
		}
	}
	d.typeStatesMu.Unlock()
}

// Open opens or creates the SQLite database at the specified path
func Open(dbPath string) (*DB, error) {
	var dsn string
	if dbPath == ":memory:" || strings.Contains(dbPath, "mode=memory") {
		dsn = ":memory:"
	} else if strings.HasPrefix(dbPath, "file:") {
		dsn = dbPath
	} else {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
		dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", filepath.ToSlash(dbPath))
	}

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Set connection pool settings suitable for SQLite
	conn.SetMaxOpenConns(1)

	db := &DB{
		conn:            conn,
		typeFieldsCache: make(map[string]*typeFieldsCacheEntry),
		typeStatesCache: make(map[string]*typeStatesCacheEntry),
	}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	if d.conn != nil {
		return d.conn.Close()
	}
	return nil
}

func (d *DB) initSchema() error {
	if _, err := d.conn.Exec(schemaSQL); err != nil {
		return err
	}
	// Migrations for newly added date columns if upgrading existing db
	_, _ = d.conn.Exec(`ALTER TABLE work_items ADD COLUMN start_date TEXT;`)
	_, _ = d.conn.Exec(`ALTER TABLE work_items ADD COLUMN target_date TEXT;`)
	_, _ = d.conn.Exec(`ALTER TABLE work_items ADD COLUMN finish_date TEXT;`)

	// Backfill identities from existing work_items table
	_, _ = d.conn.Exec(`
		INSERT OR IGNORE INTO identities (id, display_name, email, avatar_url)
		SELECT LOWER(assigned_to_email), assigned_to_name, assigned_to_email, assigned_to_avatar
		FROM work_items WHERE assigned_to_name != '' AND assigned_to_email != '';
	`)
	// Clean up duplicate identities where both a GUID row and email row exist with the same email
	_, _ = d.conn.Exec(`
		DELETE FROM identities 
		WHERE id = email 
		  AND EXISTS (
		    SELECT 1 FROM identities i2 
		    WHERE LOWER(i2.email) = LOWER(identities.email) AND i2.id != identities.id
		  );
	`)
	// Seed default settings if not present
	now := time.Now().UTC()
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('gemini_model', 'gemini-3.8-flash', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('open_in_browser_cmd', '', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('default_org', 'my-org', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('default_project', 'my-project', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('auto_calc_remaining', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('effort_same_as_estimate', 'false', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_effort', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_estimate', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_start_date', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_target_date', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_finish_date', 'false', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_types', 'Task,Bug', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_statuses', 'New,Active,To Do,In Progress,Committed,Resolved,Done,Closed', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_date_statuses', 'In Progress,Active,Committed,Resolved,Done,Closed', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_type_tasks', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_type_bugs', 'true', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_type_pbi', 'false', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('tag_regex', '', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_tag_regex', 'false', ?)`, now)
	_, _ = d.conn.Exec(`INSERT OR IGNORE INTO app_settings (key, value, updated_at) VALUES ('incomplete_criteria_activity', 'false', ?)`, now)

	// Schema migrations for tracked_queries, query_id, custom_title & activity
	_, _ = d.conn.Exec(`CREATE TABLE IF NOT EXISTS tracked_queries (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		path TEXT,
		org TEXT NOT NULL,
		project TEXT NOT NULL,
		url TEXT,
		query_type TEXT,
		tracked_at DATETIME NOT NULL
	);`)
	_, _ = d.conn.Exec(`ALTER TABLE tracked_roots ADD COLUMN query_id TEXT;`)
	_, _ = d.conn.Exec(`ALTER TABLE tracked_roots ADD COLUMN custom_title TEXT DEFAULT '';`)
	_, _ = d.conn.Exec(`ALTER TABLE work_items ADD COLUMN activity TEXT DEFAULT '';`)

	return nil
}

// SaveTrackedQuery inserts or updates a tracked ADO query
func (d *DB) SaveTrackedQuery(id, name, path, org, project, url, queryType string) error {
	query := `
	INSERT INTO tracked_queries (id, name, path, org, project, url, query_type, tracked_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		path = excluded.path,
		org = excluded.org,
		project = excluded.project,
		url = excluded.url,
		query_type = excluded.query_type,
		tracked_at = excluded.tracked_at;
	`
	_, err := d.conn.Exec(query, id, name, path, org, project, url, queryType, time.Now().UTC())
	return err
}

// UpdateTrackedQueryName updates the name of a tracked query
func (d *DB) UpdateTrackedQueryName(id string, name string) error {
	_, err := d.conn.Exec("UPDATE tracked_queries SET name = ? WHERE id = ?", name, id)
	return err
}

// GetTrackedQueries retrieves all tracked queries with their current item count
func (d *DB) GetTrackedQueries() ([]TrackedQuery, error) {
	query := `
	SELECT q.id, q.name, COALESCE(q.path, ''), q.org, q.project, COALESCE(q.url, ''), COALESCE(q.query_type, ''), q.tracked_at,
	       (SELECT COUNT(*) FROM tracked_roots r WHERE r.query_id = q.id) as item_count
	FROM tracked_queries q
	ORDER BY q.tracked_at DESC
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []TrackedQuery
	for rows.Next() {
		var q TrackedQuery
		if err := rows.Scan(&q.ID, &q.Name, &q.Path, &q.Org, &q.Project, &q.URL, &q.QueryType, &q.TrackedAt, &q.ItemCount); err != nil {
			return nil, err
		}
		queries = append(queries, q)
	}
	return queries, nil
}

// DeleteTrackedQuery removes a tracked query and all associated tracked roots
func (d *DB) DeleteTrackedQuery(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tracked_roots WHERE query_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM tracked_queries WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveTrackedRoot inserts or updates a tracked root work item
func (d *DB) SaveTrackedRoot(id int, org, project, url string) error {
	return d.SaveTrackedRootWithQuery(id, org, project, url, "")
}

// SaveTrackedRootWithQuery inserts or updates a tracked root work item optionally linked to a query
func (d *DB) SaveTrackedRootWithQuery(id int, org, project, url, queryID string) error {
	query := `
	INSERT INTO tracked_roots (id, org, project, url, query_id, custom_title, tracked_at)
	VALUES (?, ?, ?, ?, ?, '', ?)
	ON CONFLICT(id) DO UPDATE SET
		org = excluded.org,
		project = excluded.project,
		url = excluded.url,
		query_id = CASE WHEN excluded.query_id != '' THEN excluded.query_id ELSE tracked_roots.query_id END,
		tracked_at = excluded.tracked_at;
	`
	_, err := d.conn.Exec(query, id, org, project, url, queryID, time.Now().UTC())
	return err
}

// UpdateTrackedRootCustomTitle updates the custom title / alias for a tracked root
func (d *DB) UpdateTrackedRootCustomTitle(id int, customTitle string) error {
	_, err := d.conn.Exec("UPDATE tracked_roots SET custom_title = ? WHERE id = ?", customTitle, id)
	return err
}

// GetTrackedRoots retrieves all tracked roots
func (d *DB) GetTrackedRoots() ([]TrackedRoot, error) {
	query := `SELECT id, org, project, COALESCE(url, ''), COALESCE(query_id, ''), COALESCE(custom_title, ''), tracked_at FROM tracked_roots ORDER BY tracked_at DESC, id DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []TrackedRoot
	for rows.Next() {
		var r TrackedRoot
		if err := rows.Scan(&r.ID, &r.Org, &r.Project, &r.URL, &r.QueryID, &r.CustomTitle, &r.TrackedAt); err != nil {
			return nil, err
		}
		roots = append(roots, r)
	}
	return roots, nil
}

// TrackedRootDetail represents a tracked root with joined work item details
type TrackedRootDetail struct {
	ID           int
	Org          string
	Project      string
	URL          string
	QueryID      string
	CustomTitle  string
	TrackedAt    time.Time
	Title        string
	WorkItemType string
	State        string
}

// DisplayTitle returns the custom title if present, otherwise the real work item title
func (t TrackedRootDetail) DisplayTitle() string {
	if t.CustomTitle != "" {
		return t.CustomTitle
	}
	return t.Title
}

// GetTrackedRootsWithDetails retrieves all tracked roots joined with their work items
func (d *DB) GetTrackedRootsWithDetails() ([]TrackedRootDetail, error) {
	query := `
	SELECT r.id, r.org, r.project, COALESCE(r.url, ''), COALESCE(r.query_id, ''), COALESCE(r.custom_title, ''), r.tracked_at,
	       COALESCE(w.title, ''),
	       COALESCE(w.work_item_type, ''), COALESCE(w.state, '')
	FROM tracked_roots r
	LEFT JOIN work_items w ON r.id = w.id
	ORDER BY r.tracked_at DESC, r.id DESC
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []TrackedRootDetail
	for rows.Next() {
		var r TrackedRootDetail
		if err := rows.Scan(&r.ID, &r.Org, &r.Project, &r.URL, &r.QueryID, &r.CustomTitle, &r.TrackedAt, &r.Title, &r.WorkItemType, &r.State); err != nil {
			return nil, err
		}
		roots = append(roots, r)
	}
	return roots, nil
}

// GetStandaloneTrackedRootsWithDetails retrieves only standalone tracked roots (not linked to queries) joined with their work items
func (d *DB) GetStandaloneTrackedRootsWithDetails() ([]TrackedRootDetail, error) {
	query := `
	SELECT r.id, r.org, r.project, COALESCE(r.url, ''), COALESCE(r.query_id, ''), COALESCE(r.custom_title, ''), r.tracked_at,
	       COALESCE(w.title, ''),
	       COALESCE(w.work_item_type, ''), COALESCE(w.state, '')
	FROM tracked_roots r
	LEFT JOIN work_items w ON r.id = w.id
	WHERE r.query_id IS NULL OR r.query_id = ''
	ORDER BY r.tracked_at DESC, r.id DESC
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []TrackedRootDetail
	for rows.Next() {
		var r TrackedRootDetail
		if err := rows.Scan(&r.ID, &r.Org, &r.Project, &r.URL, &r.QueryID, &r.CustomTitle, &r.TrackedAt, &r.Title, &r.WorkItemType, &r.State); err != nil {
			return nil, err
		}
		roots = append(roots, r)
	}
	return roots, nil
}

// DeleteTrackedRoot removes a tracked root from tracking
func (d *DB) DeleteTrackedRoot(id int) error {
	_, err := d.conn.Exec("DELETE FROM tracked_roots WHERE id = ?", id)
	return err
}

// SaveWorkItem inserts or updates a single work item from ADO
func (d *DB) SaveWorkItem(org, project string, item *ado.WorkItem) error {
	assignedTo := item.AssignedTo()
	var assignedName, assignedEmail, assignedAvatar string
	if assignedTo != nil {
		assignedName = assignedTo.DisplayName
		assignedEmail = assignedTo.UniqueName
		assignedAvatar = assignedTo.ImageUrl
	}

	createdBy := item.CreatedBy()
	var createdName, createdEmail string
	if createdBy != nil {
		createdName = createdBy.DisplayName
		createdEmail = createdBy.UniqueName
	}

	changedBy := item.ChangedBy()
	var changedName string
	if changedBy != nil {
		changedName = changedBy.DisplayName
	}

	itemProject := item.GetStringField("System.TeamProject")
	if itemProject == "" {
		itemProject = project
	}

	query := `
	INSERT INTO work_items (
		id, org, project, work_item_type, title, state, reason,
		assigned_to_name, assigned_to_email, assigned_to_avatar,
		created_date, created_by_name, created_by_email,
		changed_date, changed_by_name,
		start_date, target_date, finish_date,
		description, acceptance_criteria, repro_steps,
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, area_path, iteration_path, tags, activity, parent_id, url, last_synced_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		org = excluded.org,
		project = excluded.project,
		work_item_type = excluded.work_item_type,
		title = excluded.title,
		state = excluded.state,
		reason = excluded.reason,
		assigned_to_name = excluded.assigned_to_name,
		assigned_to_email = excluded.assigned_to_email,
		assigned_to_avatar = excluded.assigned_to_avatar,
		created_date = excluded.created_date,
		created_by_name = excluded.created_by_name,
		created_by_email = excluded.created_by_email,
		changed_date = excluded.changed_date,
		changed_by_name = excluded.changed_by_name,
		start_date = excluded.start_date,
		target_date = excluded.target_date,
		finish_date = excluded.finish_date,
		description = excluded.description,
		acceptance_criteria = excluded.acceptance_criteria,
		repro_steps = excluded.repro_steps,
		original_estimate = excluded.original_estimate,
		remaining_work = excluded.remaining_work,
		completed_work = excluded.completed_work,
		effort = excluded.effort,
		story_points = excluded.story_points,
		priority = excluded.priority,
		area_path = excluded.area_path,
		iteration_path = excluded.iteration_path,
		tags = excluded.tags,
		activity = excluded.activity,
		parent_id = CASE WHEN excluded.parent_id > 0 THEN excluded.parent_id ELSE work_items.parent_id END,
		url = excluded.url,
		last_synced_at = excluded.last_synced_at;
	`

	webURL := ado.BuildWebURL(org, itemProject, item.ID)
	origEst, remWork, compWork, eff, storyPts := d.ExtractWorkItemEstimates(org, itemProject, item)

	_, err := d.conn.Exec(query,
		item.ID, org, itemProject, item.Type(), item.Title(), item.State(), item.Reason(),
		assignedName, assignedEmail, assignedAvatar,
		item.CreatedDate(), createdName, createdEmail,
		item.ChangedDate(), changedName,
		item.StartDate(), item.TargetDate(), item.FinishDate(),
		item.Description(), item.AcceptanceCriteria(), item.ReproSteps(),
		origEst, remWork, compWork, eff, storyPts,
		item.Priority(), item.AreaPath(), item.IterationPath(), item.Tags(), item.Activity(), item.ParentID(), webURL, time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	// Update relations if returned by API
	if len(item.Relations) > 0 {
		_ = d.SaveRelations(item.ID, item.Relations)
	}
	d.SaveIdentitiesFromItem(item)
	return nil
}

// ExtractWorkItemEstimates extracts original estimate, remaining work, completed work, effort, and story points
// using the dynamic project/type schema mappings from SQLite when available, falling back to standard field names.
func (d *DB) ExtractWorkItemEstimates(org, project string, item *ado.WorkItem) (orig, rem, comp, effort, points float64) {
	if item == nil {
		return 0, 0, 0, 0, 0
	}
	orig = item.OriginalEstimate()
	rem = item.RemainingWork()
	comp = item.CompletedWork()
	effort = item.Effort()
	points = item.StoryPoints()

	if d == nil || d.conn == nil {
		return
	}

	itemType := item.Type()
	if ref, ok := d.TypeGetEstimateField(org, project, itemType); ok && ref != "" {
		if _, exists := item.Fields[ref]; exists {
			orig = item.GetFloatField(ref)
		}
	}
	if ref, ok := d.TypeGetRemainingWorkField(org, project, itemType); ok && ref != "" {
		if _, exists := item.Fields[ref]; exists {
			rem = item.GetFloatField(ref)
		}
	}
	if ref, ok := d.TypeGetCompletedWorkField(org, project, itemType); ok && ref != "" {
		if _, exists := item.Fields[ref]; exists {
			comp = item.GetFloatField(ref)
		}
	}
	if ref, ok := d.TypeGetEffortField(org, project, itemType); ok && ref != "" {
		if _, exists := item.Fields[ref]; exists {
			effort = item.GetFloatField(ref)
		}
	}
	return
}

// SaveWorkItems inserts or updates a slice of work items in a single transaction
func (d *DB) SaveWorkItems(org, project string, items []*ado.WorkItem) error {
	if len(items) == 0 {
		return nil
	}

	// Pre-extract estimates and fields before opening the write transaction to prevent SQLite connection deadlocks
	type itemEstimates struct {
		orig, rem, comp, eff, story float64
	}
	estimatesList := make([]itemEstimates, len(items))
	for i, item := range items {
		if item == nil {
			continue
		}
		itemProject := item.GetStringField("System.TeamProject")
		if itemProject == "" {
			itemProject = project
		}
		orig, rem, comp, eff, story := d.ExtractWorkItemEstimates(org, itemProject, item)
		estimatesList[i] = itemEstimates{
			orig: orig, rem: rem, comp: comp, eff: eff, story: story,
		}
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT INTO work_items (
		id, org, project, work_item_type, title, state, reason,
		assigned_to_name, assigned_to_email, assigned_to_avatar,
		created_date, created_by_name, created_by_email,
		changed_date, changed_by_name,
		start_date, target_date, finish_date,
		description, acceptance_criteria, repro_steps,
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, area_path, iteration_path, tags, activity, parent_id, url, last_synced_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		org = excluded.org,
		project = excluded.project,
		work_item_type = excluded.work_item_type,
		title = excluded.title,
		state = excluded.state,
		reason = excluded.reason,
		assigned_to_name = excluded.assigned_to_name,
		assigned_to_email = excluded.assigned_to_email,
		assigned_to_avatar = excluded.assigned_to_avatar,
		created_date = excluded.created_date,
		created_by_name = excluded.created_by_name,
		created_by_email = excluded.created_by_email,
		changed_date = excluded.changed_date,
		changed_by_name = excluded.changed_by_name,
		start_date = excluded.start_date,
		target_date = excluded.target_date,
		finish_date = excluded.finish_date,
		description = excluded.description,
		acceptance_criteria = excluded.acceptance_criteria,
		repro_steps = excluded.repro_steps,
		original_estimate = excluded.original_estimate,
		remaining_work = excluded.remaining_work,
		completed_work = excluded.completed_work,
		effort = excluded.effort,
		story_points = excluded.story_points,
		priority = excluded.priority,
		area_path = excluded.area_path,
		iteration_path = excluded.iteration_path,
		tags = excluded.tags,
		activity = excluded.activity,
		parent_id = CASE WHEN excluded.parent_id > 0 THEN excluded.parent_id ELSE work_items.parent_id END,
		url = excluded.url,
		last_synced_at = excluded.last_synced_at;
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for i, item := range items {
		if item == nil {
			continue
		}
		assignedTo := item.AssignedTo()
		var assignedName, assignedEmail, assignedAvatar string
		if assignedTo != nil {
			assignedName = assignedTo.DisplayName
			assignedEmail = assignedTo.UniqueName
			assignedAvatar = assignedTo.ImageUrl
		}

		createdBy := item.CreatedBy()
		var createdName, createdEmail string
		if createdBy != nil {
			createdName = createdBy.DisplayName
			createdEmail = createdBy.UniqueName
		}

		changedBy := item.ChangedBy()
		var changedName string
		if changedBy != nil {
			changedName = changedBy.DisplayName
		}

		itemProject := item.GetStringField("System.TeamProject")
		if itemProject == "" {
			itemProject = project
		}

		webURL := ado.BuildWebURL(org, itemProject, item.ID)
		est := estimatesList[i]

		_, err := stmt.Exec(
			item.ID, org, itemProject, item.Type(), item.Title(), item.State(), item.Reason(),
			assignedName, assignedEmail, assignedAvatar,
			item.CreatedDate(), createdName, createdEmail,
			item.ChangedDate(), changedName,
			item.StartDate(), item.TargetDate(), item.FinishDate(),
			item.Description(), item.AcceptanceCriteria(), item.ReproSteps(),
			est.orig, est.rem, est.comp, est.eff, est.story,
			item.Priority(), item.AreaPath(), item.IterationPath(), item.Tags(), item.Activity(), item.ParentID(), webURL, now,
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, item := range items {
		if len(item.Relations) > 0 {
			_ = d.SaveRelations(item.ID, item.Relations)
		}
		d.SaveIdentitiesFromItem(item)
	}

	return nil
}

// SaveRelations saves or updates work item relations
func (d *DB) SaveRelations(sourceID int, relations []ado.WorkItemRelation) error {
	_, _ = d.conn.Exec("DELETE FROM relations WHERE source_id = ?", sourceID)

	query := `INSERT OR REPLACE INTO relations (source_id, target_id, rel_type) VALUES (?, ?, ?)`
	stmt, err := d.conn.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rel := range relations {
		targetID := 0
		if rel.URL != "" {
			parts := strings.Split(strings.TrimRight(rel.URL, "/"), "/")
			if len(parts) > 0 {
				if id, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
					targetID = id
				}
			}
		}
		if targetID > 0 && targetID != sourceID {
			_, _ = stmt.Exec(sourceID, targetID, rel.Rel)
		}
	}
	return nil
}

// GetWorkItem retrieves a single work item record by ID
func (d *DB) GetWorkItem(id int) (*WorkItemRecord, error) {
	query := `
	SELECT
		id, org, project, work_item_type, title, state, COALESCE(reason, ''),
		COALESCE(assigned_to_name, ''), COALESCE(assigned_to_email, ''), COALESCE(assigned_to_avatar, ''),
		COALESCE(created_date, ''), COALESCE(created_by_name, ''), COALESCE(created_by_email, ''),
		COALESCE(changed_date, ''), COALESCE(changed_by_name, ''),
		COALESCE(start_date, ''), COALESCE(target_date, ''), COALESCE(finish_date, ''),
		COALESCE(description, ''), COALESCE(acceptance_criteria, ''), COALESCE(repro_steps, ''),
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, COALESCE(area_path, ''), COALESCE(iteration_path, ''), COALESCE(tags, ''),
		COALESCE(activity, ''), parent_id, COALESCE(url, ''), last_synced_at
	FROM work_items WHERE id = ?
	`
	row := d.conn.QueryRow(query, id)

	var w WorkItemRecord
	err := row.Scan(
		&w.ID, &w.Org, &w.Project, &w.WorkItemType, &w.Title, &w.State, &w.Reason,
		&w.AssignedToName, &w.AssignedToEmail, &w.AssignedToAvatar,
		&w.CreatedDate, &w.CreatedByName, &w.CreatedByEmail,
		&w.ChangedDate, &w.ChangedByName,
		&w.StartDate, &w.TargetDate, &w.FinishDate,
		&w.Description, &w.AcceptanceCriteria, &w.ReproSteps,
		&w.OriginalEstimate, &w.RemainingWork, &w.CompletedWork, &w.Effort, &w.StoryPoints,
		&w.Priority, &w.AreaPath, &w.IterationPath, &w.Tags,
		&w.Activity, &w.ParentID, &w.URL, &w.LastSyncedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// GetAllWorkItems retrieves all work items in the database
func (d *DB) GetAllWorkItems() ([]*WorkItemRecord, error) {
	query := `
	SELECT
		id, org, project, work_item_type, title, state, COALESCE(reason, ''),
		COALESCE(assigned_to_name, ''), COALESCE(assigned_to_email, ''), COALESCE(assigned_to_avatar, ''),
		COALESCE(created_date, ''), COALESCE(created_by_name, ''), COALESCE(created_by_email, ''),
		COALESCE(changed_date, ''), COALESCE(changed_by_name, ''),
		COALESCE(start_date, ''), COALESCE(target_date, ''), COALESCE(finish_date, ''),
		COALESCE(description, ''), COALESCE(acceptance_criteria, ''), COALESCE(repro_steps, ''),
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, COALESCE(area_path, ''), COALESCE(iteration_path, ''), COALESCE(tags, ''),
		COALESCE(activity, ''), parent_id, COALESCE(url, ''), last_synced_at
	FROM work_items ORDER BY id ASC
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WorkItemRecord
	for rows.Next() {
		var w WorkItemRecord
		if err := rows.Scan(
			&w.ID, &w.Org, &w.Project, &w.WorkItemType, &w.Title, &w.State, &w.Reason,
			&w.AssignedToName, &w.AssignedToEmail, &w.AssignedToAvatar,
			&w.CreatedDate, &w.CreatedByName, &w.CreatedByEmail,
			&w.ChangedDate, &w.ChangedByName,
			&w.StartDate, &w.TargetDate, &w.FinishDate,
			&w.Description, &w.AcceptanceCriteria, &w.ReproSteps,
			&w.OriginalEstimate, &w.RemainingWork, &w.CompletedWork, &w.Effort, &w.StoryPoints,
			&w.Priority, &w.AreaPath, &w.IterationPath, &w.Tags,
			&w.Activity, &w.ParentID, &w.URL, &w.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, &w)
	}
	return items, nil
}

// GetChildWorkItems retrieves direct child work items for a given parent ID
func (d *DB) GetChildWorkItems(parentID int) ([]*WorkItemRecord, error) {
	query := `
	SELECT
		id, org, project, work_item_type, title, state, COALESCE(reason, ''),
		COALESCE(assigned_to_name, ''), COALESCE(assigned_to_email, ''), COALESCE(assigned_to_avatar, ''),
		COALESCE(created_date, ''), COALESCE(created_by_name, ''), COALESCE(created_by_email, ''),
		COALESCE(changed_date, ''), COALESCE(changed_by_name, ''),
		COALESCE(start_date, ''), COALESCE(target_date, ''), COALESCE(finish_date, ''),
		COALESCE(description, ''), COALESCE(acceptance_criteria, ''), COALESCE(repro_steps, ''),
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, COALESCE(area_path, ''), COALESCE(iteration_path, ''), COALESCE(tags, ''),
		COALESCE(activity, ''), parent_id, COALESCE(url, ''), last_synced_at
	FROM work_items WHERE parent_id = ? ORDER BY id ASC
	`
	rows, err := d.conn.Query(query, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WorkItemRecord
	for rows.Next() {
		var w WorkItemRecord
		if err := rows.Scan(
			&w.ID, &w.Org, &w.Project, &w.WorkItemType, &w.Title, &w.State, &w.Reason,
			&w.AssignedToName, &w.AssignedToEmail, &w.AssignedToAvatar,
			&w.CreatedDate, &w.CreatedByName, &w.CreatedByEmail,
			&w.ChangedDate, &w.ChangedByName,
			&w.StartDate, &w.TargetDate, &w.FinishDate,
			&w.Description, &w.AcceptanceCriteria, &w.ReproSteps,
			&w.OriginalEstimate, &w.RemainingWork, &w.CompletedWork, &w.Effort, &w.StoryPoints,
			&w.Priority, &w.AreaPath, &w.IterationPath, &w.Tags,
			&w.Activity, &w.ParentID, &w.URL, &w.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, &w)
	}
	return items, nil
}

// GetDescendantWorkItems recursively retrieves all descendant work items under parentID
func (d *DB) GetDescendantWorkItems(parentID int) ([]*WorkItemRecord, error) {
	allItems, err := d.GetAllWorkItems()
	if err != nil {
		return nil, err
	}

	childrenMap := make(map[int][]*WorkItemRecord)
	for _, item := range allItems {
		if item.ParentID > 0 {
			childrenMap[item.ParentID] = append(childrenMap[item.ParentID], item)
		}
	}

	// Also check relations table for Hierarchy-Forward links
	relRows, err := d.conn.Query(`SELECT source_id, target_id FROM relations WHERE rel_type = 'System.LinkTypes.Hierarchy-Forward'`)
	if err == nil {
		defer relRows.Close()
		itemMap := make(map[int]*WorkItemRecord, len(allItems))
		for _, it := range allItems {
			itemMap[it.ID] = it
		}
		for relRows.Next() {
			var src, tgt int
			if err := relRows.Scan(&src, &tgt); err == nil {
				if tgtItem, exists := itemMap[tgt]; exists {
					alreadyPresent := false
					for _, existing := range childrenMap[src] {
						if existing.ID == tgt {
							alreadyPresent = true
							break
						}
					}
					if !alreadyPresent {
						childrenMap[src] = append(childrenMap[src], tgtItem)
					}
				}
			}
		}
	}

	var descendants []*WorkItemRecord
	queue := []int{parentID}
	visited := map[int]bool{parentID: true}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, child := range childrenMap[curr] {
			if !visited[child.ID] {
				visited[child.ID] = true
				descendants = append(descendants, child)
				queue = append(queue, child.ID)
			}
		}
	}

	return descendants, nil
}

// GetAncestorWorkItems returns the list of ancestor work items from top-level root down to immediate parent
func (d *DB) GetAncestorWorkItems(itemID int) ([]*WorkItemRecord, error) {
	item, err := d.GetWorkItem(itemID)
	if err != nil || item == nil {
		return nil, err
	}
	var ancestors []*WorkItemRecord
	currID := item.ParentID
	visited := map[int]bool{itemID: true}

	for currID > 0 && !visited[currID] {
		visited[currID] = true
		parent, err := d.GetWorkItem(currID)
		if err != nil || parent == nil {
			break
		}
		ancestors = append([]*WorkItemRecord{parent}, ancestors...)
		currID = parent.ParentID
	}
	return ancestors, nil
}

// GetWorkItemSubtree retrieves the full hierarchy subtree under rootID as a HierarchyNode with rollups and a flat list of descendants
func (d *DB) GetWorkItemSubtree(rootID int) (*HierarchyNode, []*WorkItemRecord, error) {
	rootRec, err := d.GetWorkItem(rootID)
	if err != nil {
		return nil, nil, err
	}
	if rootRec == nil {
		return nil, nil, fmt.Errorf("work item #%d not found", rootID)
	}
	descendants, err := d.GetDescendantWorkItems(rootID)
	if err != nil {
		return nil, nil, err
	}
	treeNode := BuildSingleTree(rootRec, descendants)
	return treeNode.ToHierarchyNode(), descendants, nil
}

// GetNearestParentIteration returns the IterationPath of the immediate parent work item.
// It does not traverse past the immediate parent if it is empty.
func (d *DB) GetNearestParentIteration(parentID int) string {
	if parentID <= 0 {
		return ""
	}
	item, err := d.GetWorkItem(parentID)
	if err != nil || item == nil {
		return ""
	}
	return strings.TrimSpace(item.IterationPath)
}

// GetNearestParentArea returns the AreaPath of the immediate parent work item.
// It does not traverse past the immediate parent if it is empty.
func (d *DB) GetNearestParentArea(parentID int) string {
	if parentID <= 0 {
		return ""
	}
	item, err := d.GetWorkItem(parentID)
	if err != nil || item == nil {
		return ""
	}
	return strings.TrimSpace(item.AreaPath)
}

// GetNearestParentAreaAndIteration returns the AreaPath and IterationPath of the immediate parent.
func (d *DB) GetNearestParentAreaAndIteration(parentID int) (string, string) {
	return d.GetNearestParentArea(parentID), d.GetNearestParentIteration(parentID)
}

// DeleteWorkItem removes a work item and associated data from SQLite
func (d *DB) DeleteWorkItem(id int) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, _ = tx.Exec("DELETE FROM comments WHERE work_item_id = ?", id)
	_, _ = tx.Exec("DELETE FROM relations WHERE source_id = ? OR target_id = ?", id, id)
	_, _ = tx.Exec("DELETE FROM tracked_roots WHERE id = ?", id)
	_, _ = tx.Exec("DELETE FROM work_items WHERE id = ?", id)

	return tx.Commit()
}

// SaveComments inserts comments for a work item
func (d *DB) SaveComments(workItemID int, comments []*ado.CommentItem) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, _ = tx.Exec("DELETE FROM comments WHERE work_item_id = ?", workItemID)

	query := `INSERT INTO comments (work_item_id, comment_id, author_name, author_email, text, created_date) VALUES (?, ?, ?, ?, ?, ?)`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range comments {
		var authorName, authorEmail string
		if c.CreatedBy != nil {
			authorName = c.CreatedBy.DisplayName
			authorEmail = c.CreatedBy.UniqueName
		}
		_, err := stmt.Exec(workItemID, c.ID, authorName, authorEmail, c.Text, c.CreatedDate)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, c := range comments {
		if c.CreatedBy != nil && (c.CreatedBy.ID != "" || c.CreatedBy.UniqueName != "") {
			_ = d.SaveIdentity(c.CreatedBy.ID, c.CreatedBy.DisplayName, c.CreatedBy.UniqueName, c.CreatedBy.ImageUrl)
		}
		if c.ModifiedBy != nil && (c.ModifiedBy.ID != "" || c.ModifiedBy.UniqueName != "") {
			_ = d.SaveIdentity(c.ModifiedBy.ID, c.ModifiedBy.DisplayName, c.ModifiedBy.UniqueName, c.ModifiedBy.ImageUrl)
		}
		for _, m := range c.Mentions {
			if m != nil {
				if m.ResolvedUser != nil {
					_ = d.SaveIdentity(m.ResolvedUser.ID, m.ResolvedUser.DisplayName, m.ResolvedUser.UniqueName, m.ResolvedUser.ImageUrl)
				}
				if m.Person != nil {
					_ = d.SaveIdentity(m.Person.ID, m.Person.DisplayName, m.Person.UniqueName, m.Person.ImageUrl)
				}
			}
		}
	}

	return nil
}

// GetComments retrieves all stored comments for a work item (latest first)
func (d *DB) GetComments(workItemID int) ([]CommentRecord, error) {
	query := `SELECT id, work_item_id, comment_id, COALESCE(author_name, ''), COALESCE(author_email, ''), text, created_date FROM comments WHERE work_item_id = ? ORDER BY created_date DESC, comment_id DESC`
	rows, err := d.conn.Query(query, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CommentRecord
	for rows.Next() {
		var c CommentRecord
		if err := rows.Scan(&c.ID, &c.WorkItemID, &c.CommentID, &c.AuthorName, &c.AuthorEmail, &c.Text, &c.CreatedDate); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

// SaveIdentity stores an identity record in SQLite
func (d *DB) SaveIdentity(id, displayName, email, avatarUrl string) error {
	if id == "" && email == "" {
		return nil
	}
	if displayName == "" {
		if email != "" {
			displayName = email
		} else {
			displayName = id
		}
	}
	id = strings.ToLower(strings.TrimSpace(id))
	email = strings.ToLower(strings.TrimSpace(email))

	if id != "" {
		query := `INSERT INTO identities (id, display_name, email, avatar_url) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			display_name = excluded.display_name,
			email = CASE WHEN excluded.email != '' THEN excluded.email ELSE identities.email END,
			avatar_url = CASE WHEN excluded.avatar_url != '' THEN excluded.avatar_url ELSE identities.avatar_url END;`
		_, _ = d.conn.Exec(query, id, displayName, email, avatarUrl)
	}
	return nil
}

// SaveIdentitiesFromItem extracts and saves all identities present in a work item
func (d *DB) SaveIdentitiesFromItem(item *ado.WorkItem) {
	if item == nil {
		return
	}
	for _, idRef := range item.AllIdentities() {
		if idRef != nil && (idRef.ID != "" || idRef.UniqueName != "") {
			_ = d.SaveIdentity(idRef.ID, idRef.DisplayName, idRef.UniqueName, idRef.ImageUrl)
		}
	}
}

// SaveIdentities batch-upserts a slice of ADO identity references in a single transaction
func (d *DB) SaveIdentities(identities []*ado.IdentityRef) error {
	if len(identities) == 0 {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO identities (id, display_name, email, avatar_url) VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		display_name = excluded.display_name,
		email = CASE WHEN excluded.email != '' THEN excluded.email ELSE identities.email END,
		avatar_url = CASE WHEN excluded.avatar_url != '' THEN excluded.avatar_url ELSE identities.avatar_url END;`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	seen := make(map[string]bool)
	for _, idRef := range identities {
		if idRef == nil {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(idRef.ID))
		email := strings.ToLower(strings.TrimSpace(idRef.UniqueName))
		if id == "" && email == "" {
			continue
		}
		if id == "" {
			id = email
		}
		key := email
		if key == "" {
			key = id
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		disp := idRef.DisplayName
		if disp == "" {
			disp = email
		}
		_, _ = stmt.Exec(id, disp, email, idRef.ImageUrl)
	}

	return tx.Commit()
}


// GetIdentitiesMap returns a map of lowercased identity IDs and emails to display names
func (d *DB) GetIdentitiesMap() (map[string]string, error) {
	identities := make(map[string]string)
	rows, err := d.conn.Query("SELECT id, display_name, COALESCE(email, '') FROM identities")
	if err != nil {
		return identities, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, displayName, email string
		if err := rows.Scan(&id, &displayName, &email); err == nil {
			if id != "" {
				identities[strings.ToLower(id)] = displayName
			}
			if email != "" {
				identities[strings.ToLower(email)] = displayName
			}
		}
	}
	return identities, nil
}

// FindIdentity searches for an identity by exact ID, email, or display name (or prefix match)
func (d *DB) FindIdentity(query string) (*IdentityRecord, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("empty identity query")
	}
	qLower := strings.ToLower(q)

	row := d.conn.QueryRow(`
		SELECT id, display_name, COALESCE(email, ''), COALESCE(avatar_url, '')
		FROM identities
		WHERE LOWER(id) = ? OR LOWER(email) = ? OR LOWER(display_name) = ?
		LIMIT 1
	`, qLower, qLower, qLower)

	var id IdentityRecord
	if err := row.Scan(&id.ID, &id.DisplayName, &id.Email, &id.AvatarURL); err != nil {
		// Try prefix match on display name if exact match not found
		qPattern := qLower + "%"
		rowPrefix := d.conn.QueryRow(`
			SELECT id, display_name, COALESCE(email, ''), COALESCE(avatar_url, '')
			FROM identities
			WHERE LOWER(display_name) LIKE ?
			ORDER BY LENGTH(display_name) ASC
			LIMIT 1
		`, qPattern)
		if errPrefix := rowPrefix.Scan(&id.ID, &id.DisplayName, &id.Email, &id.AvatarURL); errPrefix != nil {
			return nil, err
		}
	}
	return &id, nil
}

// GetAllIdentities returns all cached identity records
func (d *DB) GetAllIdentities() ([]*IdentityRecord, error) {
	rows, err := d.conn.Query("SELECT id, display_name, COALESCE(email, ''), COALESCE(avatar_url, '') FROM identities ORDER BY display_name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*IdentityRecord
	for rows.Next() {
		var id IdentityRecord
		if err := rows.Scan(&id.ID, &id.DisplayName, &id.Email, &id.AvatarURL); err == nil {
			result = append(result, &id)
		}
	}
	return result, nil
}

// NormalizeSettingKey converts kebab-case and any variant into standardized snake_case
func NormalizeSettingKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	return strings.ReplaceAll(k, "-", "_")
}

// GetSetting retrieves a configuration value from the app_settings table
func (d *DB) GetSetting(key string) (string, error) {
	normKey := NormalizeSettingKey(key)
	var val string
	err := d.conn.QueryRow("SELECT value FROM app_settings WHERE key = ? OR key = ?", normKey, key).Scan(&val)
	if err != nil {
		return "", err
	}
	if normKey == "pat_token" || normKey == "gemini_api_key" || normKey == "gemini_model" || normKey == "default_org" || normKey == "default_project" || normKey == "open_in_browser_cmd" {
		val = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(val, "\r", ""), "\n", ""))
	}
	return val, nil
}

// SetSetting stores a configuration key-value pair in the app_settings table
func (d *DB) SetSetting(key, val string) error {
	normKey := NormalizeSettingKey(key)
	if normKey == "pat_token" || normKey == "gemini_api_key" || normKey == "gemini_model" || normKey == "default_org" || normKey == "default_project" || normKey == "open_in_browser_cmd" {
		val = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(val, "\r", ""), "\n", ""))
	}
	query := `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;`
	_, err := d.conn.Exec(query, normKey, val, time.Now().UTC())
	return err
}

// GetAllSettings returns all settings as a key-value map
func (d *DB) GetAllSettings() (map[string]string, error) {
	settings := make(map[string]string)
	rows, err := d.conn.Query("SELECT key, value FROM app_settings ORDER BY key ASC")
	if err != nil {
		return settings, err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			settings[k] = v
		}
	}
	return settings, nil
}

// UIState stores persistent UI navigation and layout state across sessions
type UIState struct {
	ActiveTabRootID  int                      `json:"active_tab_root_id"`
	ActiveTabIdx     int                      `json:"active_tab_idx"`
	SelectedItemID   int                      `json:"selected_item_id"`
	CollapsedNodeIDs []int                    `json:"collapsed_node_ids"`
	GeminiDockHeight int                      `json:"gemini_dock_height"`
	Filter           FilterOptions            `json:"filter"`
	TabFilters       map[string]FilterOptions `json:"tab_filters,omitempty"`
}

// GetUIState retrieves persisted UI state from SQLite
func (d *DB) GetUIState() (*UIState, error) {
	state := &UIState{
		GeminiDockHeight: 8,
		CollapsedNodeIDs: make([]int, 0),
		TabFilters:       make(map[string]FilterOptions),
	}

	if val, err := d.GetSetting("ui_active_tab_root_id"); err == nil && val != "" {
		if id, err := strconv.Atoi(val); err == nil {
			state.ActiveTabRootID = id
		}
	}
	if val, err := d.GetSetting("ui_active_tab_idx"); err == nil && val != "" {
		if idx, err := strconv.Atoi(val); err == nil {
			state.ActiveTabIdx = idx
		}
	}
	if val, err := d.GetSetting("ui_selected_item_id"); err == nil && val != "" {
		if id, err := strconv.Atoi(val); err == nil {
			state.SelectedItemID = id
		}
	}
	if val, err := d.GetSetting("gemini_dock_height"); err == nil && val != "" {
		if h, err := strconv.Atoi(val); err == nil && h > 0 {
			state.GeminiDockHeight = h
		}
	}
	if val, err := d.GetSetting("ui_tree_collapsed_nodes"); err == nil && val != "" {
		var ids []int
		if err := json.Unmarshal([]byte(val), &ids); err == nil {
			state.CollapsedNodeIDs = ids
		}
	}
	if val, err := d.GetSetting("ui_tree_filter"); err == nil && val != "" {
		var f FilterOptions
		if err := json.Unmarshal([]byte(val), &f); err == nil {
			state.Filter = f
		}
	}
	if val, err := d.GetSetting("ui_tab_filters"); err == nil && val != "" {
		var tf map[string]FilterOptions
		if err := json.Unmarshal([]byte(val), &tf); err == nil {
			state.TabFilters = tf
		}
	}

	return state, nil
}

// SaveUIState stores UI state into app_settings
func (d *DB) SaveUIState(state *UIState) error {
	if state == nil {
		return nil
	}
	if err := d.SetSetting("ui_active_tab_root_id", strconv.Itoa(state.ActiveTabRootID)); err != nil {
		return err
	}
	if err := d.SetSetting("ui_active_tab_idx", strconv.Itoa(state.ActiveTabIdx)); err != nil {
		return err
	}
	if err := d.SetSetting("ui_selected_item_id", strconv.Itoa(state.SelectedItemID)); err != nil {
		return err
	}
	if state.GeminiDockHeight > 0 {
		if err := d.SetSetting("gemini_dock_height", strconv.Itoa(state.GeminiDockHeight)); err != nil {
			return err
		}
	}
	if data, err := json.Marshal(state.CollapsedNodeIDs); err == nil {
		if err := d.SetSetting("ui_tree_collapsed_nodes", string(data)); err != nil {
			return err
		}
	}
	if data, err := json.Marshal(state.Filter); err == nil {
		if err := d.SetSetting("ui_tree_filter", string(data)); err != nil {
			return err
		}
	}
	if state.TabFilters != nil {
		if data, err := json.Marshal(state.TabFilters); err == nil {
			if err := d.SetSetting("ui_tab_filters", string(data)); err != nil {
				return err
			}
		}
	}
	return nil
}

// SavePullRequest stores a Git pull request in SQLite
func (d *DB) SavePullRequest(pr *ado.GitPullRequest, org, project string) error {
	if pr == nil || pr.PullRequestID == 0 {
		return nil
	}

	var repoID, repoName string
	if pr.Repository != nil {
		repoID = pr.Repository.ID
		repoName = pr.Repository.Name
	}
	if repoName == "" {
		repoName = project
	}

	var creatorName, creatorEmail string
	if pr.CreatedBy != nil {
		creatorName = pr.CreatedBy.DisplayName
		creatorEmail = pr.CreatedBy.UniqueName
	}

	webURL := ado.BuildPRWebURL(org, project, repoName, pr.PullRequestID)

	query := `
	INSERT INTO pull_requests (
		id, repository_id, repository_name, title, status,
		created_by_name, created_by_email, source_branch, target_branch,
		merge_status, url, last_synced_at
	) VALUES (
		?, ?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		repository_id = excluded.repository_id,
		repository_name = excluded.repository_name,
		title = excluded.title,
		status = excluded.status,
		created_by_name = excluded.created_by_name,
		created_by_email = excluded.created_by_email,
		source_branch = excluded.source_branch,
		target_branch = excluded.target_branch,
		merge_status = excluded.merge_status,
		url = excluded.url,
		last_synced_at = excluded.last_synced_at;
	`
	_, err := d.conn.Exec(query,
		pr.PullRequestID, repoID, repoName, pr.Title, pr.Status,
		creatorName, creatorEmail, pr.SourceBranch(), pr.TargetBranch(),
		pr.MergeStatus, webURL, time.Now().UTC(),
	)
	return err
}

// LinkWorkItemPullRequest records the link between a work item and a pull request
func (d *DB) LinkWorkItemPullRequest(workItemID, pullRequestID int) error {
	query := "INSERT OR IGNORE INTO work_item_pull_requests (work_item_id, pull_request_id) VALUES (?, ?)"
	_, err := d.conn.Exec(query, workItemID, pullRequestID)
	return err
}

// GetPullRequestsForWorkItem retrieves all PRs linked to a work item
func (d *DB) GetPullRequestsForWorkItem(workItemID int) ([]*PullRequestRecord, error) {
	query := `
	SELECT
		pr.id, pr.repository_id, COALESCE(pr.repository_name, ''),
		pr.title, pr.status,
		COALESCE(pr.created_by_name, ''), COALESCE(pr.created_by_email, ''),
		COALESCE(pr.source_branch, ''), COALESCE(pr.target_branch, ''),
		COALESCE(pr.merge_status, ''), COALESCE(pr.url, ''), pr.last_synced_at
	FROM pull_requests pr
	JOIN work_item_pull_requests link ON pr.id = link.pull_request_id
	WHERE link.work_item_id = ?
	ORDER BY pr.id DESC
	`
	rows, err := d.conn.Query(query, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []*PullRequestRecord
	for rows.Next() {
		var pr PullRequestRecord
		if err := rows.Scan(
			&pr.ID, &pr.RepositoryID, &pr.RepositoryName,
			&pr.Title, &pr.Status,
			&pr.CreatedByName, &pr.CreatedByEmail,
			&pr.SourceBranch, &pr.TargetBranch,
			&pr.MergeStatus, &pr.URL, &pr.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		prs = append(prs, &pr)
	}
	return prs, nil
}

// SearchPullRequests searches pull requests by ID, title, or branch
func (d *DB) SearchPullRequests(query string, limit int) ([]*PullRequestRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	q := strings.TrimSpace(query)
	qPattern := "%" + strings.ToLower(q) + "%"

	sqlQuery := `
	SELECT
		id, repository_id, COALESCE(repository_name, ''),
		title, status,
		COALESCE(created_by_name, ''), COALESCE(created_by_email, ''),
		COALESCE(source_branch, ''), COALESCE(target_branch, ''),
		COALESCE(merge_status, ''), COALESCE(url, ''), last_synced_at
	FROM pull_requests
	WHERE CAST(id AS TEXT) LIKE ? OR LOWER(title) LIKE ? OR LOWER(source_branch) LIKE ?
	ORDER BY id DESC
	LIMIT ?
	`
	rows, err := d.conn.Query(sqlQuery, qPattern, qPattern, qPattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []*PullRequestRecord
	for rows.Next() {
		var pr PullRequestRecord
		if err := rows.Scan(
			&pr.ID, &pr.RepositoryID, &pr.RepositoryName,
			&pr.Title, &pr.Status,
			&pr.CreatedByName, &pr.CreatedByEmail,
			&pr.SourceBranch, &pr.TargetBranch,
			&pr.MergeStatus, &pr.URL, &pr.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		prs = append(prs, &pr)
	}
	return prs, nil
}

// GetRelatedWorkItems returns all work items directly related or linked to the given ID
func (d *DB) GetRelatedWorkItems(workItemID int) ([]*WorkItemRecord, error) {
	query := `
	SELECT DISTINCT
		w.id, w.org, w.project, w.work_item_type, w.title, w.state, COALESCE(w.reason, ''),
		COALESCE(w.assigned_to_name, ''), COALESCE(w.assigned_to_email, ''), COALESCE(w.assigned_to_avatar, ''),
		COALESCE(w.created_date, ''), COALESCE(w.created_by_name, ''), COALESCE(w.created_by_email, ''),
		COALESCE(w.changed_date, ''), COALESCE(w.changed_by_name, ''),
		COALESCE(w.start_date, ''), COALESCE(w.target_date, ''), COALESCE(w.finish_date, ''),
		COALESCE(w.description, ''), COALESCE(w.acceptance_criteria, ''), COALESCE(w.repro_steps, ''),
		w.original_estimate, w.remaining_work, w.completed_work, w.effort, w.story_points,
		w.priority, COALESCE(w.area_path, ''), COALESCE(w.iteration_path, ''), COALESCE(w.tags, ''),
		COALESCE(w.activity, ''), w.parent_id, COALESCE(w.url, ''), w.last_synced_at
	FROM work_items w
	JOIN relations r ON (r.source_id = ? AND r.target_id = w.id) OR (r.target_id = ? AND r.source_id = w.id)
	WHERE w.id != ?
	ORDER BY w.id ASC
	`
	rows, err := d.conn.Query(query, workItemID, workItemID, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WorkItemRecord
	for rows.Next() {
		var w WorkItemRecord
		if err := rows.Scan(
			&w.ID, &w.Org, &w.Project, &w.WorkItemType, &w.Title, &w.State, &w.Reason,
			&w.AssignedToName, &w.AssignedToEmail, &w.AssignedToAvatar,
			&w.CreatedDate, &w.CreatedByName, &w.CreatedByEmail,
			&w.ChangedDate, &w.ChangedByName,
			&w.StartDate, &w.TargetDate, &w.FinishDate,
			&w.Description, &w.AcceptanceCriteria, &w.ReproSteps,
			&w.OriginalEstimate, &w.RemainingWork, &w.CompletedWork, &w.Effort, &w.StoryPoints,
			&w.Priority, &w.AreaPath, &w.IterationPath, &w.Tags,
			&w.Activity, &w.ParentID, &w.URL, &w.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, &w)
	}
	return items, nil
}

// SearchWorkItems searches work items by ID, title, or assignee
func (d *DB) SearchWorkItems(query string, limit int) ([]*WorkItemRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	q := strings.TrimSpace(query)
	qPattern := "%" + strings.ToLower(q) + "%"

	sqlQuery := `
	SELECT
		id, org, project, work_item_type, title, state, COALESCE(reason, ''),
		COALESCE(assigned_to_name, ''), COALESCE(assigned_to_email, ''), COALESCE(assigned_to_avatar, ''),
		COALESCE(created_date, ''), COALESCE(created_by_name, ''), COALESCE(created_by_email, ''),
		COALESCE(changed_date, ''), COALESCE(changed_by_name, ''),
		COALESCE(start_date, ''), COALESCE(target_date, ''), COALESCE(finish_date, ''),
		COALESCE(description, ''), COALESCE(acceptance_criteria, ''), COALESCE(repro_steps, ''),
		original_estimate, remaining_work, completed_work, effort, story_points,
		priority, COALESCE(area_path, ''), COALESCE(iteration_path, ''), COALESCE(tags, ''),
		COALESCE(activity, ''), parent_id, COALESCE(url, ''), last_synced_at
	FROM work_items
	WHERE CAST(id AS TEXT) LIKE ? OR LOWER(title) LIKE ? OR LOWER(assigned_to_name) LIKE ?
	ORDER BY id DESC
	LIMIT ?
	`
	rows, err := d.conn.Query(sqlQuery, qPattern, qPattern, qPattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WorkItemRecord
	for rows.Next() {
		var w WorkItemRecord
		if err := rows.Scan(
			&w.ID, &w.Org, &w.Project, &w.WorkItemType, &w.Title, &w.State, &w.Reason,
			&w.AssignedToName, &w.AssignedToEmail, &w.AssignedToAvatar,
			&w.CreatedDate, &w.CreatedByName, &w.CreatedByEmail,
			&w.ChangedDate, &w.ChangedByName,
			&w.StartDate, &w.TargetDate, &w.FinishDate,
			&w.Description, &w.AcceptanceCriteria, &w.ReproSteps,
			&w.OriginalEstimate, &w.RemainingWork, &w.CompletedWork, &w.Effort, &w.StoryPoints,
			&w.Priority, &w.AreaPath, &w.IterationPath, &w.Tags,
			&w.Activity, &w.ParentID, &w.URL, &w.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, &w)
	}
	return items, nil
}

// GetDistinctActivities retrieves all unique non-empty activity values recorded in the database
func (d *DB) GetDistinctActivities() ([]string, error) {
	query := `SELECT DISTINCT activity FROM work_items WHERE activity IS NOT NULL AND activity != '' ORDER BY activity ASC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []string
	for rows.Next() {
		var act string
		if err := rows.Scan(&act); err == nil && act != "" {
			activities = append(activities, act)
		}
	}
	return activities, nil
}

// UpdateWorkItemActivity updates the activity field for a work item in local database
func (d *DB) UpdateWorkItemActivity(id int, activity string) error {
	query := `UPDATE work_items SET activity = ?, last_synced_at = ? WHERE id = ?`
	_, err := d.conn.Exec(query, activity, time.Now().UTC(), id)
	return err
}

// SearchIdentities searches identities by name or email
func (d *DB) SearchIdentities(query string, limit int) ([]*IdentityRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	q := strings.TrimSpace(query)
	qPattern := "%" + strings.ToLower(q) + "%"

	sqlQuery := `
	SELECT id, display_name, COALESCE(email, ''), COALESCE(avatar_url, '')
	FROM identities
	WHERE LOWER(display_name) LIKE ? OR LOWER(email) LIKE ?
	ORDER BY display_name ASC
	`
	rows, err := d.conn.Query(sqlQuery, qPattern, qPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var identities []*IdentityRecord
	for rows.Next() {
		var id IdentityRecord
		if err := rows.Scan(&id.ID, &id.DisplayName, &id.Email, &id.AvatarURL); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(id.Email))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(id.DisplayName))
		}
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(id.ID))
		}
		if !seen[key] {
			seen[key] = true
			identities = append(identities, &id)
			if len(identities) >= limit {
				break
			}
		}
	}
	return identities, nil
}

// GetDistinctStates returns all unique states in the database
func (d *DB) GetDistinctStates() ([]string, error) {
	query := `SELECT DISTINCT state FROM work_items WHERE state != '' ORDER BY state ASC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, nil
}

// GetDistinctStatesForType returns unique states for a specific work item type in the database
func (d *DB) GetDistinctStatesForType(itemType string) ([]string, error) {
	query := `SELECT DISTINCT state FROM work_items WHERE work_item_type = ? AND state != '' ORDER BY state ASC`
	rows, err := d.conn.Query(query, itemType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, nil
}

// SaveWorkItemTypeStates caches the states for a specific work item type
func (d *DB) SaveWorkItemTypeStates(org, project, itemType string, states []ado.WorkItemState) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delQuery := `DELETE FROM work_item_type_states WHERE org = ? AND project = ? AND work_item_type = ?`
	if _, err := tx.Exec(delQuery, org, project, itemType); err != nil {
		return err
	}

	insQuery := `INSERT INTO work_item_type_states (org, project, work_item_type, state_name, category, color, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?)`
	for i, st := range states {
		if _, err := tx.Exec(insQuery, org, project, itemType, st.Name, st.Category, st.Color, i); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	d.InvalidateTypeSchemaCache(itemType)
	return nil
}

func (d *DB) getTypeStatesEntry(org, project, itemType string) *typeStatesCacheEntry {
	if d == nil || d.conn == nil || itemType == "" {
		return &typeStatesCacheEntry{stateCategories: make(map[string]string)}
	}

	key := makeTypeCacheKey(org, project, itemType)

	d.typeStatesMu.RLock()
	if d.typeStatesCache != nil {
		if entry, ok := d.typeStatesCache[key]; ok {
			d.typeStatesMu.RUnlock()
			return entry
		}
	}
	d.typeStatesMu.RUnlock()

	d.typeStatesMu.Lock()
	defer d.typeStatesMu.Unlock()

	if d.typeStatesCache == nil {
		d.typeStatesCache = make(map[string]*typeStatesCacheEntry)
	}
	if entry, ok := d.typeStatesCache[key]; ok {
		return entry
	}

	entry := &typeStatesCacheEntry{
		stateCategories: make(map[string]string),
	}

	var query string
	var rows *sql.Rows
	var err error
	if org != "" && project != "" {
		query = `SELECT state_name, category FROM work_item_type_states WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ORDER BY sort_order ASC`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT state_name, category FROM work_item_type_states WHERE LOWER(work_item_type) = LOWER(?) ORDER BY sort_order ASC`
		rows, err = d.conn.Query(query, itemType)
	}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var stateName string
			var cat sql.NullString
			if err := rows.Scan(&stateName, &cat); err == nil {
				entry.hasStates = true
				entry.states = append(entry.states, stateName)
				if cat.Valid && cat.String != "" {
					entry.stateCategories[strings.ToLower(stateName)] = cat.String
				}
			}
		}
	}

	d.typeStatesCache[key] = entry
	return entry
}

// GetWorkItemTypeStates retrieves cached states for a specific work item type
func (d *DB) GetWorkItemTypeStates(org, project, itemType string) ([]string, error) {
	entry := d.getTypeStatesEntry(org, project, itemType)
	if entry == nil || !entry.hasStates {
		return nil, nil
	}
	return entry.states, nil
}

// GetWorkItemStatesDetailed retrieves cached states with full details (category, color)
func (d *DB) GetWorkItemStatesDetailed(org, project, itemType string) ([]ado.WorkItemState, error) {
	if d == nil || d.conn == nil || itemType == "" {
		return nil, nil
	}
	var query string
	var rows *sql.Rows
	var err error
	if org != "" && project != "" {
		query = `SELECT state_name, COALESCE(category, ''), COALESCE(color, '') FROM work_item_type_states WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ORDER BY sort_order ASC`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT state_name, COALESCE(category, ''), COALESCE(color, '') FROM work_item_type_states WHERE LOWER(work_item_type) = LOWER(?) ORDER BY sort_order ASC`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []ado.WorkItemState
	for rows.Next() {
		var st ado.WorkItemState
		if err := rows.Scan(&st.Name, &st.Category, &st.Color); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}

// GetStateCategory retrieves the ADO category for a given state (Proposed, InProgress, Resolved, Completed, Removed)
func (d *DB) GetStateCategory(org, project, itemType, stateName string) (string, bool) {
	if d == nil || itemType == "" || stateName == "" {
		return "", false
	}
	entry := d.getTypeStatesEntry(org, project, itemType)
	if entry == nil {
		return "", false
	}
	cat, ok := entry.stateCategories[strings.ToLower(stateName)]
	return cat, ok
}

// IsStateCompleted dynamically checks if a work item state is completed/resolved/closed
// using ADO's state category from the database, with a keyword fallback if metadata is missing.
func (d *DB) IsStateCompleted(org, project, itemType, stateName string) bool {
	if stateName == "" {
		return false
	}
	if d != nil {
		if cat, ok := d.GetStateCategory(org, project, itemType, stateName); ok && cat != "" {
			c := strings.ToLower(cat)
			return c == "completed" || c == "resolved" || c == "removed"
		}
	}
	return ado.IsStateDoneOrClosed(stateName)
}

// HasWorkItemTypeStates checks if there are cached states for a specific work item type
func (d *DB) HasWorkItemTypeStates(org, project, itemType string) (bool, error) {
	if d == nil || itemType == "" {
		return false, nil
	}
	entry := d.getTypeStatesEntry(org, project, itemType)
	if entry == nil {
		return false, nil
	}
	return entry.hasStates, nil
}

// TypeHasField checks if a specific field is supported by the work item type schema
func (d *DB) TypeHasField(org, project, itemType, fieldRefName string) bool {
	if d == nil || d.conn == nil || itemType == "" || fieldRefName == "" {
		return false
	}
	var query string
	var rows *sql.Rows
	var err error
	if org != "" && project != "" {
		query = `SELECT 1 FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) AND LOWER(reference_name) = LOWER(?) LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType, fieldRefName)
	} else {
		query = `SELECT 1 FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) AND LOWER(reference_name) = LOWER(?) LIMIT 1`
		rows, err = d.conn.Query(query, itemType, fieldRefName)
	}
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

// SaveWorkItemTypeFields caches the fields schema for a specific work item type
func (d *DB) SaveWorkItemTypeFields(org, project, itemType string, fields []ado.WorkItemTypeField) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delQuery := `DELETE FROM work_item_type_fields WHERE org = ? AND project = ? AND work_item_type = ?`
	if _, err := tx.Exec(delQuery, org, project, itemType); err != nil {
		return err
	}

	insQuery := `INSERT INTO work_item_type_fields (org, project, work_item_type, reference_name, name, always_required)
VALUES (?, ?, ?, ?, ?, ?)`
	for _, f := range fields {
		req := 0
		if f.AlwaysRequired {
			req = 1
		}
		if _, err := tx.Exec(insQuery, org, project, itemType, f.ReferenceName, f.Name, req); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	d.InvalidateTypeSchemaCache(itemType)
	return nil
}

// GetWorkItemTypeFields retrieves cached field reference names for a specific work item type
func (d *DB) GetWorkItemTypeFields(org, project, itemType string) ([]string, error) {
	query := `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND work_item_type = ?`
	rows, err := d.conn.Query(query, org, project, itemType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

func (d *DB) getTypeFieldsEntry(org, project, itemType string) *typeFieldsCacheEntry {
	if d == nil || d.conn == nil || itemType == "" {
		return &typeFieldsCacheEntry{}
	}

	key := makeTypeCacheKey(org, project, itemType)

	d.typeFieldsMu.RLock()
	if d.typeFieldsCache != nil {
		if entry, ok := d.typeFieldsCache[key]; ok {
			d.typeFieldsMu.RUnlock()
			return entry
		}
	}
	d.typeFieldsMu.RUnlock()

	d.typeFieldsMu.Lock()
	defer d.typeFieldsMu.Unlock()

	if d.typeFieldsCache == nil {
		d.typeFieldsCache = make(map[string]*typeFieldsCacheEntry)
	}
	if entry, ok := d.typeFieldsCache[key]; ok {
		return entry
	}

	entry := &typeFieldsCacheEntry{}

	var rows *sql.Rows
	var err error
	if org != "" && project != "" {
		rows, err = d.conn.Query(`SELECT 1 FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) LIMIT 1`, org, project, itemType)
	} else {
		rows, err = d.conn.Query(`SELECT 1 FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) LIMIT 1`, itemType)
	}
	if err == nil {
		if rows.Next() {
			entry.hasFields = true
		}
		rows.Close()
	}

	if entry.hasFields {
		entry.effortField, entry.hasEffort = d.rawQueryEffortField(org, project, itemType)
		entry.estimateField, entry.hasEstimate = d.rawQueryEstimateField(org, project, itemType)
		entry.remainingField, entry.hasRemaining = d.rawQueryRemainingWorkField(org, project, itemType)
		entry.completedField, entry.hasCompleted = d.rawQueryCompletedWorkField(org, project, itemType)
		entry.startDateField, entry.hasStartDate = d.rawQueryStartDateField(org, project, itemType)
		entry.targetDateField, entry.hasTargetDate = d.rawQueryTargetDateField(org, project, itemType)
		entry.finishDateField, entry.hasFinishDate = d.rawQueryFinishDateField(org, project, itemType)
	}

	d.typeFieldsCache[key] = entry
	return entry
}

// HasWorkItemTypeFields checks if there are cached fields for a specific work item type
func (d *DB) HasWorkItemTypeFields(org, project, itemType string) (bool, error) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.hasFields, nil
}

// TypeGetEffortField retrieves the exact field reference name for Effort/StoryPoints for a work item type
func (d *DB) TypeGetEffortField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.effortField, entry.hasEffort
}

// TypeHasEffortField checks if a work item type dynamically contains an Effort or StoryPoints field in its schema
func (d *DB) TypeHasEffortField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetEffortField(org, project, itemType)
	return ok, nil
}

// TypeGetEstimateField retrieves the exact field reference name for Original Estimate for a work item type
func (d *DB) TypeGetEstimateField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.estimateField, entry.hasEstimate
}

// TypeHasEstimateField checks if a work item type dynamically contains an Original Estimate field in its schema
func (d *DB) TypeHasEstimateField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetEstimateField(org, project, itemType)
	return ok, nil
}

// TypeGetRemainingWorkField retrieves the exact field reference name for Remaining Work for a work item type
func (d *DB) TypeGetRemainingWorkField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.remainingField, entry.hasRemaining
}

// TypeHasRemainingWorkField checks if a work item type dynamically contains a Remaining Work field in its schema
func (d *DB) TypeHasRemainingWorkField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetRemainingWorkField(org, project, itemType)
	return ok, nil
}

// ItemFieldPermissions describes field visibility and reference names resolved dynamically from ADO metadata
type ItemFieldPermissions struct {
	ShowEffort      bool
	ShowOriginal    bool
	ShowRemaining   bool
	ShowCompleted   bool
	FieldEffort     string
	FieldOriginal   string
	FieldRemaining  string
	FieldCompleted  string
	IsStateComplete bool
}

// ResolveItemFieldPermissions resolves field permissions dynamically from the database based on type schema and state category.
// Returns (permissions, isCached). isCached is false if the type fields or states are not yet present in the database.
func (d *DB) ResolveItemFieldPermissions(org, project, itemType, state string) (ItemFieldPermissions, bool) {
	perms := ItemFieldPermissions{
		FieldEffort:    "Microsoft.VSTS.Scheduling.Effort",
		FieldOriginal:  "Microsoft.VSTS.Scheduling.OriginalEstimate",
		FieldRemaining: "Microsoft.VSTS.Scheduling.RemainingWork",
		FieldCompleted: "Microsoft.VSTS.Scheduling.CompletedWork",
	}
	if d == nil || d.conn == nil || itemType == "" {
		return perms, false
	}

	hasFields, _ := d.HasWorkItemTypeFields(org, project, itemType)
	hasStates, _ := d.HasWorkItemTypeStates(org, project, itemType)
	isCached := hasFields && hasStates

	isDone := d.IsStateCompleted(org, project, itemType, state)
	perms.IsStateComplete = isDone

	if hasFields {
		if ref, ok := d.TypeGetEffortField(org, project, itemType); ok {
			perms.ShowEffort = true
			perms.FieldEffort = ref
		}
		if ref, ok := d.TypeGetEstimateField(org, project, itemType); ok {
			perms.ShowOriginal = true
			perms.FieldOriginal = ref
		}
		if ref, ok := d.TypeGetCompletedWorkField(org, project, itemType); ok {
			perms.ShowCompleted = true
			perms.FieldCompleted = ref
		}
		if ref, ok := d.TypeGetRemainingWorkField(org, project, itemType); ok {
			perms.ShowRemaining = !isDone
			perms.FieldRemaining = ref
		}
	} else {
		// When no schema is cached yet, all 4 fields are initially shown (RemainingWork suppressed if Done)
		perms.ShowEffort = true
		perms.ShowOriginal = true
		perms.ShowCompleted = true
		perms.ShowRemaining = !isDone
	}

	return perms, isCached
}

// TypeGetCompletedWorkField retrieves the exact field reference name for Completed Work for a work item type
func (d *DB) TypeGetCompletedWorkField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.completedField, entry.hasCompleted
}

// TypeHasCompletedWorkField checks if a work item type dynamically contains a Completed Work field in its schema
func (d *DB) TypeHasCompletedWorkField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetCompletedWorkField(org, project, itemType)
	return ok, nil
}

// TypeGetStartDateField retrieves the exact field reference name for Start Date for a work item type
func (d *DB) TypeGetStartDateField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.startDateField, entry.hasStartDate
}

// TypeHasStartDateField checks if a work item type dynamically contains a Start Date field in its schema
func (d *DB) TypeHasStartDateField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetStartDateField(org, project, itemType)
	return ok, nil
}

// TypeGetTargetDateField retrieves the exact field reference name for Target Date / Due Date for a work item type
func (d *DB) TypeGetTargetDateField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.targetDateField, entry.hasTargetDate
}

// TypeHasTargetDateField checks if a work item type dynamically contains a Target Date / Due Date field in its schema
func (d *DB) TypeHasTargetDateField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetTargetDateField(org, project, itemType)
	return ok, nil
}

// TypeGetFinishDateField retrieves the exact field reference name for Finish Date for a work item type
func (d *DB) TypeGetFinishDateField(org, project, itemType string) (string, bool) {
	entry := d.getTypeFieldsEntry(org, project, itemType)
	return entry.finishDateField, entry.hasFinishDate
}

// TypeHasFinishDateField checks if a work item type dynamically contains a Finish Date field in its schema
func (d *DB) TypeHasFinishDateField(org, project, itemType string) (bool, error) {
	_, ok := d.TypeGetFinishDateField(org, project, itemType)
	return ok, nil
}

func (d *DB) rawQueryEffortField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%effort%' OR 
		LOWER(reference_name) LIKE '%storypoint%' OR 
		LOWER(reference_name) LIKE '%story_point%' OR 
		LOWER(name) LIKE '%effort%' OR 
		LOWER(name) LIKE '%story point%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.Effort' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.StoryPoints' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.Effort' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.StoryPoints' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryEstimateField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%originalestimate%' OR 
		LOWER(reference_name) LIKE '%original_estimate%' OR 
		LOWER(name) LIKE '%original estimate%' OR
		(LOWER(reference_name) LIKE '%estimate%' AND LOWER(reference_name) NOT LIKE '%cost%')
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.OriginalEstimate' THEN 1 WHEN reference_name = 'Custom.OriginalEstimate' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.OriginalEstimate' THEN 1 WHEN reference_name = 'Custom.OriginalEstimate' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryRemainingWorkField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%remainingwork%' OR 
		LOWER(reference_name) LIKE '%remaining_work%' OR 
		LOWER(name) LIKE '%remaining work%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.RemainingWork' THEN 1 WHEN reference_name = 'Custom.RemainingWork' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.RemainingWork' THEN 1 WHEN reference_name = 'Custom.RemainingWork' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryCompletedWorkField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%completedwork%' OR 
		LOWER(reference_name) LIKE '%completed_work%' OR 
		LOWER(name) LIKE '%completed work%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.CompletedWork' THEN 1 WHEN reference_name = 'Custom.CompletedWork' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.CompletedWork' THEN 1 WHEN reference_name = 'Custom.CompletedWork' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryStartDateField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%startdate%' OR 
		LOWER(reference_name) LIKE '%start_date%' OR 
		LOWER(name) LIKE '%start date%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.StartDate' THEN 1 WHEN reference_name = 'Custom.StartDate' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.StartDate' THEN 1 WHEN reference_name = 'Custom.StartDate' THEN 2 ELSE 3 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryTargetDateField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%targetdate%' OR 
		LOWER(reference_name) LIKE '%target_date%' OR 
		LOWER(reference_name) LIKE '%duedate%' OR 
		LOWER(name) LIKE '%target date%' OR 
		LOWER(name) LIKE '%due date%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.TargetDate' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.DueDate' THEN 2 WHEN reference_name = 'Custom.TargetDate' THEN 3 WHEN reference_name = 'Custom.DueDate' THEN 4 ELSE 5 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.TargetDate' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.DueDate' THEN 2 WHEN reference_name = 'Custom.TargetDate' THEN 3 WHEN reference_name = 'Custom.DueDate' THEN 4 ELSE 5 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

func (d *DB) rawQueryFinishDateField(org, project, itemType string) (string, bool) {
	var query string
	var rows *sql.Rows
	var err error

	filter := `AND (
		LOWER(reference_name) LIKE '%finishdate%' OR 
		LOWER(reference_name) LIKE '%finish_date%' OR 
		LOWER(name) LIKE '%finish date%'
	)`

	if org != "" && project != "" {
		query = `SELECT reference_name FROM work_item_type_fields WHERE org = ? AND project = ? AND LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.FinishDate' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.CompletedDate' THEN 2 WHEN reference_name = 'Custom.FinishDate' THEN 3 ELSE 4 END LIMIT 1`
		rows, err = d.conn.Query(query, org, project, itemType)
	} else {
		query = `SELECT reference_name FROM work_item_type_fields WHERE LOWER(work_item_type) = LOWER(?) ` + filter + ` ORDER BY CASE WHEN reference_name = 'Microsoft.VSTS.Scheduling.FinishDate' THEN 1 WHEN reference_name = 'Microsoft.VSTS.Scheduling.CompletedDate' THEN 2 WHEN reference_name = 'Custom.FinishDate' THEN 3 ELSE 4 END LIMIT 1`
		rows, err = d.conn.Query(query, itemType)
	}
	if err != nil {
		return "", false
	}
	defer rows.Close()
	if rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err == nil && ref != "" {
			return ref, true
		}
	}
	return "", false
}

// GetDistinctWorkItemTypes returns all unique work item types in the database
func (d *DB) GetDistinctWorkItemTypes() ([]string, error) {
	query := `SELECT DISTINCT work_item_type FROM work_items WHERE work_item_type != '' ORDER BY work_item_type ASC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, nil
}

// GetDistinctIterationPaths returns all unique iteration paths in the database
func (d *DB) GetDistinctIterationPaths() ([]string, error) {
	query := `SELECT DISTINCT iteration_path FROM work_items WHERE iteration_path != '' ORDER BY iteration_path ASC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var iterations []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		iterations = append(iterations, s)
	}
	return iterations, nil
}

// GetDistinctAreaPaths returns all unique area paths in the database
func (d *DB) GetDistinctAreaPaths() ([]string, error) {
	query := `SELECT DISTINCT area_path FROM work_items WHERE area_path != '' ORDER BY area_path ASC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		areas = append(areas, s)
	}
	return areas, nil
}

// GetDistinctTags returns all unique individual tags in the database
func (d *DB) GetDistinctTags() ([]string, error) {
	query := `SELECT DISTINCT tags FROM work_items WHERE tags != ''`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tagMap := make(map[string]bool)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		parts := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ';' || r == ','
		})
		for _, p := range parts {
			clean := strings.TrimSpace(p)
			if clean != "" {
				tagMap[clean] = true
			}
		}
	}

	var tags []string
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
}

// GetDistinctAssignees returns all unique assignees in the database
func (d *DB) GetDistinctAssignees() ([]IdentityRecord, error) {
	query := `
	SELECT DISTINCT display_name, email FROM (
		SELECT display_name, email FROM identities WHERE display_name != ''
		UNION
		SELECT assigned_to_name AS display_name, assigned_to_email AS email FROM work_items WHERE assigned_to_name != ''
	)
	ORDER BY display_name ASC
	`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignees []IdentityRecord
	for rows.Next() {
		var a IdentityRecord
		if err := rows.Scan(&a.DisplayName, &a.Email); err != nil {
			return nil, err
		}
		assignees = append(assignees, a)
	}
	return assignees, nil
}

// ResolveAssignee resolves an input name/email/string to a canonical email or identity value suitable for ADO API
func (d *DB) ResolveAssignee(input string) (emailOrName string, isUnassign bool) {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "@")
	if input == "" || strings.EqualFold(input, "unassigned") || strings.EqualFold(input, "none") || strings.EqualFold(input, "[all]") {
		return "", true
	}

	// 1. If input contains <email@domain>, extract the email
	if idxStart := strings.Index(input, "<"); idxStart != -1 {
		if idxEnd := strings.Index(input[idxStart:], ">"); idxEnd != -1 {
			extracted := strings.TrimSpace(input[idxStart+1 : idxStart+idxEnd])
			if strings.Contains(extracted, "@") {
				return extracted, false
			}
		}
	}

	// 2. If already a clean email, return as-is
	if strings.Contains(input, "@") && !strings.Contains(input, " ") {
		return input, false
	}

	// 3. Search in DB identities table
	if d != nil {
		if identities, err := d.SearchIdentities(input, 20); err == nil && len(identities) > 0 {
			// First look for exact match on display name
			for _, id := range identities {
				if strings.EqualFold(strings.TrimSpace(id.DisplayName), input) {
					if id.Email != "" {
						return id.Email, false
					}
					return id.DisplayName, false
				}
			}
			// Look for partial match with email
			for _, id := range identities {
				if id.Email != "" {
					return id.Email, false
				}
			}
			// Fallback to first matched display name
			if identities[0].DisplayName != "" {
				return identities[0].DisplayName, false
			}
		}

		// Also check distinct work item assignees
		if assignees, err := d.GetDistinctAssignees(); err == nil {
			for _, a := range assignees {
				if strings.EqualFold(strings.TrimSpace(a.DisplayName), input) {
					if a.Email != "" {
						return a.Email, false
					}
					return a.DisplayName, false
				}
				if strings.Contains(strings.ToLower(a.DisplayName), strings.ToLower(input)) {
					if a.Email != "" {
						return a.Email, false
					}
				}
			}
		}
	}

	return input, false
}



package db

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// TreeNode represents a node in the hierarchical work item tree
type TreeNode struct {
	Record                 *WorkItemRecord
	Children               []*TreeNode
	Parent                 *TreeNode
	Depth                  int
	Expanded               bool
	IsQuery                bool
	QueryID                string
	CustomTitle            string
	RollupOriginalEstimate float64
	RollupCompletedWork    float64
	RollupRemainingWork    float64
	RollupEffort           float64
	TotalItemsCount        int
	DoneItemsCount         int
}

// TabTitle returns the tab display title (CustomTitle if set, otherwise Record.Title)
func (n *TreeNode) TabTitle() string {
	if n == nil {
		return ""
	}
	if n.CustomTitle != "" {
		return n.CustomTitle
	}
	if n.Record != nil {
		return n.Record.Title
	}
	return ""
}

// TreeRow represents a single line in the flattened visual tree
type TreeRow struct {
	Node   *TreeNode
	Depth  int
	Indent string
	Branch string
	Glyph  string
	Prefix string
}

// BuildTree constructs the tree hierarchy from database records, tracked roots, and tracked queries.
// Tracked queries are represented as top-level root tree containers named after the query.
// Standalone tracked roots are represented as top-level work item root trees.
func BuildTree(items []*WorkItemRecord, trackedRoots []TrackedRoot, trackedQueries ...[]TrackedQuery) []*TreeNode {
	start := time.Now()
	var rootNodes []*TreeNode
	defer func() {
		slog.Debug("Tree built",
			slog.Int("total_items", len(items)),
			slog.Int("roots_count", len(rootNodes)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	}()

	var queries []TrackedQuery
	if len(trackedQueries) > 0 {
		queries = trackedQueries[0]
	}

	if len(items) == 0 && len(trackedRoots) == 0 && len(queries) == 0 {
		return nil
	}

	itemMap := make(map[int]*WorkItemRecord, len(items))
	childrenMap := make(map[int][]*WorkItemRecord)

	for _, item := range items {
		itemMap[item.ID] = item
		if item.ParentID > 0 {
			childrenMap[item.ParentID] = append(childrenMap[item.ParentID], item)
		}
	}

	// Helper to recursively build an independent subtree
	var buildSubtree func(rec *WorkItemRecord, parent *TreeNode, depth int, visited map[int]bool) *TreeNode
	buildSubtree = func(rec *WorkItemRecord, parent *TreeNode, depth int, visited map[int]bool) *TreeNode {
		if visited[rec.ID] || depth > 50 {
			return nil
		}
		visited[rec.ID] = true

		node := &TreeNode{
			Record:   rec,
			Parent:   parent,
			Depth:    depth,
			Expanded: true, // Expanded by default
			Children: make([]*TreeNode, 0),
		}

		childRecs := childrenMap[rec.ID]
		for _, childRec := range childRecs {
			childVisited := make(map[int]bool, len(visited)+1)
			for k, v := range visited {
				childVisited[k] = v
			}
			childNode := buildSubtree(childRec, node, depth+1, childVisited)
			if childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}

		return node
	}

	queryRootsMap := make(map[string][]TrackedRoot)
	var standaloneRoots []TrackedRoot

	for _, tr := range trackedRoots {
		if tr.QueryID != "" {
			queryRootsMap[tr.QueryID] = append(queryRootsMap[tr.QueryID], tr)
		} else {
			standaloneRoots = append(standaloneRoots, tr)
		}
	}

	// 1. Build Query Root Trees
	for _, tq := range queries {
		queryRec := &WorkItemRecord{
			ID:           0,
			Org:          tq.Org,
			Project:      tq.Project,
			WorkItemType: "Query",
			Title:        tq.Name,
			State:        "Active",
			URL:          tq.URL,
		}

		queryNode := &TreeNode{
			Record:   queryRec,
			Depth:    0,
			Expanded: true,
			IsQuery:  true,
			QueryID:  tq.ID,
			Children: make([]*TreeNode, 0),
		}

		seenQueryItems := make(map[int]bool)
		for _, tr := range queryRootsMap[tq.ID] {
			if seenQueryItems[tr.ID] {
				continue
			}
			rootRec, exists := itemMap[tr.ID]
			if !exists {
				continue
			}
			// If rootRec's parent exists in itemMap, it is already nested under its parent's subtree.
			if rootRec.ParentID > 0 && itemMap[rootRec.ParentID] != nil {
				continue
			}
			seenQueryItems[tr.ID] = true
			childNode := buildSubtree(rootRec, queryNode, 1, make(map[int]bool))
			if childNode != nil {
				queryNode.Children = append(queryNode.Children, childNode)
			}
		}

		// If no specific tracked roots found for query, attach matching project root items whose parent is not in itemMap
		if len(queryNode.Children) == 0 {
			for _, it := range items {
				if (it.ParentID == 0 || itemMap[it.ParentID] == nil) && (it.Project == tq.Project || tq.Project == "") {
					if !seenQueryItems[it.ID] {
						seenQueryItems[it.ID] = true
						childNode := buildSubtree(it, queryNode, 1, make(map[int]bool))
						if childNode != nil {
							queryNode.Children = append(queryNode.Children, childNode)
						}
					}
				}
			}
		}

		if len(queryNode.Children) > 0 {
			computeRollupsAndDepths(queryNode, 0)
			rootNodes = append(rootNodes, queryNode)
		}
	}

	// 2. Build Standalone Root Trees
	seenRoots := make(map[int]bool)
	for _, tr := range standaloneRoots {
		if seenRoots[tr.ID] {
			continue
		}
		rootRec, exists := itemMap[tr.ID]
		if !exists {
			continue
		}
		seenRoots[tr.ID] = true
		rootNode := buildSubtree(rootRec, nil, 0, make(map[int]bool))
		if rootNode != nil {
			rootNode.CustomTitle = tr.CustomTitle
			computeRollupsAndDepths(rootNode, 0)
			rootNodes = append(rootNodes, rootNode)
		}
	}

	return rootNodes
}

func isStateDone(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	return s == "done" || s == "closed" || s == "resolved" || s == "completed"
}

func computeRollupsAndDepths(node *TreeNode, depth int) {
	node.Depth = depth

	if node.IsQuery || (node.Record != nil && node.Record.WorkItemType == "Query") {
		node.TotalItemsCount = 0
		node.DoneItemsCount = 0
		node.RollupOriginalEstimate = 0
		node.RollupCompletedWork = 0
		node.RollupRemainingWork = 0
		node.RollupEffort = 0

		var childOrig, childComp, childRem, childEff float64
		var childTotal, childDone int

		for _, child := range node.Children {
			computeRollupsAndDepths(child, depth+1)
			childOrig += child.RollupOriginalEstimate
			childComp += child.RollupCompletedWork
			childRem += child.RollupRemainingWork
			childEff += child.RollupEffort
			childTotal += child.TotalItemsCount
			childDone += child.DoneItemsCount
		}

		node.RollupOriginalEstimate = childOrig
		node.RollupCompletedWork = childComp
		node.RollupRemainingWork = childRem
		node.RollupEffort = childEff
		node.TotalItemsCount = childTotal
		node.DoneItemsCount = childDone
		return
	}

	// Initialize with own values
	node.RollupOriginalEstimate = node.Record.OriginalEstimate
	node.RollupCompletedWork = node.Record.CompletedWork
	node.RollupRemainingWork = node.Record.RemainingWork
	node.RollupEffort = node.Record.Effort
	node.TotalItemsCount = 1

	if isStateDone(node.Record.State) {
		node.DoneItemsCount = 1
	} else {
		node.DoneItemsCount = 0
	}

	if len(node.Children) == 0 {
		return
	}

	var childOrig, childComp, childRem, childEff float64
	var childTotal, childDone int

	for _, child := range node.Children {
		computeRollupsAndDepths(child, depth+1)
		childOrig += child.RollupOriginalEstimate
		childComp += child.RollupCompletedWork
		childRem += child.RollupRemainingWork
		childEff += child.RollupEffort
		childTotal += child.TotalItemsCount
		childDone += child.DoneItemsCount
	}

	// If children have estimates, use children's sum (children represent the breakdown of the parent)
	if childOrig > 0 {
		node.RollupOriginalEstimate = childOrig
	}
	if childComp > 0 {
		node.RollupCompletedWork = childComp
	}
	if childRem > 0 {
		node.RollupRemainingWork = childRem
	}
	if childEff > 0 && node.RollupEffort == 0 {
		node.RollupEffort = childEff
	}

	node.TotalItemsCount += childTotal
	node.DoneItemsCount += childDone
}

// FilterOptions defines the criteria for filtering the visual tree
type FilterOptions struct {
	Status     string   `json:"status,omitempty"`     // Single status (backward compatibility)
	Statuses   []string `json:"statuses,omitempty"`   // Multi-select statuses
	Op1        string   `json:"op1,omitempty"`
	Assignee   string   `json:"assignee,omitempty"`   // Single assignee (backward compatibility)
	Assignees  []string `json:"assignees,omitempty"`  // Multi-select assignees
	Op2        string   `json:"op2,omitempty"`
	ItemType   string   `json:"item_type,omitempty"`  // Single type (backward compatibility)
	ItemTypes  []string `json:"item_types,omitempty"` // Multi-select types
	Op3        string   `json:"op3,omitempty"`
	SearchTerm string   `json:"search_term,omitempty"`
}

// GetEffectiveStatuses returns the active list of status filters
func (f FilterOptions) GetEffectiveStatuses() []string {
	if len(f.Statuses) > 0 {
		var list []string
		for _, s := range f.Statuses {
			s = strings.TrimSpace(s)
			if s != "" && !strings.EqualFold(s, "[all]") && !strings.EqualFold(s, "all") {
				list = append(list, s)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	if f.Status != "" && !strings.EqualFold(f.Status, "[all]") && !strings.EqualFold(f.Status, "all") {
		return []string{strings.TrimSpace(f.Status)}
	}
	return nil
}

// GetEffectiveAssignees returns the active list of assignee filters
func (f FilterOptions) GetEffectiveAssignees() []string {
	if len(f.Assignees) > 0 {
		var list []string
		for _, a := range f.Assignees {
			a = strings.TrimSpace(a)
			if a != "" && !strings.EqualFold(a, "[all]") && !strings.EqualFold(a, "all") {
				list = append(list, a)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	if f.Assignee != "" && !strings.EqualFold(f.Assignee, "[all]") && !strings.EqualFold(f.Assignee, "all") {
		return []string{strings.TrimSpace(f.Assignee)}
	}
	return nil
}

// GetEffectiveItemTypes returns the active list of item type filters
func (f FilterOptions) GetEffectiveItemTypes() []string {
	if len(f.ItemTypes) > 0 {
		var list []string
		for _, t := range f.ItemTypes {
			t = strings.TrimSpace(t)
			if t != "" && !strings.EqualFold(t, "[all]") && !strings.EqualFold(t, "all") {
				list = append(list, t)
			}
		}
		if len(list) > 0 {
			return list
		}
	}
	if f.ItemType != "" && !strings.EqualFold(f.ItemType, "[all]") && !strings.EqualFold(f.ItemType, "all") {
		return []string{strings.TrimSpace(f.ItemType)}
	}
	return nil
}

func (f FilterOptions) IsActive() bool {
	if len(f.GetEffectiveStatuses()) > 0 {
		return true
	}
	if len(f.GetEffectiveAssignees()) > 0 {
		return true
	}
	if len(f.GetEffectiveItemTypes()) > 0 {
		return true
	}
	if strings.TrimSpace(f.SearchTerm) != "" {
		return true
	}
	return false
}

func (f FilterOptions) MatchesItem(rec *WorkItemRecord) bool {
	if rec == nil {
		return false
	}
	if !f.IsActive() {
		return true
	}

	type activeCriterion struct {
		matches bool
		opAfter string // Operator to the next active criterion
	}

	var active []activeCriterion

	op1 := f.Op1
	if op1 == "" {
		op1 = "AND"
	}
	op2 := f.Op2
	if op2 == "" {
		op2 = "AND"
	}
	op3 := f.Op3
	if op3 == "" {
		op3 = "AND"
	}

	// 1. Statuses
	if statuses := f.GetEffectiveStatuses(); len(statuses) > 0 {
		var matches bool
		for _, s := range statuses {
			if strings.EqualFold(rec.State, s) {
				matches = true
				break
			}
		}
		active = append(active, activeCriterion{matches: matches, opAfter: op1})
	}

	// 2. Assignees
	if assignees := f.GetEffectiveAssignees(); len(assignees) > 0 {
		var matches bool
		for _, a := range assignees {
			if strings.EqualFold(a, "unassigned") {
				if rec.AssignedToEmail == "" && rec.AssignedToName == "" {
					matches = true
					break
				}
			} else {
				cleanName := a
				cleanEmail := ""
				if strings.Contains(a, "<") && strings.HasSuffix(a, ">") {
					parts := strings.SplitN(a, "<", 2)
					cleanName = strings.TrimSpace(parts[0])
					cleanEmail = strings.TrimSpace(strings.TrimSuffix(parts[1], ">"))
				}

				if strings.EqualFold(rec.AssignedToName, cleanName) ||
					strings.EqualFold(rec.AssignedToEmail, cleanName) ||
					(cleanEmail != "" && strings.EqualFold(rec.AssignedToEmail, cleanEmail)) ||
					strings.Contains(strings.ToLower(rec.AssignedToName), strings.ToLower(cleanName)) {
					matches = true
					break
				}
			}
		}
		active = append(active, activeCriterion{matches: matches, opAfter: op2})
	}

	// 3. ItemTypes
	if types := f.GetEffectiveItemTypes(); len(types) > 0 {
		var matches bool
		for _, t := range types {
			if strings.EqualFold(rec.WorkItemType, t) {
				matches = true
				break
			}
		}
		active = append(active, activeCriterion{matches: matches, opAfter: op3})
	}

	// 4. SearchTerm
	if strings.TrimSpace(f.SearchTerm) != "" {
		q := strings.ToLower(strings.TrimSpace(f.SearchTerm))
		matches := strings.Contains(strings.ToLower(rec.Title), q) ||
			strings.Contains(fmt.Sprintf("%d", rec.ID), q) ||
			strings.Contains(strings.ToLower(rec.Tags), q) ||
			strings.Contains(strings.ToLower(rec.Description), q) ||
			strings.Contains(strings.ToLower(rec.WorkItemType), q) ||
			strings.Contains(strings.ToLower(rec.State), q)
		active = append(active, activeCriterion{matches: matches, opAfter: "AND"})
	}

	if len(active) == 0 {
		return true
	}

	// Evaluate left-to-right
	result := active[0].matches
	for i := 0; i < len(active)-1; i++ {
		op := strings.ToUpper(active[i].opAfter)
		nextMatch := active[i+1].matches
		if op == "OR" {
			result = result || nextMatch
		} else {
			result = result && nextMatch
		}
	}

	return result
}

func (f FilterOptions) MatchesNodeOrDescendant(node *TreeNode) bool {
	if node == nil || node.Record == nil {
		return false
	}
	if f.MatchesItem(node.Record) {
		return true
	}
	for _, child := range node.Children {
		if f.MatchesNodeOrDescendant(child) {
			return true
		}
	}
	return false
}

// SummaryString returns a clean string describing active filters, e.g. "Status: Active AND Types: [Bug, Task]"
func (f FilterOptions) SummaryString() string {
	if !f.IsActive() {
		return ""
	}
	var parts []string
	op1 := f.Op1
	if op1 == "" {
		op1 = "AND"
	}
	op2 := f.Op2
	if op2 == "" {
		op2 = "AND"
	}
	op3 := f.Op3
	if op3 == "" {
		op3 = "AND"
	}

	if statuses := f.GetEffectiveStatuses(); len(statuses) > 0 {
		if len(statuses) == 1 {
			parts = append(parts, fmt.Sprintf("Status: %s", statuses[0]))
		} else {
			parts = append(parts, fmt.Sprintf("Status: [%s]", strings.Join(statuses, ", ")))
		}
	}
	if assignees := f.GetEffectiveAssignees(); len(assignees) > 0 {
		if len(parts) > 0 {
			parts = append(parts, op1)
		}
		var cleanNames []string
		for _, a := range assignees {
			if strings.Contains(a, "<") {
				p := strings.Split(a, "<")
				cleanNames = append(cleanNames, strings.TrimSpace(p[0]))
			} else {
				cleanNames = append(cleanNames, a)
			}
		}
		if len(cleanNames) == 1 {
			parts = append(parts, fmt.Sprintf("Assignee: %s", cleanNames[0]))
		} else {
			parts = append(parts, fmt.Sprintf("Assignees: [%s]", strings.Join(cleanNames, ", ")))
		}
	}
	if types := f.GetEffectiveItemTypes(); len(types) > 0 {
		if len(parts) > 0 {
			parts = append(parts, op2)
		}
		if len(types) == 1 {
			parts = append(parts, fmt.Sprintf("Type: %s", types[0]))
		} else {
			parts = append(parts, fmt.Sprintf("Types: [%s]", strings.Join(types, ", ")))
		}
	}
	if strings.TrimSpace(f.SearchTerm) != "" {
		if len(parts) > 0 {
			parts = append(parts, op3)
		}
		parts = append(parts, fmt.Sprintf("Search: %q", f.SearchTerm))
	}
	return strings.Join(parts, " ")
}

// FlattenTree flattens the tree into visible rows based on expanded state and simple query
func FlattenTree(roots []*TreeNode, filterQuery string) []*TreeRow {
	var opts FilterOptions
	if filterQuery != "" {
		opts.SearchTerm = filterQuery
	}
	return FlattenTreeWithFilter(roots, opts)
}

// FlattenTreeWithFilter flattens the tree with full FilterOptions
func FlattenTreeWithFilter(roots []*TreeNode, filter FilterOptions) []*TreeRow {
	return FlattenTreeWithFilters(roots, filter)
}

// FlattenTreeWithFilters flattens the tree with multiple FilterOptions (e.g. global, tab, search).
// A node is included if it satisfies ALL active filters.
func FlattenTreeWithFilters(roots []*TreeNode, filters ...FilterOptions) []*TreeRow {
	var activeFilters []FilterOptions
	for _, f := range filters {
		if f.IsActive() {
			activeFilters = append(activeFilters, f)
		}
	}
	var rows []*TreeRow
	for _, root := range roots {
		flattenNodeWithFilters(root, "", true, &rows, activeFilters)
	}
	return rows
}

func matchesAllFiltersNodeOrDescendant(node *TreeNode, filters []FilterOptions) bool {
	if node == nil || node.Record == nil {
		return false
	}
	if len(filters) == 0 {
		return true
	}
	matchesSelf := true
	for _, f := range filters {
		if !f.MatchesItem(node.Record) {
			matchesSelf = false
			break
		}
	}
	if matchesSelf {
		return true
	}
	for _, child := range node.Children {
		if matchesAllFiltersNodeOrDescendant(child, filters) {
			return true
		}
	}
	return false
}

func flattenNodeWithFilters(node *TreeNode, indent string, isLast bool, rows *[]*TreeRow, filters []FilterOptions) {
	if len(filters) > 0 && !matchesAllFiltersNodeOrDescendant(node, filters) {
		return
	}

	var glyph string
	hasChildren := len(node.Children) > 0

	if hasChildren {
		if node.Expanded {
			glyph = "▼ "
		} else {
			glyph = "▶ "
		}
	} else {
		glyph = "• "
	}

	var branch string
	if node.Depth == 0 {
		branch = ""
	} else if isLast {
		branch = "└── "
	} else {
		branch = "├── "
	}

	prefix := indent + branch + glyph

	*rows = append(*rows, &TreeRow{
		Node:   node,
		Depth:  node.Depth,
		Indent: indent,
		Branch: branch,
		Glyph:  glyph,
		Prefix: prefix,
	})

	if hasChildren && node.Expanded {
		var nextIndent string
		if node.Depth == 0 {
			nextIndent = ""
		} else if isLast {
			nextIndent = indent + "    "
		} else {
			nextIndent = indent + "│   "
		}

		for i, child := range node.Children {
			childIsLast := (i == len(node.Children)-1)
			flattenNodeWithFilters(child, nextIndent, childIsLast, rows, filters)
		}
	}
}

// RollupProgressString returns a human-readable progress string e.g. "6/12 completed" or "6.0/12.0h (50%)"
func (n *TreeNode) RollupProgressString() string {
	if n.RollupOriginalEstimate > 0 || n.RollupCompletedWork > 0 {
		completed := n.RollupCompletedWork
		total := n.RollupOriginalEstimate
		if total <= 0 {
			total = completed + n.RollupRemainingWork
		}
		if total > 0 {
			pct := int((completed / total) * 100)
			if pct > 100 {
				pct = 100
			}
			return fmt.Sprintf("%.0f/%.0fh (%d%%)", completed, total, pct)
		}
	}

	if n.TotalItemsCount > 1 {
		return fmt.Sprintf("%d/%d done", n.DoneItemsCount, n.TotalItemsCount)
	}

	if isStateDone(n.Record.State) {
		return "Done"
	}
	return n.Record.State
}

// EstimateDisplayString returns estimate text e.g. "12h" or "24pts"
func (n *TreeNode) EstimateDisplayString() string {
	if n.RollupOriginalEstimate > 0 {
		return fmt.Sprintf("%.0fh", n.RollupOriginalEstimate)
	}
	if n.RollupEffort > 0 {
		if n.RollupEffort == float64(int(n.RollupEffort)) {
			return fmt.Sprintf("%.0fpts", n.RollupEffort)
		}
		return fmt.Sprintf("%.1fpts", n.RollupEffort)
	}
	if n.Record.OriginalEstimate > 0 {
		return fmt.Sprintf("%.0fh", n.Record.OriginalEstimate)
	}
	if n.Record.Effort > 0 {
		if n.Record.Effort == float64(int(n.Record.Effort)) {
			return fmt.Sprintf("%.0fpts", n.Record.Effort)
		}
		return fmt.Sprintf("%.1fpts", n.Record.Effort)
	}
	return "-"
}

// ApplyCollapsedNodes applies a set of collapsed work item IDs to a forest of tree nodes
func ApplyCollapsedNodes(roots []*TreeNode, collapsedIDs []int) {
	if len(roots) == 0 {
		return
	}
	collapsedMap := make(map[int]bool, len(collapsedIDs))
	for _, id := range collapsedIDs {
		collapsedMap[id] = true
	}
	for _, root := range roots {
		applyCollapsedRecursive(root, collapsedMap)
	}
}

func applyCollapsedRecursive(node *TreeNode, collapsedMap map[int]bool) {
	if node == nil || node.Record == nil {
		return
	}
	if collapsedMap[node.Record.ID] {
		node.Expanded = false
	} else {
		node.Expanded = true
	}
	for _, child := range node.Children {
		applyCollapsedRecursive(child, collapsedMap)
	}
}

// CollectCollapsedNodeIDs returns a slice of work item IDs for all currently collapsed nodes
func CollectCollapsedNodeIDs(roots []*TreeNode) []int {
	var ids []int
	for _, root := range roots {
		collectCollapsedRecursive(root, &ids)
	}
	return ids
}

func collectCollapsedRecursive(node *TreeNode, ids *[]int) {
	if node == nil || node.Record == nil {
		return
	}
	if len(node.Children) > 0 && !node.Expanded {
		*ids = append(*ids, node.Record.ID)
	}
	for _, child := range node.Children {
		collectCollapsedRecursive(child, ids)
	}
}

// HierarchyNode represents a clean JSON-serializable node in the hierarchical work item tree (without cyclic parent pointers)
type HierarchyNode struct {
	ID                     int              `json:"id"`
	Type                   string           `json:"type"`
	Title                  string           `json:"title"`
	State                  string           `json:"state"`
	AssignedToName         string           `json:"assigned_to_name,omitempty"`
	AssignedToEmail        string           `json:"assigned_to_email,omitempty"`
	AssignedToAvatar       string           `json:"assigned_to_avatar,omitempty"`
	OriginalEstimate       float64          `json:"original_estimate,omitempty"`
	RemainingWork          float64          `json:"remaining_work,omitempty"`
	CompletedWork          float64          `json:"completed_work,omitempty"`
	Effort                 float64          `json:"effort,omitempty"`
	StoryPoints            float64          `json:"story_points,omitempty"`
	StartDate              string           `json:"start_date,omitempty"`
	TargetDate             string           `json:"target_date,omitempty"`
	FinishDate             string           `json:"finish_date,omitempty"`
	Tags                   string           `json:"tags,omitempty"`
	Priority               int              `json:"priority,omitempty"`
	AreaPath               string           `json:"area_path,omitempty"`
	IterationPath          string           `json:"iteration_path,omitempty"`
	ParentID               int              `json:"parent_id,omitempty"`
	URL                    string           `json:"url,omitempty"`
	Depth                  int              `json:"depth"`
	RollupOriginalEstimate float64          `json:"rollup_original_estimate,omitempty"`
	RollupCompletedWork    float64          `json:"rollup_completed_work,omitempty"`
	RollupRemainingWork    float64          `json:"rollup_remaining_work,omitempty"`
	RollupEffort           float64          `json:"rollup_effort,omitempty"`
	TotalItemsCount        int              `json:"total_items_count"`
	DoneItemsCount         int              `json:"done_items_count"`
	Children               []*HierarchyNode `json:"children,omitempty"`
}

// ToHierarchyNode converts a TreeNode to a clean JSON-serializable HierarchyNode
func (n *TreeNode) ToHierarchyNode() *HierarchyNode {
	if n == nil {
		return nil
	}
	h := &HierarchyNode{
		Depth:                  n.Depth,
		RollupOriginalEstimate: n.RollupOriginalEstimate,
		RollupCompletedWork:    n.RollupCompletedWork,
		RollupRemainingWork:    n.RollupRemainingWork,
		RollupEffort:           n.RollupEffort,
		TotalItemsCount:        n.TotalItemsCount,
		DoneItemsCount:         n.DoneItemsCount,
	}
	if n.Record != nil {
		h.ID = n.Record.ID
		h.Type = n.Record.WorkItemType
		h.Title = n.Record.Title
		h.State = n.Record.State
		h.AssignedToName = n.Record.AssignedToName
		h.AssignedToEmail = n.Record.AssignedToEmail
		h.AssignedToAvatar = n.Record.AssignedToAvatar
		h.OriginalEstimate = n.Record.OriginalEstimate
		h.RemainingWork = n.Record.RemainingWork
		h.CompletedWork = n.Record.CompletedWork
		h.Effort = n.Record.Effort
		h.StoryPoints = n.Record.StoryPoints
		h.StartDate = n.Record.StartDate
		h.TargetDate = n.Record.TargetDate
		h.FinishDate = n.Record.FinishDate
		h.Tags = n.Record.Tags
		h.Priority = n.Record.Priority
		h.AreaPath = n.Record.AreaPath
		h.IterationPath = n.Record.IterationPath
		h.ParentID = n.Record.ParentID
		h.URL = n.Record.URL
	}
	if len(n.Children) > 0 {
		h.Children = make([]*HierarchyNode, 0, len(n.Children))
		for _, child := range n.Children {
			if ch := child.ToHierarchyNode(); ch != nil {
				h.Children = append(h.Children, ch)
			}
		}
	}
	return h
}

// BuildSingleTree constructs a standalone TreeNode hierarchy rooted at rootRec from a slice of descendant items
func BuildSingleTree(rootRec *WorkItemRecord, items []*WorkItemRecord) *TreeNode {
	if rootRec == nil {
		return nil
	}
	itemMap := make(map[int]*WorkItemRecord, len(items)+1)
	itemMap[rootRec.ID] = rootRec
	childrenMap := make(map[int][]*WorkItemRecord)

	for _, item := range items {
		if item == nil {
			continue
		}
		itemMap[item.ID] = item
		if item.ParentID > 0 {
			childrenMap[item.ParentID] = append(childrenMap[item.ParentID], item)
		}
	}

	var buildSubtree func(rec *WorkItemRecord, parent *TreeNode, depth int, visited map[int]bool) *TreeNode
	buildSubtree = func(rec *WorkItemRecord, parent *TreeNode, depth int, visited map[int]bool) *TreeNode {
		if visited[rec.ID] || depth > 50 {
			return nil
		}
		visited[rec.ID] = true

		node := &TreeNode{
			Record:   rec,
			Parent:   parent,
			Depth:    depth,
			Expanded: true,
			Children: make([]*TreeNode, 0),
		}

		childRecs := childrenMap[rec.ID]
		for _, childRec := range childRecs {
			childVisited := make(map[int]bool, len(visited)+1)
			for k, v := range visited {
				childVisited[k] = v
			}
			childNode := buildSubtree(childRec, node, depth+1, childVisited)
			if childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}

		return node
	}

	rootNode := buildSubtree(rootRec, nil, 0, make(map[int]bool))
	if rootNode != nil {
		computeRollupsAndDepths(rootNode, 0)
	}
	return rootNode
}



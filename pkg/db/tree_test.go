package db

import (
	"testing"
)

func TestBuildTreeAndRollups(t *testing.T) {
	// Create mock hierarchy:
	// Root Feature (ID 100)
	//   ├── Child Story 1 (ID 101, Estimate 20, Completed 10, Remaining 10)
	//   │     ├── Task 1 (ID 102, Estimate 10, Completed 10, Remaining 0, State "Done")
	//   │     └── Task 2 (ID 103, Estimate 10, Completed 0, Remaining 10, State "To Do")
	//   └── Child Story 2 (ID 104, Estimate 30, Completed 15, Remaining 15, State "In Progress")

	items := []*WorkItemRecord{
		{
			ID:               100,
			Title:            "Feature 100",
			WorkItemType:     "Feature",
			State:            "In Progress",
			ParentID:         0,
			OriginalEstimate: 0,
			CompletedWork:    0,
			RemainingWork:    0,
		},
		{
			ID:               101,
			Title:            "Story 101",
			WorkItemType:     "User Story",
			State:            "In Progress",
			ParentID:         100,
			OriginalEstimate: 0,
			CompletedWork:    0,
			RemainingWork:    0,
		},
		{
			ID:               102,
			Title:            "Task 102",
			WorkItemType:     "Task",
			State:            "Done",
			ParentID:         101,
			OriginalEstimate: 10,
			CompletedWork:    10,
			RemainingWork:    0,
		},
		{
			ID:               103,
			Title:            "Task 103",
			WorkItemType:     "Task",
			State:            "To Do",
			ParentID:         101,
			OriginalEstimate: 10,
			CompletedWork:    0,
			RemainingWork:    10,
		},
		{
			ID:               104,
			Title:            "Story 104",
			WorkItemType:     "User Story",
			State:            "In Progress",
			ParentID:         100,
			OriginalEstimate: 30,
			CompletedWork:    15,
			RemainingWork:    15,
		},
	}

	roots := []TrackedRoot{
		{ID: 100, Org: "testorg", Project: "testproj"},
	}

	tree := BuildTree(items, roots)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}

	root := tree[0]
	if root.Record.ID != 100 {
		t.Errorf("expected root ID 100, got %d", root.Record.ID)
	}

	// Verify rollups on Root Feature 100:
	// Total Original Estimate should be 10 + 10 + 30 = 50
	if root.RollupOriginalEstimate != 50 {
		t.Errorf("expected root RollupOriginalEstimate 50, got %f", root.RollupOriginalEstimate)
	}
	// Total Completed Work should be 10 + 0 + 15 = 25
	if root.RollupCompletedWork != 25 {
		t.Errorf("expected root RollupCompletedWork 25, got %f", root.RollupCompletedWork)
	}
	// Total Remaining Work should be 0 + 10 + 15 = 25
	if root.RollupRemainingWork != 25 {
		t.Errorf("expected root RollupRemainingWork 25, got %f", root.RollupRemainingWork)
	}
	// Total items count = 5
	if root.TotalItemsCount != 5 {
		t.Errorf("expected root TotalItemsCount 5, got %d", root.TotalItemsCount)
	}
	// Done count = 1 (Task 102)
	if root.DoneItemsCount != 1 {
		t.Errorf("expected root DoneItemsCount 1, got %d", root.DoneItemsCount)
	}

	// Verify progress string
	progStr := root.RollupProgressString()
	expectedProg := "25/50h (50%)"
	if progStr != expectedProg {
		t.Errorf("expected progress '%s', got '%s'", expectedProg, progStr)
	}

	// Test flattening
	flattened := FlattenTree(tree, "")
	if len(flattened) != 5 {
		t.Errorf("expected 5 flattened rows, got %d", len(flattened))
	}
}

func TestBuildTreeEmptyRootsAndMultipleRoots(t *testing.T) {
	items := []*WorkItemRecord{
		{ID: 100, Title: "Feature 100", WorkItemType: "Feature", ParentID: 0},
		{ID: 101, Title: "Task 101", WorkItemType: "Task", ParentID: 100},
		{ID: 200, Title: "Feature 200", WorkItemType: "Feature", ParentID: 0},
		{ID: 201, Title: "Task 201", WorkItemType: "Task", ParentID: 200},
		{ID: 300, Title: "Untracked Epic 300", WorkItemType: "Epic", ParentID: 0},
	}

	// 1. Empty roots should return nil
	emptyTree := BuildTree(items, []TrackedRoot{})
	if emptyTree != nil {
		t.Fatalf("expected nil tree when tracked roots is empty, got %d roots", len(emptyTree))
	}

	// 2. Multiple tracked roots
	roots := []TrackedRoot{
		{ID: 100, Org: "org", Project: "proj"},
		{ID: 200, Org: "org", Project: "proj"},
	}
	tree := BuildTree(items, roots)
	if len(tree) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree))
	}
	if tree[0].Record.ID != 100 || tree[1].Record.ID != 200 {
		t.Fatalf("expected roots 100 and 200, got %d and %d", tree[0].Record.ID, tree[1].Record.ID)
	}

	// Verify untracked #300 is excluded from flattened tree
	flattened := FlattenTree(tree, "")
	if len(flattened) != 4 {
		t.Fatalf("expected 4 flattened rows (excluding untracked 300), got %d", len(flattened))
	}
	for _, row := range flattened {
		if row.Node.Record.ID == 300 {
			t.Fatalf("untracked item 300 should not be in flattened tree")
		}
	}

	// 3. Track both parent (#100) and child (#101) alongside another root (#200) -> all 3 should be root tabs
	roots3 := []TrackedRoot{
		{ID: 100, Org: "org", Project: "proj"},
		{ID: 200, Org: "org", Project: "proj"},
		{ID: 101, Org: "org", Project: "proj"},
	}
	tree3 := BuildTree(items, roots3)
	if len(tree3) != 3 {
		t.Fatalf("expected 3 roots when 3 items are tracked, got %d", len(tree3))
	}
	if tree3[0].Record.ID != 100 || tree3[1].Record.ID != 200 || tree3[2].Record.ID != 101 {
		t.Fatalf("expected roots 100, 200, 101, got %d, %d, %d", tree3[0].Record.ID, tree3[1].Record.ID, tree3[2].Record.ID)
	}
	// Root #101 should be depth 0 in its own tree
	if tree3[2].Depth != 0 {
		t.Fatalf("expected root 101 to have depth 0 in its own tree, got %d", tree3[2].Depth)
	}
	// Child #101 should still be depth 1 inside root #100's tree
	if len(tree3[0].Children) != 1 || tree3[0].Children[0].Record.ID != 101 || tree3[0].Children[0].Depth != 1 {
		t.Fatalf("expected child 101 inside root 100 to have depth 1")
	}
}

func TestApplyAndCollectCollapsedNodes(t *testing.T) {
	items := []*WorkItemRecord{
		{ID: 100, Title: "Feature 100", WorkItemType: "Feature", ParentID: 0},
		{ID: 101, Title: "Task 101", WorkItemType: "Task", ParentID: 100},
		{ID: 200, Title: "Feature 200", WorkItemType: "Feature", ParentID: 0},
		{ID: 201, Title: "Task 201", WorkItemType: "Task", ParentID: 200},
	}
	roots := []TrackedRoot{
		{ID: 100, Org: "org", Project: "proj"},
		{ID: 200, Org: "org", Project: "proj"},
	}
	tree := BuildTree(items, roots)
	if len(tree) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree))
	}

	// Initially all nodes are expanded
	collapsed := CollectCollapsedNodeIDs(tree)
	if len(collapsed) != 0 {
		t.Fatalf("expected 0 collapsed nodes initially, got %v", collapsed)
	}

	// Apply collapsing to node 100
	ApplyCollapsedNodes(tree, []int{100})
	if tree[0].Expanded {
		t.Fatalf("expected node 100 to be collapsed")
	}
	if !tree[1].Expanded {
		t.Fatalf("expected node 200 to remain expanded")
	}

	// Collect collapsed nodes
	collapsed = CollectCollapsedNodeIDs(tree)
	if len(collapsed) != 1 || collapsed[0] != 100 {
		t.Fatalf("expected [100] collapsed, got %v", collapsed)
	}
}

func TestBuildTreeWithTrackedQueries(t *testing.T) {
	// Setup items:
	// Query "Active Sprint PBIs & Bugs" contains:
	// - PBI 501 (Est: 10, Comp: 5, Rem: 5)
	//   - Task 502 (Est: 5, Comp: 5, Rem: 0, Done)
	//   - Task 503 (Est: 5, Comp: 0, Rem: 5, To Do)
	// - Bug 504 (Est: 8, Comp: 8, Rem: 0, Done)
	// And standalone root Feature 600 (ParentID: 0)

	items := []*WorkItemRecord{
		{ID: 501, Title: "PBI 501", WorkItemType: "Product Backlog Item", State: "In Progress", ParentID: 0, OriginalEstimate: 10, CompletedWork: 5, RemainingWork: 5},
		{ID: 502, Title: "Task 502", WorkItemType: "Task", State: "Done", ParentID: 501, OriginalEstimate: 5, CompletedWork: 5, RemainingWork: 0},
		{ID: 503, Title: "Task 503", WorkItemType: "Task", State: "To Do", ParentID: 501, OriginalEstimate: 5, CompletedWork: 0, RemainingWork: 5},
		{ID: 504, Title: "Bug 504", WorkItemType: "Bug", State: "Closed", ParentID: 0, OriginalEstimate: 8, CompletedWork: 8, RemainingWork: 0},
		{ID: 600, Title: "Standalone Feature 600", WorkItemType: "Feature", State: "Active", ParentID: 0, OriginalEstimate: 20, CompletedWork: 10, RemainingWork: 10},
	}

	trackedRoots := []TrackedRoot{
		{ID: 501, Org: "org", Project: "proj", QueryID: "query-uuid-123"},
		{ID: 504, Org: "org", Project: "proj", QueryID: "query-uuid-123"},
		{ID: 600, Org: "org", Project: "proj", QueryID: ""}, // Standalone
	}

	trackedQueries := []TrackedQuery{
		{
			ID:        "query-uuid-123",
			Name:      "Active Sprint PBIs & Bugs",
			Path:      "Shared Queries/Active Sprint PBIs & Bugs",
			Org:       "org",
			Project:   "proj",
			QueryType: "flat",
			URL:       "https://dev.azure.com/org/proj/_queries/query/query-uuid-123",
		},
	}

	tree := BuildTree(items, trackedRoots, trackedQueries)

	// We expect 2 top-level tabs/roots:
	// Tab 1: Query "Active Sprint PBIs & Bugs"
	// Tab 2: Standalone Feature 600
	if len(tree) != 2 {
		t.Fatalf("expected 2 root tabs (1 query + 1 standalone), got %d", len(tree))
	}

	// Tab 1: Query
	queryNode := tree[0]
	if !queryNode.IsQuery {
		t.Errorf("expected tree[0] to be Query, got IsQuery=false")
	}
	if queryNode.Record.Title != "Active Sprint PBIs & Bugs" {
		t.Errorf("expected query title 'Active Sprint PBIs & Bugs', got '%s'", queryNode.Record.Title)
	}
	if queryNode.Record.WorkItemType != "Query" {
		t.Errorf("expected query item type 'Query', got '%s'", queryNode.Record.WorkItemType)
	}
	if len(queryNode.Children) != 2 {
		t.Fatalf("expected query to have 2 top-level children (501 and 504), got %d", len(queryNode.Children))
	}

	// Verify child 501 has children 502 and 503
	pbiNode := queryNode.Children[0]
	if pbiNode.Record.ID != 501 {
		t.Errorf("expected first query child to be 501, got %d", pbiNode.Record.ID)
	}
	if len(pbiNode.Children) != 2 {
		t.Errorf("expected PBI 501 to have 2 task children, got %d", len(pbiNode.Children))
	}

	// Verify rollups on the Query root:
	// Total items in query = 501, 502, 503, 504 = 4 items
	if queryNode.TotalItemsCount != 4 {
		t.Errorf("expected query TotalItemsCount 4, got %d", queryNode.TotalItemsCount)
	}
	// Done items in query = 502 (Done) + 504 (Closed) = 2
	if queryNode.DoneItemsCount != 2 {
		t.Errorf("expected query DoneItemsCount 2, got %d", queryNode.DoneItemsCount)
	}
	// Total Original Estimate: 5 + 5 (from 501 tasks) + 8 (from 504) = 18
	if queryNode.RollupOriginalEstimate != 18 {
		t.Errorf("expected query RollupOriginalEstimate 18, got %f", queryNode.RollupOriginalEstimate)
	}
	// Total Completed Work: 5 + 8 = 13
	if queryNode.RollupCompletedWork != 13 {
		t.Errorf("expected query RollupCompletedWork 13, got %f", queryNode.RollupCompletedWork)
	}
	// Total Remaining Work: 5 + 0 = 5
	if queryNode.RollupRemainingWork != 5 {
		t.Errorf("expected query RollupRemainingWork 5, got %f", queryNode.RollupRemainingWork)
	}

	// Tab 2: Standalone Feature 600
	standaloneNode := tree[1]
	if standaloneNode.IsQuery {
		t.Errorf("expected tree[1] to NOT be Query")
	}
	if standaloneNode.Record.ID != 600 {
		t.Errorf("expected tree[1] ID 600, got %d", standaloneNode.Record.ID)
	}
}

func TestBuildTreeQueryDeduplication(t *testing.T) {
	// Hierarchy:
	// Query "UE Phase 1.2"
	//   ├── #648227 (ParentID = 0)
	//   └── #656841 (Epic, ParentID = 0)
	//         └── #656842 (Feature, ParentID = 656841)
	//               └── #659219 (Tech Enabler, ParentID = 656842)
	//                     └── #662396 (Task, ParentID = 659219)

	items := []*WorkItemRecord{
		{ID: 648227, Title: "deploy uptime monitor", WorkItemType: "Tech Enabler", ParentID: 0, OriginalEstimate: 0},
		{ID: 656841, Title: "GCP Cost Optimization", WorkItemType: "Epic", ParentID: 0, OriginalEstimate: 0},
		{ID: 656842, Title: "GKE & Spot Instances", WorkItemType: "Feature", ParentID: 656841, OriginalEstimate: 0},
		{ID: 659219, Title: "GKE Architecture Analysis", WorkItemType: "Tech Enabler", ParentID: 656842, OriginalEstimate: 114, CompletedWork: 38, RemainingWork: 76},
		{ID: 662396, Title: "Analysis", WorkItemType: "Task", ParentID: 659219, OriginalEstimate: 24, CompletedWork: 24, RemainingWork: 0, State: "Done"},
	}

	// Suppose ADO query returned 648227, 656841, AND 659219 as candidate roots for query "q1"
	trackedRoots := []TrackedRoot{
		{ID: 648227, Org: "org", Project: "proj", QueryID: "q1"},
		{ID: 659219, Org: "org", Project: "proj", QueryID: "q1"}, // Should NOT be a direct child of query because parent 656842 exists
		{ID: 656841, Org: "org", Project: "proj", QueryID: "q1"},
	}

	trackedQueries := []TrackedQuery{
		{ID: "q1", Name: "UE Phase 1.2", Org: "org", Project: "proj"},
	}

	tree := BuildTree(items, trackedRoots, trackedQueries)
	if len(tree) != 1 {
		t.Fatalf("expected 1 query tree root, got %d", len(tree))
	}

	queryNode := tree[0]
	// Top level children of query must only be 648227 and 656841 (2 items, NOT 3)
	if len(queryNode.Children) != 2 {
		t.Fatalf("expected query to have 2 top-level children, got %d", len(queryNode.Children))
	}

	if queryNode.Children[0].Record.ID != 648227 {
		t.Errorf("expected first child to be 648227, got %d", queryNode.Children[0].Record.ID)
	}
	if queryNode.Children[1].Record.ID != 656841 {
		t.Errorf("expected second child to be 656841, got %d", queryNode.Children[1].Record.ID)
	}

	// Verify 659219 is correctly nested inside 656841 -> 656842 -> 659219
	epicNode := queryNode.Children[1]
	if len(epicNode.Children) != 1 || epicNode.Children[0].Record.ID != 656842 {
		t.Fatalf("expected Epic 656841 to have child Feature 656842")
	}
	featureNode := epicNode.Children[0]
	if len(featureNode.Children) != 1 || featureNode.Children[0].Record.ID != 659219 {
		t.Fatalf("expected Feature 656842 to have child Tech Enabler 659219")
	}
	techEnablerNode := featureNode.Children[0]
	if len(techEnablerNode.Children) != 1 || techEnablerNode.Children[0].Record.ID != 662396 {
		t.Fatalf("expected Tech Enabler 659219 to have child Task 662396")
	}

	// Check that total items in query is exactly 5 without duplicates
	if queryNode.TotalItemsCount != 5 {
		t.Errorf("expected total items in query to be 5, got %d", queryNode.TotalItemsCount)
	}

	// Check that flattened tree has exactly 6 rows (1 query root + 5 items)
	flattened := FlattenTree(tree, "")
	if len(flattened) != 6 {
		t.Fatalf("expected 6 rows in flattened tree, got %d", len(flattened))
	}
}

func TestFlattenTreeWithFilters(t *testing.T) {
	items := []*WorkItemRecord{
		{ID: 100, Title: "Feature Alpha", WorkItemType: "Feature", State: "Active", AssignedToName: "Alice", ParentID: 0},
		{ID: 101, Title: "Task Backend", WorkItemType: "Task", State: "Active", AssignedToName: "Bob", ParentID: 100},
		{ID: 102, Title: "Task Frontend", WorkItemType: "Task", State: "Done", AssignedToName: "Alice", ParentID: 100},
		{ID: 200, Title: "Feature Beta", WorkItemType: "Feature", State: "New", AssignedToName: "Charlie", ParentID: 0},
	}
	roots := []TrackedRoot{
		{ID: 100, Org: "org", Project: "proj"},
		{ID: 200, Org: "org", Project: "proj"},
	}

	tree := BuildTree(items, roots)

	// Filter 1: Global filter for Active state
	globalFilter := FilterOptions{Statuses: []string{"Active"}}
	// Filter 2: Tab filter for Assignee Alice
	tabFilter := FilterOptions{Assignees: []string{"Alice"}}

	// When both are applied:
	// Root #100 is Active and Assigned to Alice -> matches both filters
	// Child #101 is Active and Assigned to Bob -> matches global (Active) but fails tab (Alice)
	// Child #102 is Done and Assigned to Alice -> matches tab (Alice) but fails global (Active)
	// Root #200 is New and Charlie -> fails both
	res := FlattenTreeWithFilters(tree, globalFilter, tabFilter)
	if len(res) != 1 {
		t.Fatalf("expected 1 matching row (#100), got %d", len(res))
	}
	if res[0].Node.Record.ID != 100 {
		t.Errorf("expected row to be #100, got #%d", res[0].Node.Record.ID)
	}

	// Now filter where child matches and parent is shown as ancestor:
	// Global: Task, Tab: Alice
	// Only #102 matches both (Task + Alice). Since #100 is parent, #100 is included as ancestor
	res2 := FlattenTreeWithFilters(tree, FilterOptions{ItemTypes: []string{"Task"}}, FilterOptions{Assignees: []string{"Alice"}})
	if len(res2) != 2 {
		t.Fatalf("expected 2 rows (#100 ancestor + #102 match), got %d", len(res2))
	}
	if res2[0].Node.Record.ID != 100 || res2[1].Node.Record.ID != 102 {
		t.Errorf("expected #100 and #102, got #%d and #%d", res2[0].Node.Record.ID, res2[1].Node.Record.ID)
	}
}

func TestBuildTreeCustomTitle(t *testing.T) {
	items := []*WorkItemRecord{
		{ID: 100, Title: "Original Server Name", WorkItemType: "Feature", ParentID: 0},
	}
	roots := []TrackedRoot{
		{ID: 100, Org: "org", Project: "proj", CustomTitle: "Custom Tab Title"},
	}

	tree := BuildTree(items, roots)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if tree[0].CustomTitle != "Custom Tab Title" {
		t.Errorf("expected tree[0].CustomTitle == 'Custom Tab Title', got %q", tree[0].CustomTitle)
	}
	if tree[0].TabTitle() != "Custom Tab Title" {
		t.Errorf("expected tree[0].TabTitle() == 'Custom Tab Title', got %q", tree[0].TabTitle())
	}
	if tree[0].Record.Title != "Original Server Name" {
		t.Errorf("expected original work item title 'Original Server Name' to be preserved, got %q", tree[0].Record.Title)
	}
}

func TestEstimateDisplayString(t *testing.T) {
	nodePointsInt := &TreeNode{
		Record: &WorkItemRecord{WorkItemType: "User Story", Effort: 5.0},
	}
	if str := nodePointsInt.EstimateDisplayString(); str != "5pts" {
		t.Errorf("expected '5pts', got %q", str)
	}

	nodePointsDecimal := &TreeNode{
		Record: &WorkItemRecord{WorkItemType: "User Story", Effort: 5.5},
	}
	if str := nodePointsDecimal.EstimateDisplayString(); str != "5.5pts" {
		t.Errorf("expected '5.5pts', got %q", str)
	}

	nodeHours := &TreeNode{
		Record: &WorkItemRecord{WorkItemType: "Task", OriginalEstimate: 8.0},
	}
	if str := nodeHours.EstimateDisplayString(); str != "8h" {
		t.Errorf("expected '8h', got %q", str)
	}
}




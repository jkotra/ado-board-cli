package tui

import (
	"fmt"
	"strings"
	"time"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ScreenTree is the Home / Landing Tree View (Screen 1)
type ScreenTree struct {
	TreeRoots        []*db.TreeNode
	Flattened        []*db.TreeRow
	Cursor           int
	ActiveTab        int
	FilterInput      textinput.Model
	Filtering        bool
	GlobalFilterOpts db.FilterOptions
	TabFilterOpts    map[string]db.FilterOptions
	FilterOpts       db.FilterOptions // legacy alias for GlobalFilterOpts
	IncompleteView   bool
	incompleteCache  map[int]bool
	Config           *config.Config
	DB               *db.DB
	Width            int
	Height           int
	ScrollOffset     int
	MarqueeTick      int
}

func NewScreenTree() *ScreenTree {
	ti := textinput.New()
	ti.Placeholder = "Type to filter items..."
	ti.CharLimit = 50
	ti.Width = 30
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &ScreenTree{
		FilterInput:     ti,
		TabFilterOpts:   make(map[string]db.FilterOptions),
		incompleteCache: make(map[int]bool),
	}
}

// ClearIncompleteCache invalidates the cached incomplete evaluations
func (s *ScreenTree) ClearIncompleteCache() {
	if s.incompleteCache != nil {
		s.incompleteCache = make(map[int]bool)
	}
}

func (s *ScreenTree) SetData(roots []*db.TreeNode) {
	s.TreeRoots = roots
	if s.ActiveTab >= len(s.TreeRoots) {
		s.ActiveTab = len(s.TreeRoots) - 1
	}
	if s.ActiveTab < 0 {
		s.ActiveTab = 0
	}
	s.rebuildFlattened()
}

func (s *ScreenTree) GetTabKey(tabIdx int) string {
	if tabIdx < 0 || tabIdx >= len(s.TreeRoots) {
		return fmt.Sprintf("idx:%d", tabIdx)
	}
	root := s.TreeRoots[tabIdx]
	if root == nil || root.Record == nil {
		return fmt.Sprintf("idx:%d", tabIdx)
	}
	if root.IsQuery || root.Record.WorkItemType == "Query" {
		return fmt.Sprintf("query:%s", root.Record.Title)
	}
	return fmt.Sprintf("root:%d", root.Record.ID)
}

func (s *ScreenTree) GetCurrentTabKey() string {
	return s.GetTabKey(s.ActiveTab)
}

func (s *ScreenTree) GetTabFilter(tabIdx int) db.FilterOptions {
	if s.TabFilterOpts == nil {
		return db.FilterOptions{}
	}
	key := s.GetTabKey(tabIdx)
	return s.TabFilterOpts[key]
}

func (s *ScreenTree) SetFilter(opts db.FilterOptions) {
	s.SetGlobalFilter(opts)
}

func (s *ScreenTree) SetGlobalFilter(opts db.FilterOptions) {
	s.GlobalFilterOpts = opts
	s.FilterOpts = opts
	s.rebuildFlattened()
}

func (s *ScreenTree) SetTabFilter(tabIdx int, opts db.FilterOptions) {
	if s.TabFilterOpts == nil {
		s.TabFilterOpts = make(map[string]db.FilterOptions)
	}
	key := s.GetTabKey(tabIdx)
	if opts.IsActive() {
		s.TabFilterOpts[key] = opts
	} else {
		delete(s.TabFilterOpts, key)
	}
	s.rebuildFlattened()
}

func (s *ScreenTree) ClearFilter() {
	s.ClearAllFilters()
}

func (s *ScreenTree) ClearAllFilters() {
	s.GlobalFilterOpts = db.FilterOptions{}
	s.FilterOpts = db.FilterOptions{}
	s.TabFilterOpts = make(map[string]db.FilterOptions)
	s.FilterInput.Reset()
	s.Filtering = false
	s.rebuildFlattened()
}

func (s *ScreenTree) ClearGlobalFilter() {
	s.GlobalFilterOpts = db.FilterOptions{}
	s.FilterOpts = db.FilterOptions{}
	s.FilterInput.Reset()
	s.Filtering = false
	s.rebuildFlattened()
}

func (s *ScreenTree) ClearTabFilter(tabIdx int) {
	if s.TabFilterOpts != nil {
		delete(s.TabFilterOpts, s.GetTabKey(tabIdx))
	}
	s.rebuildFlattened()
}

// NextTab switches to the next tracked root tab
func (s *ScreenTree) NextTab() {
	if len(s.TreeRoots) <= 1 {
		return
	}
	s.ActiveTab = (s.ActiveTab + 1) % len(s.TreeRoots)
	s.Cursor = 0
	s.ScrollOffset = 0
	s.MarqueeTick = 0
	s.rebuildFlattened()
}

// PrevTab switches to the previous tracked root tab
func (s *ScreenTree) PrevTab() {
	if len(s.TreeRoots) <= 1 {
		return
	}
	s.ActiveTab--
	if s.ActiveTab < 0 {
		s.ActiveTab = len(s.TreeRoots) - 1
	}
	s.Cursor = 0
	s.ScrollOffset = 0
	s.MarqueeTick = 0
	s.rebuildFlattened()
}

// SetActiveTab switches directly to the specified tab index
func (s *ScreenTree) SetActiveTab(idx int) {
	if idx < 0 || idx >= len(s.TreeRoots) {
		return
	}
	s.ActiveTab = idx
	s.Cursor = 0
	s.ScrollOffset = 0
	s.MarqueeTick = 0
	s.rebuildFlattened()
}

// SelectTabByNameOrRootID searches and switches to a tab matching title or root ID
func (s *ScreenTree) SelectTabByNameOrRootID(name string, rootID int) bool {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	for i, root := range s.TreeRoots {
		if root == nil || root.Record == nil {
			continue
		}
		if rootID > 0 && root.Record.ID == rootID {
			s.SetActiveTab(i)
			return true
		}
		if nameLower != "" && strings.Contains(strings.ToLower(root.Record.Title), nameLower) {
			s.SetActiveTab(i)
			return true
		}
	}
	return false
}

func expandPathToItem(node *db.TreeNode, targetID int) bool {
	if node == nil || node.Record == nil {
		return false
	}
	if node.Record.ID == targetID {
		return true
	}
	for _, child := range node.Children {
		if expandPathToItem(child, targetID) {
			node.Expanded = true
			return true
		}
	}
	return false
}

// NavigateToItem searches all tabs for the target work item ID, expands its ancestor path,
// switches to the containing tab, and moves the cursor to that row.
func (s *ScreenTree) NavigateToItem(itemID int) bool {
	for i, root := range s.TreeRoots {
		if expandPathToItem(root, itemID) {
			s.ActiveTab = i
			s.rebuildFlattened()
			for rowIdx, row := range s.Flattened {
				if row.Node != nil && row.Node.Record != nil && row.Node.Record.ID == itemID {
					s.Cursor = rowIdx
					s.adjustScroll()
					return true
				}
			}
			return true
		}
	}
	return false
}

func (s *ScreenTree) rebuildFlattened() {
	s.ClearIncompleteCache()

	var searchOpt db.FilterOptions
	if s.Filtering && s.FilterInput.Value() != "" {
		searchOpt.SearchTerm = s.FilterInput.Value()
	}

	tabFilter := s.GetTabFilter(s.ActiveTab)

	if len(s.TreeRoots) == 0 {
		s.Flattened = nil
	} else {
		if s.ActiveTab >= len(s.TreeRoots) {
			s.ActiveTab = len(s.TreeRoots) - 1
		}
		if s.ActiveTab < 0 {
			s.ActiveTab = 0
		}
		activeRoots := []*db.TreeNode{s.TreeRoots[s.ActiveTab]}
		s.Flattened = db.FlattenTreeWithFilters(activeRoots, s.GlobalFilterOpts, tabFilter, searchOpt)
	}

	if s.Cursor >= len(s.Flattened) {
		s.Cursor = len(s.Flattened) - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	s.adjustScroll()
}

func (s *ScreenTree) adjustScroll() {
	tabBarHeight := 0
	if len(s.TreeRoots) > 0 {
		tabBarHeight = 1
	}
	headerHeight := 4
	footerHeight := 1
	visibleHeight := s.Height - headerHeight - footerHeight - tabBarHeight
	if visibleHeight <= 0 {
		visibleHeight = 5
	}

	if s.Cursor < s.ScrollOffset {
		s.ScrollOffset = s.Cursor
	} else if s.Cursor >= s.ScrollOffset+visibleHeight {
		s.ScrollOffset = s.Cursor - visibleHeight + 1
	}

	maxScroll := len(s.Flattened) - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if s.ScrollOffset > maxScroll {
		s.ScrollOffset = maxScroll
	}
	if s.ScrollOffset < 0 {
		s.ScrollOffset = 0
	}
}

func (s *ScreenTree) SelectedNode() *db.TreeNode {
	if s.Cursor >= 0 && s.Cursor < len(s.Flattened) {
		return s.Flattened[s.Cursor].Node
	}
	return nil
}

func (s *ScreenTree) SelectedItem() *db.WorkItemRecord {
	node := s.SelectedNode()
	if node != nil {
		return node.Record
	}
	return nil
}

// ActiveRootID returns the work item ID of the root node of the active tab
func (s *ScreenTree) ActiveRootID() int {
	if s.ActiveTab >= 0 && s.ActiveTab < len(s.TreeRoots) && s.TreeRoots[s.ActiveTab] != nil && s.TreeRoots[s.ActiveTab].Record != nil {
		return s.TreeRoots[s.ActiveTab].Record.ID
	}
	return 0
}

// SelectedItemID returns the currently selected work item ID, or 0 if none
func (s *ScreenTree) SelectedItemID() int {
	item := s.SelectedItem()
	if item != nil {
		return item.ID
	}
	return 0
}

// CollapsedNodeIDs returns the IDs of all currently collapsed nodes in the tree
func (s *ScreenTree) CollapsedNodeIDs() []int {
	return db.CollectCollapsedNodeIDs(s.TreeRoots)
}

// RestoreState restores the active tab, collapsed nodes, filters, and selected work item
func (s *ScreenTree) RestoreState(activeRootID int, activeTabIdx int, selectedItemID int, collapsedIDs []int, filter db.FilterOptions) {
	if len(s.TreeRoots) == 0 {
		return
	}
	// 1. Apply collapsed node states
	if len(collapsedIDs) > 0 {
		db.ApplyCollapsedNodes(s.TreeRoots, collapsedIDs)
	}

	// 2. Select tab
	tabMatched := false
	if activeRootID > 0 {
		for i, r := range s.TreeRoots {
			if r.Record != nil && r.Record.ID == activeRootID {
				s.ActiveTab = i
				tabMatched = true
				break
			}
		}
	}
	if !tabMatched {
		if activeTabIdx >= 0 && activeTabIdx < len(s.TreeRoots) {
			s.ActiveTab = activeTabIdx
		} else {
			s.ActiveTab = 0
		}
	}

	// 3. Apply filter options
	s.GlobalFilterOpts = filter
	s.FilterOpts = filter
	if filter.SearchTerm != "" {
		s.FilterInput.SetValue(filter.SearchTerm)
	}

	// 4. Rebuild flattened tree for the active tab
	s.rebuildFlattened()

	// 5. Select item if specified
	if selectedItemID > 0 {
		s.SelectItemID(selectedItemID)
	}
}

// SelectItemID moves the cursor to the row with the given work item ID, switching tabs and expanding parent nodes if needed
func (s *ScreenTree) SelectItemID(id int) {
	for i, row := range s.Flattened {
		if row.Node.Record.ID == id {
			s.Cursor = i
			s.MarqueeTick = 0
			s.adjustScroll()
			return
		}
	}

	for tabIdx, root := range s.TreeRoots {
		if nodeExistsInTree(root, id) {
			s.ActiveTab = tabIdx
			ensureAncestorsExpanded(root, id)
			s.rebuildFlattened()
			for i, row := range s.Flattened {
				if row.Node.Record.ID == id {
					s.Cursor = i
					s.MarqueeTick = 0
					s.adjustScroll()
					return
				}
			}
			return
		}
	}
}

func ensureAncestorsExpanded(n *db.TreeNode, id int) bool {
	if n == nil || n.Record == nil {
		return false
	}
	if n.Record.ID == id {
		return true
	}
	for _, child := range n.Children {
		if ensureAncestorsExpanded(child, id) {
			n.Expanded = true
			return true
		}
	}
	return false
}

func nodeExistsInTree(n *db.TreeNode, id int) bool {
	if n == nil || n.Record == nil {
		return false
	}
	if n.Record.ID == id {
		return true
	}
	for _, child := range n.Children {
		if nodeExistsInTree(child, id) {
			return true
		}
	}
	return false
}

func (s *ScreenTree) ToggleExpand() {
	node := s.SelectedNode()
	if node == nil {
		return
	}
	if len(node.Children) > 0 {
		node.Expanded = !node.Expanded
		s.rebuildFlattened()
	} else if node.Parent != nil {
		node.Parent.Expanded = false
		s.rebuildFlattened()
	}
}

func (s *ScreenTree) Expand() {
	node := s.SelectedNode()
	if node != nil && len(node.Children) > 0 {
		if !node.Expanded {
			node.Expanded = true
			s.rebuildFlattened()
		} else if s.Cursor < len(s.Flattened)-1 {
			s.Cursor++
			s.adjustScroll()
		}
	}
}

func (s *ScreenTree) Collapse() {
	node := s.SelectedNode()
	if node != nil {
		if len(node.Children) > 0 && node.Expanded {
			node.Expanded = false
			s.rebuildFlattened()
		} else if node.Parent != nil {
			for i, r := range s.Flattened {
				if r.Node == node.Parent {
					s.Cursor = i
					s.adjustScroll()
					break
				}
			}
		}
	}
}

func (s *ScreenTree) CursorUp() {
	if s.Cursor > 0 {
		s.Cursor--
		s.adjustScroll()
	}
}

func (s *ScreenTree) CursorDown() {
	if s.Cursor < len(s.Flattened)-1 {
		s.Cursor++
		s.adjustScroll()
	}
}

func (s *ScreenTree) Update(msg tea.Msg) (*ScreenTree, tea.Cmd) {
	var cmd tea.Cmd

	if s.Filtering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "enter":
				s.Filtering = false
				s.FilterInput.Blur()
				return s, nil
			}
		}
		s.FilterInput, cmd = s.FilterInput.Update(msg)
		s.rebuildFlattened()
		return s, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		s.MarqueeTick = 0
		switch msg.String() {
		case "up", "k":
			s.CursorUp()
		case "down", "j":
			s.CursorDown()
		case "left", "h":
			s.Collapse()
		case "right", "l":
			s.Expand()
		case " ", "space":
			s.ToggleExpand()
		case "[", "{", "shift+tab":
			s.PrevTab()
		case "]", "}", "tab":
			s.NextTab()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			s.SetActiveTab(idx)
		case "home":
			s.Cursor = 0
			s.adjustScroll()
		case "end", "G":
			if len(s.Flattened) > 0 {
				s.Cursor = len(s.Flattened) - 1
				s.adjustScroll()
			}
		case "pageup":
			s.Cursor -= 10
			if s.Cursor < 0 {
				s.Cursor = 0
			}
			s.adjustScroll()
		case "pagedown":
			s.Cursor += 10
			if s.Cursor >= len(s.Flattened) {
				s.Cursor = len(s.Flattened) - 1
			}
			s.adjustScroll()
		case "/":
			s.Filtering = true
			s.FilterInput.Reset()
			s.FilterInput.Focus()
			return s, nil
		}
	}

	return s, nil
}

func (s *ScreenTree) View(width, height int, toastMsg string, isError bool) string {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}
	s.Width = width
	s.Height = height
	s.adjustScroll()

	// 1. App Header Bar with Azure DevOps brand logo
	headerTitle := RenderAzureDevOpsBrand()
	sep := lipgloss.NewStyle().Foreground(ColorSubtle).Render("│")
	subtitle := lipgloss.NewStyle().Foreground(ColorMuted).Render("Work Item Hierarchy & Progress")
	headerLeft := lipgloss.JoinHorizontal(lipgloss.Center, headerTitle, "  ", sep, "  ", subtitle)

	// Filter bar (if active or has filter)
	tabFilter := s.GetTabFilter(s.ActiveTab)
	var filterElements []string

	if s.GlobalFilterOpts.IsActive() {
		filterBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(ColorYellow).
			Bold(true).
			Padding(0, 1).
			Render("🌐 GLOBAL")
		summary := lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true).
			Render(s.GlobalFilterOpts.SummaryString())
		filterElements = append(filterElements, filterBadge, summary)
	}

	if tabFilter.IsActive() {
		tabFilterBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(ColorCyan).
			Bold(true).
			Padding(0, 1).
			Render("📑 TAB")
		tabSummary := lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true).
			Render(tabFilter.SummaryString())
		filterElements = append(filterElements, tabFilterBadge, tabSummary)
	}

	if len(filterElements) > 0 {
		filterJoined := strings.Join(filterElements, " ")
		headerLeft = lipgloss.JoinHorizontal(lipgloss.Center, headerLeft, "   ", filterJoined)
	} else if s.Filtering || s.FilterInput.Value() != "" {
		searchBar := "   " + s.FilterInput.View()
		headerLeft = lipgloss.JoinHorizontal(lipgloss.Center, headerLeft, searchBar)
	}

	var headerRight string
	if s.IncompleteView {
		headerRight = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(ColorYellow).
			Bold(true).
			Padding(0, 1).
			Render("⚠️  INCOMPLETE VIEW")
	}

	leftWidth := lipgloss.Width(headerLeft)
	rightWidth := lipgloss.Width(headerRight)
	gap := width - leftWidth - rightWidth - 2
	var headerBar string
	if headerRight != "" && gap > 0 {
		headerBar = headerLeft + strings.Repeat(" ", gap) + headerRight
	} else if headerRight != "" {
		headerBar = lipgloss.JoinHorizontal(lipgloss.Center, headerLeft, "  ", headerRight)
	} else {
		headerBar = headerLeft
	}
	headerBar = lipgloss.NewStyle().Width(width).MaxHeight(1).Render(headerBar)
	if strings.Contains(headerBar, "\n") {
		headerBar = strings.Split(headerBar, "\n")[0]
	}

	// 2. Table Column Dimensions
	typeColWidth := 16
	statusColWidth := 16
	assigneeColWidth := 20
	estColWidth := 12
	progColWidth := 18

	fixedColsWidth := typeColWidth + statusColWidth + assigneeColWidth + estColWidth + progColWidth + 2
	treeColWidth := width - fixedColsWidth
	if treeColWidth < 20 {
		treeColWidth = 20
	}

	// Table Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CBD5E1"))
	thTree := headerStyle.Width(treeColWidth).MaxHeight(1).Render("  WORK ITEM (TREE)")
	thType := headerStyle.Width(typeColWidth).MaxHeight(1).Render(" TYPE")
	thStatus := headerStyle.Width(statusColWidth).MaxHeight(1).Render(" STATUS")
	thAssignee := headerStyle.Width(assigneeColWidth).MaxHeight(1).Render(" ASSIGNED TO")
	thEst := headerStyle.Width(estColWidth).MaxHeight(1).Render(" ESTIMATE")
	thProg := headerStyle.Width(progColWidth).MaxHeight(1).Render(" PROGRESS / WORK")

	usedHeaderWidth := treeColWidth + typeColWidth + statusColWidth + assigneeColWidth + estColWidth + progColWidth
	restHeaderWidth := width - usedHeaderWidth
	if restHeaderWidth < 0 {
		restHeaderWidth = 0
	}
	thRest := headerStyle.MaxHeight(1).Render(strings.Repeat(" ", restHeaderWidth))

	tableHeader := lipgloss.JoinHorizontal(lipgloss.Left, thTree, thType, thStatus, thAssignee, thEst, thProg, thRest)
	if strings.Contains(tableHeader, "\n") {
		tableHeader = strings.Split(tableHeader, "\n")[0]
	}
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).MaxHeight(1).Render(strings.Repeat("─", width))
	if strings.Contains(separator, "\n") {
		separator = strings.Split(separator, "\n")[0]
	}

	// 3. Bottom Tab Bar (horizontally above quick command ribbon)
	tabBar := s.renderTabBar(width)
	tabBarHeight := 0
	if tabBar != "" {
		tabBarHeight = 1
	}

	headerHeight := 4 // headerBar (1) + blank spacer (1) + tableHeader (1) + separator (1)
	footerHeight := 1 // ribbon (1)
	visibleHeight := height - headerHeight - footerHeight - tabBarHeight
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	var renderedRows []string

	if len(s.Flattened) == 0 {
		emptyMsg := "No work items loaded. Press [t] to track an item, or [r] to sync."
		if s.FilterOpts.IsActive() || s.FilterInput.Value() != "" {
			emptyMsg = "No work items match current filter. Press [F] or [f] to modify filter."
		}
		emptyLine := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Padding(2, 4).
			MaxHeight(1).
			Render(emptyMsg)
		renderedRows = append(renderedRows, emptyLine)
		for len(renderedRows) < visibleHeight {
			renderedRows = append(renderedRows, "")
		}
	} else {
		endIdx := s.ScrollOffset + visibleHeight
		if endIdx > len(s.Flattened) {
			endIdx = len(s.Flattened)
		}

		for i := s.ScrollOffset; i < endIdx; i++ {
			row := s.Flattened[i]
			node := row.Node
			rec := node.Record
			isSelected := (i == s.Cursor)

			typeColor := GetTypeColor(rec.WorkItemType)
			isRemoved := strings.EqualFold(rec.State, "Removed")

			// Base style for row cells
			cellBase := lipgloss.NewStyle()
			styleTree := lipgloss.NewStyle().Foreground(ColorTreeLine)
			styleGlyph := lipgloss.NewStyle().Foreground(typeColor).Bold(true)
			styleID := lipgloss.NewStyle().Foreground(ColorMuted)
			styleTitle := lipgloss.NewStyle().Foreground(typeColor).Bold(true)
			if isRemoved {
				styleTitle = styleTitle.Strikethrough(true).Foreground(ColorMuted)
			}
			styleAssignee := lipgloss.NewStyle().Foreground(ColorFg)
			if rec.AssignedToName == "" {
				styleAssignee = styleAssignee.Foreground(ColorMuted)
			}
			styleEst := lipgloss.NewStyle().Foreground(ColorMuted)
			styleProg := lipgloss.NewStyle().Foreground(ColorSecondary)

			if isSelected {
				cellBase = lipgloss.NewStyle().Background(ColorSelectedBg)
				if rec.AssignedToName != "" {
					styleAssignee = styleAssignee.Foreground(ColorYellow).Bold(true)
				}
				styleTree = styleTree.Background(ColorSelectedBg)
				styleGlyph = styleGlyph.Background(ColorSelectedBg)
				styleID = styleID.Background(ColorSelectedBg)
				styleTitle = styleTitle.Background(ColorSelectedBg)
				styleAssignee = styleAssignee.Background(ColorSelectedBg)
				styleEst = styleEst.Background(ColorSelectedBg)
				styleProg = styleProg.Background(ColorSelectedBg)
			}

			// Tree connectors & depth indentation
			connectors := styleTree.Render("  " + row.Indent + row.Branch)
			glyph := styleGlyph.Render(row.Glyph)

			// Work item ID (empty for Query root node)
			var idStr string
			if rec.WorkItemType != "Query" && !node.IsQuery {
				idStr = styleID.Render(fmt.Sprintf("#%d ", rec.ID))
			}

			// Incomplete View Warning Indicator
			var warnGlyph string
			if s.IncompleteView {
				isIncomplete := false
				if s.incompleteCache != nil {
					if cached, ok := s.incompleteCache[rec.ID]; ok {
						isIncomplete = cached
					} else {
						isIncomplete = IsItemIncompleteWithDB(rec, s.Config, s.DB, time.Now())
						s.incompleteCache[rec.ID] = isIncomplete
					}
				} else {
					isIncomplete = IsItemIncompleteWithDB(rec, s.Config, s.DB, time.Now())
				}

				if isIncomplete {
					warnStyle := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
					if isSelected {
						warnStyle = warnStyle.Background(ColorSelectedBg)
					}
					warnGlyph = warnStyle.Render("⚠️ ")
				}
			}

			// Title with dynamic column clipping and marquee scroll on selection
			prefixLen := runewidth.StringWidth(StripEscapes(connectors)) +
				runewidth.StringWidth(StripEscapes(glyph)) +
				runewidth.StringWidth(StripEscapes(warnGlyph)) +
				runewidth.StringWidth(StripEscapes(idStr))

			availTitleWidth := treeColWidth - prefixLen
			if availTitleWidth < 0 {
				availTitleWidth = 0
			}

			titleText := rec.Title
			if isSelected {
				titleText = ScrollClippedText(titleText, availTitleWidth, s.MarqueeTick)
			} else {
				titleText = truncateString(titleText, availTitleWidth)
			}

			titleStyled := styleTitle.Render(titleText)
			treeCellContent := connectors + glyph + warnGlyph + idStr + titleStyled

			// Badges & Fields
			typeBadge := RenderTypeBadge(rec.WorkItemType)
			var statusBadge string
			if rec.WorkItemType != "Query" {
				statusBadge = RenderStateBadge(rec.State)
			}

			assigneeText := rec.AssignedToName
			if assigneeText == "" {
				if rec.WorkItemType == "Query" || node.IsQuery {
					assigneeText = ""
				} else {
					assigneeText = "Unassigned"
				}
			}
			maxAssigneeLen := assigneeColWidth - 3
			if maxAssigneeLen < 0 {
				maxAssigneeLen = 0
			}
			var dispAssignee string
			if isSelected && rec.AssignedToName != "" {
				dispAssignee = ScrollClippedText(assigneeText, maxAssigneeLen, s.MarqueeTick)
			} else {
				dispAssignee = truncateString(assigneeText, maxAssigneeLen)
			}
			assigneeStr := styleAssignee.Render(dispAssignee)

			estStr := styleEst.Render(node.EstimateDisplayString())
			progStr := styleProg.Render(node.RollupProgressString())

			// Cell rendering with exact widths and MaxHeight(1)
			cTree := cellBase.Width(treeColWidth).MaxHeight(1).Render(treeCellContent)
			cType := cellBase.Width(typeColWidth).MaxHeight(1).Render(" " + typeBadge)
			cStatus := cellBase.Width(statusColWidth).MaxHeight(1).Render(" " + statusBadge)
			cAssignee := cellBase.Width(assigneeColWidth).MaxHeight(1).Render(" " + assigneeStr)
			cEst := cellBase.Width(estColWidth).MaxHeight(1).Render(" " + estStr)
			cProg := cellBase.Width(progColWidth).MaxHeight(1).Render(" " + progStr)

			usedWidth := treeColWidth + typeColWidth + statusColWidth + assigneeColWidth + estColWidth + progColWidth
			restWidth := width - usedWidth
			if restWidth < 0 {
				restWidth = 0
			}
			cRest := cellBase.MaxHeight(1).Render(strings.Repeat(" ", restWidth))

			rowLine := lipgloss.JoinHorizontal(lipgloss.Top, cTree, cType, cStatus, cAssignee, cEst, cProg, cRest)
			if strings.Contains(rowLine, "\n") {
				rowLine = strings.Split(rowLine, "\n")[0]
			}
			renderedRows = append(renderedRows, rowLine)
		}

		// Pad with blank lines if fewer items than visible height to prevent jumping
		for len(renderedRows) < visibleHeight {
			renderedRows = append(renderedRows, "")
		}
		if len(renderedRows) > visibleHeight {
			renderedRows = renderedRows[:visibleHeight]
		}
	}

	bodyContent := strings.Join(renderedRows, "\n")

	// 4. Bottom Ribbon Shortcuts (concise labels for crisp one-line display)
	shortcuts := []Shortcut{
		{Key: "Enter", Label: "View"},
		{Key: "f", Label: "Filter"},
	}
	if s.GlobalFilterOpts.IsActive() || s.GetTabFilter(s.ActiveTab).IsActive() {
		shortcuts = append(shortcuts, Shortcut{Key: "F", Label: "Clear"})
	}
	shortcuts = append(shortcuts,
		Shortcut{Key: "g", Label: "Gemini"},
		Shortcut{Key: "o", Label: "Open"},
		Shortcut{Key: "C", Label: "Copy"},
		Shortcut{Key: "I", Label: "Incomplete"},
		Shortcut{Key: "c", Label: "Create"},
		Shortcut{Key: "t", Label: "Track"},
		Shortcut{Key: "T", Label: "Tabs"},
		Shortcut{Key: "s", Label: "Status"},
		Shortcut{Key: "v", Label: "Activity"},
		Shortcut{Key: "S", Label: "Settings"},
		Shortcut{Key: "d", Label: "Dates"},
		Shortcut{Key: "a", Label: "Assign"},
		Shortcut{Key: "e", Label: "Est"},
		Shortcut{Key: "D", Label: "Del"},
		Shortcut{Key: "r", Label: "Sync"},
		Shortcut{Key: "/", Label: "Search"},
		Shortcut{Key: "q", Label: "Quit"},
	)

	ribbon := RenderBottomRibbon(width, "", shortcuts, toastMsg, isError)
	if strings.Contains(ribbon, "\n") {
		ribbon = strings.Split(ribbon, "\n")[0]
	}

	var elements []string
	elements = append(elements, headerBar, "", tableHeader, separator, bodyContent)
	if tabBar != "" {
		elements = append(elements, tabBar)
	}
	elements = append(elements, ribbon)

	return strings.Join(elements, "\n")
}

func (s *ScreenTree) renderTabBar(width int) string {
	if len(s.TreeRoots) == 0 {
		return ""
	}

	prefix := lipgloss.NewStyle().Foreground(ColorMuted).Bold(true).Render("Roots: ")
	prefixWidth := runewidth.StringWidth(StripEscapes(prefix))

	tabCount := len(s.TreeRoots)
	if tabCount == 0 {
		return ""
	}

	type pillInfo struct {
		rendered string
		width    int
	}
	pills := make([]pillInfo, tabCount)

	for i, root := range s.TreeRoots {
		if root.Record == nil {
			continue
		}
		isActive := (i == s.ActiveTab)
		isQuery := root.IsQuery || root.Record.WorkItemType == "Query"

		title := root.TabTitle()
		if title == "" {
			if isQuery {
				title = "Query"
			} else {
				title = "(Root)"
			}
		}
		title = truncateString(title, 24)
		if s.GetTabFilter(i).IsActive() {
			title += " 📑"
		}

		numStr := fmt.Sprintf("%d:", i+1)

		var pillStr string
		if isQuery {
			if isActive {
				numPart := lipgloss.NewStyle().Foreground(ColorRibbonKey).Bold(true).Render(numStr)
				typePart := lipgloss.NewStyle().Foreground(ColorPurple).Bold(true).Render("[Query]")
				titlePart := lipgloss.NewStyle().Foreground(ColorFg).Bold(true).Render(title)

				pillContent := fmt.Sprintf("▶ %s %s %s", numPart, typePart, titlePart)
				pillStr = lipgloss.NewStyle().
					Foreground(ColorRibbonKey).
					Bold(true).
					Render("[" + pillContent + "]")
			} else {
				numPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(numStr)
				typePart := lipgloss.NewStyle().Foreground(ColorPurple).Render("[Query]")
				titlePart := lipgloss.NewStyle().Foreground(ColorMuted).Render(title)

				pillContent := fmt.Sprintf("  %s %s %s  ", numPart, typePart, titlePart)
				pillStr = lipgloss.NewStyle().
					Foreground(ColorMuted).
					Render(pillContent)
			}
		} else {
			typeColor := GetTypeColor(root.Record.WorkItemType)
			idStr := fmt.Sprintf("#%d", root.Record.ID)
			shortType := FormatItemType(root.Record.WorkItemType)

			if isActive {
				numPart := lipgloss.NewStyle().Foreground(ColorRibbonKey).Bold(true).Render(numStr)
				idPart := lipgloss.NewStyle().Foreground(ColorRibbonKey).Bold(true).Render(idStr)
				typePart := lipgloss.NewStyle().Foreground(typeColor).Bold(true).Render("[" + shortType + "]")
				titlePart := lipgloss.NewStyle().Foreground(ColorFg).Bold(true).Render(title)

				pillContent := fmt.Sprintf("▶ %s %s %s %s", numPart, idPart, typePart, titlePart)
				pillStr = lipgloss.NewStyle().
					Foreground(ColorRibbonKey).
					Bold(true).
					Render("[" + pillContent + "]")
			} else {
				numPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(numStr)
				idPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(idStr)
				typePart := lipgloss.NewStyle().Foreground(ColorSubtle).Render("[" + shortType + "]")
				titlePart := lipgloss.NewStyle().Foreground(ColorMuted).Render(title)

				pillContent := fmt.Sprintf("  %s %s %s %s  ", numPart, idPart, typePart, titlePart)
				pillStr = lipgloss.NewStyle().
					Foreground(ColorMuted).
					Render(pillContent)
			}
		}

		pills[i] = pillInfo{
			rendered: pillStr,
			width:    runewidth.StringWidth(StripEscapes(pillStr)),
		}
	}

	active := s.ActiveTab
	if active < 0 {
		active = 0
	}
	if active >= tabCount {
		active = tabCount - 1
	}

	availWidth := width - prefixWidth
	if availWidth < 10 {
		availWidth = 10
	}

	leftInd := lipgloss.NewStyle().Foreground(ColorRibbonKey).Bold(true).Render("◀ ")
	leftIndWidth := runewidth.StringWidth(StripEscapes(leftInd))
	rightInd := lipgloss.NewStyle().Foreground(ColorRibbonKey).Bold(true).Render(" ▶")
	rightIndWidth := runewidth.StringWidth(StripEscapes(rightInd))

	startIdx := active
	endIdx := active

	calcTotalWidth := func(s, e int) int {
		w := 0
		if s > 0 {
			w += leftIndWidth
		}
		for i := s; i <= e; i++ {
			w += pills[i].width
			if i < e {
				w += 1
			}
		}
		if e < tabCount-1 {
			w += rightIndWidth
		}
		return w
	}

	// Expand sliding window to fit as many surrounding tabs as possible while keeping active tab in view
	for {
		expanded := false
		if startIdx > 0 && calcTotalWidth(startIdx-1, endIdx) <= availWidth {
			startIdx--
			expanded = true
		}
		if endIdx < tabCount-1 && calcTotalWidth(startIdx, endIdx+1) <= availWidth {
			endIdx++
			expanded = true
		}
		if !expanded {
			break
		}
	}

	var visiblePills []string
	for i := startIdx; i <= endIdx; i++ {
		visiblePills = append(visiblePills, pills[i].rendered)
	}

	var tabLine strings.Builder
	tabLine.WriteString(prefix)
	if startIdx > 0 {
		tabLine.WriteString(leftInd)
	}
	tabLine.WriteString(strings.Join(visiblePills, " "))
	if endIdx < tabCount-1 {
		tabLine.WriteString(rightInd)
	}

	rendered := lipgloss.NewStyle().
		Width(width).
		MaxHeight(1).
		Render(tabLine.String())
	if strings.Contains(rendered, "\n") {
		rendered = strings.Split(rendered, "\n")[0]
	}
	return rendered
}

package tui

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// RelatedLinkItem represents a linked work item or PR in Screen 2
type RelatedLinkItem struct {
	ID        int
	Type      string // "Parent", "Child", "Related", "Pull Request", "Attachment", "Link"
	Title     string
	State     string
	Subtitle  string
	URL       string
	IsPR      bool
}

func formatEffort(val float64) string {
	if val <= 0 {
		return "—"
	}
	if val == float64(int(val)) {
		return fmt.Sprintf("%.0fpts", val)
	}
	return fmt.Sprintf("%.1fpts", val)
}

// ScreenItem is the Detailed Work Item View (Screen 2)
type ScreenItem struct {
	Item            *db.WorkItemRecord
	Comments        []db.CommentRecord
	RelatedItems    []*db.WorkItemRecord
	PullRequests    []*db.PullRequestRecord
	RelatedLinks    []RelatedLinkItem
	SelectedLinkIdx int
	Config          *config.Config
	DB              *db.DB
	IncompleteView  bool
	Viewport        viewport.Model
	Ready           bool
	Width           int
	Height          int

	dirty              bool
	lastRenderedWidth  int
	lastIncompleteView bool
	lastSelectedLink   int
}

func NewScreenItem() *ScreenItem {
	return &ScreenItem{}
}

func (s *ScreenItem) SetItem(item *db.WorkItemRecord, comments []db.CommentRecord) {
	s.Item = item
	s.Comments = comments
	s.rebuildRelatedLinks()
	s.dirty = true
	s.updateViewportContent()
}

func (s *ScreenItem) SetData(item *db.WorkItemRecord, comments []db.CommentRecord, relatedItems []*db.WorkItemRecord, prs []*db.PullRequestRecord) {
	s.Item = item
	s.Comments = comments
	s.RelatedItems = relatedItems
	s.PullRequests = prs
	s.rebuildRelatedLinks()
	s.dirty = true
	s.updateViewportContent()
}

func (s *ScreenItem) SetComments(comments []db.CommentRecord) {
	s.Comments = comments
	s.dirty = true
	s.updateViewportContent()
}

var (
	linkPrURLRegex = regexp.MustCompile(`(?i)/_git/([^/]+)/pullrequest/(\d+)`)
	linkWiURLRegex = regexp.MustCompile(`(?i)/_workitems/edit/(\d+)`)
)

func (s *ScreenItem) rebuildRelatedLinks() {
	s.RelatedLinks = nil
	if s.Item == nil {
		return
	}

	seen := make(map[string]bool)

	// 1. Explicit PR records from DB/API
	for _, pr := range s.PullRequests {
		if pr.URL != "" {
			seen[pr.URL] = true
		}
		s.RelatedLinks = append(s.RelatedLinks, RelatedLinkItem{
			ID:       pr.ID,
			Type:     "Pull Request",
			Title:    pr.Title,
			State:    pr.Status,
			Subtitle: fmt.Sprintf("%s ➔ %s", pr.SourceBranch, pr.TargetBranch),
			URL:      pr.URL,
			IsPR:     true,
		})
	}

	// 2. Discover PRs and links mentioned in Description, AC, and Comments
	var textToScan strings.Builder
	textToScan.WriteString(s.Item.Description + "\n")
	textToScan.WriteString(s.Item.AcceptanceCriteria + "\n")
	for _, c := range s.Comments {
		textToScan.WriteString(c.Text + "\n")
	}

	var discoveredPRs []RelatedLinkItem
	var discoveredOthers []RelatedLinkItem

	matches := urlRegex.FindAllString(textToScan.String(), -1)
	for _, m := range matches {
		u, _ := CleanURL(m)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true

		if prMatch := linkPrURLRegex.FindStringSubmatch(u); len(prMatch) >= 3 {
			repo := prMatch[1]
			prID, _ := strconv.Atoi(prMatch[2])
			discoveredPRs = append(discoveredPRs, RelatedLinkItem{
				ID:       prID,
				Type:     "Pull Request",
				Title:    fmt.Sprintf("PR #%d (%s)", prID, repo),
				State:    "Active",
				Subtitle: u,
				URL:      u,
				IsPR:     true,
			})
		} else if wiMatch := linkWiURLRegex.FindStringSubmatch(u); len(wiMatch) >= 2 {
			wiID, _ := strconv.Atoi(wiMatch[1])
			if wiID != s.Item.ID {
				discoveredOthers = append(discoveredOthers, RelatedLinkItem{
					ID:       wiID,
					Type:     "Work Item",
					Title:    fmt.Sprintf("Work Item #%d", wiID),
					State:    "",
					Subtitle: u,
					URL:      u,
					IsPR:     false,
				})
			}
		} else if strings.Contains(u, "/_apis/wit/attachments/") || strings.Contains(u, "/attachments/") {
			fileName := "Attachment"
			if parsedURL, err := url.Parse(u); err == nil {
				if fn := parsedURL.Query().Get("fileName"); fn != "" {
					if unescaped, err := url.QueryUnescape(fn); err == nil {
						fileName = unescaped
					} else {
						fileName = fn
					}
				}
			}
			discoveredOthers = append(discoveredOthers, RelatedLinkItem{
				ID:       0,
				Type:     "Attachment",
				Title:    fileName,
				State:    "",
				Subtitle: u,
				URL:      u,
				IsPR:     false,
			})
		} else {
			discoveredOthers = append(discoveredOthers, RelatedLinkItem{
				ID:       0,
				Type:     "Link",
				Title:    u,
				State:    "",
				Subtitle: "",
				URL:      u,
				IsPR:     false,
			})
		}
	}

	s.RelatedLinks = append(s.RelatedLinks, discoveredPRs...)
	s.RelatedLinks = append(s.RelatedLinks, discoveredOthers...)

	if s.SelectedLinkIdx >= len(s.RelatedLinks) {
		s.SelectedLinkIdx = 0
	}
}

func (s *ScreenItem) SelectedLink() *RelatedLinkItem {
	if len(s.RelatedLinks) > 0 && s.SelectedLinkIdx >= 0 && s.SelectedLinkIdx < len(s.RelatedLinks) {
		return &s.RelatedLinks[s.SelectedLinkIdx]
	}
	return nil
}

func (s *ScreenItem) NextLink() {
	if len(s.RelatedLinks) > 0 {
		s.SelectedLinkIdx = (s.SelectedLinkIdx + 1) % len(s.RelatedLinks)
		s.updateViewportContent()
	}
}

func (s *ScreenItem) PrevLink() {
	if len(s.RelatedLinks) > 0 {
		s.SelectedLinkIdx = (s.SelectedLinkIdx - 1 + len(s.RelatedLinks)) % len(s.RelatedLinks)
		s.updateViewportContent()
	}
}

func (s *ScreenItem) updateViewportContent() {
	if s.Item == nil {
		return
	}

	content := s.renderItemContent(s.Width)
	s.Viewport.SetContent(content)
	s.dirty = false
	s.lastRenderedWidth = s.Width
	s.lastIncompleteView = s.IncompleteView
	s.lastSelectedLink = s.SelectedLinkIdx
}

func (s *ScreenItem) InitViewport(width, height int) {
	s.Width = width
	s.Height = height
	headerHeight := 1
	footerHeight := 1
	vpHeight := height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}

	vpWidth := width - 2
	if vpWidth < 10 {
		vpWidth = 10
	}

	if !s.Ready {
		s.Viewport = viewport.New(vpWidth, vpHeight)
		s.Ready = true
	} else {
		s.Viewport.Width = vpWidth
		s.Viewport.Height = vpHeight
	}
	s.updateViewportContent()
}

func (s *ScreenItem) SetSize(width, height int) {
	s.InitViewport(width, height)
}

func (s *ScreenItem) Update(msg tea.Msg) (*ScreenItem, tea.Cmd) {
	var cmd tea.Cmd
	s.Viewport, cmd = s.Viewport.Update(msg)
	return s, cmd
}

func (s *ScreenItem) renderItemContent(width int) string {
	if s.Item == nil {
		return "No work item selected."
	}

	cardWidth := width - 4
	if cardWidth < 40 {
		cardWidth = 40
	}

	lblStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	valStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorFg)

	// 1. Details & Estimates in a single unified, compact header card
	assignee := s.Item.AssignedToName
	if s.Item.AssignedToEmail != "" {
		assignee = fmt.Sprintf("%s <%s>", s.Item.AssignedToName, s.Item.AssignedToEmail)
	}
	if assignee == "" {
		assignee = "Unassigned"
	}

	creator := s.Item.CreatedByName
	if s.Item.CreatedDate != "" {
		creator = fmt.Sprintf("%s on %s", s.Item.CreatedByName, formatDateTime(s.Item.CreatedDate))
	}

	priorityStr := fmt.Sprintf("%d", s.Item.Priority)
	if s.Item.Priority == 0 {
		priorityStr = "—"
	}

	tagsStr := s.Item.Tags
	if tagsStr == "" {
		tagsStr = "—"
	}

	orig := s.Item.OriginalEstimate
	rem := s.Item.RemainingWork
	comp := s.Item.CompletedWork
	effort := s.Item.Effort

	var total float64 = orig
	if total <= 0 {
		total = comp + rem
	}

	progressBar := RenderProgressBar(16, comp, total)

	activityStr := s.Item.Activity
	if activityStr == "" {
		activityStr = "—"
	}

	metaLine1 := fmt.Sprintf(
		"%s %s    │    %s %s",
		lblStyle.Render("Assigned:"), valStyle.Foreground(ColorYellow).Render(assignee),
		lblStyle.Render("Iteration:"), valStyle.Foreground(ColorSecondary).Render(s.Item.IterationPath),
	)
	metaLine2 := fmt.Sprintf(
		"%s %s    │    %s %s",
		lblStyle.Render("Created: "), valStyle.Render(creator),
		lblStyle.Render("Area Path:"), valStyle.Render(s.Item.AreaPath),
	)
	metaLine3 := fmt.Sprintf(
		"%s %s       │    %s %s       │    %s %s",
		lblStyle.Render("Priority:"), valStyle.Foreground(ColorYellow).Render(priorityStr),
		lblStyle.Render("Activity:"), valStyle.Foreground(ColorCyan).Render(activityStr),
		lblStyle.Render("Tags:     "), valStyle.Render(tagsStr),
	)

	now := time.Now()

	startDateStr := ado.FormatDisplayDateTime(s.Item.StartDate)
	targetDateStr := ado.FormatDisplayDateTime(s.Item.TargetDate)
	finishDateStr := ado.FormatDisplayDateTime(s.Item.FinishDate)

	colStart := fmt.Sprintf("%s %s", lblStyle.Render("Start Date:"), valStyle.Foreground(ColorCyan).Render(startDateStr))
	colTarget := fmt.Sprintf("%s %s", lblStyle.Render("Target Date:"), valStyle.Foreground(ColorYellow).Render(targetDateStr))
	colFinish := fmt.Sprintf("%s %s", lblStyle.Render("Finish Date:"), valStyle.Foreground(ColorGreen).Render(finishDateStr))
	sep := "   │   "

	dateLine := fmt.Sprintf("%s%s%s%s%s", colStart, sep, colTarget, sep, colFinish)

	// Check if Target Date is overdue and format small warning text under Target Date (only for active items)
	var dateOverdueLine string
	if !IsItemDoneOrClosed(s.Item) {
		targetOverdue := FormatOverdueSmallText(s.Item.TargetDate, now)
		if targetOverdue != "" {
			warnSmallStyle := lipgloss.NewStyle().Foreground(ColorYellow).Italic(true)
			wStart := lipgloss.Width(colStart)
			wSep := lipgloss.Width(sep)
			targetWarning := lipgloss.NewStyle().Foreground(ColorYellow).Render("↳ ") + warnSmallStyle.Render(targetOverdue)
			dateOverdueLine = strings.Repeat(" ", wStart+wSep) + targetWarning
		}
	}

	// Check for incomplete criteria (only when Incomplete View mode is active)
	var incompleteNotice string
	if s.IncompleteView {
		if incompleteReasons := GetIncompleteReasonsWithDB(s.Item, s.Config, s.DB, now); len(incompleteReasons) > 0 {
			incompleteNotice = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorRed).
				Render("⚠️  Incomplete: " + strings.Join(incompleteReasons, " • "))
		}
	}

	hasEffort := true
	hasOrig := true
	hasComp := true
	hasRem := true

	if s.DB != nil && s.Item != nil {
		perms, _ := s.DB.ResolveItemFieldPermissions(s.Item.Org, s.Item.Project, s.Item.WorkItemType, s.Item.State)
		hasEffort = perms.ShowEffort
		hasOrig = perms.ShowOriginal
		hasComp = perms.ShowCompleted
		hasRem = perms.ShowRemaining
	}

	var estSegments []string
	if hasOrig {
		estSegments = append(estSegments, fmt.Sprintf("%s %s", lblStyle.Render("Original:"), valStyle.Foreground(ColorYellow).Render(fmt.Sprintf("%.1fh", orig))))
	}
	if hasComp {
		estSegments = append(estSegments, fmt.Sprintf("%s %s", lblStyle.Render("Completed:"), valStyle.Foreground(ColorGreen).Render(fmt.Sprintf("%.1fh", comp))))
	}
	if hasRem {
		estSegments = append(estSegments, fmt.Sprintf("%s %s", lblStyle.Render("Remaining:"), valStyle.Foreground(ColorSecondary).Render(fmt.Sprintf("%.1fh", rem))))
	}

	if hasEffort {
		effortStr := formatEffort(effort)
		effortStyle := valStyle.Foreground(ColorPurple)
		if effort <= 0 {
			effortStyle = valStyle.Foreground(ColorMuted)
		}
		estSegments = append(estSegments, fmt.Sprintf("%s %s", lblStyle.Render("Effort:"), effortStyle.Render(effortStr)))
	}

	if len(estSegments) == 0 {
		effortStr := formatEffort(effort)
		effortStyle := valStyle.Foreground(ColorPurple)
		if effort <= 0 {
			effortStyle = valStyle.Foreground(ColorMuted)
		}
		estSegments = []string{
			fmt.Sprintf("%s %s", lblStyle.Render("Original:"), valStyle.Foreground(ColorYellow).Render(fmt.Sprintf("%.1fh", orig))),
			fmt.Sprintf("%s %s", lblStyle.Render("Completed:"), valStyle.Foreground(ColorGreen).Render(fmt.Sprintf("%.1fh", comp))),
			fmt.Sprintf("%s %s", lblStyle.Render("Remaining:"), valStyle.Foreground(ColorSecondary).Render(fmt.Sprintf("%.1fh", rem))),
			fmt.Sprintf("%s %s", lblStyle.Render("Effort:"), effortStyle.Render(effortStr)),
		}
	}

	estLine := strings.Join(estSegments, "  │  ")
	if hasOrig || hasComp {
		estLine += fmt.Sprintf("   %s %s %s", lblStyle.Render("Progress:"), progressBar, lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("(%.1f / %.1fh)", comp, total)))
	}

	divider := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", cardWidth-4))

	compactHeaderRows := []string{
		metaLine1,
		metaLine2,
		metaLine3,
		dateLine,
	}
	if dateOverdueLine != "" {
		compactHeaderRows = append(compactHeaderRows, dateOverdueLine)
	}
	compactHeaderRows = append(compactHeaderRows, divider, estLine)
	if incompleteNotice != "" {
		compactHeaderRows = append(compactHeaderRows, divider, incompleteNotice)
	}

	compactHeaderContent := lipgloss.JoinVertical(lipgloss.Left, compactHeaderRows...)

	metaBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(0, 1).
		Width(cardWidth).
		Render(compactHeaderContent)

	innerWidth := cardWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	// 2. Description Section
	descHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("📝 Description")
	var descFormatted string
	if descRaw := StripAndFormatHTML(s.Item.Description); descRaw != "" {
		descFormatted = RenderMarkdown(descRaw, innerWidth)
	}
	if strings.TrimSpace(descFormatted) == "" {
		descFormatted = lipgloss.NewStyle().Foreground(ColorMuted).Render("No description provided.")
	}

	descBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(1, 2).
		Width(cardWidth).
		Render(descFormatted)

	// 3. Acceptance Criteria (if present)
	var acSection string
	if acRaw := StripAndFormatHTML(s.Item.AcceptanceCriteria); acRaw != "" {
		acHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("Acceptance Criteria")
		acFormatted := RenderMarkdown(acRaw, innerWidth)
		if strings.TrimSpace(acFormatted) != "" {
			acBox := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSubtle).
				Padding(1, 2).
				Width(cardWidth).
				Render(acFormatted)
			acSection = "\n\n" + acHeader + "\n" + acBox
		}
	}

	// 4. Comments Section
	commentsHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render(fmt.Sprintf("Comments & Discussion (%d)", len(s.Comments)))
	var commentBlocks []string

	commentInnerWidth := cardWidth - 8
	if commentInnerWidth < 20 {
		commentInnerWidth = 20
	}

	if len(s.Comments) == 0 {
		emptyComments := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("No comments yet. Press [c] to add a comment.")
		commentBlocks = append(commentBlocks, emptyComments)
	} else {
		for _, c := range s.Comments {
			author := c.AuthorName
			if author == "" {
				author = c.AuthorEmail
			}
			dateStr := c.CreatedDate.Format("2006-01-02 15:04")

			cHeader := fmt.Sprintf("%s %s",
				lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(author),
				lipgloss.NewStyle().Foreground(ColorMuted).Render("• "+dateStr),
			)
			cRaw := StripAndFormatHTML(c.Text)
			var cBody string
			if cRaw != "" {
				cBody = RenderMarkdown(cRaw, commentInnerWidth)
			}
			if strings.TrimSpace(cBody) == "" {
				cBody = lipgloss.NewStyle().Foreground(ColorMuted).Render("(empty comment)")
			}

			cBox := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(ColorPrimary).
				Padding(0, 1).
				MarginBottom(1).
				Render(cHeader + "\n" + cBody)

			commentBlocks = append(commentBlocks, cBox)
		}
	}

	commentsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(1, 2).
		Width(cardWidth).
		Render(strings.Join(commentBlocks, "\n"))

	// 5. Pull Requests & Linked Resources Section (at bottom below comments)
	var prSection string
	if len(s.RelatedLinks) > 0 {
		prHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render(
			fmt.Sprintf("🔗 Pull Requests & Linked Resources (%d)", len(s.RelatedLinks)),
		)
		var linkCards []string
		for i, link := range s.RelatedLinks {
			isSelected := i == s.SelectedLinkIdx

			prColor := ColorYellow
			if link.IsPR {
				if strings.EqualFold(link.State, "completed") {
					prColor = ColorGreen
				} else if strings.EqualFold(link.State, "abandoned") {
					prColor = ColorRed
				}
			} else if link.Type == "Attachment" {
				prColor = ColorPurple
			} else if link.Type == "Work Item" {
				prColor = ColorSecondary
			} else {
				prColor = ColorCyan
			}

			var prefix string
			var badge string
			var stateBadge string
			var titleStyle lipgloss.Style
			var subStyle lipgloss.Style

			badgeText := fmt.Sprintf("PR #%d", link.ID)
			if !link.IsPR {
				if link.Type == "Attachment" {
					badgeText = "ATTACHMENT"
				} else if link.Type == "Work Item" || link.ID > 0 {
					badgeText = fmt.Sprintf("#%d", link.ID)
				} else {
					badgeText = "LINK"
				}
			}

			if isSelected {
				prefix = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
				badge = lipgloss.NewStyle().
					Background(prColor).
					Foreground(lipgloss.Color("#000000")).
					Padding(0, 1).
					Bold(true).
					Render(badgeText)
				if link.State != "" {
					stateBadge = lipgloss.NewStyle().
						Foreground(prColor).
						Bold(true).
						Render(fmt.Sprintf("[%s]", strings.ToUpper(link.State)))
				}
				titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
				subStyle = lipgloss.NewStyle().Foreground(ColorSecondary)
			} else {
				prefix = "  "
				badge = lipgloss.NewStyle().
					Foreground(ColorMuted).
					Bold(true).
					Render(fmt.Sprintf("[%s]", badgeText))
				if link.State != "" {
					stateBadge = lipgloss.NewStyle().
						Foreground(ColorMuted).
						Render(fmt.Sprintf("[%s]", strings.ToLower(link.State)))
				}
				titleStyle = lipgloss.NewStyle().Foreground(ColorMuted)
				subStyle = lipgloss.NewStyle().Foreground(ColorMuted)
			}

			titleStr := titleStyle.Render(link.Title)
			var branchInfo string
			if link.Subtitle != "" && link.Subtitle != link.Title {
				availWidth := cardWidth - 10 - runewidth.StringWidth(StripEscapes(badgeText)) - runewidth.StringWidth(StripEscapes(link.Title))
				if stateBadge != "" {
					availWidth -= runewidth.StringWidth(StripEscapes(stateBadge)) + 1
				}
				if availWidth > 12 {
					subText := truncateString(link.Subtitle, availWidth)
					branchInfo = " " + subStyle.Render("("+subText+")")
				}
			}

			var contentLine string
			if stateBadge != "" {
				contentLine = fmt.Sprintf("%s%s %s %s%s", prefix, badge, stateBadge, titleStr, branchInfo)
			} else {
				contentLine = fmt.Sprintf("%s%s %s%s", prefix, badge, titleStr, branchInfo)
			}
			linkCards = append(linkCards, contentLine)
		}

		var currentItemIDStr string
		if s.Item != nil {
			currentItemIDStr = fmt.Sprintf(" (#%d)", s.Item.ID)
		}

		var hintText string
		if len(s.RelatedLinks) > 1 {
			hintText = fmt.Sprintf("[Tab / Shift+Tab] Cycle links  •  [p / Enter] Open selected link  •  [o] Open Current Item%s", currentItemIDStr)
		} else {
			hintText = fmt.Sprintf("[p / Enter] Open link in browser  •  [o] Open Current Item%s", currentItemIDStr)
		}

		hint := lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1).
			Render(hintText)

		prBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Padding(1, 2).
			Width(cardWidth).
			Render(strings.Join(linkCards, "\n") + "\n" + hint)

		prSection = "\n\n" + prHeader + "\n" + prBox
	}

	// Assemble all sections
	return lipgloss.JoinVertical(lipgloss.Left,
		metaBox,
		"",
		descHeader,
		descBox,
		acSection,
		"",
		commentsHeader,
		commentsBox,
		prSection,
		"\n",
	)
}

func (s *ScreenItem) View(width, height int, toastMsg string, isError bool) string {
	headerHeight := 1
	footerHeight := 1
	vpHeight := height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	vpWidth := width - 2
	if vpWidth < 10 {
		vpWidth = 10
	}

	if !s.Ready || s.Viewport.Width != vpWidth || s.Viewport.Height != vpHeight {
		s.InitViewport(width, height)
	} else if s.dirty || s.lastRenderedWidth != width || s.lastIncompleteView != s.IncompleteView || s.lastSelectedLink != s.SelectedLinkIdx {
		s.updateViewportContent()
	}

	if s.Item == nil {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, "No work item selected.")
	}

	// 1. Header Bar
	typeBadge := RenderTypeBadge(s.Item.WorkItemType)
	stateBadge := RenderStateBadge(s.Item.State)
	idText := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("#%d", s.Item.ID))
	
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorFg)
	if strings.EqualFold(s.Item.State, "Removed") {
		titleStyle = titleStyle.Strikethrough(true).Foreground(ColorMuted)
	}
	titleText := titleStyle.Render(s.Item.Title)

	backHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Esc] Back")

	headerLeft := lipgloss.JoinHorizontal(lipgloss.Center, typeBadge, " ", stateBadge, " ", idText, " ", titleText)
	gap := width - lipgloss.Width(headerLeft) - lipgloss.Width(backHint) - 2
	if gap < 1 {
		gap = 1
	}
	headerBar := headerLeft + strings.Repeat(" ", gap) + backHint

	// 2. Viewport Content
	viewportContent := s.Viewport.View()

	// 3. Bottom Ribbon
	shortcuts := []Shortcut{
		{Key: "g", Label: "Gemini"},
		{Key: "e", Label: "Edit"},
		{Key: "c", Label: "Comment"},
		{Key: "C", Label: "Copy"},
	}

	if len(s.RelatedLinks) > 0 {
		shortcuts = append(shortcuts,
			Shortcut{Key: "p", Label: "Open Link/PR"},
		)
	}

	shortcuts = append(shortcuts,
		Shortcut{Key: "o", Label: "Open Current Item"},
		Shortcut{Key: "w", Label: "Est"},
		Shortcut{Key: "v", Label: "Activity"},
		Shortcut{Key: "s", Label: "Status"},
		Shortcut{Key: "d", Label: "Dates"},
		Shortcut{Key: "a", Label: "Assign"},
		Shortcut{Key: "D", Label: "Del"},
		Shortcut{Key: "r", Label: "Sync"},
		Shortcut{Key: "Esc", Label: "Back"},
	)

	ribbon := RenderBottomRibbon(width, "", shortcuts, toastMsg, isError)

	return lipgloss.JoinVertical(lipgloss.Left,
		headerBar,
		viewportContent,
		ribbon,
	)
}

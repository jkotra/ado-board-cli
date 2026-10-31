package tui

import (
	"fmt"
	"strings"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScreenCreate represents Screen 3 (Simplified Create Item Form)
type ScreenCreate struct {
	AvailableTypes []string
	TypeIndex      int

	ParentRecord *db.WorkItemRecord
	DB           *db.DB

	// Background inherited metadata (editable later from item screen)
	parentID      int
	assignedTo    string
	iterationPath string
	areaPath      string
	tags          string

	// UI Input components
	InputTitle   textinput.Model
	TextareaDesc textarea.Model

	FocusedField int
	Viewport     viewport.Model
	Ready        bool
	Width        int
	Height       int
}

const (
	fieldCreateType = iota // 0
	fieldCreateTitle       // 1
	fieldCreateDesc        // 2
	fieldCreateSubmit      // 3
	fieldCreateTotal       // 4
)

func setupCreateInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 250
	ti.Width = width
	ti.Prompt = ""
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	return ti
}

func NewScreenCreate() *ScreenCreate {
	types := []string{"Task", "Bug", "Production Issue", "Product Backlog Item", "User Story", "Technical Enablement", "Feature"}

	inTitle := setupCreateInput("Item title (required)...", 60)

	taDesc := textarea.New()
	taDesc.Placeholder = "Detailed description, steps to reproduce, or acceptance criteria..."
	taDesc.CharLimit = 10000
	taDesc.SetWidth(60)
	taDesc.SetHeight(10)
	taDesc.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	taDesc.FocusedStyle.Text = lipgloss.NewStyle().Foreground(ColorFg)
	taDesc.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
	taDesc.FocusedStyle.CursorLine = lipgloss.NewStyle()
	taDesc.ShowLineNumbers = false

	sc := &ScreenCreate{
		AvailableTypes: types,
		TypeIndex:      0,
		InputTitle:     inTitle,
		TextareaDesc:   taDesc,
		FocusedField:   fieldCreateTitle,
	}

	sc.updateFocus()
	return sc
}

func (s *ScreenCreate) InitViewport(width, height int) {
	headerHeight := 2
	ribbonHeight := 1
	vpHeight := height - headerHeight - ribbonHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	vpWidth := width
	if vpWidth < 10 {
		vpWidth = 10
	}
	s.Viewport = viewport.New(vpWidth, vpHeight)
	s.Ready = true
	s.Width = width
	s.Height = height
	s.scrollToFocused()
}

func (s *ScreenCreate) Reset(parent *db.WorkItemRecord, defaultAssignee string, database *db.DB) {
	s.ParentRecord = parent
	s.DB = database
	s.TypeIndex = 0
	s.assignedTo = defaultAssignee
	s.tags = ""

	if parent != nil {
		s.parentID = parent.ID
		s.iterationPath = parent.IterationPath
		s.areaPath = parent.AreaPath
		var tagRegex string
		if database != nil {
			if iter := database.GetNearestParentIteration(parent.ID); iter != "" {
				s.iterationPath = iter
			}
			if area := database.GetNearestParentArea(parent.ID); area != "" {
				s.areaPath = area
			}
			if tr, err := database.GetSetting("tag_regex"); err == nil {
				tagRegex = tr
			}
		}
		s.tags = FilterInheritedTags(parent.Tags, tagRegex)
		// Intelligent default child type selection
		switch strings.ToLower(parent.WorkItemType) {
		case "epic":
			s.TypeIndex = 5 // Feature
		case "feature":
			s.TypeIndex = 2 // Product Backlog Item
		default:
			s.TypeIndex = 0 // Task
		}
	} else {
		s.parentID = 0
		s.iterationPath = ""
		s.areaPath = ""
	}

	s.InputTitle.Reset()
	s.TextareaDesc.Reset()
	s.FocusedField = fieldCreateTitle
	s.updateFocus()
	if s.Ready {
		s.Viewport.GotoTop()
	}
}

func (s *ScreenCreate) SelectedType() string {
	if s.TypeIndex >= 0 && s.TypeIndex < len(s.AvailableTypes) {
		return s.AvailableTypes[s.TypeIndex]
	}
	return "Task"
}

func (s *ScreenCreate) ParentID() int {
	return s.parentID
}

func (s *ScreenCreate) getEffectiveParent() (id int, title, itemType, state, area, iteration string) {
	pID := s.ParentID()
	if pID <= 0 {
		return 0, "", "", "", "", ""
	}
	var iter, a string
	if s.DB != nil {
		iter = s.DB.GetNearestParentIteration(pID)
		a = s.DB.GetNearestParentArea(pID)
	}
	if s.ParentRecord != nil && s.ParentRecord.ID == pID {
		if iter == "" {
			iter = s.ParentRecord.IterationPath
		}
		if a == "" {
			a = s.ParentRecord.AreaPath
		}
		return pID, s.ParentRecord.Title, s.ParentRecord.WorkItemType, s.ParentRecord.State, a, iter
	}
	if s.DB != nil {
		if rec, err := s.DB.GetWorkItem(pID); err == nil && rec != nil {
			if iter == "" {
				iter = rec.IterationPath
			}
			if a == "" {
				a = rec.AreaPath
			}
			return pID, rec.Title, rec.WorkItemType, rec.State, a, iter
		}
	}
	return pID, "", "", "", a, iter
}

func (s *ScreenCreate) Title() string {
	return strings.TrimSpace(s.InputTitle.Value())
}

func (s *ScreenCreate) AssignedTo() string {
	return strings.TrimSpace(s.assignedTo)
}

func (s *ScreenCreate) Tags() string {
	return strings.TrimSpace(s.tags)
}

func (s *ScreenCreate) Iteration() string {
	return strings.TrimSpace(s.iterationPath)
}

func (s *ScreenCreate) Area() string {
	return strings.TrimSpace(s.areaPath)
}

func (s *ScreenCreate) Estimate() float64 {
	return 0
}

func (s *ScreenCreate) RemainingWork() float64 {
	return 0
}

func (s *ScreenCreate) StartDate() string {
	return ""
}

func (s *ScreenCreate) TargetDate() string {
	return ""
}

func (s *ScreenCreate) FinishDate() string {
	return ""
}

func (s *ScreenCreate) Description() string {
	return strings.TrimSpace(s.TextareaDesc.Value())
}

func (s *ScreenCreate) updateFocus() {
	s.InputTitle.Blur()
	s.TextareaDesc.Blur()

	switch s.FocusedField {
	case fieldCreateTitle:
		s.InputTitle.Focus()
	case fieldCreateDesc:
		s.TextareaDesc.Focus()
	}

	s.scrollToFocused()
}

func (s *ScreenCreate) scrollToFocused() {
	if !s.Ready || s.Viewport.Height <= 0 {
		return
	}

	var targetTop, targetBottom int
	switch s.FocusedField {
	case fieldCreateType:
		targetTop = 0
		targetBottom = 7
	case fieldCreateTitle:
		targetTop = 5
		targetBottom = 11
	case fieldCreateDesc:
		targetTop = 10
		targetBottom = 10 + s.TextareaDesc.Height() + 4
	case fieldCreateSubmit:
		targetTop = 12 + s.TextareaDesc.Height()
		targetBottom = 60
	}

	if targetTop < s.Viewport.YOffset {
		s.Viewport.SetYOffset(targetTop)
	} else if targetBottom > s.Viewport.YOffset+s.Viewport.Height {
		s.Viewport.SetYOffset(targetBottom - s.Viewport.Height)
	}
	if s.Viewport.YOffset < 0 {
		s.Viewport.SetYOffset(0)
	}
}

func (s *ScreenCreate) Update(msg tea.Msg) (*ScreenCreate, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Type == tea.MouseWheelUp {
			s.Viewport.LineUp(3)
			return s, nil
		} else if msg.Button == tea.MouseButtonWheelDown || msg.Type == tea.MouseWheelDown {
			s.Viewport.LineDown(3)
			return s, nil
		}
		if msg.Action == tea.MouseActionPress || msg.Type == tea.MouseLeft {
			headerHeight := 2
			if msg.Y >= headerHeight {
				contentY := (msg.Y - headerHeight) + s.Viewport.YOffset
				if msg.Y >= s.Height-4 || contentY >= 14+s.TextareaDesc.Height() {
					s.FocusedField = fieldCreateSubmit
				} else if contentY <= 7 {
					s.FocusedField = fieldCreateType
				} else if contentY <= 11 {
					s.FocusedField = fieldCreateTitle
				} else {
					s.FocusedField = fieldCreateDesc
				}
				s.updateFocus()
				return s, nil
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "pgup":
			s.Viewport.HalfViewUp()
			return s, nil
		case "pgdown":
			s.Viewport.HalfViewDown()
			return s, nil
		case "tab":
			s.FocusedField = (s.FocusedField + 1) % fieldCreateTotal
			s.updateFocus()
			return s, nil
		case "shift+tab":
			s.FocusedField = (s.FocusedField - 1 + fieldCreateTotal) % fieldCreateTotal
			s.updateFocus()
			return s, nil
		case "left", "h":
			if s.FocusedField == fieldCreateType {
				s.TypeIndex = (s.TypeIndex - 1 + len(s.AvailableTypes)) % len(s.AvailableTypes)
				return s, nil
			}
		case "right", "l":
			if s.FocusedField == fieldCreateType {
				s.TypeIndex = (s.TypeIndex + 1) % len(s.AvailableTypes)
				return s, nil
			}
		case "enter":
			if s.FocusedField == fieldCreateType {
				s.FocusedField = fieldCreateTitle
				s.updateFocus()
				return s, nil
			}
			if s.FocusedField == fieldCreateTitle {
				s.FocusedField = fieldCreateDesc
				s.updateFocus()
				return s, nil
			}
		case "down":
			if s.FocusedField == fieldCreateType {
				s.FocusedField = fieldCreateTitle
				s.updateFocus()
				return s, nil
			}
			if s.FocusedField == fieldCreateTitle {
				s.FocusedField = fieldCreateDesc
				s.updateFocus()
				return s, nil
			}
		case "up":
			if s.FocusedField == fieldCreateSubmit {
				s.FocusedField = fieldCreateDesc
				s.updateFocus()
				return s, nil
			} else if s.FocusedField == fieldCreateTitle {
				s.FocusedField = fieldCreateType
				s.updateFocus()
				return s, nil
			}
		}
	}

	var cmd tea.Cmd
	switch s.FocusedField {
	case fieldCreateTitle:
		s.InputTitle, cmd = s.InputTitle.Update(msg)
	case fieldCreateDesc:
		s.TextareaDesc, cmd = s.TextareaDesc.Update(msg)
	}

	return s, cmd
}

func (s *ScreenCreate) View(width, height int, toastMsg string, isError bool) string {
	headerHeight := 2
	ribbonHeight := 1
	vpHeight := height - headerHeight - ribbonHeight
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !s.Ready || s.Width != width || s.Height != height {
		s.InitViewport(width, height)
	}

	cardWidth := width - 4
	if cardWidth < 60 {
		cardWidth = 60
	}
	if cardWidth > 115 {
		cardWidth = 115
	}

	// 1. Header & Parent Resolution
	pID, pTitle, pType, pState, pArea, pIter := s.getEffectiveParent()
	headerTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render("CREATE NEW WORK ITEM")

	var subtitleStr string
	if pID > 0 {
		if pTitle != "" {
			subtitleStr = fmt.Sprintf("Creating child under #%d (%s)", pID, truncateString(pTitle, 35))
		} else {
			subtitleStr = fmt.Sprintf("Creating child under #%d", pID)
		}
	} else {
		subtitleStr = "Creating top-level work item (no parent)"
	}
	subtitle := lipgloss.NewStyle().Foreground(ColorSecondary).Render(subtitleStr)
	headerBar := lipgloss.JoinHorizontal(lipgloss.Center, headerTitle, "  ", subtitle)
	separator := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", width-2))

	// Available content width inside the main card with Padding(0, 1) and Border
	contentWidth := cardWidth - 4
	if contentWidth < 46 {
		contentWidth = 46
	}

	fullInnerW := contentWidth - 4
	if fullInnerW < 42 {
		fullInnerW = 42
	}

	// 2. Compact 1-line Target Parent Banner
	var parentBanner string
	if pID > 0 {
		lbl := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("Target Parent: ")
		idBadge := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(fmt.Sprintf("#%d", pID))
		var typeBadge string
		if pType != "" {
			typeBadge = " " + RenderTypeBadge(pType)
		}
		var stateBadge string
		if pState != "" {
			stateBadge = " " + RenderStateBadge(pState)
		}
		var areaIterBadge string
		if pArea != "" || pIter != "" {
			var parts []string
			if pArea != "" {
				parts = append(parts, "Area: "+pArea)
			}
			if pIter != "" {
				parts = append(parts, "Iter: "+pIter)
			}
			areaIterBadge = " " + lipgloss.NewStyle().Foreground(ColorSecondary).Render(fmt.Sprintf("[%s]", strings.Join(parts, " • ")))
		}
		availTitle := fullInnerW - lipgloss.Width(lbl) - lipgloss.Width(idBadge) - lipgloss.Width(typeBadge) - lipgloss.Width(stateBadge) - lipgloss.Width(areaIterBadge) - 4
		if availTitle < 10 {
			availTitle = 10
		}
		var titleStyled string
		if pTitle != "" {
			titleStyled = " " + lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(truncateString(pTitle, availTitle))
		}
		parentContent := lipgloss.JoinHorizontal(lipgloss.Center, lbl, idBadge, typeBadge, titleStyled, stateBadge, areaIterBadge)
		parentBanner = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary).
			Padding(0, 1).
			Width(fullInnerW).
			Render(parentContent)
	} else {
		parentContent := lipgloss.NewStyle().Foreground(ColorMuted).Render("Target Parent: None (Item will be created as a top-level root work item)")
		parentBanner = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Padding(0, 1).
			Width(fullInnerW).
			Render(parentContent)
	}

	// 3. Work Item Type Selector (Clean Buttons with representative colors)
	var typePills []string
	for i, t := range s.AvailableTypes {
		typeName := FormatItemType(t)
		if i == s.TypeIndex {
			pill := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(GetTypeColor(t)).
				Padding(0, 1).
				Render(typeName)
			typePills = append(typePills, pill)
		} else {
			pill := lipgloss.NewStyle().
				Foreground(ColorMuted).
				Background(lipgloss.Color("#22272e")).
				Padding(0, 1).
				Render(typeName)
			typePills = append(typePills, pill)
		}
	}
	typeRow := strings.Join(typePills, " ")

	lblStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary)
	activeLblStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)

	fieldBorderColor := func(fieldIdx int) lipgloss.Color {
		if s.FocusedField == fieldIdx {
			return ColorYellow
		}
		return ColorSubtle
	}

	getTypeLabel := func() string {
		if s.FocusedField == fieldCreateType {
			return activeLblStyle.Render("Work Item Type (← / → to cycle):")
		}
		return lblStyle.Render("Work Item Type (← / → to cycle):")
	}

	// Full-width field wrapper
	wrapFullInput := func(label string, fieldIdx int, view string) string {
		var lbl string
		if s.FocusedField == fieldIdx {
			lbl = activeLblStyle.Render(label)
		} else {
			lbl = lblStyle.Render(label)
		}
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(fieldBorderColor(fieldIdx)).
			Padding(0, 1).
			Width(fullInnerW).
			Render(view)
		return lipgloss.JoinVertical(lipgloss.Left, lbl, box)
	}

	// Title Input
	s.InputTitle.Width = fullInnerW - 2
	fieldTitle := wrapFullInput("Title (required):", fieldCreateTitle, s.InputTitle.View())

	// Dynamic Description Area calculation (compact and clean, keeping submit button well within view)
	fixedLines := 22
	descHeight := vpHeight - fixedLines
	if descHeight < 6 {
		descHeight = 6
	}
	if descHeight > 10 {
		descHeight = 10
	}
	s.TextareaDesc.SetWidth(fullInnerW - 2)
	s.TextareaDesc.SetHeight(descHeight)

	fieldDesc := wrapFullInput("Description / Notes (Markdown):", fieldCreateDesc, s.TextareaDesc.View())

	// Submit Button styling (representative colors only, no symbols)
	submitBtnStyle := lipgloss.NewStyle().Padding(0, 4).Bold(true)
	if s.FocusedField == fieldCreateSubmit {
		submitBtnStyle = submitBtnStyle.Background(ColorGreen).Foreground(lipgloss.Color("#000000"))
	} else {
		submitBtnStyle = submitBtnStyle.Background(ColorSubtle).Foreground(ColorFg)
	}
	submitBtn := submitBtnStyle.Render("[ Create Work Item (Enter / Ctrl+S) ]")

	formContent := lipgloss.JoinVertical(lipgloss.Left,
		parentBanner,
		"",
		getTypeLabel(),
		"  "+typeRow,
		"",
		fieldTitle,
		"",
		fieldDesc,
		"",
		"  "+submitBtn,
	)

	formCard := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(0, 1).
		Width(cardWidth).
		Render(formContent)

	cardHeight := lipgloss.Height(formCard)

	// Horizontal centering
	var centeredContent string
	indent := 0
	if width > cardWidth {
		indent = (width - cardWidth) / 2
	}
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(formCard, "\n")
	var indentedLines []string
	for _, line := range lines {
		indentedLines = append(indentedLines, prefix+line)
	}
	centeredContent = strings.Join(indentedLines, "\n")

	// If card fits vertically inside viewport without scrolling, center it vertically
	if cardHeight <= vpHeight {
		topPad := (vpHeight - cardHeight) / 2
		if topPad > 0 {
			centeredContent = strings.Repeat("\n", topPad) + centeredContent
		}
		s.Viewport.SetContent(centeredContent)
		s.Viewport.SetYOffset(0)
	} else {
		s.Viewport.SetContent(centeredContent)
		s.scrollToFocused()
	}
	viewportView := s.Viewport.View()

	// 4. Bottom Ribbon
	shortcuts := []Shortcut{
		{Key: "Tab", Label: "Next Field"},
		{Key: "Shift+Tab", Label: "Prev Field"},
		{Key: "←/→", Label: "Select Type"},
		{Key: "Ctrl+S", Label: "Create Item"},
		{Key: "Esc", Label: "Cancel"},
	}

	ribbon := RenderBottomRibbon(width, "", shortcuts, toastMsg, isError)

	return lipgloss.JoinVertical(lipgloss.Left,
		headerBar,
		separator,
		viewportView,
		ribbon,
	)
}

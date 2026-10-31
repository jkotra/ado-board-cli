package tui

import (
	"fmt"
	"strconv"
	"strings"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	fieldSettingsPat = iota
	fieldSettingsGeminiKey
	fieldSettingsGeminiModel
	fieldSettingsAutoCalc
	fieldSettingsEffortSame
	fieldSettingsAutoSetFinishDate
	fieldSettingsAutoSyncInterval
	// Incomplete Criteria Checkboxes
	fieldSettingsIncompleteEffort
	fieldSettingsIncompleteEstimate
	fieldSettingsIncompleteStartDate
	fieldSettingsIncompleteTargetDate
	fieldSettingsIncompleteFinishDate
	fieldSettingsIncompleteTagRegex
	fieldSettingsIncompleteActivity
	// Tag Regex Input
	fieldSettingsTagRegex
	// Incomplete Types & Statuses Multi-Select Rows
	fieldSettingsIncompleteTypes
	fieldSettingsIncompleteStatuses
	fieldSettingsIncompleteDateStatuses
	// Action buttons
	fieldSettingsSaveBtn
	fieldSettingsCancelBtn
	fieldSettingsTotal
)

// ValidationStatus represents the credential validation state
type ValidationStatus int

const (
	ValidationStatusNone ValidationStatus = iota
	ValidationStatusChecking
	ValidationStatusValid
	ValidationStatusInvalid
	ValidationStatusNotConfigured
)

// ScreenSettings represents the Settings Screen (Screen 4)
type ScreenSettings struct {
	InputPat                     textinput.Model
	InputGeminiKey               textinput.Model
	InputGeminiModel             textinput.Model
	AutoCalcRemaining            bool
	EffortSameAsEstimate         bool
	AutoSetFinishDate            bool
	InputAutoSyncInterval        textinput.Model
	IncompleteCriteriaEffort     bool
	IncompleteCriteriaEstimate   bool
	IncompleteCriteriaStartDate  bool
	IncompleteCriteriaTargetDate bool
	IncompleteCriteriaFinishDate bool
	IncompleteCriteriaTagRegex   bool
	IncompleteCriteriaActivity   bool
	InputTagRegex                textinput.Model
	IncompleteTypes              []string
	TypeOptions                  []string
	TypePicker                   *FilterPicker
	IncompleteStatuses           []string
	StatusOptions                []string
	StatusPicker                 *FilterPicker
	IncompleteDateStatuses       []string
	DateStatusPicker             *FilterPicker
	FocusedIndex                 int
	Viewport                     viewport.Model
	Ready                        bool
	Width                        int
	Height                       int
	DB                           *db.DB
	Config                       *config.Config

	PatStatus    ValidationStatus
	PatError     string
	GeminiStatus ValidationStatus
	GeminiError  string
}

func setupSettingsInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 250
	ti.Width = width
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	ti.EchoMode = textinput.EchoNormal
	return ti
}

func NewScreenSettings() *ScreenSettings {
	return &ScreenSettings{
		InputPat:                     setupSettingsInput("Enter Azure DevOps Personal Access Token", 50),
		InputGeminiKey:               setupSettingsInput("Enter Gemini API Key (AI features)", 50),
		InputGeminiModel:             setupSettingsInput("gemini-3.8-flash", 30),
		AutoCalcRemaining:            true,
		EffortSameAsEstimate:         false,
		AutoSetFinishDate:            true,
		InputAutoSyncInterval:        setupSettingsInput("120 (0 to disable)", 30),
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteCriteriaFinishDate: false,
		IncompleteCriteriaTagRegex:   false,
		IncompleteCriteriaActivity:   false,
		InputTagRegex:                setupSettingsInput("e.g. ^BE-.*, ^FE-.*, ^release-.*", 50),
		IncompleteTypes:              []string{"Task", "Bug"},
		TypeOptions:                  []string{"[All]", "Task", "Bug", "Product Backlog Item", "User Story", "Feature", "Epic", "Issue"},
		TypePicker:                   NewFilterPicker(),
		IncompleteStatuses:           []string{"New", "Active", "To Do", "In Progress", "Committed", "Resolved", "Done", "Closed"},
		StatusOptions:                []string{"[All]", "New", "Active", "To Do", "In Progress", "Committed", "Approved", "Resolved", "Done", "Closed", "Removed"},
		StatusPicker:                 NewFilterPicker(),
		IncompleteDateStatuses:       []string{"In Progress", "Active", "Committed", "Resolved", "Done", "Closed"},
		DateStatusPicker:             NewFilterPicker(),
		FocusedIndex:                 fieldSettingsPat,
	}
}

func (s *ScreenSettings) InitViewport(width, height int) {
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

func (s *ScreenSettings) Reset(cfg *config.Config, database *db.DB) {
	s.Config = cfg
	s.DB = database

	pat := ""
	geminiKey := ""
	geminiModel := config.DefaultGeminiModel
	autoCalc := true
	effortSame := false
	autoSetFinishDate := true
	autoSyncInterval := 120
	critEffort := true
	critEstimate := true
	critStartDate := true
	critTargetDate := true
	critFinishDate := false
	critTagRegex := false
	critActivity := false
	tagRegex := ""
	incTypes := []string{"Task", "Bug"}
	incStatuses := []string{"New", "Active", "To Do", "In Progress", "Committed", "Resolved", "Done", "Closed"}
	incDateStatuses := []string{"In Progress", "Active", "Committed", "Resolved", "Done", "Closed"}

	if cfg != nil {
		pat = cleanSingleLineInput(cfg.PatToken)
		geminiKey = cleanSingleLineInput(cfg.GeminiApiKey)
		if cfg.GeminiModel != "" {
			geminiModel = cleanSingleLineInput(cfg.GeminiModel)
		}
		autoCalc = cfg.AutoCalcRemaining
		effortSame = cfg.EffortSameAsEstimate
		autoSetFinishDate = cfg.AutoSetFinishDate
		if cfg.AutoSyncIntervalSeconds >= 0 {
			autoSyncInterval = cfg.AutoSyncIntervalSeconds
		}
		critEffort = cfg.IncompleteCriteriaEffort
		critEstimate = cfg.IncompleteCriteriaEstimate
		critStartDate = cfg.IncompleteCriteriaStartDate
		critTargetDate = cfg.IncompleteCriteriaTargetDate
		critFinishDate = cfg.IncompleteCriteriaFinishDate
		critTagRegex = cfg.IncompleteCriteriaTagRegex
		critActivity = cfg.IncompleteCriteriaActivity
		tagRegex = cleanSingleLineInput(cfg.TagRegex)
		if len(cfg.IncompleteTypes) > 0 {
			incTypes = make([]string, len(cfg.IncompleteTypes))
			copy(incTypes, cfg.IncompleteTypes)
		} else if cfg.IncompleteTypeTasks || cfg.IncompleteTypeBugs || cfg.IncompleteTypePBI {
			incTypes = nil
			if cfg.IncompleteTypeTasks {
				incTypes = append(incTypes, "Task")
			}
			if cfg.IncompleteTypeBugs {
				incTypes = append(incTypes, "Bug")
			}
			if cfg.IncompleteTypePBI {
				incTypes = append(incTypes, "Product Backlog Item", "User Story")
			}
		}
		if len(cfg.IncompleteStatuses) > 0 {
			incStatuses = make([]string, len(cfg.IncompleteStatuses))
			copy(incStatuses, cfg.IncompleteStatuses)
		}
		if len(cfg.IncompleteDateStatuses) > 0 {
			incDateStatuses = make([]string, len(cfg.IncompleteDateStatuses))
			copy(incDateStatuses, cfg.IncompleteDateStatuses)
		}
	}

	if database != nil {
		if val, err := database.GetSetting("pat_token"); err == nil && val != "" {
			pat = cleanSingleLineInput(val)
		}
		if val, err := database.GetSetting("gemini_api_key"); err == nil && val != "" {
			geminiKey = cleanSingleLineInput(val)
		}
		if val, err := database.GetSetting("gemini_model"); err == nil && val != "" {
			geminiModel = cleanSingleLineInput(val)
		}
		if val, err := database.GetSetting("auto_calc_remaining"); err == nil && val != "" {
			autoCalc = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("effort_same_as_estimate"); err == nil && val != "" {
			effortSame = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("auto_set_finish_date"); err == nil && val != "" {
			autoSetFinishDate = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("auto_sync_interval_seconds"); err == nil && val != "" {
			if iv, err := strconv.Atoi(cleanSingleLineInput(val)); err == nil && iv >= 0 {
				autoSyncInterval = iv
			}
		}
		if val, err := database.GetSetting("incomplete_criteria_effort"); err == nil && val != "" {
			critEffort = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_estimate"); err == nil && val != "" {
			critEstimate = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_start_date"); err == nil && val != "" {
			critStartDate = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_target_date"); err == nil && val != "" {
			critTargetDate = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_finish_date"); err == nil && val != "" {
			critFinishDate = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_tag_regex"); err == nil && val != "" {
			critTagRegex = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("incomplete_criteria_activity"); err == nil && val != "" {
			critActivity = (val == "true" || val == "1")
		}
		if val, err := database.GetSetting("tag_regex"); err == nil && val != "" {
			tagRegex = cleanSingleLineInput(val)
		}
		if val, err := database.GetSetting("incomplete_types"); err == nil && val != "" {
			incTypes = nil
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					incTypes = append(incTypes, p)
				}
			}
		}
		if val, err := database.GetSetting("incomplete_statuses"); err == nil && val != "" {
			incStatuses = nil
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					incStatuses = append(incStatuses, p)
				}
			}
		}
		if val, err := database.GetSetting("incomplete_date_statuses"); err == nil && val != "" {
			incDateStatuses = nil
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					incDateStatuses = append(incDateStatuses, p)
				}
			}
		}
	}

	// Build TypeOptions dynamically from DB
	seenTypes := make(map[string]bool)
	s.TypeOptions = []string{"[All]"}
	seenTypes["[all]"] = true

	if database != nil {
		if dbTypes, err := database.GetDistinctWorkItemTypes(); err == nil {
			for _, t := range dbTypes {
				t = strings.TrimSpace(t)
				if t != "" && !seenTypes[strings.ToLower(t)] {
					seenTypes[strings.ToLower(t)] = true
					s.TypeOptions = append(s.TypeOptions, t)
				}
			}
		}
	}

	commonTypes := []string{"Task", "Bug", "Product Backlog Item", "User Story", "Feature", "Epic", "Issue"}
	for _, ct := range commonTypes {
		if !seenTypes[strings.ToLower(ct)] {
			seenTypes[strings.ToLower(ct)] = true
			s.TypeOptions = append(s.TypeOptions, ct)
		}
	}

	// Build StatusOptions dynamically from DB
	seenStatuses := make(map[string]bool)
	s.StatusOptions = []string{"[All]"}
	seenStatuses["[all]"] = true

	if database != nil {
		if dbStates, err := database.GetDistinctStates(); err == nil {
			for _, st := range dbStates {
				st = strings.TrimSpace(st)
				if st != "" && !seenStatuses[strings.ToLower(st)] {
					seenStatuses[strings.ToLower(st)] = true
					s.StatusOptions = append(s.StatusOptions, st)
				}
			}
		}
	}

	commonStatuses := []string{"New", "Active", "To Do", "In Progress", "Committed", "Approved", "Resolved", "Done", "Closed", "Removed"}
	for _, cs := range commonStatuses {
		if !seenStatuses[strings.ToLower(cs)] {
			seenStatuses[strings.ToLower(cs)] = true
			s.StatusOptions = append(s.StatusOptions, cs)
		}
	}

	s.InputPat.SetValue(cleanSingleLineInput(pat))
	s.InputGeminiKey.SetValue(cleanSingleLineInput(geminiKey))
	s.InputGeminiModel.SetValue(cleanSingleLineInput(geminiModel))
	s.AutoCalcRemaining = autoCalc
	s.EffortSameAsEstimate = effortSame
	s.AutoSetFinishDate = autoSetFinishDate
	s.InputAutoSyncInterval.SetValue(fmt.Sprintf("%d", autoSyncInterval))
	s.IncompleteCriteriaEffort = critEffort
	s.IncompleteCriteriaEstimate = critEstimate
	s.IncompleteCriteriaStartDate = critStartDate
	s.IncompleteCriteriaTargetDate = critTargetDate
	s.IncompleteCriteriaFinishDate = critFinishDate
	s.IncompleteCriteriaTagRegex = critTagRegex
	s.IncompleteCriteriaActivity = critActivity
	s.InputTagRegex.SetValue(cleanSingleLineInput(tagRegex))
	s.IncompleteTypes = incTypes
	s.IncompleteStatuses = incStatuses
	s.IncompleteDateStatuses = incDateStatuses

	if s.PatStatus == ValidationStatusNone {
		if cleanSingleLineInput(pat) == "" {
			s.PatStatus = ValidationStatusNotConfigured
		}
	}
	if s.GeminiStatus == ValidationStatusNone {
		if cleanSingleLineInput(geminiKey) == "" {
			s.GeminiStatus = ValidationStatusNotConfigured
		}
	}

	if s.TypePicker == nil {
		s.TypePicker = NewFilterPicker()
	} else {
		s.TypePicker.Close()
	}
	if s.StatusPicker == nil {
		s.StatusPicker = NewFilterPicker()
	} else {
		s.StatusPicker.Close()
	}
	if s.DateStatusPicker == nil {
		s.DateStatusPicker = NewFilterPicker()
	} else {
		s.DateStatusPicker.Close()
	}
	s.FocusedIndex = fieldSettingsPat
	s.updateFocus()
	if s.Ready {
		s.Viewport.GotoTop()
	}
}

func (s *ScreenSettings) updateFocus() {
	s.InputPat.Blur()
	s.InputGeminiKey.Blur()
	s.InputGeminiModel.Blur()
	s.InputAutoSyncInterval.Blur()
	s.InputTagRegex.Blur()

	switch s.FocusedIndex {
	case fieldSettingsPat:
		s.InputPat.Focus()
	case fieldSettingsGeminiKey:
		s.InputGeminiKey.Focus()
	case fieldSettingsGeminiModel:
		s.InputGeminiModel.Focus()
	case fieldSettingsAutoSyncInterval:
		s.InputAutoSyncInterval.Focus()
	case fieldSettingsTagRegex:
		s.InputTagRegex.Focus()
	}

	s.scrollToFocused()
}

func (s *ScreenSettings) scrollToFocused() {
	if !s.Ready || s.Viewport.Height <= 0 {
		return
	}

	var targetTop, targetBottom int
	switch s.FocusedIndex {
	case fieldSettingsPat:
		targetTop = 0
		targetBottom = 10
	case fieldSettingsGeminiKey:
		targetTop = 8
		targetBottom = 17
	case fieldSettingsGeminiModel:
		targetTop = 15
		targetBottom = 24
	case fieldSettingsAutoCalc:
		targetTop = 23
		targetBottom = 31
	case fieldSettingsEffortSame:
		targetTop = 27
		targetBottom = 35
	case fieldSettingsAutoSetFinishDate:
		targetTop = 31
		targetBottom = 39
	case fieldSettingsAutoSyncInterval:
		targetTop = 35
		targetBottom = 45
	case fieldSettingsIncompleteEffort:
		targetTop = 41
		targetBottom = 48
	case fieldSettingsIncompleteEstimate:
		targetTop = 45
		targetBottom = 52
	case fieldSettingsIncompleteStartDate:
		targetTop = 49
		targetBottom = 56
	case fieldSettingsIncompleteTargetDate:
		targetTop = 53
		targetBottom = 60
	case fieldSettingsIncompleteFinishDate:
		targetTop = 57
		targetBottom = 64
	case fieldSettingsIncompleteTagRegex:
		targetTop = 61
		targetBottom = 68
	case fieldSettingsIncompleteActivity:
		targetTop = 65
		targetBottom = 72
	case fieldSettingsTagRegex:
		targetTop = 69
		targetBottom = 78
	case fieldSettingsIncompleteTypes:
		targetTop = 75
		targetBottom = 81
	case fieldSettingsIncompleteStatuses:
		targetTop = 77
		targetBottom = 83
	case fieldSettingsIncompleteDateStatuses:
		targetTop = 79
		targetBottom = 86
	case fieldSettingsSaveBtn, fieldSettingsCancelBtn:
		targetTop = 83
		targetBottom = 90
	}

	if targetTop < s.Viewport.YOffset {
		s.Viewport.SetYOffset(targetTop)
	} else if targetBottom > s.Viewport.YOffset+s.Viewport.Height {
		s.Viewport.SetYOffset(targetBottom - s.Viewport.Height)
	}
}

func cleanSingleLineInput(val string) string {
	val = strings.ReplaceAll(val, "\r\n", "")
	val = strings.ReplaceAll(val, "\r", "")
	val = strings.ReplaceAll(val, "\n", "")
	return strings.TrimSpace(val)
}

func (s *ScreenSettings) Values() (
	pat, geminiKey, geminiModel string,
	autoCalc, effortSame, autoSetFinishDate bool,
	autoSyncInterval int,
	critEffort, critEstimate, critStartDate, critTargetDate, critFinishDate, critTagRegex, critActivity bool,
	tagRegex string,
	incompleteTypes, incompleteStatuses, incompleteDateStatuses []string,
) {
	interval := 120
	if iv, err := strconv.Atoi(cleanSingleLineInput(s.InputAutoSyncInterval.Value())); err == nil && iv >= 0 {
		interval = iv
	}
	return cleanSingleLineInput(s.InputPat.Value()),
		cleanSingleLineInput(s.InputGeminiKey.Value()),
		cleanSingleLineInput(s.InputGeminiModel.Value()),
		s.AutoCalcRemaining,
		s.EffortSameAsEstimate,
		s.AutoSetFinishDate,
		interval,
		s.IncompleteCriteriaEffort,
		s.IncompleteCriteriaEstimate,
		s.IncompleteCriteriaStartDate,
		s.IncompleteCriteriaTargetDate,
		s.IncompleteCriteriaFinishDate,
		s.IncompleteCriteriaTagRegex,
		s.IncompleteCriteriaActivity,
		cleanSingleLineInput(s.InputTagRegex.Value()),
		s.IncompleteTypes,
		s.IncompleteStatuses,
		s.IncompleteDateStatuses
}

type SettingsAction int

const (
	SettingsActionNone SettingsAction = iota
	SettingsActionSave
	SettingsActionCancel
)

func (s *ScreenSettings) Update(msg tea.Msg) (SettingsAction, tea.Cmd) {
	if s.TypePicker != nil && s.TypePicker.Active {
		sel, confirmed, closed, cmd := s.TypePicker.Update(msg)
		if closed {
			if confirmed {
				s.IncompleteTypes = sel
			}
		}
		return SettingsActionNone, cmd
	}
	if s.StatusPicker != nil && s.StatusPicker.Active {
		sel, confirmed, closed, cmd := s.StatusPicker.Update(msg)
		if closed {
			if confirmed {
				s.IncompleteStatuses = sel
			}
		}
		return SettingsActionNone, cmd
	}
	if s.DateStatusPicker != nil && s.DateStatusPicker.Active {
		sel, confirmed, closed, cmd := s.DateStatusPicker.Update(msg)
		if closed {
			if confirmed {
				s.IncompleteDateStatuses = sel
			}
		}
		return SettingsActionNone, cmd
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Type == tea.MouseWheelUp {
			s.Viewport.LineUp(3)
			return SettingsActionNone, nil
		} else if msg.Button == tea.MouseButtonWheelDown || msg.Type == tea.MouseWheelDown {
			s.Viewport.LineDown(3)
			return SettingsActionNone, nil
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.FocusedIndex = (s.FocusedIndex + 1) % fieldSettingsTotal
			s.updateFocus()
			return SettingsActionNone, nil
		case "shift+tab":
			s.FocusedIndex = (s.FocusedIndex - 1 + fieldSettingsTotal) % fieldSettingsTotal
			s.updateFocus()
			return SettingsActionNone, nil
		case "down":
			if s.FocusedIndex == fieldSettingsSaveBtn {
				s.FocusedIndex = fieldSettingsCancelBtn
			} else if s.FocusedIndex < fieldSettingsCancelBtn {
				s.FocusedIndex++
			}
			s.updateFocus()
			return SettingsActionNone, nil
		case "up":
			if s.FocusedIndex == fieldSettingsCancelBtn {
				s.FocusedIndex = fieldSettingsSaveBtn
			} else if s.FocusedIndex > fieldSettingsPat {
				s.FocusedIndex--
			}
			s.updateFocus()
			return SettingsActionNone, nil
		case "right", "l":
			if s.FocusedIndex == fieldSettingsSaveBtn {
				s.FocusedIndex = fieldSettingsCancelBtn
				s.updateFocus()
				return SettingsActionNone, nil
			}
		case "left", "h":
			if s.FocusedIndex == fieldSettingsCancelBtn {
				s.FocusedIndex = fieldSettingsSaveBtn
				s.updateFocus()
				return SettingsActionNone, nil
			}
		case "pgup", "ctrl+u":
			s.Viewport.HalfViewUp()
			return SettingsActionNone, nil
		case "pgdown", "ctrl+d":
			s.Viewport.HalfViewDown()
			return SettingsActionNone, nil
		case " ":
			switch s.FocusedIndex {
			case fieldSettingsAutoCalc:
				s.AutoCalcRemaining = !s.AutoCalcRemaining
				return SettingsActionNone, nil
			case fieldSettingsEffortSame:
				s.EffortSameAsEstimate = !s.EffortSameAsEstimate
				return SettingsActionNone, nil
			case fieldSettingsAutoSetFinishDate:
				s.AutoSetFinishDate = !s.AutoSetFinishDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteEffort:
				s.IncompleteCriteriaEffort = !s.IncompleteCriteriaEffort
				return SettingsActionNone, nil
			case fieldSettingsIncompleteEstimate:
				s.IncompleteCriteriaEstimate = !s.IncompleteCriteriaEstimate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteStartDate:
				s.IncompleteCriteriaStartDate = !s.IncompleteCriteriaStartDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTargetDate:
				s.IncompleteCriteriaTargetDate = !s.IncompleteCriteriaTargetDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteFinishDate:
				s.IncompleteCriteriaFinishDate = !s.IncompleteCriteriaFinishDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTagRegex:
				s.IncompleteCriteriaTagRegex = !s.IncompleteCriteriaTagRegex
				return SettingsActionNone, nil
			case fieldSettingsIncompleteActivity:
				s.IncompleteCriteriaActivity = !s.IncompleteCriteriaActivity
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTypes:
				if s.TypePicker != nil {
					s.TypePicker.Open("Select Applicable Work Item Types", filterFieldType, s.TypeOptions, s.IncompleteTypes, ColorCyan)
				}
				return SettingsActionNone, nil
			case fieldSettingsIncompleteStatuses:
				if s.StatusPicker != nil {
					s.StatusPicker.Open("Select Applicable Item Statuses", filterFieldStatus, s.StatusOptions, s.IncompleteStatuses, ColorSecondary)
				}
				return SettingsActionNone, nil
			case fieldSettingsIncompleteDateStatuses:
				if s.DateStatusPicker != nil {
					s.DateStatusPicker.Open("Select Statuses Requiring Dates", filterFieldStatus, s.StatusOptions, s.IncompleteDateStatuses, ColorSecondary)
				}
				return SettingsActionNone, nil
			case fieldSettingsSaveBtn:
				return SettingsActionSave, nil
			case fieldSettingsCancelBtn:
				return SettingsActionCancel, nil
			}
		case "enter":
			switch s.FocusedIndex {
			case fieldSettingsAutoCalc:
				s.AutoCalcRemaining = !s.AutoCalcRemaining
				return SettingsActionNone, nil
			case fieldSettingsEffortSame:
				s.EffortSameAsEstimate = !s.EffortSameAsEstimate
				return SettingsActionNone, nil
			case fieldSettingsAutoSetFinishDate:
				s.AutoSetFinishDate = !s.AutoSetFinishDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteEffort:
				s.IncompleteCriteriaEffort = !s.IncompleteCriteriaEffort
				return SettingsActionNone, nil
			case fieldSettingsIncompleteEstimate:
				s.IncompleteCriteriaEstimate = !s.IncompleteCriteriaEstimate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteStartDate:
				s.IncompleteCriteriaStartDate = !s.IncompleteCriteriaStartDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTargetDate:
				s.IncompleteCriteriaTargetDate = !s.IncompleteCriteriaTargetDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteFinishDate:
				s.IncompleteCriteriaFinishDate = !s.IncompleteCriteriaFinishDate
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTagRegex:
				s.IncompleteCriteriaTagRegex = !s.IncompleteCriteriaTagRegex
				return SettingsActionNone, nil
			case fieldSettingsIncompleteActivity:
				s.IncompleteCriteriaActivity = !s.IncompleteCriteriaActivity
				return SettingsActionNone, nil
			case fieldSettingsIncompleteTypes:
				if s.TypePicker != nil {
					s.TypePicker.Open("Select Applicable Work Item Types", filterFieldType, s.TypeOptions, s.IncompleteTypes, ColorCyan)
				}
				return SettingsActionNone, nil
			case fieldSettingsIncompleteStatuses:
				if s.StatusPicker != nil {
					s.StatusPicker.Open("Select Applicable Item Statuses", filterFieldStatus, s.StatusOptions, s.IncompleteStatuses, ColorSecondary)
				}
				return SettingsActionNone, nil
			case fieldSettingsIncompleteDateStatuses:
				if s.DateStatusPicker != nil {
					s.DateStatusPicker.Open("Select Statuses Requiring Dates", filterFieldStatus, s.StatusOptions, s.IncompleteDateStatuses, ColorSecondary)
				}
				return SettingsActionNone, nil
			case fieldSettingsSaveBtn:
				return SettingsActionSave, nil
			case fieldSettingsCancelBtn:
				return SettingsActionCancel, nil
			default:
				// On text fields, move to next field
				s.FocusedIndex = (s.FocusedIndex + 1) % fieldSettingsTotal
				s.updateFocus()
				return SettingsActionNone, nil
			}
		case "ctrl+s":
			return SettingsActionSave, nil
		case "esc":
			return SettingsActionCancel, nil
		}
	}

	var cmd tea.Cmd
	switch s.FocusedIndex {
	case fieldSettingsPat:
		s.InputPat, cmd = s.InputPat.Update(msg)
		if strings.ContainsAny(s.InputPat.Value(), "\r\n") {
			s.InputPat.SetValue(cleanSingleLineInput(s.InputPat.Value()))
		}
	case fieldSettingsGeminiKey:
		s.InputGeminiKey, cmd = s.InputGeminiKey.Update(msg)
		if strings.ContainsAny(s.InputGeminiKey.Value(), "\r\n") {
			s.InputGeminiKey.SetValue(cleanSingleLineInput(s.InputGeminiKey.Value()))
		}
	case fieldSettingsGeminiModel:
		s.InputGeminiModel, cmd = s.InputGeminiModel.Update(msg)
		if strings.ContainsAny(s.InputGeminiModel.Value(), "\r\n") {
			s.InputGeminiModel.SetValue(cleanSingleLineInput(s.InputGeminiModel.Value()))
		}
	case fieldSettingsAutoSyncInterval:
		s.InputAutoSyncInterval, cmd = s.InputAutoSyncInterval.Update(msg)
		if strings.ContainsAny(s.InputAutoSyncInterval.Value(), "\r\n") {
			s.InputAutoSyncInterval.SetValue(cleanSingleLineInput(s.InputAutoSyncInterval.Value()))
		}
	case fieldSettingsTagRegex:
		s.InputTagRegex, cmd = s.InputTagRegex.Update(msg)
		if strings.ContainsAny(s.InputTagRegex.Value(), "\r\n") {
			s.InputTagRegex.SetValue(cleanSingleLineInput(s.InputTagRegex.Value()))
		}
	}

	return SettingsActionNone, cmd
}

func (s *ScreenSettings) View(width, height int, toastMsg string, isError bool) string {
	if s.TypePicker != nil && s.TypePicker.Active {
		return s.TypePicker.View(width, height)
	}
	if s.StatusPicker != nil && s.StatusPicker.Active {
		return s.StatusPicker.View(width, height)
	}
	if s.DateStatusPicker != nil && s.DateStatusPicker.Active {
		return s.DateStatusPicker.View(width, height)
	}

	if width < 40 {
		width = 40
	}
	if height < 15 {
		height = 15
	}

	headerHeight := 2 // headerBar (1) + separator (1)
	ribbonHeight := 1
	vpHeight := height - headerHeight - ribbonHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	vpWidth := width
	if vpWidth < 10 {
		vpWidth = 10
	}

	if !s.Ready || s.Viewport.Width != vpWidth || s.Viewport.Height != vpHeight {
		s.InitViewport(width, height)
	}

	cardWidth := 74
	if width > 0 && width < 78 {
		cardWidth = width - 4
	}
	if cardWidth < 46 {
		cardWidth = 46
	}
	contentWidth := cardWidth - 4

	// 1. Top Header Bar
	headerTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("Application Settings")

	subtitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(fmt.Sprintf("Configure Azure DevOps & AI preferences (Storage: %s)", config.GetAppDir()))

	escHint := lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Render("[Esc] Cancel")

	availHeaderSpace := width - lipgloss.Width(headerTitle) - lipgloss.Width(subtitle) - lipgloss.Width(escHint) - 4
	if availHeaderSpace < 1 {
		availHeaderSpace = 1
	}
	headerBar := lipgloss.JoinHorizontal(lipgloss.Center,
		headerTitle,
		"  ",
		subtitle,
		strings.Repeat(" ", availHeaderSpace),
		escHint,
	)

	separator := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", width-2))

	// Styling helpers
	lblStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary)
	activeLblStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	hintStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	fieldBorderColor := func(fieldIdx int) lipgloss.Color {
		if s.FocusedIndex == fieldIdx {
			return ColorSecondary
		}
		return ColorSubtle
	}

	sWidth := contentWidth - 4
	if sWidth < 20 {
		sWidth = 20
	}
	inWidth := sWidth - 4
	if inWidth < 10 {
		inWidth = 10
	}

	// Full-width text input wrapper
	wrapInputBox := func(label string, fieldIdx int, view string, hint string, status ValidationStatus, statusErr string) string {
		var lbl string
		if s.FocusedIndex == fieldIdx {
			lbl = activeLblStyle.Render("▶ " + label)
		} else {
			lbl = lblStyle.Render("  " + label)
		}

		var statusBadge string
		switch status {
		case ValidationStatusChecking:
			statusBadge = lipgloss.NewStyle().Foreground(ColorYellow).Render("[⟳ Checking...]")
		case ValidationStatusValid:
			statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[✓ Valid]")
		case ValidationStatusInvalid:
			statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("[✗ Invalid]")
		case ValidationStatusNotConfigured:
			statusBadge = lipgloss.NewStyle().Foreground(ColorMuted).Render("[○ Not Configured]")
		}

		var labelLine string
		if statusBadge != "" {
			labelLine = lipgloss.JoinHorizontal(lipgloss.Center, lbl, "  ", statusBadge)
		} else {
			labelLine = lbl
		}

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(fieldBorderColor(fieldIdx)).
			Padding(0, 1).
			Width(sWidth).
			Render(view)

		items := []string{labelLine, box}
		if status == ValidationStatusInvalid && statusErr != "" {
			items = append(items, "  "+lipgloss.NewStyle().Foreground(ColorRed).Render("✗ "+statusErr))
		}
		if hint != "" {
			items = append(items, "  "+hintStyle.Render(hint))
		}
		return lipgloss.JoinVertical(lipgloss.Left, items...)
	}

	// Synchronize input widths and clean any newlines
	s.InputPat.Width = inWidth
	s.InputGeminiKey.Width = inWidth
	s.InputGeminiModel.Width = inWidth
	s.InputAutoSyncInterval.Width = inWidth
	s.InputTagRegex.Width = inWidth

	if strings.ContainsAny(s.InputPat.Value(), "\r\n") {
		s.InputPat.SetValue(cleanSingleLineInput(s.InputPat.Value()))
	}
	if strings.ContainsAny(s.InputGeminiKey.Value(), "\r\n") {
		s.InputGeminiKey.SetValue(cleanSingleLineInput(s.InputGeminiKey.Value()))
	}
	if strings.ContainsAny(s.InputGeminiModel.Value(), "\r\n") {
		s.InputGeminiModel.SetValue(cleanSingleLineInput(s.InputGeminiModel.Value()))
	}
	if strings.ContainsAny(s.InputAutoSyncInterval.Value(), "\r\n") {
		s.InputAutoSyncInterval.SetValue(cleanSingleLineInput(s.InputAutoSyncInterval.Value()))
	}
	if strings.ContainsAny(s.InputTagRegex.Value(), "\r\n") {
		s.InputTagRegex.SetValue(cleanSingleLineInput(s.InputTagRegex.Value()))
	}

	fieldPat := wrapInputBox("Azure DevOps PAT Token:", fieldSettingsPat, s.InputPat.View(), "Personal Access Token for Azure DevOps REST API authentication", s.PatStatus, s.PatError)
	fieldGeminiKey := wrapInputBox("Gemini API Key:", fieldSettingsGeminiKey, s.InputGeminiKey.View(), "Required for Gemini AI Chat Dock and intelligent assistance", s.GeminiStatus, s.GeminiError)
	fieldGeminiModel := wrapInputBox("Gemini Model:", fieldSettingsGeminiModel, s.InputGeminiModel.View(), "Default: gemini-3.8-flash (e.g. gemini-2.5-pro, gemini-3.8-flash)", ValidationStatusNone, "")
	fieldAutoSync := wrapInputBox("Auto-Sync Interval (seconds):", fieldSettingsAutoSyncInterval, s.InputAutoSyncInterval.View(), "Background sync interval in seconds (default: 120; 0 to disable)", ValidationStatusNone, "")
	fieldTagRegex := wrapInputBox("Tag Inheritance & Validation Regex Rules:", fieldSettingsTagRegex, s.InputTagRegex.View(), "Defines regex rules (separated by commas or semicolons) for child tag inheritance & incomplete validation (e.g. ^BE-.*, ^FE-.*)", ValidationStatusNone, "")

	// Checkboxes rendering
	renderCheckbox := func(label string, checked bool, fieldIdx int, hint string) string {
		checkMark := "[ ]"
		if checked {
			checkMark = "[✓]"
		}

		checkStyle := lipgloss.NewStyle().Bold(true)
		if checked {
			checkStyle = checkStyle.Foreground(ColorGreen)
		} else {
			checkStyle = checkStyle.Foreground(ColorMuted)
		}

		var line string
		if s.FocusedIndex == fieldIdx {
			line = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("▶ ") +
				checkStyle.Render(checkMark) + " " +
				lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(label)
		} else {
			line = "  " + checkStyle.Render(checkMark) + " " +
				lipgloss.NewStyle().Foreground(ColorFg).Render(label)
		}

		items := []string{line}
		if hint != "" {
			items = append(items, "      "+hintStyle.Render(hint))
		}
		return lipgloss.JoinVertical(lipgloss.Left, items...)
	}

	cbAutoCalc := renderCheckbox("Compute Remaining Work Automatically", s.AutoCalcRemaining, fieldSettingsAutoCalc, "Auto-calculates Remaining Work = Original Estimate - Completed Work")
	cbEffortSame := renderCheckbox("Effort same as Estimate", s.EffortSameAsEstimate, fieldSettingsEffortSame, "Sets Effort to match Estimate unless a custom Effort is entered")
	cbAutoSetFinishDate := renderCheckbox("Automatically Set Finish Date on Done", s.AutoSetFinishDate, fieldSettingsAutoSetFinishDate, "Sets Finish Date to current date/time when changing status to Done / Completed")

	// Incomplete Criteria checkboxes with clear titles and consistent subheadings
	cbCritEffort := renderCheckbox("Require Effort / Story Points", s.IncompleteCriteriaEffort, fieldSettingsIncompleteEffort, "Flags items with missing or zero (0) Effort")
	cbCritEstimate := renderCheckbox("Require Original Estimate", s.IncompleteCriteriaEstimate, fieldSettingsIncompleteEstimate, "Flags items with missing or zero (0) Original Estimate")
	cbCritStartDate := renderCheckbox("Require Start Date", s.IncompleteCriteriaStartDate, fieldSettingsIncompleteStartDate, "Flags items missing a scheduled Start Date")
	cbCritTargetDate := renderCheckbox("Require Target Date (Overdue Detection)", s.IncompleteCriteriaTargetDate, fieldSettingsIncompleteTargetDate, "Flags items missing a Target Date or scheduled past due")
	cbCritFinishDate := renderCheckbox("Require Finish Date", s.IncompleteCriteriaFinishDate, fieldSettingsIncompleteFinishDate, "Flags items missing a recorded Finish Date")
	cbCritTagRegex := renderCheckbox("Require Tag matching Regex", s.IncompleteCriteriaTagRegex, fieldSettingsIncompleteTagRegex, "Flags items if no tag matches configured Tag Regex rules")
	cbCritActivity := renderCheckbox("Require Activity", s.IncompleteCriteriaActivity, fieldSettingsIncompleteActivity, "Flags items missing an Activity assignment (e.g. Development, Testing)")

	// Work Item Types Multi-select row (styled similar to filter modal, without description)
	var typeValRendered string
	if len(s.IncompleteTypes) == 0 || (len(s.IncompleteTypes) == 1 && strings.EqualFold(s.IncompleteTypes[0], "[all]")) {
		if s.FocusedIndex == fieldSettingsIncompleteTypes {
			typeValRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[All]")
		} else {
			typeValRendered = lipgloss.NewStyle().Foreground(ColorMuted).Render("[All]")
		}
	} else {
		if len(s.IncompleteTypes) <= 3 {
			var badges []string
			for _, t := range s.IncompleteTypes {
				badges = append(badges, RenderTypeBadge(t))
			}
			typeValRendered = strings.Join(badges, " ")
		} else {
			var badges []string
			for i := 0; i < 2; i++ {
				badges = append(badges, RenderTypeBadge(s.IncompleteTypes[i]))
			}
			moreCount := len(s.IncompleteTypes) - 2
			moreBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("+%d more (%d)", moreCount, len(s.IncompleteTypes)))
			typeValRendered = strings.Join(badges, " ") + " " + moreBadge
		}
	}

	var typeRow string
	if s.FocusedIndex == fieldSettingsIncompleteTypes {
		typeHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  [Enter: Choose Checkboxes]")
		typeRow = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("▶ ") +
			lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render("Applicable Item Types:     ") +
			typeValRendered + typeHint
	} else {
		typeRow = "  " + lipgloss.NewStyle().Foreground(ColorFg).Render("Applicable Item Types:     ") +
			typeValRendered
	}

	// Work Item Statuses Multi-select row (styled similar to filter modal, without description)
	var statusValRendered string
	if len(s.IncompleteStatuses) == 0 || (len(s.IncompleteStatuses) == 1 && strings.EqualFold(s.IncompleteStatuses[0], "[all]")) {
		if s.FocusedIndex == fieldSettingsIncompleteStatuses {
			statusValRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[All]")
		} else {
			statusValRendered = lipgloss.NewStyle().Foreground(ColorMuted).Render("[All]")
		}
	} else {
		if len(s.IncompleteStatuses) <= 3 {
			var badges []string
			for _, st := range s.IncompleteStatuses {
				badges = append(badges, RenderStateBadge(st))
			}
			statusValRendered = strings.Join(badges, " ")
		} else {
			var badges []string
			for i := 0; i < 2; i++ {
				badges = append(badges, RenderStateBadge(s.IncompleteStatuses[i]))
			}
			moreCount := len(s.IncompleteStatuses) - 2
			moreBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("+%d more (%d)", moreCount, len(s.IncompleteStatuses)))
			statusValRendered = strings.Join(badges, " ") + " " + moreBadge
		}
	}

	var statusRow string
	if s.FocusedIndex == fieldSettingsIncompleteStatuses {
		statusHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  [Enter: Choose Checkboxes]")
		statusRow = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("▶ ") +
			lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render("Applicable Item Statuses:  ") +
			statusValRendered + statusHint
	} else {
		statusRow = "  " + lipgloss.NewStyle().Foreground(ColorFg).Render("Applicable Item Statuses:  ") +
			statusValRendered
	}

	// Date Statuses Multi-select row (styled similar to status row)
	var dateStatusValRendered string
	if len(s.IncompleteDateStatuses) == 0 || (len(s.IncompleteDateStatuses) == 1 && strings.EqualFold(s.IncompleteDateStatuses[0], "[all]")) {
		if s.FocusedIndex == fieldSettingsIncompleteDateStatuses {
			dateStatusValRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[All]")
		} else {
			dateStatusValRendered = lipgloss.NewStyle().Foreground(ColorMuted).Render("[All]")
		}
	} else {
		if len(s.IncompleteDateStatuses) <= 3 {
			var badges []string
			for _, st := range s.IncompleteDateStatuses {
				badges = append(badges, RenderStateBadge(st))
			}
			dateStatusValRendered = strings.Join(badges, " ")
		} else {
			var badges []string
			for i := 0; i < 2; i++ {
				badges = append(badges, RenderStateBadge(s.IncompleteDateStatuses[i]))
			}
			moreCount := len(s.IncompleteDateStatuses) - 2
			moreBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("+%d more (%d)", moreCount, len(s.IncompleteDateStatuses)))
			dateStatusValRendered = strings.Join(badges, " ") + " " + moreBadge
		}
	}

	var dateStatusRow string
	if s.FocusedIndex == fieldSettingsIncompleteDateStatuses {
		dateStatusHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  [Enter: Choose Checkboxes]")
		dateStatusRow = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("▶ ") +
			lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render("Applicable Date Statuses:  ") +
			dateStatusValRendered + dateStatusHint
	} else {
		dateStatusRow = "  " + lipgloss.NewStyle().Foreground(ColorFg).Render("Applicable Date Statuses:  ") +
			dateStatusValRendered
	}

	// Buttons rendering
	btnSaveStyle := lipgloss.NewStyle().Padding(0, 3).Bold(true)
	if s.FocusedIndex == fieldSettingsSaveBtn {
		btnSaveStyle = btnSaveStyle.Background(ColorGreen).Foreground(lipgloss.Color("#000000"))
	} else {
		btnSaveStyle = btnSaveStyle.Background(ColorSubtle).Foreground(ColorFg)
	}
	saveBtn := btnSaveStyle.Render("[ Save Settings (Ctrl+S) ]")

	btnCancelStyle := lipgloss.NewStyle().Padding(0, 3).Bold(true)
	if s.FocusedIndex == fieldSettingsCancelBtn {
		btnCancelStyle = btnCancelStyle.Background(ColorRed).Foreground(lipgloss.Color("#ffffff"))
	} else {
		btnCancelStyle = btnCancelStyle.Background(ColorSubtle).Foreground(ColorMuted)
	}
	cancelBtn := btnCancelStyle.Render("[ Cancel (Esc) ]")

	buttonsRow := lipgloss.JoinHorizontal(lipgloss.Center, "  ", saveBtn, "    ", cancelBtn)

	// Section Headers
	sectionHeader := func(title string) string {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Render("── " + title + " " + strings.Repeat("─", contentWidth-len(title)-4))
	}

	formContent := lipgloss.JoinVertical(lipgloss.Left,
		sectionHeader("AUTHENTICATION & AI CONFIGURATION"),
		"",
		fieldPat,
		"",
		fieldGeminiKey,
		"",
		fieldGeminiModel,
		"",
		sectionHeader("ESTIMATION & WORK ITEM PREFERENCES"),
		"",
		cbAutoCalc,
		"",
		cbEffortSame,
		"",
		cbAutoSetFinishDate,
		"",
		fieldAutoSync,
		"",
		sectionHeader("INCOMPLETE VIEW CRITERIA"),
		"",
		cbCritEffort,
		"",
		cbCritEstimate,
		"",
		cbCritStartDate,
		"",
		cbCritTargetDate,
		"",
		cbCritFinishDate,
		"",
		cbCritTagRegex,
		"",
		cbCritActivity,
		"",
		fieldTagRegex,
		"",
		typeRow,
		"",
		statusRow,
		"",
		dateStatusRow,
		"",
		sectionHeader("ACTIONS"),
		"",
		buttonsRow,
	)

	formCard := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(1, 2).
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
	if cardHeight < vpHeight {
		topPad := (vpHeight - cardHeight) / 2
		if topPad > 0 {
			centeredContent = strings.Repeat("\n", topPad) + centeredContent
		}
	}

	s.Viewport.SetContent(centeredContent)
	if s.FocusedIndex != fieldSettingsPat && s.Viewport.YOffset == 0 {
		s.scrollToFocused()
	}
	viewportView := s.Viewport.View()

	// Bottom Ribbon
	shortcuts := []Shortcut{
		{Key: "Tab / ↓", Label: "Next Field"},
		{Key: "Shift+Tab / ↑", Label: "Prev Field"},
		{Key: "Space", Label: "Toggle / Select"},
		{Key: "Wheel / PgDn", Label: "Scroll"},
		{Key: "Ctrl+S / Enter", Label: "Save"},
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

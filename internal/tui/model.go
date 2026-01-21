package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"phopy/internal/domain"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Phase represents the current state of the TUI
type Phase int

const (
	PhaseScanning Phase = iota
	PhasePreview
	PhaseConfirm
	PhaseExecuting
	PhaseDone
	PhaseError
)

// Messages for the TUI
type (
	PlanReadyMsg struct {
		Plan domain.CopyPlan
	}
	ScanProgressMsg struct {
		Current int
		Total   int
	}
	CopyProgressMsg struct {
		Current int
		Total   int
		File    string
	}
	CopyDoneMsg struct {
		OverridesConfirmed int
	}
	ErrorMsg struct {
		Err error
	}
	tickMsg time.Time
)

// ExecuteCopyFunc is called to start the copy operation
// It should run the copy in a goroutine and send progress/done messages
type ExecuteCopyFunc func(plan domain.CopyPlan, includeOverrides bool) tea.Cmd

// Config for the TUI
type Config struct {
	SourceDir   string
	TargetDir   string
	DryRun      bool
	Verbose     bool
	ExecuteCopy ExecuteCopyFunc
}

// Model is the main TUI model
type Model struct {
	config             Config
	Phase              Phase
	Plan               domain.CopyPlan
	spinner            spinner.Model
	progress           progress.Model
	scanCurrent        int
	scanTotal          int
	copyProgress       int
	copyTotal          int
	currentFile        string
	confirmSelection   bool // true = yes, false = no
	OverridesConfirmed int
	Err                error
	Quitting           bool
	width              int
	height             int
}

// NewModel creates a new TUI model
func NewModel(cfg Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(50),
		progress.WithoutPercentage(),
	)

	return Model{
		config:           cfg,
		Phase:            PhaseScanning,
		spinner:          s,
		progress:         p,
		confirmSelection: false, // default to No
		width:            80,
		height:           24,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = min(msg.Width-20, 60)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.Quitting = true
			return m, tea.Quit
		case "left", "h":
			if m.Phase == PhaseConfirm {
				m.confirmSelection = true
			}
		case "right", "l":
			if m.Phase == PhaseConfirm {
				m.confirmSelection = false
			}
		case "y", "Y":
			if m.Phase == PhaseConfirm {
				m.confirmSelection = true
			}
		case "n", "N":
			if m.Phase == PhaseConfirm {
				m.confirmSelection = false
			}
		case "enter":
			if m.Phase == PhaseConfirm {
				return m, func() tea.Msg {
					return ConfirmMsg{Confirmed: m.confirmSelection}
				}
			}
			if m.Phase == PhaseDone || m.Phase == PhaseError {
				return m, tea.Quit
			}
		}

	case ScanProgressMsg:
		m.scanCurrent = msg.Current
		m.scanTotal = msg.Total
		return m, nil

	case PlanReadyMsg:
		m.Plan = msg.Plan
		if m.config.DryRun {
			m.Phase = PhaseDone
		} else if len(m.Plan.OverrideItems) > 0 {
			m.Phase = PhaseConfirm
		} else {
			// No overrides needed, start copy immediately
			m.Phase = PhaseExecuting
			if m.config.ExecuteCopy != nil {
				return m, tea.Batch(tickCmd(), m.config.ExecuteCopy(m.Plan, false))
			}
		}
		return m, nil

	case ConfirmMsg:
		includeOverrides := msg.Confirmed
		if includeOverrides {
			m.OverridesConfirmed = len(m.Plan.OverrideItems)
		}
		// Start copy
		m.Phase = PhaseExecuting
		if m.config.ExecuteCopy != nil {
			return m, tea.Batch(tickCmd(), m.config.ExecuteCopy(m.Plan, includeOverrides))
		}
		return m, nil

	case CopyProgressMsg:
		m.copyProgress = msg.Current
		m.copyTotal = msg.Total
		m.currentFile = msg.File
		return m, nil

	case CopyDoneMsg:
		m.Phase = PhaseDone
		if msg.OverridesConfirmed > 0 {
			m.OverridesConfirmed = msg.OverridesConfirmed
		}
		return m, nil

	case ErrorMsg:
		m.Phase = PhaseError
		m.Err = msg.Err
		return m, nil

	case spinner.TickMsg:
		if m.Phase == PhaseScanning || m.Phase == PhaseExecuting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case tickMsg:
		if m.Phase == PhaseExecuting {
			var cmds []tea.Cmd
			if m.copyTotal > 0 {
				cmds = append(cmds, m.progress.SetPercent(float64(m.copyProgress)/float64(m.copyTotal)))
			}
			cmds = append(cmds, tickCmd(), m.spinner.Tick)
			return m, tea.Batch(cmds...)
		}
	}

	return m, nil
}

// Additional message types
type (
	ConfirmMsg struct{ Confirmed bool }
)

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) View() string {
	if m.Quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	switch m.Phase {
	case PhaseScanning:
		b.WriteString(m.renderScanning())
	case PhasePreview:
		b.WriteString(m.renderPreview())
	case PhaseDone:
		b.WriteString(m.renderPreview())
		if !m.config.DryRun {
			b.WriteString("\n")
			b.WriteString(m.renderCopyCompletion())
		}
	case PhaseConfirm:
		b.WriteString(m.renderPreview())
		b.WriteString("\n")
		b.WriteString(m.renderConfirmPrompt())
	case PhaseExecuting:
		b.WriteString(m.renderPreview())
		b.WriteString("\n")
		b.WriteString(m.renderExecution())
	case PhaseError:
		b.WriteString(m.renderError())
	}

	// Help
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderHeader() string {
	title := titleStyle.Render("📷 Phopy")
	subtitle := subtitleStyle.Render("Photo organization made simple")

	dimStyle := lipgloss.NewStyle().Foreground(dimTextColor)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		dimStyle.Render(fmt.Sprintf("%s Source: %s", iconFolder, shortenPath(m.config.SourceDir))),
		dimStyle.Render(fmt.Sprintf("%s Target: %s", iconFolder, shortenPath(m.config.TargetDir))),
	)
}

func (m Model) renderScanning() string {
	if m.scanTotal > 0 {
		percent := float64(m.scanCurrent) / float64(m.scanTotal)
		progressBar := m.progress.ViewAs(percent)

		countStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
		percentStyle := lipgloss.NewStyle().Foreground(dimTextColor)

		return fmt.Sprintf("%s Scanning photos...\n\n  %s\n  %s %s",
			m.spinner.View(),
			progressBar,
			countStyle.Render(fmt.Sprintf("%d/%d", m.scanCurrent, m.scanTotal)),
			percentStyle.Render(fmt.Sprintf("(%.0f%%)", percent*100)),
		)
	}
	return fmt.Sprintf("%s Scanning photos...", m.spinner.View())
}

func (m Model) renderPreview() string {
	var b strings.Builder

	// Files to copy section
	b.WriteString(sectionStyle.Render("Files to Copy"))
	b.WriteString("\n\n")

	if len(m.Plan.Items) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(dimTextColor)
		b.WriteString(dimStyle.Render("  No files to copy"))
		b.WriteString("\n")
	} else {
		lines := formatFileList(m.Plan.Items, 4)
		for _, line := range lines {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Override section if any
	if len(m.Plan.OverrideItems) > 0 {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render(fmt.Sprintf("%s Override Required (%d files)", iconOverride, len(m.Plan.OverrideItems))))
		b.WriteString("\n\n")

		for i, item := range m.Plan.OverrideItems {
			if i >= 4 {
				b.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.Plan.OverrideItems)-4))
				break
			}
			b.WriteString(fmt.Sprintf("  %s %s\n",
				overrideStyle.Render(iconOverride),
				fileNameStyle.Render(item.FileMeta.Name),
			))
		}
	}

	// Summary
	b.WriteString("\n")
	b.WriteString(m.renderSummary())

	// Warnings
	if m.config.Verbose && len(m.Plan.Warnings) > 0 {
		b.WriteString("\n\n")
		b.WriteString(warningStyle.Render("Warnings:"))
		b.WriteString("\n")
		for _, w := range m.Plan.Warnings {
			b.WriteString(fmt.Sprintf("  %s %s\n", iconOverride, w))
		}
	}

	return b.String()
}

func (m Model) renderSummary() string {
	var b strings.Builder

	b.WriteString(sectionStyle.Render("Summary"))
	b.WriteString("\n\n")

	// Date range
	if m.Plan.RangeStart != nil && m.Plan.RangeEnd != nil {
		dateRange := fmt.Sprintf("%s %s %s",
			m.Plan.RangeStart.Format("2006-01-02"),
			iconArrow,
			m.Plan.RangeEnd.Format("2006-01-02"),
		)
		b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Date Range:"), dateStyle.Render(dateRange)))
	}

	// File counts
	rawStat := fmt.Sprintf("%s %d", iconRAW, m.Plan.RawCount)
	jpegStat := fmt.Sprintf("%s %d", iconJPEG, m.Plan.JpegCount)

	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("RAW files:"), rawFileStyle.Render(rawStat)))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("JPEG files:"), jpegFileStyle.Render(jpegStat)))
	dimStyle := lipgloss.NewStyle().Foreground(dimTextColor)
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Skipped JPEGs:"), dimStyle.Render(fmt.Sprintf("%s %d", iconSkipped, m.Plan.SkippedJPEGs))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Skipped RAWs (date):"), dimStyle.Render(fmt.Sprintf("%s %d", iconSkipped, m.Plan.SkippedRAWsDate))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Skipped RAWs (dupl):"), dimStyle.Render(fmt.Sprintf("%s %d", iconSkipped, m.Plan.SkippedRAWsDupl))))

	if m.Plan.RawOverrides+m.Plan.JpegOverrides > 0 {
		overrideCount := m.Plan.RawOverrides + m.Plan.JpegOverrides
		b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Overrides:"), warningStyle.Render(fmt.Sprintf("%s %d", iconOverride, overrideCount))))
	}

	if m.config.DryRun {
		b.WriteString("\n")
		b.WriteString(highlightBoxStyle.Render("🔍 Dry Run - No files were copied"))
	}

	return b.String()
}

func (m Model) renderConfirmPrompt() string {
	prompt := confirmPromptStyle.Render(fmt.Sprintf("Override %d existing files?", len(m.Plan.OverrideItems)))

	var yesBtn, noBtn string
	if m.confirmSelection {
		yesBtn = highlightBoxStyle.Copy().
			Background(lipgloss.Color("#2D5A27")).
			Render(" Yes ")
		noBtn = boxStyle.Render(" No ")
	} else {
		yesBtn = boxStyle.Render(" Yes ")
		noBtn = highlightBoxStyle.Copy().
			Background(lipgloss.Color("#5A2727")).
			Render(" No ")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "  ", noBtn)

	return lipgloss.JoinVertical(lipgloss.Left, prompt, "", buttons)
}

func (m Model) renderExecution() string {
	var b strings.Builder

	b.WriteString(sectionStyle.Render("Copying Files"))
	b.WriteString("\n\n")

	// Progress bar
	percent := 0.0
	if m.copyTotal > 0 {
		percent = float64(m.copyProgress) / float64(m.copyTotal)
	}

	// Spinner and progress
	b.WriteString(fmt.Sprintf("  %s Copying...\n\n", m.spinner.View()))
	b.WriteString(fmt.Sprintf("  %s\n", m.progress.ViewAs(percent)))

	countStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	percentStyle := lipgloss.NewStyle().Foreground(dimTextColor)

	b.WriteString(fmt.Sprintf("  %s %s\n",
		countStyle.Render(fmt.Sprintf("%d/%d files", m.copyProgress, m.copyTotal)),
		percentStyle.Render(fmt.Sprintf("(%.0f%%)", percent*100)),
	))

	if m.currentFile != "" {
		b.WriteString(fmt.Sprintf("\n  %s %s\n",
			iconArrow,
			fileNameStyle.Render(m.currentFile),
		))
	}

	return b.String()
}

func (m Model) renderCopyCompletion() string {
	var b strings.Builder

	b.WriteString(sectionStyle.Render("Copy Complete"))
	b.WriteString("\n\n")

	// Success message
	icon := successStyle.Render(iconSuccess)
	msg := successStyle.Render("Copy completed successfully!")
	b.WriteString(fmt.Sprintf("  %s %s\n\n", icon, msg))

	// Statistics
	totalCopied := m.Plan.RawCount + m.Plan.JpegCount
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("RAW files copied:"), rawFileStyle.Render(fmt.Sprintf("%s %d", iconRAW, m.Plan.RawCount))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("JPEG files copied:"), jpegFileStyle.Render(fmt.Sprintf("%s %d", iconJPEG, m.Plan.JpegCount))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Total copied:"), statValueStyle.Render(fmt.Sprintf("%d files", totalCopied))))

	if m.Plan.SkippedJPEGs > 0 {
		dimStyle := lipgloss.NewStyle().Foreground(dimTextColor)
		b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Skipped JPEGs:"), dimStyle.Render(fmt.Sprintf("%s %d (RAW exists)", iconSkipped, m.Plan.SkippedJPEGs))))
	}

	if m.OverridesConfirmed > 0 {
		b.WriteString(fmt.Sprintf("  %s  %s\n", statLabelStyle.Render("Files overwritten:"), warningStyle.Render(fmt.Sprintf("%s %d", iconOverride, m.OverridesConfirmed))))
	}

	return b.String()
}

func (m Model) renderError() string {
	icon := errorStyle.Render(iconError)
	msg := errorStyle.Render(fmt.Sprintf("Error: %s", m.Err.Error()))

	return highlightBoxStyle.Copy().
		BorderForeground(errorColor).
		Render(fmt.Sprintf("%s %s", icon, msg))
}

func (m Model) renderHelp() string {
	var help string
	switch m.Phase {
	case PhaseScanning:
		help = "Press q to quit"
	case PhasePreview:
		help = "Press q to quit"
	case PhaseConfirm:
		help = "← → or y/n to select • Enter to confirm • q to quit"
	case PhaseExecuting:
		help = "Copying files... Please wait"
	case PhaseDone:
		help = "Press Enter to exit"
	case PhaseError:
		help = "Press Enter or q to exit"
	}
	return helpStyle.Render(help)
}

// formatFileList formats a list of copy items for display
func formatFileList(items []domain.CopyItem, maxItems int) []string {
	if len(items) == 0 {
		return []string{}
	}

	lines := make([]string, 0, min(len(items), maxItems+1))

	showCount := min(len(items), maxItems)
	if len(items) > maxItems {
		// Show first half and last half
		half := maxItems / 2
		for i := 0; i < half; i++ {
			lines = append(lines, formatFileItem(items[i]))
		}
		dimStyle := lipgloss.NewStyle().Foreground(dimTextColor)
		lines = append(lines, dimStyle.Render(fmt.Sprintf("... %d more files ...", len(items)-maxItems)))
		for i := len(items) - half; i < len(items); i++ {
			lines = append(lines, formatFileItem(items[i]))
		}
	} else {
		for i := 0; i < showCount; i++ {
			lines = append(lines, formatFileItem(items[i]))
		}
	}

	return lines
}

func formatFileItem(item domain.CopyItem) string {
	icon := iconJPEG
	style := jpegFileStyle
	if item.FileMeta.IsRAW {
		icon = iconRAW
		style = rawFileStyle
	}

	name := style.Render(item.FileMeta.Name)
	date := dateStyle.Render(item.FileMeta.TakenAt.Format("2006-01-02 15:04"))

	return fmt.Sprintf("%s %s  %s", icon, name, date)
}


func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// shortenPath replaces the home directory prefix with ~ for display
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

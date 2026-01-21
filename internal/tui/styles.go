package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Color palette - warm, photography-inspired
	primaryColor   = lipgloss.Color("#E8A87C") // warm orange
	secondaryColor = lipgloss.Color("#85DCB0") // mint green
	accentColor    = lipgloss.Color("#C38D9E") // dusty rose
	warningColor   = lipgloss.Color("#F6AE2D") // amber warning
	errorColor     = lipgloss.Color("#E85D75") // soft red
	mutedColor     = lipgloss.Color("#6B7280") // gray
	textColor      = lipgloss.Color("#F3F4F6") // light text
	dimTextColor   = lipgloss.Color("#9CA3AF") // dim text

	// Base styles
	baseStyle = lipgloss.NewStyle()

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(dimTextColor).
			Italic(true)

	// Section header
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(mutedColor).
			MarginTop(1).
			MarginBottom(1).
			PaddingBottom(0)

	// File display styles
	fileNameStyle = lipgloss.NewStyle().
			Foreground(textColor)

	rawFileStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	jpegFileStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	dateStyle = lipgloss.NewStyle().
			Foreground(dimTextColor)

	pathStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	// Status indicators
	successStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Box styles for sections
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(1, 2).
			MarginTop(1)

	highlightBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(1, 2).
				MarginTop(1)

	// Summary stat styles
	statLabelStyle = lipgloss.NewStyle().
			Foreground(dimTextColor).
			Width(20)

	statValueStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Bold(true)

	// Progress bar styling
	progressStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	// Confirmation prompt
	confirmPromptStyle = lipgloss.NewStyle().
				Foreground(warningColor).
				Bold(true).
				MarginTop(1)

	confirmYesStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	confirmNoStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Override item style
	overrideStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	// Spinner style
	spinnerStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	// Help text
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			MarginTop(2)

	// Icon characters
	iconCopy     = "📷"
	iconRAW      = "◆"
	iconJPEG     = "◇"
	iconSkipped  = "○"
	iconOverride = "⚠"
	iconSuccess  = "✓"
	iconError    = "✗"
	iconArrow    = "→"
	iconFolder   = "📁"
)

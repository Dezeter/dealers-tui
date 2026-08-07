package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	networkMainnet = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true) // red = real money
	networkTestnet = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)  // green = safe

	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	sectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true).Underline(true)
	focusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)

	alertCritStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("124")).Bold(true)
	alertWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("214")).Bold(true)
)

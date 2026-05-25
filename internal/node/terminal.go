package node

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	promptStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	commandStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("86")).Padding(0, 1)
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
)

func (n *Node) prompt() string {
	return promptStyle.Render(fmt.Sprintf("%s", n.id)) + mutedStyle.Render(" mesh> ")
}

func okf(format string, args ...any) {
	fmt.Println(okStyle.Render(fmt.Sprintf(format, args...)))
}

func warnf(format string, args ...any) {
	fmt.Println(warnStyle.Render(fmt.Sprintf(format, args...)))
}

func errf(format string, args ...any) {
	fmt.Println(errStyle.Render(fmt.Sprintf(format, args...)))
}

func labelf(label, value string) string {
	return labelStyle.Render(label) + " " + value
}

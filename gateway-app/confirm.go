package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// confirmModel is a tiny "do this? [y/N]" prompt, used when ssh'ing
// directly to an agent name that has no Sandbox yet.
type confirmModel struct {
	question string
	answer   bool // only meaningful once done is true
	done     bool
}

func newConfirmModel(question string) confirmModel {
	return confirmModel{question: question}
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		m.answer = true
		m.done = true
		return m, tea.Quit
	case "n", "N", "enter", "esc", "ctrl+c":
		m.answer = false
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

var (
	confirmStyle = lipgloss.NewStyle().Bold(true)
	hintStyle    = lipgloss.NewStyle().Faint(true)
)

func (m confirmModel) View() tea.View {
	s := confirmStyle.Render(m.question) + " [y/N] "
	if !m.done {
		s += "\r\n" + hintStyle.Render("y = yes, anything else = no")
	}
	return tea.NewView(s)
}

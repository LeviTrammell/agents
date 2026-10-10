package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// resumeCountdown is how long the reconnect prompt waits before going back
// into the last agent on its own.
const resumeCountdown = 3

// resumePromptModel asks, with a short countdown, whether a reconnect should
// go back into the agent the dropped connection was in. Waiting or enter
// resumes; any other key goes to the menu, so picking a different agent is
// one keypress away.
type resumePromptModel struct {
	agent     string
	remaining int
	resume    bool
	done      bool
}

type resumeTickMsg struct{}

func newResumePromptModel(agent string) resumePromptModel {
	return resumePromptModel{agent: agent, remaining: resumeCountdown}
}

func resumeTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return resumeTickMsg{} })
}

func (m resumePromptModel) Init() tea.Cmd { return resumeTick() }

func (m resumePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resumeTickMsg:
		m.remaining--
		if m.remaining <= 0 {
			m.resume, m.done = true, true
			return m, tea.Quit
		}
		return m, resumeTick()
	case tea.KeyPressMsg:
		m.resume = msg.String() == "enter"
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m resumePromptModel) View() tea.View {
	s := confirmStyle.Render(fmt.Sprintf("Reconnecting to agent-%s in %d…", m.agent, m.remaining))
	s += "\r\n" + hintStyle.Render("enter: now · any other key: menu")
	return tea.NewView(s)
}

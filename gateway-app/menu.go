package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

// menuAction is what the menu asked the caller to do once its Program
// exits.
type menuAction int

const (
	menuActionQuit menuAction = iota
	menuActionNew
)

type menuMode int

const (
	modeList menuMode = iota
	modeDeleteConfirm
)

// menuModel is the Bubble Tea model for `ssh levi@agents` / `ssh
// menu@agents`: a refreshing list of agents with single-key actions.
type menuModel struct {
	ctx         context.Context
	clients     *k8sctl.Clients
	namespace   string
	broadcaster *sizeBroadcaster
	term        string
	debugEcho   bool
	width       int
	height      int

	agents  []k8sctl.AgentInfo
	cursor  int
	status  string
	loading bool
	err     error

	mode        menuMode
	deleteInput string

	action menuAction
}

func newMenuModel(ctx context.Context, clients *k8sctl.Clients, namespace, term string, debugEcho bool, broadcaster *sizeBroadcaster) menuModel {
	return menuModel{
		ctx:         ctx,
		clients:     clients,
		namespace:   namespace,
		broadcaster: broadcaster,
		term:        term,
		debugEcho:   debugEcho,
		width:       45,
		height:      20,
	}
}

type agentsListedMsg struct {
	agents []k8sctl.AgentInfo
	err    error
}
type refreshTickMsg struct{}
type actionDoneMsg struct {
	status string
	err    error
}

func listAgentsCmd(ctx context.Context, clients *k8sctl.Clients, namespace string) tea.Cmd {
	return func() tea.Msg {
		list, err := clients.ListAgents(ctx, namespace)
		if err == nil {
			sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		}
		return agentsListedMsg{agents: list, err: err}
	}
}

func refreshTick() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg { return refreshTickMsg{} })
}

func (m menuModel) Init() tea.Cmd {
	return tea.Batch(listAgentsCmd(m.ctx, m.clients, m.namespace), refreshTick())
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case agentsListedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.agents = msg.agents
			if m.cursor >= len(m.agents) {
				m.cursor = len(m.agents) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
		return m, nil
	case refreshTickMsg:
		return m, tea.Batch(listAgentsCmd(m.ctx, m.clients, m.namespace), refreshTick())
	case actionDoneMsg:
		m.status = msg.status
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, listAgentsCmd(m.ctx, m.clients, m.namespace)
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m menuModel) selected() (k8sctl.AgentInfo, bool) {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return k8sctl.AgentInfo{}, false
	}
	return m.agents[m.cursor], true
}

func (m menuModel) updateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeDeleteConfirm {
		return m.updateDeleteConfirmKey(key)
	}

	switch key.String() {
	case "ctrl+c", "q":
		m.action = menuActionQuit
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.agents)-1 {
			m.cursor++
		}
		return m, nil
	case "n":
		m.action = menuActionNew
		return m, tea.Quit
	case "enter":
		agent, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, m.attachCmd(agent.Name)
	case "s":
		agent, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.status = fmt.Sprintf("suspending agent-%s...", agent.Name)
		return m, m.setModeCmd(agent.Name, "Suspended")
	case "r":
		agent, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.status = fmt.Sprintf("starting agent-%s...", agent.Name)
		return m, m.setModeCmd(agent.Name, "Running")
	case "d":
		agent, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.mode = modeDeleteConfirm
		m.deleteInput = ""
		m.status = ""
		_ = agent
		return m, nil
	}
	return m, nil
}

func (m menuModel) updateDeleteConfirmKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		m.deleteInput = ""
		return m, nil
	case "enter":
		agent, ok := m.selected()
		if !ok || m.deleteInput != agent.Name {
			m.status = "name did not match; delete cancelled"
			m.mode = modeList
			m.deleteInput = ""
			return m, nil
		}
		m.mode = modeList
		m.status = fmt.Sprintf("deleting agent-%s...", agent.Name)
		return m, m.deleteCmd(agent.Name)
	case "backspace":
		if len(m.deleteInput) > 0 {
			m.deleteInput = m.deleteInput[:len(m.deleteInput)-1]
		}
		return m, nil
	default:
		if key.Text != "" {
			m.deleteInput += key.Text
		}
		return m, nil
	}
}

func (m menuModel) attachCmd(name string) tea.Cmd {
	cmd := &attachExecCommand{
		ctx:         m.ctx,
		clients:     m.clients,
		namespace:   m.namespace,
		agentName:   name,
		command:     herdrCommand(m.term, m.debugEcho),
		broadcaster: m.broadcaster,
	}
	return tea.Exec(cmd, func(err error) tea.Msg {
		if err != nil {
			return actionDoneMsg{err: fmt.Errorf("attach to agent-%s: %w", name, err)}
		}
		return actionDoneMsg{status: fmt.Sprintf("detached from agent-%s", name)}
	})
}

func (m menuModel) setModeCmd(name, mode string) tea.Cmd {
	return func() tea.Msg {
		if err := m.clients.SetOperatingMode(m.ctx, m.namespace, name, mode); err != nil {
			return actionDoneMsg{err: fmt.Errorf("setting agent-%s to %s: %w", name, mode, err)}
		}
		verb := "started"
		if mode == "Suspended" {
			verb = "suspended"
		}
		return actionDoneMsg{status: fmt.Sprintf("agent-%s %s", name, verb)}
	}
}

func (m menuModel) deleteCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.clients.DeleteSandbox(m.ctx, m.namespace, name); err != nil {
			return actionDoneMsg{err: fmt.Errorf("deleting agent-%s: %w", name, err)}
		}
		return actionDoneMsg{status: fmt.Sprintf("agent-%s deleted", name)}
	}
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Faint(true)
	warnStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)

func (m menuModel) View() tea.View {
	var b strings.Builder
	b.WriteString(headerStyle.Render("agents") + "\r\n")

	if m.err != nil {
		fmt.Fprintf(&b, "%s\r\n", warnStyle.Render("error: "+m.err.Error()))
	} else if len(m.agents) == 0 {
		b.WriteString(dimStyle.Render("(no agents yet; press n)") + "\r\n")
	}

	for i, a := range m.agents {
		line := fmt.Sprintf("%-14s %-9s %-7s %s", truncate(a.Name, 14), a.Mode, a.Ready, humanAge(a.CreatedAt))
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> "+line) + "\r\n")
		} else {
			b.WriteString("  " + line + "\r\n")
		}
	}

	b.WriteString("\r\n")
	if m.mode == modeDeleteConfirm {
		agent, _ := m.selected()
		fmt.Fprintf(&b, "%s\r\n", warnStyle.Render(fmt.Sprintf("delete agent-%s and its home volume?", agent.Name)))
		fmt.Fprintf(&b, "type %q to confirm: %s█\r\n", agent.Name, m.deleteInput)
		b.WriteString(dimStyle.Render("esc to cancel"))
	} else if m.status != "" {
		b.WriteString(dimStyle.Render(m.status) + "\r\n")
		b.WriteString(dimStyle.Render("enter attach  n new  s stop  r start  d delete  q quit"))
	} else {
		b.WriteString(dimStyle.Render("enter attach  n new  s stop  r start  d delete  q quit"))
	}

	return tea.NewView(b.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < 2*time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < 2*time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

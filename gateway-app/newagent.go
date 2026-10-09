package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/levitrammell/agents/gateway-app/internal/agents"
	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

type newAgentStage int

const (
	stageName newAgentStage = iota
	stageRepo
	stageCreating
	stageDone
)

// newAgentProgress is a tiny latest-value-wins mailbox so the background
// creation goroutine can report status lines without a *tea.Program
// reference; the model polls it on a tea.Tick.
type newAgentProgress struct {
	mu   sync.Mutex
	line string
}

func (p *newAgentProgress) set(line string) {
	p.mu.Lock()
	p.line = line
	p.mu.Unlock()
}

func (p *newAgentProgress) get() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.line
}

// newAgentModel drives the `ssh new@agents` flow: prompt for a name,
// prompt for an optional git repo, create the Sandbox, wait for Ready,
// optionally clone, then quit so the caller can attach.
type newAgentModel struct {
	ctx       context.Context
	clients   *k8sctl.Clients
	namespace string
	image     string

	stage    newAgentStage
	name     string
	repo     string
	input    string
	nameErr  string
	progress *newAgentProgress
	result   newAgentResult
}

type newAgentResult struct {
	Name string
	Err  error
}

type progressTickMsg struct{ line string }
type createDoneMsg struct{ result newAgentResult }

func newNewAgentModel(ctx context.Context, clients *k8sctl.Clients, namespace, image string) newAgentModel {
	return newAgentModel{
		ctx:       ctx,
		clients:   clients,
		namespace: namespace,
		image:     image,
		stage:     stageName,
		progress:  &newAgentProgress{},
	}
}

func (m newAgentModel) Init() tea.Cmd { return nil }

func progressTick(p *newAgentProgress) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(_ time.Time) tea.Msg {
		return progressTickMsg{line: p.get()}
	})
}

func (m newAgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressTickMsg:
		if m.stage == stageCreating {
			return m, progressTick(m.progress)
		}
		return m, nil
	case createDoneMsg:
		m.result = msg.result
		m.stage = stageDone
		return m, tea.Quit
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m newAgentModel) updateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+c" || key.String() == "esc" {
		m.result = newAgentResult{Err: errCancelled}
		m.stage = stageDone
		return m, tea.Quit
	}

	switch m.stage {
	case stageName, stageRepo:
		switch key.String() {
		case "enter":
			return m.submitField()
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		default:
			if key.Text != "" {
				m.input += key.Text
			}
			return m, nil
		}
	}
	return m, nil
}

func (m newAgentModel) submitField() (tea.Model, tea.Cmd) {
	switch m.stage {
	case stageName:
		name := strings.TrimSpace(m.input)
		if err := agents.ValidateName(name); err != nil {
			m.nameErr = err.Error()
			return m, nil
		}
		if agents.IsReserved(name) {
			m.nameErr = fmt.Sprintf("%q is reserved; choose another name", name)
			return m, nil
		}
		m.name = name
		m.input = ""
		m.nameErr = ""
		m.stage = stageRepo
		return m, nil
	case stageRepo:
		m.repo = strings.TrimSpace(m.input)
		m.input = ""
		m.stage = stageCreating
		return m, tea.Batch(progressTick(m.progress), startCreate(m))
	}
	return m, nil
}

func startCreate(m newAgentModel) tea.Cmd {
	return func() tea.Msg {
		spec := agents.DefaultSandboxSpec(m.name, m.namespace, m.image)
		m.progress.set(fmt.Sprintf("creating agent-%s...", m.name))
		if _, err := m.clients.CreateSandbox(m.ctx, spec); err != nil {
			return createDoneMsg{newAgentResult{Err: fmt.Errorf("creating agent-%s: %w", m.name, err)}}
		}
		m.progress.set(fmt.Sprintf("waiting for agent-%s to be Ready...", m.name))
		if err := m.clients.WaitReady(m.ctx, m.namespace, m.name, func(status string) {
			m.progress.set(fmt.Sprintf("waiting for agent-%s to be Ready (status: %s)...", m.name, status))
		}); err != nil {
			return createDoneMsg{newAgentResult{Name: m.name, Err: fmt.Errorf("waiting for agent-%s: %w", m.name, err)}}
		}
		if m.repo != "" {
			dest, err := cloneDirFor(m.repo)
			if err != nil {
				return createDoneMsg{newAgentResult{Name: m.name, Err: err}}
			}
			m.progress.set(fmt.Sprintf("cloning %s...", m.repo))
			podName := agents.SandboxName(m.name)
			if err := m.clients.Exec(m.ctx, m.namespace, podName, []string{"git", "clone", m.repo, "/home/agent/work/" + dest}, k8sctl.ExecStreams{}); err != nil {
				return createDoneMsg{newAgentResult{Name: m.name, Err: fmt.Errorf("cloning %s: %w", m.repo, err)}}
			}
		}
		return createDoneMsg{newAgentResult{Name: m.name}}
	}
}

// cloneDirFor mirrors clone_dir_for() in bin/agent: the basename of the
// repo URL with a trailing .git stripped, restricted to a single safe path
// segment.
func cloneDirFor(repo string) (string, error) {
	base := repo
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	if base == "" || strings.ContainsAny(base, "/\\") {
		return "", fmt.Errorf("cannot derive a safe clone directory from repo %q; use a URL whose last path segment is a repository name", repo)
	}
	for _, r := range base {
		if !(r == '.' || r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", fmt.Errorf("cannot derive a safe clone directory from repo %q; use a URL whose last path segment is a repository name", repo)
		}
	}
	return base, nil
}

var errCancelled = fmt.Errorf("cancelled")

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	errStyle   = lipgloss.NewStyle().Bold(true)
)

func (m newAgentModel) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Create a new agent") + "\r\n\r\n")
	switch m.stage {
	case stageName:
		fmt.Fprintf(&b, "Name: %s█\r\n", m.input)
		if m.nameErr != "" {
			fmt.Fprintf(&b, "%s\r\n", errStyle.Render(m.nameErr))
		}
		b.WriteString(hintStyle.Render("lowercase letters, digits, hyphens; enter to continue; esc to cancel"))
	case stageRepo:
		fmt.Fprintf(&b, "Agent: agent-%s\r\n", m.name)
		fmt.Fprintf(&b, "Git repo (optional): %s█\r\n", m.input)
		b.WriteString(hintStyle.Render("enter to create; esc to cancel"))
	case stageCreating:
		fmt.Fprintf(&b, "Agent: agent-%s\r\n\r\n%s\r\n", m.name, m.progress.get())
	case stageDone:
		if m.result.Err != nil {
			fmt.Fprintf(&b, "%s\r\n", errStyle.Render(m.result.Err.Error()))
		} else {
			fmt.Fprintf(&b, "agent-%s is ready.\r\n", m.result.Name)
		}
	}
	return tea.NewView(b.String())
}

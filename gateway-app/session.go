package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	sshlib "charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"github.com/levitrammell/agents/gateway-app/internal/agents"
	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

// runTeaProgram runs a Bubble Tea program bound to sess's streams,
// registering it with broadcaster so window-change events reach it (and,
// for the duration of any nested tea.Exec within it, still reach the
// terminal-size queue a pod exec is reading from). It returns the model's
// final state.
func runTeaProgram(sess sshlib.Session, broadcaster *sizeBroadcaster, model tea.Model) (tea.Model, error) {
	opts := bubbletea.MakeOptions(sess)
	// Some clients report a zero-sized pty-req window before the first
	// real resize; MakeOptions would otherwise hand the renderer a 0x0
	// viewport, which loops clearing the screen without ever drawing
	// anything. Override with a sane default in that case (a real resize
	// corrects it moments later via the broadcaster).
	if pty, _, ok := sess.Pty(); ok && (pty.Window.Width <= 0 || pty.Window.Height <= 0) {
		opts = append(opts, tea.WithWindowSize(80, 24))
	}
	p := tea.NewProgram(model, opts...)
	broadcaster.setProgram(p)
	defer broadcaster.setProgram(nil)
	final, err := p.Run()
	p.Kill()
	return final, err
}

// newSessionHandler returns the wish.Middleware chain's terminal handler:
// it routes an SSH session by username to the menu, the "new agent" flow,
// or a direct agent attach.
func newSessionHandler(clients *k8sctl.Clients, cfg Config) wish.Middleware {
	resumes := newResumeStore()
	return func(next sshlib.Handler) sshlib.Handler {
		return func(sess sshlib.Session) {
			handleSession(sess, clients, cfg, resumes)
			next(sess)
		}
	}
}

func handleSession(sess sshlib.Session, clients *k8sctl.Clients, cfg Config, resumes *resumeStore) {
	var ctx context.Context = sess.Context()
	user := sess.User()
	logger := slog.With("user", user, "remote", sess.RemoteAddr().String())

	pty, windowChanges, hasPty := sess.Pty()

	if !hasPty {
		// `ssh -N` never opens a session channel (its forwards are their own
		// direct-tcpip channels), so anything landing here wanted a shell or
		// command without a terminal. Refuse instead of hanging.
		wish.Fatalln(sess, "agent-ssh requires a PTY; reconnect with ssh -t")
		logger.Info("rejected session: no pty")
		return
	}

	// Some clients (or test harnesses) report a zero-sized pty-req window
	// before the first window-change. Bubble Tea's renderer loops without
	// drawing anything useful into a 0x0 viewport, so fall back to a sane
	// default until a real resize arrives.
	initialWindow := pty.Window
	if initialWindow.Width <= 0 || initialWindow.Height <= 0 {
		initialWindow.Width, initialWindow.Height = 80, 24
	}

	broadcaster := newSizeBroadcaster(ctx, initialWindow, windowChanges)

	route := agents.DecideRoute(user)
	logger.Info("routing session", "route", route.Kind.String())

	// levi@ and new@ sessions remember a dropped attach so a quick reconnect
	// (phone clients drop idle connections) lands back in that agent.
	// menu@ always shows the menu, and name@ already goes straight in.
	if (route.Kind == agents.RouteMenu && user != "menu") || route.Kind == agents.RouteNew {
		key := user + " " + gossh.FingerprintSHA256(sess.PublicKey())
		ctx = withResumeTracker(ctx, &resumeTracker{store: resumes, key: key})
		if agent, ok := resumes.recent(key); ok {
			logger.Info("resuming dropped attach", "agent", agent)
			if !resumeDroppedAttach(ctx, sess, clients, cfg, broadcaster, agent, pty.Term) {
				return
			}
			if route.Kind == agents.RouteNew {
				return
			}
		}
	}

	switch route.Kind {
	case agents.RouteInvalid:
		wish.Fatalf(sess, "invalid username %q: %v\r\n", user, route.Err)
	case agents.RouteMenu:
		runMenuLoop(ctx, sess, clients, cfg, broadcaster, pty.Term)
	case agents.RouteNew:
		runNewAgentFlow(ctx, sess, clients, cfg, broadcaster, pty.Term)
	case agents.RouteDirect:
		runDirectAttach(ctx, sess, clients, cfg, broadcaster, route.AgentName, pty.Term)
	}
}

// runMenuLoop runs the menu; when it asks for "new", it runs the new-agent
// flow and then loops back to the menu (so creating or attaching from the
// menu returns to the menu, per the task's UX).
func runMenuLoop(ctx context.Context, sess sshlib.Session, clients *k8sctl.Clients, cfg Config, broadcaster *sizeBroadcaster, term string) {
	for {
		model := newMenuModel(ctx, clients, cfg.Namespace, term, cfg.DebugEcho, broadcaster)
		final, err := runTeaProgram(sess, broadcaster, model)
		if err != nil {
			slog.Error("menu program exited with error", "error", err)
			return
		}
		mm, ok := final.(menuModel)
		if !ok {
			return
		}
		if mm.action != menuActionNew {
			return
		}
		if !runNewAgentFlow(ctx, sess, clients, cfg, broadcaster, term) {
			return
		}
		// fall through: loop back to the (now refreshed) menu
	}
}

// runNewAgentFlow runs the `new` TUI, then attaches to the created agent
// directly (outside Bubble Tea, since herdr owns the terminal from here).
// It returns false if the session ended (disconnected or cancelled) and
// true if the caller should keep going (e.g. back to the menu).
func runNewAgentFlow(ctx context.Context, sess sshlib.Session, clients *k8sctl.Clients, cfg Config, broadcaster *sizeBroadcaster, term string) bool {
	model := newNewAgentModel(ctx, clients, cfg.Namespace, cfg.Image)
	final, err := runTeaProgram(sess, broadcaster, model)
	if err != nil {
		slog.Error("new-agent program exited with error", "error", err)
		return false
	}
	nm, ok := final.(newAgentModel)
	if !ok {
		return false
	}
	if nm.result.Err != nil {
		if nm.result.Err != errCancelled {
			wish.Errorf(sess, "%v\r\n", nm.result.Err)
		}
		return true
	}

	pod := agents.SandboxName(nm.result.Name)
	err = trackAttach(ctx, nm.result.Name, sess, func(in io.Reader) error {
		return attachCommand(ctx, clients, cfg.Namespace, pod, herdrCommand(term, cfg.DebugEcho), in, sess, nil, broadcaster.TerminalSizeQueue())
	})
	if err != nil {
		wish.Errorf(sess, "attach to agent-%s: %v\r\n", nm.result.Name, err)
	}
	return true
}

// runDirectAttach implements `ssh <name>@agents`: attach straight to the
// Sandbox, offering to create it first if it doesn't exist yet.
func runDirectAttach(ctx context.Context, sess sshlib.Session, clients *k8sctl.Clients, cfg Config, broadcaster *sizeBroadcaster, name, term string) {
	exists, err := clients.SandboxExists(ctx, cfg.Namespace, name)
	if err != nil {
		wish.Fatalf(sess, "checking agent-%s: %v\r\n", name, err)
		return
	}

	if !exists {
		model := newConfirmModel(fmt.Sprintf("agent-%s does not exist. Create it?", name))
		final, err := runTeaProgram(sess, broadcaster, model)
		if err != nil {
			slog.Error("confirm program exited with error", "error", err)
			return
		}
		cm, ok := final.(confirmModel)
		if !ok || !cm.answer {
			wish.Println(sess, "not created.")
			return
		}
		spec := agents.DefaultSandboxSpec(name, cfg.Namespace, cfg.Image)
		if _, err := clients.CreateSandbox(ctx, spec); err != nil {
			wish.Fatalf(sess, "creating agent-%s: %v\r\n", name, err)
			return
		}
	}

	pod, err := resumeAndWaitReady(ctx, clients, cfg.Namespace, name, sess)
	if err != nil {
		wish.Fatalf(sess, "%v\r\n", err)
		return
	}

	if err := attachCommand(ctx, clients, cfg.Namespace, pod, herdrCommand(term, cfg.DebugEcho), sess, sess, nil, broadcaster.TerminalSizeQueue()); err != nil {
		wish.Errorf(sess, "attach to agent-%s: %v\r\n", name, err)
	}
}

// resumeDroppedAttach reattaches to the agent a previous connection was in
// when it dropped. It returns false if this connection has ended too.
func resumeDroppedAttach(ctx context.Context, sess sshlib.Session, clients *k8sctl.Clients, cfg Config, broadcaster *sizeBroadcaster, name, term string) bool {
	wish.Printf(sess, "Reconnecting to agent-%s (log in as menu@ for the menu)\r\n", name)
	pod, err := resumeAndWaitReady(ctx, clients, cfg.Namespace, name, sess)
	if err != nil {
		if t, ok := ctx.Value(resumeTrackerKey{}).(*resumeTracker); ok {
			t.store.forget(t.key)
		}
		wish.Errorf(sess, "%v\r\n", err)
		return ctx.Err() == nil
	}
	err = trackAttach(ctx, name, sess, func(in io.Reader) error {
		return attachCommand(ctx, clients, cfg.Namespace, pod, herdrCommand(term, cfg.DebugEcho), in, sess, nil, broadcaster.TerminalSizeQueue())
	})
	if err != nil && ctx.Err() == nil {
		wish.Errorf(sess, "attach to agent-%s: %v\r\n", name, err)
	}
	return ctx.Err() == nil
}

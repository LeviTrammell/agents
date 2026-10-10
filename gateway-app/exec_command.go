package main

import (
	"context"
	"io"

	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

// attachExecCommand implements tea.ExecCommand so the menu can hand off
// the terminal to a pod exec via tea.Exec: Bubble Tea calls SetStdin/
// SetStdout/SetStderr with the real session streams (after releasing the
// terminal) and then Run, and restores the terminal once Run returns.
type attachExecCommand struct {
	ctx         context.Context
	clients     *k8sctl.Clients
	namespace   string
	agentName   string
	command     []string
	broadcaster *sizeBroadcaster

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (c *attachExecCommand) SetStdin(r io.Reader)  { c.stdin = r }
func (c *attachExecCommand) SetStdout(w io.Writer) { c.stdout = w }
func (c *attachExecCommand) SetStderr(w io.Writer) { c.stderr = w }

func (c *attachExecCommand) Run() error {
	pod, err := resumeAndWaitReady(c.ctx, c.clients, c.namespace, c.agentName, c.stdout)
	if err != nil {
		return err
	}
	return trackAttach(c.ctx, c.agentName, func() error {
		return attachCommand(c.ctx, c.clients, c.namespace, pod, c.command, c.stdin, c.stdout, c.stderr, c.broadcaster.TerminalSizeQueue())
	})
}

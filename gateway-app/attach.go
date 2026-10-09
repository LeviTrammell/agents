package main

import (
	"context"
	"fmt"
	"io"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/levitrammell/agents/gateway-app/internal/agents"
	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

// readyTimeout bounds how long attach will wait for a Sandbox to become
// Ready after creating or resuming it, matching bin/agent's `kubectl wait
// --timeout=300s`.
const readyTimeout = 300 * time.Second

// resumeAndWaitReady resumes name's Sandbox if it is Suspended, waits for
// it to become Ready, and returns the pod name to exec into. Progress is
// written to out as it happens.
func resumeAndWaitReady(ctx context.Context, clients *k8sctl.Clients, namespace, name string, out io.Writer) (string, error) {
	sb, err := clients.GetSandbox(ctx, namespace, name)
	if err != nil {
		return "", fmt.Errorf("getting agent-%s: %w", name, err)
	}

	mode, _, _ := unstructured.NestedString(sb.Object, "spec", "operatingMode")
	if mode == "Suspended" {
		fmt.Fprintf(out, "Resuming agent-%s...\r\n", name)
		if err := clients.SetOperatingMode(ctx, namespace, name, "Running"); err != nil {
			return "", fmt.Errorf("resuming agent-%s: %w", name, err)
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	lastStatus := ""
	fmt.Fprintf(out, "Waiting for agent-%s to be Ready...\r\n", name)
	if err := clients.WaitReady(waitCtx, namespace, name, func(status string) {
		if status != lastStatus {
			fmt.Fprintf(out, "  status: %s\r\n", status)
			lastStatus = status
		}
	}); err != nil {
		return "", fmt.Errorf("waiting for agent-%s to be Ready: %w", name, err)
	}

	return agents.SandboxName(name), nil
}

// attachCommand attaches to pod via a TTY exec of command, bridging the
// SSH-session-side streams given in streams. It blocks until the remote
// process exits or ctx is cancelled.
func attachCommand(ctx context.Context, clients *k8sctl.Clients, namespace, pod string, command []string, stdin io.Reader, stdout, stderr io.Writer, sizeQ remotecommand.TerminalSizeQueue) error {
	return clients.Exec(ctx, namespace, pod, command, k8sctl.ExecStreams{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		TTY:               true,
		TerminalSizeQueue: sizeQ,
	})
}

// herdrCommand returns the command to run to attach to an agent's terminal
// session: `env TERM=<term> herdr`, or `echo ok` in debug mode so the
// attach path can be exercised without a real herdr binary in the image.
func herdrCommand(term string, debugEcho bool) []string {
	if debugEcho {
		return []string{"echo", "ok"}
	}
	if term == "" {
		term = "xterm-256color"
	}
	return []string{"env", "TERM=" + term, "herdr"}
}

// sandboxNotFound reports whether err is a Kubernetes "not found" error for
// a Sandbox Get.
func sandboxNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

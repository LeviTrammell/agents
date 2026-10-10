package k8sctl

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecStreams carries the SSH-session-side streams to bridge into a pod
// exec session.
type ExecStreams struct {
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer // ignored when TTY is true
	TTY               bool
	TerminalSizeQueue remotecommand.TerminalSizeQueue
}

// Exec runs command in the named pod's "agent" container and bridges it to
// streams until it exits or ctx is cancelled. It uses SPDY only: client-go's
// WebSocket executor kept its heartbeat goroutine pinging a dead connection
// forever after an SSH client dropped mid-attach.
func (c *Clients) Exec(ctx context.Context, namespace, podName string, command []string, streams ExecStreams) error {
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "agent",
			Command:   command,
			Stdin:     streams.Stdin != nil,
			Stdout:    streams.Stdout != nil,
			Stderr:    !streams.TTY,
			TTY:       streams.TTY,
		}, scheme.ParameterCodec)

	url := req.URL()

	spdyExec, err := remotecommand.NewSPDYExecutor(c.Config, "POST", url)
	if err != nil {
		return err
	}

	return spdyExec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             streams.Stdin,
		Stdout:            streams.Stdout,
		Stderr:            streams.Stderr,
		Tty:               streams.TTY,
		TerminalSizeQueue: streams.TerminalSizeQueue,
	})
}

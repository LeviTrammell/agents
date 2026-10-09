package k8sctl

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
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
// streams until it exits or ctx is cancelled. It prefers the WebSocket
// executor and falls back to SPDY on upgrade failure, per the task's
// instructions (newer kubelets speak both; this keeps the gateway working
// against older ones too).
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

	executor := spdyExec
	if wsExec, err := remotecommand.NewWebSocketExecutor(c.Config, "GET", url.String()); err == nil {
		if fb, err := remotecommand.NewFallbackExecutor(wsExec, spdyExec, func(err error) bool {
			return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
		}); err == nil {
			executor = fb
		}
	}

	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             streams.Stdin,
		Stdout:            streams.Stdout,
		Stderr:            streams.Stderr,
		Tty:               streams.TTY,
		TerminalSizeQueue: streams.TerminalSizeQueue,
	})
}

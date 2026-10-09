package main

import (
	"context"
	"io"
	"log/slog"

	sshlib "charm.land/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/levitrammell/agents/gateway-app/internal/agents"
	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

// localForwardChannelData mirrors the RFC4254 7.2 direct-tcpip payload
// (charm.land/ssh's own copy of this struct is unexported, so direct-tcpip
// channels need their own decoder to implement a custom handler).
type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// newLocalPortForwardingCallback only allows forwarding to loopback
// destinations, and only for direct-agent SSH usernames: the menu/new
// usernames have no single Sandbox to forward into, and forwarding
// anywhere but loopback would turn the gateway into an open relay.
func newLocalPortForwardingCallback() sshlib.LocalPortForwardingCallback {
	return func(ctx sshlib.Context, destHost string, destPort uint32) bool {
		user := ctx.User()
		if !agents.AllowForwardUser(user) {
			slog.Warn("rejected port forward: not a direct-agent user", "user", user)
			return false
		}
		if !agents.AllowForwardDestination(destHost) {
			slog.Warn("rejected port forward: non-loopback destination", "user", user, "dest", destHost)
			return false
		}
		return true
	}
}

// newDirectTCPIPHandler returns a ssh.ChannelHandler for the
// "direct-tcpip" channel type that behaves like charm.land/ssh's own
// DirectTCPIPHandler, except the "dial" is a tunnel into the target
// agent's Sandbox pod via pods/portforward instead of a raw net.Dial --
// so a destination of 127.0.0.1 means loopback inside the pod, which is
// what makes `ssh -L 3000:localhost:3000 myagent@agents` reach a server
// that only bound 127.0.0.1 in the sandbox.
func newDirectTCPIPHandler(clients *k8sctl.Clients, forwarder *k8sctl.PodForwarder, cfg Config) sshlib.ChannelHandler {
	return func(srv *sshlib.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, ctx sshlib.Context) {
		d := localForwardChannelData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, "error parsing forward data: "+err.Error())
			return
		}

		if srv.LocalPortForwardingCallback == nil || !srv.LocalPortForwardingCallback(ctx, d.DestAddr, d.DestPort) {
			_ = newChan.Reject(gossh.Prohibited, "port forwarding is disabled")
			return
		}

		user := ctx.User()
		agentName := user // AllowForwardUser already proved this is a direct-agent username

		ch, reqs, err := newChan.Accept()
		if err != nil {
			return
		}
		go gossh.DiscardRequests(reqs)

		go func() {
			defer ch.Close()
			if err := forwardToAgent(ctx, clients, forwarder, cfg.Namespace, agentName, uint16(d.DestPort), ch); err != nil {
				slog.Warn("port forward ended with error", "user", user, "port", d.DestPort, "error", err)
			}
		}()
	}
}

// forwardToAgent resumes the agent's Sandbox if needed, then tunnels the
// direct-tcpip channel into the pod on remotePort until either side
// closes.
func forwardToAgent(ctx context.Context, clients *k8sctl.Clients, forwarder *k8sctl.PodForwarder, namespace, agentName string, remotePort uint16, ch io.ReadWriteCloser) error {
	pod, err := resumeAndWaitReady(ctx, clients, namespace, agentName, io.Discard)
	if err != nil {
		return err
	}
	return forwarder.Forward(namespace, pod, remotePort, ch)
}

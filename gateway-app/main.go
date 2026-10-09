// Command agent-ssh is the SSH gateway for the homelab coding-agent
// platform: it replaces the shell-based sshd+bin/agent gateway with a
// native Go server (charmbracelet/wish) that talks to the cluster
// directly, serves a Bubble Tea menu, and tunnels `ssh -L` port forwards
// into each agent's Sandbox pod.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	sshlib "charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/logging"

	"github.com/levitrammell/agents/gateway-app/internal/k8sctl"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("agent-ssh exiting", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()

	restCfg, err := k8sctl.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading kubernetes config: %w", err)
	}
	clients, err := k8sctl.NewClients(restCfg)
	if err != nil {
		return fmt.Errorf("building kubernetes clients: %w", err)
	}
	forwarder := k8sctl.NewPodForwarder(clients)

	signers, err := loadHostSigners(cfg.HostKeyDir)
	if err != nil {
		return fmt.Errorf("loading host keys: %w", err)
	}

	authKeys := newAuthorizedKeysChecker(cfg.AuthorizedKeysPath)

	server := &sshlib.Server{
		Addr: cfg.ListenAddr,
		PublicKeyHandler: func(ctx sshlib.Context, key sshlib.PublicKey) bool {
			allowed := authKeys.Allowed(key)
			slog.Info("public key auth", "user", ctx.User(), "allowed", allowed)
			return allowed
		},
		LocalPortForwardingCallback: newLocalPortForwardingCallback(),
		ChannelHandlers: map[string]sshlib.ChannelHandler{
			"session":      sshlib.DefaultSessionHandler,
			"direct-tcpip": newDirectTCPIPHandler(clients, forwarder, cfg),
		},
	}
	for _, signer := range signers {
		server.AddHostKey(signer)
	}

	// Session middleware first (innermost: it's the real work) then
	// logging last (outermost: wraps the whole session so it can log
	// connect/disconnect around it). See WithMiddleware's doc comment:
	// middlewares are composed from first to last, so the last one runs
	// first/outermost.
	if err := server.SetOption(wish.WithMiddleware(
		newSessionHandler(clients, cfg),
		logging.StructuredMiddleware(),
	)); err != nil {
		return fmt.Errorf("configuring middleware: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("agent-ssh listening", "addr", cfg.ListenAddr, "namespace", cfg.Namespace, "image", cfg.Image)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != sshlib.ErrServerClosed {
			return fmt.Errorf("serving: %w", err)
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		return nil
	}
}

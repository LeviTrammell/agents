package main

import (
	"flag"
	"os"
)

// Config holds the gateway's env/flag-configurable settings. Flags take
// the same names as the env vars (lowercased with dashes) so either works;
// env vars are the default since that's how the Deployment configures it.
type Config struct {
	Namespace          string
	Image              string
	ListenAddr         string
	AuthorizedKeysPath string
	HostKeyDir         string
	DebugEcho          bool // if set, "herdr" is replaced with "echo ok" for testing without a real herdr binary
}

func loadConfig() Config {
	cfg := Config{
		Namespace:          getenv("AGENTS_NAMESPACE", "agents"),
		Image:              getenv("AGENT_IMAGE", "ghcr.io/levitrammell/agent-coding:latest"),
		ListenAddr:         getenv("LISTEN_ADDR", ":2222"),
		AuthorizedKeysPath: getenv("AUTHORIZED_KEYS_PATH", "/etc/agent-gateway/authorized-keys/authorized_keys"),
		HostKeyDir:         getenv("HOST_KEY_DIR", "/etc/ssh/hostkeys"),
	}

	flag.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "Kubernetes namespace for Sandboxes (AGENTS_NAMESPACE)")
	flag.StringVar(&cfg.Image, "image", cfg.Image, "default image for new Sandboxes (AGENT_IMAGE)")
	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "SSH listen address (LISTEN_ADDR)")
	flag.StringVar(&cfg.AuthorizedKeysPath, "authorized-keys", cfg.AuthorizedKeysPath, "path to authorized_keys (AUTHORIZED_KEYS_PATH)")
	flag.StringVar(&cfg.HostKeyDir, "host-key-dir", cfg.HostKeyDir, "directory holding ssh_host_ed25519_key/ssh_host_rsa_key (HOST_KEY_DIR)")
	flag.BoolVar(&cfg.DebugEcho, "debug-echo-attach", false, "replace herdr with 'echo ok' for attach, for testing without a herdr binary")
	flag.Parse()

	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

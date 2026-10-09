// Package k8sctl wraps the Kubernetes client-go pieces the gateway needs:
// a dynamic client for Sandbox objects (which have no generated Go types
// here), a typed clientset for Pods/exec/portforward, and the raw
// rest.Config for building SPDY/WebSocket upgrades by hand.
package k8sctl

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Clients bundles everything the gateway needs to talk to the cluster.
type Clients struct {
	Config    *rest.Config
	Dynamic   dynamic.Interface
	Clientset kubernetes.Interface
}

// LoadConfig returns an in-cluster config when running in a pod, otherwise
// falls back to $KUBECONFIG or ~/.kube/config, for local testing.
func LoadConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("no in-cluster config and could not determine home directory for kubeconfig: %w", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig %s: %w", kubeconfig, err)
	}
	return cfg, nil
}

// NewClients builds a Clients bundle from a rest.Config.
func NewClients(cfg *rest.Config) (*Clients, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building dynamic client: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building clientset: %w", err)
	}
	return &Clients{Config: cfg, Dynamic: dyn, Clientset: cs}, nil
}

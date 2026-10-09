package k8sctl

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/levitrammell/agents/gateway-app/internal/agents"
)

// SandboxGVR is the GroupVersionResource for agent-sandbox Sandboxes.
var SandboxGVR = schema.GroupVersionResource{
	Group:    agents.SandboxGroup,
	Version:  agents.SandboxVersion,
	Resource: agents.SandboxResource,
}

// AgentInfo is the Sandbox state the menu and routing logic need: enough to
// list, decide readiness, and render "NAME MODE READY NODE AGE".
type AgentInfo struct {
	Name      string // bare agent name, without the "agent-" prefix
	Mode      string // spec.operatingMode: Running or Suspended
	Ready     string // status.conditions[Ready].status, or "Unknown"
	Node      string // pod's spec.nodeName, or "" if no pod
	CreatedAt time.Time
}

// GetSandbox fetches the Sandbox named agent-<name>, or
// apierrors.IsNotFound(err) if it doesn't exist.
func (c *Clients) GetSandbox(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(SandboxGVR).Namespace(namespace).Get(ctx, agents.SandboxName(name), metav1.GetOptions{})
}

// SandboxExists reports whether the Sandbox named agent-<name> exists.
func (c *Clients) SandboxExists(ctx context.Context, namespace, name string) (bool, error) {
	_, err := c.GetSandbox(ctx, namespace, name)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// CreateSandbox creates the Sandbox for spec and returns the created
// object.
func (c *Clients) CreateSandbox(ctx context.Context, spec agents.SandboxSpec) (*unstructured.Unstructured, error) {
	obj := agents.RenderSandbox(spec)
	return c.Dynamic.Resource(SandboxGVR).Namespace(spec.Namespace).Create(ctx, obj, metav1.CreateOptions{})
}

// SetOperatingMode patches spec.operatingMode to "Running" or "Suspended",
// the same merge patch cmd_stop/cmd_start use in bin/agent.
func (c *Clients) SetOperatingMode(ctx context.Context, namespace, name, mode string) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"operatingMode":%q}}`, mode))
	_, err := c.Dynamic.Resource(SandboxGVR).Namespace(namespace).Patch(ctx, agents.SandboxName(name), types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

// DeleteSandbox deletes the Sandbox named agent-<name> (and, via owner
// references managed by the agent-sandbox controller, its home PVC).
func (c *Clients) DeleteSandbox(ctx context.Context, namespace, name string) error {
	return c.Dynamic.Resource(SandboxGVR).Namespace(namespace).Delete(ctx, agents.SandboxName(name), metav1.DeleteOptions{})
}

// WaitReady polls the Sandbox until its Ready condition is True, or ctx is
// done. onTick, if non-nil, is called after every poll with the current
// Ready status string, for progress UIs.
func (c *Clients) WaitReady(ctx context.Context, namespace, name string, onTick func(status string)) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		obj, err := c.GetSandbox(ctx, namespace, name)
		if err != nil {
			return fmt.Errorf("getting sandbox agent-%s: %w", name, err)
		}
		status := readyStatus(obj)
		if onTick != nil {
			onTick(status)
		}
		if status == "True" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// readyStatus reads status.conditions[type=Ready].status from a Sandbox,
// or "Unknown" if absent.
func readyStatus(obj *unstructured.Unstructured) string {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return "Unknown"
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _, _ := unstructured.NestedString(cond, "type"); t == "Ready" {
			if s, _, _ := unstructured.NestedString(cond, "status"); s != "" {
				return s
			}
			return "Unknown"
		}
	}
	return "Unknown"
}

// ListAgents lists every agent-sandbox Sandbox labelled
// app.kubernetes.io/part-of=agents and the node its pod (if any) is
// scheduled on, sorted by name.
func (c *Clients) ListAgents(ctx context.Context, namespace string) ([]AgentInfo, error) {
	list, err := c.Dynamic.Resource(SandboxGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=agents",
	})
	if err != nil {
		return nil, fmt.Errorf("listing sandboxes: %w", err)
	}

	infos := make([]AgentInfo, 0, len(list.Items))
	for _, obj := range list.Items {
		fullName := obj.GetName()
		name := fullName
		if len(fullName) > len("agent-") && fullName[:len("agent-")] == "agent-" {
			name = fullName[len("agent-"):]
		}
		mode, _, _ := unstructured.NestedString(obj.Object, "spec", "operatingMode")
		if mode == "" {
			mode = "Unknown"
		}
		info := AgentInfo{
			Name:      name,
			Mode:      mode,
			Ready:     readyStatus(&obj),
			CreatedAt: obj.GetCreationTimestamp().Time,
		}

		pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, fullName, metav1.GetOptions{})
		if err == nil {
			info.Node = pod.Spec.NodeName
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// PodReady reports whether the agent's pod container is running (used to
// decide whether an exec attempt is worth making beyond the Sandbox Ready
// condition).
func PodPhase(pod *corev1.Pod) string {
	return string(pod.Status.Phase)
}

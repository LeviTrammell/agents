package agents

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SandboxGroupVersion is the agent-sandbox API group/version used for every
// Sandbox object.
const (
	SandboxGroup    = "agents.x-k8s.io"
	SandboxVersion  = "v1beta1"
	SandboxResource = "sandboxes"
	SandboxKind     = "Sandbox"
)

// Default resource sizing for a new Sandbox, matching cmd_new's defaults in
// services/agents/bin/agent.
const (
	DefaultCPURequest    = "500m"
	DefaultMemoryRequest = "1Gi"
	DefaultCPULimit      = "4"
	DefaultMemoryLimit   = "6Gi"
	DefaultDisk          = "20Gi"
	DefaultImage         = "ghcr.io/levitrammell/agent-coding:latest"
)

// SandboxSpec carries the knobs render_sandbox in bin/agent accepts. The
// gateway only ever uses the defaults (it has no per-agent sizing flags),
// but the struct keeps RenderSandbox testable against arbitrary values.
type SandboxSpec struct {
	Name          string
	Namespace     string
	Image         string
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
	Disk          string
}

// DefaultSandboxSpec returns a SandboxSpec with every sizing field at
// bin/agent's default, for the given agent name/namespace/image.
func DefaultSandboxSpec(name, namespace, image string) SandboxSpec {
	return SandboxSpec{
		Name:          name,
		Namespace:     namespace,
		Image:         image,
		CPURequest:    DefaultCPURequest,
		MemoryRequest: DefaultMemoryRequest,
		CPULimit:      DefaultCPULimit,
		MemoryLimit:   DefaultMemoryLimit,
		Disk:          DefaultDisk,
	}
}

// RenderSandbox builds the Sandbox object for spec, in the same shape as
// render_sandbox() in services/agents/bin/agent: same apiVersion/kind,
// labels, operatingMode, pod securityContext, container securityContext,
// envFrom, resources, volumeMounts, and volumeClaimTemplates.
func RenderSandbox(spec SandboxSpec) *unstructured.Unstructured {
	sandboxName := SandboxName(spec.Name)
	labels := map[string]interface{}{
		"app.kubernetes.io/part-of": "agents",
		"agents.ima.dev/name":       spec.Name,
	}

	obj := map[string]interface{}{
		"apiVersion": SandboxGroup + "/" + SandboxVersion,
		"kind":       SandboxKind,
		"metadata": map[string]interface{}{
			"name":      sandboxName,
			"namespace": spec.Namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"operatingMode": "Running",
			"podTemplate": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": labels,
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Always",
					"securityContext": map[string]interface{}{
						"runAsUser":  int64(1000),
						"runAsGroup": int64(1000),
						"fsGroup":    int64(1000),
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":            "agent",
							"image":           spec.Image,
							"imagePullPolicy": "Always",
							"securityContext": map[string]interface{}{
								"runAsUser":  int64(1000),
								"runAsGroup": int64(1000),
							},
							"envFrom": []interface{}{
								map[string]interface{}{
									"secretRef": map[string]interface{}{
										"name":     "agent-credentials",
										"optional": true,
									},
								},
							},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    spec.CPURequest,
									"memory": spec.MemoryRequest,
								},
								"limits": map[string]interface{}{
									"cpu":    spec.CPULimit,
									"memory": spec.MemoryLimit,
								},
							},
							"volumeMounts": []interface{}{
								map[string]interface{}{
									"name":      "home",
									"mountPath": "/home/agent",
								},
							},
						},
					},
				},
			},
			"volumeClaimTemplates": []interface{}{
				map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "home",
					},
					"spec": map[string]interface{}{
						"storageClassName": "longhorn",
						"accessModes":      []interface{}{"ReadWriteOnce"},
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"storage": spec.Disk,
							},
						},
					},
				},
			},
		},
	}

	return &unstructured.Unstructured{Object: obj}
}

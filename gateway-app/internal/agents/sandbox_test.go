package agents

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"sigs.k8s.io/yaml"
)

// agentScriptPath locates bin/agent relative to this test
// file, so the test works regardless of the working directory it's run
// from.
func agentScriptPath(t *testing.T) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// this file is gateway-app/internal/agents/sandbox_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "bin", "agent")
}

// normalize round-trips a value through JSON so that map[string]interface{}
// vs map[string]any and number types line up for reflect.DeepEqual,
// regardless of how each side was constructed.
func normalize(t *testing.T, v interface{}) interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestRenderSandboxParity checks that RenderSandbox's output matches, field
// for field, the manifest services/agents/bin/agent prints for `agent new
// <name> --dry-run`. --dry-run never talks to the cluster, so this runs
// without a kubeconfig.
func TestRenderSandboxParity(t *testing.T) {
	script := agentScriptPath(t)
	name := "wishtest-parity"

	cmd := exec.Command("bash", script, "new", name, "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bin/agent new %s --dry-run: %v\n%s", name, err, out)
	}

	var bashManifest map[string]interface{}
	if err := yaml.Unmarshal(out, &bashManifest); err != nil {
		t.Fatalf("parsing bin/agent output as YAML: %v\n%s", err, out)
	}

	spec := DefaultSandboxSpec(name, "agents", DefaultImage)
	goManifest := RenderSandbox(spec).Object

	got := normalize(t, goManifest)
	want := normalize(t, bashManifest)

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("RenderSandbox output does not match bin/agent --dry-run.\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

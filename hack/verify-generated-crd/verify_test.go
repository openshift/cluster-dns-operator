package verifygeneratedcrd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const script = "../verify-generated-crd.sh"

// runGuard runs hack/verify-generated-crd.sh against dir and returns its exit
// code and combined output.
func runGuard(t *testing.T, dir string) (int, string) {
	t.Helper()
	out, err := exec.Command("bash", script, dir).CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("failed to run %s: %v", script, err)
	}
	t.Logf("%s output:\n%s", script, out)
	return exitErr.ExitCode(), string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGeneratedCRDManifests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     string
		manifest string
		want     int
	}{
		{
			name: "dnses CRD",
			file: "crd.yaml",
			manifest: `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: dnses.operator.openshift.io
`,
			want: 1,
		},
		{
			name: "dnsnameresolvers CRD",
			file: "crd.yaml",
			manifest: `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: dnsnameresolvers.network.openshift.io
`,
			want: 1,
		},
		{
			name: "dnses CRD name on the next line",
			file: "crd.yaml",
			manifest: `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name:
    dnses.operator.openshift.io
`,
			want: 1,
		},
		{
			name: "dnsnameresolvers CRD name on the next line",
			file: "crd.yml",
			manifest: `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name:
    dnsnameresolvers.network.openshift.io
`,
			want: 1,
		},
		{
			name: "quoted name with a comment",
			file: "crd.yaml",
			manifest: `kind: "CustomResourceDefinition"
metadata:
  name: "dnses.operator.openshift.io" # comment
`,
			want: 1,
		},
		{
			name: "folded block scalar name",
			file: "crd.yaml",
			manifest: `kind: CustomResourceDefinition
metadata:
  name: >-
    dnsnameresolvers.network.openshift.io
`,
			want: 1,
		},
		{
			name: "tagged name",
			file: "crd.yaml",
			manifest: `kind: CustomResourceDefinition
metadata:
  name: !!str dnses.operator.openshift.io
`,
			want: 1,
		},
		{
			name:     "CRLF line endings",
			file:     "crd.yaml",
			manifest: "kind: CustomResourceDefinition\r\nmetadata:\r\n  name: dnses.operator.openshift.io\r\n",
			want:     1,
		},
		{
			name:     "flow-style YAML",
			file:     "crd.yaml",
			manifest: "{apiVersion: apiextensions.k8s.io/v1, kind: CustomResourceDefinition, metadata: {name: dnses.operator.openshift.io}}\n",
			want:     1,
		},
		{
			name:     "JSON",
			file:     "crd.json",
			manifest: `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"dnsnameresolvers.network.openshift.io"}}`,
			want:     1,
		},
		{
			name: "List",
			file: "list.yaml",
			manifest: `apiVersion: v1
kind: List
items:
- apiVersion: apiextensions.k8s.io/v1
  kind: CustomResourceDefinition
  metadata:
    name: dnses.operator.openshift.io
`,
			want: 1,
		},
		{
			name: "unrelated manifest",
			file: "configmap.yaml",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: dns-default
  namespace: openshift-dns
`,
			want: 0,
		},
		{
			name: "RBAC rule for the dnses resource",
			file: "role.yaml",
			manifest: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openshift-dns-operator
rules:
- apiGroups:
  - operator.openshift.io
  resources:
  - dnses
  verbs:
  - get
`,
			want: 0,
		},
		{
			name: "names that embed a CRD name",
			file: "configmap.yaml",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: xdnses.operator.openshift.io
data:
  a: dnses.operator.openshift.io.example
  b: my-dnsnameresolvers.network.openshift.io
  c: dnses.operator.openshift.io_backup
`,
			want: 0,
		},
		{
			name:     "file that is not a manifest",
			file:     "notes.txt",
			manifest: "dnses.operator.openshift.io\n",
			want:     0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, tc.file)
			writeFile(t, path, tc.manifest)
			got, out := runGuard(t, dir)
			if got != tc.want {
				t.Errorf("exit code = %d, want %d", got, tc.want)
			}
			if tc.want == 1 && !strings.Contains(out, path) {
				t.Errorf("output does not report %s:\n%s", path, out)
			}
		})
	}
}

func TestVerifyGeneratedCRDNestedDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, "crd.yaml"), "metadata:\n  name: dnses.operator.openshift.io\n")
	if got, _ := runGuard(t, dir); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
}

func TestVerifyGeneratedCRDRepositoryManifests(t *testing.T) {
	t.Parallel()
	if got, _ := runGuard(t, filepath.Join("..", "..", "manifests")); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestVerifyGeneratedCRDEmptyDirectory(t *testing.T) {
	t.Parallel()
	if got, _ := runGuard(t, t.TempDir()); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestVerifyGeneratedCRDMissingDirectory(t *testing.T) {
	t.Parallel()
	if got, _ := runGuard(t, filepath.Join(t.TempDir(), "missing")); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

func TestVerifyGeneratedCRDUnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "crd.yaml")
	writeFile(t, path, "metadata:\n  name: dnses.operator.openshift.io\n")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if got, _ := runGuard(t, dir); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

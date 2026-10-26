//go:build e2e

// Package e2e runs the real doriki binary against a CPU kind cluster serving llama.cpp
// (DESIGN.md section 9.1). Run it with make e2e; it needs kind and a running Docker, and skips
// without them unless DORIKI_E2E_REQUIRED=1 (CI), where it fails instead. It creates the kind
// cluster "doriki" when absent and leaves it for the next run; it deletes the llama.cpp
// Deployment when done. It reads the cluster's kubeconfig into a temporary file, so no
// kubeconfig of the user's is touched.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/schema"
)

const (
	clusterName = "doriki"
	namespace   = "doriki"
	deployment  = "llamacpp"
	account     = "doriki-e2e" // bound to deploy/rbac.yaml; doriki runs as it
	root        = "../../"
	manifest    = root + "packs/llamacpp/testdata/11889/llamacpp.yaml" // the fixture's own manifest
	rbacFile    = root + "deploy/rbac.yaml"
)

// prerequisites skips, or in CI fails, when kind or Docker is not there.
func prerequisites(t *testing.T) {
	t.Helper()
	var missing []string
	if _, err := exec.LookPath("kind"); err != nil {
		missing = append(missing, "kind")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		missing = append(missing, "a running Docker")
	}
	if len(missing) == 0 {
		return
	}
	msg := "e2e: needs " + strings.Join(missing, " and ") + " (see PROJECT.md section 9)"
	if os.Getenv("DORIKI_E2E_REQUIRED") == "1" {
		t.Fatal(msg + "; DORIKI_E2E_REQUIRED=1, so this is a failure, not a skip")
	}
	t.Skip(msg + "; skipped")
}

// step runs one stage and logs how long it took, for the time budget.
func step(t *testing.T, name string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	t.Logf("step %-34s %6.1fs", name, time.Since(start).Seconds())
}

func run(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput() // #nosec G204 -- fixed tools, test arguments
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

// cluster makes sure the kind cluster exists and returns its kubeconfig file and whether this
// run created it (then the node has never pulled the llama.cpp image).
func cluster(t *testing.T) (kubeconfig string, created bool) {
	t.Helper()
	kubeconfig = filepath.Join(t.TempDir(), "kubeconfig")
	exists := false
	for _, c := range strings.Fields(string(run(t, "kind", "get", "clusters"))) {
		exists = exists || c == clusterName
	}
	if !exists {
		run(t, "kind", "create", "cluster", "--name", clusterName, "--kubeconfig", kubeconfig, "--wait", "120s")
		return kubeconfig, true
	}
	if err := os.WriteFile(kubeconfig, run(t, "kind", "get", "kubeconfig", "--name", clusterName), 0o600); err != nil {
		t.Fatal(err)
	}
	return kubeconfig, false
}

// objects decodes a multi-document YAML file into typed objects.
func objects(t *testing.T, path string) []runtime.Object {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- a repository file
	if err != nil {
		t.Fatal(err)
	}
	var out []runtime.Object
	for _, doc := range strings.Split(string(b), "\n---") {
		if strings.TrimSpace(stripComments(doc)) == "" {
			continue
		}
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, obj)
	}
	return out
}

func stripComments(doc string) string {
	var lines []string
	for _, l := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// apply creates each object, or replaces it when it exists. The cluster is a test fixture:
// this is the test setting it up, not doriki.
func apply(ctx context.Context, t *testing.T, api kubernetes.Interface, objs ...runtime.Object) {
	t.Helper()
	for _, obj := range objs {
		var err error
		switch o := obj.(type) {
		case *corev1.Namespace:
			_, err = api.CoreV1().Namespaces().Create(ctx, o, metav1.CreateOptions{})
		case *corev1.ServiceAccount:
			_, err = api.CoreV1().ServiceAccounts(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		case *rbacv1.ClusterRole:
			if _, err = api.RbacV1().ClusterRoles().Create(ctx, o, metav1.CreateOptions{}); apierrors.IsAlreadyExists(err) {
				_, err = api.RbacV1().ClusterRoles().Update(ctx, o, metav1.UpdateOptions{})
			}
		case *rbacv1.ClusterRoleBinding:
			if _, err = api.RbacV1().ClusterRoleBindings().Create(ctx, o, metav1.CreateOptions{}); apierrors.IsAlreadyExists(err) {
				_, err = api.RbacV1().ClusterRoleBindings().Update(ctx, o, metav1.UpdateOptions{})
			}
		case *appsv1.Deployment:
			_, err = api.AppsV1().Deployments(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		default:
			t.Fatalf("apply: unexpected %T", obj)
		}
		if err != nil && !apierrors.IsAlreadyExists(err) {
			t.Fatalf("apply %T: %v", obj, err)
		}
	}
}

// removeDeployment deletes the llama.cpp Deployment and waits until its pods are gone, so the
// next apply is a fresh boot even on a reused cluster.
func removeDeployment(ctx context.Context, t *testing.T, api kubernetes.Interface) {
	t.Helper()
	err := api.AppsV1().Deployments(namespace).Delete(ctx, deployment, metav1.DeleteOptions{PropagationPolicy: new(metav1.DeletePropagationForeground)})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("delete deployment: %v", err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		pods, err := api.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment})
		if err != nil {
			t.Fatalf("list pods: %v", err)
		}
		if len(pods.Items) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d llama.cpp pods still there after 3 minutes", len(pods.Items))
		}
		time.Sleep(2 * time.Second)
	}
}

// doriki runs the real binary as the read-only account and returns stdout and the exit code.
func doriki(t *testing.T, kubeconfig string, args ...string) ([]byte, int) {
	t.Helper()
	bin := os.Getenv("DORIKI_BIN")
	if bin == "" {
		t.Fatal("DORIKI_BIN is not set; run make e2e")
	}
	args = append(args, "-n", namespace, "--kubeconfig", kubeconfig, "--as", "system:serviceaccount:"+namespace+":"+account)
	cmd := exec.Command(bin, args...) // #nosec G204 G702 -- the binary under test, from DORIKI_BIN
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return stdout.Bytes(), 0
	case errors.As(err, &exit):
		t.Logf("doriki %s: exit %d, stderr: %s", strings.Join(args, " "), exit.ExitCode(), stderr.String())
		return stdout.Bytes(), exit.ExitCode()
	}
	t.Fatalf("doriki %s: %v", strings.Join(args, " "), err)
	return nil, 0
}

// validate checks a report against schema/report.v1.json, the way the schema test does.
func validate(t *testing.T, report []byte) {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	for name := range schema.Files {
		f, err := os.Open(root + "schema/" + name) // #nosec G304 -- a schema file name from schema.Files
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AddResource(schema.BaseURL+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	s, err := c.Compile(schema.BaseURL + "report.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(report))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(inst); err != nil {
		t.Errorf("report does not validate against report.v1.json: %v", err)
	}
}

// checkBoot asserts what a healthy llama.cpp boot shows (M2 task acceptance 9, DESIGN.md
// section 11 M2).
func checkBoot(t *testing.T, what string, r model.Report, coldNode bool) {
	t.Helper()
	states := map[model.PhaseName]model.Phase{}
	for _, p := range r.Phases {
		states[p.Name] = p
	}
	for _, name := range []model.PhaseName{model.PhaseImagePull, model.PhaseEngineInit, model.PhaseWeightFetch, model.PhaseWeightLoad, model.PhaseServerReady} {
		if states[name].State != model.StateComplete {
			t.Errorf("%s: %s is %s, want complete", what, name, states[name].State)
		}
	}
	for _, name := range []model.PhaseName{model.PhaseCompile, model.PhaseGraphCapture} {
		if p := states[name]; p.State != model.StateSkipped || p.Attrs[model.AttrNotApplicable] != "true" {
			t.Errorf("%s: %s = %+v, want skipped as not applicable", what, name, p)
		}
	}
	// A cluster this run created has never had the image, so the pull is real.
	if coldNode && states[model.PhaseImagePull].Attrs[model.AttrCached] == "true" {
		t.Errorf("%s: image-pull is cached on a new cluster; the node must pull from ghcr", what)
	}
	if total := r.Totals.PendingToReadySeconds; total == nil || *total <= 0 {
		t.Errorf("%s: pending_to_ready = %v, want > 0", what, total)
	}
	if r.Manifest == nil || r.Manifest.Engine == nil || r.Manifest.Engine.ParserPack != "llamacpp/11889" {
		t.Errorf("%s: parser pack %+v, want llamacpp/11889", what, r.Manifest)
	}
}

func decode(t *testing.T, what string, out []byte) model.Report {
	t.Helper()
	var r model.Report
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("%s: not a report: %v\n%s", what, err, out)
	}
	return r
}

// TestLlamaCppLive boots llama.cpp on kind and profiles it live, as the read-only account: with
// --follow from the moment it is applied, then one-shot by pod name, then as text.
func TestLlamaCppLive(t *testing.T) {
	prerequisites(t)
	ctx := context.Background()

	var kubeconfig string
	var created bool
	step(t, "cluster (create or reuse)", func() { kubeconfig, created = cluster(t) })
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var deploy *appsv1.Deployment
	step(t, "namespace, RBAC, account", func() {
		var rest []runtime.Object
		for _, obj := range objects(t, manifest) {
			if d, ok := obj.(*appsv1.Deployment); ok {
				deploy = d
				continue
			}
			rest = append(rest, obj)
		}
		rest = append(rest, objects(t, rbacFile)...)
		rest = append(rest,
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: account, Namespace: namespace}},
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: account},
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "doriki-reader"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: account, Namespace: namespace}},
			})
		apply(ctx, t, api, rest...)
	})
	if deploy == nil {
		t.Fatalf("%s has no Deployment", manifest)
	}
	step(t, "remove previous deployment", func() { removeDeployment(ctx, t, api) })
	t.Cleanup(func() { removeDeployment(context.Background(), t, api) })

	var follow []byte
	var followCode int
	step(t, "apply + doriki --follow until ready", func() {
		apply(ctx, t, api, deploy)
		follow, followCode = doriki(t, kubeconfig, "deploy/"+deployment, "--follow", "--timeout", "10m", "-o", "json")
	})
	if followCode != 0 {
		t.Fatalf("doriki --follow exited %d:\n%s", followCode, follow)
	}
	validate(t, follow)
	followed := decode(t, "--follow", follow)
	checkBoot(t, "--follow", followed, created)

	step(t, "doriki pod/<name> one-shot", func() {
		out, code := doriki(t, kubeconfig, "pod/"+followed.Manifest.Pod.Name, "-o", "json")
		if code != 0 {
			t.Fatalf("one-shot exited %d:\n%s", code, out)
		}
		validate(t, out)
		once := decode(t, "one-shot", out)
		checkBoot(t, "one-shot", once, created)
		if !reflect.DeepEqual(once.Phases, followed.Phases) {
			t.Errorf("one-shot phases differ from --follow's:\n one-shot %+v\n follow   %+v", once.Phases, followed.Phases)
		}
	})

	step(t, "doriki deploy/<name> text", func() {
		out, code := doriki(t, kubeconfig, "deploy/"+deployment)
		if code != 0 || !strings.Contains(string(out), "server-ready") {
			t.Errorf("text exited %d, want 0 with a server-ready row:\n%s", code, out)
		}
		t.Logf("waterfall:\n%s", out)
	})
}

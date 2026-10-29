package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/collect/k8s"
	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
	"github.com/AmitwalaH/doriki/packs"
)

// The redaction levels capture accepts (DESIGN.md sections 7.2 and 10.3). There is no "none":
// a bundle is made to be shared.
const (
	redactDefault = "default"
	redactStrict  = "strict"
)

// lastApplied is kubectl apply's copy of the whole spec, env values included; a bundle never
// needs it.
const lastApplied = "kubectl.kubernetes.io/last-applied-configuration"

// captureSalt salts strict mode's name hashes, fresh for every capture and never written, so a
// short name cannot be recovered by hashing guesses. A variable so a test could fix it.
var captureSalt = rand.Text

func newCaptureCmd() *cobra.Command {
	var out, level string
	kube := genericclioptions.NewConfigFlags(true)
	cmd := &cobra.Command{
		Use:   "capture pod/<name> | deploy/<name> | sts/<name> --out <dir> [--redact-level default|strict]",
		Short: "Save a pod's objects and logs as a redacted bundle to share, replayed with doriki --from-files <dir>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCapture(cmd, args[0], kube, out, level)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "the directory to write the bundle to; it must not exist or be empty")
	cmd.Flags().StringVar(&level, "redact-level", redactDefault, "default (secrets removed) or strict (also pod, namespace, owner, node and image names hashed)")
	kube.AddFlags(cmd.Flags())
	return cmd
}

func runCapture(cmd *cobra.Command, arg string, kube *genericclioptions.ConfigFlags, out, level string) error {
	if level != redactDefault && level != redactStrict {
		return fmt.Errorf("--redact-level %q: capture takes default or strict", level)
	}
	if out == "" {
		return errors.New("capture needs --out <dir>")
	}
	if err := emptyDir(out); err != nil {
		return err
	}
	t, err := parseTarget(arg)
	if err != nil {
		return err
	}
	client, ns, err := liveClient(kube)
	if err != nil {
		return &ExitError{Code: ExitUnavailable, Err: err}
	}
	t.Namespace = ns
	o, err := collect(cmd.Context(), client, t, time.Time{})
	if err != nil {
		return &ExitError{Code: ExitUnavailable, Err: err}
	}

	b, err := redactObjects(o, level == redactStrict)
	if err != nil {
		return err
	}
	if level == redactStrict {
		h, err := newHasher(o, t.Name)
		if err != nil {
			return err
		}
		for name, body := range b.files {
			b.files[name] = []byte(h.apply(string(body)))
		}
		b.target, b.images = string(t.Kind)+"/"+h.apply(t.Name), h.imageRules
	}
	for name, body := range b.files {
		if err := os.WriteFile(filepath.Join(out, name), body, 0o600); err != nil {
			return err
		}
	}

	// The report is built from the files just written, so the bundle is consistent with itself
	// and --from-files on it gives this very report.
	at := now()
	collected, warnings, err := files.LoadDir(out)
	if err != nil {
		return err
	}
	r, _, err := buildReport(collected, warnings, reportOptions{}, at)
	if err != nil {
		return err
	}
	var report bytes.Buffer
	if err := renderjson.Write(&report, r); err != nil {
		return err
	}
	m, err := json.MarshalIndent(r.Manifest, "", "  ")
	if err != nil {
		return err
	}
	written := map[string][]byte{
		"report.json":   report.Bytes(),
		"manifest.json": append(m, '\n'),
		"SOURCE.md":     []byte(sourceMD(b, level, r.Manifest.Engine.ParserPack, at)),
	}
	for name, body := range written {
		if err := os.WriteFile(filepath.Join(out, name), body, 0o600); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (redaction %s); replay with: doriki --from-files %s\n", out, level, out)
	return err
}

// emptyDir makes dir if it does not exist, and refuses one that already holds files, so a
// capture never mixes with or overwrites other files.
func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(dir, 0o750)
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("--out %s already holds files; capture writes only to a new or empty directory", dir)
	}
	return nil
}

// captured is the redacted bundle before it is written: file name to contents, and what
// SOURCE.md says about it.
type captured struct {
	files  map[string][]byte
	target string
	images []string // strict only: which rule each image got
}

// redactObjects applies the default level to everything the collector read: env outside the
// allowlist dropped and secrets redacted in every container, secret flags redacted in args and
// command, kubectl's last-applied copy and managedFields removed, the node cut to what doriki
// reads, every token-shaped string in every object and log line redacted. strict also drops
// the pod's and node's IP addresses.
func redactObjects(o k8s.Objects, strict bool) (captured, error) {
	pod := o.Pod.DeepCopy()
	pod.TypeMeta = metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"}
	delete(pod.Annotations, lastApplied)
	pod.ManagedFields = nil
	for i := range pod.Spec.InitContainers {
		redactContainer(&pod.Spec.InitContainers[i].Env, &pod.Spec.InitContainers[i].Args, &pod.Spec.InitContainers[i].Command)
	}
	for i := range pod.Spec.Containers {
		redactContainer(&pod.Spec.Containers[i].Env, &pod.Spec.Containers[i].Args, &pod.Spec.Containers[i].Command)
	}
	for i := range pod.Spec.EphemeralContainers {
		c := &pod.Spec.EphemeralContainers[i].EphemeralContainerCommon
		redactContainer(&c.Env, &c.Args, &c.Command)
	}
	if strict {
		pod.Status.PodIP, pod.Status.PodIPs, pod.Status.HostIP, pod.Status.HostIPs = "", nil, "", nil
	}

	objects := map[string]any{
		files.PodFile:    pod,
		files.EventsFile: corev1.EventList{TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "v1"}, Items: o.Events},
	}
	if o.Node != nil {
		// Only what the manifest reads: labels (instance type, zone, GPU product), the kubelet
		// version, capacity. Not the node's addresses, annotations or its other images, and not
		// the IDs that name the host (DK-CLI-BUG-20261029-capture-node-host-ids).
		info := o.Node.Status.NodeInfo
		info.MachineID, info.SystemUUID, info.BootID = "", "", ""
		objects[files.NodeFile] = corev1.Node{
			TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: o.Node.Name, Labels: o.Node.Labels},
			Status:     corev1.NodeStatus{NodeInfo: info, Capacity: o.Node.Status.Capacity, Allocatable: o.Node.Status.Allocatable},
		}
	}
	c := captured{files: map[string][]byte{}, target: ""}
	for name, v := range objects {
		b, err := json.Marshal(v)
		if err != nil {
			return captured{}, err
		}
		if c.files[name], err = manifest.RedactJSON(b); err != nil {
			return captured{}, err
		}
		c.files[name] = append(c.files[name], '\n')
	}
	c.files[files.LogFile] = []byte(manifest.RedactLine(string(o.Log)))
	if o.PreviousLog != nil {
		c.files[files.PreviousLogFile] = []byte(manifest.RedactLine(string(o.PreviousLog)))
	}
	return c, nil
}

// redactContainer applies the manifest's env and args rules to one container, in place.
func redactContainer(env *[]corev1.EnvVar, args, command *[]string) {
	kept := (*env)[:0]
	for _, e := range *env {
		v, keep := manifest.RedactEnv(e.Name, e.Value)
		if !keep {
			continue
		}
		if e.Value != "" {
			e.Value = v
		}
		kept = append(kept, e)
	}
	*env = kept
	if len(*args) > 0 {
		*args = manifest.RedactArgList(*args)
	}
	if len(*command) > 0 {
		*command = manifest.RedactArgList(*command)
	}
}

// nameToken is one name as it stands in text: Kubernetes names and image path parts are made
// of these characters, so a name is replaced only where it stands whole, never inside a longer
// word ("vllm" in "vllm-openai", "default" in "default_loader").
var nameToken = regexp.MustCompile(`[A-Za-z0-9._-]+`)

// hasher replaces names with "h-" and eight hex digits of their salted sha256, the same name
// with the same hash in every file of one capture.
type hasher struct {
	salt       string
	names      map[string]string
	imageRules []string
}

// newHasher collects the names strict mode hides: the pod, its generateName, namespace, owners
// and node, the target's name, and for every image its registry host and org path, and its
// name too unless a pack's image_regex matches it (owner decision, M3 slice 5), so pack
// selection still works on the bundle.
func newHasher(o k8s.Objects, target string) (*hasher, error) {
	h := &hasher{salt: captureSalt(), names: map[string]string{}}
	p := o.Pod
	h.add(p.Name, p.GenerateName, p.Namespace, p.Spec.NodeName, target)
	for _, ref := range p.OwnerReferences {
		h.add(ref.Name)
		if hash := p.Labels["pod-template-hash"]; ref.Kind == "ReplicaSet" && hash != "" {
			h.add(strings.TrimSuffix(ref.Name, "-"+hash))
		}
	}
	if _, walked, ok := strings.Cut(o.Owner, "/"); ok {
		h.add(walked)
	}
	if o.Node != nil {
		h.add(o.Node.Name)
	}

	all, err := packs.All()
	if err != nil {
		return nil, err
	}
	var images []string
	for _, c := range slices.Concat(p.Spec.InitContainers, p.Spec.Containers) {
		images = append(images, c.Image)
	}
	for _, s := range slices.Concat(p.Status.InitContainerStatuses, p.Status.ContainerStatuses) {
		images = append(images, s.Image, s.ImageID)
	}
	kept := map[string]string{}
	for _, img := range images {
		host, orgs, name := imageParts(img)
		h.add(host)
		h.add(orgs...)
		if name == "" {
			continue
		}
		if pack := packFor(all, name); pack != "" {
			kept[name] = pack
		} else {
			h.add(name)
		}
	}
	// A kept image name stays even if a workload has the same name: pack selection needs it.
	for name := range kept {
		delete(h.names, name)
	}
	seen := map[string]bool{}
	for _, c := range p.Spec.Containers {
		if seen[c.Image] {
			continue
		}
		seen[c.Image] = true
		_, _, name := imageParts(c.Image)
		rule := "registry, org and name hashed: no pack's image_regex matches the name"
		if pack, ok := kept[name]; ok {
			rule = "registry and org hashed, name kept: it matches pack " + pack + "'s image_regex"
		}
		h.imageRules = append(h.imageRules, "`"+h.apply(c.Image)+"`: "+rule)
	}
	return h, nil
}

func (h *hasher) add(names ...string) {
	for _, n := range names {
		if n == "" || !nameToken.MatchString(n) || nameToken.FindString(n) != n {
			continue
		}
		sum := sha256.Sum256([]byte(h.salt + n))
		h.names[n] = "h-" + hex.EncodeToString(sum[:])[:8]
	}
}

func (h *hasher) apply(s string) string {
	return nameToken.ReplaceAllStringFunc(s, func(tok string) string {
		if v, ok := h.names[tok]; ok {
			return v
		}
		return tok
	})
}

// imageParts splits an image reference into its registry host (empty when implicit), its org
// path and its name, without tag or digest. A bare digest (sha256:...) has none of them.
func imageParts(ref string) (host string, orgs []string, name string) {
	if bareDigest.MatchString(ref) {
		return "", nil, ""
	}
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	parts := strings.Split(ref, "/")
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		host, _, _ = strings.Cut(parts[0], ":") // the port stays: hashing "5000" would hit every 5000
		parts = parts[1:]
	}
	if ref == "" || len(parts) == 0 {
		return host, nil, ""
	}
	return host, parts[:len(parts)-1], parts[len(parts)-1]
}

// bareDigest is an image id with no name, as containerd reports status.image after a pull by
// digest ("sha256:3671…").
var bareDigest = regexp.MustCompile(`^[a-z0-9]+:[0-9a-f]{16,}$`)

// packFor is the first pack whose image_regex matches an image name, or "".
func packFor(all []model.Pack, name string) string {
	for _, p := range all {
		if p.Detect.ImageRegex == "" {
			continue
		}
		if re, err := regexp.Compile(p.Detect.ImageRegex); err == nil && re.MatchString(name) {
			return p.Name
		}
	}
	return ""
}

// sourceMD is the bundle's SOURCE.md: what made it and what was removed, so the person it is
// sent to knows what they hold (DESIGN.md section 10.3).
func sourceMD(c captured, level, pack string, at time.Time) string {
	target := c.target
	if target == "" {
		target = "the pod named on the command line"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# doriki capture\n\n")
	fmt.Fprintf(&b, "- doriki: %s (commit %s)\n- Captured: %s\n- Parser pack: %s\n- Redaction: %s\n\n",
		version, commit, at.UTC().Format(time.RFC3339), pack, level)
	fmt.Fprintf(&b, "## Commands\n\n```sh\ndoriki capture %s --out <dir> --redact-level %s\ndoriki --from-files <dir>   # replays this report\n```\n\n", target, level)
	b.WriteString(`## What was removed

Every level:
- Environment variables outside the allowlist (DESIGN.md section 4.3) are removed from every
  container; an allowlisted variable whose name says secret has its value replaced by
  ` + "`[redacted]`" + `.
- The value of a secret-named flag (` + "`--api-key x`, `--hf-token=x`" + `) in args and command.
- The ` + "`" + lastApplied + "`" + ` annotation and managedFields.
- From node.json, everything but its name, labels, kubelet and system info, and capacity;
  the system info without the machine ID, system UUID and boot ID.
- In every string of every file and every log line: values shaped like a known token (Hugging
  Face, OpenAI-style, GitHub, Slack, AWS access key ids, JWTs).

Not removed: a secret with no known token shape that the engine prints in its own log (for
example a password it echoes), and secret values inside a longer word.
`)
	if level == redactStrict {
		b.WriteString(`
Strict, in addition:
- The pod, namespace, owner and node names, the registry host and org path of each image are
  replaced by ` + "`h-`" + ` and eight hex digits of a salted sha256, the same in every file. The salt
  is new for each capture and not kept. A name is replaced only where it stands as a whole
  word.
- The pod's and node's IP addresses are removed.
- The manifest fingerprint in manifest.json and report.json is computed after redaction, so it
  differs from the fingerprint of a live report of the same pod.

Images:
`)
		for _, rule := range c.images {
			fmt.Fprintf(&b, "- %s\n", rule)
		}
	}
	return b.String()
}

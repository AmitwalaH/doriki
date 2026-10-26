package files

import (
	"encoding/json"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Well-known labels and resources the manifest reads (DESIGN.md sections 4.3 and 8.1). The
// beta labels are what older clusters still write.
var (
	instanceTypeLabels = []string{"node.kubernetes.io/instance-type", "beta.kubernetes.io/instance-type"}
	zoneLabels         = []string{"topology.kubernetes.io/zone", "failure-domain.beta.kubernetes.io/zone"}
)

const (
	nvidiaGPU         = corev1.ResourceName("nvidia.com/gpu")
	nvidiaProduct     = "nvidia.com/gpu.product" // GPU Feature Discovery
	podTemplateHash   = "pod-template-hash"      // set by the Deployment controller
	deploymentRSKind  = "ReplicaSet"
	deploymentKind    = "Deployment"
	acceleratorNvidia = "nvidia"
)

// objectFacts reads the manifest facts of the pod, its profiled container and, when given, its
// node: one fact event each, stamped with pod creation. Env facts hold literal values only
// (a valueFrom reference has none), unfiltered; manifest.Sanitize decides what is kept. Env
// and args values are never put in Raw.
func objectFacts(p *corev1.Pod, container string, node *corev1.Node) []model.Event {
	var out []model.Event
	add := func(key, value, raw string) {
		if value == "" {
			return
		}
		e := event(p.CreationTimestamp.Time, model.SourceManifest, model.KindFact, raw)
		e.Attrs = map[string]string{model.AttrFactKey: key, model.AttrFactValue: value}
		out = append(out, e)
	}

	add(model.FactPodUID, string(p.UID), "pod uid "+string(p.UID))
	if ref, ok := ownerRef(p); ok {
		add(model.FactPodOwnerRef, ref.Kind+"/"+ref.Name, "owner reference "+ref.Kind+"/"+ref.Name)
		owner := inferOwner(p, ref)
		add(model.FactPodOwner, owner, "owner "+owner)
	}

	var gpus int64
	for _, c := range p.Spec.Containers {
		if c.Name != container {
			continue
		}
		if len(c.Args) > 0 {
			args, _ := json.Marshal(c.Args) // a []string always marshals
			add(model.FactArgs, string(args), "container "+container+" args")
		}
		for _, env := range c.Env {
			add(model.FactEnvPrefix+env.Name, env.Value, "container "+container+" env "+env.Name)
		}
		if q, ok := c.Resources.Limits[nvidiaGPU]; ok {
			gpus = q.Value()
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Name == container {
			if _, digest, ok := strings.Cut(cs.ImageID, "@"); ok {
				add(model.FactImageDigest, digest, "image id "+cs.ImageID)
			}
		}
	}

	var product string
	if node != nil {
		add(model.FactNodeInstanceType, firstLabel(node, instanceTypeLabels), "node label instance type")
		add(model.FactNodeZone, firstLabel(node, zoneLabels), "node label zone")
		product = node.Labels[nvidiaProduct]
	}
	if gpus > 0 || product != "" {
		add(model.FactAcceleratorVendor, acceleratorNvidia, "nvidia.com resources or labels")
		add(model.FactAcceleratorProduct, product, "node label "+nvidiaProduct)
		if gpus > 0 {
			n := strconv.FormatInt(gpus, 10)
			add(model.FactAcceleratorCount, n, "container limit "+string(nvidiaGPU)+" "+n)
		}
	}
	return out
}

// ownerRef is the pod's controller owner reference, or its first one.
func ownerRef(p *corev1.Pod) (ref struct{ Kind, Name string }, ok bool) {
	refs := p.OwnerReferences
	if len(refs) == 0 {
		return ref, false
	}
	chosen := refs[0]
	for _, r := range refs {
		if r.Controller != nil && *r.Controller {
			chosen = r
			break
		}
	}
	ref.Kind, ref.Name = chosen.Kind, chosen.Name
	return ref, true
}

// inferOwner names the workload without asking the API: the Deployment controller names a
// ReplicaSet <deployment>-<pod-template-hash> and puts the hash on the pod, so a ReplicaSet
// whose name ends that way belongs to that Deployment. Anything else is the owner as written.
// A live collector that walks the ReplicaSet is the authority over this.
func inferOwner(p *corev1.Pod, ref struct{ Kind, Name string }) string {
	if hash := p.Labels[podTemplateHash]; ref.Kind == deploymentRSKind && hash != "" {
		if deploy, ok := strings.CutSuffix(ref.Name, "-"+hash); ok && deploy != "" {
			return deploymentKind + "/" + deploy
		}
	}
	return ref.Kind + "/" + ref.Name
}

func firstLabel(n *corev1.Node, keys []string) string {
	for _, k := range keys {
		if v := n.Labels[k]; v != "" {
			return v
		}
	}
	return ""
}

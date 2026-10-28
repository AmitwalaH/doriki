package rules

import (
	"regexp"
	"strconv"

	"github.com/AmitwalaH/doriki/internal/model"
)

// pull-dominant: the image pull is a large part of the boot and the image was not already on
// the node (DESIGN.md section 6.2). Sources for the fixes: pre-pulled images,
// kubernetes.io/docs/concepts/containers/images/#pre-pulled-images; lazy pulling,
// github.com/awslabs/soci-snapshotter and github.com/containerd/stargz-snapshotter; image
// volumes, kubernetes.io/docs/tasks/configure-pod-container/image-volumes/ (KEP-4639).
const (
	// pullShare is the trigger: image-pull at 35% or more of the total.
	pullShare = 0.35
	// imageVolumeMinor is the first Kubernetes 1.x minor with image volumes on by default:
	// alpha in 1.31 and 1.32, beta but off by default (feature gate) in 1.33 and 1.34, on by
	// default in 1.35, stable in 1.36 (verified by the owner on 2026-10-26).
	imageVolumeMinor = 35
)

// kubeletVersion reads the major and minor of a kubelet version such as v1.33.2 or
// v1.35.0-eks-1a2b3.
var kubeletVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

func pullDominant(r model.Report) (model.Advice, bool) {
	p, share, ok := phaseShare(r, model.PhaseImagePull)
	// A cached image is a complete 0 s pull marked cached (decision 0004): nothing to prepull.
	if !ok || share < pullShare || p.Attrs["cached"] == "true" {
		return model.Advice{}, false
	}
	ev := evidence(r, p, share)
	text := took(r, p, share) + " and the image was not on the node; "
	fix := "a prepull DaemonSet for the image; lazy pulling with the SOCI or estargz snapshotter"

	kubelet := ""
	if r.Manifest != nil && r.Manifest.Node != nil {
		kubelet = r.Manifest.Node.Kubelet
	}
	switch volumes, known := imageVolumesOn(kubelet); {
	case !known:
		ev["kubelet"] = "unknown"
		text += "prepull it with a DaemonSet or pull lazily with SOCI or estargz (image volumes are not suggested: " +
			"the node's kubelet version could not be read)."
	case !volumes:
		ev["kubelet"] = kubelet
		text += "prepull it with a DaemonSet or pull lazily with SOCI or estargz (image volumes are not suggested: " +
			"they are off by default before Kubernetes 1.35, and this node runs " + kubelet + ")."
	default:
		ev["kubelet"] = kubelet
		text += "move the model weights into an image volume, prepull the image with a DaemonSet, or pull lazily with SOCI or estargz."
		fix = "an image volume (spec.volumes[].image) for the model weights; " + fix
	}
	return model.Advice{Rule: "pull-dominant", Text: text, Evidence: ev, Fix: fix}, true
}

// imageVolumesOn says whether a kubelet of this version has image volumes on by default;
// known is false when the version is empty or unreadable.
func imageVolumesOn(kubelet string) (on, known bool) {
	m := kubeletVersion.FindStringSubmatch(kubelet)
	if m == nil {
		return false, false
	}
	major, _ := strconv.Atoi(m[1]) // the regexp matched digits
	minor, _ := strconv.Atoi(m[2])
	return major > 1 || major == 1 && minor >= imageVolumeMinor, true
}

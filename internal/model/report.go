package model

import "time"

// ReportVersion is the report schema version. The report is append-only: fields are added,
// never renamed or removed.
const ReportVersion = 1

// Report is the contract every renderer consumes (DESIGN.md section 4.4). Optional fields
// are omitted when empty and never written as null.
type Report struct {
	ReportVersion int       `json:"report_version"`
	GeneratedAt   time.Time `json:"generated_at,omitzero"`
	DorikiVersion string    `json:"doriki_version,omitempty"`
	Manifest      *Manifest `json:"manifest,omitempty"`

	// The boot attempt the top-level phases describe. Attempt numbers follow the
	// container's restartCount, so the first boot is attempt 0 and is always written.
	Attempt       int `json:"attempt"`
	AttemptsTotal int `json:"attempts_total"`

	Phases   []Phase   `json:"phases,omitempty"`
	Gaps     []Gap     `json:"gaps,omitempty"`
	Totals   Totals    `json:"totals,omitzero"`
	Advice   []Advice  `json:"advice,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`

	// Every attempt, filled only with --all-attempts.
	Attempts []Attempt `json:"attempts,omitempty"`
	// The input events, filled only with --include-events.
	Events []Event `json:"events,omitempty"`
}

// Attempt is one boot of the container: the first start and each restart.
type Attempt struct {
	Attempt int     `json:"attempt"`
	Phases  []Phase `json:"phases,omitempty"`
	Gaps    []Gap   `json:"gaps,omitempty"`
	Totals  Totals  `json:"totals,omitzero"`
}

// Totals use the wall-clock span, not the sum of phases. A value is nil when its end point
// was not observed; 0 is a real measurement.
type Totals struct {
	PendingToReadySeconds      *float64 `json:"pending_to_ready_seconds,omitempty"`
	PendingToFirstTokenSeconds *float64 `json:"pending_to_first_token_seconds,omitempty"`
}

// ManifestVersion is the manifest schema version; like the report, it is append-only.
const ManifestVersion = 1

// Manifest is the deployment manifest (DESIGN.md section 4.3). Every field is optional and
// omitted when unknown. Pod, node, container and accelerator come from the Kubernetes objects,
// engine and model from the selected pack and the engine log.
type Manifest struct {
	ManifestVersion int                  `json:"manifest_version"`
	Pod             *ManifestPod         `json:"pod,omitempty"`
	Node            *ManifestNode        `json:"node,omitempty"`
	Container       *ManifestContainer   `json:"container,omitempty"`
	Engine          *ManifestEngine      `json:"engine,omitempty"`
	Model           *ManifestModel       `json:"model,omitempty"`
	Accelerator     *ManifestAccelerator `json:"accelerator,omitempty"`
	Caches          *ManifestCaches      `json:"caches,omitempty"`
	// Fingerprint is "sha256:" and the hash of every other field except the pod's name and
	// UID, so two boots of the same deployment share it (DESIGN.md section 4.3).
	Fingerprint string `json:"fingerprint,omitempty"`
}

type ManifestPod struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	UID       string `json:"uid,omitempty"`
	// Owner is the workload, Kind/name: a ReplicaSet's Deployment when known.
	Owner string `json:"owner,omitempty"`
	// OwnerRef is the pod's own ownerReferences entry, Kind/name, as written on the pod.
	OwnerRef string `json:"owner_ref,omitempty"`
}

type ManifestNode struct {
	Name         string `json:"name,omitempty"`
	InstanceType string `json:"instance_type,omitempty"`
	Zone         string `json:"zone,omitempty"`
	// Kubelet is the node's status.nodeInfo.kubeletVersion, as written (v1.33.2).
	Kubelet string `json:"kubelet,omitempty"`
}

// ManifestContainer is the profiled container. EnvAllowlist holds allowlisted variables only,
// with secret values redacted; Args are redacted the same way.
type ManifestContainer struct {
	Image        string            `json:"image,omitempty"`
	ImageDigest  string            `json:"image_digest,omitempty"`
	Args         []string          `json:"args,omitempty"`
	EnvAllowlist map[string]string `json:"env_allowlist,omitempty"`
}

type ManifestEngine struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	ParserPack string `json:"parser_pack,omitempty"`
}

type ManifestModel struct {
	Ref string `json:"ref,omitempty"`
	// Source is the host the weights download from, derived from Ref by manifest.ModelSource;
	// omitted when Ref is a local path or unknown.
	Source string `json:"source,omitempty"`
}

// ManifestAccelerator comes from node labels and container limits only; doriki reads no GPU.
type ManifestAccelerator struct {
	Vendor  string `json:"vendor,omitempty"`
	Product string `json:"product,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// ManifestCaches is what the engine log said about its caches. TorchCompile is "hit" or
// "miss", set by a pack fact; a miss anywhere in the boot is a miss.
type ManifestCaches struct {
	TorchCompile string `json:"torch_compile,omitempty"`
}

// CacheMiss is the ManifestCaches value for a cache the boot could not use; manifest keeps it
// over a later hit, and compile-miss fires on it.
const CacheMiss = "miss"

// Advice is one rule's finding (DESIGN.md section 6.2). Text is the one sentence the terminal
// prints, fix included; Evidence holds the numbers the rule used; Fix names the flag, resource
// or setting to change.
type Advice struct {
	Rule     string         `json:"rule"`
	Severity string         `json:"severity"`
	Text     string         `json:"text"`
	Evidence map[string]any `json:"evidence,omitempty"`
	Fix      string         `json:"fix,omitempty"`
}

// WarnPhaseUnobserved is the warning code for an expected engine phase still absent once the
// pod is Ready (decision 0005). The phase engine writes it; the exit code reads it, so a
// warned phase does not hold a booted pod at exit 1.
const WarnPhaseUnobserved = "phase-unobserved"

// Warning is something the user should know about the inputs, such as clock skew.
type Warning struct {
	Code string `json:"code"`
	Text string `json:"text"`
	// Phase names the phase the warning is about, when it is about one.
	Phase PhaseName `json:"phase,omitempty"`
}

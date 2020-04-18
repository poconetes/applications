package v1

import (
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelApp       = "poco-app"
	LabelFormation = "poco-formation"
)

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	// +required
	Formations []Formation `json:"formations"`
	// +required
	Image string `json:"image"`

	RunSpec `json:",inline"`
}

// Mount ...
type Mount struct {
	// +required
	Name string `json:"name"`
	// +required
	Path string `json:"path"`
	// +optional
	ConfigMap *corev1.LocalObjectReference `json:"configMapRef,omitempty"`
	// +optional
	Secret *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// Formation ...
type Formation struct {
	// +required
	Name string `json:"name"`
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
	// +optional
	Scaling []autoscaling.MetricSpec `json:"scaling,omitempty"`

	RunSpec `json:",inline"`
}

type RunSpec struct {
	// +optional
	Command []string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// +optional
	Ports []FormationPort `json:"ports,omitempty"`
	// +optional
	Environment []corev1.EnvVar `json:"environment,omitempty"`
	// +optional
	EnvironmentRefs []corev1.EnvFromSource `json:"environmentRefs,omitempty"`
	// +optional
	Mounts []Mount `json:"mounts,omitempty"`
	// +optional
	SLO *corev1.ResourceRequirements `json:"slo,omitempty"`
}

// FormationPort ...
type FormationPort struct {
	// +required
	Name string `json:"name"`
	// +required
	Port int32 `json:"port,omitempty"`
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	ObservedGeneration int64             `json:"observedGeneration,omitempty"`
	Formations         []FormationStatus `json:"formations,omitempty"`
	Pocolets           int32             `json:"pocolets"`
	State              ApplicationState  `json:"state"`
}

type ApplicationState string

const (
	ApplicationStateOnline  ApplicationState = "online"
	ApplicationStateWaiting ApplicationState = "waiting"
	ApplicationStateError   ApplicationState = "error"
)

// FormationStatus ...
type FormationStatus struct {
	Name                string         `json:"name,omitempty"`
	State               FormationState `json:"state,omitempty"`
	Message             string         `json:"message,omitempty"`
	ReplicasDesired     int32          `json:"replicasDesired,omitempty"`
	ReplicasAvailable   int32          `json:"replicasAvailable"`
	ReplicasUnavailable int32          `json:"replicasUnavailable"`
}

type FormationStatuses []FormationStatus

func (fs FormationStatuses) Len() int {
	return len(fs)
}

func (fs FormationStatuses) Less(i, j int) bool {
	return fs[i].Name < fs[j].Name
}

func (fs FormationStatuses) Swap(i, j int) {
	fs[i], fs[j] = fs[j], fs[i]
}

type FormationState string

const (
	FormationStateOnline  FormationState = "online"
	FormationStateWaiting FormationState = "waiting"
	FormationStateError   FormationState = "error"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=app
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Pocolets",type=string,JSONPath=`.status.pocolets`

// Application is the Schema for the applications API
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}

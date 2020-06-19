package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelPlan       = "poco-plan"
	DefaultPlanName = "default"
)

// PlanSpec defines the desired state of Plan
type PlanSpec struct {
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// +optional
	ContainerResources *corev1.ResourceRequirements `json:"container_resources,omitempty"`
	// +optional
	InitializerResources *corev1.ResourceRequirements `json:"initializer_resources,omitempty"`
	// +optional
	SidecarResources *corev1.ResourceRequirements `json:"sidecar_resources,omitempty"`
}

// PlanStatus defines the observed state of Plan
type PlanStatus struct {
	Replicas int32 `json:"replicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=plan,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Pocolets",type=string,JSONPath=`.status.pocolets`

// Plan is the Schema for the plans API
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec,omitempty"`
	Status PlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PlanList contains a list of Plan
type PlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Plan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Plan{}, &PlanList{})
}

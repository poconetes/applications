package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SidecarSpec defines the desired state of Sidecar
type SidecarSpec struct {
	// +required
	Image string `json:"image"`

	RunSpec `json:",inline"`
}

// SidecarStatus defines the observed state of Sidecar
type SidecarStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// Sidecar is the Schema for the sidecars API
type Sidecar struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SidecarSpec   `json:"spec,omitempty"`
	Status SidecarStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SidecarList contains a list of Sidecar
type SidecarList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Sidecar `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Sidecar{}, &SidecarList{})
}

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitializerSpec defines the desired state of Initializer
type InitializerSpec struct {
	// +required
	Image string `json:"image"`

	RunSpec `json:",inline"`
}

// InitializerStatus defines the observed state of Initializer
type InitializerStatus struct {
}

// +kubebuilder:object:root=true

// Initializer is the Schema for the initializers API
type Initializer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InitializerSpec   `json:"spec,omitempty"`
	Status InitializerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InitializerList contains a list of Initializer
type InitializerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Initializer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Initializer{}, &InitializerList{})
}

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelApp       = "poco-app"
	LabelFormation = "poco-formation"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	Image string `json:"image"`
	// +optional
	Environment []corev1.EnvVar `json:"environment"`
	// +optional
	Mounts []Mount `json:"mounts"`
	// +required
	Formations []Formation `json:"formations"`
}

// Mount ...
type Mount struct {
	// +required
	Name string `json:"name"`
	// +required
	Path string `json:"path"`
	// +optional
	Exclusive bool `json:"exclusive,omitempty"`
	// +optional
	Template *corev1.PersistentVolumeClaimSpec `json:"template,omitempty"`
	// +optional
	ObjectRef *corev1.ObjectReference `json:"objectRef,omitempty"`
}

// Formation ...
type Formation struct {
	// +required
	Name string `json:"name"`
	// +required
	MinReplicas int32 `json:"minReplicas"`
	// +required
	MaxReplicas int32 `json:"maxReplicas"`
	// +optional
	Cmd string `json:"cmd,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// +optional
	Ports []corev1.ContainerPort `json:"ports,omitempty"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkResourceSpec defines the desired state of NetworkResource
type NetworkResourceSpec struct {
	Hosts []string

	RoutingPeerRef corev1.LocalObjectReference `json:"routingPeerRef"`

	ServiceRef corev1.LocalObjectReference `json:"serviceRef"`

	// +optional
	Groups []ResourceReference `json:"groups,omitempty"`
}

// NetworkResourceStatus defines the observed state of NetworkResource.
type NetworkResourceStatus struct {
	ResourceID *string `json:"resourceID,omitempty"`

	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource

// NetworkResource is the Schema for the networkresources API
type NetworkResource struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NetworkResource
	// +required
	Spec NetworkResourceSpec `json:"spec"`

	// status defines the observed state of NetworkResource
	// +optional
	Status NetworkResourceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkResourceList contains a list of NetworkResource
type NetworkResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NetworkResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkResource{}, &NetworkResourceList{})
}

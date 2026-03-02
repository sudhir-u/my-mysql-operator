package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MySQLSpec defines the desired state of MySQL
type MySQLSpec struct {
	// Version of MySQL to deploy (e.g., "8.0", "5.7")
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// StorageSize for the MySQL data volume (e.g., "10Gi")
	// +kubebuilder:validation:Required
	StorageSize string `json:"storageSize"`

	// RootPassword for MySQL root user (should reference a secret in production)
	// +optional
	RootPassword string `json:"rootPassword,omitempty"`

	// Replicas is the number of MySQL pods (1 = single instance, 2 = primary/standby for failover).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// MySQLStatus defines the observed state of MySQL
type MySQLStatus struct {
	// Phase represents the current state of the MySQL instance
	// Possible values: Pending, Running, Failed
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Ready indicates whether the MySQL instance is ready to accept connections
	Ready bool `json:"ready,omitempty"`

	// PrimaryPodName is the name of the pod currently designated as primary (for HA with 2 replicas).
	// Used by the failover controller to route the primary Service and to decide failover target.
	// +optional
	PrimaryPodName string `json:"primaryPodName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Primary",type=string,JSONPath=`.status.primaryPodName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MySQL is the Schema for the mysqls API
type MySQL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MySQLSpec   `json:"spec,omitempty"`
	Status MySQLStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MySQLList contains a list of MySQL
type MySQLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MySQL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQL{}, &MySQLList{})
}

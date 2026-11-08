package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaintenanceWindowSpec defines a daily UTC wall-clock window when pin-driven upgrades may roll.
type MaintenanceWindowSpec struct {
	// Start is the window start in UTC as HH:MM (24h). Example: "02:00"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	Start string `json:"start"`

	// End is the window end in UTC as HH:MM (24h). Example: "04:00"
	// Windows that cross midnight are supported (e.g. start 23:00, end 01:00).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	End string `json:"end"`
}

// MySQLSpec defines the desired state of MySQL
type MySQLSpec struct {
	// Version is the MySQL image tag: a channel (e.g. "8.4", "8.0") or a pin (e.g. "8.4.13").
	// Channels are resolved via the Offering-owned channel pins ConfigMap to a concrete patch tag.
	// Prefer not to use "latest". Image form: mysql:<resolved-tag>.
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

	// MaintenanceWindow is the daily UTC window when pin-driven (channel map) upgrades may roll the StatefulSet.
	// If unset, pin-driven upgrades are deferred until a window is configured (safe default).
	// Explicit Spec pin changes (customer-initiated) are applied immediately.
	// +optional
	MaintenanceWindow *MaintenanceWindowSpec `json:"maintenanceWindow,omitempty"`
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

	// ResolvedImage is the concrete container image the operator is reconciling toward (e.g. mysql:8.4.13).
	// +optional
	ResolvedImage string `json:"resolvedImage,omitempty"`
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

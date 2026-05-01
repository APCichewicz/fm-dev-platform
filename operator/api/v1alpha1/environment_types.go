/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of Environment. Edit environment_types.go to remove/update
	// +optional
	Owner         string                    `json:"owner"`
	ExpiresAt     metav1.Time               `json:"expiresAt"`
	Deployments   map[string]DeploymentSpec `json:"deployments"`
	NetworkPolicy *NetworkPolicySpec        `json:"networkPolicy"`
}

type NetworkPolicySpec struct {
	Allow *NetworkPolicyAllow `json:"allow,omitempty"`
}

type NetworkPolicyAllow struct {
	Namespaces []string  `json:"namespaces"`
	IpBlocks   []IpBlock `json:"ipBlocks"`
}

type IpBlock struct {
	Cidr   string   `json:"cidr"`
	Except []string `json:"except"`
}

type DeploymentSpec struct {
	// Required
	Image string `json:"image"`

	// Optional, default IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Optional. When omitted, no Service or IngressRoute is emitted.
	Port *int32 `json:"port,omitempty"`

	// Optional. requests/limits.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Optional, default false. When true, WorkloadIdentity becomes required.
	UsesServiceAccount bool `json:"usesServiceAccount,omitempty"`

	// Required when UsesServiceAccount is true (chart enforces).
	WorkloadIdentity *WorkloadIdentitySpec `json:"workloadIdentity,omitempty"`

	// Optional. corev1.EnvVar gives you valueFrom (secretKeyRef / configMapKeyRef) for free.
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Optional. Pull whole ConfigMaps / Secrets in as env.
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Optional. All three sub-probes default to off.
	Probes *ProbesSpec `json:"probes,omitempty"`

	// Optional.
	Volumes      []corev1.Volume      `json:"volumes,omitempty"`
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Optional. Merged on top of platform-standard labels.
	Labels    map[string]string `json:"labels,omitempty"`
	PodLabels map[string]string `json:"podLabels,omitempty"`
}

type WorkloadIdentitySpec struct {
	// Azure AD app registration UUID.
	ClientID string `json:"clientId"`
}

type ProbesSpec struct {
	Liveness  *corev1.Probe `json:"liveness,omitempty"`
	Readiness *corev1.Probe `json:"readiness,omitempty"`
	Startup   *corev1.Probe `json:"startup,omitempty"`
}

type EnvironmentPhase string

const (
	PhasePending      EnvironmentPhase = "Pending"
	PhaseProvisioning EnvironmentPhase = "Provisioning"
	PhaseReady        EnvironmentPhase = "Ready"
	PhaseDegraded     EnvironmentPhase = "Degraded"
	PhaseExpiring     EnvironmentPhase = "Expiring"
	PhaseDeleting     EnvironmentPhase = "Deleting"
	PhaseFailed       EnvironmentPhase = "Failed"
)

type EnvironmentStatus struct {
	// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Degraded;Expiring;Deleting;Failed
	// +optional
	Phase EnvironmentPhase `json:"phase,omitempty"`

	// +optional
	ApplicationRef *ObjectRef `json:"applicationRef,omitempty"`

	// +optional
	NamespaceRef string `json:"namespaceRef,omitempty"`

	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// +optional
	Deployments map[string]DeploymentStatus `json:"deployments,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ObjectRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type DeploymentStatus struct {
	// +optional
	Ready bool `json:"ready,omitempty"`

	// +optional
	URL string `json:"url,omitempty"`

	// +optional
	AppliedResources *corev1.ResourceRequirements `json:"appliedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Environment is the Schema for the environments API
type Environment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Environment
	// +required
	Spec EnvironmentSpec `json:"spec"`

	// status defines the observed state of Environment
	// +optional
	Status EnvironmentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}

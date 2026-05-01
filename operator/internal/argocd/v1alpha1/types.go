// Package v1alpha1 contains a minimal local subset of the ArgoCD Application API.
// Importing the upstream argo-cd Go module pulls in the Argo server and CLI as
// transitive deps; the Argo team explicitly recommends against that. We only
// need to read/write a handful of Application fields, so we model them locally.
//
// +kubebuilder:object:generate=true
// +groupName=argoproj.io
package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "argoproj.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

// +kubebuilder:object:root=true

// Application is a minimal model of argoproj.io/v1alpha1 Application.
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApplicationSpec   `json:"spec,omitempty"`
	Status            ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList is a list of Applications.
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

type ApplicationSpec struct {
	Project     string                 `json:"project"`
	Source      ApplicationSource      `json:"source"`
	Destination ApplicationDestination `json:"destination"`

	// SyncPolicy is a pointer so we can clear it (set to nil) during teardown
	// to disable automated sync before deleting child resources.
	SyncPolicy *SyncPolicy `json:"syncPolicy,omitempty"`
}

type ApplicationSource struct {
	RepoURL        string                 `json:"repoURL"`
	Chart          string                 `json:"chart,omitempty"`
	TargetRevision string                 `json:"targetRevision,omitempty"`
	Helm           *ApplicationSourceHelm `json:"helm,omitempty"`
}

type ApplicationSourceHelm struct {
	// ValuesObject is the structured form of helm values. Preferred over
	// the legacy `values` string — round-trips through JSON cleanly.
	ValuesObject *apiextensionsv1.JSON `json:"valuesObject,omitempty"`

	ReleaseName string `json:"releaseName,omitempty"`
}

type ApplicationDestination struct {
	Server    string `json:"server,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type SyncPolicy struct {
	Automated *SyncPolicyAutomated `json:"automated,omitempty"`
}

type SyncPolicyAutomated struct {
	Prune    bool `json:"prune,omitempty"`
	SelfHeal bool `json:"selfHeal,omitempty"`
}

type ApplicationStatus struct {
	Health HealthStatus `json:"health,omitempty"`
	Sync   SyncStatus   `json:"sync,omitempty"`
}

type HealthStatus struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

type SyncStatus struct {
	Status   string `json:"status,omitempty"`
	Revision string `json:"revision,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}

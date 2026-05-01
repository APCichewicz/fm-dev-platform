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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/apcichewicz/fm-dev-platform-operator/api/v1alpha1"
	argocdv1alpha1 "github.com/apcichewicz/fm-dev-platform-operator/internal/argocd/v1alpha1"
	"github.com/apcichewicz/fm-dev-platform-operator/internal/controller/config"
)

const (
	finalizerName = "platform.fastmarkets.io/finalizer"
)

// EnvironmentReconciler reconciles a Environment object
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.Config
}

// +kubebuilder:rbac:groups=platform.fastmarkets.io,resources=environments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.fastmarkets.io,resources=environments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.fastmarkets.io,resources=environments/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Environment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var environment platformv1alpha1.Environment
	if err := r.Get(ctx, req.NamespacedName, &environment); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logf.FromContext(ctx).Info("Reconciling Environment", "Environment", environment.Name)
	if !environment.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&environment, finalizerName) {
			result, err := r.runTeardown(ctx, &environment)
			if err != nil {
				logf.FromContext(ctx).Error(err, "Failed to run teardown")
				return ctrl.Result{}, err
			}
			if result.Requeue || result.RequeueAfter > 0 {
				return result, nil
			}
			controllerutil.RemoveFinalizer(&environment, finalizerName)
			if err := r.Update(ctx, &environment); err != nil {
				logf.FromContext(ctx).Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
			return result, nil
		}
		return ctrl.Result{}, nil
	}
	// idempotently add a finalizer. this is after the env is created but before a finalizer is put on it. otherwise, doesnt run
	if !controllerutil.ContainsFinalizer(&environment, finalizerName) {
		controllerutil.AddFinalizer(&environment, finalizerName)
		if err := r.Update(ctx, &environment); err != nil {
			logf.FromContext(ctx).Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// check TTL, if expired start deletion
	now := metav1.Now()
	if environment.Spec.ExpiresAt.Before(&now) {
		err := r.Delete(ctx, &environment)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to start deletion")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.ensureNamespace(ctx, &environment); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to ensure namespace")
		return ctrl.Result{}, err
	}
	if err := r.ensureApplication(ctx, &environment); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to ensure application")
		return ctrl.Result{}, err
	}

	err := r.Status().Update(ctx, &environment)
	if err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Until(environment.Spec.ExpiresAt.Time)}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Environment{}).
		Named("environment").
		Complete(r)
}

func (r *EnvironmentReconciler) runTeardown(ctx context.Context, environment *platformv1alpha1.Environment) (ctrl.Result, error) {
	// handle application first
	var app argocdv1alpha1.Application
	appName := fmt.Sprintf("env-%s", environment.Name)
	err := r.Get(ctx, client.ObjectKey{Name: appName, Namespace: r.Config.ArgoCDNamespace}, &app)
	switch {
	case errors.IsNotFound(err):
		// fall through, app already deleted
	case err != nil:
		return ctrl.Result{}, err
	default:
		needsPatch := app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Automated != nil
		if !controllerutil.ContainsFinalizer(&app, "resources-finalizer.argocd.argoproj.io") {
			needsPatch = true
		}
		if needsPatch {
			patch := client.MergeFrom(app.DeepCopy())
			if app.Spec.SyncPolicy != nil {
				app.Spec.SyncPolicy.Automated = nil
			}
			controllerutil.AddFinalizer(&app, "resources-finalizer.argocd.argoproj.io")
			if err := r.Patch(ctx, &app, patch); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		// queue deletion
		if app.DeletionTimestamp.IsZero() {
			err := r.Delete(ctx, &app)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		// requeue and wait for 10 seconds
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// we dont get here until app is deleted and we fall through the switch statement
	var ns corev1.Namespace
	nsName := fmt.Sprintf("env-%s", environment.Name)
	err = r.Get(ctx, client.ObjectKey{Name: nsName}, &ns)
	switch {
	case errors.IsNotFound(err):
		// both gone — teardown complete
		return ctrl.Result{}, nil
	case err != nil:
		return ctrl.Result{}, err
	default:
		if ns.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, &ns); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func (r *EnvironmentReconciler) ensureNamespace(ctx context.Context, environment *platformv1alpha1.Environment) error {
	nsName := fmt.Sprintf("env-%s", environment.Name)

	var existing corev1.Namespace
	err := r.Get(ctx, client.ObjectKey{Name: nsName}, &existing)
	if err == nil {
		// already exists; v1 doesn't reconcile drift on namespace labels/annotations
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce":         "restricted", // spec SEC-1
				"pod-security.kubernetes.io/enforce-version": "latest",
				"platform.fastmarkets.io/env-name":           environment.Name,
				"platform.fastmarkets.io/managed-by":         "fm-dev-platform-operator",
			},
			Annotations: map[string]string{
				"platform.fastmarkets.io/owner":      environment.Spec.Owner,
				"platform.fastmarkets.io/expires-at": environment.Spec.ExpiresAt.Format(time.RFC3339),
			},
		},
	}
	return r.Create(ctx, ns)
}

func (r *EnvironmentReconciler) ensureApplication(ctx context.Context, environment *platformv1alpha1.Environment) error {
	appName := fmt.Sprintf("env-%s", environment.Name)
	nsName := fmt.Sprintf("env-%s", environment.Name)

	values, err := r.buildHelmValues(environment)
	if err != nil {
		return fmt.Errorf("build helm values: %w", err)
	}

	app := &argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: r.Config.ArgoCDNamespace,
		},
	}

	// CreateOrUpdate: Get the object; if missing, run the mutator and Create;
	// if found, run the mutator and Update only if anything changed.
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		if app.Labels == nil {
			app.Labels = map[string]string{}
		}
		app.Labels["platform.fastmarkets.io/env-name"] = environment.Name
		app.Labels["platform.fastmarkets.io/managed-by"] = "fm-dev-platform-operator"

		app.Spec = argocdv1alpha1.ApplicationSpec{
			Project: "dev-environments",
			Source: argocdv1alpha1.ApplicationSource{
				RepoURL:        r.Config.ChartRegistry,
				Chart:          r.Config.ChartName,
				TargetRevision: r.Config.ChartVersion,
				Helm: &argocdv1alpha1.ApplicationSourceHelm{
					ValuesObject: values,
					ReleaseName:  environment.Name,
				},
			},
			Destination: argocdv1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: nsName,
			},
			SyncPolicy: &argocdv1alpha1.SyncPolicy{
				Automated: &argocdv1alpha1.SyncPolicyAutomated{
					Prune:    true,
					SelfHeal: true,
				},
			},
		}
		return nil
	})
	return err
}

// helmValues mirrors the chart's values.yaml shape. Marshalling this into JSON
// produces the payload Argo passes to `helm install --values ...`.
type helmValues struct {
	Env           helmValuesEnv                              `json:"env"`
	Ingress       helmValuesIngress                          `json:"ingress"`
	NetworkPolicy helmValuesNetworkPolicy                    `json:"networkPolicy"`
	Deployments   map[string]platformv1alpha1.DeploymentSpec `json:"deployments"`
}

type helmValuesEnv struct {
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expiresAt"`
}

type helmValuesIngress struct {
	Host          string `json:"host"`
	PathPrefix    string `json:"pathPrefix"`
	TLSSecretName string `json:"tlsSecretName"`
}

type helmValuesNetworkPolicy struct {
	TraefikNamespace string                       `json:"traefikNamespace"`
	Allow            helmValuesNetworkPolicyAllow `json:"allow"`
}

type helmValuesNetworkPolicyAllow struct {
	Namespaces []string                   `json:"namespaces"`
	IpBlocks   []platformv1alpha1.IpBlock `json:"ipBlocks"`
}

func (r *EnvironmentReconciler) buildHelmValues(environment *platformv1alpha1.Environment) (*apiextensionsv1.JSON, error) {
	v := helmValues{
		Env: helmValuesEnv{
			Name:      environment.Name,
			Owner:     environment.Spec.Owner,
			ExpiresAt: environment.Spec.ExpiresAt.Format(time.RFC3339),
		},
		Ingress: helmValuesIngress{
			Host:          r.Config.IngressHost,
			PathPrefix:    r.Config.IngressPathPrefix,
			TLSSecretName: r.Config.TLSSecretName,
		},
		NetworkPolicy: helmValuesNetworkPolicy{
			TraefikNamespace: r.Config.TraefikNamespace,
			Allow: helmValuesNetworkPolicyAllow{
				Namespaces: []string{},
				IpBlocks:   []platformv1alpha1.IpBlock{},
			},
		},
		Deployments: environment.Spec.Deployments,
	}
	if environment.Spec.NetworkPolicy != nil && environment.Spec.NetworkPolicy.Allow != nil {
		v.NetworkPolicy.Allow.Namespaces = environment.Spec.NetworkPolicy.Allow.Namespaces
		v.NetworkPolicy.Allow.IpBlocks = environment.Spec.NetworkPolicy.Allow.IpBlocks
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal helm values: %w", err)
	}
	return &apiextensionsv1.JSON{Raw: raw}, nil
}

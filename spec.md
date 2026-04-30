# Developer Self-Service Environments Platform

**Status:** Draft
**Version:** 0.1
**Owner:** Platform Engineering
**Last updated:** 2026-04-30

---

## 1. Summary

A self-service platform that lets Fastmarkets developers provision short-lived, standards-compliant Kubernetes environments through an existing internal web UI. Each environment is a complete bundle — Deployment, Service, IngressRoute, autoscaler, resource policies — created from a single high-level request and automatically reaped after a TTL.

The platform is implemented as a Kubebuilder operator that manages an `Environment` custom resource. Each `Environment` produces an ArgoCD `Application` that syncs a curated Helm chart. The operator owns lifecycle (creation, status aggregation, TTL expiry, deletion ordering); ArgoCD owns reconciliation and drift correction; the Helm chart owns the bundle definition.

## 2. Problem & Motivation

Developers currently need short-lived Kubernetes environments for feature branches, demos, integration testing, and reproducing production issues. Today this is done via ad-hoc ArgoCD Applications, hand-edited Helm values, or copy-pasted manifests. This produces three concrete problems:

- **Standards drift.** Security context, resource requests, network policies, and labels are inconsistent across environments. Anything we want to enforce platform-wide (PSS restricted, default NetworkPolicy, ResourceQuota) ends up retro-applied via Kyverno after the fact.
- **Cluster sprawl.** Environments outlive their usefulness. There is no enforced TTL and no clear owner, so cleanup is manual and erratic. AKS cost optimization is undermined by long-lived idle workloads.
- **Friction.** Developers without deep platform knowledge need help from platform engineers for what should be a templated, repeatable action.

A self-service tool with a constrained API and enforced expiry resolves all three.

## 3. Goals

1. Developers can provision a standards-compliant Kubernetes environment in under five minutes from a web form, with no platform-team involvement.
2. Every environment has an enforced TTL (default 8h, max 7d) and is automatically deleted on expiry without leaving orphaned resources.
3. Environments are produced from a single curated Helm chart, so platform-wide changes (e.g., a new default security context, a new label) are made in one place.
4. The platform integrates with existing tooling: ArgoCD for sync, Helm for templating, Traefik for ingress, KEDA for autoscaling, Sealed Secrets for secret material, the existing internal web UI for the user surface.
5. Status is observable end-to-end: developers see provisioning progress and expiry countdown in the UI; platform engineers see fleet-wide health, cost, and TTL distribution in Grafana.

### Non-Goals

- Full multi-tenancy with cluster-level isolation. Environments share a cluster and rely on namespace-scoped RBAC, NetworkPolicy, and ResourceQuota for separation.
- Long-lived production or pre-production environments. This tool is explicitly for ephemeral developer use.
- Build pipelines. The platform consumes pre-built container images; CI is out of scope.
- Multi-cluster targeting in v1. Single cluster only; multi-cluster is a future-work item.
- Custom resource types beyond what the curated chart provides. If a developer needs something the chart doesn't expose, that's a chart change, not a per-environment override.

## 4. Users & Use Cases

**Primary user — Application Developer.** Wants to deploy their service from a feature branch, hit it with a real DNS name and TLS, and have it disappear when they're done. Doesn't want to learn Helm values, IngressRoute syntax, or KEDA ScaledObject schemas.

**Secondary user — Platform Engineer.** Owns the curated Helm chart and the operator. Needs to push chart changes, monitor environment health and cost, debug stuck deletions, and adjust defaults (TTL, resource caps, security context).

**Tertiary user — Engineering Manager.** Wants a fleet view: who has environments running, how long, what they cost, and which are nearing expiry.

### Representative use cases

- "I'm working on a feature branch and need to demo it to product tomorrow at 2pm."
- "QA needs to reproduce a production bug against image tag `v2.3.4-hotfix`."
- "I'm doing a load test and need a real env with autoscaling enabled for two days."
- "We're onboarding a new service; let me spin up an env to validate the chart works for it."

## 5. Architecture Overview

```
┌─────────────┐         ┌─────────────────┐         ┌──────────────────┐
│   Web UI    │────────▶│  Backend API    │────────▶│  Kubernetes API  │
│ (existing)  │  HTTPS  │  (auth, audit)  │  apply  │  Environment CR  │
└─────────────┘         └─────────────────┘         └──────────────────┘
                                                              │
                                                     watches  │
                                                              ▼
                                                    ┌──────────────────┐
                                                    │ Environment      │
                                                    │ Operator         │
                                                    │ (Kubebuilder)    │
                                                    └──────────────────┘
                                                              │
                                                              │ creates
                                                              ▼
                                                    ┌──────────────────┐
                                                    │ ArgoCD           │
                                                    │ Application      │
                                                    └──────────────────┘
                                                              │
                                                              │ syncs
                                                              ▼
                                                    ┌──────────────────┐
                                                    │ Helm Chart       │
                                                    │ "dev-environment"│
                                                    │  - Namespace     │
                                                    │  - Deployment    │
                                                    │  - Service       │
                                                    │  - IngressRoute  │
                                                    │  - ScaledObject  │
                                                    │  - VPA (Off)     │
                                                    │  - ResourceQuota │
                                                    │  - NetworkPolicy │
                                                    └──────────────────┘
```

**Component responsibilities.** The web UI handles authentication, form rendering, and status display. The backend API handles authorization (which user can create what), audit logging, and translation of form input to an `Environment` CR. The operator owns lifecycle: namespace creation, Application creation, status aggregation from Application health, TTL enforcement, deletion ordering. ArgoCD owns sync, drift correction, and self-heal. The Helm chart owns the manifest bundle and its defaults.

## 6. Functional Requirements

### Environment lifecycle

- **FR-1.** A user with the `environment.create` permission MUST be able to submit a form in the web UI and receive a running environment within 5 minutes (P95).
- **FR-2.** Each environment MUST have a unique DNS hostname under a configured base domain, with a wildcard TLS certificate provisioned via cert-manager.
- **FR-3.** Each environment MUST live in its own dedicated namespace named `env-<environment-name>`.
- **FR-4.** Each environment MUST have a TTL between 1 hour and 7 days. Default is 8 hours.
- **FR-5.** A user MUST be able to extend the TTL of their own environment up to the 7-day cap before expiry.
- **FR-6.** A user MUST be able to manually delete their own environment before TTL expiry.
- **FR-7.** Within 10 minutes of TTL expiry, the environment and all child resources (Application, namespace, all in-namespace resources) MUST be fully deleted.
- **FR-8.** Environment names MUST be validated against a regex (`^[a-z][a-z0-9-]{2,30}$`) and a denylist of reserved names.

### Bundle contents

- **FR-9.** The chart MUST produce a Deployment with: enforced `securityContext` (`runAsNonRoot: true`, `readOnlyRootFilesystem: true`, dropped capabilities, seccomp `RuntimeDefault`), resource requests and limits, liveness and readiness probes, and standard labels (`app.kubernetes.io/*`, `platform.fastmarkets.io/owner`, `platform.fastmarkets.io/env-name`, `platform.fastmarkets.io/expires-at`).
- **FR-10.** The chart MUST produce a ClusterIP Service selecting the Deployment.
- **FR-11.** The chart MUST produce a Traefik IngressRoute attached to the Service, using HTTPS with the wildcard cert and the configured hostname.
- **FR-12.** The chart MUST produce a KEDA ScaledObject targeting the Deployment with CPU and memory triggers, configurable min/max replicas (defaults: min 1, max 5).
- **FR-13.** The chart MUST produce a VerticalPodAutoscaler in `Off` (recommender) mode targeting the Deployment. The operator MUST surface VPA recommendations on `Environment.status` so the UI can display rightsizing hints.
- **FR-14.** The chart MUST produce a ResourceQuota and LimitRange in the namespace to cap blast radius.
- **FR-15.** The chart MUST produce a default-deny NetworkPolicy plus explicit allow rules for ingress from Traefik and egress to cluster DNS and the configured shared services (Mimir, Loki, etc.).

### API & UI

- **FR-16.** The `Environment` CRD MUST expose a strongly-typed `spec` validated by an OpenAPI v3 schema. Untyped passthrough values are not supported in v1.
- **FR-17.** The web UI MUST render a form generated from (or aligned to) the CRD schema, including help text, defaults, and validation errors.
- **FR-18.** The web UI MUST display per-environment status: phase, URL, owner, created-at, expires-at, current replica count, and VPA recommendations.
- **FR-19.** The web UI MUST display a "my environments" view and an "all environments" view (the latter gated by an admin permission).
- **FR-20.** The backend MUST log every create, extend, and delete action with user identity, environment name, and timestamp.

### Authorization

- **FR-21.** The backend MUST authenticate users via the existing SSO and authorize actions against a per-user role.
- **FR-22.** A user MUST only be able to extend or delete environments they own, unless they have an admin role.
- **FR-23.** A configurable per-user concurrent environment limit MUST be enforced (default 3). Quota refusals MUST surface as a clear UI error, not a 500.

## 7. Non-Functional Requirements

- **NFR-1.** Environment creation P95 latency (form submit to Ready) ≤ 5 minutes.
- **NFR-2.** Environment deletion P95 latency (expiry to fully gone) ≤ 10 minutes.
- **NFR-3.** Operator availability ≥ 99.5% measured over 30 days. Operator restart MUST NOT cause any environment to be deleted, recreated, or otherwise disrupted.
- **NFR-4.** All operator and backend code MUST emit OpenTelemetry traces, metrics, and structured logs to the existing Grafana stack.
- **NFR-5.** The platform MUST tolerate ArgoCD being temporarily unavailable: pending creates queue, pending deletes are retried, no `Environment` CRs enter an unrecoverable state.
- **NFR-6.** The chart and CRD schema MUST be versioned independently. Chart upgrades MUST be backward-compatible within a minor version.
- **NFR-7.** Resource overhead per environment (excluding the user's workload) SHOULD be ≤ 50m CPU / 64Mi memory across operator and Argo footprint.

## 8. API Specification

### Environment CRD (v1alpha1)

```yaml
apiVersion: platform.fastmarkets.io/v1alpha1
kind: Environment
metadata:
  name: feature-xyz
  namespace: dev-environments    # operator namespace, not the env's namespace
spec:
  owner: andrew@fastmarkets.com
  ttl: 8h                        # 1h ≤ ttl ≤ 168h
  chart:
    version: "1.4.2"             # semver, pinned per environment
  workload:
    image: registry.fastmarkets.io/myapp:abc123
    port: 8080
    env:
      - name: LOG_LEVEL
        value: debug
    secrets:                     # references to pre-sealed SealedSecrets
      - name: db-creds
  hostname: feature-xyz          # combined with base domain at runtime
  resources:
    requests: { cpu: 100m, memory: 256Mi }
    limits:   { cpu: 500m, memory: 512Mi }
  scaling:
    minReplicas: 1
    maxReplicas: 5
    cpuTarget: 70
    memoryTarget: 80
status:
  phase: Ready                   # Pending | Provisioning | Ready | Degraded | Expiring | Deleting | Failed
  url: https://feature-xyz.dev.fastmarkets.io
  applicationRef:
    namespace: argocd
    name: env-feature-xyz
  namespaceRef: env-feature-xyz
  createdAt: "2026-04-30T14:00:00Z"
  expiresAt: "2026-04-30T22:00:00Z"
  vpaRecommendation:
    cpu: 180m
    memory: 312Mi
  conditions:
    - type: Ready
      status: "True"
      reason: ApplicationHealthy
      lastTransitionTime: "2026-04-30T14:03:12Z"
```

### Status phase semantics

| Phase | Meaning |
|---|---|
| Pending | CR accepted, reconcile not yet started |
| Provisioning | Application created, sync in progress |
| Ready | Application Synced + Healthy |
| Degraded | Application Healthy=false; surfaced to user with link to Argo |
| Expiring | Within 1 hour of TTL expiry; UI surfaces extend prompt |
| Deleting | Finalizer running; Application deletion in progress |
| Failed | Terminal failure; manual intervention required |

## 9. UI Requirements

- **UI-1.** Form view: a single screen with grouped sections (Workload, Networking, Scaling, Lifecycle). Sensible defaults pre-populated. Inline validation matching CRD schema. Estimated cost displayed if available.
- **UI-2.** List view: a table of environments with phase, owner, created-at, expires-at, URL, and actions (extend, delete, view in Argo).
- **UI-3.** Detail view: full status, recent events (from `kubectl describe`-equivalent on the CR and Application), VPA recommendations with a "copy as values override" affordance, log link.
- **UI-4.** Expiry banner: any environment within 1 hour of expiry displays a persistent banner with a one-click extend.
- **UI-5.** Empty state: links to platform docs explaining what an environment is and what's included.

## 10. Lifecycle & Operations

### Creation flow

1. User submits form. Backend validates input against the CRD schema and per-user quota.
2. Backend creates the `Environment` CR via Kubernetes API with user identity in `spec.owner` and an `audit.platform.fastmarkets.io/created-by` annotation.
3. Operator reconciles: creates the dedicated namespace with PSS labels, creates the ArgoCD `Application` in the `argocd` namespace targeting the namespace, populates `spec.source.helm.valuesObject` from the CR.
4. ArgoCD syncs. Operator watches `Application` status and updates `Environment.status.phase`.
5. Once `Application.status.health.status == Healthy && sync.status == Synced`, the operator sets phase to `Ready` and emits a `Ready` event.

### Expiry & deletion flow

The operator finalizer (`platform.fastmarkets.io/finalizer`) ensures correct ordering:

1. On TTL expiry (or manual delete), set `Environment.status.phase = Deleting`.
2. Patch the `Application` to add the `resources-finalizer.argocd.argoproj.io` finalizer (cascades resource deletion).
3. Disable the Application's automated sync to prevent fight conditions during teardown.
4. Delete the `Application`. Poll until fully gone (timeout 5 minutes; on timeout, raise alert and require manual intervention).
5. Delete the namespace. Poll until fully gone (timeout 5 minutes).
6. Remove the finalizer from the `Environment` CR. Kubernetes garbage-collects it.

### TTL implementation

Reconcile loop returns `ctrl.Result{RequeueAfter: time.Until(expiresAt)}`. On the next reconcile after expiry, the operator initiates the deletion flow. A backup CronJob runs hourly to catch any environments missed due to operator downtime (`expiresAt < now() - 10m && phase != Deleting`).

### AppProject scoping

A dedicated `dev-environments` ArgoCD AppProject restricts:

- Source: only the OCI registry for the curated chart.
- Destination namespaces: only `env-*`.
- Allowed cluster resources: `Namespace` only.
- Allowed namespaced resources: the explicit set the chart produces (Deployment, Service, IngressRoute, ScaledObject, VPA, ResourceQuota, LimitRange, NetworkPolicy, ConfigMap, SealedSecret).

This is the blast-radius limit if a chart version is compromised or a values payload tries to inject extra manifests.

## 11. Security

- **SEC-1.** Pod Security Standards `restricted` enforced at namespace level via labels.
- **SEC-2.** All workloads run with non-root user, read-only root filesystem, dropped capabilities, seccomp `RuntimeDefault`. Enforced both in the chart (defaults) and via Kyverno (audit + enforce).
- **SEC-3.** All secret material handled via existing Sealed Secrets workflow. The CRD references SealedSecret names; raw secret values are never accepted in the API.
- **SEC-4.** Operator runs with least-privilege RBAC: read/write on `Environment` CRs and `Application` CRs, namespace create/delete, status patches. No cluster-admin.
- **SEC-5.** Backend API authenticates via SSO and authorizes via existing role mappings. No service account tokens or kubeconfigs handled at the user layer.
- **SEC-6.** All hostnames validated to live under the configured base domain. Hostname collisions across environments are rejected at admission.
- **SEC-7.** Default-deny NetworkPolicy in every environment namespace; explicit allows for ingress from Traefik and egress to required shared services.

## 12. Observability

### Operator metrics (Prometheus)

- `environment_total` (gauge, labels: phase, owner-team)
- `environment_creation_duration_seconds` (histogram, labels: result)
- `environment_deletion_duration_seconds` (histogram, labels: result)
- `environment_reconcile_errors_total` (counter, labels: error-type)
- `environment_ttl_seconds_remaining` (gauge, per-environment)

### Dashboards (Grafana)

- **Fleet view:** count by phase, age distribution, TTL distribution, top owners by count.
- **Cost view:** estimated cost per env, total fleet cost trend, cost by team.
- **Operator health:** reconcile latency, error rate, queue depth, watch lag.

### Alerts

- Operator reconcile error rate > 5% for 10 minutes.
- Environment stuck in `Deleting` phase for > 30 minutes.
- Environment stuck in `Provisioning` phase for > 15 minutes.
- TTL backup CronJob failure.

### SLOs

- Creation success rate ≥ 99% over 30 days.
- Deletion success rate ≥ 99.9% over 30 days. (Higher bar — stuck deletes are the primary risk.)
- UI form-submit to Ready P95 ≤ 5 minutes.

## 13. Open Questions

1. **Image source policy.** Do we restrict images to a registry allowlist, or is any image fair game? Recommendation: allowlist `registry.fastmarkets.io` and ghcr.io for personal projects; configurable.
2. **Cost attribution.** What's the source of truth for cost per env — Kubecost, OpenCost, or a homegrown estimate from requests × node price? Affects UI design.
3. **Multi-cluster.** Do dev environments need to span clusters (e.g., AKS prod-mirror vs. AKS dev) in v2? Affects whether the operator becomes a multi-cluster controller or stays single-cluster.
4. **Persistent storage.** v1 explicitly excludes PVCs to keep cleanup clean. Do we need a stateful escape hatch (e.g., shared dev databases referenced by env) and how do we model it?
5. **Chart customization.** When developers need a chart capability that doesn't exist, what's the contribution path? Recommendation: the chart lives in a platform-owned repo with PR review; we add a `CONTRIBUTING.md` to make it clear.
6. **Pre-existing UI integration.** What's the existing UI's tech stack and how does the form schema get generated from the CRD — handwritten, generated from OpenAPI, or driven by a JSON schema endpoint the backend exposes?

## 14. Out of Scope (v1)

- Multi-cluster targeting.
- Stateful workloads (PVCs, StatefulSets).
- Custom domain names per environment outside the configured base.
- CI/CD integration (build on PR open, deploy preview env automatically). Considered for v2.
- Cost showback / chargeback. Visibility only in v1.
- Cross-team environment sharing with granular RBAC. v1 is owner-only + admin-all.
- Environment templates / "favorites" / saved configurations. v2.

## 15. Milestones

| Milestone | Scope | Rough size |
|---|---|---|
| M0 — Chart skeleton | Curated `dev-environment` Helm chart producing all required resources, tested via `helm install` against a kind cluster | 1–2 weeks |
| M1 — Operator MVP | Kubebuilder operator with `Environment` CRD, creates Application, reads back status, no TTL yet | 2 weeks |
| M2 — TTL & deletion | Full lifecycle including finalizer, deletion ordering, backup CronJob, AppProject scoping | 1 week |
| M3 — UI integration | Form, list view, detail view, extend/delete actions, expiry banner | 2 weeks |
| M4 — Observability | Metrics, dashboards, alerts, SLO tracking | 1 week |
| M5 — Hardening & launch | Load test, security review, runbook, internal docs, beta with one team | 1–2 weeks |

Total: ~8–10 weeks for a single platform engineer. Parallelizable if frontend work is split out.

---

## Appendix A — Glossary

- **Environment:** A complete, isolated, short-lived deployment of a single workload, represented by an `Environment` custom resource.
- **Bundle:** The set of Kubernetes resources produced by the curated Helm chart for one environment.
- **Curated chart:** The single Helm chart (`dev-environment`) maintained by the platform team that defines what every environment looks like.
- **TTL:** Time-to-live; the duration after which an environment is automatically deleted.
- **Owner:** The user who created the environment, recorded on `spec.owner` and as a label on every resource.

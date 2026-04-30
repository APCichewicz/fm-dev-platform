{{/*
Common labels stamped onto every resource.
Call with: (dict "root" $ "deploymentName" $name)
*/}}
{{- define "fm-dev-platform.commonLabels" -}}
{{- $root := .root -}}
{{- $name := .deploymentName -}}
app.kubernetes.io/name: {{ $name }}
app.kubernetes.io/instance: {{ $root.Values.env.name }}
app.kubernetes.io/managed-by: {{ $root.Release.Service }}
app.kubernetes.io/part-of: fm-dev-platform
platform.fastmarkets.io/env-name: {{ $root.Values.env.name }}
platform.fastmarkets.io/deployment: {{ $name }}
{{- end -}}

{{/*
Selector labels — minimal, immutable subset.
Call with: (dict "root" $ "deploymentName" $name)
*/}}
{{- define "fm-dev-platform.selectorLabels" -}}
{{- $root := .root -}}
{{- $name := .deploymentName -}}
app.kubernetes.io/name: {{ $name }}
app.kubernetes.io/instance: {{ $root.Values.env.name }}
{{- end -}}

{{/*
Common annotations. Owner (email) and expires-at (RFC3339) live here,
not in labels, since label values reject "@" and ":".
Call with: (dict "root" $)
*/}}
{{- define "fm-dev-platform.commonAnnotations" -}}
{{- $root := .root -}}
platform.fastmarkets.io/owner: {{ $root.Values.env.owner | quote }}
platform.fastmarkets.io/expires-at: {{ $root.Values.env.expiresAt | quote }}
{{- end -}}

{{/*
Pod-level securityContext. Locked down by chart, not user-overridable.
*/}}
{{- define "fm-dev-platform.podSecurityContext" -}}
runAsNonRoot: true
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{/*
Container-level securityContext. Locked down by chart, not user-overridable.
*/}}
{{- define "fm-dev-platform.containerSecurityContext" -}}
runAsNonRoot: true
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities:
  drop:
    - ALL
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{- define "sealos-storage-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sealos-storage-manager.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "sealos-storage-manager.namespace" -}}
{{- default .Release.Namespace .Values.namespace.name -}}
{{- end -}}

{{- define "sealos-storage-manager.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
app.kubernetes.io/name: {{ include "sealos-storage-manager.name" . | quote }}
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end -}}

{{- define "sealos-storage-manager.backendSelectorLabels" -}}
app.kubernetes.io/name: "viewer-backend"
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "sealos-storage-manager.webSelectorLabels" -}}
app.kubernetes.io/name: "viewer-web"
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "sealos-storage-manager.backendServiceAccountName" -}}
{{- .Values.backend.serviceAccount.name -}}
{{- end -}}

{{- define "sealos-storage-manager.backendImage" -}}
{{- printf "%s:%s" .Values.backend.image.repository (.Values.backend.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "sealos-storage-manager.webImage" -}}
{{- printf "%s:%s" .Values.web.image.repository (.Values.web.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "sealos-storage-manager.backendURL" -}}
{{- printf "http://%s.%s.svc.cluster.local" .Values.backend.service.name (include "sealos-storage-manager.namespace" .) -}}
{{- end -}}

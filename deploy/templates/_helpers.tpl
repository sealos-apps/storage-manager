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

{{- define "sealos-storage-manager.backendClusterRoleName" -}}
{{- printf "storage-manager-%s" .Values.backend.serviceAccount.name | trunc 63 | trimSuffix "-" -}}
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

{{- define "sealos-storage-manager.scheme" -}}
{{- if eq (toString .Values.global.disableHttps) "true" -}}http{{- else -}}https{{- end -}}
{{- end -}}

{{- define "sealos-storage-manager.publicPort" -}}
{{- $scheme := include "sealos-storage-manager.scheme" . -}}
{{- $port := toString .Values.global.cloudPort -}}
{{- if eq $scheme "http" -}}
{{- $port = toString .Values.global.httpPort -}}
{{- end -}}
{{- if or (and (eq $scheme "https") (or (eq $port "") (eq $port "443"))) (and (eq $scheme "http") (or (eq $port "") (eq $port "80"))) -}}
{{- "" -}}
{{- else -}}
{{- $port -}}
{{- end -}}
{{- end -}}

{{- define "sealos-storage-manager.publicPortSuffix" -}}
{{- $port := include "sealos-storage-manager.publicPort" . -}}
{{- if $port -}}:{{ $port }}{{- end -}}
{{- end -}}

{{- define "sealos-storage-manager.webHost" -}}
{{- default (printf "storage-manager.%s" .Values.global.cloudDomain) .Values.web.publicHost -}}
{{- end -}}

{{- define "sealos-storage-manager.webOrigin" -}}
{{- include "sealos-storage-manager.scheme" . -}}://{{ include "sealos-storage-manager.webHost" . }}{{ include "sealos-storage-manager.publicPortSuffix" . }}
{{- end -}}

{{- define "sealos-storage-manager.viewerHostTemplate" -}}
{{- printf "%s-{{ .PodSessionID }}.%s" .Values.backend.config.viewer.ingress.hostPrefix .Values.global.cloudDomain -}}
{{- end -}}

{{- define "sealos-storage-manager.viewerTLSSecretName" -}}
{{- if .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- else if not (eq (toString .Values.global.disableHttps) "true") -}}
{{- .Values.global.certSecretName -}}
{{- end -}}
{{- end -}}

{{- define "sealos-storage-manager.backendVerifyURL" -}}
{{- if .Values.backend.config.viewer.backendVerifyUrl -}}
{{- .Values.backend.config.viewer.backendVerifyUrl -}}
{{- else -}}
{{- include "sealos-storage-manager.backendURL" . -}}/internal/filebrowser-hook/verify
{{- end -}}
{{- end -}}

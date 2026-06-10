{{- define "storage-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "storage-manager.fullname" -}}
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

{{- define "storage-manager.namespace" -}}
{{- default .Release.Namespace .Values.namespace.name -}}
{{- end -}}

{{- define "storage-manager.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
app.kubernetes.io/name: {{ include "storage-manager.name" . | quote }}
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end -}}

{{- define "storage-manager.backendSelectorLabels" -}}
app.kubernetes.io/name: "viewer-backend"
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "storage-manager.webSelectorLabels" -}}
app.kubernetes.io/name: "viewer-web"
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "storage-manager.backendServiceAccountName" -}}
{{- .Values.backend.serviceAccount.name -}}
{{- end -}}

{{- define "storage-manager.backendClusterRoleName" -}}
{{- printf "storage-manager-%s" .Values.backend.serviceAccount.name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "storage-manager.backendImage" -}}
{{- printf "%s:%s" .Values.backend.image.repository (.Values.backend.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "storage-manager.webImage" -}}
{{- printf "%s:%s" .Values.web.image.repository (.Values.web.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "storage-manager.backendURL" -}}
{{- printf "http://%s.%s.svc.cluster.local" .Values.backend.service.name (include "storage-manager.namespace" .) -}}
{{- end -}}

{{- define "storage-manager.scheme" -}}
{{- if eq (toString (default false .Values.disableHttps)) "true" -}}http{{- else -}}https{{- end -}}
{{- end -}}

{{- define "storage-manager.publicPort" -}}
{{- $scheme := include "storage-manager.scheme" . -}}
{{- $port := toString (default "" .Values.cloudPort) -}}
{{- if eq $scheme "http" -}}
{{- $port = toString (default "" .Values.httpPort) -}}
{{- end -}}
{{- if or (and (eq $scheme "https") (or (eq $port "") (eq $port "443"))) (and (eq $scheme "http") (or (eq $port "") (eq $port "80"))) -}}
{{- "" -}}
{{- else -}}
{{- $port -}}
{{- end -}}
{{- end -}}

{{- define "storage-manager.publicPortSuffix" -}}
{{- $port := include "storage-manager.publicPort" . -}}
{{- if $port -}}:{{ $port }}{{- end -}}
{{- end -}}

{{- define "storage-manager.webHost" -}}
{{- default (printf "storage-manager.%s" (default "127.0.0.1.nip.io" .Values.cloudDomain)) .Values.web.publicHost -}}
{{- end -}}

{{- define "storage-manager.webOrigin" -}}
{{- include "storage-manager.scheme" . -}}://{{ include "storage-manager.webHost" . }}{{ include "storage-manager.publicPortSuffix" . }}
{{- end -}}

{{- define "storage-manager.viewerHostTemplate" -}}
{{- printf "%s-{{ .PodSessionID }}.%s" .Values.backend.config.viewer.ingress.hostPrefix (default "127.0.0.1.nip.io" .Values.cloudDomain) -}}
{{- end -}}

{{- define "storage-manager.viewerTLSSecretName" -}}
{{- if .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- else if not (eq (toString (default false .Values.disableHttps)) "true") -}}
{{- (default "wildcard-cert" .Values.certSecretName) -}}
{{- end -}}
{{- end -}}

{{- define "storage-manager.backendVerifyURL" -}}
{{- if .Values.backend.config.viewer.backendVerifyUrl -}}
{{- .Values.backend.config.viewer.backendVerifyUrl -}}
{{- else -}}
{{- include "storage-manager.backendURL" . -}}/internal/filebrowser-hook/verify
{{- end -}}
{{- end -}}

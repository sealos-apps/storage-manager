{{- define "storage-manager.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "storage-manager.namespace" -}}
{{- .Release.Namespace -}}
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
{{- $config := default dict .Values.config -}}
{{- $user := default dict .Values.user -}}
{{- $webValues := default dict .Values.web -}}
{{- $configWeb := default dict (get $config "web") -}}
{{- $webUser := default dict (get $user "web") -}}
{{- $publicHost := default (get $webValues "publicHost") (get $config "publicHost") -}}
{{- $publicHost = default $publicHost (get $configWeb "publicHost") -}}
{{- $publicHost = default $publicHost (get $webUser "publicHost") -}}
{{- default (printf "storage-manager.%s" (default "127.0.0.1.nip.io" .Values.cloudDomain)) $publicHost -}}
{{- end -}}

{{- define "storage-manager.webOrigin" -}}
{{- include "storage-manager.scheme" . -}}://{{ include "storage-manager.webHost" . }}{{ include "storage-manager.publicPortSuffix" . }}
{{- end -}}

{{- define "storage-manager.viewerHostTemplate" -}}
{{- $config := default dict .Values.config -}}
{{- $user := default dict .Values.user -}}
{{- $configViewer := default dict (get $config "viewer") -}}
{{- $viewerUser := default dict (get $user "viewer") -}}
{{- $backendConfig := default dict .Values.backend.config -}}
{{- $backendViewer := default dict (get $backendConfig "viewer") -}}
{{- $backendIngress := default dict (get $backendViewer "ingress") -}}
{{- $hostPrefix := default (get $backendIngress "hostPrefix") (get $configViewer "hostPrefix") -}}
{{- $hostPrefix = default $hostPrefix (get $viewerUser "hostPrefix") -}}
{{- printf "%s-{{ .PodSessionID }}.%s" $hostPrefix (default "127.0.0.1.nip.io" .Values.cloudDomain) -}}
{{- end -}}

{{- define "storage-manager.viewerTLSSecretName" -}}
{{- if .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- .Values.backend.config.viewer.ingress.tlsSecretName -}}
{{- else if not (eq (toString (default false .Values.disableHttps)) "true") -}}
{{- (default "wildcard-cert" .Values.certSecretName) -}}
{{- end -}}
{{- end -}}

{{- define "storage-manager.backendVerifyURL" -}}
{{- $config := default dict .Values.config -}}
{{- $user := default dict .Values.user -}}
{{- $configViewer := default dict (get $config "viewer") -}}
{{- $viewerUser := default dict (get $user "viewer") -}}
{{- $backendConfig := default dict .Values.backend.config -}}
{{- $backendViewer := default dict (get $backendConfig "viewer") -}}
{{- $backendVerifyURL := default (get $backendViewer "backendVerifyUrl") (get $configViewer "backendVerifyUrl") -}}
{{- $backendVerifyURL = default $backendVerifyURL (get $viewerUser "backendVerifyUrl") -}}
{{- if $backendVerifyURL -}}
{{- $backendVerifyURL -}}
{{- else -}}
{{- include "storage-manager.backendURL" . -}}/internal/filebrowser-hook/verify
{{- end -}}
{{- end -}}

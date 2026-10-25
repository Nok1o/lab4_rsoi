{{- define "http-service.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name | quote }}
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "http-service.labels" -}}
{{ include "http-service.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
app.kubernetes.io/part-of: library-system
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}

{{- define "http-service.image" -}}
{{- $registry := required "image.registry is required" .Values.image.registry -}}
{{- $namespace := required "Set image.namespace to your Docker Hub user or organization" .Values.image.namespace -}}
{{- $repository := required "image.repository is required" .Values.image.repository -}}
{{- $tag := required "Set image.tag to a published image tag" .Values.image.tag -}}
{{- printf "%s/%s/%s:%s%s" $registry $namespace $repository .Values.image.tagPrefix $tag -}}
{{- end -}}

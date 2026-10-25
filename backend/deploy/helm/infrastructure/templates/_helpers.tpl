{{- define "infrastructure.postgresqlName" -}}
{{- printf "%s-postgresql" (.Release.Name | trunc 40 | trimSuffix "-") -}}
{{- end -}}

{{- define "infrastructure.redisName" -}}
{{- printf "%s-redis" (.Release.Name | trunc 40 | trimSuffix "-") -}}
{{- end -}}

{{- define "infrastructure.labels" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: library-system
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}

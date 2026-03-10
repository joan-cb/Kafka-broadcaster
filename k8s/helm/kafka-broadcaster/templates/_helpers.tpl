{{/*
Expand the name of the chart.
*/}}
{{- define "kafka-broadcaster.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kafka-broadcaster.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if .Values.client }}
{{- printf "%s-%s" $name .Values.client | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kafka-broadcaster.labels" -}}
helm.sh/chart: {{ include "kafka-broadcaster.name" . }}-{{ .Chart.Version }}
{{ include "kafka-broadcaster.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Values.client }}
app.kubernetes.io/client: {{ .Values.client }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kafka-broadcaster.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kafka-broadcaster.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

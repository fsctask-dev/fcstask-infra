{{- define "fcstask-monitoring.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "fcstask-monitoring.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.labels" -}}
helm.sh/chart: {{ include "fcstask-monitoring.chart" . }}
{{ include "fcstask-monitoring.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "fcstask-monitoring.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fcstask-monitoring.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "fcstask-monitoring.prometheus.fullname" -}}
{{- printf "%s-prometheus" (include "fcstask-monitoring.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.alertmanager.fullname" -}}
{{- printf "%s-alertmanager" (include "fcstask-monitoring.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.prometheus.alertmanagerTarget" -}}
{{- printf "%s:9093" (include "fcstask-monitoring.alertmanager.fullname" .) }}
{{- end }}

{{- define "fcstask-monitoring.postgresExporter.fullname" -}}
{{- printf "%s-postgres-exporter" (include "fcstask-monitoring.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.sqlExporter.fullname" -}}
{{- printf "%s-sql-exporter" (include "fcstask-monitoring.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "fcstask-monitoring.database.dataSourceName" -}}
{{- printf "postgresql://%s:%s@%s:%v/%s?sslmode=disable" .Values.database.username .Values.database.password .Values.database.host .Values.database.port .Values.database.database }}
{{- end }}

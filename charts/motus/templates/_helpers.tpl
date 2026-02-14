{{/*
Expand the name of the chart.
*/}}
{{- define "motus.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "motus.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "motus.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "motus.labels" -}}
helm.sh/chart: {{ include "motus.chart" . }}
app.kubernetes.io/name: {{ include "motus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app: {{ include "motus.name" . }}
{{- end }}

{{/*
Selector labels (backward compatible with old format)
*/}}
{{- define "motus.selectorLabels" -}}
app: {{ include "motus.name" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "motus.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "motus.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Postgres component labels
*/}}
{{- define "motus.postgres.labels" -}}
helm.sh/chart: {{ include "motus.chart" . }}
app.kubernetes.io/name: {{ include "motus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: database
app: {{ include "motus.name" . }}-postgres
{{- end }}

{{/*
Postgres selector labels (backward compatible with old format)
*/}}
{{- define "motus.postgres.selectorLabels" -}}
app: {{ include "motus.name" . }}-postgres
{{- end }}

{{/*
Postgres fullname
*/}}
{{- define "motus.postgres.fullname" -}}
{{- printf "%s-postgres" (include "motus.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Redis component labels
*/}}
{{- define "motus.redis.labels" -}}
helm.sh/chart: {{ include "motus.chart" . }}
app.kubernetes.io/name: {{ include "motus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: cache
app: {{ include "motus.name" . }}-redis
{{- end }}

{{/*
Redis selector labels (backward compatible with old format)
*/}}
{{- define "motus.redis.selectorLabels" -}}
app: {{ include "motus.name" . }}-redis
{{- end }}

{{/*
Redis fullname
*/}}
{{- define "motus.redis.fullname" -}}
{{- printf "%s-redis" (include "motus.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Demo component labels
*/}}
{{- define "motus.demo.labels" -}}
helm.sh/chart: {{ include "motus.chart" . }}
app.kubernetes.io/name: {{ include "motus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: gps-simulator
app: {{ include "motus.name" . }}-demo
{{- end }}

{{/*
Demo selector labels (backward compatible with old format)
*/}}
{{- define "motus.demo.selectorLabels" -}}
app: {{ include "motus.name" . }}-demo
{{- end }}

{{/*
Demo fullname
*/}}
{{- define "motus.demo.fullname" -}}
{{- printf "%s-demo" (include "motus.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Database host helper
*/}}
{{- define "motus.database.host" -}}
{{- if .Values.postgres.enabled }}
{{- include "motus.postgres.fullname" . }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Database port helper
*/}}
{{- define "motus.database.port" -}}
{{- if .Values.postgres.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.externalDatabase.port }}
{{- end }}
{{- end }}

{{/*
Database name helper
*/}}
{{- define "motus.database.name" -}}
{{- if .Values.postgres.enabled }}
{{- .Values.postgres.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Database user helper
*/}}
{{- define "motus.database.user" -}}
{{- if .Values.postgres.enabled }}
{{- .Values.postgres.username }}
{{- else }}
{{- .Values.externalDatabase.username }}
{{- end }}
{{- end }}

{{/*
Database password helper
*/}}
{{- define "motus.database.password" -}}
{{- if .Values.postgres.enabled }}
{{- .Values.postgres.password }}
{{- else }}
{{- .Values.externalDatabase.password }}
{{- end }}
{{- end }}

{{/*
Database sslmode helper
*/}}
{{- define "motus.database.sslmode" -}}
{{- if .Values.postgres.enabled }}
{{- "disable" }}
{{- else }}
{{- .Values.externalDatabase.sslmode }}
{{- end }}
{{- end }}

{{/*
Redis URL helper
*/}}
{{- define "motus.redis.url" -}}
{{- if .Values.redis.external.enabled }}
{{- .Values.redis.external.url }}
{{- else }}
{{- printf "redis://%s:6379/0" (include "motus.redis.fullname" .) }}
{{- end }}
{{- end }}

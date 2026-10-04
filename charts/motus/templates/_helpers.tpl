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
{{- define "motus.baseLabels" -}}
helm.sh/chart: {{ include "motus.chart" . }}
app.kubernetes.io/name: {{ include "motus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "motus.labels" -}}
{{ include "motus.baseLabels" . }}
app: {{ include "motus.name" . }}
{{- end }}

{{/*
Component labels. Usage: include "motus.componentLabels" (list . "<component>" "<app suffix>")
*/}}
{{- define "motus.componentLabels" -}}
{{- $ctx := index . 0 -}}
{{ include "motus.baseLabels" $ctx }}
app.kubernetes.io/component: {{ index . 1 }}
app: {{ include "motus.name" $ctx }}-{{ index . 2 }}
{{- end }}

{{/*
Selector labels (backward compatible with old format)
*/}}
{{- define "motus.selectorLabels" -}}
app: {{ include "motus.name" . }}
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
{{- required "postgres.password is required — no default is provided" .Values.postgres.password }}
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

{{/*
Database connection env vars
*/}}
{{- define "motus.databaseEnv" -}}
{{- if .Values.postgresUriSecret.enabled -}}
- name: POSTGRES_URI
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresUriSecret.name }}
      key: {{ .Values.postgresUriSecret.key }}
{{- else -}}
- name: MOTUS_DATABASE_HOST
  value: {{ include "motus.database.host" . | quote }}
- name: MOTUS_DATABASE_PORT
  value: {{ include "motus.database.port" . | quote }}
- name: MOTUS_DATABASE_NAME
  value: {{ include "motus.database.name" . | quote }}
- name: MOTUS_DATABASE_USER
  value: {{ include "motus.database.user" . | quote }}
{{- if .Values.externalDatabase.existingSecret }}
- name: MOTUS_DATABASE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.externalDatabase.existingSecret }}
      key: {{ .Values.externalDatabase.existingSecretKey | default "password" }}
{{- else }}
- name: MOTUS_DATABASE_PASSWORD
  value: {{ include "motus.database.password" . | quote }}
{{- end }}
- name: MOTUS_DATABASE_SSLMODE
  value: {{ include "motus.database.sslmode" . | quote }}
{{- end }}
{{- end }}

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
Component selector labels. Usage: include "motus.componentSelectorLabels" (list . "<component>")
*/}}
{{- define "motus.componentSelectorLabels" -}}
app: {{ include "motus.name" (index . 0) }}-{{ index . 1 }}
{{- end }}

{{/*
Component fullname. Usage: include "motus.componentFullname" (list . "<component>")
*/}}
{{- define "motus.componentFullname" -}}
{{- printf "%s-%s" (include "motus.fullname" (index . 0)) (index . 1) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Container image reference
*/}}
{{- define "motus.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}{{ with .Values.image.digest }}@{{ . }}{{ end }}
{{- end }}

{{/*
Redis URL helper
*/}}
{{- define "motus.redis.url" -}}
{{- if .Values.redis.external.enabled }}
{{- .Values.redis.external.url }}
{{- else }}
{{- printf "redis://%s:6379/0" (include "motus.componentFullname" (list . "redis")) }}
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
{{- $db := .Values.externalDatabase }}
{{- if .Values.postgres.enabled }}
{{- $db = dict "host" (include "motus.componentFullname" (list . "postgres")) "port" "5432" "database" .Values.postgres.database "username" .Values.postgres.username "sslmode" "disable" }}
{{- end -}}
- name: MOTUS_DATABASE_HOST
  value: {{ $db.host | quote }}
- name: MOTUS_DATABASE_PORT
  value: {{ $db.port | quote }}
- name: MOTUS_DATABASE_NAME
  value: {{ $db.database | quote }}
- name: MOTUS_DATABASE_USER
  value: {{ $db.username | quote }}
{{- if .Values.externalDatabase.existingSecret }}
- name: MOTUS_DATABASE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.externalDatabase.existingSecret }}
      key: {{ .Values.externalDatabase.existingSecretKey | default "password" }}
{{- else if .Values.postgres.enabled }}
- name: MOTUS_DATABASE_PASSWORD
  value: {{ required "postgres.password is required — no default is provided" .Values.postgres.password | quote }}
{{- else }}
- name: MOTUS_DATABASE_PASSWORD
  value: {{ .Values.externalDatabase.password | quote }}
{{- end }}
- name: MOTUS_DATABASE_SSLMODE
  value: {{ $db.sslmode | quote }}
{{- end }}
{{- end }}

{{- define "lp.labels" -}}
app.kubernetes.io/part-of: lakehouse-platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* Env vars exposing the CloudNativePG app user's password as PG_PASSWORD. */}}
{{- define "lp.pgEnv" -}}
- name: PG_USER
  valueFrom: {secretKeyRef: {name: lp-pg-app, key: username}}
- name: PG_PASSWORD
  valueFrom: {secretKeyRef: {name: lp-pg-app, key: password}}
{{- end -}}

{{- define "lp.s3Env" -}}
- name: AWS_ACCESS_KEY_ID
  valueFrom: {secretKeyRef: {name: {{ .Values.s3.credentialsSecret }}, key: {{ .Values.s3.accessKeyKey }}}}
- name: AWS_SECRET_ACCESS_KEY
  valueFrom: {secretKeyRef: {name: {{ .Values.s3.credentialsSecret }}, key: {{ .Values.s3.secretKeyKey }}}}
{{- end -}}

{{- define "lp.lakekeeperEnv" -}}
{{ include "lp.pgEnv" . }}
- name: LAKEKEEPER__PG_DATABASE_URL_READ
  value: postgresql://$(PG_USER):$(PG_PASSWORD)@lp-pg-ro:5432/lakekeeper
- name: LAKEKEEPER__PG_DATABASE_URL_WRITE
  value: postgresql://$(PG_USER):$(PG_PASSWORD)@lp-pg-rw:5432/lakekeeper
- name: LAKEKEEPER__PG_ENCRYPTION_KEY
  valueFrom: {secretKeyRef: {name: lp-lakekeeper, key: encryptionKey}}
- name: LAKEKEEPER__BASE_URI
  value: http://lp-lakekeeper.{{ .Release.Namespace }}.svc:8181
{{- end -}}

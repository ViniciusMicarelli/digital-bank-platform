{{- define "db.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/part-of: digital-bank
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
helm.sh/chart: {{ .root.Chart.Name }}-{{ .root.Chart.Version }}
{{- end }}

{{- define "db.selector" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/part-of: digital-bank
{{- end }}

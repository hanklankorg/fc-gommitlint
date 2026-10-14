{{ if .Versions -}}

# Changelog

All notable changes to this project will be documented in this file.
{{ if .Unreleased.CommitGroups -}}
<a name="unreleased"></a>

## [Unreleased]

{{ range .Unreleased.CommitGroups -}}

### {{ .Title }}

{{ range .Commits -}}
{{- /* Skip release, deps, pr, pull commits */ -}}
{{ if not (or (eq .Scope "release") (eq .Scope "deps") (hasPrefix .Scope "deps-") (eq .Scope "pr") (eq .Scope "pull")) -}}

- {{ if .Scope }}*({{ .Scope }})* {{ end }}{{ if .Notes }}[**breaking**] {{ end }}{{ .Subject }}
{{ end -}}
{{ end }}
{{ end -}}
{{ end -}}
{{ range .Versions }}
<a name="{{ .Tag.Name }}"></a>

## {{ if .Tag.Previous }}[{{ .Tag.Name }}]{{ else }}{{ .Tag.Name }}{{ end }} - {{ datetime "2006-01-02" .Tag.Date }}

{{ range .CommitGroups -}}

### {{ .Title }}

{{ range .Commits -}}
{{- /* Skip release, deps, pr, pull commits */ -}}
{{ if not (or (eq .Scope "release") (eq .Scope "deps") (hasPrefix .Scope "deps-") (eq .Scope "pr") (eq .Scope "pull")) -}}

- {{ if .Scope }}*({{ .Scope }})* {{ end }}{{ if .Notes }}[**breaking**] {{ end }}{{ .Subject }}
{{ end -}}
{{ end }}
{{ end -}}
{{- if .NoteGroups -}}
{{ range .NoteGroups -}}

### {{ .Title }}

{{ range .Notes }}
{{ .Body }}
{{ end }}
{{ end -}}
{{ end -}}
{{ end -}}

{{- if .Versions }}
[Unreleased]: {{ .Info.RepositoryURL }}/compare/{{ $latest := index .Versions 0 }}{{ $latest.Tag.Name }}...HEAD
{{ range .Versions -}}
{{ if .Tag.Previous -}}
[{{ .Tag.Name }}]: {{ $.Info.RepositoryURL }}/compare/{{ .Tag.Previous.Name }}...{{ .Tag.Name }}
{{ end -}}
{{ end -}}
{{ end -}}
{{ end -}}

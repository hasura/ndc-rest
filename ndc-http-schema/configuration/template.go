package configuration

import (
	"embed"
	"text/template"
)

//go:embed all:templates/* templates
var templateFS embed.FS
var _templates *template.Template

const (
	templateEmptySettings = "server_empty.gotmpl"
	templateEnvVariables  = "env_variables.gotmpl"
)

func getTemplates() (*template.Template, error) {
	if _templates != nil {
		return _templates, nil
	}

	var err error
	_templates, err = template.ParseFS(templateFS, "templates/*.gotmpl")
	if err != nil {
		return nil, err
	}

	return _templates, nil
}

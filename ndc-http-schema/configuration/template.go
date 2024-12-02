package configuration

import (
	"embed"
	"io"
	"text/template"
)

//go:embed all:templates/* templates
var templateFS embed.FS
var _templates *template.Template

const (
	templateEmptySettings = "server_empty.gotmpl"
	templateEnvVariables  = "env_variables.gotmpl"
)

const (
	ansiReset          = "\033[0m"
	ansiFaint          = "\033[2m"
	ansiResetFaint     = "\033[22m"
	ansiBrightRed      = "\033[91m"
	ansiBrightGreen    = "\033[92m"
	ansiBrightYellow   = "\033[93m"
	ansiBrightRedFaint = "\033[91;2m"
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

func writeColorTextIf(w io.Writer, text string, color string, noColor bool) {
	if noColor {
		_, _ = w.Write([]byte(text))

		return
	}

	_, _ = w.Write([]byte(color))
	_, _ = w.Write([]byte(text))
	_, _ = w.Write([]byte(ansiReset))
}

func writeErrorIf(w io.Writer, text string, noColor bool) {
	writeColorTextIf(w, "ERROR", ansiBrightRed, noColor)
	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

func writeWarningIf(w io.Writer, text string, noColor bool) {
	writeColorTextIf(w, "WARNING", ansiBrightYellow, noColor)
	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

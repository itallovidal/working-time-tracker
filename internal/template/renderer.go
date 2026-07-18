package template

import (
	"html/template"
	"io"
	"io/fs"

	"github.com/labstack/echo/v5"
)

type Renderer struct {
	templates *template.Template
}

func NewRendererFromFS(fsys fs.FS, patterns ...string) *Renderer {
	tmpl := template.Must(template.New("").ParseFS(fsys, patterns...))
	return &Renderer{templates: tmpl}
}

func (r *Renderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

package template

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/labstack/echo/v5"
)

// Renderer guarda um conjunto de templates por página. Cada conjunto tem os
// layouts, os partials e uma única página, então todas podem definir o bloco
// "content" sem colidir.
type Renderer struct {
	pages map[string]*template.Template
}

// New lê templates/layouts/*, templates/partials/* e cada templates/pages/*.gohtml.
// Um template com erro faz a inicialização falhar, e não a primeira requisição.
func New(fsys fs.FS) (*Renderer, error) {
	base, err := template.New("").ParseFS(fsys, "templates/layouts/*.gohtml", "templates/partials/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse layouts: %w", err)
	}
	files, err := fs.Glob(fsys, "templates/pages/*.gohtml")
	if err != nil {
		return nil, err
	}
	pages := make(map[string]*template.Template, len(files))
	for _, f := range files {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(fsys, f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		pages[strings.TrimSuffix(path.Base(f), ".gohtml")] = t
	}
	return &Renderer{pages: pages}, nil
}

// Pages lista as páginas carregadas, para os testes conferirem que todas existem.
func (r *Renderer) Pages() []string {
	names := make([]string, 0, len(r.pages))
	for name := range r.pages {
		names = append(names, name)
	}
	return names
}

func (r *Renderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	t, ok := r.pages[name]
	if !ok {
		return fmt.Errorf("template page %q not found", name)
	}
	return t.ExecuteTemplate(w, "base", data)
}

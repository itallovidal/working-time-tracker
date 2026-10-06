package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	amigrate "ariga.io/atlas/sql/migrate"
	atlas "ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"

	entmigrate "working-time-tracker/ent/migrate"
)

var migrationName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// upOnly é o formato do goose só com a seção Up. As migrações andam só para frente:
// desfazer uma mudança é escrever uma migração nova.
var upOnly = func() amigrate.Formatter {
	funcs := template.FuncMap{"now": amigrate.NewVersion}
	f, err := amigrate.NewTemplateFormatter(
		template.Must(template.New("name").Funcs(funcs).Parse(
			"{{ now }}{{ with .Name }}_{{ . }}{{ end }}.sql")),
		template.Must(template.New("content").Parse(
			"-- +goose Up\n{{ range .Changes }}{{ with .Comment }}-- {{ println . }}{{ end }}{{ printf \"%s;\\n\" .Cmd }}{{ end }}")),
	)
	if err != nil {
		panic(err)
	}
	return f
}()

// newMigration compara o que os arquivos de migração produzem com o ent/schema e
// grava a diferença num arquivo novo. Não toca no banco do DATABASE_URL: usa um banco
// temporário no mesmo servidor.
func newMigration(ctx context.Context, dsn, name string) error {
	if !migrationName.MatchString(name) {
		return fmt.Errorf("nome inválido %q: use minúsculas, dígitos e _ (ex.: add_invoice_number)", name)
	}
	dir, err := sqltool.NewGooseDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("%s: %w (rode da raiz do repositório)", migrationsDir, err)
	}
	if err := amigrate.Validate(dir); err != nil {
		return fmt.Errorf("%s: %w (se a edição foi de propósito, rode `go run ./cmd/migrate checksum`)", migrationsDir, err)
	}
	before, err := fileNames(dir)
	if err != nil {
		return err
	}

	scratch, cleanup, err := createScratch(ctx, dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	var report diffReport
	m, err := schema.NewMigrate(entsql.OpenDB(dialect.Postgres, scratch),
		schema.WithDir(dir),
		schema.WithFormatter(upOnly),
		schema.WithMigrationMode(schema.ModeReplay),
		// Sem estes dois, remover um campo ou um índice do ent/schema não geraria nada.
		// Aqui o DROP não é aplicado: fica escrito no arquivo, para revisão.
		schema.WithDropColumn(true),
		schema.WithDropIndex(true),
		schema.WithIndent("  "),
		schema.WithErrNoPlan(true),
		schema.WithDiffHook(report.hook),
	)
	if err != nil {
		return err
	}
	err = m.NamedDiff(ctx, name, entmigrate.Tables...)
	report.print()
	if errors.Is(err, amigrate.ErrNoPlan) {
		fmt.Println("nada a gerar: as migrações já produzem o schema do ent/schema")
		return nil
	}
	if err != nil {
		return fmt.Errorf("gerando a migração: %w", err)
	}

	after, err := fileNames(dir)
	if err != nil {
		return err
	}
	for _, f := range after {
		if !slices.Contains(before, f) {
			fmt.Printf("criado %s\n", filepath.Join(migrationsDir, f))
		}
	}
	fmt.Println("revise o SQL antes de commitar; se editar à mão, rode `go run ./cmd/migrate checksum`")
	return nil
}

// writeChecksum recalcula o atlas.sum depois de uma edição manual nos arquivos.
func writeChecksum(path string) error {
	dir, err := sqltool.NewGooseDir(path)
	if err != nil {
		return fmt.Errorf("%s: %w (rode da raiz do repositório)", path, err)
	}
	sum, err := dir.Checksum()
	if err != nil {
		return err
	}
	if err := amigrate.WriteSumFile(dir, sum); err != nil {
		return err
	}
	fmt.Printf("%s atualizado\n", filepath.Join(path, amigrate.HashFileName))
	return nil
}

func fileNames(dir amigrate.Dir) ([]string, error) {
	files, err := dir.Files()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name()
	}
	return names, nil
}

// diffReport junta avisos sobre as mudanças que podem perder dados ou falhar num
// banco com linhas. O gerador não sabe o que há nas tabelas; quem decide é a revisão.
type diffReport struct {
	warnings []string
}

func (r *diffReport) hook(next schema.Differ) schema.Differ {
	return schema.DiffFunc(func(current, desired *atlas.Schema) ([]atlas.Change, error) {
		changes, err := next.Diff(current, desired)
		if err != nil {
			return nil, err
		}
		r.inspect(changes)
		return changes, nil
	})
}

func (r *diffReport) warn(format string, args ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

func (r *diffReport) inspect(changes []atlas.Change) {
	for _, c := range changes {
		switch c := c.(type) {
		case *atlas.DropTable:
			r.warn("tabela %s saiu do ent/schema: o Ent não gera DROP TABLE; escreva à mão se for para apagar", c.T.Name)
		case *atlas.ModifyTable:
			r.inspectTable(c)
		}
	}
}

func (r *diffReport) inspectTable(t *atlas.ModifyTable) {
	table := t.T.Name
	var dropped, added []string
	for _, c := range t.Changes {
		switch c := c.(type) {
		case *atlas.DropColumn:
			dropped = append(dropped, c.C.Name)
			r.warn("%s.%s: DROP COLUMN apaga os dados da coluna", table, c.C.Name)
		case *atlas.AddColumn:
			added = append(added, c.C.Name)
			if !c.C.Type.Null && c.C.Default == nil {
				r.warn("%s.%s: coluna NOT NULL sem default falha se a tabela tiver linhas; crie nullable, preencha e só então SET NOT NULL", table, c.C.Name)
			}
		case *atlas.ModifyColumn:
			if c.Change.Is(atlas.ChangeType) {
				r.warn("%s.%s: mudança de tipo; confira se os valores existentes convertem", table, c.To.Name)
			}
			if c.Change.Is(atlas.ChangeNull) && !c.To.Type.Null {
				r.warn("%s.%s: SET NOT NULL falha se houver NULL; preencha antes, na mesma migração", table, c.To.Name)
			}
		case *atlas.DropIndex:
			r.warn("%s: DROP INDEX %s", table, c.I.Name)
		case *atlas.DropForeignKey:
			r.warn("%s: DROP CONSTRAINT %s (chave estrangeira)", table, c.F.Symbol)
		case *atlas.ModifyForeignKey:
			r.warn("%s: a chave estrangeira %s muda de regra; confira ON DELETE", table, c.To.Symbol)
		}
	}
	if len(dropped) > 0 && len(added) > 0 {
		r.warn("%s: remove (%s) e adiciona (%s) colunas; se for rename, troque por ALTER TABLE ... RENAME COLUMN para não perder os dados",
			table, strings.Join(dropped, ", "), strings.Join(added, ", "))
	}
}

func (r *diffReport) print() {
	if len(r.warnings) == 0 {
		return
	}
	fmt.Println("ATENÇÃO: esta migração mexe em dados existentes")
	for _, w := range r.warnings {
		fmt.Println("  - " + w)
	}
}

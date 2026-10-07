// Package permission é o catálogo do que cada pessoa pode fazer, além do que todo
// mundo já faz (ver as próprias tarefas, bater o próprio ponto).
//
// O dono e os admins da organização podem tudo. Os outros têm só o que foi liberado, em
// dois escopos: o do projeto, que vale num projeto só e fica na alocação da pessoa nele,
// e o da organização, que vale em todos e fica na própria pessoa. Ao adicionar alguém a
// um projeto escolhe-se um grupo (Preset), que é uma lista pronta de permissões do
// projeto; cada permissão avulsa pode vir depois.
package permission

import "slices"

// Permissões do escopo do projeto.
const (
	ProjectEdit         = "project.edit"         // alterar e excluir o projeto, e a aba Configurações
	TeamsManage         = "teams.manage"         // criar, editar e excluir times e escolher quem está neles
	CollaboratorsManage = "collaborators.manage" // pôr e tirar pessoas do projeto, e escolher o grupo delas
	RatesView           = "rates.view"           // ver o valor pago aos outros e as horas de todos
	RatesManage         = "rates.manage"         // definir o valor pago por hora
	BillingView         = "billing.view"         // ver o valor cobrado, a receita, o custo e a margem
	BillingManage       = "billing.manage"       // definir o cliente e o valor cobrado
	LabelsManage        = "labels.manage"        // criar, renomear e excluir as etiquetas
	IntegrationsManage  = "integrations.manage"  // criar, editar e excluir as integrações
)

// Permissões do escopo da organização.
const (
	ProjectsCreate  = "projects.create"  // criar projetos
	CustomersManage = "customers.manage" // cadastrar e alterar os clientes
	PeopleManage    = "people.manage"    // convidar pessoas e definir a jornada semanal
)

// ProjectKeys e OrganizationKeys são as permissões de cada escopo, na ordem em que
// as telas as mostram.
var (
	ProjectKeys = []string{
		ProjectEdit, TeamsManage, CollaboratorsManage, RatesView, RatesManage,
		BillingView, BillingManage, LabelsManage, IntegrationsManage,
	}
	OrganizationKeys = []string{ProjectsCreate, CustomersManage, PeopleManage}
)

// Os grupos pré-definidos do projeto.
const (
	PresetMember  = "member"
	PresetManager = "manager"
	PresetFinance = "finance"
	PresetAdmin   = "admin"
	// PresetCustom é a lista que não é de nenhum grupo: alguém mexeu numa permissão avulsa.
	PresetCustom = "custom"
)

// Preset é um grupo: um nome e as permissões do projeto que ele dá.
type Preset struct {
	ID          string   `json:"id"`
	Permissions []string `json:"permissions"`
}

// Presets são os grupos que a tela oferece, do que dá menos para o que dá mais.
var Presets = []Preset{
	{PresetMember, []string{}},
	{PresetManager, []string{ProjectEdit, TeamsManage, CollaboratorsManage, RatesView, RatesManage, LabelsManage, IntegrationsManage}},
	{PresetFinance, []string{RatesView, RatesManage, BillingView, BillingManage}},
	{PresetAdmin, ProjectKeys},
}

// PresetByID devolve o grupo de nome id.
func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetOf diz de qual grupo é a lista: o primeiro que tem exatamente essas permissões,
// ou "custom" quando nenhum tem.
func PresetOf(keys []string) string {
	keys = Normalize(keys, ProjectKeys)
	for _, p := range Presets {
		if slices.Equal(Normalize(p.Permissions, ProjectKeys), keys) {
			return p.ID
		}
	}
	return PresetCustom
}

// Normalize deixa só as permissões de allowed, sem repetir e na ordem do catálogo.
func Normalize(keys, allowed []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range allowed {
		if slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	return out
}

// Valid diz se toda permissão de keys existe em allowed.
func Valid(keys, allowed []string) bool {
	for _, k := range keys {
		if !slices.Contains(allowed, k) {
			return false
		}
	}
	return true
}

// Set é o que uma pessoa pode fazer num projeto. All vale para o dono e os admins.
type Set struct {
	All  bool
	Keys []string
}

// Has diz se a permissão está no conjunto.
func (s Set) Has(key string) bool {
	return s.All || slices.Contains(s.Keys, key)
}

// HasAny diz se alguma das permissões está no conjunto.
func (s Set) HasAny(keys ...string) bool {
	for _, k := range keys {
		if s.Has(k) {
			return true
		}
	}
	return false
}

// Manages diz se o conjunto tem alguma permissão do projeto: quem tem vê o que o time
// todo registrou, e não só o próprio ponto.
func (s Set) Manages() bool {
	return s.All || len(s.Keys) > 0
}

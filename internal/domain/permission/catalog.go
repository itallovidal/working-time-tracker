// Package permission é o catálogo do que cada pessoa pode fazer, além do que todo
// mundo já faz (ver as próprias tarefas, bater o próprio ponto).
//
// Os cargos são quatro: o dono (tudo, inclusive o dinheiro), o admin (cria projetos internos, convida pessoas e
// administra todos os projetos, sem cliente, cobrança nem pagamentos), o administrador de projeto (o grupo manager
// de um projeto) e o colaborador. As permissões têm dois escopos: o do projeto, que vale num projeto só e fica na
// alocação da pessoa nele, e o da organização, que decorre do cargo (OrgKeysFor) e não é dada pessoa a pessoa. Ao
// adicionar alguém a um projeto escolhe-se um grupo (Preset), que é uma lista pronta de permissões do projeto;
// cada permissão avulsa pode vir depois.
package permission

import "slices"

// Permissões do escopo do projeto.
const (
	ProjectEdit         = "project.edit"         // alterar o projeto, e a aba Configurações
	ProjectDelete       = "project.delete"       // excluir o projeto, com as horas e os valores dele
	TeamsManage         = "teams.manage"         // criar, editar e excluir times e escolher quem está neles
	CollaboratorsManage = "collaborators.manage" // pôr e tirar pessoas do projeto, e escolher o grupo delas
	RatesView           = "rates.view"           // ver o valor pago aos outros e as horas de todos
	RatesManage         = "rates.manage"         // definir o valor pago por hora
	BillingView         = "billing.view"         // ver o valor cobrado, a receita, o custo e a margem
	BillingManage       = "billing.manage"       // definir o cliente e o valor cobrado
	LabelsManage        = "labels.manage"        // criar, renomear e excluir as etiquetas
	IntegrationsManage  = "integrations.manage"  // criar, editar e excluir as integrações
)

// Permissões do escopo da organização. Decorrem do cargo (OrgKeysFor): ninguém as recebe avulsas.
const (
	ProjectsCreate  = "projects.create"  // criar projetos internos (o dono cria também com cliente)
	CustomersManage = "customers.manage" // cadastrar e alterar os clientes: só o dono
	PeopleManage    = "people.manage"    // convidar pessoas e definir a jornada semanal
	PaymentsManage  = "payments.manage"  // os pagamentos da equipe e a regra de pagamento de cada pessoa: só o dono
)

// ProjectKeys e OrganizationKeys são as permissões de cada escopo, na ordem em que
// as telas as mostram.
var (
	ProjectKeys = []string{
		ProjectEdit, ProjectDelete, TeamsManage, CollaboratorsManage, RatesView, RatesManage,
		BillingView, BillingManage, LabelsManage, IntegrationsManage,
	}
	// OrganizationKeys é o discriminador de Data.Can e do JavaScript: uma chave daqui vale na organização inteira, e
	// não no projeto da página.
	OrganizationKeys = []string{ProjectsCreate, CustomersManage, PeopleManage, PaymentsManage}
	// AdminProjectKeys é o que o admin tem em todo projeto: tudo do projeto, menos o dinheiro de cobrança (ver e
	// definir o cliente e o valor cobrado), que é do dono. O custo de cada pessoa (rates.*) ele vê e define.
	AdminProjectKeys = []string{
		ProjectEdit, ProjectDelete, TeamsManage, CollaboratorsManage, RatesView, RatesManage,
		LabelsManage, IntegrationsManage,
	}
)

// OrgKeysFor devolve as permissões da organização do cargo: o dono tem todas; o admin cria projeto (interno) e cuida
// das pessoas; o colaborador, nenhuma. Nada disso fica gravado na pessoa.
func OrgKeysFor(role string, isOwner bool) []string {
	switch {
	case isOwner:
		return append([]string{}, OrganizationKeys...)
	case role == "admin":
		return []string{ProjectsCreate, PeopleManage}
	}
	return []string{}
}

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

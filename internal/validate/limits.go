package validate

// Os tamanhos máximos dos campos de texto e do valor em dinheiro, num lugar só. O servidor confere por eles, a tela
// os lê pela função de template `limit` (maxlength) e pelo JavaScript em window.BOOT.limits, e o teste de web/ falha
// se um maxlength literal reaparecer num template. Os tamanhos são em caracteres (runas), não em bytes.
const (
	// MaxName é o nome de uma pessoa, projeto, time, integração, cliente ou organização, e da pessoa de contato.
	MaxName = 120
	// MaxTaskName é o nome de uma tarefa. A importação do GitHub e do Trello corta o título neste tamanho.
	MaxTaskName = 255
	// MaxLabel é o nome de um rótulo.
	MaxLabel = 50
	// MaxEmail é o e-mail de uma pessoa, de um convite ou de contato.
	MaxEmail = 255
	// MaxURL é o site e o LinkedIn de uma organização.
	MaxURL = 255
	// MaxItemURL é o link do item externo (uma issue, um cartão), que costuma ser mais longo.
	MaxItemURL = 2048
	// MaxExternalItem é o número ou o código do item externo.
	MaxExternalItem = 128
	// MaxToken é o token de uma integração.
	MaxToken = 512
	// MaxLegalName e MaxAddress são a razão social e as linhas do endereço da organização e do cliente.
	MaxLegalName = 200
	MaxAddress   = 200
	// MaxSummary é o resumo de uma organização.
	MaxSummary = 160
	// MaxDescription é a descrição de uma organização e de um projeto.
	MaxDescription = 2000
	// MaxTaskDescription é a descrição de uma tarefa.
	MaxTaskDescription = 65536
	// MaxSearch é a busca da lista de tarefas.
	MaxSearch = 100
	// MaxRepo é o repositório, projeto ou quadro de uma integração.
	MaxRepo = 255
	// MaxPhone é o telefone do cliente; MinPhone é o mínimo de caracteres e MinPhoneDigits o mínimo de dígitos.
	MaxPhone       = 32
	MinPhone       = 8
	MinPhoneDigits = 7
	// MaxTaxID é o documento fiscal de um país sem regra própria. O CNPJ e o EIN têm o tamanho da máscara.
	MaxTaxID = 32
	// MaxCents é o teto de um valor em dinheiro (valor por hora, valor cobrado): 1.000.000,00, em centavos.
	MaxCents = 100_000_000
)

// Limits são os tamanhos que a tela precisa, por nome, em window.BOOT.limits e na função de template limit.
func Limits() map[string]int {
	return map[string]int{
		"name": MaxName, "task_name": MaxTaskName, "label": MaxLabel, "email": MaxEmail, "url": MaxURL,
		"item_url": MaxItemURL, "external_item": MaxExternalItem, "token": MaxToken, "legal_name": MaxLegalName,
		"address": MaxAddress, "summary": MaxSummary, "description": MaxDescription,
		"task_description": MaxTaskDescription, "search": MaxSearch, "repo": MaxRepo, "phone": MaxPhone,
		"phone_min": MinPhone, "phone_min_digits": MinPhoneDigits, "tax_id": MaxTaxID, "max_cents": MaxCents,
	}
}

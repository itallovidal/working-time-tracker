# Códigos de erro da API

Este arquivo é gerado por `internal/server/errors_doc_test.go` a partir dos códigos declarados no
código (`apperr.New`) e dos textos de `internal/i18n/locales`. Não edite a tabela à mão: depois de
mexer num código ou num texto, rode

```
UPDATE_ERROR_DOCS=1 go test ./internal/server -run ErrorCodesDoc
```

## Formato da resposta

A API **não manda mensagem**: manda um código estável, e quem chama mostra o texto no idioma
da pessoa. Toda resposta de erro tem este corpo, com o status HTTP da tabela:

```json
{ "error": { "code": "organization.field_too_long", "params": { "field": "summary", "max": 160 } } }
```

- `code` é `domínio.motivo`, em minúsculas e com `_`. Vale como contrato: um código existente não muda de
  significado nem some sem aviso.
- `params` só existe quando o texto precisa de valores, e traz exatamente os parâmetros da coluna
  *Parâmetros*. O parâmetro `field` é o nome de um campo da API; o cliente mostra o rótulo dele.
- Um erro que a API não previu vira `internal.server_error` (500), sem nenhum texto interno. O detalhe fica
  só no log do servidor.
- `GET /api/tasks/:taskId/external-details` responde 200 mesmo quando a plataforma falha: o campo
  `error` dessa resposta tem o mesmo formato (`{"code": "...", "params": {...}}`).

## Como o cliente mostra a mensagem

O navegador procura o texto em `errors.<código>` no catálogo do idioma (`WTT.errorText` em `app.js`),
troca os `{{.parâmetro}}` pelos valores e, para `field`, usa o rótulo `fields.<campo>`. Um código sem
texto mostra `errors.unknown`. Outro cliente da API pode ter a própria tabela de textos.

## Como criar um código

1. Declare no `errors.go` do domínio: `ErrX = apperr.New("dominio.motivo", http.StatusBadRequest, "param")`.
2. Devolva o erro no service (`return ErrX` ou `ErrX.With("param", valor)`); o handler responde com
   `apperr.Respond(c, status, err)`.
3. Escreva o texto em `errors.dominio.motivo` nos dois `locales/*.yaml`, só com os placeholders declarados.
4. Rode o comando acima e commite este arquivo. Os testes recusam um código sem texto, um texto sem código,
   um placeholder que o código não declara e este arquivo desatualizado.

## Códigos

### allocation

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `allocation.invalid_preset` | 400 | — | `grupo de permissões desconhecido` | `unknown permission group` |
| `allocation.invalid_rate` | 400 | — | `o valor por hora deve ficar entre 0 e 1.000.000,00` | `the hourly rate must be between 0 and 1,000,000.00` |
| `allocation.not_defined` | 404 | — | `esta pessoa não tem valor definido neste projeto` | `this person has no rate set on this project` |
| `allocation.own_rates_only` | 403 | — | `você só pode ver os seus próprios valores` | `you can only see your own rates` |
| `allocation.person_not_in_org` | 400 | — | `pessoa não encontrada nesta organização` | `person not found in this organization` |
| `allocation.preset_above_yours` | 403 | — | `o grupo dá uma permissão que você não tem` | `the group gives a permission you do not have` |
| `allocation.rate_required` | 400 | — | `informe o valor por hora` | `enter the hourly rate` |

### auth

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `auth.account_exists` | 409 | — | `já existe uma conta com este email` | `an account with this email already exists` |
| `auth.admin_only` | 403 | — | `só admins podem fazer isso` | `only admins can do this` |
| `auth.clerk_account_linked` | 409 | — | `este email já está ligado a outro usuário do login` | `this email is already linked to another sign-in user` |
| `auth.clerk_disabled` | 404 | — | `o login externo não está ligado neste servidor` | `external sign-in is not turned on on this server` |
| `auth.clerk_email_unverified` | 403 | — | `o seu email ainda não foi verificado. Verifique-o e tente de novo` | `your email has not been verified yet. Verify it and try again` |
| `auth.clerk_invite_failed` | 502 | — | `o serviço de login não aceitou o convite. Confira o email e tente de novo` | `the sign-in service did not accept the invitation. Check the email and try again` |
| `auth.clerk_token_invalid` | 401 | — | `não foi possível confirmar o seu login. Entre de novo` | `could not confirm your sign-in. Sign in again` |
| `auth.clerk_unavailable` | 502 | — | `o serviço de login não respondeu. Tente de novo em instantes` | `the sign-in service did not answer. Try again in a moment` |
| `auth.invalid_country` | 400 | — | `país inválido: use um código como BR ou US` | `invalid country: use a code like BR or US` |
| `auth.invalid_credentials` | 401 | — | `email ou senha incorretos` | `wrong email or password` |
| `auth.invite_email_mismatch` | 400 | — | `este convite foi feito para outro email` | `this invitation was made for another email` |
| `auth.invite_invalid` | 404 | — | `este convite não é válido: ele expirou, foi revogado ou já foi usado` | `this invitation is not valid: it expired, was revoked or was already used` |
| `auth.long_password` | 400 | — | `a senha pode ter no máximo 72 caracteres` | `the password can have at most 72 characters` |
| `auth.name_required` | 400 | — | `informe o seu nome` | `enter your name` |
| `auth.org_name_required` | 400 | — | `informe o nome da organização` | `enter the organization name` |
| `auth.own_profile_only` | 403 | — | `você só pode alterar o seu próprio perfil` | `you can only change your own profile` |
| `auth.owner_only` | 403 | — | `só o dono da organização pode fazer isso` | `only the organization owner can do this` |
| `auth.permission_required` | 403 | — | `você não tem permissão para fazer isso` | `you do not have permission to do this` |
| `auth.unauthenticated` | 401 | — | `faça login para continuar` | `sign in to continue` |
| `auth.weak_password` | 400 | — | `a senha precisa ter pelo menos 8 caracteres` | `the password needs at least 8 characters` |
| `auth.wrong_password` | 400 | — | `a senha atual está incorreta` | `the current password is wrong` |

### collaborator

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `collaborator.not_in_project` | 404 | — | `esta pessoa não está neste projeto` | `this person is not on this project` |

### customer

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `customer.contact_too_long` | 400 | — | `o nome do contato pode ter até 120 caracteres` | `the contact name can have up to 120 characters` |
| `customer.has_projects` | 400 | — | `este cliente tem projetos; tire o cliente dos projetos antes de excluir` | `this customer has projects; remove the customer from the projects before deleting` |
| `customer.invalid_document` | 400 | — | `CNPJ inválido: confira os números e os dígitos verificadores` | `invalid CNPJ: check the numbers and the check digits` |
| `customer.invalid_email` | 400 | — | `informe um email de contato válido` | `enter a valid contact email` |
| `customer.invalid_organization` | 400 | — | `organização inválida` | `invalid organization` |
| `customer.invalid_phone` | 400 | — | `telefone inválido: use números, espaços, +, parênteses e hífen` | `invalid phone: use digits, spaces, +, parentheses and hyphen` |
| `customer.name_required` | 400 | — | `informe o nome do cliente` | `enter the customer name` |
| `customer.name_too_long` | 400 | — | `o nome do cliente pode ter até 120 caracteres` | `the customer name can have up to 120 characters` |
| `customer.not_found` | 404 | — | `cliente não encontrado` | `customer not found` |

### integration

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `integration.disabled` | 400 | — | `a integração está desativada` | `the integration is disabled` |
| `integration.field_not_text` | 400 | `field`, `provider` | `o campo "{{.field}}" do {{.provider}} precisa ser um texto` | `the "{{.field}}" field of {{.provider}} must be text` |
| `integration.field_required` | 400 | `field`, `provider` | `informe o campo "{{.field}}" do {{.provider}}` | `enter the "{{.field}}" field of {{.provider}}` |
| `integration.forbidden` | 403 | `provider` | `o token do {{.provider}} não tem permissão para isso` | `the {{.provider}} token has no permission for that` |
| `integration.github_invalid_repo` | 400 | — | `repositório do GitHub inválido: use dono/repositorio` | `invalid GitHub repository: use owner/repository` |
| `integration.github_oauth_denied` | 400 | — | `a autorização no GitHub foi cancelada` | `the authorization on GitHub was cancelled` |
| `integration.github_oauth_exchange` | 400 | — | `o GitHub não aceitou a autorização: tente conectar de novo` | `GitHub did not accept the authorization: try connecting again` |
| `integration.github_oauth_not_configured` | 400 | — | `a conexão com o GitHub não está configurada neste servidor: quem administra precisa definir GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET e PUBLIC_URL` | `the GitHub connection is not set up on this server: whoever runs it must define GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET and PUBLIC_URL` |
| `integration.github_oauth_state` | 400 | — | `a conexão com o GitHub venceu ou não pôde ser confirmada: tente de novo` | `the GitHub connection expired or could not be confirmed: try again` |
| `integration.github_repo_not_found` | 400 | — | `repositório do GitHub não encontrado ou o token não tem acesso a ele` | `GitHub repository not found, or the token has no access to it` |
| `integration.gitlab_forbidden` | 400 | — | `o token do GitLab não tem permissão para ler o projeto` | `the GitLab token has no permission to read the project` |
| `integration.gitlab_invalid_project` | 400 | — | `projeto do GitLab inválido: use grupo/projeto` | `invalid GitLab project: use group/project` |
| `integration.gitlab_project_not_found` | 400 | — | `projeto do GitLab não encontrado ou o token não tem acesso a ele` | `GitLab project not found, or the token has no access to it` |
| `integration.invalid_issue_number` | 400 | — | `o número da issue precisa ter só dígitos` | `the issue number must contain only digits` |
| `integration.invalid_token` | 400 | `provider` | `token do {{.provider}} inválido` | `invalid {{.provider}} token` |
| `integration.issue_gone` | 404 | `item` | `a issue {{.item}} não existe mais ou mudou de repositório` | `issue {{.item}} no longer exists or moved to another repository` |
| `integration.issues_disabled` | 400 | `provider` | `o repositório do {{.provider}} está com as issues desligadas, então não há onde criar uma` | `the {{.provider}} repository has issues turned off, so there is nowhere to create one` |
| `integration.item_not_found` | 400 | `item` | `item {{.item}} não encontrado` | `item {{.item}} not found` |
| `integration.list_too_long` | 400 | `provider` | `o {{.provider}} tem itens demais para listar de uma vez` | `{{.provider}} has too many items to list at once` |
| `integration.name_required` | 400 | — | `informe o nome da integração` | `enter the integration name` |
| `integration.no_credential` | 400 | — | `a integração está sem credencial: edite-a e conecte de novo` | `the integration has no credential: edit it and connect again` |
| `integration.no_repositories` | 400 | — | `esta integração não lista repositórios` | `this integration does not list repositories` |
| `integration.not_connectable` | 400 | — | `esta plataforma não se conecta por autorização` | `this platform does not connect through an authorization` |
| `integration.not_found` | 404 | — | `integração não encontrada` | `integration not found` |
| `integration.provider_status` | 400 | `provider`, `status` | `o {{.provider}} respondeu com status {{.status}}` | `{{.provider}} responded with status {{.status}}` |
| `integration.provider_unreachable` | 400 | `provider` | `não foi possível falar com o {{.provider}}` | `could not reach {{.provider}}` |
| `integration.rate_limited` | 429 | `provider` | `o {{.provider}} limitou as requisições: tente de novo mais tarde` | `{{.provider}} limited the requests: try again later` |
| `integration.sync_needs_enabled` | 400 | — | `ative a integração antes de ligar a sincronização` | `enable the integration before turning the sync on` |
| `integration.sync_needs_repo` | 400 | — | `escolha o repositório ou o quadro antes de ligar a sincronização` | `pick the repository or the board before turning the sync on` |
| `integration.sync_off` | 400 | — | `a sincronização está desligada nesta integração` | `the sync is off on this integration` |
| `integration.sync_repo_locked` | 400 | — | `com a sincronização ligada o repositório ou o quadro não muda: desligue a sincronização, troque e ligue de novo` | `with the sync on the repository or board cannot change: turn the sync off, change it and turn it on again` |
| `integration.sync_running` | 409 | — | `já há uma sincronização em andamento nesta integração` | `a sync is already running on this integration` |
| `integration.sync_unsupported` | 400 | `provider` | `o {{.provider}} não tem sincronização` | `{{.provider}} has no sync` |
| `integration.token_required` | 400 | `provider` | `informe o token do {{.provider}}` | `enter the {{.provider}} token` |
| `integration.trello_board_not_found` | 400 | — | `quadro do Trello não encontrado` | `Trello board not found` |
| `integration.trello_card_other_board` | 400 | `card` | `o cartão {{.card}} é de outro quadro` | `card {{.card}} belongs to another board` |
| `integration.trello_invalid_board` | 400 | — | `quadro do Trello inválido: use o endereço do quadro, o link curto ou o id` | `invalid Trello board: use the board address, the short link or the id` |
| `integration.trello_invalid_card` | 400 | — | `cartão do Trello inválido: use o link curto, o id ou o endereço do cartão` | `invalid Trello card: use the short link, the id or the card address` |
| `integration.trello_invalid_key` | 400 | — | `chave da API do Trello inválida` | `invalid Trello API key` |
| `integration.trello_no_access_board` | 400 | — | `chave ou token do Trello inválido, ou sem acesso ao quadro` | `invalid Trello key or token, or no access to the board` |
| `integration.trello_no_access_card` | 400 | — | `chave ou token do Trello inválido, ou sem acesso ao cartão` | `invalid Trello key or token, or no access to the card` |
| `integration.trello_no_list` | 400 | — | `o quadro do Trello não tem lista aberta: crie uma para os cartões poderem ser adicionados` | `the Trello board has no open list: create one so the cards can be added to it` |
| `integration.trello_oauth_denied` | 400 | — | `o acesso não foi autorizado no Trello` | `the access was not authorized on Trello` |
| `integration.trello_oauth_not_configured` | 400 | — | `a conexão com o Trello não está configurada neste servidor` | `the Trello connection is not set up on this server` |
| `integration.trello_oauth_state` | 400 | — | `a volta do Trello não corresponde à conexão que foi iniciada: tente de novo` | `the return from Trello does not match the connection that was started: try again` |
| `integration.type_coming_soon` | 400 | `provider` | `a integração com o {{.provider}} ainda não está disponível` | `the {{.provider}} integration is not available yet` |
| `integration.type_required` | 400 | — | `informe o tipo da integração` | `enter the integration type` |
| `integration.unexpected_response` | 400 | `provider` | `resposta inesperada do {{.provider}}` | `unexpected response from {{.provider}}` |
| `integration.unreadable_credential` | 400 | — | `não foi possível ler a credencial guardada: edite a integração e conecte de novo` | `could not read the stored credential: edit the integration and connect again` |
| `integration.unsupported_type` | 400 | `type` | `tipo de integração não suportado: {{.type}}` | `integration type not supported: {{.type}}` |

### internal

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `internal.server_error` | 500 | — | `erro interno, tente de novo` | `internal error, please try again` |

### issue_sync

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `issue_sync.label_refused` | 400 | — | `o token não pode criar etiquetas do outro lado: a etiqueta nova fica só aqui` | `the token cannot create labels on the other side: the new label stays here only` |
| `issue_sync.no_login` | 400 | — | `não achei o usuário do GitHub de quem é responsável aqui: ele precisa ter o e-mail público no perfil do GitHub` | `could not find the GitHub user of the person assigned here: they need a public email on their GitHub profile` |
| `issue_sync.publish_read_only` | 400 | — | `o token desta integração não escreve no repositório ou quadro: o item novo sairia sem parte dos dados, então a tarefa não foi postada` | `the token of this integration cannot write to the repository or board: the new item would come out without some of its data, so the task was not posted` |
| `issue_sync.push_discarded` | 400 | — | `a plataforma ignorou uma mudança deste item (o token pode não ter permissão de escrita): ela fica só aqui` | `the platform ignored a change to this item (the token may lack write permission): it stays here only` |
| `issue_sync.push_rejected` | 400 | — | `a plataforma recusou uma mudança deste item: ela fica só aqui` | `the platform refused a change to this item: it stays here only` |
| `issue_sync.read_only` | 400 | — | `o token não escreve neste repositório ou quadro: os itens só vêm para cá, e as mudanças daqui não saem` | `the token cannot write to this repository or board: items only come here, and changes made here do not go out` |
| `issue_sync.remove_only_closed` | 400 | — | `a plataforma não deixou apagar o item (a conta conectada precisa ser admin do repositório), então ele só foi fechado` | `the platform did not let the item be deleted (the connected account must be an admin of the repository), so it was only closed` |
| `issue_sync.remove_read_only` | 400 | — | `o token desta integração não escreve no repositório ou quadro, então o item continua lá` | `the token of this integration cannot write to the repository or board, so the item is still there` |

### label

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `label.name_required` | 400 | — | `informe o nome da etiqueta` | `enter the label name` |
| `label.name_taken` | 400 | — | `o projeto já tem uma etiqueta com este nome` | `the project already has a label with this name` |
| `label.name_too_long` | 400 | `max` | `o nome da etiqueta aceita até {{.max}} caracteres` | `the label name accepts up to {{.max}} characters` |
| `label.not_found` | 404 | — | `etiqueta não encontrada` | `label not found` |

### organization

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `organization.field_too_long` | 400 | `field`, `max` | `{{.field}} pode ter até {{.max}} caracteres` | `{{.field}} can have up to {{.max}} characters` |
| `organization.has_projects` | 400 | — | `exclua todos os projetos antes de excluir a organização` | `delete all projects before deleting the organization` |
| `organization.invalid_cnpj` | 400 | — | `CNPJ inválido: confira os números e os dígitos verificadores` | `invalid CNPJ: check the numbers and the check digits` |
| `organization.invalid_country` | 400 | — | `país inválido: use um código como BR ou US` | `invalid country: use a code like BR or US` |
| `organization.invalid_currency` | 400 | — | `moeda inválida: use BRL, USD ou EUR` | `invalid currency: use BRL, USD or EUR` |
| `organization.invalid_ein` | 400 | — | `EIN inválido: use 9 dígitos, por exemplo 12-3456789` | `invalid EIN: use 9 digits, for example 12-3456789` |
| `organization.invalid_email` | 400 | — | `informe um email de contato válido` | `enter a valid contact email` |
| `organization.invalid_founded_year` | 400 | — | `o ano de fundação deve ficar entre 1900 e o ano atual` | `the year founded must be between 1900 and the current year` |
| `organization.invalid_phone` | 400 | — | `telefone inválido: use números, espaços, +, parênteses e hífen` | `invalid phone: use digits, spaces, +, parentheses and hyphen` |
| `organization.invalid_postal_code` | 400 | — | `código postal inválido para o país da organização (CEP no Brasil, ZIP nos EUA)` | `invalid postal code for the organization's country (CEP in Brazil, ZIP in the US)` |
| `organization.invalid_size` | 400 | — | `porte inválido: use 1-10, 11-50, 51-200, 201-500 ou 500+` | `invalid size: use 1-10, 11-50, 51-200, 201-500 or 500+` |
| `organization.invalid_state` | 400 | — | `estado inválido para o país da organização: use a sigla, por exemplo SP ou TX` | `invalid state for the organization's country: use the abbreviation, for example SP or TX` |
| `organization.invalid_timezone` | 400 | — | `fuso horário inválido: use um nome como America/Sao_Paulo` | `invalid time zone: use a name like America/Sao_Paulo` |
| `organization.invalid_url` | 400 | — | `endereço inválido: use um link http ou https, por exemplo https://exemplo.com.br` | `invalid address: use an http or https link, for example https://example.com` |
| `organization.invalid_work_mode` | 400 | — | `regime de trabalho inválido: use remote, hybrid ou onsite` | `invalid work mode: use remote, hybrid or onsite` |
| `organization.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `organization.not_found` | 404 | — | `organização não encontrada` | `organization not found` |

### overview

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `overview.invalid_task_state` | 400 | — | `estado de tarefa inválido (state): use open ou closed` | `invalid task state (state): use open or closed` |

### person

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `person.admin_has_all_permissions` | 400 | — | `os admins já têm todas as permissões` | `admins already have every permission` |
| `person.email_in_use` | 409 | — | `este email já está em uso` | `this email is already in use` |
| `person.email_required` | 400 | — | `informe o email` | `enter the email` |
| `person.invalid_email` | 400 | — | `informe um email válido` | `enter a valid email` |
| `person.invalid_organization` | 400 | — | `organização inválida` | `invalid organization` |
| `person.invalid_permission` | 400 | — | `permissão da organização desconhecida` | `unknown organization permission` |
| `person.invalid_role` | 400 | — | `papel inválido: use admin ou member` | `invalid role: use admin or member` |
| `person.invalid_week_hours` | 400 | — | `a jornada semanal deve ficar entre 1 e 168 horas` | `the weekly hours must be between 1 and 168` |
| `person.last_admin` | 400 | — | `a organização precisa de pelo menos um admin` | `the organization needs at least one admin` |
| `person.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `person.not_found` | 404 | — | `pessoa não encontrada` | `person not found` |
| `person.owner_is_admin` | 400 | — | `o dono da organização é sempre admin` | `the organization owner is always an admin` |

### project

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `project.customer_meeting_time_without_day` | 400 | — | `o horário da reunião com o cliente precisa do dia da reunião` | `the customer meeting time needs the meeting day` |
| `project.customer_not_found` | 400 | — | `cliente não encontrado nesta organização` | `customer not found in this organization` |
| `project.invalid_bill_rate` | 400 | — | `o valor cobrado por hora deve ficar entre 0 e 1.000.000,00` | `the rate billed per hour must be between 0 and 1,000,000.00` |
| `project.invalid_customer_meeting_time` | 400 | — | `o horário da reunião com o cliente deve estar no formato HH:MM, por exemplo 10:30` | `the customer meeting time must use the HH:MM format, for example 10:30` |
| `project.invalid_daily_time` | 400 | — | `o horário da daily deve estar no formato HH:MM, por exemplo 09:30` | `the daily time must use the HH:MM format, for example 09:30` |
| `project.invalid_organization` | 400 | — | `organização inválida` | `invalid organization` |
| `project.invalid_page` | 400 | — | `página inválida (page): use um número a partir de 1` | `invalid page (page): use a number from 1` |
| `project.invalid_per_page` | 400 | — | `tamanho de página inválido (per_page): use um número a partir de 1` | `invalid page size (per_page): use a number from 1` |
| `project.invalid_sprint` | 400 | — | `a sprint precisa ter entre 1 e 90 dias` | `the sprint must be between 1 and 90 days long` |
| `project.invalid_weekday` | 400 | — | `dia da semana inválido: use monday, tuesday, wednesday, thursday, friday, saturday ou sunday` | `invalid weekday: use monday, tuesday, wednesday, thursday, friday, saturday or sunday` |
| `project.invalid_weekly_time` | 400 | — | `o horário da weekly deve estar no formato HH:MM, por exemplo 13:00` | `the weekly time must use the HH:MM format, for example 13:00` |
| `project.name_required` | 400 | — | `informe o nome do projeto` | `enter the project name` |
| `project.not_found` | 404 | — | `projeto não encontrado` | `project not found` |
| `project.weekly_time_without_day` | 400 | — | `o horário da weekly precisa do dia da weekly` | `the weekly time needs the weekly day` |

### projectinvite

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `projectinvite.team_not_in_project` | 400 | — | `o time não é deste projeto` | `the team is not in this project` |

### request

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `request.invalid_body` | 400 | — | `corpo da requisição inválido` | `invalid request body` |
| `request.json_required` | 415 | — | `envie o corpo como JSON (Content-Type: application/json)` | `send the body as JSON (Content-Type: application/json)` |
| `request.not_found` | 404 | — | `não encontrado` | `not found` |
| `request.too_many_attempts` | 429 | — | `muitas tentativas seguidas; espere um minuto e tente de novo` | `too many attempts in a row; wait a minute and try again` |

### task

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `task.already_assigned` | 409 | — | `esta tarefa já tem responsável` | `this task already has an assignee` |
| `task.already_linked` | 400 | — | `a tarefa já está ligada a um item desta integração` | `the task is already linked to an item of this integration` |
| `task.assignee_not_in_team` | 400 | — | `o responsável precisa estar neste projeto` | `the assignee must be on this project` |
| `task.description_too_long` | 400 | `max` | `a descrição aceita até {{.max}} caracteres` | `the description accepts up to {{.max}} characters` |
| `task.integration_not_found` | 400 | — | `integração não encontrada` | `integration not found` |
| `task.integration_other_project` | 400 | — | `a integração é de outro projeto` | `the integration belongs to another project` |
| `task.integrations_unavailable` | 400 | — | `integrações indisponíveis` | `integrations unavailable` |
| `task.invalid_assignee` | 400 | — | `responsável inválido` | `invalid assignee` |
| `task.invalid_assignee_filter` | 400 | — | `responsável inválido (assignee_id)` | `invalid assignee (assignee_id)` |
| `task.invalid_deadline_filter` | 400 | — | `prazo inválido (deadline_to): use data e hora, como 2026-10-12T23:59:59Z` | `invalid deadline (deadline_to): use date and time, like 2026-10-12T23:59:59Z` |
| `task.invalid_label_filter` | 400 | — | `etiqueta inválida (label_id): use ids separados por vírgula` | `invalid label (label_id): use ids separated by commas` |
| `task.invalid_page` | 400 | — | `página inválida (page): use um número a partir de 1` | `invalid page (page): use a number from 1` |
| `task.invalid_per_page` | 400 | — | `tamanho de página inválido (per_page): use um número a partir de 1` | `invalid page size (per_page): use a number from 1` |
| `task.invalid_priority` | 400 | — | `prioridade inválida: use urgent, high, medium, low ou none` | `invalid priority: use urgent, high, medium, low or none` |
| `task.invalid_priority_filter` | 400 | — | `prioridade inválida (priority): use urgent, high, medium, low ou none, separadas por vírgula` | `invalid priority (priority): use urgent, high, medium, low or none, separated by commas` |
| `task.invalid_status` | 400 | — | `status inválido: use backlog, in_progress, awaiting_closure ou closed` | `invalid status: use backlog, in_progress, awaiting_closure or closed` |
| `task.invalid_status_filter` | 400 | — | `status inválido (status): use backlog, in_progress, awaiting_closure ou closed, separados por vírgula` | `invalid status (status): use backlog, in_progress, awaiting_closure or closed, separated by commas` |
| `task.item_taken` | 400 | — | `este item já está ligado a outra tarefa` | `this item is already linked to another task` |
| `task.label_other_project` | 400 | — | `uma das etiquetas não existe neste projeto` | `one of the labels does not exist in this project` |
| `task.link_fields_required` | 400 | — | `informe a integração, o item e o link` | `enter the integration, the item and the link` |
| `task.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `task.no_external_item` | 400 | — | `a tarefa não tem item externo vinculado` | `the task has no linked external item` |
| `task.not_found` | 404 | — | `tarefa não encontrada` | `task not found` |
| `task.query_too_long` | 400 | `max` | `a busca aceita até {{.max}} caracteres (q)` | `the search accepts up to {{.max}} characters (q)` |

### team

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `team.already_member` | 400 | — | `a pessoa já está neste time` | `the person is already on this team` |
| `team.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `team.no_rate` | 400 | — | `esta pessoa ainda não está no projeto: adicione-a com o valor por hora antes de colocá-la num time` | `this person is not on the project yet: add them with an hourly rate before putting them on a team` |
| `team.not_found` | 404 | — | `time não encontrado` | `team not found` |
| `team.person_not_in_org` | 400 | — | `pessoa não encontrada nesta organização` | `person not found in this organization` |
| `team.person_required` | 400 | — | `informe a pessoa (person_id)` | `enter the person (person_id)` |

### work_session

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `work_session.already_open` | 400 | — | `já existe um ponto aberto para esta pessoa; pare a sessão atual antes de iniciar outra` | `this person already has an open clock-in; stop the current session before starting another` |
| `work_session.filter_required` | 400 | — | `filtre por task_id ou person_id` | `filter by task_id or person_id` |
| `work_session.invalid_interval` | 400 | — | `o intervalo precisa começar e terminar dentro da sessão, com o fim depois do começo` | `the interval must start and end inside the session, with the end after the start` |
| `work_session.invalid_person_filter` | 400 | — | `pessoa inválida (person_id)` | `invalid person (person_id)` |
| `work_session.invalid_task_filter` | 400 | — | `tarefa inválida (task_id)` | `invalid task (task_id)` |
| `work_session.last_task` | 400 | — | `a sessão precisa ficar com ao menos uma tarefa; enquanto aberta, ao menos uma em andamento` | `the session must keep at least one task; while it is open, at least one still in progress` |
| `work_session.no_rate` | 400 | — | `esta pessoa ainda não tem valor por hora neste projeto; um admin precisa definir na aba Colaboradores antes do ponto` | `this person has no hourly rate on this project yet; an admin must set one on the Collaborators tab before clocking in` |
| `work_session.not_found` | 404 | — | `sessão não encontrada` | `session not found` |
| `work_session.not_open` | 400 | — | `não há ponto aberto para esta pessoa` | `this person has no open clock-in` |
| `work_session.not_yours` | 403 | — | `só quem bateu o ponto, ou um admin, pode mexer nas tarefas desta sessão` | `only the person who clocked in, or an admin, can change the tasks of this session` |
| `work_session.other_person_admin_only` | 403 | — | `só admins podem registrar o ponto de outra pessoa` | `only admins can clock in for another person` |
| `work_session.person_not_found` | 400 | — | `pessoa não encontrada` | `person not found` |
| `work_session.person_not_in_org` | 400 | — | `pessoa não encontrada nesta organização` | `person not found in this organization` |
| `work_session.person_required` | 400 | — | `informe a pessoa (person_id)` | `enter the person (person_id)` |
| `work_session.task_link_not_found` | 404 | — | `esta tarefa não está na sessão` | `this task is not in the session` |
| `work_session.task_not_found` | 400 | — | `tarefa não encontrada` | `task not found` |
| `work_session.task_other_project` | 400 | — | `a tarefa não é deste projeto` | `the task does not belong to this project` |
| `work_session.task_overlap` | 400 | — | `a tarefa já está na sessão neste período` | `the task is already in the session during this period` |
| `work_session.task_required` | 400 | — | `informe a tarefa (task_id)` | `enter the task (task_id)` |

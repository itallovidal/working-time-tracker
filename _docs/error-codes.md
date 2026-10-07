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
| `allocation.invalid_rate` | 400 | — | `o valor por hora deve ficar entre 0 e 1.000.000,00` | `the hourly rate must be between 0 and 1,000,000.00` |
| `allocation.not_defined` | 404 | — | `esta pessoa não tem valor definido neste projeto` | `this person has no rate set on this project` |
| `allocation.own_rates_only` | 403 | — | `você só pode ver os seus próprios valores` | `you can only see your own rates` |
| `allocation.person_not_in_org` | 400 | — | `pessoa não encontrada nesta organização` | `person not found in this organization` |
| `allocation.rate_required` | 400 | — | `informe o valor por hora` | `enter the hourly rate` |

### auth

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `auth.account_exists` | 409 | — | `já existe uma conta com este email` | `an account with this email already exists` |
| `auth.admin_only` | 403 | — | `só admins podem fazer isso` | `only admins can do this` |
| `auth.invalid_credentials` | 401 | — | `email ou senha incorretos` | `wrong email or password` |
| `auth.invite_email_mismatch` | 400 | — | `este convite foi feito para outro email` | `this invitation was made for another email` |
| `auth.invite_invalid` | 404 | — | `este convite não é válido: ele expirou, foi revogado ou já foi usado` | `this invitation is not valid: it expired, was revoked or was already used` |
| `auth.long_password` | 400 | — | `a senha pode ter no máximo 72 caracteres` | `the password can have at most 72 characters` |
| `auth.name_required` | 400 | — | `informe o seu nome` | `enter your name` |
| `auth.org_name_required` | 400 | — | `informe o nome da organização` | `enter the organization name` |
| `auth.own_profile_only` | 403 | — | `você só pode alterar o seu próprio perfil` | `you can only change your own profile` |
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
| `integration.github_invalid_repo` | 400 | — | `repositório do GitHub inválido: use dono/repositorio` | `invalid GitHub repository: use owner/repository` |
| `integration.github_repo_not_found` | 400 | — | `repositório do GitHub não encontrado ou o token não tem acesso a ele` | `GitHub repository not found, or the token has no access to it` |
| `integration.gitlab_forbidden` | 400 | — | `o token do GitLab não tem permissão para ler o projeto` | `the GitLab token has no permission to read the project` |
| `integration.gitlab_invalid_project` | 400 | — | `projeto do GitLab inválido: use grupo/projeto` | `invalid GitLab project: use group/project` |
| `integration.gitlab_project_not_found` | 400 | — | `projeto do GitLab não encontrado ou o token não tem acesso a ele` | `GitLab project not found, or the token has no access to it` |
| `integration.invalid_issue_number` | 400 | — | `o número da issue precisa ter só dígitos` | `the issue number must contain only digits` |
| `integration.invalid_token` | 400 | `provider` | `token do {{.provider}} inválido` | `invalid {{.provider}} token` |
| `integration.item_not_found` | 400 | `item` | `item {{.item}} não encontrado` | `item {{.item}} not found` |
| `integration.name_required` | 400 | — | `informe o nome da integração` | `enter the integration name` |
| `integration.no_credential` | 400 | — | `a integração está sem credencial: edite-a e informe o token` | `the integration has no credential: edit it and enter the token` |
| `integration.not_found` | 404 | — | `integração não encontrada` | `integration not found` |
| `integration.provider_status` | 400 | `provider`, `status` | `o {{.provider}} respondeu com status {{.status}}` | `{{.provider}} responded with status {{.status}}` |
| `integration.provider_unreachable` | 400 | `provider` | `não foi possível falar com o {{.provider}}` | `could not reach {{.provider}}` |
| `integration.token_required` | 400 | `provider` | `informe o token do {{.provider}}` | `enter the {{.provider}} token` |
| `integration.trello_board_not_found` | 400 | — | `quadro do Trello não encontrado` | `Trello board not found` |
| `integration.trello_card_other_board` | 400 | `card` | `o cartão {{.card}} é de outro quadro` | `card {{.card}} belongs to another board` |
| `integration.trello_invalid_board` | 400 | — | `quadro do Trello inválido: use o endereço do quadro, o link curto ou o id` | `invalid Trello board: use the board address, the short link or the id` |
| `integration.trello_invalid_card` | 400 | — | `cartão do Trello inválido: use o link curto, o id ou o endereço do cartão` | `invalid Trello card: use the short link, the id or the card address` |
| `integration.trello_invalid_key` | 400 | — | `chave da API do Trello inválida` | `invalid Trello API key` |
| `integration.trello_no_access_board` | 400 | — | `chave ou token do Trello inválido, ou sem acesso ao quadro` | `invalid Trello key or token, or no access to the board` |
| `integration.trello_no_access_card` | 400 | — | `chave ou token do Trello inválido, ou sem acesso ao cartão` | `invalid Trello key or token, or no access to the card` |
| `integration.type_required` | 400 | — | `informe o tipo da integração` | `enter the integration type` |
| `integration.unexpected_response` | 400 | `provider` | `resposta inesperada do {{.provider}}` | `unexpected response from {{.provider}}` |
| `integration.unreadable_credential` | 400 | — | `não foi possível ler a credencial guardada: edite a integração e informe o token de novo` | `could not read the stored credential: edit the integration and enter the token again` |
| `integration.unsupported_type` | 400 | `type` | `tipo de integração não suportado: {{.type}}` | `integration type not supported: {{.type}}` |

### internal

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `internal.server_error` | 500 | — | `erro interno, tente de novo` | `internal error, please try again` |

### organization

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `organization.field_too_long` | 400 | `field`, `max` | `{{.field}} pode ter até {{.max}} caracteres` | `{{.field}} can have up to {{.max}} characters` |
| `organization.has_projects` | 400 | — | `exclua todos os projetos antes de excluir a organização` | `delete all projects before deleting the organization` |
| `organization.invalid_cnpj` | 400 | — | `CNPJ inválido: confira os números e os dígitos verificadores` | `invalid CNPJ: check the numbers and the check digits` |
| `organization.invalid_currency` | 400 | — | `moeda inválida: use BRL, USD ou EUR` | `invalid currency: use BRL, USD or EUR` |
| `organization.invalid_email` | 400 | — | `informe um email de contato válido` | `enter a valid contact email` |
| `organization.invalid_founded_year` | 400 | — | `o ano de fundação deve ficar entre 1900 e o ano atual` | `the year founded must be between 1900 and the current year` |
| `organization.invalid_phone` | 400 | — | `telefone inválido: use números, espaços, +, parênteses e hífen` | `invalid phone: use digits, spaces, +, parentheses and hyphen` |
| `organization.invalid_size` | 400 | — | `porte inválido: use 1-10, 11-50, 51-200, 201-500 ou 500+` | `invalid size: use 1-10, 11-50, 51-200, 201-500 or 500+` |
| `organization.invalid_timezone` | 400 | — | `fuso horário inválido: use um nome como America/Sao_Paulo` | `invalid time zone: use a name like America/Sao_Paulo` |
| `organization.invalid_url` | 400 | — | `endereço inválido: use um link http ou https, por exemplo https://exemplo.com.br` | `invalid address: use an http or https link, for example https://example.com` |
| `organization.invalid_work_mode` | 400 | — | `regime de trabalho inválido: use remote, hybrid ou onsite` | `invalid work mode: use remote, hybrid or onsite` |
| `organization.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `organization.not_found` | 404 | — | `organização não encontrada` | `organization not found` |

### person

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `person.email_in_use` | 409 | — | `este email já está em uso` | `this email is already in use` |
| `person.email_required` | 400 | — | `informe o email` | `enter the email` |
| `person.invalid_email` | 400 | — | `informe um email válido` | `enter a valid email` |
| `person.invalid_organization` | 400 | — | `organização inválida` | `invalid organization` |
| `person.invalid_role` | 400 | — | `papel inválido: use admin ou member` | `invalid role: use admin or member` |
| `person.invalid_week_hours` | 400 | — | `a jornada semanal deve ficar entre 1 e 168 horas` | `the weekly hours must be between 1 and 168` |
| `person.last_admin` | 400 | — | `a organização precisa de pelo menos um admin` | `the organization needs at least one admin` |
| `person.name_required` | 400 | — | `informe o nome` | `enter the name` |
| `person.not_found` | 404 | — | `pessoa não encontrada` | `person not found` |

### project

| Código | Status | Parâmetros | pt-BR | en |
|---|---|---|---|---|
| `project.customer_not_found` | 400 | — | `cliente não encontrado nesta organização` | `customer not found in this organization` |
| `project.invalid_bill_rate` | 400 | — | `o valor cobrado por hora deve ficar entre 0 e 1.000.000,00` | `the rate billed per hour must be between 0 and 1,000,000.00` |
| `project.invalid_daily_time` | 400 | — | `o horário da daily deve estar no formato HH:MM, por exemplo 09:30` | `the daily time must use the HH:MM format, for example 09:30` |
| `project.invalid_organization` | 400 | — | `organização inválida` | `invalid organization` |
| `project.invalid_sprint` | 400 | — | `a sprint precisa ter entre 1 e 90 dias` | `the sprint must be between 1 and 90 days long` |
| `project.invalid_weekday` | 400 | — | `dia da weekly inválido: use monday, tuesday, wednesday, thursday, friday, saturday ou sunday` | `invalid weekly day: use monday, tuesday, wednesday, thursday, friday, saturday or sunday` |
| `project.invalid_weekly_time` | 400 | — | `o horário da weekly deve estar no formato HH:MM, por exemplo 13:00` | `the weekly time must use the HH:MM format, for example 13:00` |
| `project.name_required` | 400 | — | `informe o nome do projeto` | `enter the project name` |
| `project.not_found` | 404 | — | `projeto não encontrado` | `project not found` |
| `project.weekly_time_without_day` | 400 | — | `o horário da weekly precisa do dia da weekly` | `the weekly time needs the weekly day` |

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
| `task.assignee_not_in_team` | 400 | — | `o responsável precisa estar em algum time deste projeto` | `the assignee must be on a team of this project` |
| `task.integration_not_found` | 400 | — | `integração não encontrada` | `integration not found` |
| `task.integration_other_project` | 400 | — | `a integração é de outro projeto` | `the integration belongs to another project` |
| `task.integrations_unavailable` | 400 | — | `integrações indisponíveis` | `integrations unavailable` |
| `task.invalid_assignee` | 400 | — | `responsável inválido` | `invalid assignee` |
| `task.invalid_assignee_filter` | 400 | — | `responsável inválido (assignee_id)` | `invalid assignee (assignee_id)` |
| `task.invalid_deadline_filter` | 400 | — | `prazo inválido (deadline_to): use data e hora, como 2026-10-12T23:59:59Z` | `invalid deadline (deadline_to): use date and time, like 2026-10-12T23:59:59Z` |
| `task.invalid_page` | 400 | — | `página inválida (page): use um número a partir de 1` | `invalid page (page): use a number from 1` |
| `task.invalid_per_page` | 400 | — | `tamanho de página inválido (per_page): use um número a partir de 1` | `invalid page size (per_page): use a number from 1` |
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
| `work_session.invalid_person_filter` | 400 | — | `pessoa inválida (person_id)` | `invalid person (person_id)` |
| `work_session.invalid_task_filter` | 400 | — | `tarefa inválida (task_id)` | `invalid task (task_id)` |
| `work_session.no_rate` | 400 | — | `esta pessoa ainda não tem valor por hora neste projeto; um admin precisa definir na aba Colaboradores antes do ponto` | `this person has no hourly rate on this project yet; an admin must set one on the Collaborators tab before clocking in` |
| `work_session.not_open` | 400 | — | `não há ponto aberto para esta pessoa` | `this person has no open clock-in` |
| `work_session.other_person_admin_only` | 403 | — | `só admins podem registrar o ponto de outra pessoa` | `only admins can clock in for another person` |
| `work_session.person_not_found` | 400 | — | `pessoa não encontrada` | `person not found` |
| `work_session.person_not_in_org` | 400 | — | `pessoa não encontrada nesta organização` | `person not found in this organization` |
| `work_session.person_required` | 400 | — | `informe a pessoa (person_id)` | `enter the person (person_id)` |
| `work_session.task_not_found` | 400 | — | `tarefa não encontrada` | `task not found` |
| `work_session.task_other_project` | 400 | — | `a tarefa não é deste projeto` | `the task does not belong to this project` |
| `work_session.task_required` | 400 | — | `informe a tarefa (task_id)` | `enter the task (task_id)` |

# Proposta: cargos, permissões e regras de dinheiro

**Status: proposta, nada implementado.** O que vale hoje está em [permissions.md](permissions.md). Este arquivo reúne o
que foi conversado em 10 out 2026 para virar sprints depois. Não há produção nem dados reais, então nenhuma mudança
abaixo precisa de migration de correção ou de caminho legado.

## Princípios

1. **Dois eixos.** Gerir gente e projeto é uma hierarquia; dinheiro (custo e cobrança) é outro eixo e não segue o cargo.
   Por enquanto o Dono concentra o dinheiro; o **Financeiro** entra depois, junto com o custo de infraestrutura.
2. **Time é só organização visual.** Serve para achar pessoas num projeto de 30. Não dá autoridade e nada filtra por
   ele; por isso não existe "líder de time".
3. **Ninguém trabalha de graça, e todo cliente paga.** Ver "Regras de dinheiro".
4. **Cada cargo só convida e promove quem está abaixo dele.** A regra `allocation.preset_above_yours` continua.

## Cargos

| Cargo | Escopo | Em uma frase |
|---|---|---|
| **Dono** (super admin) | Organização | Tudo, inclusive todo o dinheiro. Um só por organização |
| **Admin** | Organização | Cria projetos internos, convida pessoas e administra todos os projetos. Sem cliente, cobrança nem pagamentos |
| **Administrador de projeto** | Um projeto | Administra o projeto que lhe deram: convida, põe gente, define custo, times, integrações |
| **Colaborador** | Projetos em que está | Tarefas, ponto e o que é dele |
| *Financeiro* (depois) | Organização | Custo, cobrança, pagamentos e contas a receber, sem gerir gente nem projeto |

## Tabela

✔ = pode · — = não pode · (seu) = só no projeto que lhe deram · ✱ = ainda não confirmado

| Capacidade | Dono | Admin | Administrador de projeto | Colaborador |
|---|---|---|---|---|
| Ver as informações gerais da organização | ✔ | ✔ | ✔ | ✔ |
| Editar os dados da organização | ✔ | — | — | — |
| Editar papéis e permissões (inclui convidar um Admin) | ✔ | — | — | — |
| Excluir a organização | ✔ | — | — | — |
| Cadastrar e alterar clientes | ✔ | — | — | — |
| Criar projeto interno | ✔ | ✔ | — | — |
| Criar projeto com cliente | ✔ | — | — | — |
| Definir cliente e valor cobrado | ✔ | — | — | — |
| Ver cobrança, receita e margem | ✔ | — | — | — |
| Ver as pessoas: nome, e-mail e papel | ✔ todas | ✔ todas | ✔ todas (p/ achar quem convidar) | quem está nos projetos dele |
| Ver a lista de permissões de alguém ✱ | ✔ | ✔ | — | só a sua |
| Convidar pessoas | ✔ | ✔ | ✔ (p/ o seu projeto) | — |
| Pôr e tirar gente do projeto | ✔ | ✔ | ✔ (seu) | — |
| Ver o custo de quem está no projeto | ✔ | ✔ | ✔ (seu) | só o seu |
| Definir o custo (obrigatório para pôr alguém) | ✔ | ✔ | ✔ (seu) | — |
| Editar projeto, integrações, times e etiquetas | ✔ | ✔ | ✔ (seu) | — |
| Excluir projeto ✱ | ✔ | ✔ | — | — |
| Ver as horas da equipe | ✔ | ✔ | ✔ (seu) | só as suas |
| Ver a jornada semanal | ✔ | ✔ | ✔ (seu) | só a sua |
| Definir a jornada semanal | ✔ | ✔ | — | — |
| Ver os pagamentos de uma pessoa | ✔ todos | — | — | só os seus |
| Página de pagamentos da equipe | ✔ | — | — | — |
| Definir a regra de pagamento | ✔ | — | — | — |
| Tarefas, ponto, ver o próprio valor | ✔ | ✔ | ✔ | ✔ |

A jornada é da pessoa (vale na organização inteira); o que varia por projeto é o custo. Por isso o Administrador de
projeto vê a jornada e não a define: definir vai além do projeto dele.

## Regras de dinheiro

- **Custo** (o que a organização paga por hora a quem trabalha): mínimo **10,00**. Zero não vale.
  Exceção: o **Dono**, com custo 0. Ele leva o que o cliente paga, e as horas dele valem o valor cobrado.
- **Valor cobrado** (o que o cliente paga por hora): mínimo **10,00** e **obrigatório** quando o projeto tem cliente.
  Com os dois no piso, a margem é 0. Não se exige "cobrado ≥ custo": margem negativa é informação, não erro.
- **Um projeto ou tem cliente (e valor cobrado) ou é interno.** Interno não tem cliente nem valor cobrado: é
  investimento, só existe o custo. A API recusa as combinações no meio: valor cobrado sem cliente e cliente sem valor.
- **Ninguém entra num projeto sem custo.** Estar no projeto é ter custo definido (uma regra só).
- O valor fica **congelado em cada sessão** no clock-in. Mudar um valor não reescreve as sessões antigas.
- O **custo de infraestrutura** (servidor etc.) é uma etapa futura, com o Financeiro.

## O que muda no que existe hoje

**Dinheiro**
- `validate`: piso de 10,00 (`MinRateCents = 1000`) e um validador de valor por hora, usado na alocação, no convite de
  projeto e no valor cobrado. A tela lê o piso de `window.BOOT.limits`, como na Decisão 16.
- `allocation.Service.Set`: a checagem do dono vem antes da do valor (o dono entra com 0).
- `POST /orgs/:id/projects` passa a aceitar `bill_rate_cents`: obrigatório com `customer_id`, recusado sem ele. A criação
  vira uma chamada só (hoje são duas: criar e `PUT /billing`). `PUT /billing` segue a mesma regra.
- A tela de Configurações do projeto só mostra o campo do valor cobrado com cliente (hoje aparece sempre), e tirar o
  cliente limpa o valor.
- Quem cria projeto sem ser admin entrava com custo 0 (`project/handler.go`): esse caminho some, porque criar projeto
  passa a ser do Dono e do Admin.
- A união "tem custo **ou** está num time" (`team/membership_store.go`, `inProject`) deixa de ser necessária: era só
  para bancos antigos.

**Cargos e permissões**
- Hoje `role: admin` é um atalho para "pode tudo" (cerca de 23 pontos usam `IsAdmin()`, e `ProjectSet` devolve
  `Set{All: true}`). O Admin limitado exige separar **"vê todos os projetos"** de **"tem todas as permissões"**. É a
  maior mudança desta proposta.
- Hoje o dinheiro só existe no escopo do projeto (`rates.*`, `billing.*`). O Financeiro precisará de permissões de
  dinheiro no escopo da organização.
- `project.edit` junta editar e **excluir** o projeto, e excluir apaga horas e valores em cascata: separar.
- `people.manage` junta convidar, a jornada e a regra de pagamento: separar a regra de pagamento (dinheiro).
- Esconder de quem não é a pessoa, o Dono, o Admin ou (no caso da jornada) o Administrador de projeto: a **jornada** e a
  **lista de permissões da organização**, que hoje vão no cadastro que o colega de projeto recebe. O papel e o e-mail
  continuam visíveis.
- Grupos do projeto nas telas: **Colaborador** e **Administrador de projeto** (hoje `manager`, com o texto "Gerente de
  projeto"). O grupo `admin` do projeto (que só acrescenta cobrança) e o `finance` ficam guardados no código, escondidos,
  até o Financeiro existir.
- Quem cria projeto com cliente precisa de quem defina o valor cobrado: só o Dono (o Admin cria interno e o Dono
  converte).

## Em aberto

1. **As três permissões da organização** (`projects.create`, `customers.manage`, `people.manage`), hoje dadas pelo Dono a
   um membro: com os cargos acima elas ficam implícitas no cargo. Remover de vez, ou manter como avulsas?
2. **Excluir projeto** só para Dono e Admin (✱).
3. **Admin vê a lista de permissões** de alguém (✱).
4. O Administrador de projeto vê a jornada de quem? Assumido: das pessoas do projeto dele.
5. Quando o Financeiro entrar: custo de infraestrutura, contas a receber e o dinheiro da organização inteira.

## Ordem sugerida

1. **Regras de dinheiro** (piso, cliente ⇔ valor cobrado, criação em uma chamada). Independente dos cargos e pequena.
2. **Esconder campos e separar permissões**: jornada, lista de permissões, excluir projeto, regra de pagamento.
3. **Admin limitado e os grupos do projeto**, a mudança de base. Fica por último porque mexe no atalho de admin.

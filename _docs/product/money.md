# Dinheiro

Como o tempo vira dinheiro: os clientes e os termos de cobrança, os dois valores por hora, a margem, a regra de pagamento
de cada pessoa, as contas a receber do modo solo e quem vê cada número. Quem pode mudar cada coisa está em
[permissions.md](permissions.md).

## Clientes

O cliente é quem contrata a organização. Só o nome é obrigatório; o país (que começa no da organização), o documento
fiscal, o contato, o e-mail e o telefone são opcionais. O documento é conferido pela regra do país do cliente: CNPJ no
Brasil, EIN nos EUA e texto livre de até 32 caracteres nos outros países (ver
[../development/i18n-and-countries.md](../development/i18n-and-countries.md)).

Só o dono cadastra, altera e exclui clientes (`customers.manage`). Um cliente com projetos não pode ser excluído
(`customer.has_projects`), nem um com recebíveis (`customer.has_receivables`: o dono os desfaz antes, na página
A receber do modo solo).

### A cobrança mora no cliente

A cobrança se define no cliente e vale para todos os projetos dele: o projeto só soma um **acréscimo**
([Decisão 21](../architecture/decisions.md#decisão-21-cobrança-num-lugar-só--o-valor-no-cliente-o-acréscimo-no-projeto)).
O reajuste se faz no cliente, e as sessões já registradas guardam o valor delas.

Todo cliente tem um **modo de cobrança**, um ou outro, nunca os dois. Na organização com equipe todo cliente é **por
hora**, e a tela de Clientes pede só o valor por hora. No **modo solo** (`organizations.solo`) ela mostra também o
seletor do modo, o contrato mensal e os prazos. A API traz os campos para todos (`GET /api/orgs/:orgId/customers`), e
os valores são em centavos, na moeda da organização.

| | Por hora (`hourly`, padrão) | Mensal (`monthly`, só no modo solo) |
|---|---|---|
| O que é | As horas dos projetos vezes o valor por hora do cliente, mais o acréscimo de cada projeto | Um valor fixo por mês, o contrato, mais o acréscimo mensal de cada projeto |
| Só dele | `hourly_rate_cents`: o valor por hora, **obrigatório**, com o piso de 10,00 | `monthly_fee_cents` (obrigatório, maior que zero), `contract_start` (obrigatório) e `contract_end` (opcional, depois do início); `late_fee_bp` (multa, 200 = 2%), `late_interest_bp_month` (juros ao mês) e `grace_days` (tolerância depois do vencimento), que ficam num bloco recolhido na tela |
| Só dele, o ciclo | `billing_cycle`: `monthly` (padrão, fecha no `closing_day`) ou `biweekly` (a cada 14 dias, o bi-weekly dos EUA), com `cycle_anchor`, o último dia do último período que o cliente já pagou | O ciclo é sempre o mensal |
| Dos dois | `closing_day` (1 a 31, padrão 31: 31 é o último dia do mês, e o mês curto fecha no último dia dele; sem uso no ciclo de 14 dias) e `payment_days` (dias entre o fechamento e o vencimento, padrão 0) | |

- **Sem o valor por hora o cliente por hora é recusado** (`request.field_required`, field `hourly_rate_cents`), na
  criação e ao apagá-lo. Um cliente antigo, de antes da Decisão 21, sem projetos e sem valor, aparece na lista como
  "sem valor por hora", e um projeto não se liga a ele (`project.customer_without_rate`) até o dono informar o valor.
- **Campo do outro modo é recusado** (`customer.field_hourly_only` e `customer.field_monthly_only`, com o campo), na
  tela (que só mostra os do modo escolhido) e no servidor. Trocar o modo apaga os campos do modo que saiu.
- **O mensal só vale no modo solo** (`customer.monthly_needs_solo`). Desligar o modo solo esconde as telas, e o
  cliente mensal segue guardado, como o documento do outro país.
- **O modo trava quando o cliente já tem projetos** (`409 customer.mode_locked`): a tela mostra o seletor desabilitado,
  com o motivo. Trocar o modo com projetos fica para depois (issue #25). O valor por hora e o contrato seguem
  editáveis.
- **Teto dos termos.** O prazo e a tolerância vão até 365 dias, e a multa e os juros até 100% (10000 pontos-base), para
  um número absurdo não estourar uma data.

## Os dois valores por hora

| Valor | De quem | Onde fica | Regra |
|---|---|---|---|
| **Cobrado** | Do cliente: o que ele paga por hora naquele projeto | **Calculado**: `customers.hourly_rate_cents` mais `projects.bill_extra_cents` | Só com cliente por hora; o projeto interno e o do cliente mensal não têm |
| **Pago** (o custo) | De cada pessoa em cada projeto: o que ela recebe por hora ali | `allocations.pay_rate_cents` | Obrigatório para estar no projeto |

- **O acréscimo do projeto** (`bill_extra_cents`, zero por padrão) soma à cobrança do cliente só naquele projeto, para
  algo pequeno ou temporário: **por hora** com cliente por hora, **por mês** com cliente mensal (soma à mensalidade
  em A receber). Vai de zero ao teto de um valor em dinheiro (`project.invalid_bill_extra`) e não tem data de fim: o
  dono o zera quando acaba.
- **Um projeto tem cliente, ou é interno, sem cliente e sem acréscimo.** O cliente e o acréscimo andam juntos
  (`PUT /api/projects/:projectId/billing`, e `POST` do projeto): acréscimo sem cliente dá
  `project.bill_extra_needs_customer`. Trocar o cliente do projeto troca a cobrança, e o acréscimo passa a valer no
  modo do cliente novo. O projeto interno não tem receita.
- **O valor cobrado é calculado, não gravado no projeto** (`customer.BillRate`): reajustar o cliente muda o valor
  cobrado de todos os projetos dele. As respostas de leitura (o billing do projeto, a visão geral, as sessões) seguem
  com `bill_rate_cents`; o billing traz também `bill_extra_cents` e a cobrança do cliente
  (`customer_hourly_rate_cents`, `customer_monthly_fee_cents`).
- **O cliente mensal não tem valor por hora.** O contrato é um valor por mês, então o projeto dele não tem valor
  cobrado por hora. As sessões desse projeto guardam `bill_rate_cents` nulo e o ponto funciona igual: as horas e o
  custo contam, e a receita vem do recebível mensal, não das horas.
- **A mesma pessoa pode receber valores diferentes em projetos diferentes**: 20 num, 25 no outro.
- **Pisos e teto.** O valor por hora do cliente e o pago têm o piso de 10,00 por hora (`validate.MinRateCents`) e o
  teto de 1.000.000,00. Ninguém trabalha de graça, e todo cliente paga.
- **O dono não recebe por hora.** Entra nos projetos com valor pago zero, e o que ele tira do projeto é a margem. As
  sessões dele levam a marca `owner_hours`: o custo é zero, e o valor cobrado conta como receita.
- A **margem por hora** de uma pessoa num projeto é o valor cobrado menos o valor pago dela.

## Da sessão ao dinheiro

```mermaid
flowchart LR
  CI["Clock-in"] --> S["A sessão guarda o valor pago e o cobrado desse momento"]
  S --> T["Tempo da sessão: até o clock-out, ou até agora se está aberta"]
  T --> C["Custo = tempo × valor pago"]
  T --> R["Receita = tempo × valor cobrado"]
  C --> M["Margem = receita − custo"]
  R --> M
```

- **Mudar um valor só vale dali em diante.** Cada sessão guarda os valores do clock-in (o valor cobrado é o do
  cliente mais o acréscimo naquele momento), então reajustar o cliente ou mudar o acréscimo não muda as sessões antigas.
- **Sem valor no projeto, não há ponto** (`work_session.no_rate`).
- **Arredondamento por sessão.** O valor de cada sessão é arredondado ao centavo, e os totais somam valores já
  arredondados, para a soma da tela bater com as linhas. Por isso o total da organização é a soma dos projetos.
- **Várias tarefas numa sessão** não multiplicam o dinheiro: a pessoa e o projeto contam a sessão uma vez só (ver
  [overview.md](overview.md#ponto)).

## Regra de pagamento

Cada pessoa pode ter uma **regra de pagamento**, que diz quando ela é paga. Só o dono define
(`PATCH /api/persons/:personId/payment`, `payments.manage`).

| Regra | Como é | Data do pagamento |
|---|---|---|
| **Mensal** | Um dia fixo do mês, de 1 a 31 | Esse dia. Num mês mais curto, o último dia |
| **Quinzenal** | Uma data de início, e daí períodos de 15 dias | O último dia de cada período |

O dia do pagamento é o **último dia do período**: o período fecha à meia-noite do fim desse dia, no fuso da
organização. Uma sessão que atravessa a virada conta em cada período só o trecho dela.

**Os períodos são calculados; a baixa é registrada.** O período corrente, os próximos e o histórico saem da regra e
das sessões, na hora (`internal/domain/payment`). O que fica gravado é só a **baixa** (tabela `payouts`): o dono marca
que pagou um período, a qualquer momento depois que ele começou, e a baixa guarda as horas e o valor calculados naquele
momento, o valor pago pelas horas (vem com o calculado e pode mudar) e um **bônus** opcional (zero quando não houve,
nunca negativo). O que a pessoa registra depois de uma baixa é o **restante** do período: volta para os pagamentos a
fazer, com o que já foi pago, e leva outra baixa, que cobre só ele. Por isso:

- trocar a regra recalcula onde os períodos passados caem; uma baixa antiga deixa de casar com os períodos novos, mas
  continua em Pagos recentemente com os números dela;
- excluir um projeto tira as sessões dele do histórico;
- quem não tem regra ainda tem as horas e o total **desde o início**, só sem períodos;
- o dono não é pago: não tem valor, não entra nas tabelas de Pagamentos nem em "sem regra".

**Onde aparece.**

- No **perfil** da pessoa: as horas e o valor do período corrente, o próximo pagamento e quantos dias faltam, o andamento
  contra a jornada, o total desde o início, o histórico dos períodos fechados (cada um com a baixa, "Pago em …", ou
  "Pendente") e as próximas datas (`GET /api/persons/:personId/payments`).
- Na página **Pagamentos** (`/orgs/:orgId/payments`, `GET /api/orgs/:orgId/payments`), só do dono: os cartões (o que
  pagar, atrasados incluídos, o que paga em 7 e 30 dias, as horas da equipe e quem está sem regra), **Próximos
  pagamentos** (os períodos sem baixa, o corrente de cada pessoa e os fechados dos últimos 60 dias, marcados Atrasado,
  e o restante dos que já tiveram baixa, com **Dar baixa**) e **Pagos recentemente** (as baixas dos últimos 30 dias,
  com o bônus, o restante que o período ganhou depois da baixa e **Desfazer**; `POST` e `DELETE /api/orgs/:orgId/payouts`). As tabelas seguem a data do pagamento, não as
  horas.

## Contas a receber (modo solo)

Na organização do **modo solo** (uma pessoa só), o dono acompanha o que cobra de cada cliente na página **A receber**
(`/orgs/:orgId/receivables`, `receivables.manage`, só o dono; `404` fora do modo solo). Cada conta a receber é de um
cliente e de um período: o app **calcula** o valor e o vencimento, e o dono **recebe** direto ou, antes, **ajusta** o
valor, o vencimento e a nota (Decisão 24). Só o recebido e o ajustado ficam gravados, como um **recebível**: um por
cliente e período, com o valor da conta e, depois, o que entrou. O sistema não emite
nota: o número dela é só uma anotação. As rotas estão em [../api/reference.md](../api/reference.md#contas-a-receber-modo-solo).

**Como o período fecha.** O cliente tem um `closing_day` (1 a 31; 31 é o último dia do mês, e um mês curto fecha no
último dia dele) e os `payment_days` entre o fechamento e o vencimento. O ciclo é a mesma conta da regra mensal de
pagamento da pessoa (`payment.PeriodAt`): o período vai do dia seguinte ao fechamento do mês anterior até o fechamento,
os dois dias inclusivos, e **fecha à meia-noite do fim do dia do fechamento, no fuso da organização**. O vencimento é o fim do
período mais os `payment_days`.

**O ciclo de 14 dias** (`billing_cycle: biweekly`, só no cliente por hora) é o bi-weekly dos clientes dos EUA. O dono
informa a `cycle_anchor`, o último dia do último período que o cliente já pagou, e os períodos começam no dia seguinte,
com 14 dias cada. As horas até a âncora já foram acertadas fora do app e **nunca são propostas**. Exemplo: âncora
27/10/2026 e 3 dias de prazo dão 28/10 a 10/11 (vence 13/11), 11/11 a 24/11 (vence 27/11), e assim por diante. É a
mesma conta da regra quinzenal da pessoa, com 14 dias em vez de 15 (`PaymentRule.PeriodDays`, que não sai na API). A
âncora não pode ser depois de hoje; para um cliente novo, o dono usa o dia 14 dias antes do primeiro fechamento.

**O que o sistema calcula** (na hora; nada é gravado até o dono receber ou ajustar):

| Modo do cliente | Períodos calculados | Valor calculado |
|---|---|---|
| **Por hora** | Os períodos **já fechados** com tempo nos projetos do cliente, ainda sem recebível. No máximo os 24 mais recentes no ciclo mensal e os 52 no de 14 dias (cerca de dois anos nos dois). O período corrente, com tempo, entra em Em andamento com as horas até agora | A soma das sessões de **todos os projetos do cliente**, cortadas no período e arredondadas ao centavo **por sessão**, pelo valor cobrado congelado nela (a mesma conta de [Da sessão ao dinheiro](#da-sessão-ao-dinheiro)). A sessão aberta conta até agora |
| **Mensal** | Do período do `contract_start` até o **período corrente** (o seguinte só entra quando começa), parando no período que contém o `contract_end`, ainda sem recebível. No máximo os 60 mais recentes. Um `contract_start` no próprio dia de fechamento começa no **período seguinte**: no do início o contrato teria só esse último dia, e sem pro-rata isso seria um mês cobrado por um dia (início em 05/10 com fechamento no dia 5: o primeiro período é 06/10–05/11) | A `monthly_fee_cents` mais os acréscimos mensais dos projetos do cliente, como estão hoje (`extra_cents` diz quanto deles). O primeiro mês parcial, e um acréscimo que já acabou, o dono corrige ao ajustar: não há pro-rata |

A conta traz também o **valor por hora efetivo** (o valor dividido pelas horas). Abaixo de 10,00
(`validate.MinRateCents`) vira um **aviso** na tela, nunca um erro: no mensal, as horas só estão completas depois que o
período fecha.

**Receber direto** (`POST /receivables/receive`, o período calculado) grava o recebível já recebido, num passo só: as
horas e o valor que o sistema calcula naquele momento (`seconds`, `calculated_cents`, e `amount_cents` igual a eles), o
vencimento padrão, o recebimento, o número da nota e a observação, e a moeda da organização (sem conversão).

**Ajustar** (`POST /receivables`, o "Editar" da tela) grava o recebível sem recebimento, com o valor que o dono quer
cobrar (`amount_cents`), o vencimento, a nota e a observação; a conta continua em A receber, marcada como ajustada, e
`PATCH` a altera de novo. Nos dois casos, uma hora registrada depois não reescreve o que foi gravado.

- **Por hora**: só depois que o período fechou (`409 receivable.period_open`).
- **Mensal**: a qualquer momento, inclusive adiantado (o período corrente, antes de fechar: o "Recebi adiantado" da
  tela), mas só dentro do contrato e até o período corrente. O seguinte só quando começar.
- O período tem de ser um do ciclo do cliente (`400 receivable.not_a_period`) e grava uma vez só
  (`409 receivable.already_confirmed`, garantido também pelo índice único em cliente e período).
- Um período calculado sem valor (sessões sem valor cobrado) não se recebe direto (`400 receivable.invalid_amount`):
  ajuste o valor antes.

**Receber** a conta ajustada (`POST /receivables/:id/receive`) registra o dia (hoje, por padrão; depois de hoje é
recusado), o valor e, se vier, o número da nota. As regras do valor valem também para o recebimento direto:

- **Por hora**: entra o valor da conta; multa e juros são recusados.
- **Mensal**: o valor recebido é livre (maior que zero); a **multa** e os **juros** são zero ou mais e são partes
  informativas do valor recebido, então somados não passam dele.

**Multa e juros sugeridos** (`GET /receivables/:id/late-charges?on=` na conta ajustada, e
`GET /receivables/late-charges?customer_id=&period_start=&period_end=&on=` na calculada), só no mensal. Se a data passa de
`vencimento + grace_days`:

```
multa = valor × late_fee_bp / 10000
juros = valor × late_interest_bp_month / 10000 × diasAtraso / 30
```

Os dois são arredondados ao centavo (o meio centavo sobe). `diasAtraso` são os dias entre o **vencimento** e a data: a
tolerância adia a cobrança, mas não os dias que contam (passou da tolerância, os juros valem desde o vencimento). Antes
disso a sugestão é zero. Exemplo: 6.300,00 com 2% de multa e 1% ao mês, recebidos 10 dias depois do vencimento, são
126,00 de multa, 21,00 de juros e 6.447,00 no total. É só uma sugestão: o dono aceita ou muda os valores no modal de
receber.

**Atrasado** é derivado, não gravado: sem recebimento e com hoje depois do vencimento, na conta calculada e na
ajustada.

**Desfazer.** `DELETE /receivables/:id/receive` desfaz o recebimento (a multa e os juros informados se vão com ele), e
a conta volta para A receber como ajustada. `DELETE /receivables/:id` ("Voltar ao valor calculado") apaga o recebível,
e o período volta a ser calculado. Com o recebimento registrado ele não se apaga (`409 receivable.already_received`):
desfaça o recebimento antes. Editar (`PATCH`: valor, vencimento, nota e observação) também só vale enquanto não foi
recebido.

**A página** (`GET /receivables`). Quatro cartões (em andamento, a receber, atrasado e recebido no mês) e três listas:

- **Em andamento** (`in_progress`): o período atual de cada cliente, ainda aberto. O mês corrente do contrato mensal,
  com a mensalidade, e o período corrente do por hora com tempo, com as horas e o valor até agora. A linha mostra em que
  dia do período está e quando fecha; no mensal, "Recebi adiantado" recebe o mês antes de fechar. Um mês ajustado
  adiantado (gravado e não recebido) também fica aqui, pelo valor ajustado, até o período fechar.
- **A receber** (`to_receive`): o que fechou e não entrou, calculado (`id` nulo) ou ajustado (`adjusted`), do
  vencimento mais antigo (o mais atrasado) para o mais novo, com o selo do vencimento: atrasado há N dias, vence hoje,
  amanhã ou em N dias. Ações: Receber e Editar.
- **Recebidos** (`received`): os 50 mais recentes, com Desfazer recebimento.

"A receber" inclui o atrasado; "Recebido no mês" é a soma de `received_cents` com `received_on` no mês do calendário da
organização.

**Limites que valem saber.** Os períodos são calculados do cadastro do cliente de hoje, e "já cobrado" compara as
datas exatas do período. Por isso o cliente com recebíveis não troca o `closing_day` (`customer.closing_day_locked`;
mandar o dia que ele já tem passa): um dia novo geraria períodos por cima dos gravados e as mesmas horas seriam
calculadas de novo. Desfeitos os recebíveis, o dia muda. O ciclo e a âncora travam do mesmo jeito
(`customer.cycle_locked`), com uma exceção: passar para o de 14 dias, ou mover a âncora, com ela no fim do recebível
mais recente ou depois, porque aí os períodos novos começam depois de tudo o que já foi gravado. É assim que um
cliente com histórico mensal passa para 14 dias. Pelo mesmo motivo, e porque a sessão não guarda o cliente (vale
o do projeto de hoje), o projeto com sessões não sai de um cliente que já tem recebíveis (`project.customer_locked`).
Receber um recebível cujo valor foi editado depois de a tela lê-lo é recusado (`receivable.changed`): o dono confere e
tenta de novo. Excluir o cliente com recebíveis é recusado (`customer.has_receivables`); apagar a organização leva tudo.

## Receita e Receita projetada (modo solo)

A tela inicial do [modo solo](overview.md#painéis) mostra duas contas de **datas diferentes**, e elas não se misturam. A
janela é de calendário no fuso da organização: Este mês, Mês passado ou Ano (de 1º de janeiro ao fim do mês corrente).

| | Receita | Receita projetada |
|---|---|---|
| O que é | O que **entrou**: o dinheiro que o dono marcou como recebido | O que se **espera** dos períodos de cobrança que terminam na janela, pagos ou não |
| A data que conta | O dia do recebimento (`received_on`) | O último dia do período (`period_end`) |
| Valor | A soma de `received_cents` (com a multa e os juros do mensal, que são partes dele) | O `amount_cents` do recebível, quando o período já foi gravado (ajustado ou recebido); senão o calculado |

- **Uma cobrança de janeiro recebida em 5 de fevereiro é Receita de fevereiro** e projetada de janeiro. Desfazer o
  recebimento tira o valor da Receita e não mexe na projetada; voltar ao valor calculado devolve o período ao
  calculado, e a projetada passa a ser o das horas (ou a mensalidade).
- **O gravado vale mais que o calculado.** Se o dono ajustou o valor, é o ajustado que se espera, e as horas
  registradas depois de gravar não o mexem. Cada período conta uma vez só: o calculado já exclui os períodos que têm
  recebível.
- **O calculado, na Receita projetada.** No cliente por hora ela traz também o **período em andamento**, com as horas até
  agora vezes o valor cobrado congelado nas sessões (no meio do mês, é o que se tem para receber até o fechamento). No
  mensal é a mensalidade mais os acréscimos do período do contrato, também o corrente. Um período corrente que termina
  depois do fim do mês fica para o mês dele.
- **Valor por hora efetivo** = Receita projetada ÷ horas da janela, ao centavo, ou um travessão sem horas. Abaixo do
  piso de 10,00 por hora é um aviso. O total divide pelas horas de **todos** os projetos, internos também; a linha de um
  cliente divide pelas horas dos projetos dele.
- **Limite que vale saber.** As horas são da janela de calendário e a projetada é dos períodos que **terminam** nela. Com
  `closing_day` 31 (o padrão) os dois coincidem; com outro dia de fechamento o mês tem horas de um período que só termina
  no mês seguinte, e o valor por hora efetivo do cliente é uma aproximação (chega a zero, com o aviso, num mês em que
  nenhum período dele termina). No ciclo de 14 dias um mês tem dois ou três fins de período, e a conta também é uma
  aproximação.
- **Os tetos do calculado** (por cliente por hora, 24 períodos fechados no ciclo mensal e 52 no de 14 dias; 60 por
  cliente mensal) não chegam a valer na tela: uma janela tem no máximo 12 períodos mensais de cada cliente, ou 26
  fechados do ciclo de 14 dias, sempre os mais recentes.

## Quem vê o quê

O dinheiro é um eixo à parte dos cargos: o admin gere gente e projetos, mas a cobrança e os pagamentos são do dono.

| Número | Dono | Admin | Administrador de projeto | Colaborador |
|---|---|---|---|---|
| Cliente e valor cobrado | ✔ | — | — | — |
| Receita e margem (projeto e organização) | ✔ | — | — | — |
| Custo do projeto e da organização | ✔ | ✔ | ✔ (seu projeto) | — |
| Valor pago dos colegas | ✔ | ✔ | ✔ (seu projeto) | — |
| O próprio valor pago e o que a sessão rendeu a ele | ✔ | ✔ | ✔ | ✔ |
| Regra e pagamentos de outra pessoa | ✔ | só as horas | — | — |
| Os próprios pagamentos | só as horas | ✔ | ✔ | ✔ |
| Contas a receber (modo solo) | ✔ | — | — | — |
| Receita, Receita projetada e valor por hora efetivo da tela inicial (modo solo) | ✔ | — | — | — |

Na prática, a API apaga o que quem chama não pode ver antes de responder. O valor cobrado, a receita e a margem saem
das sessões e da visão geral do projeto sem `billing.view`, e da visão geral da organização para quem não é o dono. O
valor pago dos colegas sai sem `rates.view` ou `rates.manage`. A regra de pagamento
(`payment`) de uma pessoa só vai para ela mesma e para o dono. A tabela completa, rota por rota, está em
[permissions.md](permissions.md).

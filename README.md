# Working Time Tracker

Ponto por tarefa para equipes que trabalham por projeto. Foi pensado principalmente para empresas de desenvolvimento, como software houses e consultorias, que atendem vários clientes e alocam as pessoas em um ou mais projetos, mas nada nele depende disso. A pessoa faz **clock-in** numa tarefa, faz **clock-out** quando para, e o sistema soma o tempo por tarefa e por pessoa. Tudo fica organizado em organização, projetos e times, e as tarefas podem apontar para issues do GitHub ou do GitLab e para cartões do Trello.

É um único binário em Go que serve a API JSON e a interface web.

## O que dá para fazer

- **Contas e organizações.** O signup cria uma organização com você como admin e **dono** dela. Outras pessoas entram por **convite** (uso único, válido por 7 dias, opcionalmente preso a um email): com o Clerk ligado o convite com email **sai por email**; sem ele, é um link para copiar.
- **Cadastro e boas-vindas.** O cadastro da organização tem **duas etapas**: a organização (o nome dela, o **país** e o seu nome) e o acesso (email, senha e a **confirmação da senha**, que precisa ser igual; um botão mostra ou oculta as duas). A conferência é só da tela: o servidor recebe uma senha, de 8 a 72 caracteres. O Google, do Clerk, e o "Já tem conta?" ficam abaixo do formulário, nas duas etapas. Ao entrar pela primeira vez, **o dono** (só ele) abre a página inicial com as **boas-vindas**, um modal em quatro etapas: os dados mais importantes da organização (resumo, segmento, regime de trabalho, fuso e moeda), o primeiro cliente, o primeiro convite (por email ou por link, como na aba Colaboradores) e, por fim, onde ajustar cada coisa, com um link para a documentação (`/help`). As três primeiras podem ser puladas (**Ver depois**, **Cadastrar depois**, **Convidar depois**), e fechar o modal de qualquer jeito (Concluir, Esc, o X ou o fundo) o encerra para sempre: a data fica em `persons.onboarded_at`, e `POST /api/auth/onboarding/complete` a grava na primeira vez. Quem entra por convite não vê as boas-vindas, e as pessoas que já existiam quando o recurso chegou já constam como tendo visto (a migração preenche a data). O `GET /api/auth/me` traz `needs_onboarding`. O seed marca o dono da demonstração como já tendo visto.
- **Perfil da organização.** Resumo, descrição, segmento, contato, dados jurídicos (razão social, o documento fiscal do país, endereço) e como ela trabalha (regime remoto, híbrido ou presencial, fuso e moeda). O admin edita; todos os membros leem na aba **Sobre**, onde campo sem valor aparece como "Não informado", e cada cartão tem um ícone no título.
- **País da organização.** A organização tem um **país**, o código (`BR` ou `US`), escolhido na etapa 1 do cadastro (o seletor já vem com o país do idioma da página: pt-BR sugere o Brasil, en sugere os EUA) e alterável na edição. É ele que decide o que vale nos dados jurídicos, sem mudar a tabela (uma só, com todas as colunas, as de outro país ficando vazias): o **documento fiscal** é o **CNPJ** no Brasil e o **EIN** (`XX-XXXXXXX`) nos EUA, cada um conferido pela regra do país dono dele; o **estado** é uma lista (as 27 UFs, ou os 50 estados, DC, territórios e códigos militares dos EUA) e o **código postal** é o CEP (`00000-000`) ou o ZIP (`00000` ou `00000-0000`); os rótulos, as máscaras (que se aplicam enquanto se digita) e o placeholder do telefone seguem o país. Ao criar a organização, o país define também a moeda e o fuso de partida (BRL e America/Sao_Paulo, ou USD e America/New_York); depois disso, trocar o país **não** muda a moeda nem o fuso. Trocar o país também não apaga o documento do outro país (ele fica guardado e escondido na tela), e um estado ou código postal já guardado que não serve ao país novo faz o pedido ser recusado, em vez de ser apagado: na tela, trocar o país limpa os dois campos para a pessoa escolher de novo. O **cliente** também tem país (o código ISO, que começa no da organização, e pode ser qualquer um): o documento dele é conferido pela regra desse país, o CNPJ no Brasil e o EIN nos EUA, e nos outros países é um texto livre de até 32 caracteres, sem conferência; trocar o país do cliente esvazia o campo na tela, e na API confere o documento guardado pelo país novo e recusa o que não serve. O cadastro dos países (rótulos, máscaras, validadores, estados, moeda e fuso) está em `internal/country`, e a tela o recebe em `window.BOOT.countries`, então um país novo é uma entrada nova ali, mais os textos em `countries.*` dos catálogos.
- **Início da organização.** A página inicial é um painel, sem título nem frase de abertura: começa direto nos blocos. Para **admins**, abre com a **Visão geral** de todos os projetos somados, numa janela de **7 dias, 30 dias ou tudo** (30 dias ao abrir; trocar de janela não pede nada ao servidor): as **suas horas**, as **horas da equipe** (as dos outros; o total de todos vai na dica), quem está com o ponto aberto, a **receita**, o **custo** e a **margem** (com a parte da receita que ela é), e a **Equipe**: quem está com o ponto aberto agora e em que tarefa cada pessoa num cartão de ponta a ponta (o avatar, o nome, "Trabalhando agora", a tarefa e o projeto; quem tem mais de uma tarefa na sessão mostra a primeira e "e mais N"; na outra ponta, um ícone que abre a tarefa), quem trabalha agora primeiro e depois as outras pessoas por nome, cinco por página, com os atalhos Ver colaboradores e Adicionar colaborador. De propósito não há horas por pessoa nem ordem por quem trabalhou mais: comparar tempo dá a entender que quem trabalhou mais merece mais reconhecimento, e o valor entregue nem sempre vem das horas. O bloco por horas ficou guardado no código (`partials/team_hours.gohtml`), sem uso. A lista é de agora, não da janela de 7 dias, 30 dias ou tudo, e o botão Atualizar a relê. As horas e os valores seguem a regra do projeto, cada sessão arredondada ao centavo, e a sessão que atravessa a borda da janela conta só o trecho de dentro; por isso o total da organização é a soma dos projetos. Para **todos**, de qualquer papel, o **Seu painel** vem antes dos projetos: quatro números (as **horas de hoje**, com a tarefa em que o ponto está aberto ou "Sem ponto aberto"; as **horas da semana**, de segunda a domingo, com a parte da jornada combinada quando há uma; as **tarefas abertas**, com quantas estão atrasadas ou vencem na semana; e as **concluídas**, com a parte que são das suas tarefas), a lista **Suas tarefas** de todos os projetos (abertas, com o prazo mais perto primeiro e as atrasadas à frente, ou concluídas; cinco por página, e a linha inteira abre a tarefa) e **Sua semana**, sete barras de horas por dia mais as horas da semana **por projeto**. É só o que é da própria pessoa (nada de dinheiro nem de colegas), medido no fuso do navegador, e se relê sozinho a cada minuto com a aba à vista (`GET /api/orgs/:orgId/me/overview?tz=` e `GET /api/orgs/:orgId/me/tasks?state=open|closed&page=`, de qualquer membro). A tarefa não guarda quando foi fechada: "concluídas" é o total, e a lista vai da mais nova para a mais antiga. Para **todos**, vêm também os **projetos em cartões** (quem não é admin vê só os projetos em que foi posto, com valor por hora; sem nenhum, a lista explica isso), **seis por página**, de fundo neutro e com a cor dele só num toque (a sigla em tom suave, os ícones e a borda ao passar o mouse; o tom é sorteado pelo id, sempre o mesmo), o nome, o cliente (ou "Projeto interno"), a descrição em duas linhas e quantas pessoas e tarefas tem; a sprint, a daily, a weekly e a reunião saíram do cartão e continuam nas configurações do projeto. A página fica no endereço (`?page=2`). O botão **Novo projeto** fica junto do título da lista, e, no bloco da Equipe, o **Adicionar colaborador** fica ao lado do Ver colaboradores e abre a página de colaboradores já com o convite aberto. Para **admins**, o dono e quem tem o papel de admin, a **Lista de tarefas** põe uma bolinha ao lado do responsável (verde: está com o ponto aberto agora, em qualquer projeto; cinza: não; a dica diz em qual tarefa) e a aba **Colaboradores** ganha a coluna **Agora**, com a tarefa em que cada pessoa está; a lista se atualiza sozinha a cada 30 segundos (`GET /api/orgs/:orgId/working-now`, que não leva tempo nem dinheiro). Quem não é admin não vê nada disso nem chama a rota.
- **Papéis.** Admins gerenciam a organização, as pessoas, os clientes, os projetos, os times, os valores e as integrações. Membros gerenciam tarefas e batem o próprio ponto, e fazem mais só se receberem **permissões**: num projeto, por **grupo** (colaborador, gerente de projeto, financeiro ou administrador do projeto, escolhido ao pôr a pessoa nele), ou na organização (criar projetos, cuidar dos clientes e das pessoas, liberadas pelo dono). A lista está em [`_docs/permissions.md`](_docs/permissions.md). O **dono** é o admin que criou a organização: um só por organização, sempre admin, e as horas dele valem o valor cobrado, sem custo, em vez de um valor pago (o que ele tira do projeto é a margem). Ele entra sozinho nos projetos que cria e nos que passa a trabalhar.
- **Jornada semanal** de cada pessoa. É o que a organização combinou com ela, em horas por semana, e vale para todos os projetos em que trabalha. O admin informa na aba Colaboradores da organização; a pessoa lê no perfil.
- **Clientes e valores por hora.** Cada projeto pode ter um cliente e o **valor cobrado** dele por hora. Cada pessoa tem um **valor pago** por hora em cada projeto, então a mesma pessoa pode receber 20 num projeto e 25 em outro. O admin vê e altera tudo, com a margem por hora; o membro vê só o que ele mesmo recebe.
- **Projetos** com duração da sprint (7 dias, 14 dias ou 1 mês), daily (com horário) e weekly (com dia e horário). O projeto pode ter só uma das duas, as duas ou nenhuma: no formulário, "O projeto tem daily" e "O projeto tem weekly" são marcas, e sem elas o projeto fica sem a reunião. Num projeto com cliente há ainda a reunião semanal com o cliente (dia e horário), no grupo "Cliente e cobrança": "Possui reunião semanal com o cliente" é outra marca, e tirar o cliente do projeto apaga a reunião. Esses valores são de cada projeto, não da organização, porque projetos diferentes podem trabalhar de formas diferentes. A aba Configurações só mostra o projeto; o admin altera tudo no modal Editar projeto.
- **Colaboradores e times** de cada projeto. A aba Colaboradores abre com um resumo (colaboradores, quantos estão sem time e times; para o admin, também a margem por hora somada, com os valores de hoje, e o valor das horas registradas, que soma as sessões de ponto já fechadas, cada um com um botão de dica que explica a conta) e mostra quem trabalha no projeto em duas visões, escolhidas logo abaixo dele: **Pessoas**, a lista de todos, com busca, os times de cada um e, para o admin, quanto recebe por hora e a margem; e **Times**, um cartão por time com os integrantes. Nenhuma lista mostra mais de cinco pessoas por vez: a tabela e cada cartão têm páginas. A pessoa entra no projeto com o valor por hora dela, que é obrigatório, e só quem já está no projeto entra num time. Adicionar pessoa tem dois passos: o primeiro escolhe a pessoa, o valor e o time, e o segundo, o **grupo de permissões** dela no projeto (colaborador, gerente de projeto, financeiro ou administrador do projeto; o dono e os admins já têm tudo e pulam o segundo passo). **Quem ainda não está na organização** entra pelo e-mail: ao digitar um e-mail que não é de ninguém da organização, a lista oferece **Convidar fulano@…**; a pessoa segue pelos mesmos dois passos (valor, time e grupo) e, no fim, o convite é enviado (um terceiro passo mostra como chegou e o link, para copiar). Quando ela aceitar, entra no projeto com o valor, o time e o grupo escolhidos. Os convites ainda não aceitos aparecem numa lista **Convites pendentes** abaixo das pessoas (sem ação na linha; revogar é na aba Colaboradores da organização). O modal do colaborador mostra o grupo atual e deixa trocar. Para o admin, clicar numa pessoa, na tabela ou no cartão de um time, abre o modal do colaborador, onde ele muda o valor por hora, marca os times e tira a pessoa do projeto. O lápis de cada cartão abre o modal Editar time, onde o admin troca o nome, marca quem faz parte, entre as pessoas do projeto, e exclui o time. Só quem está em algum time do projeto pode ser responsável por tarefas.
- **Lista de tarefas e Minhas tarefas.** Cada tarefa tem prazo (7 dias por padrão, com destaque quando está atrasada ou perto de vencer) e vínculo opcional com itens externos (uma issue do GitHub, um cartão do Trello: um por plataforma, então a mesma tarefa pode estar nas duas), e o **responsável é opcional**: uma tarefa criada sem responsável fica disponível na lista, e quem bater o ponto nela passa a ser o responsável, mesmo sem estar num time. Também dá para **pegar a tarefa sem bater o ponto**: o botão **Pegar**, na linha da lista das sem responsável, e **Pegar tarefa**, na página da tarefa, passam a tarefa para você sem mexer no status nem abrir sessão, e ela vai para Minhas tarefas; se outra pessoa pegou antes, a tarefa continua com ela e a tela avisa (a lista se atualiza). A **Lista de tarefas** abre dizendo o que é (onde o time acha o que ninguém pegou) e tem **duas listas**, cada uma com a sua paginação de 10: **Sem responsável**, para o time pegar, e **Com responsável**, só com as tarefas de **outras pessoas**: as suas ficam na aba Minhas tarefas, e a descrição da seção diz isso. A **linha inteira** de uma tarefa abre a página dela (o cursor vira a mãozinha, e uma dica acima de cada tabela avisa), sem botão Detalhes, e o nome é um texto em negrito, não um link sublinhado; a coluna de item externo saiu da lista, e o vínculo continua na página da tarefa. A aba **Minhas tarefas** reúne só as tarefas de que você é responsável, uma lista por status, na ordem em que o trabalho anda (Backlog, Em progresso, Aguardando fechamento e Fechada, esta recolhida), cada uma na mesma tabela da Lista de tarefas (tarefa, status, prioridade e prazo, com a linha inteira clicável e o botão Iniciar), e, no alto, o mesmo painel do timer do Início, de ponta a ponta, para ver há quanto tempo você trabalha e o que está na sessão. No topo da Lista de tarefas ficam os filtros que valem para as **duas listas**: a busca por nome, o prazo (atrasadas, até hoje, até o fim desta semana ou da próxima, ou até uma data), a prioridade, o status e a etiqueta. O **filtro por responsável** fica dentro da lista Com responsável, porque as tarefas sem responsável não têm dono, e só vale para ela; ele não lista você, já que as suas tarefas estão em Minhas tarefas. Cada lista tem 10 tarefas por página. A tarefa nova é criada num modal.
- **Descrição em Markdown e modais em etapas.** A descrição da tarefa aceita Markdown (títulos, listas, negrito, código, citações, tabelas e links; sem imagens): a página da tarefa a mostra renderizada, e o modal tem o alternar Escrever / Pré-visualizar. O marked gera o HTML e o DOMPurify o sanitiza (só as tags permitidas, links só para `http`, `https` e `mailto`, abrindo em outra aba), tudo no navegador, com as duas bibliotecas vendorizadas e com hash conferido. A descrição vai até 65.536 caracteres (o mesmo teto do corpo de uma issue do GitHub). O modal **Nova tarefa** é um passo a passo: a etapa 1 é o nome e a descrição, e a etapa 2, o prazo, a prioridade, as etiquetas e o responsável. Editar uma tarefa não passa por ele (ver abaixo).
- **Status da tarefa.** Toda tarefa tem um **status**: backlog, em progresso, aguardando fechamento ou fechada. A tarefa nova nasce sempre em backlog, e o status muda em **Editar detalhes**, em qualquer ordem e por qualquer pessoa do projeto, ou sozinho: **bater o ponto** numa tarefa a põe **em progresso**, seja qual for o status dela (inclusive aguardando fechamento ou fechada), e parar ou pausar não a tira de lá. O quadro tem uma coluna de status e filtra por **um ou mais status** (a tarefa passa se tem qualquer um dos marcados), com o filtro na URL como os outros, e a página da tarefa mostra o status.
- **Prioridade e etiquetas.** Toda tarefa tem uma **prioridade** (urgente, alta, média, baixa ou sem prioridade, que é o padrão) e pode ter várias **etiquetas**. As etiquetas são de cada projeto e só têm nome: admins criam, renomeiam e excluem, inclusive na hora, pelo campo "Nova etiqueta" do modal Nova tarefa, e quem não é admin escolhe entre as que existem. O quadro filtra por **uma ou mais prioridades** e por **uma ou mais etiquetas** (a tarefa passa se tem qualquer uma das marcadas), com os filtros na URL como os outros; a lista de tarefas mostra a prioridade em uma coluna e usa as etiquetas só no filtro (as linhas têm nome, status, prioridade, prazo e a coluna **Integração**, e o responsável na lista Com responsável), enquanto Minhas tarefas as mostra sob o nome, e a página da tarefa e o modal Editar detalhes trazem os dois campos. **Cada prioridade e cada status tem a sua cor**, em todo lugar onde aparecem: os chips dos filtros, os selos da lista, de Minhas tarefas e da página da tarefa e os selects dos modais Nova tarefa e Editar detalhes. Prioridade: urgente vermelha, alta laranja, média verde, baixa azul e sem prioridade branca, de borda pontilhada. Status: backlog cinza, em progresso turquesa, aguardando fechamento amarelo e fechada roxa. Nenhuma cor se repete entre os dois grupos, e cada uma tem um par para o tema claro e outro para o escuro. Nos filtros, o chip desmarcado usa o fundo suave e o marcado enche com a cor.
- **Editar a tarefa, em três lugares.** A página da tarefa não tem mais um modal único de edição em etapas (esse passo a passo ficou só para a **Nova tarefa**). O **nome e a descrição** se editam pelo lápis no canto direito do cartão do nome, num modal só com esses dois campos (a descrição com Escrever / Pré-visualizar). Os **detalhes**, no cartão ao lado, têm o botão **Editar** (o nome acessível é “Editar detalhes”): um modal com o **status**, a **prioridade**, o **responsável**, o **prazo** e as **etiquetas** (admins criam etiqueta ali mesmo), na ordem do cartão; só a data de criação não se edita. O **Excluir tarefa** é um botão à parte, na largura do cartão de detalhes e entre ele e o das integrações (no celular, no fim da página), e pede confirmação em outro modal. Os detalhes vão por `PATCH /api/tasks/:taskId/attributes`, que agora aceita também `assignee_id` e `deadline` e deixa o nome e a descrição como estão; o nome e a descrição vão pelo `PATCH /api/tasks/:taskId`, só com esses dois campos. O responsável e o prazo só vão no pedido se a pessoa os mudou no modal, para uma edição dos detalhes não desfazer uma tarefa que alguém acabou de pegar nem tirar a hora de um prazo que veio do Trello.
- **Ponto e Visão geral pessoal.** Clock-in e clock-out com cronômetro ao vivo e um **balão da sessão** que acompanha todas as páginas: fechado, é uma faixa verde "Sessão ativa • 00:00:00" no canto de baixo à direita; ao clicar, abre para cima um cartão por tarefa em andamento, com o tempo dela na sessão e uma seta que leva a ela, e embaixo os botões Tarefas da sessão e Parar (Esc ou um clique fora fecham). Cada cartão tem ao lado um botão para parar só aquela tarefa. **Uma sessão pode ter várias tarefas:** o ponto começa por uma, e outras entram e saem depois: entram pelo botão Adicionar à sessão no Início e na página da tarefa, ou pelo modal da sessão, que mostra cada tarefa com o intervalo em que esteve nela, numa barra de tempo. O tempo vale para a sessão e para cada tarefa (tarefas em paralelo contam o tempo cheio cada uma; a pessoa, o projeto e os valores contam a sessão uma vez só), e as tarefas se mexem também depois de a sessão terminar (quem bateu o ponto e os admins). O **Início**, a primeira aba e a mesma tela para admin e membro, tem o **painel do timer** (o cronômetro, que mostra também quanto a sessão aberta já rendeu), ao lado do seu tempo de hoje e da semana. Ao clicar em Iniciar numa tarefa, o cartão dela vai para dentro do painel, que mostra as tarefas em andamento como cartões e, embaixo, desde que horas o ponto está aberto e os botões Parar e Tarefas da sessão; parado, o painel convida a iniciar uma das tarefas abaixo. As **suas tarefas** (as de que você é o responsável e que ainda não estão na sessão; ao parar, elas voltam), em cartões que abrem a tarefa inteiros (o mouse vira mão e o fundo escurece; sem botão Ver detalhes). De cima para baixo: o nome, o selo de status e o de prioridade (nas mesmas cores da lista), o prazo à esquerda (só quando há; atrasado em vermelho, vencendo em 48 horas em amarelo, e o de uma tarefa fechada é só a data) e o botão Iniciar à direita (com a tarefa na sessão aberta, o botão Parar tarefa ocupa o lugar dele), uma divisória e as etiquetas ("Sem etiquetas" quando não há). O nome, o status, o prazo e as etiquetas ficam nas mesmas alturas nos cartões de uma fileira (para pegar uma tarefa disponível, use o quadro de tarefas); e as **suas sessões**, cada uma com as suas tarefas, na tabela em que a linha inteira abre o modal dela (o horário é o botão para o teclado; não há mais o olhinho). O modal é grande e tem três grupos, cada um com o nome na borda como o de novo projeto: o Resumo, as Tarefas da sessão (um cartãozinho por tarefa, com o tempo, a linha do intervalo e o botão Abrir tarefa, que leva a ela) e, para quem pode mudar a sessão, o Adicionar tarefa. A tabela tem filtros de data e tarefa (com uma tarefa no filtro, o tempo e o valor da linha e dos totais são os dela na sessão), os totais do filtro num cartão à parte e dez sessões por página. O membro só enxerga as próprias sessões, e isso vale no servidor; o admin vê as de todos na Gestão. O banco garante uma única sessão aberta por pessoa.
- **Horas em dinheiro.** Cada sessão guarda os valores por hora de quando o ponto abriu, então **mudar um valor só vale dali em diante**. Quem não tem valor definido no projeto **não bate ponto**. Na tela de ponto, o membro vê quanto ganhou; o admin vê custo, receita e margem.
- **Sincronização das issues do GitHub com as tarefas**, nos dois sentidos. Com a caixa "Sincronizar as issues deste repositório com as tarefas" ligada na integração, **toda issue aberta do repositório vira uma tarefa** (em backlog, sem prazo, ligada à issue, com o ícone do GitHub e o número da issue na coluna **Integração** das tabelas (a Lista de tarefas e Minhas tarefas), o selo "GitHub #42" nos cartões e o cartão da integração na página da tarefa), e as duas ficam iguais: o **título** é o nome, o **corpo** é a descrição, as **etiquetas** são as etiquetas e o **responsável** é o responsável, ligado pelo **e-mail público** do usuário no perfil do GitHub (quem não publica e-mail fica sem correspondência: a tarefa fica disponível, e o cartão da integração conta quantos são). Fechar vale nos dois sentidos: a tarefa Fechada fecha a issue como concluída, reabrir reabre, e a issue fechada ou reaberta no GitHub muda a tarefa (uma issue reaberta tira a tarefa de Fechada para backlog). Projetos, milestone, comentários e pull requests são ignorados, e prioridade, prazo, horas e os status Em progresso e Aguardando fechamento ficam só aqui. **Se os dois lados mudarem o mesmo campo entre duas sincronizações, vale o GitHub**; as etiquetas não conflitam, cada lado soma e tira o que fez. A sincronização roda **pelo botão Sincronizar agora** do cartão (uma rodada completa), **sozinha** de `GITHUB_SYNC_INTERVAL` em `GITHUB_SYNC_INTERVAL` (o do GitHub; o comum é `SYNC_INTERVAL`) (5 minutos por padrão; o GitHub não alcança o servidor, então não há webhook), **na hora**, poucos segundos depois de alguém mudar uma tarefa ligada a uma issue, e **pelo botão Sincronizar do cartão de integração da página da tarefa**, que relê só aquela issue (para quem abre uma tarefa e quer ver o que há de novo no GitHub), e **pelo botão Sincronizar da lista de tarefas e de Minhas tarefas**, que faz a rodada completa das integrações do projeto e recarrega a lista. Ver [Sincronizar as issues com as tarefas](#sincronizar-as-issues-com-as-tarefas).
- **Gestão do projeto**, só para admins. Um botão **Gestão**, no fim da barra de abas, leva a uma área com abas próprias (Visão geral, Colaboradores, Integrações e Configurações) e a um link para voltar ao projeto; o membro não vê o botão e recebe "Página não encontrada" nos endereços da Gestão. A **Visão geral** da Gestão mostra, numa tela só: quantas pessoas e quantos times trabalham no projeto, as horas registradas, a receita, o custo e a margem; há quanto tempo o projeto existe, em dias, semanas e meses, contados do dia em que foi cadastrado; as integrações configuradas e se estão ativas; a atividade dos últimos 7 e 30 dias e quem está com o ponto aberto; as tarefas atrasadas; e as horas, o custo e a receita de cada pessoa. É uma fotografia da hora em que foi lida, com um botão para atualizar.
- **Sincronização dos cartões do Trello com as tarefas**, nos dois sentidos, pela mesma estrutura da do GitHub. Com a caixa "Sincronizar os cartões deste quadro com as tarefas" ligada, **todo cartão aberto do quadro vira uma tarefa** (com o selo "Trello" e o link curto do cartão), e as duas ficam iguais: o **nome**, a **descrição**, as **etiquetas** e a **data de entrega** (o prazo), nos dois sentidos, e a **data de criação** do cartão vira a da tarefa na importação. O cartão **arquivado**, ou com a data de entrega marcada como concluída, fecha a tarefa, e fechar a tarefa arquiva o cartão. Se os dois lados mudarem o mesmo campo, vale o Trello. As listas e o responsável ficam de fora por enquanto, e a tarefa criada aqui vira um cartão sozinha. Ver [Sincronizar os cartões do Trello com as tarefas](#sincronizar-os-cartões-do-trello-com-as-tarefas).
- **Integrações** com GitHub, GitLab e Trello; o GitHub e o Trello se oferecem, e o GitLab aparece no modal como "Em breve" (o código dele funciona, mas a API recusa criar integração nova; as que já existem seguem valendo e se editam). Todas usam a mesma estrutura: nome, o acesso (um token, validado na plataforma, guardado criptografado e que nunca volta nas respostas) e, em `metadata`, os campos próprios da plataforma (o repositório, o projeto, a chave e o quadro), que cada integração confere antes de falar com ela. **O GitHub se conecta por autorização, sem token para colar:** o botão **Conectar com o GitHub** leva ao site dele e, na volta, o modal pede o repositório numa lista dos que a conta enxerga (a integração nasce desativada e só é ativada ao escolher um; **Reconectar** renova o acesso ou troca de conta). Isso pede o app OAuth cadastrado no GitHub, ver [Conectar com o GitHub](#conectar-com-o-github); sem ele o botão avisa que não está configurado. **O Trello se conecta do mesmo jeito**, com a chave do app no `.env` e o **Conectar com o Trello**, ver [Conectar com o Trello](#conectar-com-o-trello). O admin cria e edita num modal só, que desenha os campos da plataforma escolhida; o cartão de cada integração é só de leitura, e desativar ou excluir também ficam no modal. Os detalhes do item (o título e o estado, aberto ou fechado, da issue ou do cartão; no Trello, fechado é o cartão arquivado ou com a data de entrega concluída) são buscados na hora, e se a plataforma não responde a tela mostra o motivo, sem quebrar.
- **Ajuda.** Uma página só, em `/help` (o endereço `/ajuda` leva para ela), com tudo o que uma pessoa nova precisa: o passo a passo do primeiro uso (conta, moeda e fuso, clientes, projeto, convite, pessoas no projeto com o valor por hora, tarefas, ponto e números), o que cada tela mostra e faz, o que é o valor por hora (o pago e o cobrado) e quem vê e faz o quê, e as dúvidas mais comuns. O **sumário fica à esquerda** e acompanha a rolagem: cada item rola até o trecho (no celular ele vira um bloco recolhível, acima do texto), o item da seção que está na tela fica marcado, e cada trecho tem endereço próprio (`/help#new-project`). É **pública**: dá para mandar o link a quem ainda não tem conta; logada, a pessoa a vê com a barra superior, que tem o item Ajuda. Sai em português e em inglês, pelos catálogos (`help.*`); os caminhos como "Organização › Sobre › Editar" são montados com as chaves das próprias telas, para o guia não divergir do que está escrito nos botões.

## Como rodar

Pré-requisitos: **Go 1.26+** e **Docker** (ou um PostgreSQL 14+ seu).

```bash
# 1. Configuração
cp .env.example .env            # ajuste INTEGRATION_ENCRYPTION_KEY (openssl rand -hex 32)

# 2. Banco (sobe o Postgres com o banco de dev e o de testes)
docker compose up -d

# 3. Dados de demonstração (opcional, só num banco vazio)
go run ./cmd/seed

# 4. Servidor
go run ./cmd
```

Abra **http://localhost:8080**. O seed cria a **Jatobá Software**, uma software house com doze pessoas, seis clientes (quatro do Brasil e dois de fora, um dos EUA, com EIN, e um da Alemanha, com documento em texto livre) e oito projetos (um deles interno), em que cada pessoa tem um valor por hora diferente conforme o projeto. Entre como:

| Papel | Email | Senha |
|---|---|---|
| Admin | `ana@example.com` ou `helena@example.com` | `demo12345` |
| Membro | `bruno`, `carla`, `diego`, `elisa`, `fabio`, `gabriela`, `henrique`, `isabela`, `joao` ou `larissa`, seguidos de `@example.com` | `demo12345` |

O que o seed põe nas telas, para elas aparecerem cheias:

- **Pessoas e times.** O App de Pedidos tem nove pessoas em quatro times, então a aba Colaboradores passa de uma página. O Painel do Lojista tem a Elisa com valor e sem time, e o João está sem time em três projetos.
- **Tarefas.** 62, com prioridade (as atrasadas são urgentes), status nos quatro valores (as sem responsável ficam no backlog) e etiquetas por assunto (backend, frontend, design, qualidade, infra), com prazos de ontem a um mês: algumas atrasadas, algumas para hoje e onze **sem responsável**, disponíveis no Quadro de tarefas. Vinte e quatro estão vinculadas a itens do GitHub, do GitLab e do Trello.
- **Dinheiro.** Cada projeto tem o seu valor cobrado, e cada pessoa, o seu valor pago. A Migração do ERP cobra menos do que a maioria recebe, e a margem fica negativa; o projeto interno não tem receita.
- **Sessões.** Cerca de 1.400 sessões fechadas nos últimos 75 dias, geradas por uma sequência fixa e seguindo a jornada de cada um; cerca de três em dez têm uma segunda tarefa em paralelo, que entra no meio da sessão. O mesmo seed dá sempre o mesmo banco. O Bruno e a Carla estão com o ponto aberto, então a Visão geral mostra "Trabalhando agora"; essas sessões ficam abertas até alguém parar.
- **Integrações.** Oito, de demonstração e **sem credencial**: aparecem nas telas, mas só buscam issues depois que você edita e informa um token seu.
- **Jornada.** 40 horas para a maioria, 30 para o Diego e a Gabriela, 20 para a Elisa e o João; a Ana, sócia, fica sem jornada definida.

Sem o seed, clique em **Crie uma organização** na tela de login.

Se a porta 5432 já estiver ocupada, suba o banco com `POSTGRES_PORT=5433 docker compose up -d` e ajuste a porta no `.env`.

Para gerar o binário: `go build -o wtt ./cmd && ./wtt`. Templates e arquivos estáticos vão embutidos nele via `embed.FS`.

### Variáveis de ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `API_PORT` | sim | Porta HTTP, por exemplo `8080` |
| `DATABASE_URL` | sim | Conexão com o PostgreSQL |
| `INTEGRATION_ENCRYPTION_KEY` | sim | Chave da criptografia AES-GCM dos tokens de integração. Trocá-la torna os tokens salvos ilegíveis: cada integração pede o token de novo na edição, e o resto dela (nome e `metadata`) continua |
| `COOKIE_SECURE` | não | `true` marca o cookie de sessão como `Secure`. Use em produção, atrás de HTTPS |
| `PUBLIC_URL` | para conectar o GitHub ou o Trello, ou entrar pelo Clerk | O endereço em que as pessoas abrem o sistema, sem barra no fim (`http://localhost:8080`). O GitHub devolve a pessoa a `PUBLIC_URL/integrations/github/callback`, e o Trello a `PUBLIC_URL/integrations/trello/callback` |
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | para conectar o GitHub | O Client ID e o Client secret do app OAuth cadastrado no GitHub. Sem eles, o botão Conectar com o GitHub avisa que não está configurado |
| `GITHUB_URL`, `GITHUB_API_URL` | não | Para um GitHub Enterprise (ou um servidor fake): o site (`https://github.com`) e a API (`https://api.github.com`), que são os padrões |
| `TRELLO_API_KEY` | para conectar o Trello | A chave de API do app do Trello (só letras e dígitos). Sem ela, o botão Conectar com o Trello avisa que não está configurado. `TRELLO_APP_NAME` muda o nome que a tela de autorização do Trello mostra (padrão "Working Time Tracker") |
| `TRELLO_URL`, `TRELLO_API_URL` | não | Para um servidor fake: o site (`https://trello.com`) e a API (`https://api.trello.com/1`), que são os padrões |
| `SYNC_INTERVAL`, `GITHUB_SYNC_INTERVAL`, `TRELLO_SYNC_INTERVAL` | não | De quanto em quanto tempo a rotina de fundo olha as integrações com a sincronização ligada (`5m`, `30s`...). `SYNC_INTERVAL` vale para todas as plataformas (padrão `5m`), e `GITHUB_SYNC_INTERVAL` e `TRELLO_SYNC_INTERVAL` valem só para a delas, sobre o comum; `0` desliga a rotina de fundo (de todas, ou só daquela plataforma), e o botão Sincronizar agora e a sincronização das tarefas que mudam seguem valendo. A rotina acorda no menor dos intervalos e olha cada integração quando passou o do tipo dela. O Trello não avisa quando um cartão muda, então o que muda lá chega no próximo intervalo (ou no botão); o que muda aqui vai em poucos segundos |
| `CLERK_SECRET_KEY`, `CLERK_PUBLISHABLE_KEY` | para entrar pelo Clerk | As duas chaves do app no Clerk (`sk_…` e `pk_…`). Sem elas o login é só por email e senha, como sempre. Uma só das duas, ou as duas sem `PUBLIC_URL`, derrubam a subida. Ver [Entrar com o Clerk](#entrar-com-o-clerk) |
| `CLERK_AUTHORIZED_PARTIES` | não | As origens (`http://localhost:8080`, sem barra) cujas sessões do Clerk o servidor aceita, separadas por vírgula. O padrão é a origem do `PUBLIC_URL`. Quem sobe o sistema em duas portas (a sua e a de uma conferência) lista as duas |
| `INVITE_DELIVERY` | não | O que o convite por email faz com o Clerk ligado: `email` (o padrão) pede ao Clerk que mande o email; `terminal` **não manda** e escreve o link do convite no log do servidor, para desenvolver sem caixa de entrada. Sem o Clerk, não vale nada: o convite é só o link. Com `COOKIE_SECURE=true` o servidor avisa na subida, porque o log passa a ter links de convite |
| `CLERK_API_URL` | não | Só para um servidor fake: o endereço da Backend API do Clerk, sem o `/v1`. O padrão é `https://api.clerk.com` |
| `TEST_DATABASE_URL` | só nos testes | Banco usado por `go test`. O nome precisa terminar em `_test` |

### Entrar com o Clerk

Com as chaves do [Clerk](https://clerk.com) no `.env`, o login, o cadastro e a página do convite oferecem **dois jeitos de entrar**: o **email e a senha do sistema**, em cima, e o **Google**, logo abaixo de um "ou" (é o Clerk que faz o Google). O email e a senha do próprio Clerk não são oferecidos. Sem as chaves, nada muda: só o email e a senha do sistema. O Clerk só diz **quem a pessoa é**; as organizações, os papéis e os convites continuam daqui, porque uma pessoa pertence a uma só organização e o email é único no sistema.

**Configurar**

1. No painel do Clerk, crie o app (ou, com o CLI, `npm install -g clerk`, `clerk auth login` e `clerk link --app <id>`). Copie a *Publishable key* e a *Secret key* para `CLERK_PUBLISHABLE_KEY` e `CLERK_SECRET_KEY` no `.env` (o `clerk env pull --file <arquivo>` escreve as duas num arquivo à parte), e defina `PUBLIC_URL`.
2. No painel do Clerk, em *User & authentication*, deixe o app só com o Google:
   - aba **Email**, o bloco de baixo, **Sign-in with email**: **desligado**. É ele que faz o Clerk mostrar o campo de email e o "Continuar" junto do botão do Google;
   - aba **Password**: **desligada**. A senha é só a do sistema;
   - aba **Email**, o bloco de cima (**Sign-up with email**, **Require email address** e **Verify at sign-up**): **ligado, não mexa**. O sistema só confia em email **verificado**, e é esse bloco que faz o Clerk guardar, como email principal e já verificado, o que o Google informa;
   - em *Social connections*, o **Google** ligado, e o cadastro **Public** (qualquer pessoa pode criar a própria organização). Organizations do Clerk ficam desligadas.

   Num app de desenvolvimento o Google usa as credenciais compartilhadas do Clerk; em produção é preciso cadastrar as do próprio projeto no Google.
3. Reinicie o servidor. O log mostra `clerk enabled=true`, o endereço do Frontend API e as origens aceitas. Se o campo de email do Clerk ainda aparece abaixo do botão do Google, a opção **Sign-in with email** do passo 2 continua ligada no painel.

**Como funciona.** Depois de entrar no Clerk, o navegador chega em `/auth/clerk/continue`, que pega o token de sessão do Clerk (vale 60 segundos) e o manda ao servidor em `Authorization: Bearer`. O servidor confere a assinatura, o prazo e a origem (`azp`), busca o usuário no Clerk e abre a **sessão do sistema**, o cookie de sempre: o resto da aplicação não sabe que o Clerk existe, e as páginas internas não carregam o clerk-js.

| Situação | O que acontece |
|---|---|
| O usuário do Clerk já está ligado a uma pessoa | Entra |
| Há uma conta com o mesmo email verificado, **sem senha** | É ligada e entra |
| Há uma conta com o mesmo email, **com senha** | Pede a senha dela, uma vez, e só então liga. O cadastro por senha não verifica o email: sem essa prova, quem cadastrou o email de outra pessoa com uma senha dele ficaria com a conta |
| Não há conta | Mostra os convites pendentes para o email, para a pessoa escolher, ou pede o nome da organização para criar a dela. Nada é aceito sozinho: entrar numa organização não se desfaz |
| A pessoa chegou pelo link de um convite (`invite_token`) | Entra direto na organização dele; se o convite tem email, o email verificado do Clerk precisa ser o mesmo |
| A conta está ligada a outro usuário do Clerk que ainda existe | Recusa (`auth.clerk_account_linked`); se esse usuário foi apagado, religa |

**Convidar por email.** Em Colaboradores, **Adicionar colaborador** com um email envia o convite pelo Clerk (o botão passa a dizer **Enviar convite por email**): o servidor cria um convite do Clerk para esse email, com `redirect_url` no `/invite/<token>` do sistema, e só grava o convite daqui se o Clerk o aceitou (se não, a tela mostra o erro e nada fica gravado). A pessoa abre o link da mensagem e o Clerk a leva ao `/invite/<token>` do sistema, que mostra o formulário do convite (nome, email travado no do convite e senha **do sistema**) e, abaixo, o Google. O `__clerk_ticket` que o Clerk acrescenta à URL **não é usado**: sai da URL antes de o Clerk carregar, porque aproveitá-lo abriria um cadastro com email do próprio Clerk. Pelo Google, a pessoa volta em `/auth/clerk/continue?invite=<token>` e entra na organização com o papel do convite, desde que o email da conta Google seja o do convite. Na prática o email do Clerk é só quem entrega o link: o link do sistema também aparece no modal, e se a mensagem não chegar dá para copiá-lo e mandar à mão, com o mesmo resultado. Convidar o mesmo email de novo **substitui** o convite anterior (aqui e no Clerk): é o "reenviar". Revogar um convite o cancela no Clerk também (um problema lá não impede de revogar aqui). Sem email, ou sem o Clerk, o convite é só o link, como sempre foi.

No desenvolvimento, `INVITE_DELIVERY=terminal` faz o servidor **não** pedir o envio do email e escrever no log o email, o papel, a organização e o link do Clerk (o mesmo que iria no email, com o ticket; se o Clerk não o devolver, o link do sistema):

```
invite not emailed (INVITE_DELIVERY=terminal) email=caio@mail.com role=member organization=Acme link=https://….clerk.accounts.dev/v1/tickets/accept?ticket=…
```

O modal avisa que o email não foi enviado. Num app de desenvolvimento do Clerk, os endereços `qualquer+clerk_test@example.com` nunca recebem email de verdade (nem no modo `email`).

**Sair** leva a `/login?out=1`, e a página também desconecta do Clerk: sem isso ele entraria de novo sozinho. Com o Clerk ainda logado, uma sessão do sistema que expirou volta sozinha, para a página que a pessoa abriu. `/auth/clerk/continue` nunca manda de volta ao login em círculo: em erro mostra o motivo e deixa tentar de novo ou entrar com outra conta.

**Limites.** O email não é sincronizado entre o Clerk e o sistema (mudar o email no perfil não muda o do Clerk). Quem entra só pelo Clerk não tem senha do sistema, e o perfil não mostra a troca de senha. Os textos do Clerk em português vêm de `web/static/clerk-pt-BR.js` (o `@clerk/localizations`, licença MIT); o inglês é o do próprio Clerk.

### Conectar com o GitHub

O botão **Conectar com o GitHub** usa um **OAuth App** do GitHub, que se cadastra uma vez por servidor:

1. Em github.com/settings/developers, **New OAuth App**. *Homepage URL*: o `PUBLIC_URL` (`http://localhost:8080`). *Authorization callback URL*: `PUBLIC_URL/integrations/github/callback` (`http://localhost:8080/integrations/github/callback`).
2. Copie o **Client ID** e gere um **Client secret**. No `.env`: `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` e `PUBLIC_URL`. Reinicie o servidor.
3. Em Gestão, Integrações, **Nova integração**, GitHub, **Conectar com o GitHub**: autorize no GitHub, volte e escolha o repositório.

Pontos de atenção:

- Um OAuth App tem **uma URL de retorno só**: um app serve um endereço do sistema. Para desenvolvimento e produção, dois apps.
- O escopo pedido é `repo`, o único que dá acesso a repositório privado num OAuth App. Ele também permite escrita, e a **sincronização das issues a usa**: com a caixa ligada, o sistema muda título, corpo, etiquetas, responsáveis e estado das issues (e cria etiquetas que o repositório ainda não tem) **com a conta de quem conectou**. Apagar uma issue é a exceção: a API comum do GitHub não a apaga, só o GraphQL (`deleteIssue`), e só para quem é **admin do repositório**; sem isso a issue de uma tarefa excluída é apenas fechada (ver Importação). Sem a caixa ligada, ele só lê issues. O token fica criptografado e nunca sai do servidor. Se isso for um problema para quem usa, a saída é um GitHub App, de permissões mais finas.
- Se um repositório de uma **organização** não aparecer na lista, o dono da organização precisa aprovar o app, nas configurações dela, em acesso de aplicativos de terceiros.
- A lista traz os 100 repositórios mexidos há menos tempo; os outros se digitam no campo.

### Sincronizar as issues com as tarefas

Na edição da integração (e na escolha do repositório, logo depois de conectar, onde a caixa já vem marcada), a caixa **Sincronizar as issues deste repositório com as tarefas** liga a sincronização. Só liga com a integração ativa e o repositório escolhido, e com ela ligada **o repositório não troca** (desligue, troque e ligue de novo; trocar com ela desligada esquece o vínculo das issues do repositório antigo). Quem liga precisa da permissão de integrações, mas **qualquer pessoa do projeto que edita uma tarefa sincronizada escreve no GitHub com a conta de quem conectou**: a página da tarefa avisa isso na edição.

Como funciona, por campo (cada issue guarda o **snapshot do último acordo**, e é contra ele que se vê quem mudou):

- **Nome, descrição:** se o GitHub mudou desde o acordo, a tarefa fica com o valor dele; senão, se a tarefa mudou, a mudança vai para a issue. Finais de linha do Windows e espaços nas pontas não contam como mudança.
- **Estado:** a tarefa Fechada fecha a issue; reabrir a tarefa reabre. No GitHub, issue fechada fecha a tarefa; issue reaberta tira a tarefa de Fechada para backlog (uma tarefa aberta, em progresso ou aguardando fechamento, não muda).
- **Etiquetas:** o resultado é o que o GitHub tem, mais o que a tarefa ganhou, menos o que a tarefa perdeu, e vai para os dois lados. Uma etiqueta que o repositório não tem é criada antes de a issue recebê-la (o GitHub a descartaria sem avisar); renomear ou excluir uma etiqueta aqui passa a mexer nas issues das tarefas que a têm.
- **Responsável:** a tarefa tem um responsável só, e a issue pode ter vários. Só o usuário **ligado** à pessoa é mexido: o `login` do GitHub só se liga a uma pessoa **do projeto** se o **e-mail público** dele, no perfil do GitHub, é o dela (o e-mail é único no sistema, então a busca é sempre dentro da organização). Os outros responsáveis da issue ficam como estão nos dois sentidos. Escolher aqui alguém cujo e-mail não aparece em nenhum perfil do GitHub não manda nada e deixa o aviso "não achei o usuário do GitHub". Tirar o responsável no GitHub tira daqui, e o contrário também. Quem publica o e-mail no GitHub depois passa a ser reconhecido na rodada seguinte (o botão pergunta tudo de novo).
- **Importação:** só issue **aberta** vira tarefa. Uma tarefa que já estava ligada à issue à mão **passa a ser sincronizada** em vez de a issue virar outra tarefa (o GitHub vence no nome e na descrição, e as etiquetas dos dois lados se somam). **Excluir a tarefa descarta a issue** (ela não volta a ser importada), e o modal de exclusão tem a caixa **Apagar a issue no GitHub**: marcada, a issue é apagada pelo GraphQL e, se a conta conectada não é admin do repositório, é **só fechada** como não planejada, e o aviso diz isso; desmarcada, a issue não é tocada; **desvincular** a tarefa (pela API: a tela não tem o botão) a tira da sincronização, e a issue vira descartada; uma tarefa criada aqui **só vira issue se a pessoa pedir**, na etapa Integrações do modal Nova tarefa (ver abaixo). Uma issue apagada ou transferida fica marcada como sumida, e a tarefa fica como estava.

Quando roda: o botão **Sincronizar agora** faz uma rodada completa (todas as issues abertas, e a conferência das que já eram ligadas) e responde o que fez (`created`, `updated`, `closed`, `pushed`, `unmapped`, `errors`, `partial`); a rotina de fundo pede só as issues mexidas desde a última rodada, e a cada hora faz uma completa; e o gancho das tarefas empurra para o GitHub, em poucos segundos, o que mudou numa tarefa ligada (editar, mudar status ou etiquetas, pegar uma tarefa livre, bater o ponto nela). Uma tarefa nas duas plataformas leva a edição às duas. Se uma integração está **em espera** (o token recusado, o limite de requisições), a edição não se perde: fica guardada e sai quando ela se recupera (a rotina de fundo a devolve à fila quando a espera acaba, e um **Sincronizar** que dá certo, como o de depois de reconectar o GitHub, acaba a espera na hora). Enquanto a integração está parada, o cartão dela na página da tarefa avisa, com o motivo, que a sincronização está parada e que o que se muda ali fica guardado. Só uma rodada roda por vez em cada integração (o botão responde `409` se já há uma). Na página de uma tarefa ligada a um item, há um cartão da integração para cada item (um por plataforma: a tarefa pode ter a issue do GitHub e o cartão do Trello ao mesmo tempo), e o botão **Sincronizar** de cada cartão relê aquele item na hora e põe os dois em acordo pelas mesmas regras; para cada integração ativa, com a sincronização ligada, em que a tarefa **ainda não está** (e só enquanto ela não está Fechada), a página oferece um cartão **Publicar no Trello** (ou **no GitHub**): o botão cria o item novo com o que a tarefa tem (os mesmos avisos do modal Nova tarefa, em duas linhas) e a liga a ele, e daí em diante os dois ficam sincronizados. É assim que uma issue que veio do GitHub vira cartão do Trello, e o contrário, sem a tarefa sair do lugar; usa `POST /api/tasks/:taskId/publish`, e um erro da plataforma (token sem escrita, quadro sem lista) aparece no próprio cartão; nas integrações sem sincronização (o GitHub ou o Trello com a caixa desligada) o botão é **Atualizar** e só busca os detalhes de novo. O cartão tem também o botão **Abrir no GitHub** e o estado da issue (Aberta ou Fechada). A lista de tarefas e Minhas tarefas têm o botão **Sincronizar** (só aparece se o projeto tem uma integração com a sincronização ligada): uma rodada completa em cada integração do projeto, aberta a quem está no projeto, e a lista se recarrega com o que veio. As listas não se atualizam sozinhas: depois de uma rodada de fundo, recarregue a página. Uma rodada sem diferença nenhuma não escreve nada no GitHub.

**Uma tarefa em mais de uma plataforma.** Uma tarefa tem **no máximo um item por integração**, e um item serve a **uma tarefa só**; pode estar ao mesmo tempo numa issue do GitHub e num cartão do Trello (`POST /api/tasks/:taskId/publish` a cada integração, ou o vínculo à mão). A **tarefa é o centro**: cada item guarda o seu snapshot do último acordo com ela, e uma mudança num lado chega à tarefa na rodada dele e dela ao outro item pelo gancho das tarefas, em poucos segundos (nome, descrição, etiquetas, estado, o prazo no Trello, o responsável no GitHub); fechar a issue fecha a tarefa e arquiva o cartão, e reabrir faz o caminho de volta. Se os dois lados mudarem o mesmo campo ao mesmo tempo, cada rodada dá a vez ao seu lado e a última ganha; o resultado converge. Uma etiqueta que o GitHub recusa trava aquele empurrão inteiro (o aviso `issue_sync.push_rejected` fica no cartão dele). O vínculo à mão (`POST /api/tasks/:taskId/link-external-item`) cria um vínculo **pendente**, que a primeira rodada em que o item está aberto adota; um item já fechado não é adotado. Excluir a tarefa solta todos os itens (o modal de exclusão tem uma caixa para cada plataforma em que a tarefa está, todas desmarcadas: `DELETE /api/tasks/:taskId?remove_in=<integração>`, repetido; a tarefa sai mesmo que uma plataforma falhe, e o resultado de cada uma volta na resposta e no aviso da tela), e trocar o repositório da integração (`ResetSync`) esquece também os vínculos à mão dela.

**Postar uma tarefa nova como issue e como cartão.** Quando o projeto tem uma integração ativa, o modal **Nova tarefa** ganha uma terceira etapa, **Integrações**, com um cartão para cada integração do projeto e uma caixa para marcar onde postar a tarefa. **As caixas não se excluem: a tarefa vai para todas as marcadas**, então um clique só a cria na aplicação, no GitHub e no Trello. Só as integrações com a **sincronização ligada** têm a caixa: a do **GitHub vem desmarcada** (a tela posta a issue depois de criar a tarefa), e a do **Trello vem marcada** (o servidor posta o cartão sozinho, poucos segundos depois; desmarcá-la manda `skip_publish` na criação e a deixa de fora, para criar uma tarefa só no GitHub). O GitHub ou o Trello com a sincronização desligada e o GitLab aparecem desativados, com o motivo ("Sincronização desligada", "Em breve"). Cada plataforma tem o seu aviso no cartão; o do GitHub diz que vão para a issue o **nome** (título), a **descrição** (corpo) e as **etiquetas** (criadas no repositório se faltarem), que **prazo e prioridade não existem numa issue** e ficam só aqui, que o **responsável só vai se o e-mail público do usuário dele no GitHub for o mesmo e-mail cadastrado aqui** (senão a issue sai sem responsável, e a tarefa fica com ele) e que, depois de postada, a tarefa e a issue ficam sincronizadas nos dois sentidos. A tarefa é criada primeiro e depois postada (`POST /api/tasks/:taskId/publish`); se o GitHub recusar, a tarefa existe e o aviso diz o motivo. O token precisa escrever no repositório: com um token só de leitura o GitHub abriria a issue sem as etiquetas e o responsável, então a tarefa não é postada.

O cartão da integração mostra a última sincronização, quantas issues têm responsável sem correspondência e o último aviso:

- **o token não escreve no repositório** (ou ele está arquivado): as issues só vêm para cá e as mudanças daqui não vão (modo somente leitura);
- **o GitHub ignorou uma mudança** (`200` sem gravar, o que ele faz com quem não tem permissão para etiquetas ou responsáveis): a mesma mudança não é repetida a cada rodada, só uma mudança nova a tenta de novo;
- **limite de requisições** do GitHub: a rodada para, o que já fez fica feito, e a rotina espera o limite acabar (o token revogado também faz a rotina esperar, cada vez mais);
- o GitHub recusou a mudança, ou não deixou criar a etiqueta.

Pontos de atenção: em um repositório **público**, quem abre uma issue cria uma tarefa (o Markdown é limpo ao exibir). A descrição de uma tarefa vai até 65.536 caracteres e uma etiqueta até 50, que são os limites do GitHub, para a sincronização não cortar o texto. Responsável só por e-mail funciona para poucos, porque quase ninguém publica o e-mail no GitHub; um campo "usuário do GitHub" na pessoa resolveria e é um passo pequeno.

Para ver isso numa instância sem tocar no GitHub de verdade, `go run ./cmd/fakegithub -addr :8091` sobe o GitHub falso dos testes (o site, com a autorização, e a API no mesmo endereço) com issues de vários jeitos e um painel em `/_fake` para mexer nelas como se fosse outra pessoa; aponte o servidor para ele com `GITHUB_URL` e `GITHUB_API_URL` e use `GITHUB_CLIENT_ID=test-client-id` e `GITHUB_CLIENT_SECRET=test-client-secret`.

### Conectar com o Trello

O botão **Conectar com o Trello** usa a **chave de API** de um app do Trello, que se cria uma vez por servidor:

1. Em trello.com/power-ups/admin, crie um Power-Up (ele serve de app) e, na aba **API key**, gere a **API key**. Se o Trello recusar o endereço de retorno, acrescente o `PUBLIC_URL` (`http://localhost:8080`) em **Allowed origins**, na mesma aba.
2. No `.env`: `TRELLO_API_KEY` (e `TRELLO_APP_NAME`, se quiser outro nome na tela de autorização) e `PUBLIC_URL`. Reinicie o servidor.
3. Em Gestão, Integrações, **Nova integração**, Trello, **Conectar com o Trello**: autorize no Trello, volte e escolha o quadro numa lista dos quadros abertos que a conta enxerga, com o espaço de trabalho de cada um (a integração nasce desativada e só é ativada ao escolher um; **Reconectar** renova o acesso ou troca de conta).

Pontos de atenção:

- A chave é pública; o segredo é o **token** de cada pessoa. O Trello o devolve no **fragmento** da URL de retorno (`#token=…`), que o navegador não manda ao servidor: a página `/integrations/trello/callback` lê o token, apaga o endereço do histórico e o entrega ao servidor (`POST /integrations/trello/token`), que confere o cookie da conexão, o `state`, a pessoa e a permissão e guarda a integração com o token criptografado. O token nunca volta nas respostas.
- O acesso pedido é de leitura e escrita nos quadros, **sem prazo**; quem conectou o revoga na conta do Trello, e **Reconectar** troca por um token novo.
- A chave do app fica guardada junto de cada integração (o token do Trello pertence à chave que o gerou), então trocar a chave no `.env` não quebra as integrações que já existem; reconectar passa a usar a chave atual.
- O token da conexão escreve nos quadros **com a conta de quem conectou**: com a sincronização ligada, qualquer pessoa do projeto que edita uma tarefa sincronizada muda o cartão com essa conta.

### Sincronizar os cartões do Trello com as tarefas

Na edição da integração (e na escolha do quadro, logo depois de conectar, onde a caixa já vem marcada), a caixa **Sincronizar os cartões deste quadro com as tarefas** liga a sincronização, com as mesmas regras do GitHub: só liga com a integração ativa e o quadro escolhido, com ela ligada **o quadro não troca**, e a mesma rodada de fundo, o mesmo gancho das tarefas e os mesmos botões (**Sincronizar agora** no cartão da integração, **Sincronizar** na página da tarefa e nas listas) valem para as duas plataformas.

**Cada cartão aberto do quadro vira uma tarefa** (em backlog, ligada ao cartão, com o selo "Trello" e o link curto do cartão), e as duas ficam iguais:

- **Nome, descrição, etiquetas e data de entrega** (o prazo da tarefa): se o Trello mudou desde o acordo, a tarefa fica com o valor dele; senão, se a tarefa mudou, a mudança vai para o cartão. **Se os dois lados mudarem o mesmo campo, vale o Trello.** Uma etiqueta do Trello só com cor aparece pelo nome da cor; uma etiqueta daqui que o quadro não tem é criada nele antes de o cartão recebê-la, com uma cor tirada do nome. A data é comparada ao segundo.
- **Estado:** o cartão **arquivado**, ou com a **data de entrega marcada como concluída**, fecha a tarefa. Fechar a tarefa arquiva o cartão e, se ele tem data de entrega, a marca como concluída; reabrir a tarefa (backlog) desarquiva e desmarca. Só um cartão **aberto** vira tarefa.
- **Data de criação:** a tarefa importada nasce com a data em que o cartão foi criado (só na importação; o Trello não deixa escolher a de um cartão novo).
- **Ficam de fora:** as **listas** (por ora, um cartão é uma tarefa em qualquer lista), o **responsável** (o Trello não mostra o e-mail dos membros), comentários, checklists e anexos; prioridade, horas e os status Em progresso e Aguardando fechamento ficam só aqui.
- **Tarefa criada aqui vira um cartão sozinha** (no modal, a caixa do Trello já vem marcada; `POST /api/projects/:projectId/tasks` sem `skip_publish` faz o mesmo): criar uma tarefa num projeto com o Trello ligado e a sincronização ligada a posta na **primeira lista aberta do quadro**, com o **nome**, a **descrição**, as **etiquetas** (criadas no quadro se faltarem) e o **prazo** (como a data de entrega), e a tarefa fica ligada ao cartão e sincronizada. Quem cria a tarefa não espera o Trello: a criação só avisa, e a sincronização a posta poucos segundos depois (a tarefa aparece com o selo "Trello" na lista seguinte). A postagem sozinha é **por integração**: a tarefa que a etapa Integrações do modal posta no GitHub também vira cartão, a que já tem um cartão deste quadro (ligado à mão) não ganha outro, e uma tarefa criada com `skip_publish: ["<id da integração do Trello>"]` fica fora (os ids desconhecidos não casam com nada, e um que não é UUID dá `400` `task.integration_not_found`). A desmarcação vale também nas tentativas seguintes de uma tarefa que ficou esperando. Se o Trello recusar por si (um quadro sem lista aberta, um token que só observa), a tarefa existe sem cartão e o cartão da integração mostra o aviso; se ele estiver fora do ar ou pedir para esperar, a tarefa fica esperando e a rotina de fundo ou o botão **Sincronizar** a tenta de novo (dez vezes, no máximo). A fila é da memória do servidor: um reinício esquece as tarefas que ainda esperavam, e `POST /api/tasks/:taskId/publish` as posta. Tarefas importadas de um cartão não voltam como cartão.
- Uma tarefa que já estava ligada ao cartão à mão (pelo link curto ou pelo endereço do cartão) passa a ser sincronizada em vez de o cartão virar outra tarefa. Um id de 24 caracteres colado no vínculo não é reconhecido: use o link curto. Excluir a tarefa descarta o cartão (ele não volta a ser importado), e o modal de exclusão tem a caixa **Arquivar o cartão no Trello**: marcada, o cartão é arquivado (não apagado: dá para restaurá-lo pelo Trello, e a data de entrega não muda); desmarcada, o cartão não é tocado; um cartão apagado ou levado para outro quadro fica marcado como sumido, e a tarefa fica como estava.

O Trello não filtra os cartões por data, então **toda rodada olha o quadro inteiro** (uma chamada para os cartões abertos, mais uma por cartão ligado que saiu da lista); o limite do Trello é de 100 requisições por 10 segundos por token. Um token que só observa o quadro traz os cartões e não escreve nada, e o cartão da integração avisa; uma descrição acima de 16.384 caracteres o Trello recusa (a mudança fica só aqui). Uma rodada sem diferença nenhuma não escreve nada no Trello.

Para ver isso numa instância sem tocar no Trello de verdade, `go run ./cmd/faketrello -addr :8092` sobe o Trello falso dos testes (o site, com a autorização, e a API no mesmo endereço) com um quadro de cartões de vários jeitos e um painel em `/_fake` para mexer neles como se fosse outra pessoa; aponte o servidor para ele com `TRELLO_URL=http://localhost:8092`, `TRELLO_API_URL=http://localhost:8092/1` e `TRELLO_API_KEY=0123456789abcdef0123456789abcdef`.

O servidor e o seed aplicam as migrações pendentes ao iniciar. São arquivos SQL versionados, e só eles mudam o schema: veja [Migrações do banco](#migrações-do-banco).

## Interface web

A interface segue a Decision 8 de `_docs/design.md`. O servidor renderiza a casca de cada página em HTML (Go `html/template`), já sabendo quem está logado e qual é a organização e o projeto. Os componentes [Alpine.js](https://alpinejs.dev) buscam e alteram os dados pela API JSON em `/api`. **Não há etapa de build**: o CSS e o JavaScript são servidos como estão. A única coisa que vem de fora são os ícones ([Font Awesome Free](https://fontawesome.com)), carregados do cdnjs com SRI; sem acesso à CDN a interface continua funcionando, só sem os ícones.

### Rotas do navegador

| Rota | Tela |
|---|---|
| `/login`, `/signup` | Entrar e criar organização (em duas etapas, com a senha digitada duas vezes), com o email e a senha do sistema. Com o Clerk ligado, o Google aparece logo abaixo, depois de um "ou"; `/login?out=1` é o login depois de sair |
| `/invite/:token` | Aceitar um convite e criar a conta, com o email e a senha do sistema (o email vem travado no do convite). Com o Clerk ligado, o Google aparece logo abaixo, e o ticket do Clerk na URL é ignorado |
| `/auth/clerk/continue` | Para onde o Clerk volta depois de entrar: troca o token dele por uma sessão do sistema. Pede a senha de uma conta antiga, mostra os convites pendentes ou pede o nome da organização. Sem o Clerk ligado, leva ao login. Aceita `?next=` e `?invite=` |
| `/help`, `/ajuda` | A ajuda, **pública** (não pede sessão): o passo a passo do primeiro uso e o que cada tela faz, numa página só, com o sumário fixo à esquerda. `/ajuda` redireciona para `/help` |
| `/` | Leva para a organização de quem está logado |
| `/orgs/:orgId` | O **Início**: para admins, a visão geral da organização (horas, receita, custo e margem de todos os projetos, em 7 dias, 30 dias ou tudo, e a equipe); para todos, os projetos em cartões discretos, seis por página (`?page=`). Quem pode cria projeto num modal, pelo botão junto da lista, já com o cliente e, quando há cliente, o valor cobrado por hora |
| `/orgs/:orgId/about` | Organização, aba Sobre: o perfil da organização, para todos os membros |
| `/orgs/:orgId/settings` | Organização, tela de edição aberta pelo botão Editar da aba Sobre: perfil, regime, fuso, moeda e exclusão da organização (só admins) |
| `/orgs/:orgId/people` | Colaboradores (item próprio da barra superior, que também é a aba da Organização): os integrantes, com o papel (que muda no botão da linha) e a jornada semanal de cada um (que muda num modal, pelo lápis); os convites pendentes e o botão Adicionar colaborador, que num modal envia o convite por email (com o Clerk ligado e um email informado) ou gera o link de convite para copiar (só admins). Com `?add=1` a página abre com esse modal já aberto: é o atalho do Início |
| `/orgs/:orgId/customers` | Organização, aba Clientes: quem contrata os projetos, com cadastro e edição num modal (só admins) |
| `/orgs/:orgId/projects` | Organização, aba Projetos: tabela de gestão com o cliente, os colaboradores e as tarefas de cada projeto; a linha abre o projeto (só admins) |
| `/profile` | Seu nome, seu email, sua senha (quem entra só pelo Clerk não tem), sua jornada semanal (só para ler) e quanto você recebe por hora em cada projeto |
| `/lang/:code` | Troca o idioma (`pt-BR` ou `en`): grava o cookie e volta para `?next=` |
| `/i18n/:idioma.js` | Os textos do idioma para o JavaScript (`window.I18N`) |
| `/projects/:projectId` | Leva todos para a Visão geral |
| `/projects/:projectId/overview` | O Início do projeto, igual para admin e membro: o relógio e as suas tarefas, o seu tempo de hoje e da semana e as suas sessões, com filtros de data e tarefa, totais e páginas |
| `/projects/:projectId/collaborators` | Colaboradores para todos, só para ler: as pessoas (com busca) e os times, cinco por página, sem valor por hora, margem nem ações, nem para o admin. Quem edita é a aba da Gestão. Para admins, a coluna Agora mostra quem está trabalhando agora e em qual tarefa |
| `/projects/:projectId/management` | A raiz da Gestão (só admins): leva para a Visão geral dela |
| `/projects/:projectId/management/overview` | Gestão, Visão geral: o cartão Projeto (pessoas, times, tempo de projeto), integrações, atividade recente, tarefas atrasadas e a lista de sessões de todos, com filtros de pessoa, data e tarefa, e dez por página. Horas, custo, receita e margem aparecem uma vez só, nos totais das sessões, que sem filtro são os do projeto Os caminhos antigos `/teams`, `/integrations` e `/settings` redirecionam para a Gestão |
| `/projects/:projectId/tasks` | Lista de tarefas, com duas listas (as sem responsável e as de outras pessoas), busca, filtros e uma paginação para cada; clicar em qualquer lugar da linha abre a página da tarefa. Para admins, uma bolinha ao lado do responsável mostra se ele está trabalhando agora |
| `/projects/:projectId/my-tasks` | Minhas tarefas: o painel do timer, de ponta a ponta, e as tarefas de que a pessoa é responsável, numa lista por status, com o botão Iniciar |
| `/tasks/:taskId` | A página da tarefa, uma **tela própria**, sem as abas do projeto: um cabeçalho com o botão Voltar à lista, o caminho (Projetos / o projeto) e as ações (Pegar tarefa e Iniciar). Só para ler, em quatro cartões: o do **nome e da descrição**, com o lápis no canto direito (abre o modal Editar nome e descrição), os **Detalhes** (status, prioridade, responsável com o email, prazo, etiquetas e a data de criação), com o botão **Editar** (abre o modal dos detalhes), o tempo registrado, logo abaixo da descrição, e um cartão de integração para cada item vinculado (a issue do GitHub, o cartão do Trello), mais a oferta de **publicar** nas integrações em que a tarefa ainda não está e o formulário de vincular à mão, com o botão **Excluir tarefa** entre os detalhes e as integrações. Em tela estreita os cartões ficam numa coluna, nessa ordem. O **Pegar tarefa** só aparece quando ela não tem responsável: passa a tarefa para você sem bater o ponto. A lista de tarefas leva a ela ao clicar na linha |
| `/projects/:projectId/time-tracking` | O endereço do antigo Ponto: redireciona para o Início |
| `/projects/:projectId/management/teams` | Gestão, Colaboradores, em duas visões: Pessoas (a lista, com busca) e Times (os cartões; `?view=teams` abre nela), com cinco pessoas por página. Para admins, também o valor por hora de cada pessoa, a margem e os modais de adicionar pessoa, do colaborador, de novo time e de editar time, e a coluna Agora (quem está trabalhando agora e em qual tarefa) |
| `/projects/:projectId/management/integrations` | Gestão, Integrações (GitHub e Trello; o GitLab aparece como "Em breve"): um cartão por integração e, para admins, o modal de criar e editar. Na volta do GitHub a aba recebe `?github=<id>` (abre a escolha do repositório, ou só avisa que o acesso foi renovado) ou `?github_error=<código>` (mostra o motivo); na do Trello, `?trello=<id>` ou `?trello_error=<código>`, da mesma forma, com a escolha do quadro |
| `/projects/:projectId/management/integrations/github/connect` | Começa a conexão com o GitHub (`?integration=<id>` reconecta uma que já existe): grava o cookie da conexão e redireciona para o GitHub. Mesma permissão da aba |
| `/integrations/github/callback` | Onde o GitHub devolve a pessoa (o endereço cadastrado no app). Confere o cookie, o `state`, a pessoa e a permissão, troca o código por um token e guarda a integração; redireciona para a aba |
| `/projects/:projectId/management/integrations/trello/connect` | Começa a conexão com o Trello (`?integration=<id>` reconecta uma que já existe): grava o cookie da conexão e redireciona para a autorização do Trello. Mesma permissão da aba |
| `/integrations/trello/callback` | A página em que o Trello devolve a pessoa, com o token no fragmento da URL (`#token=…`, que só o navegador vê). Só exige login: o script da página lê o token, apaga o endereço do histórico e o entrega a `POST /integrations/trello/token` |
| `POST /integrations/trello/token` | Corpo JSON `{state, token}`. Confere o cookie da conexão, o `state`, a pessoa e a permissão, guarda a integração (ou troca o token da que se reconecta) e responde `200 {"redirect": "…"}` com a aba e o `?trello=<id>` ou `?trello_error=<código>` |
| `/projects/:projectId/management/settings` | Gestão, Configurações: a descrição, a sprint, a daily, a weekly (dia e horário), o cliente e a reunião com o cliente (dia e horário), só para ler. Para admins, também o valor cobrado e o botão Editar, que abre o modal Editar projeto, onde ficam todos os campos e a exclusão |

Sem sessão, qualquer página leva ao login, menos as públicas (entrar, criar organização, o convite e a ajuda), e a pessoa volta para a página pedida depois de entrar. Uma página de outra organização mostra "Página não encontrada".

### Navegação

- A **barra superior** mostra a organização, o menu (Início, **Projetos**, Colaboradores, Organização e Ajuda; **Projetos** é um menu que lista os projetos da pessoa por nome, de qualquer página, todos para os admins e só os dela para os outros, com busca quando passam de sete, o projeto aberto marcado, o botão marcado nas páginas de projeto e o link Ver todos os projetos, que leva à lista da primeira tela; a lista vem junto da página, sem requisição, e o menu fecha com Esc, ao clicar fora ou ao escolher; o Colaboradores só aparece para quem cuida das pessoas, admins e quem recebeu essa permissão, e leva direto à configuração delas), o **toggle de idioma** (PT | EN), quem está logado (o nome leva ao **perfil**) e o botão Sair. Sem login, o toggle fica no canto da tela (na ajuda, que é pública, ele vai dentro do cabeçalho da página, para não cobrir o texto ao rolar). Com o ponto aberto, o **balão da sessão** fica fora da barra, no canto de baixo à direita de todas as páginas de quem está logado, acima dos avisos e abaixo dos modais.
- As páginas de projeto têm duas barras de **abas**. Para todos: Início (o relógio, as suas tarefas e as suas sessões), Minhas tarefas, Lista de tarefas e Colaboradores (só para ler), e o projeto abre no Início. Para admins, o botão Gestão abre a outra: Visão geral, Colaboradores, Integrações e Configurações, em `/projects/:id/management/...`. A aba Colaboradores juntou as antigas Times e Valores. O valor cobrado do cliente fica em Configurações.
- A página **Organização** abre na aba Sobre, que todos os membros leem. Nela o admin tem o botão **Editar**, que leva à tela de edição; salvar volta para a Sobre. As abas Colaboradores, Clientes e Projetos são só de admins.
- Ações de admin não aparecem para membros. A API continua sendo quem garante as permissões.
- A lista de **tarefas** guarda a busca, os filtros e a página na URL (`?q=`, `?assignee=` (o id de uma pessoa, só para a lista Com responsável; o seu próprio id é ignorado), `?due=`, `?priority=`, `?status=`, `?label=`, e `?free_page=` e `?taken_page=` para a página de cada lista). Recarregar mantém o que estava na tela, o link pode ser compartilhado, e o botão Voltar de uma tarefa leva de volta ao mesmo ponto da lista.

### Idiomas

A interface fala **português do Brasil** (o padrão) e **inglês**. Todo texto que a pessoa lê mora em dois arquivos YAML, `internal/i18n/locales/pt-BR.yaml` e `en.yaml`; o código só conhece as chaves.

- **Qual idioma vale:** o cookie `wtt_lang` (gravado pelo toggle, por um ano); sem ele, o `Accept-Language` do navegador; sem nenhum dos dois, português. O toggle é um link para `/lang/<código>?next=<página>`, então funciona sem JavaScript e nas telas sem login. O idioma não fica na conta: é uma escolha do navegador.
- **Servidor:** [go-i18n v2](https://github.com/nicksnyder/go-i18n) lê os YAML (`internal/i18n`). Os templates usam `{{.T "chave"}}` (dentro de `range` ou `with`, `{{$.T "chave"}}`); com valores, `{{.T "chave" "nome" .Nome}}`. Os títulos das páginas em Go usam `TitleKey: "titles.login"`.
- **Navegador:** o servidor entrega o mesmo YAML como `window.I18N` (`/i18n/<idioma>.js`, com o hash na URL para o cache), antes do `app.js`. No JavaScript, `WTT.t('chave', { nome: valor })`; nas expressões do Alpine, `$t('chave', { nome: valor })`.
- **Regra importante:** dentro de uma expressão Alpine (`x-text`, `:title`, `:placeholder`) use sempre `$t`, nunca `{{.T}}`. O `html/template` só escapa HTML, e um apóstrofo de um texto em inglês ("Don't") quebraria a expressão JavaScript.
- **Como escrever as chaves:** uma chave por **frase inteira**, nunca pedaços para juntar no código (a ordem das palavras muda de um idioma para outro); valores entram por placeholder (`"Em andamento: {{.name}}"`); plural com as formas `one` e `other` e o placeholder `{{.count}}`. No português o zero também é `one` (regra do CLDR), então "nenhuma pessoa" é uma chave à parte. Valores de enum (papel, moeda, regime) continuam sendo os códigos do backend; só o rótulo vem do YAML. Não use como nome de chave `id`, `description`, `hash`, `zero`, `one`, `two`, `few`, `many`, `other`, `translation`, `leftdelim` nem `rightdelim`: o go-i18n os lê como partes de uma mensagem e recusa o arquivo (use `description_label`).
- **Adicionar um idioma:** crie `locales/<código>.yaml` com as mesmas chaves, acrescente o código em `supported` (`internal/i18n/i18n.go`) e as chaves `lang.<código>.short` e `lang.<código>.name` em todos os catálogos. O teste confere o resto.
- **Testes:** `internal/i18n` garante que todos os idiomas têm as mesmas chaves e os mesmos placeholders, e que toda chave usada nos templates, no JavaScript e nas páginas em Go existe. Em `internal/server`, o idioma por cookie, por cabeçalho e pelo toggle.
- **Datas, horas e valores** seguem o idioma (`Intl` com `WTT.lang`): "R$ 20,50" em português e "R$20.50" em inglês. A **moeda** não muda com o idioma, é a da organização. Os campos de valor mostram o separador do idioma e leem os dois jeitos de digitar ("20,5", "20.50", "1.234,56" e "1,234.56"); com um separador só e três dígitos depois ("1.234"), vale o separador de milhar do idioma.
- **Erros da API:** a API não manda mensagem, só um código estável (`{"error": {"code": "auth.invalid_credentials", "params": {...}}}`). O navegador traduz pelo código, procurando `errors.<código>` no mesmo YAML (`WTT.errorText`), e `e.message` já chega no idioma da página. Os códigos são declarados em `errors.go` de cada domínio (`apperr.New`) e estão documentados, com o status e os textos, em [`_docs/error-codes.md`](_docs/error-codes.md), um arquivo gerado por teste. Um teste recusa código sem texto, texto sem código e placeholder que o código não declara.
- **Fora da tradução:** os dados de demonstração do seed e o que as pessoas digitam (nomes de projeto, tarefas, clientes) ficam como foram escritos. Logs e comandos de linha de comando seguem em português.

### Onde fica cada coisa

```
web/
  templates/
    layouts/base.gohtml        # HTML base, carrega os ícones (cdnjs), os textos do idioma, app.css, app.js, o script da página e o Alpine
    partials/                  # barra superior, toggle de idioma, cabeçalhos da organização e do projeto, balão da sessão, toasts, modal e ícone
    pages/*.gohtml             # uma casca por tela; cada uma define o bloco "content"
  static/
    app.css                    # estilos com tema claro e escuro, sem framework
    app.js                     # api() sobre fetch, estado de formulário, toasts, modal, formatadores, cronômetro
    pages/*.js                 # componentes Alpine de cada grupo de telas (auth, org, project)
    alpine.min.js              # Alpine.js 3.14.8 vendorizado (dist/cdn.min.js do pacote npm)
    marked.min.js              # marked 18.1.0 (lib/marked.umd.js): Markdown da descrição da tarefa
    purify.min.js              # DOMPurify 3.4.16 (dist/purify.min.js): sanitiza o HTML do Markdown
internal/page/                 # handlers das páginas
internal/i18n/                 # catálogos YAML (locales/), tradução, escolha do idioma e o script dos textos
internal/template/renderer.go  # um conjunto de templates por página, carregado na inicialização
```

Para atualizar o Alpine, o marked ou o DOMPurify, troque o arquivo em `web/static/` e o hash em `web/embed_test.go` juntos (o `marked.min.js` e o `purify.min.js` são os do pacote npm sem a última linha, o comentário `sourceMappingURL`). O teste existe porque a cópia vendorizada do Alpine já esteve corrompida sem ninguém perceber, e porque o DOMPurify é a única defesa contra XSS na descrição da tarefa.

**Ícones.** `{{template "icon" "pen"}}` emite o ícone do Font Awesome com esse nome (sem o prefixo `fa-`). O ícone é decorativo: o botão mantém o texto. Um botão só de ícone, como os de editar um colaborador e de editar um time na aba Colaboradores, leva a classe `btn-icon` e o nome da ação em `aria-label` e em `title`. Num botão com texto dinâmico, ponha o texto num `<span x-text>`, porque um `x-text` no botão apagaria o ícone. Para trocar a versão, mude a URL e o `integrity` em `base.gohtml` juntos; o hash vem de `https://api.cdnjs.com/libraries/font-awesome/<versão>?fields=sri`.

**Dica.** Para explicar um número sem ocupar a tela, o bloco leva `x-data="tip"` e a classe `has-tip`, um botão `tip-btn` com o ícone `circle-info`, `aria-label`, `aria-controls` e `@click="toggle()"`, e o texto num elemento `tip-pop` com `x-ref="pop"` e `x-show="open"`. O balão abre abaixo do bloco, não passa da borda da fileira nem da tela e fecha com Esc, com um clique fora ou quando a janela muda de tamanho. Hoje usam a dica os dois blocos de dinheiro do resumo da aba Colaboradores.

**Modal.** Há um só, no layout base (`partials/modal.gohtml`), controlado por `Alpine.store('modal')`. A página entrega o conteúdo e continua dona dele:

```html
<button type="button" class="btn" @click="$store.modal.open('customer', 'Novo cliente')">Novo cliente</button>

<template x-teleport="#modal-root">
  <form x-show="$store.modal.name === 'customer'" @submit.prevent="save()">
    <input x-model="draft.name" data-autofocus>
    ...
  </form>
</template>
```

O conteúdo aparece dentro do modal, mas segue no escopo do componente da página (`x-model`, `save()`, `errors`). O `<template>` precisa de um único elemento raiz. `open(nome, título, guarda)` abre e foca o campo com `data-autofocus`; `close()` fecha; `dismiss()` é o fechamento pedido pela pessoa (Esc, clique no fundo, X, Cancelar) e respeita a guarda, uma função que devolve `false` enquanto o modal não pode fechar, por exemplo durante um salvamento.

Hoje usam o modal o cadastro de cliente, o Adicionar colaborador, a Jornada e permissões da pessoa (para o dono, também as permissões da organização), o Novo projeto, o Editar projeto, a Nova tarefa e, na aba Colaboradores do projeto, o Adicionar pessoa, o Editar colaborador, o Novo time e o Editar time, e, na aba Integrações, o formulário único de criar e editar integração. Para focar um campo que acabou de aparecer dentro do modal (como o link do convite depois de gerado), dê ao bloco um `x-transition`: sem transição, o `x-show` só mostra o elemento no ciclo seguinte e o `$nextTick` chega antes de ele aceitar foco.

## API

A referência completa, com exemplos, está em [`_test/routes.md`](_test/routes.md), e os códigos de erro em [`_docs/error-codes.md`](_docs/error-codes.md). A coleção do Insomnia está em `_test/insomnia-collection.json`: importe, rode **Signup** ou **Login** e o Insomnia guarda o cookie de sessão para as outras chamadas.

Resumo dos grupos de rotas:

| Grupo | Rotas |
|---|---|
| Autenticação | `/api/auth/signup`, `login`, `logout`, `me`, `password`, `onboarding/complete`, `invites/:token`, `clerk/login`, `clerk/signup`, `clerk/join` |
| Organização | `/api/orgs/:orgId` (+ `persons`, `projects`, `overview`, `customers`, `invites`) |
| Clientes | `/api/customers/:customerId` |
| Pessoas | `/api/persons/:personId` (+ `role`, `weekly-hours`, `allocations`) |
| Projetos | `/api/projects/:projectId` (+ `overview`, `teams`, `tasks`, `labels`, `members`, `collaborators`, `invites`, `integrations`, `work-sessions`) |
| Valores | `/api/projects/:projectId/billing`, `/api/projects/:projectId/allocations` (+ `/:personId`) |
| Times | `/api/teams/:teamId` (+ `members`) |
| Tarefas | `/api/tasks/:taskId` (+ `claim`, `attributes`, `link-external-item`, `external-details`, `sync`, `publish`); a resposta traz `links`, um por item externo (`integration_id`, `integration`, `item_id`, `url`, `last_error`) |
| Ponto | `/api/projects/:projectId/work-sessions/*`, `/api/work-sessions/active` |
| Integrações | `/api/integrations/:integrationId` (+ `repositories`, `sync`) |

**Integrações.** O corpo é o mesmo para qualquer plataforma. O que muda de uma para outra vai em `metadata`:

```json
{
  "type": "trello",
  "display_name": "Quadro do app",
  "token": "…",
  "metadata": { "api_key": "…", "board_id": "https://trello.com/b/AbC123xy/app" }
}
```

| `type` | `token` | `metadata` |
|---|---|---|
| `github` | na tela, vem da autorização no GitHub (Conectar com o GitHub); a API também aceita um token pessoal com leitura de issues | `repo`: `dono/repositorio` ou o endereço |
| `gitlab` | token com escopo `read_api` | `project_url`: `grupo/projeto` ou o endereço |
| `trello` | na tela, vem da autorização no Trello (Conectar com o Trello); a API também aceita o token da API | `api_key`: a chave do app (a tela a guarda na conexão e não a deixa trocar); `board_id`: o endereço do quadro, o link curto ou o id; `board_name`: o nome do quadro, opcional, que a tela guarda ao escolher |

`GET /api/integrations/:integrationId/repositories` (`integrations.manage`) lista o que o token guardado enxerga (`[{"full_name": "dono/repo", "private": true}]` no GitHub; no Trello, os quadros abertos, `[{"id": "5abb…", "full_name": "Acme / App", "private": true}]`), para a tela oferecer a escolha; só os tipos que implementam `RepositoryLister` (o GitHub e o Trello) respondem, e o token nunca sai do servidor. O `Descriptor` diz como cada tipo dá acesso: `Auth: "oauth"` (o GitHub e o Trello: a tela mostra o botão Conectar, sem campo de token) ou o padrão, token. Os campos do `metadata` podem ser `internal` (a conexão os guarda e a edição os mantém), `hidden` (a tela os preenche e não os mostra) e ter um `picker` (`select`, a lista fechada do quadro; `datalist`, o campo com sugestões do repositório); `caps` diz o que a sincronização do tipo faz além do básico (`deadline`, `assignee`, `server_since`, `auto_publish`).

`POST /api/tasks/:taskId/publish` (corpo `{"integration_id"}`; qualquer pessoa que veja a tarefa) posta a tarefa como uma issue nova do repositório (ou um cartão novo do quadro, no Trello) e a liga a ela, sem soltar os outros itens da tarefa, e responde `{"task", "problem"}` (`problem` é o código do aviso que o GitHub deixou, como `issue_sync.no_login`); `400` `task.already_linked` (a tarefa já tem um item **nesta** integração), `integration.sync_off`, `task.integration_other_project`, `issue_sync.publish_read_only` ou `integration.issues_disabled`. `POST /api/projects/:projectId/sync` (qualquer pessoa do projeto) faz uma rodada completa em cada integração do projeto com a sincronização ligada e responde a soma dos resumos (`409` `integration.sync_running` se todas já estão numa rodada, `400` `integration.sync_off` se nenhuma está ligada). `POST /api/tasks/:taskId/sync` (qualquer pessoa que veja a tarefa; `?integration_id=` escolhe o item, e sem ele sincroniza todos, um depois do outro) relê o item de uma tarefa e responde o mesmo resumo mais `problem`, o código do aviso que o GitHub deixou (`409` `integration.sync_running`, `404` `integration.issue_gone`, `400` `task.no_external_item` ou `integration.sync_off`). `POST /api/tasks/:taskId/link-external-item` (corpo `{"integration_id","external_item_id","external_item_url"}`) liga a tarefa à mão a um item: o id vale como escrito ou, no Trello, como o link do cartão colado; `400` `task.already_linked` se ela já tem item nesta integração e `task.item_taken` se o item é de outra tarefa. `DELETE` na mesma rota, com `?integration_id=` (obrigatório com mais de um item), a solta. `GET /api/tasks/:taskId/external-details?integration_id=` lê o item na plataforma. `POST /api/integrations/:integrationId/sync` (`integrations.manage`) faz uma rodada completa da sincronização das issues com as tarefas e responde `{"created","updated","closed","pushed","unmapped","errors","partial"}`; `409` `integration.sync_running` se já há uma rodada na integração, `400` `integration.sync_off` com a sincronização desligada. A sincronização liga e desliga pelo `PATCH /api/integrations/:integrationId` com `{"sync_issues": true}`, e as respostas de integração trazem `sync_issues`, `last_synced_at`, `last_sync_error` e `sync_unmatched`.

O `gitlab` tem `ComingSoon: true` no `Descriptor`: o `POST` dele responde `400` com `integration.type_coming_soon`, e a tela mostra o tipo desabilitado com o selo "Em breve". Tirar a flag o oferece de novo, sem outra mudança.

Um tipo novo é um arquivo em `internal/adapter` que implementa `Integration` e uma linha no `registry.go`. O `Descriptor` dele diz que campos o `metadata` tem, e é dele que a tela tira o formulário e os rótulos (as páginas de projeto o recebem em `window.BOOT.integration_types`); o `CheckMetadata` confere e normaliza esses campos sem falar com a plataforma; `Validate` e `FetchItemDetails` recebem a `Connection`, com o token e o `metadata`.

## Segurança

- **Senhas** com bcrypt. Contas sem senha (criadas antes do login existir) não conseguem entrar.
- **Entrar pelo Clerk.** O token de sessão dele só vale se a assinatura confere com as chaves públicas do app, se não venceu (com uma folga de 10 segundos), se vem de uma origem em `CLERK_AUTHORIZED_PARTIES` (um token sem origem, ou de outro app da mesma instância, é recusado) e se o usuário existe e não está bloqueado. Só o email **primário e verificado** vale, e uma conta que já tem senha só é ligada ao Clerk com a senha. As rotas `/api/auth/clerk/*` levam o token em `Authorization`, não em cookie, então um site de fora não consegue chamá-las em nome de ninguém, e têm o mesmo limite de tentativas do login. A chave secreta fica só no servidor e nenhum token vai para o log. Uma sessão do sistema já aberta não cai quando o usuário é bloqueado ou apagado no Clerk (ela vale até 7 dias), e uma que expirou volta sozinha enquanto o Clerk estiver logado.
- **Sessões e convites** usam tokens aleatórios de 32 bytes, e o banco guarda só o sha256 deles. Trocar a senha encerra as outras sessões.
- **Cookie** `wtt_session` HttpOnly e SameSite=Lax, com `Secure` via `COOKIE_SECURE`. Como a API só aceita corpo JSON em `POST`, `PUT` e `PATCH`, um formulário de outro site não consegue agir em nome de quem está logado.
- **Isolamento entre organizações.** Cada rota com ID confere se o recurso é da organização de quem chama e responde 404 caso não seja.
- **Valores.** O valor cobrado do cliente só existe em rotas de admin: ele não entra no JSON do projeto. Na lista de valores de um projeto, um membro recebe só a própria linha, e na de colaboradores, só o próprio valor. Nas sessões de ponto, a API apaga o valor pago das sessões de outras pessoas e todo valor cobrado antes de responder a quem não é admin. A visão geral do projeto, que soma o custo e a receita de todos, é uma rota só de admins, e a página dela responde "Página não encontrada" a um membro.
- **Convite por email.** O `/invite/<token>` do sistema viaja na `redirect_url` do convite do Clerk e no email, então o token fica no Clerk; ele só serve a quem prova a caixa de entrada (o email do convite precisa ser o email verificado do Clerk), e quem tem só o link, sem convite com email, entra como sempre foi. No modo `terminal` os links dos convites vão para o log do servidor: só para desenvolver. Um convite que não deu para cancelar no Clerk não dá acesso a nada: o daqui já não existe.
- **Limite de tentativas** por IP em signup, login e convites.
- **Conectar com o GitHub** usa um `state` aleatório, selado (AES-GCM) num cookie `wtt_oauth` HttpOnly, SameSite=Lax, limitado ao caminho do callback e válido por 10 minutos, que serve para uma volta só. Na volta o servidor confere o `state` em tempo constante e que a pessoa e a permissão `integrations.manage` no projeto continuam valendo. Como é uma navegação GET e não uma chamada da API, a proteção contra CSRF dessas duas rotas é esse `state`, e não o `JSONOnly`.
- **Tokens de integração** criptografados com AES-GCM (`INTEGRATION_ENCRYPTION_KEY`) e nunca devolvidos pela API. O `metadata` de uma integração fica em claro e volta nas respostas, então não é lugar de segredo: cada tipo só guarda nele os campos que declara. O que vem de quem usa e entra numa URL da plataforma (o repositório, o quadro, o número da issue, o cartão) é conferido antes.

## Testes

Os testes usam um PostgreSQL de verdade. O `docker compose` já cria o banco `working_time_tracker_test`.

```bash
TEST_DATABASE_URL=postgres://wtt:wtt@localhost:5432/working_time_tracker_test?sslmode=disable \
  go test -p 1 ./...
```

Cada pacote de teste apaga o schema desse banco e aplica as migrações de novo antes de rodar, então ele não precisa de preparo e os testes sempre veem o que os arquivos de migração produzem hoje. Por segurança, isso só acontece num banco cujo nome termina em `_test`.

O `-p 1` é necessário porque todos os pacotes recriam e usam o mesmo banco. As chamadas ao GitHub, ao GitLab e ao Trello nos testes vão para servidores fake (`httptest`, o do GitLab em `testutil/platforms.go` e, com estado, o do GitHub em `testutil/github.go` e o do Trello em `testutil/trello.go`), então a suíte não depende de rede, e o transporte HTTP padrão dos testes recusa qualquer endereço que não seja da própria máquina: um teste que apontasse para o GitHub de verdade falha em vez de escrever nas issues de alguém.

Cobertura:
- **Domínios:** services e handlers.
- **Migrações:** o banco que elas produzem bate com o `ent/schema`, regras de FK, índice parcial, recusa de banco sem histórico e checksum dos arquivos.
- **Router real:** tabela de rotas, autenticação, permissões e isolamento entre organizações.
- **Páginas:** toda página renderiza para admin e membro, redireciona sem sessão e dá 404 entre organizações; cada uma tem o modal uma única vez e carrega os ícones com SRI. As abas só de admins (as de gestão da organização e a Gestão do projeto) dão 404 ao membro e somem do menu dele.
- **Sincronização das issues:** a regra de conflito em tabelas (`merge_test.go`), a rodada contra o GitHub fake com banco (importar, fechar, edição nos dois sentidos, adoção, issue descartada, sumida, só leitura, descarte silencioso, responsável sem e-mail, rodada interrompida, limite de requisições, uma rodada por vez, rotina de fundo) e a API de ponta a ponta, incluindo o gancho (editar, tirar o responsável, bater o ponto). Uma segunda rodada sem diferença faz **zero** escritas no GitHub.
- **Visão geral:** os totais com valores exatos (o arredondamento por sessão, a sessão aberta, as janelas de 7 e 30 dias, quem saiu do projeto, projeto interno e projeto vazio) e a idade do projeto em dias, semanas e meses.
- **Alpine vendorizado:** o hash confere com o pacote oficial.

## Estrutura do projeto

```
cmd/
  main.go                 # servidor: config, banco, migrações, server.Build e a sincronização de fundo
  seed/main.go            # dados de demonstração
  fakegithub/main.go      # o GitHub falso dos testes como servidor de desenvolvimento, com painel em /_fake
  faketrello/main.go      # o Trello falso dos testes como servidor de desenvolvimento (porta 8092), com painel em /_fake
  migrate/                # gera, confere e aplica as migrações do banco
ent/
  schema/                 # schema do banco (Ent); o resto de ent/ é gerado: go generate ./ent
internal/
  adapter/                # clientes do GitHub, do GitLab e do Trello, o que cada tipo pede, e a criptografia do token
  config/                 # variáveis de ambiente
  database/               # conexão, arquivos de migração (migrations/) e quem os aplica
  domain/
    auth/                 # signup, login, sessões, convites, middlewares e acesso por organização
    organization/  customer/  person/  project/  team/  allocation/  collaborator/  task/  work_session/  integration/
                          # cada domínio com model, store (Ent), service e handler
    issuesync/            # sincronização das issues do GitHub com as tarefas: a regra de conflito (merge.go), a rodada, o gancho e a rotina de fundo
    overview/             # visão geral do projeto e da organização: só lê os outros domínios, sem tabela nem store
  page/                   # páginas HTML
  routes/                 # rotas da API (routes.go) e das páginas (pages.go)
  server/                 # monta o servidor completo e a sincronização das issues (Build); usado pelo main e pelos testes
  template/               # renderer dos templates
  country/                # o cadastro dos países (BR, US): documento fiscal, código postal, estados, moeda e fuso de cada um
  validate/               # validações de formato usadas por mais de um domínio (CNPJ, EIN, CEP, ZIP, links)
testutil/                 # conexão, schema e limpeza do banco de teste, e as plataformas fake (o GitHub e o Trello, com estado)
web/                      # templates e arquivos estáticos (embutidos no binário)
docker-compose.yml        # PostgreSQL de dev e de testes
_docs/                    # proposta, design e plano das sprints
_test/                    # referência da API e coleção do Insomnia
```

## Modelo de dados

```
Organization                          nome, perfil (resumo, contato, dados jurídicos) e como trabalha (regime, fuso, moeda)
Organization (1) ── (N) Customer      cliente: nome, país, documento fiscal (CNPJ, EIN ou texto livre) e contato
Customer  (0..1) ── (N) Project       projeto interno fica sem cliente; o projeto guarda o valor cobrado por hora
Organization (1) ── (N) Project       sprint, daily e weekly são do projeto
Organization (1) ── (N) Person        email único no sistema, senha (bcrypt, opcional), usuário do Clerk (opcional, único), papel admin|member, jornada semanal
Organization (1) ── (N) Invite        token (hash), email opcional, papel, expira em 7 dias, uso único
Person       (1) ── (N) Session       token (hash), expira em 7 dias
Project      (1) ── (N) Team ── (N) Person   via TeamMembership
Project      (1) ── (N) Allocation ── (1) Person   valor pago por hora, um por pessoa em cada projeto
Project      (1) ── (N) WorkSession   a sessão é do projeto e guarda o valor pago e o cobrado do clock-in
WorkSession  (1) ── (N) WorkSessionTask ── (1) Task   as tarefas da sessão, cada uma com o intervalo em que esteve nela
Project      (1) ── (N) Integration   token criptografado (credentials) e metadata em claro
Task         (1) ── (N) IssueSync     o vínculo da tarefa com um item externo (issue, cartão), no máximo um por integração: guarda a URL e o snapshot do último acordo; task_id nulo é o item descartado
Integration  (1) ── (N) IssueSync     o item da plataforma, único por integração e chave (número da issue, link curto do cartão); estado open, closed, gone ou pending (ligado à mão, ainda não adotado)
```

Uma pessoa entra num projeto pelo **valor por hora** (`Allocation`), que é obrigatório e libera o ponto: colaborador do projeto é quem tem valor nele, e não há outra tabela para isso. Só depois ela pode entrar num **time** (`TeamMembership`), que permite ser responsável por tarefas; a API recusa pôr num time quem não tem valor no projeto. Tirar alguém do projeto apaga o valor e os times de uma vez e mantém as tarefas e as sessões da pessoa. Não há como apagar só o valor.

Num banco de antes dessa regra pode haver alguém num time sem valor. Essa pessoa continua aparecendo na aba Colaboradores, com um aviso, até um admin definir o valor ou tirá-la do projeto.

Uma sessão pertence a um projeto e começa por uma tarefa dele, mas pode ganhar outras, antes ou depois de encerrada; a mesma tarefa pode entrar de novo, desde que não se sobreponha ao intervalo anterior. O tempo vale para a sessão e para cada tarefa dela: tarefas em paralelo contam o tempo cheio cada uma, e a pessoa, o projeto e os valores contam a sessão uma vez só. Excluir uma tarefa a tira das sessões, mas as sessões e as horas ficam. Excluir um projeto apaga os times, os valores, as tarefas, as sessões e as integrações dele. Excluir um cliente só é permitido quando nenhum projeto aponta para ele. Excluir uma organização só é permitido sem projetos, e apaga as pessoas, os clientes e os convites. O índice único parcial `one_active_session` em `work_sessions (person_id) WHERE end_at IS NULL` garante uma sessão aberta por pessoa.

## Migrações do banco

O schema do banco só muda por arquivos SQL versionados em `internal/database/migrations/`. O servidor e o seed aplicam os pendentes ao iniciar, em ordem, cada arquivo numa transação, e registram o que já rodou na tabela `goose_db_version`. Nada é calculado na hora: o que não está num arquivo commitado não roda.

O `ent/schema` continua sendo a fonte do modelo, e os arquivos são gerados a partir dele:

```bash
# 1. Edite ent/schema e regenere o código
go generate ./ent

# 2. Gere a migração com a diferença
go run ./cmd/migrate new add_invoice_number

# 3. Revise o arquivo criado. Se editar à mão, recalcule o checksum
go run ./cmd/migrate checksum

# 4. Rode os testes e commite o ent/schema, o código gerado e a migração juntos
```

| Comando | O que faz |
|---|---|
| `go run ./cmd/migrate new <nome>` | Gera um arquivo com a diferença entre o que as migrações produzem e o `ent/schema`. Cria um banco temporário no servidor do `DATABASE_URL`, aplica nele as migrações **do mesmo jeito que o servidor** (goose, cada arquivo numa transação) e apaga o banco ao terminar, sem tocar no seu |
| `go run ./cmd/migrate checksum` | Recalcula o `atlas.sum` depois de uma edição manual |
| `go run ./cmd/migrate status` | Lista as migrações e quais já foram aplicadas no banco do `DATABASE_URL` |
| `go run ./cmd/migrate up` | Aplica as pendentes sem subir o servidor |

**Revise sempre o arquivo gerado.** O gerador compara schemas e não sabe o que há nas tabelas. Ele avisa quando a mudança mexe em dados existentes, mas quem corrige é você:

- **Rename de campo** sai como `DROP COLUMN` + `ADD COLUMN`, o que apagaria os dados. Troque por `ALTER TABLE ... RENAME COLUMN ...`.
- **Coluna `NOT NULL` nova sem default** falha numa tabela com linhas. Crie nullable, preencha com um `UPDATE` e só então `SET NOT NULL`, no mesmo arquivo.
- **Dado que muda de lugar** (uma coluna que sai de uma tabela e vai para outra) precisa de um `UPDATE` escrito à mão antes do `DROP`.
- **Entidade removida** do `ent/schema` não gera `DROP TABLE`. Se a tabela deve sair, escreva o `DROP TABLE`.

Regras:

- As migrações andam **só para frente**. Para desfazer uma mudança, escreva uma migração nova.
- **Não edite uma migração que já está na `main`.** Os bancos que já a aplicaram não rodam o arquivo de novo.
- Ao trazer a `main` para uma branch que tem migração nova, a sua precisa ficar depois das que chegaram. Se o timestamp dela for menor, apague o arquivo, rode `checksum` e gere de novo.
- Um comando com `;` no meio (função, bloco `DO`) fica entre `-- +goose StatementBegin` e `-- +goose StatementEnd`.
- Índice ou constraint criado à mão numa migração também precisa estar declarado no `ent/schema`. O teste `TestMigrate_NoSchemaDrift` falha quando os dois divergem.

### Banco criado antes das migrações versionadas

Um banco criado pelo auto migrate antigo tem as tabelas, mas não tem histórico. O servidor se recusa a iniciar nele (`database has tables but no migration history`), em vez de adivinhar em que estado ele está. O mesmo vale para um banco com uma migração que o binário não conhece. Recrie o banco e rode o seed:

```bash
docker compose exec db psql -U wtt -d postgres \
  -c 'DROP DATABASE working_time_tracker' \
  -c 'CREATE DATABASE working_time_tracker OWNER wtt'
go run ./cmd/seed
```

Isso apaga os dados desse banco. Para mantê-los, guarde-os antes com `pg_dump --data-only`, recrie o banco, rode `go run ./cmd/migrate up` no lugar do seed e restaure o dump. Só funciona se o banco antigo estava com o schema da versão que introduziu as migrações.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go + Echo v5 |
| Banco | PostgreSQL + [Ent](https://entgo.io), com migrações versionadas aplicadas pelo [goose](https://github.com/pressly/goose) |
| Frontend | HTML renderizado no servidor (`html/template`) + Alpine.js vendorizado, sem build; ícones do Font Awesome Free pelo cdnjs |
| Entrega | Binário único com `embed.FS` |

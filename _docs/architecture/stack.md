# Stack

As tecnologias do sistema, com as versões de hoje e o motivo de cada escolha. As decisões completas, com as
alternativas que ficaram de fora, estão em [decisions.md](decisions.md).

## Servidor

| Peça | Versão | Para quê | Por quê |
|---|---|---|---|
| Go | 1.26.4 (`go.mod`) | A linguagem | Um binário compilado, sem runtime para instalar |
| Echo | v5.3.0 | Roteador HTTP, middlewares, arquivos estáticos e templates | Leve e com tudo o que um servidor só de API e HTML precisa |
| Ent | v0.14.6 | O modelo (`ent/schema`) e o cliente tipado do banco | O modelo fica em Go e o código de acesso é gerado; nada de SQL repetido à mão |
| Atlas | v1.2.3 | Só no `cmd/migrate new`: compara o schema do Ent com o banco e escreve a diferença | É o motor que o Ent usa para comparar schemas |
| goose | v3.28.0 | Aplica as migrações SQL ao subir, uma transação por arquivo, com trava no banco | Um arquivo por mudança, sem o estado "sujo" de outras ferramentas |
| pgx | v5.10.0 | O driver do PostgreSQL | O driver padrão de fato para Go |
| go-i18n | v2.6.1 | Lê os catálogos YAML, plural e placeholders | Regras de plural do CLDR sem escrever nada |
| golang.org/x/text | v0.42.0 | Escolher o idioma pelo `Accept-Language` | Junto do go-i18n |
| golang.org/x/crypto | v0.57.0 | bcrypt das senhas | |
| clerk-sdk-go | v2.7.0 | Conferir o token de sessão do Clerk e ler o usuário | O SDK oficial; o resto do sistema não sabe que o Clerk existe |
| godotenv | v1.5.1 | Lê o `.env` ao subir | |

## Banco

**PostgreSQL**, 14 ou mais novo (o `docker-compose.yml` sobe a imagem `postgres:16-alpine`, com o banco de
desenvolvimento e o de testes). O sistema depende dele para os horários, os índices únicos parciais (uma sessão
aberta por pessoa, um dono por organização) e as colunas JSON das integrações. O schema muda só por migrações
versionadas: ver [workflow.md](../development/workflow.md).

## Navegador

| Peça | Versão | Como chega | Para quê |
|---|---|---|---|
| Alpine.js | 3.14.8 | Vendorizado em `web/static/alpine.min.js`, com o hash conferido em `web/embed_test.go` | A interatividade das páginas, sem etapa de build |
| marked | 18.1.0 | Vendorizado, com hash | Renderiza o Markdown da descrição da tarefa |
| DOMPurify | 3.4.16 | Vendorizado, com hash | Limpa o HTML que o marked gera. É a única defesa contra XSS na descrição |
| Font Awesome Free | 7.3.1 | cdnjs, com SRI (`web/templates/layouts/base.gohtml`) | Os ícones. Sem acesso à CDN a tela funciona, só sem os ícones |
| clerk-js e `@clerk/ui` | `@clerk/clerk-js@6`, `@clerk/ui@1` | Do Frontend API do app no Clerk, só no login, no cadastro, no convite e no retorno do Clerk | O botão do Google. As páginas internas não o carregam |
| Textos do Clerk em português | `@clerk/localizations` 4.22.1 | Vendorizado em `web/static/clerk-pt-BR.js` (licença MIT) | O Clerk só fala inglês por conta própria |

O CSS (`app.css`) e o JavaScript (`app.js` e `pages/*.js`) são escritos à mão e servidos como estão: não há
framework de CSS, bundler nem TypeScript. Como atualizar as bibliotecas vendorizadas e os ícones está em
[frontend/overview.md](../frontend/overview.md).

## Entrega

Um binário só (`go build -o wtt ./cmd`), com os templates, os estáticos, os catálogos de idioma e as migrações
embutidos. Ao subir, ele aplica as migrações que faltam. Não há etapa de build do front-end.

## Por que assim

- **Go + Echo + Ent + PostgreSQL**: um serviço pequeno, rápido de mexer e com o modelo em um lugar só. SQLite foi
  descartado pelas garantias fracas de concorrência; um SPA (React, Vue) foi descartado porque traria um build de
  JavaScript e um segundo artefato para entregar.
- **HTML no servidor + Alpine.js**: o servidor já sabe quem está logado e entrega a casca da página; o Alpine dá
  conta do cronômetro, dos formulários em etapas e dos modais. O HTMX também servia; o Alpine ganhou pelo estado no
  navegador (o cronômetro da sessão, os rascunhos dos modais).
- **Migrações versionadas**: o banco guarda dinheiro (os valores por hora), e a migração automática do Ent já tinha
  apagado dados num rename de coluna. Com arquivos, toda mudança destrutiva é escrita, revisada e commitada.

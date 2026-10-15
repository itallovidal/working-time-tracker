# GitLab

O GitLab existe no código mas não se oferece: a tela o mostra como "Em breve", e a API recusa criar uma integração nova
dele. Este arquivo diz o que ele faz hoje e como reativá-lo. O modelo comum está em [overview.md](overview.md).

## O que existe

| | |
|---|---|
| Acesso | Token colado, com escopo `read_api`, mandado no cabeçalho `PRIVATE-TOKEN` |
| `metadata` | `project_url`: `grupo/projeto`, com subgrupos, ou o endereço do projeto no gitlab.com (um endereço de página, como `…/-/issues`, também serve) |
| Servidor | Só o gitlab.com (`https://gitlab.com/api/v4`). Não há variável para um GitLab próprio |
| Item | O número da issue (`42` ou `#42`) |
| Sincronização | Não tem: o tipo não implementa `IssueSyncer` |

O que ele faz:

- **Valida a conexão** lendo o projeto. Os erros são `integration.invalid_token`, `integration.gitlab_forbidden` e
  `integration.gitlab_project_not_found`.
- **Liga uma tarefa a uma issue à mão** (`POST /api/tasks/:taskId/link-external-item`).
- **Busca na hora os detalhes da issue** (título, estado e endereço), por
  `GET /api/tasks/:taskId/external-details`.

## "Em breve"

O `Descriptor` do GitLab tem `ComingSoon: true` (`internal/adapter/gitlab.go`):

- o `POST /api/projects/:projectId/integrations` com `"type": "gitlab"` responde `400` `integration.type_coming_soon`;
- a tela mostra o tipo desabilitado, com o selo "Em breve";
- na etapa Integrações da Nova tarefa, o GitLab aparece desativado;
- as integrações do GitLab que já existem seguem valendo e se editam.

**Para oferecê-lo de novo, basta tirar a flag**, sem outra mudança. A conferência fica no handler, e não no service,
justamente para isso.

O seed tem integrações do GitLab de demonstração, sem credencial. Nos testes, as chamadas ao GitLab vão para um
servidor fake (`testutil/platforms.go`).

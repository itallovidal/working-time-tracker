# Rotas da API — Working Time Tracker

**Base URL:** `http://localhost:8080`

---

## Healthcheck

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/healthcheck` | Verifica se o servidor está rodando |
| GET | `/healthcheck/hello` | Hello world de teste |

---

## Organization

### Criar organização
```http
POST /api/orgs
Content-Type: application/json

{
  "name": "Minha Empresa"
}
```

### Listar organizações
```http
GET /api/orgs
```

### Obter organização
```http
GET /api/orgs/:orgId
```

### Atualizar organização
```http
PATCH /api/orgs/:orgId
Content-Type: application/json

{
  "name": "Empresa Atualizada"
}
```

### Deletar organização
```http
DELETE /api/orgs/:orgId
```

---

## Person

### Criar pessoa
```http
POST /api/orgs/:orgId/persons
Content-Type: application/json

{
  "name": "João Silva",
  "email": "joao@empresa.com"
}
```

### Listar pessoas da organização
```http
GET /api/orgs/:orgId/persons
```

### Obter pessoa
```http
GET /api/persons/:personId
```

### Atualizar pessoa
```http
PATCH /api/persons/:personId
Content-Type: application/json

{
  "name": "João Silva Atualizado",
  "email": "joao.novo@empresa.com"
}
```

---

## Project

### Criar projeto
```http
POST /api/orgs/:orgId/projects
Content-Type: application/json

{
  "name": "Meu Projeto",
  "description": "Descrição do projeto",
  "sprint_duration_days": 14,
  "daily_time": "09:00",
  "weekly_sync_day": "monday"
}
```

### Listar projetos da organização
```http
GET /api/orgs/:orgId/projects
```

### Obter projeto
```http
GET /api/projects/:projectId
```

### Atualizar projeto
```http
PATCH /api/projects/:projectId
Content-Type: application/json

{
  "name": "Projeto Atualizado",
  "description": "Nova descrição",
  "sprint_duration_days": 21
}
```

### Deletar projeto
```http
DELETE /api/projects/:projectId
```

---

## Team

### Criar time
```http
POST /api/projects/:projectId/teams
Content-Type: application/json

{
  "name": "Time Alpha"
}
```

### Listar times do projeto
```http
GET /api/projects/:projectId/teams
```

### Obter time
```http
GET /api/teams/:teamId
```

### Atualizar time
```http
PATCH /api/teams/:teamId
Content-Type: application/json

{
  "name": "Time Alpha Atualizado"
}
```

### Deletar time
```http
DELETE /api/teams/:teamId
```

---

## Team Membership

### Adicionar membro ao time
```http
POST /api/teams/:teamId/members
Content-Type: application/json

{
  "person_id": "uuid-da-pessoa"
}
```

### Listar membros do time
```http
GET /api/teams/:teamId/members
```

### Remover membro do time
```http
DELETE /api/teams/:teamId/members
Content-Type: application/json

{
  "person_id": "uuid-da-pessoa"
}
```

---

## Variáveis de ambiente (Insomnia)

| Variável | Valor padrão | Descrição |
|----------|-------------|-----------|
| `base_url` | `http://localhost:8080` | URL base do servidor |
| `org_id` | | UUID retornado ao criar org |
| `person_id` | | UUID retornado ao criar person |
| `project_id` | | UUID retornado ao criar projeto |
| `team_id` | | UUID retornado ao criar time |

## Fluxo de teste sugerido

1. **Healthcheck** → `GET /healthcheck`
2. **Criar org** → `POST /api/orgs` → copiar o `id` retornado
3. **Listar orgs** → `GET /api/orgs`
4. **Criar pessoa** → `POST /api/orgs/:orgId/persons`
5. **Listar pessoas** → `GET /api/orgs/:orgId/persons`
6. **Obter pessoa** → `GET /api/persons/:personId`
7. **Atualizar pessoa** → `PATCH /api/persons/:personId`
8. **Criar projeto** → `POST /api/orgs/:orgId/projects` → copiar o `id` retornado
9. **Listar projetos** → `GET /api/orgs/:orgId/projects`
10. **Obter projeto** → `GET /api/projects/:projectId`
11. **Atualizar projeto** → `PATCH /api/projects/:projectId`
12. **Criar time** → `POST /api/projects/:projectId/teams` → copiar o `id` retornado
13. **Listar times** → `GET /api/projects/:projectId/teams`
14. **Obter time** → `GET /api/teams/:teamId`
15. **Atualizar time** → `PATCH /api/teams/:teamId`
16. **Adicionar membro ao time** → `POST /api/teams/:teamId/members`
17. **Listar membros do time** → `GET /api/teams/:teamId/members`
18. **Remover membro do time** → `DELETE /api/teams/:teamId/members`
19. **Deletar time** → `DELETE /api/teams/:teamId`
20. **Deletar projeto** → `DELETE /api/projects/:projectId`
21. **Atualizar org** → `PATCH /api/orgs/:orgId`
22. **Deletar org** → `DELETE /api/orgs/:orgId` (vai falhar se tiver projetos)

# Rotas da API — Working Time Tracker

**Base URL:** `http://localhost:8080`

---

## Healthcheck

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/healthcheck/` | Verifica se o servidor está rodando |
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

## Variáveis de ambiente (Insomnia)

| Variável | Valor padrão | Descrição |
|----------|-------------|-----------|
| `base_url` | `http://localhost:8080` | URL base do servidor |
| `org_id` | | UUID retornado ao criar org |
| `person_id` | | UUID retornado ao criar person |

## Fluxo de teste sugerido

1. **Healthcheck** → `GET /healthcheck/`
2. **Criar org** → `POST /api/orgs` → copiar o `id` retornado
3. **Listar orgs** → `GET /api/orgs`
4. **Criar pessoa** → `POST /api/orgs/:orgId/persons`
5. **Listar pessoas** → `GET /api/orgs/:orgId/persons`
6. **Obter pessoa** → `GET /api/persons/:personId`
7. **Atualizar pessoa** → `PATCH /api/persons/:personId`
8. **Atualizar org** → `PATCH /api/orgs/:orgId`
9. **Deletar org** → `DELETE /api/orgs/:orgId` (vai falhar se tiver projetos)

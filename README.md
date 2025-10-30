# Documentação de Uso da API Pipedrive Proxy

Este serviço atua como um *proxy* otimizado para a API oficial do Pipedrive, oferecendo recursos de **resiliência** (Rate Limit e Retry Queue), **observabilidade** e **manipulação de dados** (filtragem e seleção de campos) no lado do servidor.

---

## Estrutura da Resposta (Envelope Padronizado)

Todas as respostas (sucesso ou falha) são encapsuladas em um objeto JSON padrão, permitindo que o cliente sempre saiba onde procurar os dados, erros e metadados de requisição.

| Campo | Tipo | Descrição |
| :--- | :--- | :--- |
| `success` | `boolean` | Indica se a requisição do serviço foi bem-sucedida (`true`) ou se ocorreu um erro (`false`). |
| `data` | `array` ou `object` | O payload principal da resposta (e.g., lista de pipelines, dados de organização). Omitido em caso de erro. |
| `error` | `string` ou `object` | Mensagem ou estrutura detalhada do erro. |
| `metadata` | `array` | Contém um *slice* de objetos `MetaItem` com informações detalhadas da requisição. |

### Estrutura de Metadados (`metadata[0]`)

| Campo | Tipo | Descrição |
| :--- | :--- | :--- |
| `request_id` | `string` | ID da requisição, útil para rastreamento nos logs do serviço. |
| `duration_ms` | `integer` | Tempo total de processamento da requisição em milissegundos. |
| `url` | `string` | URL da requisição feita ao servidor *upstream* (Pipedrive). |
| `status` | `integer` | Código HTTP retornado pelo servidor *upstream*. |
| `rate_limit` | `object` | Detalhes sobre o Rate Limit (`limit`, `remaining`, `reset_at`). |
| `extra.total_results` | `integer` | Quantidade de resultados retornados após filtros locais. |

---

## 1. Endpoint: Pipelines

### `GET /pipedrive/pipelines`

Retorna uma lista de todos os pipelines disponíveis no Pipedrive.

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| `GET` | `/pipedrive/pipelines` | Lista pipelines com suporte a filtros e seleção de campos. |

#### Exemplo

```bash
GET /pipedrive/pipelines?fields=id,name
```

```json
{
  "success": true,
  "data": [
    { "id": 1, "name": "Vendas" },
    { "id": 2, "name": "Renovação" }
  ],
  "metadata": [...]
}
```

---

## 2. Endpoint: Organizações

A rota `/pipedrive/organizations` possui **quatro métodos** disponíveis, permitindo gerenciamento completo de Organizações no Pipedrive.

---

### 🟢 `GET /pipedrive/organizations`

#### Modos de Operação

- **Sem `id`:** Listagem paginada de organizações (`/organizations`).
- **Com `id`:** Busca detalhada em massa (`/organizations/{id}`), com suporte a múltiplos IDs.

#### Exemplos

**Listagem completa:**
```bash
GET /pipedrive/organizations?limit=100
```

**Busca por IDs:**
```bash
GET /pipedrive/organizations?id=123,456&fields=name,owner_id
```

#### Retorno
```json
{
  "success": true,
  "data": [
    { "id": 123, "name": "Setup Tecnologia", "owner_id": 42 },
    { "id": 456, "name": "DigitalUp", "owner_id": 42 }
  ],
  "metadata": [...]
}
```

---

### 🟡 `POST /pipedrive/organizations`

Cria uma ou mais organizações no Pipedrive.

#### Corpo (JSON)

| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `name` | `string` | ✅ | Nome da organização. |
| `owner_id` | `integer` | ❌ | ID do usuário responsável. |
| `visible_to` | `integer` | ❌ | Visibilidade (0 = dono, 1 = empresa, etc.). |
| `address` | `string` | ❌ | Endereço da organização. |
| `label` | `string` | ❌ | Rótulo (Label) no Pipedrive. |
| `custom_fields` | `object` | ❌ | Campos personalizados (`c9_XXX`). |

#### Exemplo - Criação única
```bash
POST /pipedrive/organizations
```

```json
{
  "name": "Setup Tecnologia LTDA"
}
```

#### Exemplo - Criação em lote
```json
[
  { "name": "Empresa Alpha", "owner_id": 101 },
  { "name": "Empresa Beta", "visible_to": 1 }
]
```

#### Retorno
```json
{
  "success": true,
  "data": {
    "status": "partial_failure",
    "results": {
      "0": { "id": 111, "name": "Empresa Alpha" },
      "1": { "error": "upstream returned 400", "status": 400 }
    }
  },
  "metadata": [...]
}
```

---

### 🟠 `PUT /pipedrive/organizations`

Atualiza uma ou várias organizações simultaneamente.

#### Modos suportados
| Tipo | Descrição |
| :--- | :--- |
| `replace` | Substitui campos existentes. |
| `add` | Concatena novos valores aos existentes. |
| `remove` | Limpa o valor de um ou mais campos. |

#### Exemplo
```json
[
  {
    "type": "replace",
    "id": 123,
    "fields": {
      "name": "Nova Razão LTDA",
      "website": "https://nova.com"
    }
  }
]
```

#### Retorno
```json
{
  "success": true,
  "data": {
    "status": "success",
    "results": {
      "123": { "success": true, "fields_altered": ["name", "website"] }
    }
  },
  "metadata": [...]
}
```

---

### 🔴 `DELETE /pipedrive/organizations`

Remove uma ou várias organizações por ID.

#### Corpo (JSON)
Aceita:
- Um único objeto:
  ```json
  { "id": 1234 }
  ```
- Ou uma lista:
  ```json
  [111, 222, 333]
  ```

#### Retorno
```json
{
  "success": true,
  "data": {
    "status": "partial_failure",
    "summary": { "requested": 3, "deleted": 2, "failed": 1 },
    "results": {
      "111": { "success": true, "deleted": 111 },
      "222": { "error": "upstream returned 404", "status": 404 },
      "333": { "success": true, "deleted": 333 }
    }
  },
  "metadata": [...]
}
```

---

## 3. Tratamento de Erros e Resiliência

| Situação | Comportamento |
| :--- | :--- |
| **429 Too Many Requests** | Retorna `HTTP 429`, com `metadata.rate_limit.reset_at` informando quando tentar novamente. |
| **Falha em requisições individuais (bulk)** | Marca item como erro sem interromper o restante. |
| **Erros genéricos (400–500)** | Refletidos diretamente no campo `status` dentro de cada resultado. |
| **Campos inválidos** | Erros descritivos retornados diretamente no corpo da resposta (`error.message`). |

---

## 4. Sumário dos Endpoints

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| `GET` | `/pipedrive/organizations` | Lista ou busca organizações. |
| `POST` | `/pipedrive/organizations` | Cria uma ou mais organizações. |
| `PUT` | `/pipedrive/organizations` | Atualiza organizações em massa (`replace`, `add`, `remove`). |
| `DELETE` | `/pipedrive/organizations` | Remove uma ou mais organizações por ID. |

---

## 5. Observações

- Todos os endpoints suportam logs e metadados unificados via `MetaItem`.
- O campo `rate_limit` sempre indica o estado da cota do token de API.
- Campos customizados do Pipedrive (`c9_xxx`) são suportados em todas as operações.
- O serviço lida automaticamente com `Retry-After` e fila de reprocessamento em caso de `429`.

---

# URL Shortener

Um serviço de encurtamento de URLs pronto para produção (como o bit.ly), construído em **Go**, com arquitetura em camadas, persistência em SQLite, logging estruturado e cobertura completa de testes.

## Stack

|O GitHub Copilot foi empregado como suporte em atividades mecânicas de codificação, como autocomplete e sugestões de refatoração. Todo o desenvolvimento da lógica de negócio, incluindo a implementação de endpoints e a estruturação do código, foi realizado manualmente por mim, garantindo total controle sobre a qualidade e arquitetura da solução.

| Camada | Tecnologia |
|---|---|
| Linguagem | Go 1.24+ |
| Roteador HTTP | [chi](https://github.com/go-chi/chi) |
| Banco de Dados | SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (Go puro, sem CGO) |
| Testes | `testing` + [testify](https://github.com/stretchr/testify) |
| Geração de ID | Base62 (a-z, A-Z, 0-9), 7 caracteres |
| Logging | `log/slog` (stdlib, JSON estruturado em produção) |
| Container | Build multi-estágio com Docker + docker-compose |

## Executando o Projeto

### Via Makefile (recomendado — testes obrigatórios)

```bash
make run        # roda os testes e sobe o servidor
make build      # roda os testes e compila o binário
make docker-build # gera a imagem Docker (testes rodam dentro do build)
```

### Go direto (sem validação de testes)

```bash
# Instalar dependências
go mod download

# Rodar o servidor (porta padrão 8080)
go run ./cmd/api

# Ou com configuração personalizada
PORT=9090 BASE_URL=http://meuhost:9090 API_KEY=segredo go run ./cmd/api
```

### Docker Compose

```bash
docker-compose up --build
```

> O `docker-compose up --build` aciona o Dockerfile, que executa todos os testes antes de compilar. O container só sobe se os testes passarem.

A API estará disponível em `http://localhost:8080`.

## Executando os Testes

Os testes estão organizados em duas categorias dentro de `tests/`:

```
tests/
├── unit/         → testes unitários por camada (handler, service, repository)
└── e2e/          → testes end-to-end com servidor HTTP real via httptest
```

```bash
# Todos os testes
make test
# ou: go test ./...

# Apenas unitários
make test-unit
# ou: go test ./tests/unit/...

# Apenas e2e
make test-e2e
# ou: go test ./tests/e2e/...

# Com saída detalhada
go test ./... -v
```

## Variáveis de Ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `PORT` | `8080` | Porta do servidor HTTP |
| `BASE_URL` | `http://localhost:8080` | URL base usada para construir as URLs encurtadas |
| `API_KEY` | `default-api-key` | Chave de API exigida no cabeçalho `X-API-Key` para operações de escrita |
| `DB_PATH` | `./data/urls.db` | Caminho para o arquivo do banco de dados SQLite |
| `LOG_LEVEL` | `info` | Nível de log (`debug`, `info`, `warn`, `error`) |

## Referência da API

### Autenticação

Os endpoints protegidos exigem o cabeçalho `X-API-Key`:

```
X-API-Key: default-api-key
```

### Criar URL Encurtada

```
POST /v1/urls
```

**Requisição:**
```json
{
  "originalUrl": "https://www.example.com/meu/caminho/longo",
  "expirationDate": "2025-12-31T23:59:59Z",
  "customAlias": "meu-alias"
}
```

- `originalUrl` — **obrigatório**, deve ser uma URL http/https válida com domínio completo (ex: `https://exemplo.com`)
- `expirationDate` — opcional, timestamp no formato ISO 8601
- `customAlias` — ID curto customizado opcional (retorna 409 se já estiver em uso)

**Resposta `201 Created`:**
```json
{
  "id": "aB3xY7z",
  "shortUrl": "http://localhost:8080/aB3xY7z",
  "originalUrl": "https://www.example.com/meu/caminho/longo",
  "createdAt": "2024-03-01T10:00:00Z",
  "expirationDate": "2025-12-31T23:59:59Z"
}
```

**Exemplo com curl:**
```bash
curl -X POST http://localhost:8080/v1/urls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: default-api-key" \
  -d '{"originalUrl":"https://www.example.com/meu/caminho/longo"}'
```

---

### Redirecionamento

```
GET /{id}
```

- `301 Moved Permanently` → redireciona para `originalUrl`
- `404 Not Found` → ID curto não existe
- `410 Gone` → URL expirada

**Exemplo com curl:**
```bash
curl -L http://localhost:8080/aB3xY7z
```

---

### Buscar Detalhes da URL

```
GET /v1/urls/{id}
```

**Resposta `200 OK`:**
```json
{
  "id": "aB3xY7z",
  "shortUrl": "http://localhost:8080/aB3xY7z",
  "originalUrl": "https://www.example.com/meu/caminho/longo",
  "createdAt": "2024-03-01T10:00:00Z",
  "expirationDate": "2025-12-31T23:59:59Z",
  "clickCount": 42
}
```

**Exemplo com curl:**
```bash
curl http://localhost:8080/v1/urls/aB3xY7z \
  -H "X-API-Key: default-api-key"
```

---

### Listar URLs (paginado)

```
GET /v1/urls?page=1&size=10
```

**Resposta `200 OK`:**
```json
{
  "data": [...],
  "page": 1,
  "size": 10,
  "total": 50
}
```

**Exemplo com curl:**
```bash
curl "http://localhost:8080/v1/urls?page=1&size=10" \
  -H "X-API-Key: default-api-key"
```

---

### Atualizar URL (parcial)

```
PATCH /v1/urls/{id}
```

Apenas os campos enviados são alterados. Os demais permanecem intactos.

**Campos disponíveis:**
- `originalUrl` — nova URL de destino (validada; retorna 409 se já estiver cadastrada)
- `expirationDate` — nova data de expiração
- `clearExpiration` — `true` para remover a data de expiração existente

**Exemplos de body:**
```json
{ "originalUrl": "https://nova-url.com" }

{ "expirationDate": "2027-12-31T23:59:59Z" }

{ "clearExpiration": true }

{ "originalUrl": "https://nova-url.com", "clearExpiration": true }
```

**Resposta `200 OK`:** retorna a URL com os dados atualizados (mesmo formato do `GET /v1/urls/{id}`).

**Exemplo com curl:**
```bash
curl -X PATCH http://localhost:8080/v1/urls/aB3xY7z \
  -H "Content-Type: application/json" \
  -H "X-API-Key: default-api-key" \
  -d '{"originalUrl":"https://nova-url.com"}'
```

---

### Remover URL

```
DELETE /v1/urls/{id}
```

- `204 No Content` → removida com sucesso
- `404 Not Found` → ID não existe

**Exemplo com curl:**
```bash
curl -X DELETE http://localhost:8080/v1/urls/aB3xY7z \
  -H "X-API-Key: default-api-key"
```

---

### Formato de Resposta de Erro

Todos os erros seguem um envelope consistente:

```json
{
  "error": {
    "code": "URL_NOT_FOUND",
    "message": "The requested short URL does not exist"
  }
}
```

| Status | Código | Cenário |
|---|---|---|
| 400 | `INVALID_URL` | URL vazia, malformada, sem domínio completo, ou não usa http/https |
| 400 | `INVALID_REQUEST` | Corpo da requisição não é JSON válido |
| 401 | `UNAUTHORIZED` | `X-API-Key` ausente ou inválida |
| 404 | `URL_NOT_FOUND` | ID curto não encontrado |
| 409 | `ALIAS_CONFLICT` | Alias customizado já está em uso |
| 409 | `URL_ALREADY_EXISTS` | A URL original já foi cadastrada anteriormente |
| 410 | `URL_EXPIRED` | URL encurtada passou da data de expiração |
| 500 | `INTERNAL_SERVER_ERROR` | Erro inesperado no servidor |

## Decisões de Arquitetura

### Arquitetura em Camadas

O código é organizado em três camadas bem definidas, cada uma com responsabilidade única:

```
cmd/api/main.go          → Conecta tudo, inicia o servidor HTTP
internal/handler/        → Camada HTTP: parseia requisições, chama o service, escreve respostas
internal/service/        → Lógica de negócio: validação de URL, geração de ID, expiração, atualização, remoção
internal/repository/     → Camada de persistência: operações CRUD no SQLite
internal/model/          → Tipos de domínio e DTOs compartilhados
internal/middleware/      → Preocupações transversais: autenticação, logging, recuperação
```

Cada camada se comunica via **interfaces** (ex.: `URLRepository`, `URLService`), facilitando a troca de implementações e a escrita de testes unitários com mocks ou bancos em memória.

### Geração de IDs

Os IDs curtos são gerados usando o alfabeto Base62 (`a-z A-Z 0-9`), com 7 caracteres, resultando em 62⁷ ≈ 3,5 trilhões de combinações possíveis. Em caso de colisão (extremamente improvável), o serviço tenta até 10 vezes. Aliases customizados ignoram a geração aleatória.

### Validação de URLs

Além do esquema (`http`/`https`), o serviço valida a estrutura do host:

- Hosts de label único são rejeitados (ex: `https://semdominio`) — é necessário um FQDN
- O TLD deve conter apenas letras e ter no mínimo 2 caracteres (ex: `.com`, `.br`, `.io`)
- IPs válidos e `localhost` são aceitos normalmente
- Cadastrar a mesma URL duas vezes retorna `409 URL_ALREADY_EXISTS`

### Persistência

SQLite é usado como banco de dados embutido — sem processo externo ou conexão de rede. O driver `modernc.org/sqlite` é Go puro (sem CGO), facilitando a compilação cruzada e a containerização. A camada de repositório expõe uma interface para que o armazenamento subjacente possa ser substituído (ex.: por PostgreSQL) sem alterar as camadas de service ou handler.

### Autenticação

Uma chave de API estática simples (cabeçalho `X-API-Key`) protege todos os endpoints autenticados: criação, listagem, busca, atualização e remoção. O endpoint de redirecionamento (`GET /{id}`) é intencionalmente público — ele precisa funcionar em navegadores e ferramentas de linha de comando sem credenciais. A chave de API padrão é `default-api-key` e pode ser sobrescrita via variável de ambiente `API_KEY`.

### Logging Estruturado

`log/slog` (stdlib do Go 1.21+) oferece logging estruturado com campos contextuais. No terminal, o logger usa formato de texto legível; caso contrário (container/CI), usa JSON para facilitar a ingestão por agregadores de logs.

## Estrutura do Projeto

```
url-shortener/
├── cmd/api/main.go              # Ponto de entrada
├── internal/
│   ├── handler/                 # Handlers HTTP
│   │   └── url_handler.go
│   ├── service/                 # Lógica de negócio
│   │   └── url_service.go
│   ├── repository/              # Persistência SQLite
│   │   └── url_repository.go
│   ├── model/                   # Structs de domínio e DTOs
│   │   └── url.go
│   └── middleware/              # Autenticação, logging, recuperação
│       └── middleware.go
├── tests/
│   ├── unit/                    # Testes unitários por camada
│   │   ├── handler/
│   │   ├── service/
│   │   └── repository/
│   └── e2e/                     # Testes end-to-end via httptest
├── go.mod
├── go.sum
├── Makefile                     # Targets com testes obrigatórios
├── Dockerfile                   # Build multi-estágio (testa antes de compilar)
├── docker-compose.yml
└── README.md
```

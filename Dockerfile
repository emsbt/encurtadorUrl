# ---- Estágio de testes ----
# O build falha aqui se qualquer teste falhar
FROM golang:1.24-alpine AS tester

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go test ./...

# ---- Estágio de build ----
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Garante que os testes passaram antes de compilar
COPY --from=tester /app/go.sum ./go.sum

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /url-shortener ./cmd/api

# ---- Estágio de runtime ----
FROM alpine:3.20

# Instala certificados CA para requisições HTTPS de saída
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copia o binário do estágio de build
COPY --from=builder /url-shortener /app/url-shortener

# Cria o diretório de dados para o SQLite
RUN mkdir -p /app/data

# Expõe a porta padrão
EXPOSE 8080

# Executa como usuário não-root
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN chown -R appuser:appgroup /app
USER appuser

ENTRYPOINT ["/app/url-shortener"]

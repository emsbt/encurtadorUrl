.PHONY: test test-unit test-e2e build run docker-build

## Roda todos os testes (unitários + e2e)
test:
	go test ./...

## Roda apenas os testes unitários
test-unit:
	go test ./tests/unit/...

## Roda apenas os testes e2e
test-e2e:
	go test ./tests/e2e/...

## Compila o binário — falha se os testes não passarem
build: test
	go build -o url-shortener ./cmd/api

## Sobe o servidor — falha se os testes não passarem
run: test
	go run ./cmd/api

## Gera a imagem Docker — testes rodam dentro do build (ver Dockerfile)
docker-build:
	docker build -t url-shortener .

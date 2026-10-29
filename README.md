# Case Técnico — Desenvolvedor Go (Golang)

Contexto

Neste desafio você irá implementar um serviço capaz de inferir políticas declarativas representadas como grafos direcionados (DAGs).

As políticas são descritas utilizando Graphviz DOT language, onde:

- Nós representam atribuições de variáveis (resultados)
- Arestas representam condições lógicas
- A execução percorre o grafo avaliando condições
- Apenas um caminho é seguido por execução (determinístico)

O serviço recebe:

- Uma política (DOT string)
- Variáveis de entrada

E retorna as variáveis enriquecidas após a inferência.

---

Objetivo

Construir uma aplicação em Go que:

1. Exponha um endpoint HTTP "POST"
2. Receba:
   - Política em formato DOT
   - Payload de variáveis
3. Execute inferência
4. Retorne resultado enriquecido

Pode ser implementado como:

- AWS Lambda (preferencial)
- ou serviço HTTP containerizado

---

Endpoint

POST /infer

---

Payload

{
  "policy_dot": "digraph {...}",
  "input": {
    "age": 25,
    "score": 720
  }
}

---

Resposta

{
  "output": {
    "age": 25,
    "score": 720,
    "approved": true,
    "segment": "prime"
  }
}

O output deve conter:

- Variáveis de entrada
- Variáveis inferidas

---

Modelo DOT

Exemplo

digraph Policy {

  start [result=""]

  approved [
    result="approved=true,segment=prime"
  ]

  rejected [
    result="approved=false"
  ]

  review [
    result="approved=false,segment=manual"
  ]

  start -> approved [cond="age>=18 && score>700"]
  start -> review   [cond="age>=18 && score<=700"]
  start -> rejected [cond="age<18"]
}

---

Semântica

Nó inicial

- O nó chamado "start" é sempre o ponto de entrada

---

Atributos de Nó

Atributo| Significado
"result"| Atribuições aplicadas ao visitar o nó

Formato:

key=value,key2=value2

Exemplo:

approved=true,segment=prime

---

Atributos de Aresta

Atributo| Significado
"cond"| Expressão booleana

Operadores suportados:

== != > < >= <=
&& ||

Tipos esperados:

- string
- number
- boolean

Sem operadores aritméticos.

---

Regras de Execução

1. Iniciar em "start"
2. Avaliar arestas de saída
3. Seguir a primeira condição verdadeira
4. Aplicar "result" do nó visitado
5. Repetir até:
   - não existir nova aresta válida
   - ou atingir folha

---

Requisitos Não Funcionais

Performance

O serviço deve suportar:

- 50 RPS
- P90 < 30ms
- Desconsiderar cold start

Não é necessário ambiente produtivo, mas inclua:

- Benchmark
- ou teste de carga
- ou profiling documentado

---

Qualidade Esperada

- Código idiomático Go
- Testes unitários
- README explicando decisões
- Tratamento de erro adequado
- Logging básico

---

Bibliotecas Permitidas

Você não precisa implementar parser DOT manualmente.

Pode usar bibliotecas como:

- gographviz
- graphviz bindings
- ou equivalente

---

Exemplo Completo

Request

{
  "policy_dot": "digraph { start [result=\"\"]; ok [result=\"approved=true\"]; no [result=\"approved=false\"]; start -> ok [cond=\"age>=18\"]; start -> no [cond=\"age<18\"]; }",
  "input": {
    "age": 20
  }
}

Response

{
  "output": {
    "age": 20,
    "approved": true
  }
}

---

Entrega

- Repositório Git
- Instruções de execução
- Prazo sugerido: 3–5 dias

---

Avaliação

Critérios considerados:

- Design de solução
- Clareza e organização do código
- Modelagem do grafo
- Testabilidade
- Performance
- Comunicação técnica

Não buscamos perfeição — queremos entender como você pensa.


---

## Solução implementada — R0–R7

Estado atual:

- `POST /infer` interpreta políticas DOT em um único `digraph`, com nós e arestas individuais e atributos `cond`/`result`.
- Condições aceitam comparações, `&&`, `||`, parênteses, booleanos isolados e curto-circuito da esquerda para a direita.
- `==` e `!=` comparam valores apenas quando os tipos coincidem; `>`, `<`, `>=` e `<=` são somente numéricos.
- `result` aceita booleanos, números e strings simples ou entre aspas, incluindo vazia e vírgula. Os escapes suportados são `\"`, `\\`, `\n` e `\t`.
- O percurso começa em `start`, aplica atribuições antes de avaliar saídas, segue a primeira condição verdadeira na ordem do DOT e encerra em folha ou sem saída verdadeira.
- Ciclos, política sem `start`, `cond` vazio/ausente, identificadores inválidos, construções DOT fora do subconjunto e sintaxe inválida em trechos não percorridos são rejeitados.
- A requisição valida método, caminho, `Content-Type`, campos exigidos, chaves JSON duplicadas e valores de entrada básicos.
- Limites configuráveis protegem corpo, quantidade de nós/arestas, tamanho de condição e profundidade de expressão. Logs registram status, código e duração sem expor política ou variáveis.

### Pré-requisitos

- Go 1.26+
- Docker Engine e AWS SAM CLI — apenas para a emulação Lambda; os testes e o
  modo HTTP local dependem somente de Go

### Verificação

```bash
go test ./...        # ou: make test
go vet ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./build/infer ./cmd/infer
sam build
sam local start-api --warm-containers EAGER
```

### Execução local

O mesmo binário serve HTTP puro quando `HTTP_ADDR` está definido, sem passar
pelo SAM. As variáveis podem vir do ambiente ou de um arquivo `.env` na raiz
(ver `.env.example`; precedência: env > `.env` > defaults):

```bash
make run     # go run ./cmd/infer -http-addr=:8080
make debug   # idem, com logs em nível debug
```

### Exemplo

```bash
curl -X POST http://127.0.0.1:3000/infer \
  -H 'Content-Type: application/json' \
  -d '{"policy_dot":"digraph Policy { start [result=\"\"]; approved [result=\"approved=true\"]; rejected [result=\"approved=false\"]; start -> approved [cond=\"age>=18 && score>700\"]; start -> rejected [cond=\"age<18\"]; }","input":{"age":20,"score":720}}'
```

Resposta esperada:

```json
{"output":{"age":20,"approved":true,"score":720}}
```

Curto-circuito: `age<18 && score>700` não acessa `score` quando `age` é menor que 18; `age>=18 || score>700` não acessa `score` quando `age` é 18 ou mais. Se a variável for efetivamente acessada e estiver ausente, a execução retorna `inference_error`, sem `output` parcial.

### Rejeições

- JSON malformado, chaves duplicadas, campos extras, `policy_dot`/`input` ausentes ou tipos errados → `invalid_json`.
- `input` não objeto, chave fora da gramática, `null`, objeto ou lista como valor → `invalid_input`.
- DOT inválido ou política fora do subconjunto → `invalid_policy`.
- Variável ausente acessada ou tipos incompatíveis durante a avaliação → `inference_error`.
- Método diferente de `POST`, caminho diferente de `/infer` e corpo acima do limite usam `method_not_allowed`, `not_found` e `request_too_large`.

### Limites e observabilidade

Configuração por variáveis de ambiente, com os defaults também definidos em `template.yaml`:

| Variável | Default |
| --- | ---: |
| `MAX_BODY_BYTES` | `1048576` |
| `MAX_POLICY_NODES` | `1000` |
| `MAX_POLICY_EDGES` | `5000` |
| `MAX_CONDITION_BYTES` | `4096` |
| `MAX_EXPRESSION_DEPTH` | `64` |
| `LOG_LEVEL` | `info` |

`HTTP_ADDR` (sem default) liga o modo HTTP local; `-http-addr` e `-log-level` sobrescrevem as variáveis equivalentes.

Corpo acima do limite retorna HTTP 413 com `request_too_large`. Limites da política retornam `invalid_policy`. Cada requisição gera um log estruturado com status, código e duração, sem gravar o DOT ou valores de `input`.

### Desempenho observado

A referência completa — metodologia, ambiente, números e limitações — está em `docs/reports/performance.md`. Resumo:

- `Infer` custa ~1 µs contra ~40 µs do `Parse` por política média (5 nós, 6 arestas); inferência é ~37× mais barata que parsing.
- Carga de 50 RPS por 60 s, zero erros, P90 bem abaixo da meta de 30 ms — tanto via `httptest` quanto via k6 contra o binário em container distroless.

```bash
go test -run '^$' -bench . -benchmem -count=3 ./internal/policy   # benchmarks do motor
make load-test                                                  # 50 RPS via httptest
make perf-container                                             # k6 em container separado
```

São medições locais: não incluem cold start de Lambda nem API Gateway. Para emular via SAM, use `make sam-local`.

### Estrutura

- `cmd/infer`: entrypoint — Lambda por padrão, HTTP puro quando `HTTP_ADDR` está definido.
- `internal/policy`: parsing e validação DOT, modelo do grafo, condições, atribuições e inferência. Limites do `Parse` usam *Functional Options* (`policy.WithLimits`).
- `internal/httpapi`: contrato HTTP/API Gateway v2, middleware (correlation id, request logging) e adaptador `net/http`.
- `internal/apperr`: códigos de erro do contrato (`code`/`message`) compartilhados pelas camadas.
- `internal/config`: configuração por variáveis de ambiente e `.env` via viper.
- `internal/observability`: correlation id, spans e logger `slog` enriquecido por contexto.
- `Dockerfile` e `template.yaml`: imagem Lambda e execução local com SAM.
- `tools/api`: requests prontos (.http e collection Postman); `tools/perf`: harness k6 + Dockerfile da imagem de teste.
- `docs/swagger`: especificação OpenAPI gerada das anotações `swag` (`make swagger`).
- `scripts/pre-commit`: hook versionado (gofmt, vet, testes e frescor do swagger); habilitar uma vez por clone com `make install-hooks`.
- `internal/policy/policy_bench_test.go` e `internal/httpapi/handler_load_test.go`: evidências de desempenho.

## Documentação da API

O contrato `POST /infer` é gerado das anotações `swag` em `internal/httpapi/handler.go` + `cmd/infer/main.go` para `docs/swagger/` (`docs.go`, `swagger.json`, `swagger.yaml`). Regenerar após mudar anotações ou tipos do contrato com `make swagger`; o pre-commit verifica que o resultado esteja commitado. A spec em `docs/specs/policy-inference.md` continua sendo a referência do comportamento.

## Relatório de performance e ferramentas

- **Relatório de performance**: `docs/reports/performance.md` — metodologia, ambiente, benchmarks do motor e testes de carga (httptest e k6 em container; meta de P90 < 30 ms atendida).
- **Processo de implementação**: `docs/implementation.md` — como o serviço foi construído: incrementos R0–R7 da spec e a etapa de hardening posterior.
- **Makefile**: `make help` lista os alvos — `fmt`, `test`, `test-race`, `vet`, `check`, `build`, `build-lambda`, `bench`, `load-test`, `perf-container`, `run`, `debug`, `swagger`, `install-hooks`, `docker-build`, `sam-build`, `sam-local` (com `--warm-containers EAGER`) e `clean`.

## Testando a API manualmente

- `tools/api/policy-inference.http` — requests prontos para VS Code (extensão REST Client) ou Rider/IntelliJ; ajuste a variável `@host`.
- `tools/api/policy-inference.postman_collection.json` — collection Postman/Insomnia com happy paths, casos de erro e testes automáticos de status/código/correlation id.

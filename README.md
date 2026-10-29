# Inferência de Políticas — case técnico Go

Serviço que recebe uma política escrita em Graphviz DOT e variáveis de
entrada, executa um único caminho determinístico pelo grafo e devolve as
variáveis enriquecidas. Um único endpoint, `POST /infer`, publicável como
AWS Lambda em container (emulado localmente via SAM) ou executável como
servidor HTTP simples com o mesmo handler.

## Guia do revisor

| Documento | O que responde |
| --- | --- |
| [`docs/specs/policy-inference.md`](docs/specs/policy-inference.md) | O contrato: payload, semântica da política, códigos de erro, limites e os critérios de aceite AC01–AC18 |
| [`docs/implementation.md`](docs/implementation.md) | Como foi construído: os incrementos R0–R7 da spec e a etapa de hardening posterior |
| [`docs/reports/performance.md`](docs/reports/performance.md) | Evidência de desempenho: metodologia, ambiente, benchmarks e carga a 50 RPS |
| [`docs/swagger/`](docs/swagger) | OpenAPI do endpoint, gerada das anotações `swag` no código |
| [`GLOSSARY.md`](GLOSSARY.md) | Vocabulário do domínio |
| [`docs/enunciado.md`](docs/enunciado.md) | Enunciado original do case, preservado fora do README |

Sugestão: a spec para o *o quê*, `implementation.md` para o *como*, e as
seções abaixo para reproduzir.

## Estado atual

- `POST /infer` interpreta políticas DOT em um único `digraph`, com nós e arestas individuais e atributos `cond`/`result`.
- Condições aceitam comparações, `&&`, `||`, parênteses, booleanos isolados e curto-circuito da esquerda para a direita.
- `==` e `!=` comparam valores apenas quando os tipos coincidem; `>`, `<`, `>=` e `<=` são somente numéricos.
- `result` aceita booleanos, números e strings simples ou entre aspas, incluindo vazia e vírgula. Os escapes suportados são `\"`, `\\`, `\n` e `\t`.
- O percurso começa em `start`, aplica atribuições antes de avaliar saídas, segue a primeira condição verdadeira na ordem do DOT e encerra em folha ou sem saída verdadeira.
- Ciclos, política sem `start`, `cond` vazio/ausente, identificadores inválidos, construções DOT fora do subconjunto e sintaxe inválida em trechos não percorridos são rejeitados.
- A requisição valida método, caminho, `Content-Type`, campos exigidos, chaves JSON duplicadas e valores de entrada básicos.
- Limites configuráveis protegem corpo, quantidade de nós/arestas, tamanho de condição e profundidade de expressão. Logs registram status, código e duração sem expor política ou variáveis.

## Pré-requisitos

- Go 1.26+
- Docker Engine e AWS SAM CLI — apenas para a emulação Lambda; os testes e o
  modo HTTP local dependem somente de Go

## Verificação

```bash
go test ./...        # ou: make check (fmt + vet + testes com cobertura)
go vet ./...
make cover           # cobertura total de cmd/ e internal/; falha abaixo de 90%
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./build/infer ./cmd/infer
sam build
sam local start-api --warm-containers EAGER
```

## Execução local

O mesmo binário serve HTTP puro quando `HTTP_ADDR` está definido, sem passar
pelo SAM. As variáveis podem vir do ambiente ou de um arquivo `.env` na raiz
(ver `.env.example`; precedência: flags > env > `.env` > defaults):

```bash
make run     # go run ./cmd/infer -http-addr=:8080
make debug   # idem, com logs em nível debug
```

## Exemplo

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

## Rejeições

- JSON malformado, chaves duplicadas, campos extras, `policy_dot`/`input` ausentes ou tipos errados → `invalid_json`.
- `input` não objeto, chave fora da gramática, `null`, objeto ou lista como valor → `invalid_input`.
- DOT inválido ou política fora do subconjunto → `invalid_policy`.
- Variável ausente acessada ou tipos incompatíveis durante a avaliação → `inference_error`.
- Método diferente de `POST`, caminho diferente de `/infer` e corpo acima do limite usam `method_not_allowed`, `not_found` e `request_too_large`.

## Limites e observabilidade

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

A configuração é lida e validada uma única vez na inicialização; valores inválidos (limites não positivos, nível de log desconhecido) impedem o binário de subir. Corpo acima do limite retorna HTTP 413 com `request_too_large`, tanto no Lambda quanto no modo HTTP. Limites da política retornam `invalid_policy`. Cada requisição gera um log estruturado com status, código e duração, sem gravar o DOT ou valores de `input`.

## Desempenho observado

Resumo do [relatório completo](docs/reports/performance.md) (metodologia, ambiente e limitações):

- `Infer` custa ~1 µs contra ~40 µs do `Parse` por política média (5 nós, 6 arestas); em políticas complexas o `Parse` escala com o tamanho do DAG (DAG de 82 nós ≈ 2,6 ms) e `Infer` com a profundidade do caminho.
- Carga de 50 RPS por 60 s, zero erros, P90 bem abaixo da meta de 30 ms nas formas média e complexa — tanto via `httptest` quanto via k6 contra o binário em container distroless.

```bash
go test -run '^$' -bench . -benchmem -count=3 ./internal/policy   # benchmarks do motor, por forma de política
make load-test                                                  # 50 RPS via httptest — formas medium + complex (~140 s)
make perf-container                                             # k6 em container separado, ambas as formas
```

São medições locais: não incluem cold start de Lambda nem API Gateway. Para emular via SAM, use `make sam-local`.

## Estrutura

- `cmd/infer`: entrypoint — carrega a configuração uma vez, monta o serviço e o executa como Lambda (padrão) ou servidor HTTP (`HTTP_ADDR`).
- `internal/policy`: o motor. `Parse(dot, limits)` valida a política inteira e compila cada condição em nós tipados; `Infer` é dono do contrato da entrada e executa o percurso. Não lê ambiente nem conhece HTTP.
- `internal/httpapi`: um único pipeline (`Service`) sobre uma requisição neutra de transporte, com dois adapters finos — `Lambda` (API Gateway v2: stage, base64, request id) e `ServeHTTP` (net/http). O limite de corpo, o contrato de erros, o correlation id e o logging valem igualmente nos dois.
- `internal/apperr`: códigos de erro do contrato (`code`/`message`) compartilhados pelas camadas.
- `internal/config`: único módulo que lê o ambiente; precedência flags > env > `.env` > defaults, validada na carga.
- `internal/observability`: correlation id, spans e logger `slog` por contexto (injetável em testes via `WithLogger`).
- `Dockerfile` e `template.yaml`: imagem Lambda e execução local com SAM.
- `tools/api`: requests prontos (.http e collection Postman); `tools/perf`: harness k6 + Dockerfile da imagem de teste.
- `docs/`: spec, enunciado, processo de implementação, OpenAPI gerada e relatório de performance.
- `scripts/pre-commit`: hook versionado (gofmt, vet, testes com piso de 90% de cobertura e frescor do swagger); habilitar uma vez por clone com `make install-hooks`.
- `internal/policy/policytest`: geradores de políticas de formas controladas (cadeia, fan-out, condição larga, DAG em camadas) usados por `policy_bench_test.go` e `handler_load_test.go` — evidências de desempenho.

## Documentação da API

O contrato `POST /infer` é gerado das anotações `swag` em `internal/httpapi/handler.go` + `cmd/infer/main.go` para `docs/swagger/` (`docs.go`, `swagger.json`, `swagger.yaml`). Regenerar após mudar anotações ou tipos do contrato com `make swagger`; o pre-commit verifica que o resultado esteja commitado. A spec continua sendo a referência do comportamento.

## Ferramentas e teste manual

- **Makefile**: `make help` lista os alvos — `fmt`, `test`, `test-race`, `vet`, `cover`, `check`, `build`, `build-lambda`, `bench`, `load-test`, `perf-container`, `run`, `debug`, `swagger`, `install-hooks`, `docker-build`, `sam-build`, `sam-local` (com `--warm-containers EAGER`) e `clean`.
- `tools/api/policy-inference.http` — requests prontos para VS Code (extensão REST Client) ou Rider/IntelliJ; ajuste a variável `@host`.
- `tools/api/policy-inference.postman_collection.json` — collection Postman/Insomnia com happy paths, casos de erro e testes automáticos de status/código/correlation id.

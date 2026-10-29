Case Técnico — Desenvolvedor Go (Golang)

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
- Docker Engine
- AWS SAM CLI

### Verificação

```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./build/infer ./cmd/infer
sam build
sam local start-api --warm-containers EAGER
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

Corpo acima do limite retorna HTTP 413 com `request_too_large`. Limites da política retornam `invalid_policy`. Cada requisição gera um log estruturado com status, código e duração, sem gravar o DOT ou valores de `input`.

### Desempenho observado

Ambiente: Windows amd64, AMD Ryzen 5 5500, Go 1.26.4. Política média: 5 nós, 6 arestas, condições mistas e 4 variáveis de entrada.

```bash
go test -bench . -benchmem ./internal/policy
```

| Benchmark | Tempo | Memória | Alocações |
| --- | ---: | ---: | ---: |
| `BenchmarkParseMediumPolicy` | 54,864 ns/op | 27,855 B/op | 473 |
| `BenchmarkInferMediumPolicy` | 1,202 ns/op | 368 B/op | 5 |
| `BenchmarkParseAndInferMediumPolicy` | 54,908 ns/op | 28,223 B/op | 478 |

Uma repetição feita enquanto o Docker ainda removia containers do teste SAM saturado ficou mais lenta (95,060 ns/op; 2,036 ns/op; 105,768 ns/op). A diferença é ruído ambiental, não mudança no código.

Carga HTTP real, sem o emulador SAM, via `httptest.Server` e `http.Client`: 10 segundos de aquecimento, 60 segundos de medição, oferta fixa de 50 RPS.

```bash
POLICY_INFER_LOAD_TEST=1 go test -run TestHTTPLoadMediumPolicy -v ./internal/httpapi
```

Resultado: 3.000 requisições, 49,9995 RPS alcançados, 3.000 respostas HTTP 200, zero erros, P50 de 1,093 ms, P90 de 2,885 ms, P99 de 9,701 ms e máximo de 58,118 ms. A meta de P90 menor que 30 ms foi atendida nesse transporte local.

Essas medições não são medições AWS Lambda. A tentativa anterior de carga com `sam local start-api` no modo padrão criou um container `inferfunction:rapid-x86_64` por invocação, saturou o Docker local com cerca de 1.900 containers e produziu timeouts/502; portanto não é evidência de latência do serviço. Para comparar a emulação depois, use `sam local start-api --warm-containers EAGER`, mas trate o resultado como custo do emulador, não como custo de AWS.

### Estrutura

- `internal/policy`: parsing e validação DOT, modelo do grafo, condições, atribuições e inferência.
- `internal/httpapi`: contrato HTTP/API Gateway v2, erros, limites e logging.
- `cmd/infer`: entrypoint Lambda.
- `Dockerfile` e `template.yaml`: imagem Lambda e execução local com SAM.
- `internal/policy/policy_bench_test.go` e `internal/httpapi/handler_load_test.go`: evidências de desempenho.

## Análise de design e performance

- **Relatório de design patterns**: `docs/reports/design-patterns.md` — mapeia onde cada padrão se encaixa e documenta o padrão implementado (*Functional Options* para limites do `policy.Parse`, ex.: `policy.Parse(dot, policy.WithLimits(l))`).
- **Relatório de performance**: `docs/reports/performance.md` — metodologia, ambiente, benchmarks do motor e teste de carga HTTP real (50 RPS, P90 ~0,87 ms).
- **Makefile**: alvos `test`, `test-race`, `vet`, `build`, `build-lambda`, `bench`, `load-test`, `docker-build`, `sam-build`, `sam-local` (com `--warm-containers EAGER`) e `clean`.

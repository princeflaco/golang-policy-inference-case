# Relatório de performance

Data: 2026-10-29 · Branch: `feature/policy-inference`

## Ambiente

- CPU: AMD Ryzen 5 5500 · go1.26.4 windows/amd64
- Todas as medições são **locais** (micro-benchmarks do motor + servidor HTTP
  `httptest` em processo). Medem motor + transporte, não cold start de AWS
  Lambda, não API Gateway e não o custo de containers do `sam local`.

## Carga de trabalho

Política média (ver `internal/policy/policy_bench_test.go`): 5 nós, 6 arestas,
condições mistas (`>=`, `==`, `&&`/`||`), 4 variáveis de entrada (`age`,
`score`, `segment`, `active`).

Além dela, o pacote `internal/policy/policytest` gera formas parametrizadas
que estressam partes diferentes do motor, todas dentro dos limites default:

| Forma | Gerador | O que estressa |
| --- | --- | --- |
| `chain-100` | `Chain(100)` | 100 saltos sequenciais; o mapa de estado cresce a cada nó — pior caso de `Infer` |
| `fanout-50` | `Fanout(50)` | 50 condições avaliadas por requisição (só a última é verdadeira) — pior caso da ordem de declaração |
| `wide-condition-50` | `WideCondition(50)` | uma condição com 50 operandos em `&&` — compilação e avaliação de expressão profunda |
| `layered-6x6` | `Layered(6, 6)` | DAG de 37 nós / 186 arestas (~6 KB de DOT) — `Parse` domina |
| `layered-9x9` | `Layered(9, 9)` | DAG de 82 nós / 657 arestas (~20 KB de DOT) — `Parse` perto de limites realistas |

## Metodologia

```bash
# Micro-benchmarks, 3 repetições, estatísticas de alocação
go test -run '^$' -bench . -benchmem -count=3 ./internal/policy

# Carga HTTP: 50 RPS ofertados, 10 s de aquecimento + 60 s de medição por forma
go test ./internal/httpapi -run TestHTTPLoad -v -args -load                 # ou: make load-test (todas as formas)
go test ./internal/httpapi -run TestHTTPLoad -v -args -load -load-shape=complex   # só a forma complexa

# Cold start: N invocações, cada uma em container novo (sam local invoke)
make cold-start                    # ou: SAM=<sam.cmd> sh tools/perf/cold-start.sh 10
```

## Resultados — benchmarks do motor (3 execuções)

Medição após a revisão de arquitetura, com as condições compiladas em nós
tipados no parse:

| Forma | `Parse` ns/op | `Infer` ns/op | `Infer` allocs/op |
| --- | ---: | ---: | ---: |
| medium | ~46 000 | ~1 100 | 2 |
| chain-100 | ~938 000 | ~19 200 | 11 |
| fanout-50 | ~425 000 | ~2 100 | 2 |
| wide-condition-50 | ~56 000 | ~9 200 | 4 |
| layered-6x6 | ~825 000 | ~860 | 2 |
| layered-9x9 | ~2 635 000 | ~1 560 | 5 |

Antes da revisão, `Infer` alocava 5 objetos por chamada (~1 096 ns/op) e
re-percorria a AST do `go/parser` a cada avaliação; agora executa nós já
compilados e aloca só o mapa de estado. O `Parse` ficou no mesmo patamar,
dentro do ruído entre execuções.

Leitura das formas:

- **`Parse` escala com o tamanho do DOT**, não com a complexidade da condição:
  `wide-condition-50` (1 aresta, expressão de 50 operandos) custa ~56 µs,
  quase igual à política média; `layered-9x9` custa ~2,6 ms por requisição —
  657 condições compiladas mesmo que só 9 sejam avaliadas.
- **`Infer` só fica caro quando o caminho é longo**: `chain-100` paga ~19 µs
  e 11 alocações porque o mapa de estado cresce até ~100 entradas; nos demais
  formatos fica na faixa de 1–9 µs e 2–5 alocações.
- Avaliar 50 condições por requisição (`fanout-50`) custa só ~2 µs — a ordem
  de declaração com short-circuit é barata; o custo real do fan-out está no
  `Parse`, que compila todas elas.
- Um serviço que re-parseia a mesma política a cada requisição gasta ~97% da
  CPU no parser (medium) e mais ainda em políticas largas — um cache
  `policy → *Policy` indexado por hash do conteúdo é a otimização de maior
  valor se políticas se repetem entre invocações.

## Resultados — teste de carga HTTP (50 RPS, 60 s)

O teste exercita o adapter `net/http` de produção (`httpapi.Service`), o
mesmo que o servidor local e a imagem de perf usam; antes da revisão de
arquitetura ele usava uma tradução própria da requisição.

```json
// medium — 5 nós, 6 arestas
{"requests": 3000, "errors": 0, "achieved_rps": 49.9997,
 "mean_ms": 0.5683, "p50_ms": 0.5161, "p90_ms": 0.9722,
 "p99_ms": 1.4826, "max_ms": 6.4754}

// complex — layered-6x6: 37 nós, 186 arestas, ~6 KB de DOT
{"requests": 3000, "errors": 0, "achieved_rps": 49.9987,
 "mean_ms": 1.9706, "p50_ms": 1.5743, "p90_ms": 2.4397,
 "p99_ms": 7.3702, "max_ms": 125.6588}
```

| Métrica | medium | complex | Meta | Resultado |
| --- | ---: | ---: | ---: | --- |
| RPS ofertado → alcançado | 50 → 49,9997 | 50 → 49,9987 | sustentar 50 | passou |
| Erros / não-200 | 0 / 0 | 0 / 0 | 0 | passou |
| Latência P90 | 0,972 ms | 2,44 ms | < 30 ms | passou (~12× de folga) |
| Latência máxima | 6,48 ms | 125,7 ms | — | passou |

A forma complexa multiplica a latência mediana por ~3 (0,52 → 1,57 ms), coerente
com o benchmark: `Parse` de `layered-6x6` custa ~0,8 ms por requisição. O pico
de 125,7 ms é um outlier isolado (P99 fica em 7,4 ms), provável pausa de GC ou
agendamento do Windows — não um padrão da carga.

## Ressalvas

- **Não é performance de Lambda.** As medições HTTP excluem cold start e
  saltos do API Gateway — cold start é medido separadamente na seção
  `sam local invoke` abaixo. A tentativa anterior de medir *carga* através
  do `sam local start-api` produziu dados inválidos: o modo
  padrão cria um container `rapid-x86_64` por invocação e saturou o daemon
  Docker local (~1 900 containers, timeouts/502). Qualquer medição futura via
  SAM deve usar `sam local start-api --warm-containers EAGER` (`make sam-local`).
- A latência é dominada pelo round-trip HTTP + decode do JSON + `Parse` por
  requisição; `Infer` só aparece em caminhos longos (chain-100: ~19 µs).

## Resultados — execução k6 containerizada (ambiente neutro)

A aplicação roda em container distroless (`tools/perf/Dockerfile`,
`HTTP_ADDR=:8080`, mesmo binário da imagem Lambda); o gerador de carga é
`grafana/k6` em container separado. Reproduzir com `make perf-container`
(roda as duas formas; `make perf-container POLICY_SHAPES=complex` roda só uma).
Medição feita depois da revisão de arquitetura, no binário atual.

- 10 s de aquecimento + 60 s de medição a 50 iterações/s (constant-arrival-rate)
- Thresholds atendidos nas duas formas: `http_req_failed rate==0`, `p(90)<30`

| Forma | Requisições | Falhas | média | mediana | p(90) | p(95) | máx |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| medium | 3 501 | 0 | 2,27 ms | 2,18 ms | **2,71 ms** | 2,96 ms | 18,87 ms |
| complex | 3 502 | 0 | 4,81 ms | 3,12 ms | **4,05 ms** | 4,61 ms | 55,45 ms |

A forma complexa envia ~10× mais dados (24 MB contra 2,3 MB no agregado) e
multiplica a mediana por ~1,4 — coerente com o `Parse` por requisição medido
nos benchmarks. É a medição neutra mais próxima disponível localmente:
container Linux, processo isolado, ferramenta de carga externa — ainda
excluindo cold start de Lambda e saltos do API Gateway.

## Resultados — cold start (`sam local invoke`)

Cada `sam local invoke` cria um container `rapid-x86_64` novo, então toda
invocação é um cold start por construção. 10 invocações da política média
(`tools/perf/event-medium.json`, API Gateway v2) sobre a imagem SAM, que usa
a base real `public.ecr.aws/lambda/provided:al2023` com o mesmo binário.
Reproduzir com `make cold-start` (`COLD_START_N` ajusta a amostra).

| Métrica | min | mediana | média | máx |
| --- | ---: | ---: | ---: | ---: |
| `Init Duration` (REPORT) | 0,06 ms | 0,09 ms | 0,10 ms | 0,18 ms |
| `Duration` (REPORT) | 5,28 ms | 5,87 ms | 6,14 ms | 7,95 ms |
| wall time por invoke | 8 713 ms | 8 841 ms | 8 946 ms | 9 810 ms |

Leitura:

- **`Init Duration` ~0,1 ms**: o init do binário Go estático é essencialmente
  nulo — sem interpretador nem JIT para aquecer. Este número é
  representativo: a imagem usa a base real do Lambda com o mesmo binário.
- **`Duration` ~6 ms contra ~1 ms de trabalho no handler** (visível no log
  `request completed duration_ms=1.123`): em uma invocação fria o custo
  dominante é o bootstrap do runtime (loop do Runtime API), não a política.
- **wall ~9 s** é orquestração local — SAM CLI + `docker run` + create +
  teardown. Não tem equivalente na AWS e não deve ser lido como latência de
  cold start real.
- Ressalva de sempre: Firecracker/microVMs da AWS têm outro caminho de
  provisionamento. Os números acima limitam a *magnitude* do init, não o
  cold start real em produção — para esse número só publicando e medindo na
  conta AWS de verdade.

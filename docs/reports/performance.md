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

## Metodologia

```bash
# Micro-benchmarks, 3 repetições, estatísticas de alocação
go test -run '^$' -bench . -benchmem -count=3 ./internal/policy

# Carga HTTP: 50 RPS ofertados, 10 s de aquecimento + 60 s de medição
go test -run TestHTTPLoad -load ./internal/httpapi -v   # ou: make load-test
```

## Resultados — benchmarks do motor (mediana de 3 execuções)

| Operação | ns/op | B/op | alocações/op |
| --- | ---: | ---: | ---: |
| `Parse` (política média) | ~40 558 | 21 772 | 428 |
| `Infer` | ~1 096 | 368 | 5 |
| `Parse + Infer` | ~38 743 | 22 140 | 433 |

Leitura: inferência é ~37× mais barata que parsing (1 µs vs 40 µs). Um serviço
que re-parseia a mesma política a cada requisição gasta ~97% da CPU no parser —
um cache `policy → *Policy` indexado por hash do conteúdo é a otimização de
maior valor se políticas se repetem entre invocações. `Infer` em si aloca
apenas 5 objetos por chamada (o mapa de saída e suas entradas).

## Resultados — teste de carga HTTP (50 RPS, 60 s)

```json
{
  "offered_rps": 50,
  "achieved_rps": 49.9999,
  "requests": 3000,
  "errors": 0,
  "statuses": {"200": 3000},
  "mean_ms": 0.5034,
  "p50_ms":   0.4679,
  "p90_ms":   0.8744,
  "p99_ms":   1.2483,
  "max_ms":   5.8549
}
```

| Métrica | Valor | Meta | Resultado |
| --- | ---: | ---: | --- |
| RPS ofertado → alcançado | 50 → 49,9999 | sustentar 50 | passou |
| Erros / não-200 | 0 / 0 | 0 | passou |
| Latência P90 | 0,874 ms | < 30 ms | passou (~34× de folga) |
| Latência máxima | 5,85 ms | — | passou |

## Ressalvas

- **Não é performance de Lambda.** Os números excluem cold start de container,
  init do runtime Lambda e saltos do API Gateway. A tentativa anterior de
  medir através do `sam local start-api` produziu dados inválidos: o modo
  padrão cria um container `rapid-x86_64` por invocação e saturou o daemon
  Docker local (~1 900 containers, timeouts/502). Qualquer medição futura via
  SAM deve usar `sam local start-api --warm-containers EAGER` (`make sam-local`).
- A latência é dominada pelo round-trip HTTP + decode do JSON; com 1,1 µs de
  trabalho do motor, `Infer` não é o gargalo — o `Parse` por requisição é.

## Resultados — execução k6 containerizada (ambiente neutro)

A aplicação roda em container distroless (`tools/perf/Dockerfile`,
`HTTP_ADDR=:8080`, mesmo binário da imagem Lambda); o gerador de carga é
`grafana/k6` em container separado. Reproduzir com `make perf-container`.

- 10 s de aquecimento + 60 s de medição a 50 iterações/s (constant-arrival-rate)
- 3 502 requisições, 0 falhas, 100% dos checks aprovados
- http_req_duration: média 2,15 ms, mediana 2,04 ms, **p(90) 2,56 ms**, p(95) 2,8 ms, máx 15,87 ms
- Thresholds atendidos: `http_req_failed rate==0`, `p(90)<30`

É a medição neutra mais próxima disponível localmente: container Linux,
processo isolado, ferramenta de carga externa — ainda excluindo cold start de
Lambda e saltos do API Gateway.

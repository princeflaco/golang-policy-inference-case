# Processo de implementação

Como este serviço foi construído. O contrato comportamental está na
[spec](specs/policy-inference.md); este documento registra o processo, não o
contrato.

## Método

O trabalho foi dividido em incrementos pequenos e verificáveis (R0–R7),
acompanhados por issues no GitHub (#1–#9). Cada incremento só se fecha com
testes, evidência e documentação de uso — não com código "provavelmente
certo". Ambiguidades do enunciado foram transformadas em contrato explícito
antes da implementação (R0), registradas na spec em vez de decididas
silenciosamente.

## Incrementos da spec

| Etapa | Entrega |
| --- | --- |
| R0 | Contrato fechado: gramática de identificadores, literais, escapes, códigos de erro HTTP, limites configuráveis, payload API Gateway v2 |
| R1 | Fluxo real mínimo: `POST /infer` via SAM + container Lambda, interpretando comparação numérica com duas saídas |
| R2 | Comparações e atribuições para string, number e boolean: igualdade por tipo, ordenação só numérica, literais e escapes, sem conversão automática |
| R3 | Percurso em DAGs com etapas intermediárias: atribuições em `start` e nós, primeira saída verdadeira na ordem do DOT, folha ou sem-saída encerrando, estado isolado por requisição |
| R4 | Condições compostas: `&&`, `||`, parênteses e precedência com curto-circuito; variável ausente só erra quando acessada; falha sem output parcial |
| R5 | Rejeições consistentes: validação da política inteira (inclusive trechos não percorridos), construções proibidas, tipos de entrada e casos HTTP completos |
| R6 | Limites e observabilidade: cinco limites configuráveis aplicados antes do parsing quando possível, 413, logging de duração/falhas sem expor dados |
| R7 | Desempenho e reprodutibilidade: benchmark do motor, carga HTTP a 50 RPS com P90 documentado, instruções reproduzidas de árvore limpa |

As decisões de biblioteca ficaram na seção 6 da spec (parser DOT do Gonum,
`go/parser` + avaliador próprio para condições), com alternativas e
justificativas — sem ADR por biblioteca, por decisão explícita.

## Hardening pós-spec

Depois do fechamento de R0–R7, uma etapa de consolidação e tooling:

- **Camada de erros unificada**: os códigos do contrato viviam duplicados em
  `policy.Error.Kind` (nunca lido) e `httpapi.requestErr`. Ambos foram
  substituídos por `internal/apperr` — um tipo único `Error{Code, Message}`
  que já é o corpo de erro do wire, com o mapeamento código → status HTTP
  isolado no `httpapi` (o motor não conhece HTTP).
- **Constantes centralizadas**: defaults de limites (`policy.DefaultLimits`),
  tamanho de corpo (`config.DefaultMaxBodyBytes`), gramática de
  identificadores (`policy.IsIdentifier`) e nomes de atributos DOT em um só
  lugar cada.
- **Observabilidade**: log de request passou a incluir o `code` de erro;
  `LOG_LEVEL` + flags `-http-addr`/`-log-level` permitem `make debug` sem
  depender de sintaxe POSIX de ambiente.
- **OpenAPI gerado**: anotações `swag` no handler geram `docs/swagger/`
  (spec como artefato, sem endpoint de UI — o contrato continua rejeitando
  caminhos fora de `/infer`). `swag` é pinado como Go tool no `go.mod`.
- **Pre-commit versionado**: `scripts/pre-commit` (habilitado via
  `make install-hooks`) roda gofmt, vet, testes e verifica que
  `docs/swagger` está atualizado.
- **Makefile portátil**: alvos `run`, `debug`, `load-test`, `swagger`,
  `check`, `help` funcionando em shells Windows e POSIX; o gating do teste
  de carga virou flag `-load` em vez de variável de ambiente.

## Referências

- [Spec e plano](specs/policy-inference.md) — contrato, critérios de aceite e
  detalhamento dos incrementos
- [Enunciado original](enunciado.md) — o case preservado fora do README
- [Relatório de performance](reports/performance.md) — medições e limitações
- [Glossário](../GLOSSARY.md) — vocabulário do domínio

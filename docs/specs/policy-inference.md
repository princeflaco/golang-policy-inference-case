# Spec e plano — inferência de políticas

Status: R0–R7 concluídas. POST /infer funciona localmente via SAM e container Lambda; testes, limites, logging, benchmark e carga HTTP local foram verificados. Backlog publicado nas issues #1–#9.

Esta spec registra o contrato acordado para o [case original](../enunciado.md).
Os termos do domínio estão no [glossário](../../GLOSSARY.md).
Pendências de detalhamento estão na seção 9; não são decisões implícitas.

## 1. Objetivo e escopo

Receber uma política DOT e variáveis, executar um único caminho determinístico
e retornar as variáveis enriquecidas.

Entregar `POST /infer` via API Gateway e AWS Lambda empacotada em container,
com execução local pelo AWS SAM. Publicação na AWS não é obrigatória.

Inclui parsing, validação, inferência, erros, logging básico, testes,
benchmark, teste de carga, atualização do README e documentação de uso
produzida junto de cada incremento implementado.

Fora do escopo: persistência ou versionamento de políticas, banco de dados,
interface visual, autenticação, infraestrutura produtiva, objetos e listas
como valores de variáveis, acesso a campos, funções e operações aritméticas.
Não será criado um segundo servidor HTTP Go apenas para desenvolvimento local.

## 2. Origem das regras

O README exige DOT, DAG, entrada em `start`, atribuições em nós, condições
em arestas, primeira condição verdadeira, um caminho por execução e saída
com variáveis de entrada e inferidas. Exige comparações, `&&`, `||`, tipos
string/number/boolean, testes, logging e evidência de desempenho.

São escolhas complementares aprovadas: ordem de declaração como prioridade,
atribuições em `start`, curto-circuito, parênteses, regras de tipos e erros,
representação numérica, subconjunto DOT, limites, bibliotecas, container e SAM.
Essas escolhas completam lacunas; não devem ser apresentadas como exigências
explícitas do enunciado.

O contrato comportamental desta spec é a referência de implementação. O
enunciado original permanece preservado.

## 3. Contrato HTTP

### Requisição

```http
POST /infer
Content-Type: application/json
```

```json
{
  "policy_dot": "digraph { start [result=\"\"]; ok [result=\"approved=true\"]; start -> ok [cond=\"age>=18\"]; }",
  "input": {
    "age": 20
  }
}
```

O handler valida explicitamente método `POST`, caminho `/infer` e
`Content-Type: application/json`, aceitando parâmetros como `charset=utf-8`,
mesmo que a rota do API Gateway já filtre a maioria dos casos.

`policy_dot` é obrigatório e deve ser uma string não vazia contendo o texto DOT.
`input` é obrigatório e deve ser um objeto JSON; pode ser vazio. Seus valores
individuais são strings, números ou booleanos; não aceita `null`, objetos
aninhados ou listas. As chaves de `input` seguem a mesma gramática de
identificadores das variáveis.

Rejeitar campos de primeiro nível desconhecidos e chaves duplicadas em qualquer
nível do JSON. Essa rejeição evita que o último valor de uma chave duplicada
vença silenciosamente.

### Sucesso

```json
{
  "output": {
    "age": 20,
    "approved": true
  }
}
```

A saída mantém as variáveis recebidas, exceto os valores substituídos por
atribuições, e inclui as variáveis criadas durante a inferência.

### Erros

O corpo contém `code` e `message`, sem `output` parcial:

```json
{
  "code": "invalid_policy",
  "message": "policy graph must contain a start node"
}
```

`code` é estável para clientes; `message` explica o problema.

| Status | Code | Situação |
| --- | --- | --- |
| 400 | `invalid_json` | JSON malformado, chave duplicada ou formato incompatível |
| 400 | `invalid_input` | `input` ausente, de tipo errado, com identificador inválido ou valor não suportado |
| 400 | `invalid_policy` | DOT, estrutura ou sintaxe da política inválida |
| 400 | `inference_error` | Erro durante a execução, como variável ausente acessada ou tipos incompatíveis |
| 413 | `request_too_large` | Corpo da requisição acima do limite |
| 405 | `method_not_allowed` | Método diferente de POST |
| 404 | `not_found` | Caminho diferente de `/infer` |
| 500 | `internal_error` | Falha interna inesperada |

Erros de entrada devem identificar o problema e, quando aplicável, o nó,
a aresta ou a variável. Erros internos não expõem detalhes de implementação.

## 4. Semântica da política

### Validação anterior à execução

Validar toda a estrutura e a sintaxe, inclusive trechos não percorridos:

- Um único grafo direcionado, com `start` e sem ciclos.
- Sintaxe de todas as condições e atribuições.
- Presença de `cond` não vazio em toda aresta.
- Respeito ao subconjunto DOT, operadores, construções e limites aceitos.

Essa validação não garante que toda entrada será executável. Presença e
compatibilidade de valores podem depender do percurso e são verificadas
quando as expressões correspondentes forem avaliadas.

### Percurso

1. Iniciar o estado com as variáveis recebidas.
2. Entrar em `start` e aplicar suas atribuições.
3. Avaliar as arestas de saída na ordem de declaração no DOT.
4. Seguir somente a primeira condição verdadeira.
5. Ao entrar no destino, aplicar suas atribuições e repetir.
6. Encerrar em um nó sem saídas ou sem condição de saída verdadeira.
7. Em caso de erro, falhar a requisição sem devolver resultado parcial.

Nós sem `result`, ou com `result` vazio, não alteram o estado.
Atribuições podem criar variáveis ou substituir valores e tipos existentes.
Os novos valores ficam disponíveis para condições seguintes.

## 5. Linguagem e formatos

### DOT

Aceitar um único `digraph` ou `strict digraph`, com declarações individuais
de nós e arestas. Preservar a ordem de declaração das saídas de cada nó.
Rejeitar `graph`, porque o case exige direção.

Nomes de nós e variáveis usam a mesma gramática de identificadores:
começar com letra ASCII ou `_`, seguida de letras ASCII, dígitos ou `_`.
Sem espaços, pontos, hífens ou aspas. Exemplos válidos: `start`, `approved`,
`score_2`. Exemplos inválidos: `customer.id`, `approved-2`, `"nó 1"`.

Nós podem ser declarados explicitamente ou aparecer apenas como endpoint de
uma aresta. Nós desconectados são permitidos e inertes, mas continuam sujeitos
às validações estruturais. Rejeitar declarações repetidas de um mesmo nó e
arestas repetidas entre a mesma origem e destino. Rejeitar atributos repetidos
na mesma declaração; em `result`, chaves repetidas também são erro.

Rejeitar subgrafos, portas, arestas encadeadas e declarações de atributos
padrão. Atributos visuais ou desconhecidos, como `label` e `color`, não afetam
a inferência. DOT descreve a estrutura; não avalia `cond` nem executa `result`.

### Valores

- Strings, números e booleanos.
- Números finitos representados por `float64` na entrada, condições e atribuições.
- Literais numéricos usam `18`, `0.5`, `18.25`, `-18` ou `-0.5`.
  Rejeitar `.5`, `1.` e `1e3`; o sinal negativo pertence ao literal, não é aritmética.
- Na entrada JSON, aceitar números válidos de JSON que possam ser representados
  como valores finitos dentro das limitações de `float64`.
- Documentar aproximações decimais e perda de precisão de inteiros muito grandes.
- Igualdade numérica sem tolerância implícita.
- Nenhuma conversão automática de string para número ou entre outros tipos.

### Condições

| Construção | Regra |
| --- | --- |
| `==`, `!=` | Comparar valores do mesmo tipo |
| `>`, `<`, `>=`, `<=` | Comparar somente números |
| `&&`, `\|\|` | Operandos booleanos e curto-circuito da esquerda para a direita |
| Parênteses | Alterar agrupamento |
| Precedência | Comparações antes de `&&`; `&&` antes de `\|\|` |
| Identificadores | Referenciar variáveis ou avaliar booleanos diretamente |
| Strings entre aspas duplas | Representar textos literais |
| `true`, `false` e números | Representar literais e podem formar uma condição booleana |

Variáveis booleanas e os literais `true`/`false` podem ser usados como condição
sem comparação. `!` não faz parte da linguagem; para negar, usar
`active==false`. Em strings de condições, aceitar escapes comuns `\"`, `\\`,
`\\n` e `\\t`, preservando as regras de escape de DOT e JSON.

Chamadas de função, acesso a campos, indexação, coleções e operadores
aritméticos não pertencem à linguagem aceita. A sintaxe mais ampla de Go
não amplia automaticamente este contrato.

Exemplos:

```text
age >= 18 && score > 700
segment == "prime"
age >= 18 && (score > 700 || manual == true)
```

Com `age=16`, `age>=18 && score>700` é falso sem acessar `score`.
Com `age=25` e `score` ausente, a mesma expressão gera erro.
Se uma variável ausente for acessada no lado esquerdo, o erro não é
neutralizado pelo lado direito.

Aspas de strings nas condições precisam ser escapadas ao embutir a expressão
no DOT e, novamente, ao embutir o DOT em JSON.

### Atribuições

`result` contém pares `chave=valor` separados por vírgulas fora de strings
entre aspas. As chaves usam a gramática de identificadores. Os valores são
literais, não expressões ou referências. Chaves repetidas no mesmo `result`
são inválidas.

Strings simples aceitam letras ASCII, dígitos, `_` e `-`, sem espaços e sem
vírgula. Strings explícitas usam aspas duplas e os escapes `\"`, `\\`, `\\n`
e `\\t`.

| Texto de atribuição | Significado |
| --- | --- |
| `approved=true` | Booleano |
| `score=720` | Número |
| `segment=prime` | String simples |
| `segment=age` | Texto `age`, não consulta à variável |
| `segment="true"` | String, não booleano |
| `segment=""` | String vazia |
| `segment="prime,manual"` | String contendo vírgula |

Os exemplos mostram o conteúdo de `result` após decodificação do atributo DOT.
Não dividir atribuições por vírgula sem respeitar aspas e escapes.

## 6. Implementação escolhida e justificativa

```text
Texto DOT
  → parser DOT do Gonum
  → conversão e validação do modelo de política
  → execução do percurso
       → condições: go/parser + validação restrita + avaliador próprio
       → atribuições: leitor do formato de result
  → resultado ou erro
```

A integração usa API Gateway HTTP API payload v2.0 e imagem Lambda
`linux/amd64`. O motor fica independente do transporte HTTP e da AWS. Seu chamador fornece
política e variáveis e recebe resultado ou erro, sem conhecer a AST do Gonum
ou de Go. A distribuição em pacotes será definida durante a implementação,
sem criar interfaces abstratas para variações apenas hipotéticas.

### DOT: Gonum em vez de parser próprio

Reutilizar a leitura de DOT e consumir sua AST diretamente. A análise do
código-fonte indica declarações armazenadas em ordem; comprovar com testes
na versão escolhida.

`gographviz` também oferece uma AST ordenada e seria viável. Seu fluxo usual
de AST para `Graph` valida atributos contra uma lista que não inclui `cond`
e `result` no código consultado; consumir a AST diretamente evita esse passo.
A preferência por Gonum é pela representação da AST, não por incapacidade
do concorrente.

### Condições: go/parser em vez de parser próprio ou avaliador completo

Reutilizar parsing, precedência e agrupamento. Implementar uma lista explícita
de construções aceitas e avaliar somente essas construções. Não compilar nem
executar código Go recebido.

Expr reutilizaria também a avaliação, mas exigiria restringir uma linguagem
mais ampla e verificar diferenças de tipos e valores ausentes. CEL tem
semântica de propagação de erros lógicos diferente da escolhida. Não adotar
essas dependências nesta abordagem.

As decisões ficam aqui com suas alternativas; não criar um ADR por biblioteca.
Se surgir uma decisão arquitetural de alto custo de reversão e trade-off
relevante, registrar um ADR específico.

## 7. Limites e observabilidade

Limites configuráveis iniciais:

| Recurso | Limite |
| --- | --- |
| Corpo HTTP | 1 MiB |
| Nós | 1.000 |
| Arestas | 5.000 |
| Texto de cada condição | 4 KiB |
| Profundidade de expressão | 64 |

São proteções escolhidas, não valores exigidos pelo case. Medir tamanhos
de texto em bytes e complexidade antes de avaliar. Verificar na implementação
quais proteções devem ocorrer antes do parsing e quais dependem da AST.

Os limites são configuráveis por variáveis de ambiente, com defaults:

| Variável | Default |
| --- | --- |
| `MAX_BODY_BYTES` | `1048576` |
| `MAX_POLICY_NODES` | `1000` |
| `MAX_POLICY_EDGES` | `5000` |
| `MAX_CONDITION_BYTES` | `4096` |
| `MAX_EXPRESSION_DEPTH` | `64` |

Logging básico deve permitir identificar falhas e duração sem registrar
integralmente a política ou as variáveis recebidas.

## 8. Critérios de aceite

| ID | Evidência esperada |
| --- | --- |
| AC01 | Exemplos do README produzem as saídas esperadas |
| AC02 | Duas saídas verdadeiras escolhem somente a primeira na ordem do DOT |
| AC03 | Atribuições de `start` e de nós intermediários afetam condições seguintes |
| AC04 | Nó folha e ausência de saída verdadeira encerram com o estado atual |
| AC05 | Curto-circuito evita acessos desnecessários; variável realmente acessada e ausente gera erro |
| AC06 | Comparações respeitam tipos e não fazem conversão automática |
| AC07 | Sobrescrita de valor e de tipo funciona; uso posterior incompatível gera erro |
| AC08 | Ciclos, falta de `start`, sintaxe inválida e construções proibidas são rejeitados |
| AC09 | Sintaxe inválida em trecho não percorrido também é rejeitada |
| AC10 | Strings simples, explícitas, vazias, com vírgulas e escapes são interpretadas corretamente |
| AC11 | Objetos, listas, null, valores numéricos não finitos e recursos acima dos limites são rejeitados |
| AC12 | Erros HTTP seguem o contrato e não incluem output parcial ou detalhes internos inesperados |
| AC13 | Requisições independentes não compartilham alterações de estado |
| AC14 | POST /infer funciona localmente via SAM e container Lambda |
| AC15 | Benchmark inclui parsing, validação e execução, com cenário documentado |
| AC16 | Carga HTTP a 50 RPS registra P90, erros, aquecimento, ambiente e política usada |
| AC17 | Instruções permitem reproduzir testes, execução local e medições |
| AC18 | Cada incremento inclui sua documentação de uso, exemplos e limitações aplicáveis; o README aponta para instruções existentes e distingue funcionalidades prontas das planejadas |

Meta do README: P90 menor que 30 ms a 50 RPS, desconsiderando cold start.
Benchmark isolado não comprova P90 HTTP. Emulação local não equivale a medição
na AWS. Registrar valores observados e investigar eventual descumprimento,
sem declarar a meta atingida antes de medir.

## 9. Pendências explícitas

Resolver antes da etapa que depende de cada item, por perguntas via ask quando
houver escolha de comportamento; fatos de ferramentas devem ser investigados:

- Versões fixadas das dependências, com releases de pelo menos sete dias
  quando disponíveis. Nenhuma dependência foi instalada durante a definição.
- Cenários de carga, duração, concorrência e critérios de aquecimento.
  A política representativa deve ser explícita; não generalizar um único caso.

## 10. Plano de implementação

R0–R7 estão concluídas. Em cada uma: explicar o conceito, percorrer
um exemplo, implementar uma parte pequena, verificar e revisar o aprendizado
antes de avançar. O foco didático é política, DOT e interpretação; explicar Go
quando a decisão for relevante, sem presumir que o usuário é iniciante.

O backlog publicado contém a issue principal #1 e as filhas #2–#9, mapeadas
abaixo como R0–R7. A sequência substitui o plano inicial de seis etapas e
antecipa um endpoint mínimo, ampliado a cada incremento.
Dentro de cada incremento, desenvolver e explicar o motor antes de conectá-lo
ao handler.

R0 e R7 são HITL: exigem decisões ou discussão de resultados com o usuário.
R1 a R6 são AFK no sentido técnico: podem ser implementadas sem novas decisões
após a resolução dos bloqueios. Isso não elimina os checkpoints didáticos
nem autoriza execução autônoma sem acompanhar o usuário.

Dependências aprovadas: `R0 → R1 → R2 → R3 → R4 → R5 → R6 → R7`.
A issue principal acompanha AC01 a AC18, evidências, documentação reproduzível
e conclusão das filhas, sem duplicar o contrato desta spec.

### Documentação como parte da conclusão

A spec e seu plano orientam a implementação; a documentação de uso é um
entregável a construir com ela. Uma etapa só está concluída quando, além
de seus testes e critérios específicos, cumprir AC18.

- Atualizar o README progressivamente com pré-requisitos, comandos já
  verificados e referências para documentação de uso mais detalhada.
- Preservar o texto do enunciado original, distinguindo-o das novas seções
  sobre a solução. Preservar o enunciado não significa deixar o README intocado.
- Documentar apenas comportamentos efetivamente implementados como disponíveis;
  manter itens futuros identificados como pendentes.
- Criar guias detalhados quando necessários, com exemplos, erros e limitações
  pertinentes ao incremento. A spec continua sendo a referência do contrato,
  sem copiar integralmente suas regras para vários documentos.
- Atualizar o glossário quando termos do domínio forem resolvidos e registrar
  ADRs apenas para decisões que justifiquem esse formato.
- Incluir essas obrigações nos critérios das futuras issues correspondentes.
  A etapa final revisa a documentação acumulada, não substitui sua produção.

### R0 — fechar os detalhes necessários à implementação

GitHub: #2. Tipo: HITL. Bloqueada por: nenhuma. Preparação, não incremento funcional.

- [x] Resolver via ask as pendências de identificadores, literais, escapes
  e condições booleanas isoladas.
- [x] Definir redeclarações, duplicidades, nós implícitos e construções DOT pendentes.
- [x] Definir os detalhes HTTP, códigos de erro, configuração de limites,
  versão do evento API Gateway e arquitetura da imagem.
- [x] Registrar as decisões e exemplos aceitos e rejeitados nesta spec,
  atualizando o glossário apenas quando houver termos do domínio a resolver.
- [x] Deixar cenários, duração e aquecimento da carga para decisão em R7.

Aprendizado: transformar ambiguidades em um contrato testável.

Conclusão: os incrementos R1 a R6 não dependem de decisões comportamentais
em aberto. Fatos de ferramentas são investigados, não escolhidos pelo usuário.

### R1 — executar uma decisão simples pelo endpoint local

GitHub: #3. Tipo: AFK após R0. Bloqueada por: R0.

- [x] Inicializar o módulo, selecionar versões adequadas e comprovar parsing
  de atributos personalizados, ordem das saídas e ASTs de condições.
- [x] Interpretar uma política recebida com comparação numérica e duas
  alternativas finais, atribuindo um resultado booleano.
- [x] Preservar as variáveis de entrada e acrescentar o resultado; não fixar
  a política ou a decisão em código Go.
- [x] Restringir explicitamente o subconjunto inicial e rejeitar construções
  ainda não implementadas, em vez de ignorá-las.
- [x] Implementar o fluxo HTTP mínimo com respostas de sucesso e erro,
  Dockerfile e SAM; verificar Docker Engine e requisitos locais.
- [x] Pedir aprovação antes de instalar ferramentas e registrar bloqueios.
- [x] Testar ambos os resultados e duas saídas verdadeiras pelo motor
  e pela integração HTTP.
- [x] Atualizar o README com pré-requisitos, build, execução local e chamada
  reproduzível, distinguindo suporte atual de funcionalidades futuras.

Aprendizado: JSON, DOT, AST, decisão e resposta são etapas diferentes.

Conclusão: primeiro fluxo real via SAM e container Lambda, cobrindo inicialmente
AC01, AC02 e AC12 e verificando AC14, AC17 para esse fluxo e AC18.
Se uma biblioteca falhar nas provas, discutir antes de substituí-la.
Progresso: motor e handler implementados e verificados por `go test ./...`,
`go vet ./...`, build Linux/amd64, `sam build` e `sam local start-api`.
Chamadas reais a `POST /infer` retornaram `approved=true` para `age=20` e
`approved=false` para `age=16`.

### R2 — decidir e atribuir valores dos três tipos suportados

GitHub: #4. Tipo: AFK após R0. Bloqueada por: R1.

- [x] Ampliar comparações e atribuições para strings, números e booleanos.
- [x] Implementar igualdade por tipo e ordenação somente numérica,
  sem conversão automática.
- [x] Ler strings simples, explícitas, vazias, com vírgulas e escapes.
- [x] Testar valores numéricos finitos e incompatibilidades de tipos,
  incluindo respostas de erro pelo endpoint.
- [x] Documentar tipos e exemplos de escapes entre expressão, DOT e JSON.

Aprendizado: distinguir texto, literal, referência a variável e valor tipado.

Conclusão: AC06 e AC10 verificados, cobertura de tipos em AC11 e AC12,
e documentação conforme AC18.

### R3 — executar políticas com etapas intermediárias

GitHub: #5. Tipo: AFK após R0. Bloqueada por: R2.

- [x] Percorrer DAGs com múltiplas etapas, validando ausência de ciclos
  antes de iniciar o percurso.
- [x] Aplicar atribuições em start e nos demais nós, permitindo sobrescrita
  de valor e tipo e seu uso nas condições seguintes.
- [x] Seguir somente a primeira saída verdadeira na ordem do DOT e encerrar
  em folha ou quando nenhuma saída for verdadeira.
- [x] Verificar isolamento de estado entre requisições.
- [x] Demonstrar o comportamento pelo endpoint e documentar uma política
  com entrada, estados intermediários e saída.

Aprendizado: arestas controlam o percurso; nós alteram o estado.

Conclusão: AC02, AC03, AC04, AC07 e AC13 verificados, com cobertura de ciclos
em AC08 e documentação conforme AC18.

### R4 — executar condições compostas com curto-circuito

GitHub: #6. Tipo: AFK após R0. Bloqueada por: R3.

- [x] Implementar &&, ||, parênteses e precedência acordada sobre a AST restrita.
- [x] Verificar que variáveis ausentes só geram erro quando acessadas.
- [x] Preservar erros à esquerda, sem neutralizá-los pelo lado direito.
- [x] Garantir que falhas após atribuições não devolvam output parcial.
- [x] Executar os exemplos completos do README pelo endpoint.
- [x] Documentar agrupamento, precedência e exemplos e contraexemplos
  de curto-circuito.

Aprendizado: a árvore da expressão determina a ordem de avaliação.

Conclusão: AC01 e AC05 verificados, erro sem resultado parcial em AC12
e documentação conforme AC18.

### R5 — rejeitar políticas e requisições inválidas de forma consistente

GitHub: #7. Tipo: AFK após R0. Bloqueada por: R4.

- [x] Completar a validação de toda a política, inclusive trechos não percorridos.
- [x] Rejeitar falta de start, cond ausente ou vazio e construções DOT proibidas.
- [x] Rejeitar funções, acesso a campos, aritmética e demais construções
  não pertencentes à linguagem.
- [x] Rejeitar objetos, listas e null como valores de variáveis.
- [x] Completar os casos HTTP definidos em R0 e verificar códigos e mensagens.
- [x] Testar rejeições pelo endpoint e documentar exemplos e correções.

Aprendizado: distinguir política inválida de falha dependente da entrada.

Conclusão: AC08 e AC09 verificados, validação de tipos de entrada em AC11,
contrato de erros em AC12 e documentação conforme AC18. As proteções
necessárias aos incrementos anteriores já existem; esta etapa consolida
e amplia a validação, não adia sua implementação inteira.

### R6 — aplicar limites e tornar falhas observáveis

GitHub: #8. Tipo: AFK após R0. Bloqueada por: R5.

- [x] Aplicar os cinco limites configuráveis aprovados, com proteções
  anteriores ao parsing quando possível.
- [x] Testar valores nos limites e imediatamente acima deles.
- [x] Verificar 413 para corpo excessivo e demais respostas previstas.
- [x] Registrar duração e falhas sem despejar política ou variáveis nos logs.
- [x] Documentar configuração, valores padrão e respostas esperadas.

Aprendizado: limitar trabalho e diagnosticar falhas sem expor dados recebidos.

Conclusão: limites em AC11, respostas correspondentes em AC12 e AC18
verificados pelo fluxo integrado.

### R7 — medir desempenho e verificar a entrega reproduzível

GitHub: #9. Tipo: HITL. Bloqueada por: R6.

- [x] Aprovar cenários representativos, duração e critérios de aquecimento:
  política média, 10 s de aquecimento, 60 s de medição e 50 RPS.
- [x] Executar testes e análise estática e implementar benchmark com parsing,
  validação e execução.
- [x] Ofertar 50 RPS no teste HTTP e registrar taxa efetivamente alcançada,
  P90, erros, ambiente e políticas utilizadas.
- [x] Comparar resultados à meta, investigar desvios e discutir gargalos.
- [x] Documentar comandos, resultados e limites da comparação entre motor,
  transporte, emulação local e AWS.
- [x] Reproduzir as instruções desde uma árvore limpa, registrando
  pré-requisitos e bloqueios.
- [x] Revisar README e guias acumulados, preservando o enunciado,
  identificando limitações e sem apresentar trabalho pendente como pronto.

Aprendizado: distinguir o que uma medição comprova do que não comprova.

Evidência: `go test -bench . -benchmem ./internal/policy` mediu 54,864 ns/op
em `Parse`, 1,202 ns/op em `Infer` e 54,908 ns/op em `Parse + Infer` na
política média (Windows amd64, AMD Ryzen 5 5500). Uma repetição durante a
remoção de containers Docker do teste SAM saturado registrou 95,060 ns/op,
2,036 ns/op e 105,768 ns/op, evidenciando ruído ambiental.

Carga HTTP: `POLICY_INFER_LOAD_TEST=1 go test -run TestHTTPLoadMediumPolicy
-v ./internal/httpapi` executou 10 s de aquecimento e 60 s de medição a
50 RPS. Resultado: 3.000 requisições, 49,9995 RPS, 3.000 HTTP 200, zero
erros, P50 1,093 ms, P90 2,885 ms, P99 9,701 ms e máximo 58,118 ms. A meta
P90 < 30 ms foi atendida nesse transporte HTTP local.

A carga via `sam local start-api` padrão não produziu medida válida: cada
invocação criou um container `inferfunction:rapid-x86_64`, saturou o Docker
com cerca de 1.900 containers e resultou em timeouts/502. O custo do SAM é
tratado separadamente; para futura comparação usar `--warm-containers
EAGER`. Não há medição na AWS.

Conclusão: AC15, AC16 e AC17 têm evidência reproduzível e AC18 foi atendido
ao longo dos incrementos. Não acrescentar cache ou outras otimizações
sem medição e discussão, nem alegar medição AWS não realizada.

### Verificação prevista

Após existir um módulo Go:

```text
go test ./...
go vet ./...
go test -bench . -benchmem ./...
```

Os comandos de SAM, build do container e carga serão registrados com os
arquivos e cenários efetivamente criados. Não tratar comandos previstos como
já executados. Commits, push, criação de issues e publicação AWS dependem
de solicitação específica.

## 11. Referências técnicas

- [DOT Language](https://graphviz.org/doc/info/lang.html)
- [Gonum: parser DOT](https://github.com/gonum/gonum/blob/master/graph/formats/dot/dot.go)
- [go/parser](https://pkg.go.dev/go/parser)
- [gographviz: atributos](https://github.com/awalterschulze/gographviz/blob/main/attr.go)
- [Expr: linguagem](https://expr-lang.org/docs/language-definition)
- [CEL: semântica de erros e estado parcial](https://github.com/google/cel-go/blob/master/README.md)
- [AWS: Go em imagens de container Lambda](https://docs.aws.amazon.com/lambda/latest/dg/go-image.html)

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


# Inferência de políticas

Uma política descreve decisões e alterações de variáveis ao longo de um percurso.
A inferência determina o resultado desse percurso para uma entrada concreta.

## Linguagem

**Política**:
Conjunto de regras de decisão representado por um grafo direcionado sem ciclos.

**Nó da política**:
Ponto do percurso que pode conter atribuições de variáveis.

**Aresta da política**:
Transição entre dois nós, permitida por uma condição.

**Condição**:
Expressão booleana de uma aresta, avaliada sobre o estado atual da execução.

**Atribuição**:
Definição ou substituição de uma variável por um valor literal.

**Estado da execução**:
Conjunto das variáveis de entrada com as alterações acumuladas no percurso.

**Inferência**:
Execução de um único caminho da política a partir de seu nó inicial.

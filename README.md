# spy

Monitor de sistema para o terminal. CPU, memória, processos, ordenação e árvore de
processos — tudo consolidado em uma tela só.

```
  spy   up 1d 15h   ·   12 cores   ·   524 procs, 6 running                       00:53:47

  core  [▅▄▄▃ ▂▃▅▄ ▃▅▂▃]
  CPU   [▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ · · · · · · · · · · · · · · ·]   40%     load 1.80  1.41  1.07

  MEM   [▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ ▇ · · · · · · · · · · · ·]   51%     used 15.9G of 31.2G
  SWP   [· · · · · · · · · · · · · · · · · · · · · · · ·]    0%     used swap disabled

      PID USER      S CPU%▼  MEM%    RSS     TIME COMMAND
  1499706 will      S 117.5   2.2   698M     0:08 /opt/google/chrome/chrome --type=render…
   996680 will      R  94.9   2.5   814M    15:01 /opt/google/chrome/chrome
   271282 will      S  13.6   2.2   693M    20:12 /usr/bin/warp-terminal
      357 root      D   9.0   0.0     0B     0:31 [jbd2/nvme0n1p3-8]
     5118 will      S   9.0   2.0   635M    52:17 /usr/bin/gnome-shell
     2210 postgres  S   0.3   5.4   1.7G     4:05 postgres: checkpointer
      918 will      S   0.0   0.1    32M     0:02 zsh

  ↑↓ move · c/m/p/n sort · / filter · t tree · i info · x kill · q quit           sort cpu
```


## Instalação

Requer Go 1.24+ e Linux (a leitura é feita direto em `/proc`).

```sh
go install github.com/curruwilla/spy/cmd/spy@latest
```

Ou a partir do repositório:

```sh
make build      # gera bin/spy
make install    # instala em $GOPATH/bin
```

## Uso

```sh
spy                      # padrão: atualiza a cada 2s, ordenado por CPU
spy -i 500ms             # intervalo de atualização
spy -sort mem            # cpu, mem, pid, name ou time
spy -tree                # abre já em modo árvore
spy -filter chrome       # abre já filtrado por texto
spy -min-cpu 5           # só processos com pelo menos 5% de CPU
spy -min-mem 500M        # pelo menos 500 MB residentes (ou -min-mem 2 para 2% da RAM)
spy -min-time 1m30s      # pelo menos 1m30s de CPU acumulada (90 = 90s)
spy -min-cpu 5 -min-mem 2 -min-time 1m
spy -version
```

## Teclas

| Tecla | Ação |
| --- | --- |
| `↑` `↓` / `j` `k` | move o cursor |
| `PgUp` `PgDn` / `g` `G` | página / início e fim |
| `c` `m` `p` `n` | ordena por CPU, memória, PID, nome |
| `Tab` / `Shift+Tab` | percorre as colunas (inclui TIME) |
| a mesma tecla de novo | inverte a direção |
| `t` | alterna lista plana ↔ árvore de processos |
| `/` | filtra por comando, usuário ou PID (aplica enquanto digita; `Esc` limpa) |
| `l` | limites mínimos de CPU, memória e tempo (`Enter` aplica; `Esc` limpa) |
| `x` | envia SIGTERM ao processo selecionado, com confirmação `y/N` |
| `q` / `Esc` / `Ctrl+C` | sai |

### Limites mínimos

O `l` (de limite) abre um campo onde se escreve o piso de cada medida — o que está abaixo dele some
da tabela:

```
cpu>5              pelo menos 5% de um núcleo
mem>2              pelo menos 2% da RAM total
mem>500M           ou pelo menos 500 MB residentes (K, M, G, T, P e B)
time>1m30s         pelo menos 1m30s de CPU acumulada (90 = 90 segundos)
cpu>5 mem>500M     vários limites de uma vez, todos precisam ser atendidos
```

O sinal é decoração: `cpu>5`, `cpu>=5`, `cpu 5` e `cpu=5` são a mesma coisa, "pelo menos
5". As duas formas de dizer memória são alternativas — a última escrita substitui a
outra. O campo já abre preenchido com o que está ativo, para editar em vez de redigitar,
e o texto do `/` continua valendo em conjunto: os dois filtros se somam.

O painel do `i` fica preso no PID que estava sob o cursor na hora que você apertou:
a lista continua se reordenando atrás dele, mas o painel não troca de processo — só
os números dele se atualizam. Se o processo terminar, o painel fecha e o rodapé diz
qual PID saiu.

O cursor fica na posição, não no processo: ele permanece na linha onde você o deixou,
e uma atualização da lista não o arrasta atrás do processo que estava selecionado —
só o traz de volta se a lista encurtar e aquela linha deixar de existir. Já ao
ordenar, a tela volta para a primeira linha — a ideia é ver quem está no topo agora.
No modo árvore, um filtro mantém também os processos-pai do que casou, para a
hierarquia continuar legível — vale tanto para o texto quanto para os limites.

## Lendo a tela

Nem tudo na lista merece a mesma atenção, então a linha diz sozinha o que é o quê:

| Marca | Significa |
| --- | --- |
| linha inteira apagada | thread do kernel (filha do `kthreadd`, pid 2) — encanamento da máquina, nunca é o que você procura |
| dono em destaque | processo da sua conta; os das outras contas ficam em cinza |
| comando em negrito | está fazendo alguma coisa agora: rodando, ≥ 1% de CPU ou ≥ 5% da RAM |
| `S` colorido | `R` verde está num núcleo, `D` amarelo travado em syscall, `T` parado, `Z` vermelho é zumbi |
| `CPU%` colorido | verde até 50%, amarelo até 80%, vermelho acima |

As barras de CPU, memória e swap usam a mesma escala de cor; as três linhas de fundo
preenchido (título, cabeçalho da tabela e rodapé) seguem o tema claro ou escuro do
terminal.

## Como funciona

| Dado | Origem |
| --- | --- |
| CPU total e por núcleo | `/proc/stat`, diferença entre duas leituras |
| Memória e swap | `/proc/meminfo` (`MemAvailable` quando existe) |
| Load e nº de processos | `/proc/loadavg` |
| Uptime | `/proc/uptime` |
| Processos | `/proc/<pid>/stat`, `/proc/<pid>/cmdline`, dono pelo uid do diretório |

Os contadores de `/proc` são cumulativos, então a primeira tela mostra 0% de CPU: as
porcentagens só existem a partir da segunda leitura. `CPU%` é relativa a um núcleo, como
no `htop` — um processo com várias threads pode passar de 100%.

## Desenvolvimento

```sh
make test     # go test -race ./...
make cover    # cobertura total
make lint     # go vet + golangci-lint, se instalado
make help     # lista os alvos
```

Os parsers são testados com fixtures em `internal/proc/testdata/proc`, sem depender da
máquina onde o teste roda.

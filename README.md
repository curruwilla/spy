# spy

Monitor de sistema para o terminal. CPU, memória, processos, ordenação e árvore de
processos — tudo consolidado em uma tela só.

```
spy  up 1d 15h  ·  12 cores  ·  524 procs, 6 running                             00:53:47
CPU  ████████████░░░░░░░░░░░░░░░░░░  40%  load 1.80 1.41 1.07
core ▅▄▄▃▃▃▅▄▃▅▃▃
MEM  ███████████████░░░░░░░░░░░░░░░  51%  15.9G / 31.2G
SWP  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  disabled
    PID USER      S CPU%▼  MEM%    RSS     TIME COMMAND
1499706 will      S 117.5   2.2   698M     0:08 /opt/google/chrome/chrome --type=renderer
 996680 will      S  94.9   2.5   814M    15:01 /opt/google/chrome/chrome
 271282 will      S  13.6   2.2   693M    20:12 /usr/bin/warp-terminal
    357 root      S   9.0   0.0     0B     0:31 [jbd2/nvme0n1p3-8]
   5118 will      S   9.0   2.0   635M    52:17 /usr/bin/gnome-shell
↑↓ move · c/m/p/n sort · tab column · t tree · / filter · x kill · q quit         sort cpu
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
spy -filter chrome       # abre já filtrado
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
| `x` | envia SIGTERM ao processo selecionado, com confirmação `y/N` |
| `q` / `Esc` / `Ctrl+C` | sai |

O cursor acompanha o processo, não a posição: reordenar ou atualizar a lista não muda
a seleção. No modo árvore, um filtro mantém também os processos-pai do que casou, para
a hierarquia continuar legível.

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

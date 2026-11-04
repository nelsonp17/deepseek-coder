# Compilar deepseek-coder

Guía de compilación del CLI. Todo se compila con Go; lo único extra es un script
de PowerShell que estampa la versión y genera las tres plataformas de una vez.

## Requisitos

- **Go 1.26+** ([descargar](https://go.dev/dl/)). Comprobar con `go version`.
- Nada más. Las dependencias se bajan solas con `go mod download` (o en la
  primera compilación).
- Para la versión estampada hace falta **PowerShell 7+** (`pwsh`), porque el
  script usa `#Requires -Version 7`.

## Compilación rápida

```sh
cd D:\Proyectos\mio\deepseek-coder
go build -o deepseek.exe ./cli
.\deepseek.exe
```

`./cli` es el paquete `main`. Tarda unos segundos la primera vez (compila los
16 paquetes internos) y bastante menos después, por la caché de Go.

## Compilar una versión

Un binario sin estampar se identifica como `dev`:

```
$ .\deepseek.exe --version
deepseek dev (windows/amd64, go1.26.1)
```

Para que diga qué versión es, se inyecta en la compilación.

### Con el script (recomendado)

```sh
.\build.ps1 -Version v0.1.0          # solo el binario de esta máquina
.\build.ps1 -Version v0.1.0 -All     # + Linux/amd64 + macOS/arm64
```

Salida:

```
-> windows/amd64  deepseek.exe
-> linux/amd64    deepseek-linux-amd64
-> darwin/arm64   deepseek-macos-arm64

deepseek.exe                        9,8 MB
deepseek-linux-amd64                9,8 MB
deepseek-macos-arm64                9,8 MB

Version estampada: v0.1.0
```

El script rechaza versiones con espacios y aborta si `go build` falla, así que
nunca deja un `dist/` a medias.

### A mano

Equivalente exacto, si prefieres no usar el script:

```sh
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist\deepseek.exe ./cli
```

- `-X main.version=v0.1.0` estampa la versión (la variable vive en `cli/version.go`).
- `-s -w` quita símbolos de depuración y tabla de símbolos.
- `-trimpath` quita rutas absolutas del binario (builds reproducibles).

```
$ .\dist\deepseek.exe --version
deepseek v0.1.0 (windows/amd64, go1.26.1)
```

### Tamaños de referencia

| Build | Tamaño |
|---|---|
| `go build` a secas | 13,8 MB |
| `-trimpath -ldflags "-s -w"` | **9,8 MB** (−29 %) |

## Cross-compilación

Los tres objetivos habituales, con el script:

```sh
.\build.ps1 -Version v0.1.0 -All
```

O con los comandos de siempre (en PowerShell, `GOOS`/`GOARCH` como variables de
entorno):

```sh
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist\deepseek.exe ./cli
$env:GOOS="linux";   $env:GOARCH="amd64"; go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist\deepseek-linux-amd64 ./cli
$env:GOOS="darwin";  $env:GOARCH="arm64"; go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist\deepseek-macos-arm64 ./cli
```

En Linux/macOS es la forma corta de siempre:

```sh
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist/deepseek-linux ./cli
```

Los binarios son **estáticos**: no necesitan Go instalado en la máquina de
destino.

## Verificar la build

```sh
.\dist\deepseek.exe --version     # versión + plataforma + toolchain
.\dist\deepseek.exe help          # ayuda completa
.\dist\deepseek.exe config get    # api_key, model, lang, context, sandbox...
go vet ./...                      # sin salida = limpio
go test ./...                     # 17 paquetes
```

Una comprobación rápida de que el binario funciona de verdad (gasta tokens):

```sh
.\dist\deepseek.exe ask "Responde solo: compilado"
```

## Primer arranque en otra máquina

```sh
deepseek login                              # guarda la key en el llavero del SO
# o: set DEEPSEEK_API_KEY=sk-...
deepseek config set model deepseek-flash    # requiere conectividad
deepseek
```

El binario no trae ninguna key: la configuración vive en el perfil del usuario
(`%APPDATA%\deepseek-coder\config.json` y el llavero), no al lado del ejecutable.

## Problemas comunes

**El binario dice `dev`.** Es un `go build` sin `-X main.version=...`. Compila
con el script o pasa el `-ldflags` a mano.

**Mezcla de binarios viejos en `dist\`.** Si has compilado con nombres
distintos, convive un `deepseek-windows-amd64.exe` antiguo con el `deepseek.exe`
nuevo. Borra `dist\` y recompila:

```sh
Remove-Item dist\* -Force
.\build.ps1 -Version v0.1.0 -All
```

**`aviso: no puedo guardar el historial... Acceso denegado`.** El proceso no
puede escribir en `%LOCALAPPDATA%\deepseek-coder\sessions`. La carpeta suele
estar bien (comprueba que existe y que tu usuario tiene control total); lo
normal es que el CLI se esté ejecutando dentro de un entorno restringido que solo
permite escribir en el proyecto y en temporales. Arranca el binario desde una
terminal normal. Mientras tanto el chat funciona, pero sin guardar sesión: no
habrá `--continue`, `--resume` ni `session_search`.

**`go: no matching versions for query "1.26"`.** El `go.mod` pide Go 1.26. Sube
el toolchain o actualiza Go.

## Notas

- **El proyecto se llamaba `deekseep`** (error tipográfico: la `k` y la `s`
  cambiadas). Ahora es `deepseek` en el módulo, el binario, las carpetas de datos
  y el llavero. El CLI migra solo los datos al arrancar: mueve
  `%APPDATA%\deekseep-coder` y `%LOCALAPPDATA%\deekseep-coder` a
  `deepseek-coder`, y la API key se relee del nombre antiguo del llavero y se
  guarda con el nuevo. Si el movimiento falla, sigue usando el directorio viejo
  en vez de arrancar como si fueras un usuario nuevo.
- **La carpeta del repo puede seguir llamándose `deekseep-coder`.** No afecta a
  la compilación: el módulo es `deepseek-coder` y `go build ./cli` funciona desde
  cualquier ruta. Si quieres alinearla:
  `Rename-Item D:\Proyectos\mio\deekseep-coder deepseek-coder`.
- **`code/` está vacía.** La GUI del README es intención, no código. `go build
  ./...` compila el CLI y los paquetes internos; no hay un segundo binario.
- **`dist/` está en `.gitignore`.** Los binarios no van al repo. Si alguna vez
  aparecen rastreados (`git ls-files dist` los lista), sácalos del índice con
  `git rm --cached -r dist`.
- **`--version` no toca la red ni la config.** Se resuelve antes de cargar
  nada, así que funciona con un `config.json` roto o sin llavero.

# DeepSeek Coder

Un coder asistente que usa la API de **DeepSeek** para generar, revisar y editar código, con dos versiones:

- **`cli/`** — Interfaz de línea de comandos (CLI/TUI) para terminal.
- **`code/`** — Aplicación con interfaz gráfica (GUI) para escritorio.

Multiplataforma: **Windows, macOS y Linux**.

> El proyecto se llamaba `deekseep` (error tipográfico). Ahora es **deepseek** en
> el módulo, el binario y las carpetas de datos; el CLI migra solo lo que ya
> tenías al arrancar.

---

## ✨ Características

- Conversación con DeepSeek (modelos `deepseek-chat` y `deepseek-reasoner`) vía API oficial.
- Soporte de **streaming** de respuestas (token a token).
- **Idioma configurable** (español/inglés): la interfaz y las respuestas del modelo se adaptan a tu preferencia.
- **Primer arranque guiado**: si no hay API key ni idioma configurados, el CLI te los pregunta interactivamente.
- **Modelos dinámicos**: consulta la lista de modelos disponible en la API (`deepseek models`) y cambia de modelo al vuelo dentro del chat (`/model <id>`).
- **Validación de modelo al arrancar**: si el modelo configurado ya no está disponible en la API, te avisa y te muestra una lista seleccionable para cambiarlo.
- **Contexto del proyecto**: el CLI accede a la carpeta donde se ejecuta y carga `AGENTS.md`, `DESIGN.md`, `README.md`, skills (`.agents/skills`) y el estado de git (branch, status, diff, commits) para dárselos al modelo.
- **Editor multilínea**: `Ctrl+J` inserta un salto de línea, `Enter` envía el mensaje.
- **Diccionario integrado**: `/help` documenta comandos, atajos, skills y herramientas MCP; `/help <tema>` abre la ficha de uno concreto.
- **Markdown renderizado**: las respuestas del modelo se muestran con formato (títulos, negritas, código, listas, tablas, citas y enlaces) en vez del Markdown en crudo.
- **Adjuntar archivos**: escribe `@ruta` y el modelo recibe el contenido real del archivo, con límites de tamaño y filtro de binarios.
- **Modo `--pipe`**: mete contenido por tubería (`go test ./... 2>&1 | deepseek --pipe "..."`) para usarlo en scripts y CI.
- **Memoria entre sesiones**: `session_search` busca en tus conversaciones anteriores, así el modelo puede retomar lo de ayer.
- **Lista de tareas y modo plan**: el modelo anota lo que va a hacer, y puedes exigirle que planee en solo lectura antes de tocar nada.
- **Trabajos en segundo plano**: un build largo no bloquea el chat; el resultado llega solo.
- **Confinamiento**: modo `read-only`/`workspace-write`/`full`, independiente de si pide permiso, y comandos destructivos que siempre preguntan.
- **Compactación**: cuando el historial llena la ventana, lo antiguo se resume y se sigue; el log no pierde nada.
- **Control de consumo**: tokens de entrada/salida por petición y de toda la sesión, contexto frente a la ventana del modelo, y saldo de la cuenta con `deepseek balance`.
- **Contexto ajustable**: `config set context light|off` recorta el README y el diff que se envían en cada turno (~3 470 tokens menos en este repo).
- **Soporte MCP**: conecta servidores MCP (herramientas externas) y el modelo puede usarlas **con tu permiso explícito** antes de ejecutar nada.
- **Autocompletado**: escribe `@` para completar rutas de archivo y `/` para completar comandos, skills y herramientas MCP (con Tab).
- **TUI interactiva**: interfaz de terminal a pantalla completa con historial scrolleable, colores, spinner de carga, entrada multilínea y barra de ayuda.
- **Scroll y copia cómodos**: puedes seleccionar texto con el cursor en cualquier momento (la TUI no captura el ratón), la rueda y `PgUp`/`PgDn` hacen scroll, y `Ctrl+Y` / `/copy` / `/save` copian respuestas largas sin seleccionarlas.
- **Shell nativo**: el modelo ejecuta comandos reales con PowerShell (Windows) o el shell POSIX del sistema (macOS/Linux), con timeout, límite de salida y permiso explícito por orden.
- **Herramientas de fichero**: `read_file`, `write_file`, `edit_file`, `glob` y `grep`, con política de rutas y guard de leer antes de editar.
- Contexto de archivos del proyecto: puedes adjuntar archivos o carpetas a la conversación.
- Historial de conversaciones guardado localmente.
- Configuración de API key, modelo, temperatura y otros parámetros.
- Dos interfaces sobre la misma lógica core:
  - CLI interactiva con resaltado de sintaxis.
  - GUI de escritorio con editor de código integrado.

---

## 🧭 Decisión de stack: ¿Go, Rust, Python o Node.js?

Como el cuello de botella de esta herramienta es la **red** (llamar a la API), la velocidad real no depende del lenguaje de ejecución, sino de:

1. **Tiempo de arranque** del binario (crítico en CLI).
2. **Binario único** y fácil de distribuir.
3. **Cross-compilación** trivial para Windows/macOS/Linux.
4. **Ecosistema TUI/GUI** maduro.

### Comparativa

| Criterio | 🟢 Go | 🦀 Rust | 🟡 Node.js/TS | 🐍 Python |
|---|---|---|---|---|
| Arranque CLI | ⚡ Instantáneo | ⚡ Instantáneo | 🐢 50–200 ms+ | 🐢 200 ms+ |
| Binario único | ✅ | ✅ | ⚠️ Requiere Bun/sea | ❌ PyInstaller |
| Cross-compilación | ✅ Excelente | ✅ Excelente | ⚠️ Complicada | ❌ Frágil |
| TUI | Bubble Tea, Lip Gloss | ratatui | Ink | Textual |
| GUI | Wails, Fyne | Tauri, egui | Electron (pesado) | Tkinter, PyQt |
| Curva de aprendizaje | Baja-media | Alta | Media | Baja |
| Velocidad de desarrollo | Alta | Media | Alta | Muy alta |
| Memoria | Baja | Muy baja | Media-alta | Alta |

### Recomendación

**Go** para ambas versiones, con esta división:

- **CLI (`cli/`)** → Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) (TUI) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) (estilos) + [Cobra](https://github.com/spf13/cobra) (comandos).
- **GUI (`code/`)** → Go + [Wails](https://wails.io/) (webview nativo del sistema, binario pequeño) o [Fyne](https://fyne.io/) (puro Go, sin webview).

**Por qué Go:**

- Un solo binario estático por plataforma, sin dependencias de runtime.
- Arranque instantáneo: la CLI responde al instante, que es lo que percibe el usuario como "rapidez".
- Cross-compilación en un comando: `GOOS=windows GOARCH=amd64 go build` (y lo mismo para macOS/Linux).
- Ecosistema TUI de primera clase (Charm) y GUI ligera con Wails.
- Un solo lenguaje para CLI + GUI → la lógica de la API se comparte sin duplicar.

**Alternativa si priorizas máximo rendimiento:** Rust para el CLI (ratatui) + Tauri para la GUI. El resultado es marginalmente más rápido y ligero, pero el coste de desarrollo y la curva de aprendizaje son bastante mayores.

> **Evita Python y Node para este caso:** el arranque lento y la distribución (empaquetado, runtime) chocan directamente con el requisito de "super rápida" y multiplataforma.

---

## 📁 Estructura del proyecto

```
deepseek-coder/
├── cli/                    # Versión CLI/TUI
│   ├── commands.go         # Registro de comandos (fuente única: ayuda + autocompletado)
│   └── shell_tool_test.go  # Integración del bucle de herramientas
├── code/                   # Versión GUI (Go + Wails/Fyne)
├── internal/
│   ├── attach/             # Adjuntos @ruta (límites, binarios, fences)
│   ├── clipboard/          # Portapapeles (nativo + OSC 52)
│   ├── compact/            # Planificación de la compactación del historial
│   ├── config/             # Configuración + llavero del SO
│   ├── deepseek/           # Cliente de la API (streaming, modelos)
│   ├── editor/             # Editor multilínea (Ctrl+J / Enter)
│   ├── fstools/            # read/write/edit/glob/grep con política de rutas
│   ├── i18n/               # Español / English
│   ├── jobs/               # Comandos en segundo plano con aviso inyectado
│   ├── markdown/           # Renderizador Markdown a ANSI para el chat
│   ├── mcpx/               # Cliente MCP (servidores y herramientas)
│   ├── project/            # Contexto del proyecto (AGENTS, DESIGN, README, skills, git)
│   ├── session/            # Log durable de la conversación (JSONL append-only)
│   ├── shell/              # Shell nativo por SO (PowerShell / sh) con timeout
│   ├── todo/               # Lista de tareas del modelo
│   └── tokens/             # Medición de contexto (anclada al usage real)
└── README.md
```

---

## 🚀 Compilar y ejecutar

Requisitos: **Go 1.26+** ([descargar](https://go.dev/dl/)).

### CLI

```sh
# Compilar
cd deepseek-coder
go build -o deepseek ./cli        # Linux/macOS
go build -o deepseek.exe ./cli    # Windows

# Ejecutar
go run ./cli                      # o ./deepseek tras compilar
```

Para una versión concreta, estampa el número en el binario:

```sh
./build.ps1 -Version v0.1.0          # solo esta máquina
./build.ps1 -Version v0.1.0 -All     # Windows + Linux + macOS
```

El script compila con `-trimpath -ldflags "-s -w -X main.version=..."` y deja los binarios en `dist/`. A mano es lo mismo:

```sh
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist/deepseek ./cli
```

```console
$ deepseek --version
deepseek v0.1.0 (windows/amd64, go1.26.1)
```

Sin `-X` el binario se identifica como `dev`, para que nunca se confunda un build de trabajo con una release.

### Cross-compilación (un binario por plataforma)

Lo más simple es el script:

```sh
./build.ps1 -Version v0.1.0 -All
```

Y si prefieres los comandos a mano:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist/deepseek-windows.exe ./cli
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist/deepseek-macos ./cli
GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o dist/deepseek-linux ./cli
```

Son binarios estáticos: no necesitan Go instalado en la máquina de destino.

### GUI

> Pendiente: se completará cuando exista la primera versión.

---

## 💬 Uso

```sh
# Configurar la API key (una sola vez)
deepseek config set api_key sk-xxxx

# Consulta de una sola vez
deepseek ask "escribe un hola mundo en Go"

# Adjuntar archivos al mensaje
deepseek ask "revisa @cli/main.go por bugs"

# Meter contenido por tubería
go test ./... 2>&1 | deepseek --pipe "resuelve estos errores"

# Chat interactivo con streaming
deepseek
```

Comandos disponibles:

| Comando | Descripción |
|---|---|
| `deepseek` | Chat interactivo en terminal |
| `deepseek ask "..."` | Consulta de una sola vez (admite `@ruta`) |
| `deepseek --pipe [pregunta]` | Lee la entrada estándar y la envía al modelo |
| `deepseek --json ask "..."` | Igual que `ask` pero en NDJSON, para scripts y CI |
| `deepseek -c` / `--continue` | Continúa la última sesión guardada |
| `deepseek --resume <id>` | Reanuda una sesión concreta |
| `deepseek config get` | Muestra la configuración (key enmascarada) |
| `deepseek config set <key> <valor>` | Configura `api_key`, `model`, `base_url` o `lang` |
| `deepseek config unset <key>` | Elimina un valor de configuración |
| `deepseek config set lang es\|en` | Cambia el idioma de la interfaz |
| `deepseek history` | Lista las sesiones guardadas |
| `deepseek history show <id>` | Muestra una sesión |
| `deepseek history search <texto>` | Busca en todas las conversaciones guardadas |
| `deepseek history delete <id>` | Elimina una sesión |
| `deepseek models` | Lista los modelos disponibles en la API |
| `deepseek balance` | Consulta el saldo de la cuenta en la API |
| `deepseek --version` | Muestra la versión compilada y la plataforma |
| `deepseek help` | Ayuda |

Dentro del chat interactivo puedes usar comandos de sesión:

| Comando | Descripción |
|---|---|
| `/models [id]` | Cambia y guarda el modelo por defecto (sin argumento abre el selector) |
| `/list-models` | Lista los modelos disponibles |
| `/model [id]` | Alias de `/models` |
| `/thinking [on\|off]` | Consulta o cambia el modo de razonamiento |
| `/effort [low\|high\|max]` | Consulta o cambia el esfuerzo de razonamiento |
| `/tokens` | Consumo de tokens de la sesión |
| `/compact` | Compacta el historial antiguo en un resumen |
| `/balance` | Saldo de la cuenta |
| `/login` | Configura las credenciales (API key) |
| `/mcp` | Muestra el estado de los servidores MCP |
| `/context` | Muestra el contexto del proyecto cargado |
| `/context reload` | Recarga el contexto del proyecto |
| `/shell` | Muestra el shell nativo detectado y su configuración |
| `/run <orden>` | Ejecuta una orden en el shell nativo y muestra la salida |
| `/copy` | Copia la última respuesta al portapapeles |
| `/copy all` | Copia toda la conversación al portapapeles |
| `/save [ruta]` | Guarda la conversación en un archivo Markdown |
| `/approvals [ask\|auto]` | Consulta o cambia si se pide permiso en cada herramienta |
| `/sandbox [modo]` | Consulta o cambia el confinamiento (`read-only`, `workspace-write`, `full`) |
| `/plan` | Activa o desactiva el modo plan (solo lectura hasta que apruebes) |
| `/todos` | Muestra la lista de tareas del modelo |
| `/jobs` | Muestra los trabajos en segundo plano |
| `/help [tema]` | Catálogo completo, o la ficha de un comando/skill/herramienta MCP |
| `/exit` | Sale del chat |

> 💡 **Entrada multilínea:** dentro del chat, `Enter` envía el mensaje y `Ctrl+J` inserta un salto de línea (para mensajes largos o código). Se usa `Ctrl+J` y no `Alt+Enter` porque Windows Terminal reserva `Alt+Enter` para pantalla completa; `Shift+Enter` tampoco sirve, porque el terminal lo entrega como un `Enter` normal y el mensaje se enviaría. `Alt+Enter` sigue aceptándose por si tu terminal lo entrega.

> 💡 **Autocompletado:** escribe `@` y verás sugerencias de rutas de archivo; escribe `/` y verás comandos, skills y herramientas MCP. Pulsa **Tab** para completar.

> 💡 **Skills:** escribir `/nombre-de-skill` en un mensaje **inyecta sus instrucciones** (`SKILL.md` sin el frontmatter, hasta 8 KB), no solo su nombre. También puedes referenciar una herramienta MCP con `/nombre-herramienta`.
>
> En el transcript verás **solo la referencia y lo que escribiste** (`[Skill: caveman]` · `full revisa esto`): el cuerpo de la skill viaja al modelo, no a tu pantalla.

> 💡 **TUI:** al ejecutar `deepseek` en una terminal interactiva se abre la interfaz a pantalla completa: historial scrolleable, mensajes con colores, spinner mientras el modelo responde, y barra de ayuda abajo. `Enter` envía, `Ctrl+J` nueva línea, `Tab` completa, `Ctrl+Y` copia la última respuesta, `PgUp`/`PgDn` hace scroll y `Ctrl+C` sale. Si la entrada no es una terminal (pipes/scripts), se usa automáticamente el modo de línea simple.

### 📖 El diccionario: `/help` y `/help <tema>`

`/help` sin argumentos documenta **todo lo que esta sesión tiene**: comandos,
atajos de teclado, skills del proyecto y servidores MCP con sus herramientas. No
es una lista fija: se genera de los datos vivos, así que si añades una skill o un
servidor MCP aparecen aquí solos.

```
Comandos
  Sesión
    /help [tema]            catálogo completo o ficha de un tema
    /exit                   sale del chat
  ...

Atajos de teclado
  Enter         enviar
  Ctrl+J        nueva línea
  Tab           completar
  Ctrl+Y        copiar
  PgUp/PgDn     scroll
  Ctrl+C        salir

Skills del proyecto
  /caveman        Ultra-compressed communication mode. Cuts token usage ~75%...

Servidores MCP
  filesystem
    read_file         Read a file from disk
    list_directory    List a directory
```

Con un tema, `/help` te da **su ficha**: uso, alias, subcomandos y una
descripción larga.

```
> /help copy
/copy — copia la última respuesta al portapapeles

Uso:     /copy

Copia al portapapeles. Sin argumento copia la última respuesta; con 'all' copia
la conversación completa en texto plano, sin colores ni códigos ANSI. Usa el
portapapeles nativo y si no está disponible cae a OSC 52, así que también
funciona por SSH.

Subcomandos:
  /copy all        copia toda la conversación al portapapeles
```

Funciona con cualquier cosa documentada: `/help run`, `/help save`,
`/help caveman` (una skill) o `/help read_file` (una herramienta MCP). Además,
escribir `/help ` y pulsar **Tab** autocompleta los temas.

### ✨ Markdown en las respuestas

Las respuestas del modelo se renderizan antes de mostrarse: no ves `**esto**`
ni `` `aquello` ``, ves el texto con formato. El renderizador es propio (sin
dependencias ni resaltado por lenguaje, para no engordar el binario) y cubre lo
que un modelo usa de verdad:

| Elemento | Cómo se ve |
|---|---|
| Títulos `#`–`######` | En negrita y color, con subrayado en el nivel 1 |
| `**negrita**`, `*cursiva*`, `~~tachado~~` | Aplicados |
| Código en línea `` `x` `` | En ámbar, con los backticks quitados |
| Bloques ```` ```lang ```` | En caja, con la etiqueta del lenguaje y conservando la indentación |
| Listas `-`, `*`, `1.` | Viñetas `•` y numeración, con anidamiento y casillas `☐`/`☑` |
| Citas `>` | Barra lateral en cursiva |
| Tablas | Columnas alineadas que se encogen para caber en el ancho |
| Enlaces `[texto](url)` | Texto subrayado seguido de la URL |
| `---` | Línea horizontal |
| Párrafos | Reajustados al ancho de la ventana, respetando acentos y emojis |

Dos detalles que importan en un chat con streaming:

- **El texto se reajusta al redimensionar** la ventana (se vuelve a renderizar).
- **Un bloque a medio llegar no rompe nada**: si el modelo aún no ha cerrado un
  ```` ``` ````, se muestra lo que ya llegó.
- **El scroll no se resiente**: cada mensaje se renderiza una vez y se cachea,
  así que una respuesta larga no obliga a reprocesar todo el historial en cada
  token.

El Markdown **no** se aplica a lo que imprime el propio CLI (avisos, `/help`,
salida de comandos), que ya viene formateado.

### 🖱️ Ratón: selección de texto siempre disponible

La TUI **no captura el ratón**. Eso significa que puedes seleccionar con el cursor
en cualquier momento —arrastrando en mitad de una respuesta, en el historial o en
el input— y copiar con `Ctrl+Shift+C` (o el clic derecho de siempre). No hace falta
pulsar `Shift` ni nada especial.

El scroll también funciona, porque la rueda llega al viewport como flechas:

- **Rueda del ratón** → hace scroll del chat. Solo actúa cuando el input está vacío, para no mover el cursor mientras escribes.
- **`PgUp` / `PgDn`** → scroll de página, siempre.
- **`Ctrl+Y`** → copia la última respuesta al portapapeles sin seleccionar nada.
- **`/copy all`** → copia toda la conversación en texto plano (sin colores ni códigos ANSI).
- **`/save`** → guarda la conversación en un `.md` para revisarla con total libertad.

El scroll **no te devuelve al final** mientras lees: si subes a revisar algo, los
mensajes nuevos que lleguen no moverán la vista. Solo vuelve al final al enviar
un mensaje, o si ya estabas abajo.

### 🐚 Shell nativo (la herramienta `run_shell`)

El modelo puede ejecutar comandos de verdad en el shell de tu sistema, sin
depender de ningún servidor MCP:

| Sistema | Shell que se usa |
|---|---|
| **Windows** | `pwsh` (PowerShell 7+) si está instalado; si no, `powershell.exe` |
| **macOS / Linux** | `$SHELL` si es ejecutable; si no, `/bin/bash` y luego `/bin/sh` |

Cada ejecución:

- Se lanza con `-NoProfile -NonInteractive -Command` (PowerShell) o `-c` (POSIX), en el directorio del proyecto.
- Tiene un **timeout** (2 min por defecto, configurable con `shell_timeout`) y al expirar **mata el árbol de procesos completo**, no solo el shell.
- Devuelve stdout y stderr mezclados, el **código de salida** y la duración.
- Su salida se **trunca a 32 KB** conservando el principio y el final (donde suele estar el error), así una compilación ruidosa no te revienta el contexto.
- **Pide permiso siempre** antes de ejecutar. Al aprobar se te muestra la orden real, no el JSON:

```
El modelo quiere ejecutar la herramienta run_shell:
$ go test ./...
atención: 'a' permite ejecutar CUALQUIER orden durante el resto de la sesión.
¿Permitir? [y]es / [n]o / [a]uto (sin más preguntas) / [d]eny esta herramienta:
```

`y` y `n` valen **solo para esa llamada**. `a` no es un permiso por herramienta:
**cambia la política de la sesión** y deja de preguntar por nada, así que la TUI
lo avisa en la cabecera (`⚠ sin confirmaciones`) y se revierte cuando quieras:

```sh
/approvals          # muestra el estado y las herramientas denegadas
/approvals ask      # vuelve a pedir permiso en cada llamada
/approvals auto     # desactiva las confirmaciones (peligroso)
```

Antes, `a` se recordaba por nombre de herramienta durante toda la sesión; como
solo hay una herramienta de shell, eso equivalía a «ejecuta cualquier comando
sin avisar hasta que cierres el CLI». Ahora es una política visible y
reversible, y `d` sigue denegando esa herramienta para el resto de la sesión.

En modo no interactivo (scripts, pipes) **la llamada se deniega por defecto**,
igual que el resto de herramientas.

El modelo también recibe en el system prompt el sistema operativo, el shell
detectado y el directorio de trabajo, para que no proponga comandos de bash en
Windows ni de PowerShell en Linux.

Puedes ejecutar órdenes tú mismo sin pasar por el modelo:

```sh
# Dentro del chat
/run git status
/run go build ./...
/shell          # muestra el shell detectado y su configuración
```

Y desactivar la herramienta por completo:

```sh
deepseek config set shell off
```

---

## 🗂️ Herramientas de fichero

El modelo lee y edita con herramientas nativas, no a través del shell: así no
depende del quoting de PowerShell, respeta la codificación y hay política de
rutas. Son cinco:

| Herramienta | Qué hace |
|---|---|
| `read_file` | Devuelve el archivo con **números de línea**, `offset`/`limit` y aviso de continuación. Rechaza binarios, directorios y ficheros de más de 5 MB |
| `write_file` | Crea o reemplaza por completo, con escritura **atómica** (temporal + rename) |
| `edit_file` | Reemplazo literal; exige que el texto aparezca **una sola vez** (o `replace_all`) |
| `glob` | Busca ficheros por patrón, con `*`, `?` y `**`; ordena por fecha y salta `node_modules`, `vendor`, `dist`… |
| `grep` | Busca por regexp y devuelve `archivo:línea: texto`; salta binarios y ficheros grandes |
| `session_search` | Busca en conversaciones anteriores y devuelve extractos con su id |

### Dos garantías que el shell no daba

**No puedes editar lo que no has leído.** `write_file` y `edit_file` exigen que
el fichero se haya leído antes en la sesión (`read the file before modifying it`)
y que **no haya cambiado desde entonces** (`the file changed since it was read`).
Eso evita que el modelo sobrescriba contenido que nunca vio y que pise un cambio
que hiciste tú entre medias.

**No puedes salir del proyecto.** Toda ruta se resuelve contra el directorio
donde arrancaste el CLI y se comprueba que siga dentro; `../fuera.txt` y las
rutas absolutas de fuera se rechazan, y los symlinks se resuelven antes de
comparar.

### Permisos

Leer no es un efecto secundario, así que **`read_file`, `glob` y `grep` no piden
permiso**: pedirlo por cada lectura haría el chat insufrible. Escribir sí:

```
El modelo quiere ejecutar la herramienta edit_file:
cli/main.go
- return oldValue
+ return newValue

¿Permitir? [y]es / [n]o / [a]uto (sin más preguntas) / [d]eny esta herramienta:
```

El preview muestra el cambio (líneas `-`/`+`), no el JSON de argumentos.

> 💡 **Coste en tokens:** cada esquema de herramienta viaja en cada petición. Los
> cinco añaden **~700 tokens** de entrada por turno (medido con `--json`: de 4 193 a
> 4 904 con la misma pregunta). Es el precio de que el modelo sepa usarlas;
> `/tokens` te lo mide.

---

## 📎 Adjuntar archivos con `@`

Escribe `@` seguido de una ruta y el CLI adjunta **el contenido** del archivo, no
solo el nombre. Funciona en el chat interactivo, en `ask` y en `--pipe`, y el
autocompletado de `@` sigue igual (escribe `@` y pulsa **Tab**).

```sh
deepseek ask "revisa @cli/main.go por bugs"
```

Dentro del chat:

```
▸ tú
compara @cli/commands.go con @cli/help.go

📎 cli/commands.go (2.1 KB) · cli/help.go (4.0 KB)
```

Reglas que aplica:

| Caso | Comportamiento |
|---|---|
| Archivo normal | Se adjunta con su lenguaje (fence `go`, `markdown`, `json`…) |
| Mayor de 32 KB | Se omite y se avisa del tamaño real |
| Binario (NUL en los primeros 8 KB) | Se omite: nada de `.exe`, `.db` ni imágenes en el contexto |
| Directorio | Se omite con aviso |
| No existe | Se omite con aviso |
| Más de 64 KB entre todos | El último se recorta y el resto se omite por presupuesto |
| Repetido (`@a.go` y `@./a.go`) | Se adjunta una sola vez |
| Ruta con espacios | `@"mi archivo.go"` o `@'mi archivo.go'` |

Dos decisiones de diseño que importan:

- **Tu mensaje no se reescribe.** Los archivos van en una sección aparte al
  final, así tus instrucciones se leen enteras y la referencia `@ruta` sigue
  visible para el modelo.
- **El contenido solo viaja a la API.** En el historial, el portapapeles y el
  log del chat queda una línea de resumen. Si no, cada mensaje guardaría 30 KB
  en el log de sesión y `/copy all` te escupiría el archivo entero.

El fence se ajusta al contenido: si adjuntas un `README.md` lleno de ```` ``` ````,
el bloque usa cuatro backticks y no se rompe.

---

## 🔀 Modo `--pipe`

Para meter contenido por tubería, sin abrir el chat ni tocar el portapapeles:

```sh
cat main.go | deepseek --pipe "busca fugas de recursos"

go test ./... 2>&1 | deepseek --pipe "resuelve estos errores de compilación"

git diff | deepseek --pipe "resume estos cambios en 5 líneas"

deepseek --pipe "explica esto" < notas.txt
```

- La pregunta se puede combinar con `stdin`: se envía primero tu pregunta y luego
  el contenido, etiquetado con su tamaño.
- Si `stdin` es una terminal (no hay tubería), **no se lee** — así
  `deepseek --pipe "pregunta"` escrito a mano no se queda colgado.
- La respuesta va a **stdout** en texto plano y los avisos a **stderr**, así que
  puedes encadenarlo.
- Sale con código ≠ 0 si la API falla, que es lo que necesita un CI.
- También acepta `@ruta`, por si quieres combinar tubería y archivos.

### Salida para scripts: `--json`

Con `--json`, `ask` y `--pipe` emiten **NDJSON** por stdout en vez de texto:

```sh
deepseek --json ask "Responde solo: ok"
```
```json
{"type":"start","model":"deepseek-chat"}
{"type":"delta","text":"ok"}
{"type":"usage","prompt_tokens":4193,"completion_tokens":2,"total_tokens":4195}
{"type":"done","content":"ok"}
```

Eventos: `start`, `delta` (por fragmento), `usage`, `done` y `error`. Los avisos
siguen yendo a stderr, así que `deepseek --json ... | jq` funciona sin
contaminación. El código de salida es `0` si terminó y `1` si falló.

---

## 📊 Consumo y saldo

### Tokens

Cada respuesta reporta su consumo real (lo da la API, no es una estimación).
`/tokens` muestra el detalle y el encabezado de la TUI lleva el acumulado de la
sesión (`↑` entrada, `↓` salida):

```
DeepSeek Coder · deepseek-chat · ↑12.3k ↓3.4k

> /tokens
Consumo de tokens:
  modelo:          deepseek-chat (esfuerzo high)
  última petición: 18 000 (↑ 16 000 · ↓ 2 000)
  contexto:        16 000 / 64 000 (25 %)
  sesión:          ↑ 24 500 · ↓ 5 100 · total 29 600
  caché de prompt: 12 800
```

- **contexto** es el `prompt_tokens` real de la última petición frente al
  `context_window` del modelo, así que ves cuánto margen te queda de verdad.
- **sesión** acumula todas las rondas: si el modelo encadena herramientas, cada
  ronda se factura y se suma.
- **caché de prompt** es lo que DeepSeek sirvió desde caché (más barato).

En `ask` y `--pipe` la línea de tokens se imprime por **stderr**, para no
ensuciar stdout si estás en una tubería.

> 💡 **Ojo con el contexto del proyecto.** Cada petición arrastra lo que el CLI
> carga del proyecto (README, `AGENTS.md`, skills y estado de git). En este repo
> eso son **~4 150 tokens de entrada incluso para un «hola»**, y se nota en la
> factura. Para eso está el modo de contexto.

### Modo de contexto

`context` decide cuánto del proyecto se envía en **cada** turno:

| Modo | Qué envía | Medido en este repo |
|---|---|---|
| `full` (por defecto) | Todo: `AGENTS.md`, `DESIGN.md`, `README.md`, skills y git con diff | 11.6 KB · **≈ 3 950 tokens** |
| `light` | Sin `README.md` ni `git diff`; mantiene instrucciones, skills, rama y commits | 1.4 KB · **≈ 490 tokens** |
| `off` | Nada de contexto de proyecto | 0 |

```sh
deepseek config set context light   # ahorra ~3 470 tokens por petición
deepseek config set context off     # lo mínimo, para consultas sueltas
deepseek config unset context       # vuelve a full
```

`/context` te enseña el desglose real y cuánto ahorrarías:

```
> /context
Contexto del proyecto: (modo: full)
  encabezado                  70 B  ≈ 23
  README.md                 6.1 KB  ≈ 2 070
  skills                     745 B  ≈ 248
  git                       4.7 KB  ≈ 1 615
  total                    11.6 KB  ≈ 3 956
  ahorrarías con 'light': 10.2 KB (≈ 3 468 tokens)
```

Los dos grandes son el **README** y el **diff** de git; todo lo demás (rama,
commits, skills) es barato y se mantiene en `light`. El cambio se aplica al
instante: también puedes usar `/context reload` tras editar el modo.

### Saldo

```sh
deepseek balance
```

```
Saldo de la cuenta: disponible
  CNY  total 110.00  (recargado 100.00 · regalo 10.00)
```

Dentro del chat: `/balance`. No consume tokens.

---

## 🕒 Fecha y hora

El modelo no tiene reloj: si le preguntas el día, lo deduce de su entrenamiento
y se equivoca. Por eso el system prompt incluye siempre la fecha, la hora y el
día de la semana reales del equipo, con la instrucción de usarlos tal cual:

```
- Fecha y hora actual: 2026-11-02 13:40:07 -04:00 (Monday)
- Si te preguntan la fecha, la hora o el día de la semana, usa ese valor tal
  cual: no lo calcules ni lo deduzcas de tu entrenamiento.
```

Así no depende de ejecutar `Get-Date` ni de calcular nada.

---

## 🔌 Servidores MCP

El CLI puede conectarse a servidores MCP (Model Context Protocol) para darle al modelo herramientas externas: acceso a archivos, git, APIs, bases de datos, etc.

### Configuración

Copia `mcp.example.json` a tu directorio de configuración como `mcp.json`:

- **Windows:** `%APPDATA%\deepseek-coder\mcp.json`
- **macOS:** `~/Library/Application Support/deepseek-coder/mcp.json`
- **Linux:** `~/.config/deepseek-coder/mcp.json`

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
    }
  ]
}
```

Cada servidor se lanza como subproceso (`command` + `args`) y se comunica por stdio. Puedes añadir variables de entorno con `env`.

### Permisos

**Nada se ejecuta sin tu aprobación.** Cuando el modelo quiere llamar una herramienta, el CLI te muestra qué herramienta y con qué argumentos, y te pregunta:

```
El modelo quiere ejecutar la herramienta read_file:
{"path": "./main.go"}
¿Permitir? [y]es / [n]o / [a]lways / [d]eny:
```

- `y` — permite una vez
- `n` — deniega una vez
- `a` — permite siempre (para esta sesión)
- `d` — deniega siempre (para esta sesión)

Si no hay terminal interactiva (scripts/pipes), **la llamada se deniega por defecto**: nunca se fuerza una ejecución que el usuario no pudo aprobar.

### Comandos

| Comando | Descripción |
|---|---|
| `deepseek mcp` | Muestra los servidores MCP configurados y sus herramientas |
| `/mcp` (en el chat) | Muestra el estado de los servidores MCP de la sesión |

> 💡 En Windows o máquinas compartidas, prefiere la variable de entorno `DEEPSEEK_API_KEY` en lugar de guardar la key en el archivo de config (ver [Seguridad](#-seguridad)).

---

## 🚀 Primer arranque

La primera vez que ejecutes `deepseek` (o cualquier comando) sin configuración, el CLI te preguntará:

1. **Idioma** — selecciona Español o English (la interfaz y las respuestas del modelo se adaptan).
2. **API key** — la introduces una sola vez (oculta mientras escribes) y se guarda en el llavero del sistema.
3. **Modelo por defecto** — consulta los modelos disponibles en la API y te muestra una lista numerada para elegir cuál usar.

En cada sesión se usa el último modelo seleccionado. Si el modelo configurado ya no está disponible (p. ej. lo retiraron de la API), el CLI te lo avisa y te vuelve a mostrar la lista para elegir otro.

También puedes configurarlo manualmente:

```sh
deepseek config set lang es        # o 'en'
deepseek config set api_key sk-xxxx
deepseek config set model deepseek-reasoner
```

---

## ⚙️ Configuración

Obtén tu API key en [platform.deepseek.com](https://platform.deepseek.com) y configúrala:

```sh
# CLI
deepseek config set api_key sk-xxxx
export DEEPSEEK_API_KEY=sk-xxxx   # o variable de entorno

# GUI
# Se configura desde el panel de ajustes de la aplicación
```

Variables de entorno soportadas:

| Variable | Descripción | Default |
|---|---|---|
| `DEEPSEEK_API_KEY` | Clave de la API | — |
| `DEEPSEEK_MODEL` | Modelo a usar | `deepseek-chat` |
| `DEEPSEEK_BASE_URL` | URL base de la API | `https://api.deepseek.com` |

Claves de configuración (`deepseek config set <clave> <valor>`):

| Clave | Descripción |
|---|---|
| `api_key` | API key de DeepSeek (se guarda en el llavero del SO) |
| `model` | Modelo a usar (`deepseek-chat` o `deepseek-reasoner`) |
| `base_url` | URL base de la API |
| `lang` | Idioma de la interfaz (`es` o `en`) |
| `thinking` | Modo de razonamiento (`enabled` o `disabled`) |
| `reasoning_effort` | Esfuerzo de razonamiento (`low`, `high` o `max`) |
| `shell` | Herramienta `run_shell` para el modelo (`on` por defecto, `off` para desactivarla) |
| `shell_timeout` | Timeout de las órdenes en segundos (por defecto `120`, máximo `600`) |
| `context` | Contexto del proyecto por petición: `full` (por defecto), `light` u `off` |
| `compact` | Compactación automática del historial (`auto` por defecto, `off`) |
| `sandbox` | `read-only`, `workspace-write` (por defecto) o `full` |
| `approval` | `ask` (por defecto) o `auto` |
| `context_window` | Ventana de contexto en tokens; manda sobre la que reporte la API |

---

## 🧩 Trabajo en varios pasos

### Lista de tareas

Cuando el trabajo lleva más de un paso, el modelo mantiene una lista con
`todo_write`. Se ve encima de la caja de entrada mientras trabaja y sobrevive a
un `--continue`:

```
Tareas: 1/3
● 1. Leer internal/api/server.go
◐ 2. Añadir el endpoint /health
○ 3. Ejecutar los tests
```

Solo puede haber **una tarea en curso**: un agente que marca tres cosas como
«haciendo» normalmente no está haciendo ninguna. `/todos` la muestra cuando
quieras.

### Modo plan

`/plan` deja al modelo en **solo lectura** hasta que apruebes lo que propone:

```sh
/plan
> refactoriza el manejo de errores del cliente
```

Investiga, y luego llama a `exit_plan_mode` con el plan. El prompt te lo enseña
y decides: si dices que sí, se desbloquean escritura y shell; si dices que no,
sigue en modo plan y puede proponer otro. Mientras está activo, cada intento de
escribir devuelve:

```
[plan mode: this tool is disabled until the plan is approved. Think it through and call exit_plan_mode with your plan.]
```

Aprobar un plan **siempre** pregunta, incluso con `approval auto`.

### Trabajos en segundo plano

Un build o una suite de tests largos no deben bloquear la conversación:

```
> compila y ejecuta los tests, y mientras revisa el README
Job 1 started in the background: go test ./...
```

Cuando termina, el resultado **se inyecta** en la conversación (no se sondea), y
en la TUI el modelo sigue solo si estabas inactivo:

```
[background job #1 finished (exit 0, 12.3s)]
$ go test ./...
ok  deepseek-coder/internal/...
```

`/jobs` lista los que siguen en marcha.

### Preguntas al usuario

Con `ask_user_question` el modelo propone opciones concretas en vez de adivinar.
En la TUI sale un selector (`1-9` elige, `↑↓` mueve, `Espacio` marca, `Enter`
confirma, `Esc` cancela). En modo línea ese canal no existe — la entrada la
controla el REPL — así que el modelo recibe un aviso y pregunta en texto plano,
que es exactamente lo que se ve en la captura de arriba.

---
## 🔒 Seguridad

### Dos perillas, no una

Permitir es una cosa; llegar lejos es otra. Están separadas a propósito:

| Perilla | Valores | Qué decide |
|---|---|---|
| `approval` | `ask` (por defecto) · `auto` | Si te pido permiso antes de cada herramienta |
| `sandbox` | `workspace-write` (por defecto) · `read-only` · `full` | Hasta dónde puedo llegar |

Combinarlas es lo útil: `approval auto` + `sandbox read-only` significa «no me
preguntes, pero no puedes escribir ni ejecutar nada». Al revés, `approval ask` +
`sandbox full` te deja ver cada paso pero sin límite de rutas.

| Modo | Herramientas de fichero | Shell |
|---|---|---|
| `read-only` | Solo lectura: `read_file`, `glob`, `grep` | **Denegado**: no puedo garantizar que no escriba |
| `workspace-write` | Lectura y escritura **dentro del proyecto** | Permitido, con `approval` |
| `full` | Sin confinamiento de rutas | Permitido, con `approval` |

**Fail-closed.** En `read-only` el shell se deniega entero en vez de filtrarlo:
en Windows no hay forma de confinar un proceso, y fingir que sí sería peor que
decir no. En Linux/macOS se podría envolver con `bwrap`/`sandbox-exec`; hoy no
está hecho y el README lo dice en vez de prometerlo.

Cuando una política bloquea una llamada, el modelo recibe un marcador que
reconoce, no un error genérico:

```
[sandbox: file access denied under read-only mode]
[sandbox: command execution denied under read-only mode]
```

### Comandos destructivos

Hay 25 patrones (`rm -rf /`, `mkfs`, `git push --force`, `format`, `del /f`…) y
una regla para descargar y ejecutar (`curl … | sh`). **Siempre piden
confirmación, incluso con `approval auto`**: es la red de seguridad de último
recurso, no un sandbox.

```sh
deepseek config set sandbox read-only
deepseek config set approval auto
deepseek config get
```

### API key

- **Por defecto se guarda en el llavero del sistema operativo** (cifrado gestionado por el SO):
  - **Windows:** Credential Manager
  - **macOS:** Keychain
  - **Linux:** Secret Service (GNOME Keyring / KWallet)
- Si no hay llavero disponible (p. ej. Linux sin servicio de secretos), se usa como *fallback* `config.json` en el directorio de configuración del usuario, con permisos `0600` en Linux/macOS.
- Precedencia de resolución: `DEEPSEEK_API_KEY` (env) → llavero del SO → `config.json`.
- La key nunca se muestra completa en pantalla ni se envía a ningún servidor que no sea `api.deepseek.com`.
- `deepseek config unset api_key` elimina la key del llavero y del archivo de config.

### Historial de chat

Cada conversación es un **log JSONL append-only**, una línea por hecho, en el
directorio de datos del usuario:

- **Windows:** `%LOCALAPPDATA%\deepseek-coder\sessions\<id>.jsonl`
- **macOS:** `~/Library/Application Support/deepseek-coder/sessions/<id>.jsonl`
- **Linux:** `~/.local/share/deepseek-coder/sessions/<id>.jsonl`

```json
{"seq":0,"time":"2026-11-02T14:39:39-04:00","type":"session/start","data":{...}}
{"seq":1,"time":"...","type":"user/message","data":{"content":"di solo: ok"}}
{"seq":2,"time":"...","type":"assistant/message","data":{"content":"ok","usage":{...}}}
{"seq":3,"time":"...","type":"turn/end","data":{"reason":"completed"}}
```

- El historial **no contiene la API key** (vive en el llavero, separado).
- Se consulta con `deepseek history show <id>` y se borra con `history delete <id>`.
- Un log cortado a mitad se **repara al abrirlo**: los `tool/result` que faltaban
  se rellenan y el turno abierto se cierra como `interrupted`.

### 🔁 Reanudar una conversación

```sh
deepseek -c                # continúa la última sesión
deepseek --resume <id>     # reanuda una concreta
```

El historial que ve el modelo **se deriva del log** en cada turno, no vive en
memoria: por eso reanudar es exactamente igual que no haber cerrado. Cada evento
lleva `seq` y `time`, así que una sesión también sirve de auditoría: puedes ver
qué se le envió al modelo y qué contestó, incluso los `tool/result`.

### Buscar en el pasado

```sh
deepseek history search health
2026-11-02 12:00  20261102-120000-bbbbbb  [user]  arreglamos el endpoint /health…
```

Y el modelo puede hacerlo solo con `session_search`, que además le devuelve el id
de la sesión para que la reabras con `--resume`:

```
> ¿cómo resolvimos el endpoint /health?
Ejecutando session_search...
[2026-11-02 12:00 · 20261102-120000-bbbbbb · user]
…arreglamos el endpoint /health usando time.Since…
```

Busca en las **50 sesiones más recientes**, en el contenido ya decodificado (no
en el JSON), ignorando mayúsculas y saltando líneas corruptas. Es de solo
lectura: no pide permiso.

### 🗜️ Compactación

Una conversación larga no cabe en la ventana del modelo. Cuando la presión de
contexto llega al umbral, el CLI **resume lo más antiguo y sigue**:

```
> /compact
compactando ~477 tokens del historial...
historial compactado: ≈1832 → ≈1617 tokens
```

- Se dispara solo al **80 % de la ventana** (o ventana − 8 KB, lo que sea menor)
  y conserva el **16 %** más reciente. `/compact` lo fuerza cuando quieras.
- Si la API devuelve un error de «contexto excedido», se compacta y se reintenta
  el mismo paso una vez, en vez de perder el turno.
- **Nada se borra.** Se añade un evento `compaction` con el resumen y el rango
  `from_seq`/`to_seq` que oculta; `history show` sigue mostrando la conversación
  tal y como la ve el modelo, y el log conserva todo lo original.

La ventana se toma de la API; si no la reporta, de `context_window`, y si no,
65 536 tokens. Lo que pongas en `context_window` **manda sobre la API**, para
poder forzar compactaciones tempranas:

```sh
deepseek config set context_window 32000
deepseek config set compact off      # desactiva la compactación automática
```

> 💡 `deepseek-tokens` mide con la cifra real que reporta la API y solo estima lo
> añadido desde entonces; `/tokens` te enseña la presión actual frente a la
> ventana.

---

## 🐛 Depuración

Si algo no funciona (p. ej. el chat se queda en "pensando..."), activa el logger para ver qué hace el cliente:

```sh
deepseek --debug          # o DEEPSEEK_DEBUG=1 deepseek
deepseek --debug ask "hola"
```

Registra en stderr cada petición a la API con su estado y duración:

```
[deepseek] client: baseURL=https://api.deepseek.com model=deepseek-chat apiKeySet=true
[deepseek] POST https://api.deepseek.com/chat/completions model=deepseek-chat messages=3 tools=0
[deepseek] POST /chat/completions -> 200 OK, streaming...
[deepseek] stream done after 2.3s: content=512 bytes toolCalls=0
```

En la TUI, el spinner muestra los **segundos transcurridos** mientras piensa y el **razonamiento en vivo** de los modelos de razonamiento (como `deepseek-v4-pro`), y si el modelo tarda más de 30s en responder aparece un aviso.

---

## 🛣️ Roadmap

- [x] Cliente HTTP de la API de DeepSeek (streaming + no streaming)
- [x] CLI básica: conversación interactiva en terminal
- [x] Historial durable de conversaciones (log JSONL append-only, `--continue` / `--resume`)
- [x] Modelos dinámicos (lista de la API + validación al arrancar)
- [x] Contexto del proyecto (AGENTS.md, DESIGN.md, README.md, skills, git)
- [x] Editor multilínea (Ctrl+J / Enter) + autocompletado (@ y /)
- [x] Soporte MCP (cliente de herramientas externas con permisos)
- [x] TUI interactiva (Bubble Tea)
- [x] Shell nativo por SO (`run_shell`): PowerShell / sh, permisos, timeout y truncado
- [x] Copiar sin seleccionar (`Ctrl+Y`, `/copy`, `/copy all`) y guardar (`/save`)
- [x] CLI avanzada: adjuntar archivos (`@ruta`) y modo `--pipe`
- [x] Herramientas de fichero nativas (`read_file`, `write_file`, `edit_file`, `glob`, `grep`) con «leer antes de editar» y política de rutas
- [x] Sesiones durables: `--continue`, `--resume`, `history show/search` y reparación de turnos interrumpidos
- [x] Compactación: poda de salida antigua, resumen por umbral, `/compact` y reintento si la API rechaza por contexto
- [x] Permisos de dos perillas (`approval` × `sandbox` read-only/workspace-write/full) y comandos destructivos que siempre preguntan
- [x] Trabajo en varios pasos: `todo_write` con panel, modo plan con `exit_plan_mode`, jobs en segundo plano y `ask_user_question`
- [x] `session_search`: buscar en conversaciones anteriores desde el chat y desde el CLI
- [ ] GUI: pedir carpeta al abrir + chat con editor de código
- [ ] Empaquetado y releases para Windows, macOS y Linux
- [ ] Tests y CI multiplataforma

---

## 📄 Licencia

MIT


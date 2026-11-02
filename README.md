# DeepSeek Coder

Un coder asistente que usa la API de **DeepSeek** para generar, revisar y editar código, con dos versiones:

- **`cli/`** — Interfaz de línea de comandos (CLI/TUI) para terminal.
- **`code/`** — Aplicación con interfaz gráfica (GUI) para escritorio.

Multiplataforma: **Windows, macOS y Linux**.

---

## ✨ Características

- Conversación con DeepSeek (modelos `deepseek-chat` y `deepseek-reasoner`) vía API oficial.
- Soporte de **streaming** de respuestas (token a token).
- **Idioma configurable** (español/inglés): la interfaz y las respuestas del modelo se adaptan a tu preferencia.
- **Primer arranque guiado**: si no hay API key ni idioma configurados, el CLI te los pregunta interactivamente.
- **Modelos dinámicos**: consulta la lista de modelos disponible en la API (`deekseep models`) y cambia de modelo al vuelo dentro del chat (`/model <id>`).
- **Validación de modelo al arrancar**: si el modelo configurado ya no está disponible en la API, te avisa y te muestra una lista seleccionable para cambiarlo.
- **Contexto del proyecto**: el CLI accede a la carpeta donde se ejecuta y carga `AGENTS.md`, `DESIGN.md`, `README.md`, skills (`.agents/skills`) y el estado de git (branch, status, diff, commits) para dárselos al modelo.
- **Editor multilínea**: `Alt+Enter` inserta un salto de línea, `Enter` envía el mensaje.
- **Soporte MCP**: conecta servidores MCP (herramientas externas) y el modelo puede usarlas **con tu permiso explícito** antes de ejecutar nada.
- **Autocompletado**: escribe `@` para completar rutas de archivo y `/` para completar comandos, skills y herramientas MCP (con Tab).
- **TUI interactiva**: interfaz de terminal a pantalla completa con historial scrolleable, colores, spinner de carga, entrada multilínea y barra de ayuda.
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
deekseep-coder/
├── cli/                    # Versión CLI/TUI
├── code/                   # Versión GUI (Go + Wails/Fyne)
├── internal/
│   ├── config/             # Configuración + llavero del SO
│   ├── deepseek/           # Cliente de la API (streaming, modelos)
│   ├── editor/             # Editor multilínea (Alt+Enter / Enter)
│   ├── history/            # Historial en SQLite
│   ├── i18n/               # Español / English
│   ├── mcpx/               # Cliente MCP (servidores y herramientas)
│   └── project/            # Contexto del proyecto (AGENTS, DESIGN, README, skills, git)
└── README.md
```

---

## 🚀 Compilar y ejecutar

Requisitos: **Go 1.26+** ([descargar](https://go.dev/dl/)).

### CLI

```sh
# Compilar
cd deekseep-coder
go build -o deekseep ./cli        # Linux/macOS
go build -o deekseep.exe ./cli    # Windows

# Ejecutar
go run ./cli                      # o ./deekseep tras compilar
```

### Cross-compilación (un binario por plataforma)

```sh
GOOS=windows GOARCH=amd64 go build -o deekseep-windows.exe ./cli
GOOS=darwin  GOARCH=arm64 go build -o deekseep-macos ./cli
GOOS=linux   GOARCH=amd64 go build -o deekseep-linux ./cli
```

### GUI

> Pendiente: se completará cuando exista la primera versión.

---

## 💬 Uso

```sh
# Configurar la API key (una sola vez)
deekseep config set api_key sk-xxxx

# Consulta de una sola vez
deekseep ask "escribe un hola mundo en Go"

# Chat interactivo con streaming
deekseep
```

Comandos disponibles:

| Comando | Descripción |
|---|---|
| `deekseep` | Chat interactivo en terminal |
| `deekseep ask "..."` | Consulta de una sola vez |
| `deekseep config get` | Muestra la configuración (key enmascarada) |
| `deekseep config set <key> <valor>` | Configura `api_key`, `model`, `base_url` o `lang` |
| `deekseep config unset <key>` | Elimina un valor de configuración |
| `deekseep config set lang es\|en` | Cambia el idioma de la interfaz |
| `deekseep history` | Lista las conversaciones guardadas |
| `deekseep history show <id>` | Muestra una conversación |
| `deekseep history delete <id>` | Elimina una conversación |
| `deekseep models` | Lista los modelos disponibles en la API |
| `deekseep help` | Ayuda |

Dentro del chat interactivo puedes usar comandos de sesión:

| Comando | Descripción |
|---|---|
| `/models` | Lista los modelos disponibles |
| `/model` | Muestra el modelo actual |
| `/model <id>` | Cambia y guarda el modelo por defecto |
| `/login` | Configura las credenciales (API key) |
| `/models [id]` | Cambia y guarda el modelo por defecto |
| `/list-models` | Lista los modelos disponibles |
| `/model [id]` | Alias de `/models` |
| `/mcp` | Muestra el estado de los servidores MCP |
| `/context` | Muestra el contexto del proyecto cargado |
| `/context reload` | Recarga el contexto del proyecto |
| `/help` | Ayuda de comandos de sesión |
| `/exit` | Sale del chat |

> 💡 **Entrada multilínea:** dentro del chat, `Enter` envía el mensaje y `Alt+Enter` inserta un salto de línea (para mensajes largos o código).

> 💡 **Autocompletado:** escribe `@` y verás sugerencias de rutas de archivo; escribe `/` y verás comandos, skills y herramientas MCP. Pulsa **Tab** para completar. También puedes escribir `/skill` o `/herramienta-mcp` para referenciar una skill o herramienta en tu mensaje.

> 💡 **TUI:** al ejecutar `deekseep` en una terminal interactiva se abre la interfaz a pantalla completa: historial scrolleable (PgUp/PgDn), mensajes con colores, spinner mientras el modelo responde, y barra de ayuda abajo. `Enter` envía, `Alt+Enter` nueva línea, `Tab` completa, `Ctrl+C` sale. Si la entrada no es una terminal (pipes/scripts), se usa automáticamente el modo de línea simple.

---

## 🔌 Servidores MCP

El CLI puede conectarse a servidores MCP (Model Context Protocol) para darle al modelo herramientas externas: acceso a archivos, git, APIs, bases de datos, etc.

### Configuración

Copia `mcp.example.json` a tu directorio de configuración como `mcp.json`:

- **Windows:** `%APPDATA%\deekseep-coder\mcp.json`
- **macOS:** `~/Library/Application Support/deekseep-coder/mcp.json`
- **Linux:** `~/.config/deekseep-coder/mcp.json`

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
| `deekseep mcp` | Muestra los servidores MCP configurados y sus herramientas |
| `/mcp` (en el chat) | Muestra el estado de los servidores MCP de la sesión |

> 💡 En Windows o máquinas compartidas, prefiere la variable de entorno `DEEPSEEK_API_KEY` en lugar de guardar la key en el archivo de config (ver [Seguridad](#-seguridad)).

---

## 🚀 Primer arranque

La primera vez que ejecutes `deekseep` (o cualquier comando) sin configuración, el CLI te preguntará:

1. **Idioma** — selecciona Español o English (la interfaz y las respuestas del modelo se adaptan).
2. **API key** — la introduces una sola vez (oculta mientras escribes) y se guarda en el llavero del sistema.
3. **Modelo por defecto** — consulta los modelos disponibles en la API y te muestra una lista numerada para elegir cuál usar.

En cada sesión se usa el último modelo seleccionado. Si el modelo configurado ya no está disponible (p. ej. lo retiraron de la API), el CLI te lo avisa y te vuelve a mostrar la lista para elegir otro.

También puedes configurarlo manualmente:

```sh
deekseep config set lang es        # o 'en'
deekseep config set api_key sk-xxxx
deekseep config set model deepseek-reasoner
```

---

## ⚙️ Configuración

Obtén tu API key en [platform.deepseek.com](https://platform.deepseek.com) y configúrala:

```sh
# CLI
deekseep config set api_key sk-xxxx
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

Claves de configuración (`deekseep config set <clave> <valor>`):

| Clave | Descripción |
|---|---|
| `api_key` | API key de DeepSeek (se guarda en el llavero del SO) |
| `model` | Modelo a usar (`deepseek-chat` o `deepseek-reasoner`) |
| `base_url` | URL base de la API |
| `lang` | Idioma de la interfaz (`es` o `en`) |

---

## 🔒 Seguridad

### API key

- **Por defecto se guarda en el llavero del sistema operativo** (cifrado gestionado por el SO):
  - **Windows:** Credential Manager
  - **macOS:** Keychain
  - **Linux:** Secret Service (GNOME Keyring / KWallet)
- Si no hay llavero disponible (p. ej. Linux sin servicio de secretos), se usa como *fallback* `config.json` en el directorio de configuración del usuario, con permisos `0600` en Linux/macOS.
- Precedencia de resolución: `DEEPSEEK_API_KEY` (env) → llavero del SO → `config.json`.
- La key nunca se muestra completa en pantalla ni se envía a ningún servidor que no sea `api.deepseek.com`.
- `deekseep config unset api_key` elimina la key del llavero y del archivo de config.

### Historial de chat

- Se guarda en una base de datos **SQLite** en el directorio de datos del usuario:
  - **Windows:** `%LOCALAPPDATA%\deekseep-coder\history.db`
  - **macOS:** `~/Library/Application Support/deekseep-coder/history.db`
  - **Linux:** `~/.local/share/deekseep-coder/history.db`
- El historial **no contiene la API key** (vive en el llavero, separado).
- Se puede consultar (`deekseep history show <id>`) y eliminar (`deekseep history delete <id>`) desde el CLI.

---

## 🐛 Depuración

Si algo no funciona (p. ej. el chat se queda en "pensando..."), activa el logger para ver qué hace el cliente:

```sh
deekseep --debug          # o DEEPSEEK_DEBUG=1 deekseep
deekseep --debug ask "hola"
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
- [x] Historial de conversaciones en SQLite
- [x] Modelos dinámicos (lista de la API + validación al arrancar)
- [x] Contexto del proyecto (AGENTS.md, DESIGN.md, README.md, skills, git)
- [x] Editor multilínea (Alt+Enter / Enter) + autocompletado (@ y /)
- [x] Soporte MCP (cliente de herramientas externas con permisos)
- [x] TUI interactiva (Bubble Tea)
- [ ] CLI avanzada: adjuntar archivos, modo `--pipe`
- [ ] GUI: pedir carpeta al abrir + chat con editor de código
- [ ] Empaquetado y releases para Windows, macOS y Linux
- [ ] Tests y CI multiplataforma

---

## 📄 Licencia

MIT

----

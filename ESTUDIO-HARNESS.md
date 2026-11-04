# Estudio comparativo: deepseek-coder vs. DeepSeek Harness

> Documento de trabajo. Analiza el código de referencia que hay en `deepseek-harness-master/`
> frente al proyecto Go de este repositorio, para decidir qué replicar, qué evitar y en qué orden.

---

## 1. Qué se ha analizado

| | `deepseek-coder` (tu proyecto) | `deepseek-harness-master` (referencia) |
|---|---|---|
| Lenguaje | Go 1.26, sin dependencias de runtime | TypeScript (ESM), Node ≥ 24.9 |
| Tamaño | 6 892 líneas de código + 2 537 de tests, 35 ficheros | 20 233 ficheros, 134 MB, 6 042 `.ts` + 821 `.tsx` |
| Arquitectura | Binario único, 12 paquetes internos, sin plugins | Monorepo de ~48 grupos de paquetes, todo es plugin sobre Cordis |
| Distribución | `go build` → un ejecutable estático por plataforma | `npx @deepseek-ai/dsh web`, perfiles + bundles + patches |
| Superficie | CLI/TUI de terminal | Web UI (principal), escritorio Electron, CLI headless, SDK TypeScript y Python, ACP |
| Herramientas del modelo | 1 nativa (`run_shell`) + las de MCP | 76 paquetes de herramientas documentados en `docs/tool-catalog.md` |
| Persistencia | SQLite para el historial de chat | Log de sesión append-only versionado, con migraciones y *forks* |

El estudio se ha hecho leyendo la documentación de arquitectura del propio Harness
(`docs/architecture.md`, `agent-lifecycle.md`, `tool-execution-pipeline.md`, `glossary.md`),
su catálogo de herramientas y de configuración, y el código de los paquetes que importan
para un agente de código de terminal.

**Límites, para que no te fíes de más:** esto es un estudio dirigido, no una auditoría
completa de 20 000 ficheros. He leído entero lo que se cita; no he ejecutado su test suite ni
su producto. Las cifras de coste en líneas de Go son estimaciones mías de ingeniería, no
mediciones. Y su propio README avisa de que están en *developer preview* con cambios
rompientes, así que parte de esto caducará: la arquitectura y las ideas aguantan, los detalles
de fichero y línea no.

---

## 2. Resumen ejecutivo

Cinco conclusiones que ordenan todo lo demás:

1. **Tu proyecto no es un clon pequeño: es otro producto.** El Harness es una plataforma
   extensible con Web UI; tú tienes un CLI de terminal de un solo binario. Competir en
   superficie es imposible y, sobre todo, no es lo que hace útil a un coder de terminal.
2. **La diferencia grande no está en las herramientas, está en el estado.** El Harness
   modela turno → paso → ronda, guarda *todo* en un log durable y de ahí deriva el contexto.
   Tú tienes un `[]Message` en memoria. Eso es lo que te impide tener *resume*, *fork*,
   compactación y auditoría.
3. **Las tres piezas con mejor relación valor/coste para ti son:** compactación de contexto,
   presupuesto de tokens con proyección del *siguiente* request, y un modelo de permisos de
   dos perillas (sandbox × aprobación) en vez de un `y/n/a/d` plano.
4. **Tienes cosas que el Harness no tiene y no debe perder:** Markdown renderizado en terminal,
   adjuntos `@ruta` resueltos en el prompt, `--pipe`, saldo de la cuenta en el chat y un
   diccionario `/help` generado. Eso es UX de terminal que el Harness resuelve en la Web UI.
5. **No copies el sistema de plugins, ni Cordis, ni los perfiles.** Es el 70 % de su
   complejidad y solo se amortiza si vendes una plataforma. Copia los *contratos*, no el
   andamiaje.

---

## 3. Las dos arquitecturas en una página

### 3.1 Cómo funciona deepseek-coder

```
main.go ──► app{cfg, msg, proj, mcp, perm}
              │
              ├─ repl()      → replTUI() (Bubble Tea) | replLine() (stdin)
              ├─ ask()       → una pregunta, streaming a stdout
              ├─ chatTurn()  → bucle for de 10 rondas máx:
              │                  StreamChat → tool_calls → confirmTool → executeTool → repetir
              ├─ confirmTool → y / n / a(lways) / d(eny), por nombre de herramienta
              ├─ executeTool → run_shell nativo | servidor MCP
              └─ systemPrompt = system + contexto del proyecto + entorno
```

Estado: `messages []deepseek.Message` en memoria (y en SQLite, como historial de solo
lectura). Herramientas: registro implícito (lista construida en `allTools()`).
Extensión: añadir código Go.

### 3.2 Cómo funciona DeepSeek Harness

```
dsh (perfil) → árbol de plugins Cordis (todo es plugin: adapter LLM, tools, sesión, bucle)
                 │
                 ├─ ctx.llm        adapter de proveedor (deepseek, pi-ai, ...)
                 ├─ ctx.tools      registro con scopes por agente
                 ├─ ctx.sessions   log append-only de SessionEvent
                 ├─ ctx.systemPrompt   ensamblado por secciones y esquemas
                 ├─ ctx.agents / ctx.agentLoop   bucle turn/step
                 ├─ ctx.sandbox    confinamiento de procesos
                 ├─ ctx.approval   aprobación humana puntual
                 ├─ ctx.compaction resumen + reemplazo de tramo
                 ├─ ctx.tokenMeter presión de contexto y precios por nodo
                 └─ ctx.commands   comandos humanos (/, sin turno del modelo)
```

Ciclo de un turno (resumido de `docs/agent-lifecycle.md`):

```
turn/start → claim de entrada → ensamblar prompt + tools admitidas
  → agent/pre-step (waterfall: aceptar, reescribir o rechazar)
  → step/start → agent/request → prepareCall(adapter)
  → persistir system/message, user/message, request/header
  → derivar historial del log  ← el log es la fuente de verdad
  → llm/stream (con retry y agent/request-error)
  → assistant/message (con el stream compacto embebido)
  → tool/call → tools/pre-execute → guards → approval → tools/execute
               → tools/post-execute → tool/result
  → step/end → ¿tools piden otra petición o llegó input? → siguiente paso
turn/end
```

La regla que lo explica todo: **«model-visible significa registrado»**. Nada llega al modelo
que no se pueda reconstruir del log.

---

## 4. Lo que ya tienes y está bien

No es una lista de consolación: son cosas que un clon no trae y que cuestan.

| Capacidad | Dónde | Por qué importa |
|---|---|---|
| Shell nativo por SO con timeout, kill-tree y truncado de salida | `internal/shell/` | El Harness lo resuelve con `dsh-tool-bash`/`pwsh` + sandbox; tú lo tienes en 471 líneas |
| Permiso explícito por llamada con `always`/`deny` de sesión | `confirmTool` | Equivalente funcional a `ctx.approval`, aunque con menos granularidad |
| Markdown renderizado en terminal | `internal/markdown/` (860 líneas) | El Harness lo hace en la Web UI; en terminal es tuyo y no lo tiene |
| Adjuntos `@ruta` con presupuesto, binarios y fences dinámicos | `internal/attach/` | El Harness usa `attachment`/`spill`; tu versión es más simple pero resuelve el caso real |
| Contexto de proyecto con modo `full`/`light`/`off` y medición | `loadProject`, `/context` | El Harness lo hace con `compaction` + `token-meter`; tu versión es el 20 % que da el 80 % |
| Tokens por petición y de sesión, con la ventana del modelo | `/tokens` | El Harness tiene `token-meter`; tú ya tienes lo esencial |
| Saldo de la cuenta | `deepseek balance` | El Harness lo tiene en `llm-deepseek-account`, pero en la UI |
| `--pipe` para scripts y CI | `pipeQuestion` | El Harness lo cubre con `headless` |
| Diccionario `/help` generado de datos vivos | `cli/help.go` | El Harness tiene `ctx.commands` + UI; tu catálogo en terminal es mejor que nada |
| Bilingüe es/en de verdad | `internal/i18n/` (903 líneas) | El Harness tiene i18n maduro; el tuyo cubre todo el producto |

**Diagnóstico:** tienes un coder de terminal *completo para una persona*. Lo que falta no es
cantidad de herramientas: es **gestión de estado y de contexto**.

---

## 5. El hueco real, subsistema por subsistema

### 5.1 Herramientas: 1 contra 69

El Harness documenta **69 nombres de herramienta distintos** en 76 paquetes. Tú tienes una
(`run_shell`) más las de MCP. La tabla completa está en el apéndice A; lo que importa es el
reparto por categoría y qué de eso te sirve de verdad:

| Categoría | Harness | Tú | Veredicto |
|---|---|---|---|
| Shell / terminal | `bash`, `pwsh`, persistentes, `terminal_*` (PTY) | `run_shell` | **Bien cubierto.** Tú ya tienes el nativo por SO con timeout y kill-tree |
| Ficheros | `read`, `write`, `edit`, `apply_patch`, `str_replace_editor`, `read_image` | — (el modelo hace shell) | **Hueco crítico** |
| Búsqueda | `glob`, `grep` | — (el modelo hace shell) | **Hueco crítico** |
| Planificación | `todo_write`, `exit_plan_mode`, goal (`create/get/update_goal`), `schedule_*` | — | Hueco alto en sesiones largas |
| Delegación | `subagent`, `subagent_fork`, `workflow`, `ralph`, Agent Teams | — | Hueco medio (caro) |
| Memoria | `session_search`, `session_event_*` | — | Hueco alto, barato |
| Web | `web_search`, `web_fetch` | — | Hueco medio, barato |
| Código | `lsp` (definición, referencias, hover) | — | Hueco medio, caro en Go |
| Interacción | `ask_user_question`, `present` | permiso `y/n/a/d` | Hueco bajo |
| MCP | `*_mcp_resource*` | herramientas, sin recursos | Hueco bajo |
| Varios | `load_workspace_dependencies`, `plugin_*`, browser, computer-use | — | No aplica |

**Por qué `read`/`write`/`edit`/`glob`/`grep` son el hueco más caro:** hoy todo pasa por
`run_shell`, así que cada lectura de fichero depende de que el modelo acierte con el quoting
del shell (PowerShell en tu caso, distinto de bash), con la codificación y con el paginado.
En Windows eso falla de forma intermitente y el modelo no lo ve venir. Además, sin `fs/*` no
hay política por ruta ni la garantía de **leer antes de editar** que el Harness impone como
guard.

### 5.2 Cómo se ejecutan las herramientas (lo que enseña su pipeline)

Su pipeline (`packages/core/tools/src/pipeline.ts`) es más interesante que el catálogo. Cinco
ideas que puedes copiar sin su arquitectura:

1. **Valor canónico separado del texto visible.** La herramienta devuelve un valor
   estructurado y un `render` aparte. Reemplazar lo que ve el modelo no destruye el valor
   para los consumidores programáticos.
2. **Dos niveles de intercepción.** Un *waterfall* reordenable para allow/deny/ask y unos
   *guards* monótonos que **solo pueden denegar**. Así, reordenar listeners no puede convertir
   un deny en permiso. En tu código, `confirmTool` mezcla las dos cosas.
3. **Concurrencia declarada por herramienta y fail-closed.** Solo lo que se declara
   explícitamente seguro corre en paralelo; lo demás va en barrera de orden. Tú ejecutas
   siempre secuencial (correcto pero lento con lotes de lecturas).
4. **Resultados sintéticos al abortar.** Si se cancela a mitad de un lote, las llamadas que no
   llegaron a empezar igualmente reciben su `tool/result`, para que el historial siga siendo
   válido. Tú, si el usuario pulsa Ctrl+C en mitad de un lote, dejas el historial roto (tool
   calls sin resultado).
5. **`additionalContexts`.** En vez de reescribir el resultado de una herramienta para añadir
   contexto, se encola y se inyecta como mensaje de usuario después de los resultados, en
   orden. Es el mecanismo limpio para avisos tipo «llevas 3 llamadas iguales».

---

### 5.3 Estado y sesión: ahí está la frontera

Esta es la diferencia que hace que su producto y el tuyo sean especies distintas, no
variantes de lo mismo.

| Capacidad | Harness | Tú | Veredicto |
|---|---|---|---|
| Fuente de verdad | Log append-only de `SessionEvent`, con `seq` contiguo y `surfaceOp` (`append` / `replace`) | `[]Message` en memoria; SQLite guarda **solo para mostrar** | Hueco estructural |
| Historial del modelo | Se **deriva** del log (`deriveMessages`, con caché y mensajes congelados) | Es la propia lista en memoria | Consecuencia del anterior |
| Reanudar una sesión | `open(id)` + cierre sintético del turno interrumpido + header `resume` | No existe | Hueco alto |
| Bifurcar (fork) | Copia del prefijo + `session/end-seed` | No existe | Hueco medio |
| Reparar tool calls huérfanas | Resultados sintéticos (`TOOL_OUTCOME_UNKNOWN`, `TOOL_NOT_STARTED`, `ABORTED_BEFORE_DISPATCH`) | Si el turno falla, quedan `tool_calls` sin `tool` en el historial | Hueco bajo, barato |
| Auditoría / replay | Todo evento durable, con migraciones de formato | Nada | Hueco medio |
| Buscar en sesiones | Herramientas `session_search`, `session_event_*` | `deepseek history` (listar y borrar) | Hueco medio |

**Por qué esto importa aunque tú no quieras un log:** sin un registro de eventos no puedes
hacer **compactación** (necesitas «ocultar» un tramo sin destruirlo), ni **resume**, ni
**fork**, ni saber qué se le envió al modelo en el turno 3. Ese es el precio real de tu `[]Message`.

**Recorte recomendado para Go:** no necesitas 13 tipos de evento. Con un JSONL por sesión y
cinco eventos (`message`, `tool_call`, `tool_result`, `compaction`, `turn_end`) tienes el 80 %
del valor. La clave es que el fichero sea **append-only** y que el historial se derive de él.

---

### 5.4 Contexto: tu punto más débil (y el más caro de arreglar)

| Pieza | Harness (`token-meter` + `compaction`) | Tú | Veredicto |
|---|---|---|---|
| Medir el contexto | `pressureTokens` (real de la API) + `projectedTokens` (**lo que costará el siguiente request**) + precio por nodo | `lastUsage` y totales de sesión | Bien, pero sin proyección |
| Recortar el contexto estático | No aplica: su contexto es dinámico | Modo `full`/`light`/`off` medido | **Mejor que ellos en esto** |
| Compactación automática | Dispara en `floor(min(W·0,8, W−O−B))` con `B=64 KiB`; retiene el 16 % literal | No existe | **Hueco crítico** |
| Recuperación de «contexto excedido» | `agent/request-error` → una reducción máxima → retry en el mismo paso | La petición falla y el turno muere | Hueco crítico |
| Podar resultados de herramientas antes de resumir | `tool-result pruner` | No existe | Hueco medio, barato |
| System prompt por secciones con orden | `section({name, order, text})` + interpolar variables | Una cadena concatenada | Hueco bajo |
| Contexto en caliente | Se emite como `user/message` **reemplazable** in situ | Se calcula una vez al arrancar | **Bug concreto** |

**El bug concreto:** en tu código el system prompt se construye una sola vez
(`tui.go:192`, `main.go:1051`). Si una sesión cruza la medianoche, el modelo sigue viendo la
fecha de ayer — exactamente el fallo del día de la semana que acabamos de arreglar, pero con
una ventana de 24 h. También significa que `/context reload` recarga `a.proj` pero **no
actualiza el prompt ya enviado**.

**Cómo lo resuelve el Harness y por qué está bien pensado:** en lugar de un `system` fijo,
emite un mensaje de usuario con el contexto de runtime y, cuando el texto cambia, **lo
reemplaza en su sitio** (`RuntimeContextProjection`). Así el prefijo de la conversación se
mantiene estable (no rompe la caché de prefijo) y aun así el modelo ve datos frescos.

**Arreglo barato para ti, en este orden:**
1. Reconstruir el mensaje `system` en cada turno (o sustituir su contenido) antes de enviar.
   Coste: ~20 líneas. Arregla la fecha congelada y hace que `/context reload` sirva de algo.
2. Añadir `projectedTokens` = `lastUsage.PromptTokens` + estimación de lo añadido desde
   entonces. Es lo que te dice «la próxima petición va a costar X».
3. Compactación por umbral: al 80 % de la ventana, resumir el tramo antiguo con el propio
   modelo y sustituirlo por un mensaje de resumen. Coste real: 600–900 líneas y una decisión
   de diseño (¿dónde cortas? ¿arrastras resultados de tools?).

---

### 5.5 Permisos y confinamiento: tu modelo es más simple y más peligroso

| Pieza | Harness | Tú | Veredicto |
|---|---|---|---|
| Decisión | 3 capas: `pre-execute` (allow/deny/ask) → `approval` one-shot → guards que **solo deniegan** | `y/n/a/d` por nombre de herramienta | Simplista |
| Memoria de la decisión | **Ninguna**: solo `allowed-once`. Lo durable es la *política* (`ask`/`never`) y el *modo* (`read-only`/`workspace-write`/`danger-full-access`) | `a` = permitir esa herramienta **para toda la sesión**; `d` = denegar | Peligroso |
| Granularidad | Por herramienta y llamada; por ruta vía sandbox | Por herramienta | Insuficiente |
| Sin humano delante | `never` rechaza determinísticamente; «sin answerer = deny» | Deniega si `stdin` no es interactivo | Bien |
| Confinamiento | Envuelve el argv (bwrap/landlock, Seatbelt, ACL en Windows); **fail-closed** | Ninguno | Hueco alto |
| Leer antes de escribir | Guard `fs-observation-policy` (`FS_NOT_OBSERVED`) | No aplica (no hay tool de escritura) | — |

**Lo más peligroso de tu diseño:** `a` (always) se recuerda **por nombre de herramienta**
durante toda la sesión. Como solo tienes `run_shell`, responder `a` una vez equivale a
«ejecuta cualquier comando sin preguntar hasta que cierres el CLI». El Harness no permite eso
por diseño: su aprobación es siempre puntual, y lo que se recuerda es la *política*, no el
permiso.

**Lo barato que puedes copiar ya:**
- Cambiar `a` por dos opciones separadas: **`a` = permitir esta llamada** y **`A` = cambiar la
  política de la sesión a `auto`** (con un aviso explícito de lo que implica).
- Un modo `read-only` real para el shell: denegar comandos que escriben es difícil, pero
  puedes aplicar una lista de patrones peligrosos (`rm -rf`, `del /s`, `format`, `git push`,
  `> archivo`) y pedir confirmación aparte.
- Un sandbox de escritura barato en Go: en Linux `bwrap`, en macOS `sandbox-exec`, y en
  Windows aceptar que no hay (documentarlo) en vez de fingirlo.
- El **marcador único** de denegación que el modelo ya reconoce
  (`[sandbox: file access denied under <mode> mode]`), porque hace que se recupere solo.

---

---

### 5.6 Extensibilidad: MCP sí, plugins no

| Mecanismo | Harness | Tú | Veredicto |
|---|---|---|---|
| Herramientas MCP | Sí, más *recursos* (`list_mcp_resources`, `read_mcp_resource`) | Sí, solo herramientas | Bien; los recursos son un extra barato |
| Skills | Herramienta `skill` que **carga el cuerpo** de `SKILL.md` | `/skill` inserta `[Skill: nombre]` — el modelo ve la etiqueta, **no las instrucciones** | **Bug de producto** |
| Comandos `/` | `ctx.commands` con adaptadores de UI | Registro único con ayuda y autocompletado generados | **Empate; el tuyo es más claro en terminal** |
| Hooks | Eventos `hook/invoked` y `hook/result` | No | Hueco bajo |
| Subagentes | `subagent`, `subagent_fork`, control y mensajería | No | Hueco medio, caro |
| Workflows | Script que orquesta N subagentes | No | No copiar todavía |
| Tareas programadas | `schedule_*` con rrule | No | Curioso, poco útil en terminal |
| Jobs en segundo plano | `job_*` + promoción de comandos largos | No | Útil si añades comandos largos |
| Plugins en runtime | `extensions`, mercado, auto-modificación | No | No copiar |
| Presets de agente | Composición por sesión | No | No copiar |

**El bug de las skills, en detalle:** `skillOrToolRef` convierte `/caveman` en el texto
`[Skill: caveman]`. El modelo sabe que existe una skill llamada así (porque el contexto del
proyecto lista nombre y descripción), pero **nunca lee su contenido**. En la práctica la skill
no hace nada salvo que el modelo la conozca de memoria. Arreglarlo es leer
`.agents/skills/<nombre>/SKILL.md`, quitar el frontmatter y meter el cuerpo en el mensaje
(con su propio presupuesto de tamaño). Coste: ~60 líneas. Es de las cosas con mejor retorno
de todo este informe.

**Valor/coste de cada mecanismo, ya filtrado para un CLI Go:**

| Mecanismo | Valor | Coste | Cuándo |
|---|---|---|---|
| Skills de disco con catálogo perezoso | Alto | Bajo | Ya (es un bug) |
| MCP con instrucciones del servidor al prompt y reconexión | Alto | Medio | Ya (ya tienes el cliente) |
| Jobs con aviso inyectado (no *polling*) | Alto | Bajo | Cuando haya comandos largos |
| `headless` con `--json` y `--session-id` | Alto | Bajo | Ya, para CI |
| Subagentes con id y `send_message` | Alto | Alto | Fase 5+ |
| Schedules persistentes | Medio | Medio | Solo si el CLI es un daemon |
| Workflows JS sobre runtime aislado | Alto | Muy alto | No |
| Extensiones en runtime (auto-modificación) | Medio | Muy alto | No |

Dos detalles de su MCP que sí merecen copiarse: las herramientas se exponen como
`mcp__<servidor>__<nombre>` (espacio de nombres explícito, evita colisiones con las nativas) y
las **instrucciones del servidor** se inyectan en el system prompt, no solo sus herramientas.

### 5.7 UI: aquí ganas tú, y no lo estropees

El Harness resuelve la interfaz con una **Web UI** (tarjetas por herramienta, diffs, revisión
de planes, ajustes), más escritorio Electron, CLI headless y dos SDK. Dato importante que
salió del análisis: **no tienen ninguna app TUI**. Su historia de terminal es `headless`
(one-shot con `--json`). Tú tienes lo único que ellos no tienen: una interfaz de terminal.

| Lo que tú tienes | Lo que ellos tienen |
|---|---|
| TUI con Markdown renderizado | Markdown en la web |
| Diccionario `/help` con fichas por comando/skill/tool | Ayuda en la web + `cli-help.md` generado |
| `@ruta` resuelto en el prompt, con nota compacta | Adjuntos con `attachment`/`spill` |
| `--pipe` para scripts | Perfil `headless` con `--json` |
| Tokens y saldo en el chat | Panel de la Web UI |

**No construyas una Web UI.** Lo que sí puedes robar de su UI son ideas de *renderizado*:
mostrar un diff antes de aprobar una escritura, una tarjeta por resultado de herramienta
(colapsable si es larga), y la revisión de un plan como pregunta estructurada.

---

## 6. Hoja de ruta propuesta

Ordenada por **valor / coste**, no por lo que impresiona. Cada fase es útil por sí sola.

### Fase 0 — Arreglos de horas (1 día) — ✅ **completada**

| # | Qué | Dónde | Coste | Estado |
|---|---|---|---|---|
| 0.1 | Reconstruir el mensaje `system` en cada turno (fecha y contexto frescos) | `refreshSystemPrompt` | ~20 líneas | ✅ |
| 0.2 | Separar `a` (esta llamada) de «cambiar política de sesión» + `/approvals` | `confirmTool`, `approvalsString` | ~90 líneas | ✅ |
| 0.3 | Resultados sintéticos para tool calls sin resolver al abortar | `appendAbortedResults` | ~15 líneas | ✅ |
| 0.4 | «Siguiente petición» en `/tokens` | `projectedTokens` | ~10 líneas | ✅ |
| 0.5 | `--json` (NDJSON) en `ask`/`--pipe` | `jsonEvent`, `ask` | ~70 líneas | ✅ |
| 0.6 | **Cargar el contenido de las skills**, no solo la etiqueta | `project.ReadSkillBody` | ~60 líneas | ✅ |

### Fase 1 — Herramientas de fichero (1–2 semanas) — ✅ **completada**

Hoy el modelo lee y escribe **a través del shell**. Eso significa quoting de PowerShell,
codificación y paginado a mano, y ningún control por ruta.

| Herramienta | Por qué | Coste | Estado |
|---|---|---|---|
| `read_file` (offset/limit, números de línea, cap) | Lectura fiable y paginada; base del guard | 120 líneas | ✅ |
| `write_file` | Escritura atómica y con política de ruta | 80 líneas | ✅ |
| `edit_file` (reemplazo literal único, falla si no es único) | El 80 % de las ediciones de un agente | 100 líneas | ✅ |
| `glob` y `grep` | Buscar sin depender de `Get-ChildItem`/`rg` del sistema | 150 líneas | ✅ |
| Guard «leer antes de editar» | Evita editar a ciegas; el Harness lo llama `FS_NOT_OBSERVED` | 60 líneas | ✅ |
| Política de ruta (raíz del workspace) | Impide escribir fuera del proyecto | 80 líneas | ✅ |

Total: ~700 líneas en `internal/fstools` + `cli/filetools.go`, con 22 tests. Dos cambios de comportamiento: leer (`read_file`, `glob`, `grep`) ya no pide permiso, y el prompt de escritura muestra el cambio con líneas `-`/`+` en vez del JSON de argumentos.

### Fase 2 — Log de sesión durable (2–3 semanas) — ✅ **completada**

| Paso | Coste |
|---|---|
| JSONL append-only por sesión, con `seq` y `time` | ~300 líneas | ✅ |
| Cinco tipos de evento: `message`, `tool_call`, `tool_result`, `compaction`, `turn_end` | ~150 líneas | ✅ |
| `deriveMessages()` desde el log, con caché | ~250 líneas | ✅ |
| `--continue` / `--resume` + cierre sintético del turno interrumpido | ~200 líneas | ✅ | ✅ |
| Migrar `history` de SQLite a leer del log (o mantener SQLite como índice) | ~150 líneas | ✅ |

Habilita todo lo demás: compactación, fork, auditoría, `session_search`.

Implementado en `internal/session` con 11 tests. SQLite desapareció del proyecto, así que el
binario bajó de **18,3 MB a 13,7 MB**, y `history` ahora lista, muestra y borra sesiones.

### Fase 3 — Compactación y presión de contexto (2–3 semanas) — ✅ **completada**

| Paso | Coste | Estado |
|---|---|---|
| Medición por mensaje (heurística + `usage` real) con anclaje | ~200 líneas | ✅ |
| `projectedTokens` real (presión + delta desde la última muestra) | ~120 líneas | ✅ |
| Poda de resultados de herramientas viejos antes de resumir | ~150 líneas | ⏭ pendiente |
| Resumen del tramo antiguo con el modelo + marcado como reemplazado | ~300 líneas | ✅ |
| Disparo por umbral (80 % de la ventana) y por error «contexto excedido» con retry | ~150 líneas | ✅ |
| `/compact` manual y aviso en la UI de cuánto se ha recortado | ~80 líneas | ✅ |

### Fase 4 — Permisos y confinamiento (1–2 semanas) — ✅ **completada**

- Presets de dos perillas: `sandbox` (read-only / workspace-write / full) × `approval`
  (ask / auto), como en `permission-presets`. ✅
- Modo `read-only` con denylist de patrones peligrosos. ✅
- Marcador único de denegación que el modelo reconoce. ✅
- Envolver argv con `bwrap` (Linux) y `sandbox-exec` (macOS): **no hecho**. En Windows no
  hay sandbox de procesos, así que `read-only` deniega el shell entero en vez de filtrarlo.
  **Fail-closed**: se dice lo que no se puede garantizar, en vez de fingir.

Implementado: `config.SandboxMode()`/`ApprovalAuto()`, `fstools.Set.SetConfined`,
`shell.Danger` (25 patrones + descarga-a-shell) y el marcador
`[sandbox: file access denied under <modo> mode]`. Los comandos destructivos piden
confirmación incluso en modo `auto`.

### Fase 5 — Extensibilidad (2–3 semanas, por orden de retorno) — ✅ **completada**

1. `todo_write` (lista de tareas durable y visible en la TUI). ✅ `internal/todo` + evento `todo/list`; panel encima de la entrada.
2. ✅ Jobs en segundo plano con **aviso inyectado** en vez de *polling*: el proceso largo se
   promueve a job (`internal/jobs`, canal `Done`) y, al terminar, se inyecta un mensaje.
   La TUI re-arma el listener tras cada aviso y continúa el turno si estabas inactivo.
3. ✅ `/plan` con sección de guía en el prompt y `exit_plan_mode` aprobado por el usuario (siempre pregunta).
4. ✅ `ask_user_question` estructurada (opciones, multiselección) con modal en la TUI; en modo línea el modelo pregunta en texto.
5. MCP: espacio de nombres `mcp__<servidor>__<tool>`, instrucciones del servidor al prompt,
   timeout y reconexión con retroceso exponencial.
6. Subagentes — lo más caro; solo si de verdad lo necesitas.

### Fase 6 — Lo que probablemente nunca necesites — ✅ **decidido no construir**

LSP, navegador, computer-use, Agent Teams, workflows, `run_code`/PTC, marketplace de plugins,
perfiles y patches. Son producto de plataforma, no de un coder de terminal.

El entregable de esta fase **es la decisión**, y está en la tabla de la sección 7. Construir
un runtime de plugins o una Web UI gemela habría duplicado el tamaño del proyecto sin mejorar
lo único que este CLI hace mejor que el Harness: vivir en el terminal.

Lo que sí se añadió como cierre, porque cabe en el producto y no añade superficie:
`session_search` (`internal/session/search.go`) y `deepseek history search`, que convierten
el log durable de la Fase 2 en memoria consultable.

---

## 7. Qué NO copiar

| No copies | Por qué |
|---|---|
| **Cordis y el sistema de plugins** | Es el 70 % de su complejidad. Solo se amortiza vendiendo una plataforma. Copia los *contratos* (seam = definición + proveedor + consumidor), no el runtime. |
| **Perfiles, bundles y patches YAML** | Resuelven composición de producto multi-cliente. Tú tienes un binario y un usuario. Si algún día necesitas presets, un `config.yaml` con merge por clave es suficiente. |
| **La Web UI y las superficies gemelas** (Electron, SDK TypeScript + Python, ACP) | Otro producto. Tu ventaja es el terminal, y ellos **no tienen TUI**: es lo único que no puedes copiar de ellos porque no existe. |
| **Los `dsh_*` body extensions** (subida del log de sesión al endpoint) | Es telemetría propietaria hacia DeepSeek. No aporta al usuario y añade superficie. |
| **El plugin market y el gestor de plugins** | Requiere npm/pnpm, registries, CDN y política de compatibilidad. |
| **Extensiones en runtime y auto-modificación** | Dos caras (host y navegador) y evaluación de TypeScript en el host. |
| **Settings con esquema → JSON Schema → formularios** | Es generar una UI de ajustes a partir de un esquema. Tú tienes `config set` y un `config get` legible. |
| **Workflows JS sobre PTC** | Necesita un runtime JS aislado. Sin él, es un riesgo sin retorno. |
| **`auto-review`** (un modelo revisando al otro) | Experimental, y convierte el sandbox en teatro. |
| **El sandbox ACL de Windows** | Semanas de trabajo para un `partial` que sigue sin confinar lecturas. |
| **Inferir denegaciones por firmas de stderr** | En Go tienes canal tipado; no heredes la heurística. |
| **El catálogo de 69 herramientas** | Cada herramienta es superficie de prompt y de mantenimiento. Empieza por las 6 de la Fase 1. |

---

## 8. Mapa de lectura para estudiar su código

Si quieres leer el código de referencia, este es el orden que menos tiempo te hace perder.
Todo son rutas relativas a `deepseek-harness-master/`.

**Día 1 — el modelo mental (documentos, no código)**
1. `docs/architecture.md` — la composición y los puntos de extensión.
2. `docs/glossary.md` — turno, paso, ronda, seam, scope, goal. Sin este vocabulario el resto no se entiende.
3. `docs/agent-lifecycle.md` — el diagrama del ciclo completo.

**Día 2 — el bucle y la sesión**
4. `packages/core/agent-loop/src/agent.ts` — el driver real.
5. `packages/core/agent-loop/src/inbox.ts` — por qué hay dos colas.
6. `packages/core/session/src/surface.ts` y `types.ts` — el log y `deriveMessages`.
7. `packages/core/session/src/repair.ts` — resultados sintéticos.

**Día 3 — herramientas**
8. `packages/core/tools/src/pipeline.ts` — el pipeline completo (771 líneas, es el corazón).
9. `packages/core/tools/src/batch.ts` — concurrencia y commit ordenado.
10. `docs/tool-catalog.md` — qué herramientas hay y con qué contrato (usa búsqueda, no lo leas entero).

**Día 4 — contexto**
11. `packages/llm/token-meter/src/*` — medición y proyección.
12. `packages/compaction/compaction-basic/src/region.ts` — cómo elige el tramo a resumir.
13. `packages/core/system-prompt/*` — secciones y orden.

**Día 5 — seguridad**
14. `packages/sandbox/sandbox/src/escalation.ts` — la tabla de escalado.
15. `packages/sandbox/sandbox-local/src/profiles.ts` — cómo se envuelve el argv.
16. `packages/interaction/user-approval/README.md` — el contrato de aprobación.

**Aviso:** es TypeScript con efectos, WeakMaps y genéricos tipados. No intentes trasladar
estructuras; traslada **invariantes**. Cuando algo te parezca barroco, pregúntate qué fallo
concreto evita antes de descartarlo.

---

## Apéndice A — Inventario de herramientas del Harness

69 nombres distintos. `*` = imprescindible en un clon.

| Grupo | Herramientas |
|---|---|
| Shell / terminal | `bash`*, `pwsh`*, variantes persistentes (PTY), `terminal_open/list/read/send/signal/close` |
| Ficheros | `read`*, `write`*, `edit`*, `apply_patch`, `str_replace_editor`, `read_image` |
| Búsqueda | `glob`*, `grep`* |
| Planificación | `todo_write`*, `exit_plan_mode`, `create_goal`/`get_goal`/`update_goal`, `schedule_*` |
| Delegación | `subagent`*, `subagent_fork`*, `send_message`*, `interrupt_agent`*, `list_agents`*, `workflow`*, `ralph`, `subagent_oar`, Agent Teams |
| Memoria | `session_search`*, `session_event_read/search/trace`, `session_trace` |
| Web | `web_search`*, `web_fetch`* |
| Código | `lsp` |
| Interacción | `ask_user_question`*, `present`*, `propose_action` |
| Jobs | `job_output`*, `job_list`*, `job_kill`* |
| Contexto | `skill`*, `working_directory`*, `load_workspace_dependencies`* |
| MCP | `list_mcp_resources`, `list_mcp_resource_templates`, `read_mcp_resource` |
| Ejecución alternativa | `run_code`* (PTC), `create_worktree` |
| Extensión | `plugin_manager`, `plugin_market`, `cordis_inspect_*` |
| Navegador / escritorio | `stagehand_*` (6), computer-use |

## Apéndice B — Cifras medidas

| | Valor |
|---|---|
| Tu proyecto | 6 892 líneas Go + 2 537 de test, 35 ficheros, 12 paquetes internos |
| Harness | 20 233 ficheros, 134,7 MB, 6 042 `.ts`, 821 `.tsx`, ~48 grupos de paquetes |
| Herramientas del Harness | 69 nombres en 76 paquetes |
| Contexto que envías en `full` | 11,6 KB ≈ 3 950 tokens (93 % README + diff) |
| Contexto que envías en `light` | 1,4 KB ≈ 490 tokens |
| Prompt real medido con «hola» | 4 151 tokens de entrada |




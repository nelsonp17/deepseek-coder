package i18n

import (
	"fmt"
	"strings"
	"time"
)

// Lang is a supported interface language.
type Lang string

const (
	ES Lang = "es"
	EN Lang = "en"
)

// Parse converts user input into a Lang.
func Parse(s string) (Lang, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "es", "spanish", "español", "espanol":
		return ES, true
	case "2", "en", "english", "inglés", "ingles":
		return EN, true
	}
	return "", false
}

// Messages holds every user-facing string in one language. Fields without
// format verbs are plain strings; messages with verbs are exposed as methods
// that return the already-formatted string.
type Messages struct {
	Lang Lang

	Welcome              string
	NoAPIKey             string
	EnterAPIKey          string
	Saved                string
	Removed              string
	NoConversations      string
	EmptyQuestion        string
	HistoryDisabled      string
	SessionOpenFailed    string
	CompactionNoSession  string
	CompactionNoWindow   string
	CompactionNothing    string
	CompactionEmpty      string
	CompactionFailed     string
	NoTodos              string
	NoMatches            string
	HistorySearchUsage   string
	PlanOn               string
	QuestionHelp         string
	PlanOff              string
	ShellParamBackground string
	JobsTitle            string
	JobsNone             string
	JobStarted           string
	JobFinished          string
	TodosTitle           string
	SandboxTitle         string
	SandboxUsage         string
	SandboxReadOnly      string
	SandboxWorkspace     string
	SandboxFull          string
	InvalidSandboxValue  string
	InvalidApprovalValue string
	toolBlocked          string
	SaveFailed           string
	SelectLanguage       string
	LanguageSet          string
	InvalidLangValue     string
	NotSet               string
	Default              string
	ConfigUsage          string
	SetUsage             string
	UnsetUsage           string
	HistoryShowUsage     string
	HistoryDeleteUsage   string
	Usage                string
	SystemPrompt         string
	ModelsTitle          string
	NoModels             string
	ModelUsage           string
	// Slash command help. The listing itself is rendered from the command
	// registry in the cli package; i18n only provides the wording.
	HelpTitle           string
	HelpSectionCommands string
	HelpSectionKeys     string
	HelpSectionSkills   string
	HelpSectionMCP      string
	HelpNoSkills        string
	HelpNoMCP           string
	HelpNoTools         string
	HelpNoDescription   string
	HelpFooter          string
	HelpTopicUsage      string
	HelpTopicAliases    string
	HelpTopicServer     string
	HelpSubcommands     string
	HelpSkillTag        string
	HelpToolTag         string
	CommandGroups       map[string]string
	CommandDescs        map[string]string
	CommandDetails      map[string]string

	helpSkillHowTo   string
	helpToolHowTo    string
	helpUnknownTopic string

	// Attachments (@path references) and pipe mode.
	AttachPrefix     string
	AttachNotFound   string
	AttachDirectory  string
	AttachTooLarge   string
	AttachBinary     string
	AttachBudget     string
	AttachReadFailed string
	PipeUsage        string

	attachHeader        string
	attachFileTag       string
	attachItem          string
	attachItemTruncated string
	attachSkipped       string
	pipeStdinLabel      string

	// Native shell tool.
	ShellToolDescription string // %s = detected shell name
	ShellParamCommand    string
	ShellParamDir        string
	ShellParamTimeout    string
	ShellAlwaysWarning   string
	ShellDisabled        string
	ShellDetected        string // %s = shell name, %s = executable path
	RunUsage             string
	ShellEmptyCommand    string
	ShellNoOutput        string
	ShellTimeoutLabel    string
	EnvTitle             string
	EnvOS                string
	EnvShell             string
	EnvDir               string
	EnvTool              string
	EnvToolOn            string
	EnvToolOff           string
	EnvSyntax            string
	EnvNow               string
	EnvNowHint           string

	// Token accounting and balance.
	TokensTitle        string
	TokensModel        string
	TokensEffort       string
	TokensLast         string
	TokensContext      string
	TokensSession      string
	TokensTotal        string
	TokensCache        string
	TokensNone         string
	TokensNext         string
	ToolNotStarted     string
	BalanceTitle       string
	BalanceOK          string
	BalanceUnavailable string
	BalanceTotal       string
	BalanceToppedUp    string
	BalanceGranted     string
	BalanceEmpty       string
	BalanceFailed      string

	// Project context sizing.
	ContextModeLabel string
	ContextTotal     string
	ContextOmitted   string
	ContextAll       string
	ContextSaveLight string
	ContextParts     map[string]string
	TokensProject    string

	SelectModel            string
	InvalidModelSelection  string
	ModelsUnavailable      string
	NoModelNonInteractive  string
	EditorFallback         string
	ContextTitle           string
	NoContext              string
	ContextReloaded        string
	ContextUsage           string
	ToolPrompt             string
	ApprovalsTitle         string
	ApprovalStateAsk       string
	ApprovalStateAuto      string
	ApprovalsBlocked       string
	ApprovalUsage          string
	ApprovalAutoWarning    string
	ToolDeniedContent      string
	ToolLoopLimit          string
	NoMCP                  string
	MCPError               string
	NoMCPServers           string
	LoginOK                string
	InvalidThinkingValue   string
	InvalidEffortValue     string
	InvalidMouseValue      string
	InvalidShellValue      string
	InvalidShellTimeout    string
	InvalidContextValue    string
	InvalidCompactValue    string
	InvalidWindowValue     string
	TUIInputPlaceholder    string
	TUIThinking            string
	ModelSelectPlaceholder string
	ModelSelectHint        string
	TUIKeySend             string
	TUIKeyNewline          string
	TUIKeyComplete         string
	TUIKeyScroll           string
	TUIKeyQuit             string
	TUIKeyCopy             string
	TUIHints               string
	UserLabel              string
	AssistantLabel         string
	CopyFailed             string
	SaveTranscriptFailed   string

	copied          string
	transcriptSaved string
	shellExitCode   string
	shellTimedOut   string

	slowResponse string

	toolRequest  string
	toolDenied   string
	toolRunning  string
	mcpFailed    string
	mcpConnected string
	skillRef     string
	toolRef      string

	sessionResumed             string
	sessionMissing             string
	sessionDeleted             string
	historyTurns               string
	compactionRunning          string
	compactionDone             string
	prunedLine                 string
	conversationNotFound       string
	conversationDeleted        string
	invalidID                  string
	unknownCommand             string
	unknownConfigKey           string
	unknownHistoryCmd          string
	modelNotAvailable          string
	modelNotListed             string
	modelSetDefault            string
	modelInvalidNonInteractive string
	modelCleared               string
	currentModel               string
	unknownSlashCmd            string
}

// ToolRequest returns the permission request for a tool call.
func (m *Messages) ToolRequest(name, args string) string {
	return fmt.Sprintf(m.toolRequest, name, args)
}

// ToolDenied returns "Tool %s denied by the user.".
func (m *Messages) ToolDenied(name string) string {
	return fmt.Sprintf(m.toolDenied, name)
}

// ToolRunning returns "Running %s...".
func (m *Messages) ToolRunning(name string) string {
	return fmt.Sprintf(m.toolRunning, name)
}

// MCPFailed returns the message for a server that could not start.
func (m *Messages) MCPFailed(name string, err error) string {
	return fmt.Sprintf(m.mcpFailed, name, err)
}

// MCPConnected returns the message for a connected server.
func (m *Messages) MCPConnected(name string, tools int) string {
	return fmt.Sprintf(m.mcpConnected, name, tools)
}

// SkillRef returns the user message that references a skill.
func (m *Messages) SkillRef(name string) string {
	return fmt.Sprintf(m.skillRef, name)
}

// ToolRef returns the user message that references an MCP tool.
func (m *Messages) ToolRef(name string) string {
	return fmt.Sprintf(m.toolRef, name)
}

// SlowResponse returns the hint shown when the model takes too long.
func (m *Messages) SlowResponse(elapsed time.Duration) string {
	return fmt.Sprintf(m.slowResponse, elapsed.Round(time.Second))
}

// Copied returns "<n> characters copied to the clipboard.".
func (m *Messages) Copied(chars int) string {
	return fmt.Sprintf(m.copied, chars)
}

// CommandGroup returns the localised title of a help section.
func (m *Messages) CommandGroup(id string) string {
	if g, ok := m.CommandGroups[id]; ok {
		return g
	}
	return id
}

// CommandDescription returns the localised description of a slash command.
func (m *Messages) CommandDescription(name string) string {
	if d, ok := m.CommandDescs[name]; ok {
		return d
	}
	return ""
}

// CommandDetail returns the long documentation of a slash command, falling
// back to its one-line description.
func (m *Messages) CommandDetail(name string) string {
	if d, ok := m.CommandDetails[name]; ok {
		return d
	}
	return m.CommandDescription(name)
}

// HelpSkillHowTo explains how to reference a skill.
func (m *Messages) HelpSkillHowTo(ref string) string {
	return fmt.Sprintf(m.helpSkillHowTo, ref)
}

// HelpToolHowTo explains how an MCP tool gets used.
func (m *Messages) HelpToolHowTo(name string) string {
	return fmt.Sprintf(m.helpToolHowTo, name)
}

// HelpUnknownTopic reports a topic that is not documented.
func (m *Messages) HelpUnknownTopic(topic string) string {
	return fmt.Sprintf(m.helpUnknownTopic, topic)
}

// AttachHeader opens the section of files attached to a user message.
func (m *Messages) AttachHeader(size string) string {
	return fmt.Sprintf(m.attachHeader, size)
}

// AttachFileTag wraps the path that precedes one attached file.
func (m *Messages) AttachFileTag(path string) string {
	return fmt.Sprintf(m.attachFileTag, path)
}

// AttachItem is one entry of the attachment note shown to the user.
func (m *Messages) AttachItem(path, size string, truncated bool) string {
	if truncated {
		return fmt.Sprintf(m.attachItemTruncated, path, size)
	}
	return fmt.Sprintf(m.attachItem, path, size)
}

// AttachSkipped is one entry of the note for a reference that failed.
func (m *Messages) AttachSkipped(path, reason string) string {
	return fmt.Sprintf(m.attachSkipped, path, reason)
}

// PipeStdinLabel introduces the block of text read from stdin.
func (m *Messages) PipeStdinLabel(size string) string {
	return fmt.Sprintf(m.pipeStdinLabel, size)
}

// ContextPart returns the label of one section of the project context.
func (m *Messages) ContextPart(name string) string {
	if p, ok := m.ContextParts[name]; ok {
		return p
	}
	return name
}

// TranscriptSaved returns "Conversation saved to <path>".
func (m *Messages) TranscriptSaved(path string) string {
	return fmt.Sprintf(m.transcriptSaved, path)
}

// ShellExitCode returns "exit code: <n>".
func (m *Messages) ShellExitCode(code int) string {
	return fmt.Sprintf(m.shellExitCode, code)
}

// ShellTimedOutNotice returns the note shown when a command was killed.
func (m *Messages) ShellTimedOutNotice(d time.Duration) string {
	return fmt.Sprintf(m.shellTimedOut, d)
}

// ModelNotAvailable returns "Model %s is no longer available. Pick another:".
func (m *Messages) ModelNotAvailable(id string) string {
	return fmt.Sprintf(m.modelNotAvailable, id)
}

// ModelNotListed returns the warning for a model that is not in the API list.
func (m *Messages) ModelNotListed(id string) string {
	return fmt.Sprintf(m.modelNotListed, id)
}

// ModelSetDefault returns "Default model: %s".
func (m *Messages) ModelSetDefault(id string) string {
	return fmt.Sprintf(m.modelSetDefault, id)
}

// ModelInvalidNonInteractive returns the error for a stale model in scripts.
func (m *Messages) ModelInvalidNonInteractive(id string) string {
	return fmt.Sprintf(m.modelInvalidNonInteractive, id)
}

// ModelCleared returns "Model reset: %s".
func (m *Messages) ModelCleared(id string) string {
	return fmt.Sprintf(m.modelCleared, id)
}

// CurrentModel returns "Current model: %s".
func (m *Messages) CurrentModel(id string) string {
	return fmt.Sprintf(m.currentModel, id)
}

// UnknownSlashCmd returns "Unknown command: %s (try /help)".
func (m *Messages) UnknownSlashCmd(cmd string) string {
	return fmt.Sprintf(m.unknownSlashCmd, cmd)
}

// ConversationNotFound returns "Conversation %d not found".
func (m *Messages) ConversationNotFound(id int64) string {
	return fmt.Sprintf(m.conversationNotFound, id)
}

// SessionResumed reports an existing session being reopened.
func (m *Messages) SessionResumed(id string) string {
	return fmt.Sprintf(m.sessionResumed, id)
}

// ToolBlocked reports a call refused by the sandbox mode.
func (m *Messages) ToolBlocked(name, mode string) string {
	return fmt.Sprintf(m.toolBlocked, name, mode)
}

// CompactionRunning announces a compaction of the given token size.
func (m *Messages) CompactionRunning(tokens int) string {
	return fmt.Sprintf(m.compactionRunning, tokens)
}

// Pruned reports how much old tool output was shortened.
func (m *Messages) Pruned(saved int) string {
	return fmt.Sprintf(m.prunedLine, saved)
}

// CompactionDone reports the estimated size before and after compacting.
func (m *Messages) CompactionDone(before, after int) string {
	return fmt.Sprintf(m.compactionDone, before, after)
}

// SessionNotFound reports a session id that does not exist.
func (m *Messages) SessionNotFound(id string) string {
	return fmt.Sprintf(m.sessionMissing, id)
}

// SessionDeleted reports a removed session.
func (m *Messages) SessionDeleted(id string) string {
	return fmt.Sprintf(m.sessionDeleted, id)
}

// HistoryTurns formats the turn count of a stored session.
func (m *Messages) HistoryTurns(n int) string {
	return fmt.Sprintf(m.historyTurns, n)
}

// ConversationDeleted returns "Conversation %d deleted.".
func (m *Messages) ConversationDeleted(id int64) string {
	return fmt.Sprintf(m.conversationDeleted, id)
}

// InvalidID returns "Invalid conversation id %q".
func (m *Messages) InvalidID(id string) string {
	return fmt.Sprintf(m.invalidID, id)
}

// UnknownCommand returns "Unknown command %q".
func (m *Messages) UnknownCommand(cmd string) string {
	return fmt.Sprintf(m.unknownCommand, cmd)
}

// UnknownConfigKey returns "Unknown config key %q (valid: ...)".
func (m *Messages) UnknownConfigKey(key string) string {
	return fmt.Sprintf(m.unknownConfigKey, key)
}

// UnknownHistoryCmd returns "Unknown history command %q (valid: ...)".
func (m *Messages) UnknownHistoryCmd(cmd string) string {
	return fmt.Sprintf(m.unknownHistoryCmd, cmd)
}

var es = &Messages{
	Lang:                 ES,
	Welcome:              "DeepSeek coder — escribe tu mensaje, 'exit' para salir.",
	NoAPIKey:             "No hay API key configurada: ejecuta `deekseep config set api_key <clave>` o define DEEPSEEK_API_KEY",
	EnterAPIKey:          "Introduce tu API key de DeepSeek (no se mostrará al escribir):",
	Saved:                "Guardado.",
	Removed:              "Eliminado.",
	NoConversations:      "Aún no hay conversaciones.",
	EmptyQuestion:        "Pregunta vacía: usa `deekseep ask \"<pregunta>\"`",
	HistoryDisabled:      "aviso: historial desactivado:",
	SessionOpenFailed:    "no se pudo abrir la sesión:",
	CompactionNoSession:  "esta conversación no tiene historial guardado",
	CompactionNoWindow:   "no sé la ventana de contexto del modelo; no puedo decidir cuándo compactar",
	CompactionNothing:    "todavía no hay nada que compactar",
	CompactionEmpty:      "el modelo devolvió un resumen vacío",
	CompactionFailed:     "no se pudo compactar:",
	NoTodos:              "todavía no hay ninguna lista de tareas",
	NoMatches:            "sin coincidencias en conversaciones anteriores",
	HistorySearchUsage:   "uso: deekseep history search <texto>",
	PlanOn:               "modo plan ACTIVADO: solo lectura hasta que apruebes el plan",
	QuestionHelp:         "1-9 elige · ↑↓ mueve · Espacio marca · Enter confirma · Esc cancela",
	PlanOff:              "modo plan desactivado",
	ShellParamBackground: "Si es true, el comando corre en segundo plano y recibirás un mensaje cuando termine. Úsalo para builds, tests o servidores largos.",
	JobsTitle:            "Trabajos en segundo plano:",
	JobsNone:             "no hay trabajos en marcha",
	JobStarted:           "trabajo %d en marcha",
	JobFinished:          "trabajo %d terminado",
	TodosTitle:           "Tareas:",
	SandboxTitle:         "Modo de confinamiento:",
	SandboxUsage:         "uso: /sandbox [read-only|workspace-write|full]",
	SandboxReadOnly:      "solo lectura: se deniegan escrituras y comandos",
	SandboxWorkspace:     "escritura solo dentro del proyecto (por defecto)",
	SandboxFull:          "sin confinamiento de rutas (peligroso)",
	InvalidSandboxValue:  "Valor inválido: usa 'read-only', 'workspace-write' o 'full'",
	InvalidApprovalValue: "Valor inválido: usa 'ask' o 'auto'",
	toolBlocked:          "Herramienta %s bloqueada por el modo %s",
	prunedLine:           "podados ~%d tokens de salida de herramientas antiguas",
	compactionRunning:    "compactando ~%d tokens del historial...",
	compactionDone:       "historial compactado: ≈%d → ≈%d tokens",
	sessionResumed:       "sesión %s reanudada",
	sessionMissing:       "no existe la sesión %q",
	sessionDeleted:       "sesión %s eliminada",
	historyTurns:         "%d turnos",
	SaveFailed:           "aviso: no se pudo guardar el mensaje:",
	SelectLanguage:       "Selecciona tu idioma / Select your language:\n  1) Español\n  2) English\n> ",
	LanguageSet:          "Idioma configurado: español.",
	InvalidLangValue:     "Idioma inválido: usa 'es' o 'en'",
	NotSet:               "(sin configurar)",
	Default:              "(por defecto)",
	ConfigUsage:          "uso: deekseep config get|set|unset",
	SetUsage:             "uso: deekseep config set <clave> <valor>",
	UnsetUsage:           "uso: deekseep config unset <clave>",
	HistoryShowUsage:     "uso: deekseep history show <id>",
	HistoryDeleteUsage:   "uso: deekseep history delete <id>",
	Usage: `deekseep-coder — asistente de código con DeepSeek

Uso:
  deekseep [--debug]            inicia un chat interactivo (--debug: logs de la API)
  deekseep ask "<pregunta>"     hace una consulta puntual
  deekseep --pipe [pregunta]    manda por stdin contenido (archivos, logs, errores)
  deekseep --json ask ...       salida NDJSON (start/delta/usage/done/error) para scripts
  deekseep -c                   continúa la última sesión guardada
  deekseep --resume <id>        reanuda una sesión concreta
  deekseep config get           muestra la configuración actual
  deekseep config set <clave> <valor>
                                configura un valor (api_key, model, base_url, lang,
                                thinking, reasoning_effort, shell, shell_timeout, context)
  deekseep config unset <clave> elimina un valor de configuración
  deekseep history              lista las conversaciones guardadas
  deekseep history show <id>    muestra una conversación
  deekseep history delete <id>  elimina una conversación
  deekseep models               lista los modelos disponibles de la API
  deekseep balance              consulta el saldo de la cuenta
  deekseep mcp                  muestra los servidores MCP y sus herramientas
  deekseep help                 muestra esta ayuda

Autocompletado en el chat:
  @<ruta>   completa rutas de archivo (Tab)
  /<cmd>    completa comandos, skills y herramientas MCP (Tab)

Entorno:
  DEEPSEEK_API_KEY   API key (anula llavero y archivo de config)
  DEEPSEEK_MODEL     modelo a usar (anula el archivo de config)
`,
	SystemPrompt:               "You are a helpful coding assistant. Answer concisely and provide code when relevant. Respond in Spanish.",
	conversationNotFound:       "Conversación %d no encontrada",
	conversationDeleted:        "Conversación %d eliminada.",
	invalidID:                  "ID de conversación inválido %q",
	unknownCommand:             "Comando desconocido %q",
	unknownConfigKey:           "Clave de configuración desconocida %q (válidas: api_key, model, base_url, lang, thinking, reasoning_effort, shell, shell_timeout, context)",
	unknownHistoryCmd:          "Comando de historial desconocido %q (válidos: show, delete)",
	modelCleared:               "Modelo restablecido: %s",
	currentModel:               "Modelo actual: %s",
	unknownSlashCmd:            "Comando desconocido: %s (prueba /help)",
	modelNotAvailable:          "El modelo %s ya no está disponible. Selecciona otro:",
	modelNotListed:             "El modelo %s no está en la lista de modelos disponibles. Si no existe, la API devolverá un error.",
	modelSetDefault:            "Modelo por defecto: %s",
	modelInvalidNonInteractive: "El modelo configurado %s no está disponible y no hay terminal interactiva para seleccionar otro. Ejecuta `deekseep models` o `deekseep config set model <id>`.",
	ModelsTitle:                "Modelos disponibles (* = en uso):",
	NoModels:                   "No hay modelos disponibles.",
	ModelUsage:                 "uso: /models <id> — cambia y guarda el modelo por defecto",
	SelectModel:                "Selecciona el modelo por defecto:",
	InvalidModelSelection:      "Selección inválida. Introduce un número o un ID de modelo.",
	ModelsUnavailable:          "aviso: no se pudo consultar la lista de modelos:",
	NoModelNonInteractive:      "No hay modelo configurado y no hay terminal interactiva para seleccionarlo. Ejecuta `deekseep config set model <id>`.",
	EditorFallback:             "aviso: no se pudo activar el editor multilínea, usando entrada simple:",
	ContextTitle:               "Contexto del proyecto:",
	NoContext:                  "No hay contexto de proyecto cargado.",
	ContextReloaded:            "Contexto recargado.",
	ContextUsage:               "uso: /context [reload]",
	ToolPrompt:                 "¿Permitir? [y]es / [n]o / [a]uto (sin más preguntas) / [d]eny esta herramienta: ",
	ApprovalsTitle:             "Permisos de herramientas:",
	ApprovalStateAsk:           "pide confirmación en cada llamada",
	ApprovalStateAuto:          "NO pide confirmación (salvo comandos destructivos)",
	ApprovalsBlocked:           "herramientas denegadas",
	ApprovalUsage:              "uso: /approvals [ask|auto]",
	ApprovalAutoWarning:        "atención: no volveré a pedir permiso en esta sesión; reactívalo con /approvals ask",
	ToolDeniedContent:          "El usuario denegó esta llamada a herramienta.",
	ToolLoopLimit:              "Demasiadas rondas de herramientas. Abortando.",
	NoMCP:                      "No hay servidores MCP conectados.",
	MCPError:                   "aviso: error de MCP:",
	NoMCPServers:               "No hay servidores MCP configurados (mcp.json).",
	LoginOK:                    "Credenciales guardadas.",
	InvalidThinkingValue:       "Valor inválido: usa 'enabled' o 'disabled' (o 'on'/'off')",
	InvalidEffortValue:         "Valor inválido: usa 'low', 'high' o 'max'",
	InvalidMouseValue:          "Valor inválido: usa 'on' o 'off'",
	InvalidShellValue:          "Valor inválido: usa 'on' u 'off'",
	InvalidShellTimeout:        "Valor inválido: usa un número de segundos entre 1 y 600",
	InvalidContextValue:        "Valor inválido: usa 'full', 'light' u 'off'",

	TUIInputPlaceholder:    "Escribe un mensaje... (Enter envía, Ctrl+J nueva línea)",
	TUIThinking:            "pensando...",
	ModelSelectPlaceholder: "número o ID del modelo",
	ModelSelectHint:        "Selecciona un modelo (número o ID)",
	TUIKeySend:             "enviar",
	TUIKeyNewline:          "nueva línea",
	TUIKeyComplete:         "completar",
	TUIKeyScroll:           "scroll",
	TUIKeyQuit:             "salir",
	TUIKeyCopy:             "copiar",
	TUIHints:               "Rueda: scroll · Selecciona texto con el cursor cuando quieras · Ctrl+Y: copiar la última respuesta · /copy all: copiar todo · /save: guardar a archivo",
	UserLabel:              "tú",
	AssistantLabel:         "deepseek",
	CopyFailed:             "no se pudo copiar al portapapeles:",
	SaveTranscriptFailed:   "no se pudo guardar la conversación:",
	copied:                 "%d caracteres copiados al portapapeles.",
	transcriptSaved:        "Conversación guardada en %s",
	slowResponse:           "El modelo está tardando en responder (%s)... Puedes esperar o pulsar Ctrl+C.",
	toolRequest:            "El modelo quiere ejecutar la herramienta %s:\n%s\n",
	toolDenied:             "Herramienta %s denegada por el usuario.",
	toolRunning:            "Ejecutando %s...",
	mcpFailed:              "aviso: servidor MCP %s no disponible: %v\n",
	mcpConnected:           "MCP %s conectado (%d herramientas).\n",
	skillRef:               "[Skill: %s]",
	toolRef:                "[Herramienta MCP: %s]",
	HelpTitle:              "DeepSeek Coder — referencia completa",
	HelpSectionCommands:    "Comandos",
	HelpSectionKeys:        "Atajos de teclado",
	HelpSectionSkills:      "Skills del proyecto",
	HelpSectionMCP:         "Servidores MCP",
	HelpNoSkills:           "No hay skills en el proyecto (.agents/skills).",
	HelpNoMCP:              "No hay servidores MCP conectados (mcp.json).",
	HelpNoTools:            "sin herramientas",
	HelpNoDescription:      "sin descripción",
	HelpFooter: `Escribe /help <tema> para ver la ficha de un comando, de una skill o de
una herramienta MCP (por ejemplo: /help copy o /help run).

Puedes seleccionar texto con el cursor en cualquier momento y copiarlo: la TUI
no captura el ratón. Ctrl+Y copia la última respuesta sin seleccionar.`,
	HelpTopicUsage:      "Uso",
	HelpTopicAliases:    "Alias",
	HelpTopicServer:     "Servidor",
	HelpSubcommands:     "Subcomandos:",
	HelpSkillTag:        "skill del proyecto",
	HelpToolTag:         "herramienta MCP",
	helpSkillHowTo:      "Se referencia escribiendo %s en un mensaje.",
	helpToolHowTo:       "El modelo la usa como el resto de herramientas, siempre con tu permiso.",
	helpUnknownTopic:    "No hay nada documentado con el nombre %q. Prueba /help para ver el catálogo.",
	AttachPrefix:        "📎 ",
	AttachNotFound:      "no encontrado",
	AttachDirectory:     "es un directorio",
	AttachTooLarge:      "demasiado grande",
	AttachBinary:        "archivo binario",
	AttachBudget:        "sin presupuesto",
	AttachReadFailed:    "no se pudo leer",
	PipeUsage:           "uso: deekseep --pipe [pregunta] — la pregunta también puede venir por la entrada estándar",
	attachHeader:        "Archivos adjuntos (%s):",
	attachFileTag:       "<archivo: %s>",
	attachItem:          "%s (%s)",
	attachItemTruncated: "%s (%s, truncado)",
	attachSkipped:       "omitido: %s (%s)",
	pipeStdinLabel:      "--- entrada estándar (%s) ---",
	CommandGroups: map[string]string{
		"session":   "Sesión",
		"model":     "Modelo y razonamiento",
		"usage":     "Consumo y saldo",
		"project":   "Proyecto y herramientas",
		"shell":     "Shell nativo",
		"clipboard": "Portapapeles y archivos",
	},
	CommandDescs: map[string]string{
		"help":        "catálogo completo o ficha de un tema",
		"exit":        "sale del chat",
		"login":       "configura las credenciales (API key)",
		"approvals":   "consulta o cambia si se pide permiso para cada herramienta",
		"models":      "cambia y guarda el modelo por defecto (sin argumento abre el selector)",
		"model":       "alias de /models",
		"list-models": "lista los modelos disponibles en la API",
		"thinking":    "consulta o cambia el modo de razonamiento",
		"effort":      "consulta o cambia el esfuerzo de razonamiento",
		"tokens":      "muestra el consumo de tokens de la sesión",
		"compact":     "resume el historial antiguo para liberar contexto",
		"sandbox":     "consulta o cambia el confinamiento de las herramientas",
		"plan":        "activa o desactiva el modo plan (solo lectura hasta aprobar)",
		"todos":       "muestra la lista de tareas del modelo",
		"jobs":        "muestra los trabajos en segundo plano",
		"balance":     "consulta el saldo de la cuenta en la API",
		"context":     "muestra el contexto del proyecto cargado",
		"mcp":         "muestra el estado de los servidores MCP",
		"shell":       "muestra el shell nativo detectado y su configuración",
		"run":         "ejecuta una orden en el shell nativo y muestra la salida",
		"copy":        "copia la última respuesta al portapapeles",
		"save":        "guarda la conversación en un archivo Markdown",

		"models reset":   "vuelve al modelo por defecto de la API",
		"context reload": "recarga el contexto del proyecto",
		"copy all":       "copia toda la conversación al portapapeles",
	},
	CommandDetails: map[string]string{
		"help": "Sin argumento muestra el catálogo completo: comandos, atajos de teclado, " +
			"skills del proyecto y servidores MCP con sus herramientas. Con un tema muestra " +
			"su ficha: uso, alias y descripción.",
		"sandbox":   "Confinamiento de las herramientas. `read-only` deniega escribir y ejecutar comandos; `workspace-write` (por defecto) permite escribir solo dentro del proyecto; `full` quita el confinamiento de rutas. Es independiente de /approvals: una perilla decide si te pregunto, la otra hasta dónde puedo llegar. Los comandos destructivos siempre piden confirmación, incluso en modo auto.",
		"approvals": "Muestra si se pide permiso en cada llamada y lo cambia con 'ask' o 'auto'. En 'auto' cualquier comando se ejecuta sin avisar; el estado se ve en la cabecera de la TUI y se restaura con /approvals ask. Dura lo que la sesión.",
		"exit": "Cierra la sesión. Las conversaciones ya guardadas siguen en el historial " +
			"(deekseep history).",
		"login": "Guarda la API key en el llavero del sistema. Sin argumento la pide de forma " +
			"oculta, que es lo recomendable; con argumento la toma de la línea de comandos y " +
			"queda en el historial del shell. También puedes usar DEEPSEEK_API_KEY.",
		"models": "Cambia el modelo por defecto. Sin argumento consulta la API y abre un selector " +
			"numerado; con un ID lo fija directamente; 'reset' vuelve al modelo por defecto de la API.",
		"model": "Alias de /models. Acepta exactamente los mismos argumentos.",
		"list-models": "Muestra los modelos que la API ofrece en este momento, marcando con * el que " +
			"está en uso. No cambia nada.",
		"thinking": "Consulta o cambia el modo de razonamiento de los modelos que lo soportan. " +
			"'on'/'enabled' lo activa, 'off'/'disabled' lo desactiva. Sin argumento muestra el valor actual.",
		"effort": "Consulta o cambia el esfuerzo de razonamiento: 'low', 'high' o 'max'. Más esfuerzo " +
			"suele dar mejores respuestas a costa de más latencia. Sin argumento muestra el valor actual.",
		"tokens": "Muestra los tokens de la última petición, el contexto frente a la ventana del modelo y el " +
			"acumulado de la sesión. Los números los reporta la propia API, no son una estimación.",
		"balance": "Consulta GET /user/balance y muestra el saldo disponible por moneda, con lo recargado y lo " +
			"de regalo. Necesita conexión y la API key; no consume tokens.",
		"context": "Muestra lo que el CLI carga del proyecto (AGENTS.md, DESIGN.md, README.md, skills y " +
			"estado de git) y que se envía al modelo en cada turno. 'reload' lo vuelve a leer del disco.",
		"mcp": "Lista los servidores MCP configurados en mcp.json, si consiguieron conectar y qué " +
			"herramientas expone cada uno.",
		"shell": "Muestra el shell nativo detectado para este sistema, si la herramienta run_shell " +
			"está activa y el timeout configurado.",
		"run": "Ejecuta una orden en el shell nativo y muestra su salida. No pasa por el modelo y no " +
			"pide permiso, porque la orden la escribes tú. Respeta el timeout configurado.",
		"copy": "Copia al portapapeles. Sin argumento copia la última respuesta; con 'all' copia la " +
			"conversación completa en texto plano, sin colores ni códigos ANSI. Usa el portapapeles " +
			"nativo y si no está disponible cae a OSC 52, así que también funciona por SSH.",
		"save": "Guarda la conversación en Markdown. Sin ruta usa deekseep-chat-<fecha>.md en el " +
			"directorio actual; con ruta, en el archivo indicado.",
	},
	ShellToolDescription: "Ejecuta una orden en el shell nativo del sistema operativo (%s) y devuelve la salida combinada de stdout y stderr, el código de salida y la duración. Úsala para inspeccionar el proyecto, compilar, ejecutar tests o usar git. Cada orden requiere permiso explícito del usuario.",
	ShellParamCommand:    "Orden a ejecutar, con la sintaxis del shell nativo indicado.",
	ShellParamDir:        "Directorio de trabajo (opcional; por defecto, el del proyecto).",
	ShellParamTimeout:    "Tiempo máximo de ejecución en segundos (opcional).",
	ShellAlwaysWarning:   "atención: 'a' permite ejecutar CUALQUIER orden durante el resto de la sesión.",
	ShellDisabled:        "La herramienta de shell está desactivada. Actívala con `deekseep config set shell on`.",
	ShellDetected:        "shell nativo: %s (%s)",
	RunUsage:             "uso: /run <orden>",
	ShellEmptyCommand:    "Orden vacía.",
	ShellNoOutput:        "(sin salida)",
	ShellTimeoutLabel:    "tiempo máximo",
	shellExitCode:        "código de salida: %d",
	shellTimedOut:        "la orden agotó el tiempo límite tras %s",
	EnvTitle:             "Entorno de ejecución:",
	EnvOS:                "Sistema operativo",
	EnvShell:             "Shell nativo",
	EnvDir:               "Directorio de trabajo",
	EnvTool:              "Herramienta de shell",
	EnvToolOn:            "run_shell disponible (pide permiso en cada orden)",
	EnvToolOff:           "run_shell desactivada (config: shell)",
	EnvSyntax:            "Al proponer órdenes, usa la sintaxis de ese shell y no asumas bash si es PowerShell.",
	EnvNow:               "Fecha y hora actual",
	EnvNowHint:           "Si te preguntan la fecha, la hora o el día de la semana, usa ese valor tal cual: no lo calcules ni lo deduzcas de tu entrenamiento.",
	TokensTitle:          "Consumo de tokens:",
	TokensModel:          "modelo",
	TokensEffort:         "esfuerzo",
	TokensLast:           "última petición",
	TokensContext:        "contexto",
	TokensSession:        "sesión",
	TokensTotal:          "total",
	TokensCache:          "caché de prompt",
	TokensNone:           "Aún no hay peticiones en esta sesión.",
	TokensNext:           "siguiente petición",
	ToolNotStarted:       "No se ejecutó: la conversación se interrumpió antes de resolver el permiso.",
	BalanceTitle:         "Saldo de la cuenta:",
	BalanceOK:            "disponible",
	BalanceUnavailable:   "NO disponible (saldo agotado o límite alcanzado)",
	BalanceTotal:         "total",
	BalanceToppedUp:      "recargado",
	BalanceGranted:       "regalo",
	BalanceEmpty:         "la API no devolvió información de saldo",
	BalanceFailed:        "no se pudo consultar el saldo:",
	ContextModeLabel:     "modo",
	ContextTotal:         "total",
	ContextOmitted:       "no incluido por el modo",
	ContextAll:           "todo el contexto del proyecto",
	ContextSaveLight:     "ahorrarías con 'light'",
	TokensProject:        "contexto proyecto",
	ContextParts: map[string]string{
		"header": "encabezado",
		"agents": "AGENTS.md",
		"design": "DESIGN.md",
		"readme": "README.md",
		"skills": "skills",
		"git":    "git",
	},
}

var en = &Messages{
	Lang:                 EN,
	Welcome:              "DeepSeek coder — type your message, 'exit' to quit.",
	NoAPIKey:             "No API key set: run `deekseep config set api_key <key>` or set DEEPSEEK_API_KEY",
	EnterAPIKey:          "Enter your DeepSeek API key (hidden while typing):",
	Saved:                "Saved.",
	Removed:              "Removed.",
	NoConversations:      "No conversations yet.",
	EmptyQuestion:        "Empty question: use `deekseep ask \"<question>\"`",
	HistoryDisabled:      "warning: history disabled:",
	SessionOpenFailed:    "could not open the session:",
	CompactionNoSession:  "this conversation has no saved history",
	CompactionNoWindow:   "the model context window is unknown; cannot decide when to compact",
	CompactionNothing:    "there is nothing to compact yet",
	CompactionEmpty:      "the model returned an empty summary",
	CompactionFailed:     "could not compact:",
	NoTodos:              "there is no task list yet",
	NoMatches:            "no matches in previous conversations",
	HistorySearchUsage:   "usage: deekseep history search <text>",
	PlanOn:               "PLAN MODE on: read-only until you approve the plan",
	QuestionHelp:         "1-9 pick · ↑↓ move · Space mark · Enter confirm · Esc cancel",
	PlanOff:              "plan mode off",
	ShellParamBackground: "When true the command runs in the background and you get a message when it finishes. Use it for long builds, test suites or servers.",
	JobsTitle:            "Background jobs:",
	JobsNone:             "no jobs are running",
	JobStarted:           "job %d started",
	JobFinished:          "job %d finished",
	TodosTitle:           "Tasks:",
	SandboxTitle:         "Confinement mode:",
	SandboxUsage:         "usage: /sandbox [read-only|workspace-write|full]",
	SandboxReadOnly:      "read-only: writes and commands are denied",
	SandboxWorkspace:     "writes only inside the project (default)",
	SandboxFull:          "no path confinement (dangerous)",
	InvalidSandboxValue:  "Invalid value: use 'read-only', 'workspace-write' or 'full'",
	InvalidApprovalValue: "Invalid value: use 'ask' or 'auto'",
	toolBlocked:          "Tool %s blocked by %s mode",
	prunedLine:           "pruned ~%d tokens of old tool output",
	compactionRunning:    "summarising ~%d tokens of history...",
	compactionDone:       "history compacted: ≈%d → ≈%d tokens",
	sessionResumed:       "session %s resumed",
	sessionMissing:       "session %q does not exist",
	sessionDeleted:       "session %s deleted",
	historyTurns:         "%d turns",
	SaveFailed:           "warning: could not save message:",
	SelectLanguage:       "Selecciona tu idioma / Select your language:\n  1) Español\n  2) English\n> ",
	LanguageSet:          "Language set: English.",
	InvalidLangValue:     "Invalid language: use 'es' or 'en'",
	NotSet:               "(not set)",
	Default:              "(default)",
	ConfigUsage:          "usage: deekseep config get|set|unset",
	SetUsage:             "usage: deekseep config set <key> <value>",
	UnsetUsage:           "usage: deekseep config unset <key>",
	HistoryShowUsage:     "usage: deekseep history show <id>",
	HistoryDeleteUsage:   "usage: deekseep history delete <id>",
	Usage: `deekseep-coder — DeepSeek coding assistant

Usage:
  deekseep [--debug]          start an interactive chat (--debug: API logs)
  deekseep ask "<question>"   ask a single question
  deekseep --pipe [question]  pipe content through stdin (files, logs, errors)
  deekseep --json ask ...     NDJSON output (start/delta/usage/done/error) for scripts
  deekseep -c                 continue the most recent saved session
  deekseep --resume <id>      resume a specific session
  deekseep config get         show current configuration
  deekseep config set <key> <value>
                              set a config value (api_key, model, base_url, lang,
                              thinking, reasoning_effort, shell, shell_timeout, context)
  deekseep config unset <key> remove a config value
  deekseep history            list saved conversations
  deekseep history show <id>  show a conversation
  deekseep history delete <id>
                              delete a conversation
  deekseep models             list models available on the API
  deekseep balance            check the account balance
  deekseep mcp                show MCP servers and their tools
  deekseep help               show this help

Autocompletion in the chat:
  @<path>   completes file paths (Tab)
  /<cmd>    completes commands, skills and MCP tools (Tab)

Environment:
  DEEPSEEK_API_KEY   API key (overrides keyring and config file)
  DEEPSEEK_MODEL     model to use (overrides config file)
`,
	SystemPrompt:               "You are a helpful coding assistant. Answer concisely and provide code when relevant. Respond in English.",
	conversationNotFound:       "Conversation %d not found",
	conversationDeleted:        "Conversation %d deleted.",
	invalidID:                  "Invalid conversation id %q",
	unknownCommand:             "Unknown command %q",
	unknownConfigKey:           "Unknown config key %q (valid: api_key, model, base_url, lang, thinking, reasoning_effort, shell, shell_timeout, context)",
	unknownHistoryCmd:          "Unknown history command %q (valid: show, delete)",
	modelCleared:               "Model reset: %s",
	currentModel:               "Current model: %s",
	unknownSlashCmd:            "Unknown command: %s (try /help)",
	modelNotAvailable:          "Model %s is no longer available. Pick another:",
	modelNotListed:             "Model %s is not in the available models list. If it does not exist, the API will return an error.",
	modelSetDefault:            "Default model: %s",
	modelInvalidNonInteractive: "Configured model %s is not available and there is no interactive terminal to pick another. Run `deekseep models` or `deekseep config set model <id>`.",
	ModelsTitle:                "Available models (* = in use):",
	NoModels:                   "No models available.",
	ModelUsage:                 "usage: /models <id> — switch and save the default model",
	SelectModel:                "Select the default model:",
	InvalidModelSelection:      "Invalid selection. Enter a number or a model ID.",
	ModelsUnavailable:          "warning: could not fetch the models list:",
	NoModelNonInteractive:      "No model configured and no interactive terminal to pick one. Run `deekseep config set model <id>`.",
	EditorFallback:             "warning: could not enable the multiline editor, using simple input:",
	ContextTitle:               "Project context:",
	NoContext:                  "No project context loaded.",
	ContextReloaded:            "Context reloaded.",
	ContextUsage:               "usage: /context [reload]",
	ToolPrompt:                 "Allow? [y]es / [n]o / [a]uto (stop asking) / [d]eny this tool: ",
	ApprovalsTitle:             "Tool permissions:",
	ApprovalStateAsk:           "asks before every call",
	ApprovalStateAuto:          "does NOT ask (any command runs silently)",
	ApprovalsBlocked:           "denied tools",
	ApprovalUsage:              "usage: /approvals [ask|auto]",
	ApprovalAutoWarning:        "warning: I will no longer ask for permission in this session; restore with /approvals ask",
	ToolDeniedContent:          "The user denied this tool call.",
	ToolLoopLimit:              "Too many tool rounds. Aborting.",
	NoMCP:                      "No MCP servers connected.",
	MCPError:                   "warning: MCP error:",
	NoMCPServers:               "No MCP servers configured (mcp.json).",
	LoginOK:                    "Credentials saved.",
	InvalidThinkingValue:       "Invalid value: use 'enabled' or 'disabled' (or 'on'/'off')",
	InvalidEffortValue:         "Invalid value: use 'low', 'high' or 'max'",
	InvalidMouseValue:          "Invalid value: use 'on' or 'off'",
	InvalidShellValue:          "Invalid value: use 'on' or 'off'",
	InvalidShellTimeout:        "Invalid value: use a number of seconds between 1 and 600",
	InvalidContextValue:        "Invalid value: use 'full', 'light' or 'off'",
	InvalidCompactValue:        "Invalid value: use 'auto' or 'off'",
	InvalidWindowValue:         "Invalid value: use a token count greater than 0",
	TUIInputPlaceholder:        "Type a message... (Enter sends, Ctrl+J new line)",
	TUIThinking:                "thinking...",
	ModelSelectPlaceholder:     "model number or ID",
	ModelSelectHint:            "Select a model (number or ID)",
	TUIKeySend:                 "send",
	TUIKeyNewline:              "new line",
	TUIKeyComplete:             "complete",
	TUIKeyScroll:               "scroll",
	TUIKeyQuit:                 "quit",
	TUIKeyCopy:                 "copy",
	TUIHints:                   "Wheel: scroll · Select text with the cursor any time · Ctrl+Y: copy last reply · /copy all: copy everything · /save: save to file",
	UserLabel:                  "you",
	AssistantLabel:             "deepseek",
	CopyFailed:                 "could not copy to the clipboard:",
	SaveTranscriptFailed:       "could not save the conversation:",
	copied:                     "%d characters copied to the clipboard.",
	transcriptSaved:            "Conversation saved to %s",
	slowResponse:               "The model is taking a while to respond (%s)... You can wait or press Ctrl+C.",
	toolRequest:                "The model wants to run tool %s:\n%s\n",
	toolDenied:                 "Tool %s denied by the user.",
	toolRunning:                "Running %s...",
	mcpFailed:                  "warning: MCP server %s unavailable: %v\n",
	mcpConnected:               "MCP %s connected (%d tools).\n",
	skillRef:                   "[Skill: %s]",
	toolRef:                    "[MCP tool: %s]",
	HelpTitle:                  "DeepSeek Coder — full reference",
	HelpSectionCommands:        "Commands",
	HelpSectionKeys:            "Keyboard shortcuts",
	HelpSectionSkills:          "Project skills",
	HelpSectionMCP:             "MCP servers",
	HelpNoSkills:               "No skills in this project (.agents/skills).",
	HelpNoMCP:                  "No MCP servers connected (mcp.json).",
	HelpNoTools:                "no tools",
	HelpNoDescription:          "no description",
	HelpFooter: `Type /help <topic> to see the card of a command, a skill or an MCP tool
(for example: /help copy or /help run).

You can select text with the cursor at any time and copy it: the TUI does not
capture the mouse. Ctrl+Y copies the last reply without selecting anything.`,
	HelpTopicUsage:      "Usage",
	HelpTopicAliases:    "Aliases",
	HelpTopicServer:     "Server",
	HelpSubcommands:     "Subcommands:",
	HelpSkillTag:        "project skill",
	HelpToolTag:         "MCP tool",
	helpSkillHowTo:      "Reference it by typing %s in a message.",
	helpToolHowTo:       "The model uses it like any other tool, always with your permission.",
	helpUnknownTopic:    "Nothing is documented under the name %q. Try /help for the catalog.",
	AttachPrefix:        "📎 ",
	AttachNotFound:      "not found",
	AttachDirectory:     "it is a directory",
	AttachTooLarge:      "too large",
	AttachBinary:        "binary file",
	AttachBudget:        "budget exhausted",
	AttachReadFailed:    "could not read",
	PipeUsage:           "usage: deekseep --pipe [question] — the question can also come from stdin",
	attachHeader:        "Attached files (%s):",
	attachFileTag:       "<file: %s>",
	attachItem:          "%s (%s)",
	attachItemTruncated: "%s (%s, truncated)",
	attachSkipped:       "skipped: %s (%s)",
	pipeStdinLabel:      "--- stdin (%s) ---",
	CommandGroups: map[string]string{
		"session":   "Session",
		"model":     "Model and reasoning",
		"usage":     "Usage and balance",
		"project":   "Project and tools",
		"shell":     "Native shell",
		"clipboard": "Clipboard and files",
	},
	CommandDescs: map[string]string{
		"help":        "full catalog, or the card of one topic",
		"exit":        "quit the chat",
		"login":       "set your credentials (API key)",
		"approvals":   "show or change whether every tool call asks for permission",
		"models":      "switch and save the default model (no argument opens the picker)",
		"model":       "alias for /models",
		"list-models": "list the models available in the API",
		"thinking":    "show or change the reasoning mode",
		"effort":      "show or change the reasoning effort",
		"tokens":      "show the token usage of this session",
		"compact":     "summarise old history to free context",
		"sandbox":     "show or change how far the tools may reach",
		"plan":        "toggle plan mode (read-only until the plan is approved)",
		"todos":       "show the model task list",
		"jobs":        "show the background jobs",
		"balance":     "check the account balance on the API",
		"context":     "show the loaded project context",
		"mcp":         "show MCP server status",
		"shell":       "show the detected native shell and its settings",
		"run":         "run a command in the native shell and show the output",
		"copy":        "copy the last reply to the clipboard",
		"save":        "save the conversation to a Markdown file",

		"models reset":   "go back to the API default model",
		"context reload": "reload the project context",
		"copy all":       "copy the whole conversation to the clipboard",
	},
	CommandDetails: map[string]string{
		"help": "With no argument it shows the full catalog: commands, keyboard shortcuts, " +
			"project skills and MCP servers with their tools. With a topic it shows that " +
			"entry's card: usage, aliases and description.",
		"sandbox":   "How far the tools may reach. `read-only` denies writes and command execution; `workspace-write` (default) writes only inside the project; `full` drops the path confinement. It is independent from /approvals: one knob decides whether I ask, the other how far I can go. Destructive commands always ask, even in auto mode.",
		"approvals": "Shows whether every call asks for permission and switches it with 'ask' or 'auto'. In 'auto' any command runs silently; the state is visible in the TUI header and restored with /approvals ask. It lasts for the session.",
		"exit": "Closes the session. Conversations that were already saved stay in the history " +
			"(deekseep history).",
		"login": "Stores the API key in the system keyring. With no argument it asks for it hidden, " +
			"which is the recommended way; with an argument it reads it from the command line and it " +
			"ends up in the shell history. You can also use DEEPSEEK_API_KEY.",
		"models": "Switches the default model. With no argument it queries the API and opens a numbered " +
			"picker; with an ID it sets it directly; 'reset' goes back to the API default model.",
		"model": "Alias for /models. It accepts exactly the same arguments.",
		"list-models": "Shows the models the API offers right now, marking the one in use with *. " +
			"It changes nothing.",
		"thinking": "Shows or changes the reasoning mode of the models that support it. 'on'/'enabled' " +
			"turns it on, 'off'/'disabled' turns it off. With no argument it shows the current value.",
		"effort": "Shows or changes the reasoning effort: 'low', 'high' or 'max'. More effort usually " +
			"gives better answers at the cost of latency. With no argument it shows the current value.",
		"tokens": "Shows the tokens of the last request, the context against the model window and the session " +
			"totals. The numbers come from the API itself, they are not an estimate.",
		"balance": "Queries GET /user/balance and shows the balance per currency, with the topped-up and granted " +
			"parts. It needs network access and the API key; it consumes no tokens.",
		"context": "Shows what the CLI loads from the project (AGENTS.md, DESIGN.md, README.md, skills " +
			"and git status) and sends to the model on every turn. 'reload' re-reads it from disk.",
		"mcp": "Lists the MCP servers configured in mcp.json, whether they connected and which tools " +
			"each one exposes.",
		"shell": "Shows the native shell detected for this system, whether the run_shell tool is " +
			"enabled and the configured timeout.",
		"run": "Runs a command in the native shell and shows its output. It does not go through the " +
			"model and asks no permission, because you are the one typing it. It honours the configured timeout.",
		"copy": "Copies to the clipboard. With no argument it copies the last reply; with 'all' it " +
			"copies the whole conversation as plain text, without colours or ANSI codes. It uses the " +
			"native clipboard and falls back to OSC 52, so it also works over SSH.",
		"save": "Saves the conversation as Markdown. With no path it uses deekseep-chat-<date>.md in " +
			"the current directory; with a path, that file.",
	},
	ShellToolDescription: "Run a command in the operating system's native shell (%s) and return the combined stdout and stderr, the exit code and the duration. Use it to inspect the project, build, run tests or use git. Every command requires the user's explicit permission.",
	ShellParamCommand:    "Command to run, using the syntax of the native shell above.",
	ShellParamDir:        "Working directory (optional; defaults to the project directory).",
	ShellParamTimeout:    "Maximum run time in seconds (optional).",
	ShellAlwaysWarning:   "warning: 'a' allows ANY command for the rest of this session.",
	ShellDisabled:        "The shell tool is disabled. Enable it with `deekseep config set shell on`.",
	ShellDetected:        "native shell: %s (%s)",
	RunUsage:             "usage: /run <command>",
	ShellEmptyCommand:    "Empty command.",
	ShellNoOutput:        "(no output)",
	ShellTimeoutLabel:    "timeout",
	shellExitCode:        "exit code: %d",
	shellTimedOut:        "command timed out after %s",
	EnvTitle:             "Execution environment:",
	EnvOS:                "Operating system",
	EnvShell:             "Native shell",
	EnvDir:               "Working directory",
	EnvTool:              "Shell tool",
	EnvToolOn:            "run_shell available (asks for permission on every command)",
	EnvToolOff:           "run_shell disabled (config: shell)",
	EnvSyntax:            "When proposing commands, use that shell's syntax and do not assume bash if it is PowerShell.",
	EnvNow:               "Current date and time",
	EnvNowHint:           "If asked about the date, the time or the weekday, use that value verbatim: do not compute it or infer it from your training data.",
	TokensTitle:          "Token usage:",
	TokensModel:          "model",
	TokensEffort:         "effort",
	TokensLast:           "last request",
	TokensContext:        "context",
	TokensSession:        "session",
	TokensTotal:          "total",
	TokensCache:          "prompt cache",
	TokensNone:           "No requests in this session yet.",
	TokensNext:           "next request",
	ToolNotStarted:       "Not executed: the conversation was interrupted before the permission was answered.",
	BalanceTitle:         "Account balance:",
	BalanceOK:            "available",
	BalanceUnavailable:   "NOT available (out of balance or limit reached)",
	BalanceTotal:         "total",
	BalanceToppedUp:      "topped up",
	BalanceGranted:       "granted",
	BalanceEmpty:         "the API returned no balance information",
	BalanceFailed:        "could not fetch the balance:",
	ContextModeLabel:     "mode",
	ContextTotal:         "total",
	ContextOmitted:       "not included by the mode",
	ContextAll:           "the whole project context",
	ContextSaveLight:     "would be saved with 'light'",
	TokensProject:        "project context",
	ContextParts: map[string]string{
		"header": "header",
		"agents": "AGENTS.md",
		"design": "DESIGN.md",
		"readme": "README.md",
		"skills": "skills",
		"git":    "git",
	},
}

// For returns the messages for a language (defaults to English).
func For(l Lang) *Messages {
	if l == ES {
		return es
	}
	return en
}

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

	Welcome                string
	NoAPIKey               string
	EnterAPIKey            string
	Saved                  string
	Removed                string
	NoConversations        string
	EmptyQuestion          string
	HistoryDisabled        string
	SaveFailed             string
	SelectLanguage         string
	LanguageSet            string
	InvalidLangValue       string
	NotSet                 string
	Default                string
	ConfigUsage            string
	SetUsage               string
	UnsetUsage             string
	HistoryShowUsage       string
	HistoryDeleteUsage     string
	Usage                  string
	SystemPrompt           string
	ModelsTitle            string
	NoModels               string
	ModelUsage             string
	SlashHelp              string
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
	ToolDeniedContent      string
	ToolLoopLimit          string
	NoMCP                  string
	MCPError               string
	NoMCPServers           string
	LoginOK                string
	InvalidThinkingValue   string
	InvalidEffortValue     string
	TUIInputPlaceholder    string
	TUIThinking            string
	ModelSelectPlaceholder string
	ModelSelectHint        string
	TUIKeySend             string
	TUIKeyNewline          string
	TUIKeyComplete         string
	TUIKeyScroll           string
	TUIKeyQuit             string

	slowResponse string

	toolRequest  string
	toolDenied   string
	toolRunning  string
	mcpFailed    string
	mcpConnected string
	skillRef     string
	toolRef      string

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
	Lang:               ES,
	Welcome:            "DeepSeek coder — escribe tu mensaje, 'exit' para salir.",
	NoAPIKey:           "No hay API key configurada: ejecuta `deekseep config set api_key <clave>` o define DEEPSEEK_API_KEY",
	EnterAPIKey:        "Introduce tu API key de DeepSeek (no se mostrará al escribir):",
	Saved:              "Guardado.",
	Removed:            "Eliminado.",
	NoConversations:    "Aún no hay conversaciones.",
	EmptyQuestion:      "Pregunta vacía: usa `deekseep ask \"<pregunta>\"`",
	HistoryDisabled:    "aviso: historial desactivado:",
	SaveFailed:         "aviso: no se pudo guardar el mensaje:",
	SelectLanguage:     "Selecciona tu idioma / Select your language:\n  1) Español\n  2) English\n> ",
	LanguageSet:        "Idioma configurado: español.",
	InvalidLangValue:   "Idioma inválido: usa 'es' o 'en'",
	NotSet:             "(sin configurar)",
	Default:            "(por defecto)",
	ConfigUsage:        "uso: deekseep config get|set|unset",
	SetUsage:           "uso: deekseep config set <clave> <valor>",
	UnsetUsage:         "uso: deekseep config unset <clave>",
	HistoryShowUsage:   "uso: deekseep history show <id>",
	HistoryDeleteUsage: "uso: deekseep history delete <id>",
	Usage: `deekseep-coder — asistente de código con DeepSeek

Uso:
  deekseep [--debug]            inicia un chat interactivo (--debug: logs de la API)
  deekseep ask "<pregunta>"     hace una consulta puntual
  deekseep config get           muestra la configuración actual
  deekseep config set <clave> <valor>
                                configura un valor (api_key, model, base_url, lang)
  deekseep config unset <clave> elimina un valor de configuración
  deekseep history              lista las conversaciones guardadas
  deekseep history show <id>    muestra una conversación
  deekseep history delete <id>  elimina una conversación
  deekseep models               lista los modelos disponibles de la API
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
	unknownConfigKey:           "Clave de configuración desconocida %q (válidas: api_key, model, base_url, lang, thinking, reasoning_effort)",
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
	ToolPrompt:                 "¿Permitir? [y]es / [n]o / [a]lways / [d]eny: ",
	ToolDeniedContent:          "El usuario denegó esta llamada a herramienta.",
	ToolLoopLimit:              "Demasiadas rondas de herramientas. Abortando.",
	NoMCP:                      "No hay servidores MCP conectados.",
	MCPError:                   "aviso: error de MCP:",
	NoMCPServers:               "No hay servidores MCP configurados (mcp.json).",
	LoginOK:                    "Credenciales guardadas.",
	InvalidThinkingValue:       "Valor inválido: usa 'enabled' o 'disabled' (o 'on'/'off')",
	InvalidEffortValue:         "Valor inválido: usa 'low', 'high' o 'max'",
	TUIInputPlaceholder:        "Escribe un mensaje... (Enter envía, Alt+Enter nueva línea)",
	TUIThinking:                "pensando...",
	ModelSelectPlaceholder:     "número o ID del modelo",
	ModelSelectHint:            "Selecciona un modelo (número o ID)",
	TUIKeySend:                 "enviar",
	TUIKeyNewline:              "nueva línea",
	TUIKeyComplete:             "completar",
	TUIKeyScroll:               "scroll",
	TUIKeyQuit:                 "salir",
	slowResponse:               "El modelo está tardando en responder (%s)... Puedes esperar o pulsar Ctrl+C.",
	toolRequest:                "El modelo quiere ejecutar la herramienta %s:\n%s\n",
	toolDenied:                 "Herramienta %s denegada por el usuario.",
	toolRunning:                "Ejecutando %s...",
	mcpFailed:                  "aviso: servidor MCP %s no disponible: %v\n",
	mcpConnected:               "MCP %s conectado (%d herramientas).\n",
	skillRef:                   "[Skill: %s]",
	toolRef:                    "[Herramienta MCP: %s]",
	SlashHelp: `Comandos de la sesión:
  /login           configura las credenciales (API key)
  /exit            sale del chat
  /models [id]     cambia y guarda el modelo por defecto
  /list-models     lista los modelos disponibles
  /model [id]      alias de /models
  /mcp             muestra el estado de los servidores MCP
  /context         muestra el contexto del proyecto cargado
  /context reload  recarga el contexto del proyecto
  /help            muestra esta ayuda

También puedes escribir /<skill> o /<herramienta-mcp> para referenciar una
skill o herramienta en tu mensaje.
`,
}

var en = &Messages{
	Lang:               EN,
	Welcome:            "DeepSeek coder — type your message, 'exit' to quit.",
	NoAPIKey:           "No API key set: run `deekseep config set api_key <key>` or set DEEPSEEK_API_KEY",
	EnterAPIKey:        "Enter your DeepSeek API key (hidden while typing):",
	Saved:              "Saved.",
	Removed:            "Removed.",
	NoConversations:    "No conversations yet.",
	EmptyQuestion:      "Empty question: use `deekseep ask \"<question>\"`",
	HistoryDisabled:    "warning: history disabled:",
	SaveFailed:         "warning: could not save message:",
	SelectLanguage:     "Selecciona tu idioma / Select your language:\n  1) Español\n  2) English\n> ",
	LanguageSet:        "Language set: English.",
	InvalidLangValue:   "Invalid language: use 'es' or 'en'",
	NotSet:             "(not set)",
	Default:            "(default)",
	ConfigUsage:        "usage: deekseep config get|set|unset",
	SetUsage:           "usage: deekseep config set <key> <value>",
	UnsetUsage:         "usage: deekseep config unset <key>",
	HistoryShowUsage:   "usage: deekseep history show <id>",
	HistoryDeleteUsage: "usage: deekseep history delete <id>",
	Usage: `deekseep-coder — DeepSeek coding assistant

Usage:
  deekseep [--debug]          start an interactive chat (--debug: API logs)
  deekseep ask "<question>"   ask a single question
  deekseep config get         show current configuration
  deekseep config set <key> <value>
                              set a config value (api_key, model, base_url, lang)
  deekseep config unset <key> remove a config value
  deekseep history            list saved conversations
  deekseep history show <id>  show a conversation
  deekseep history delete <id>
                              delete a conversation
  deekseep models             list models available on the API
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
	unknownConfigKey:           "Unknown config key %q (valid: api_key, model, base_url, lang, thinking, reasoning_effort)",
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
	ToolPrompt:                 "Allow? [y]es / [n]o / [a]lways / [d]eny: ",
	ToolDeniedContent:          "The user denied this tool call.",
	ToolLoopLimit:              "Too many tool rounds. Aborting.",
	NoMCP:                      "No MCP servers connected.",
	MCPError:                   "warning: MCP error:",
	NoMCPServers:               "No MCP servers configured (mcp.json).",
	LoginOK:                    "Credentials saved.",
	InvalidThinkingValue:       "Invalid value: use 'enabled' or 'disabled' (or 'on'/'off')",
	InvalidEffortValue:         "Invalid value: use 'low', 'high' or 'max'",
	TUIInputPlaceholder:        "Type a message... (Enter sends, Alt+Enter new line)",
	TUIThinking:                "thinking...",
	ModelSelectPlaceholder:     "model number or ID",
	ModelSelectHint:            "Select a model (number or ID)",
	TUIKeySend:                 "send",
	TUIKeyNewline:              "new line",
	TUIKeyComplete:             "complete",
	TUIKeyScroll:               "scroll",
	TUIKeyQuit:                 "quit",
	slowResponse:               "The model is taking a while to respond (%s)... You can wait or press Ctrl+C.",
	toolRequest:                "The model wants to run tool %s:\n%s\n",
	toolDenied:                 "Tool %s denied by the user.",
	toolRunning:                "Running %s...",
	mcpFailed:                  "warning: MCP server %s unavailable: %v\n",
	mcpConnected:               "MCP %s connected (%d tools).\n",
	skillRef:                   "[Skill: %s]",
	toolRef:                    "[MCP tool: %s]",
	SlashHelp: `Session commands:
  /login           set your credentials (API key)
  /exit            quit the chat
  /models [id]     switch and save the default model
  /list-models     list available models
  /model [id]      alias for /models
  /mcp             show MCP server status
  /context         show the loaded project context
  /context reload  reload the project context
  /help            show this help

You can also type /<skill> or /<mcp-tool> to reference a skill or tool
in your message.
`,
}

// For returns the messages for a language (defaults to English).
func For(l Lang) *Messages {
	if l == ES {
		return es
	}
	return en
}

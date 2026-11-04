package main

import (
	"fmt"
	"sort"
	"strings"

	"deepseek-coder/internal/i18n"
)

// slashCommand is one entry of the command registry.
type slashCommand struct {
	// Name is the command without the leading slash.
	Name string
	// Args is the argument hint shown in the help, e.g. "[id]".
	Args string
	// Aliases are alternative names that run the same command.
	Aliases []string
	// Subcommands are extra completion entries, e.g. "all" for "/copy all".
	Subcommands []string
	// Group is the help section this command belongs to.
	Group string
	// Hidden keeps a command out of the help listing while staying usable.
	Hidden bool
}

// names returns every name the command answers to.
func (c slashCommand) names() []string {
	out := append([]string{c.Name}, c.Aliases...)
	return out
}

// usage returns the command with its argument hint, e.g. "/save [ruta]".
func (c slashCommand) usage() string {
	if c.Args == "" {
		return "/" + c.Name
	}
	return "/" + c.Name + " " + c.Args
}

// slashCommandGroups is the order in which help sections are printed.
var slashCommandGroups = []string{"session", "model", "usage", "project", "shell", "clipboard"}

// slashCommands is the single source of truth for slash commands. The help
// text, the Tab autocompletion and the accepted names are all derived from it,
// so a new command only has to be registered here (plus a description entry in
// i18n) to show up everywhere.
var slashCommands = []slashCommand{
	{Name: "help", Args: "[tema]", Group: "session"},
	{Name: "exit", Group: "session"},
	{Name: "login", Args: "[clave]", Group: "session"},
	{Name: "approvals", Args: "[ask|auto]", Group: "session"},
	{Name: "sandbox", Args: "[read-only|workspace-write|full]", Group: "session"},
	{Name: "plan", Group: "session"},
	{Name: "todos", Group: "session"},
	{Name: "jobs", Group: "shell"},

	{Name: "models", Args: "[id]", Subcommands: []string{"reset"}, Group: "model"},
	{Name: "model", Args: "[id]", Subcommands: []string{"reset"}, Group: "model", Hidden: true},
	{Name: "list-models", Group: "model"},
	{Name: "thinking", Args: "[on|off]", Group: "model"},
	{Name: "effort", Args: "[low|high|max]", Group: "model"},

	{Name: "tokens", Group: "usage"},
	{Name: "compact", Group: "usage"},
	{Name: "balance", Group: "usage"},

	{Name: "context", Subcommands: []string{"reload"}, Group: "project"},
	{Name: "mcp", Group: "project"},

	{Name: "shell", Group: "shell"},
	{Name: "run", Args: "<orden>", Group: "shell"},

	{Name: "copy", Subcommands: []string{"all"}, Group: "clipboard"},
	{Name: "save", Args: "[ruta]", Group: "clipboard"},
}

// commandNames returns every accepted command name, sorted.
func commandNames() []string {
	var out []string
	for _, c := range slashCommands {
		out = append(out, c.names()...)
	}
	sort.Strings(out)
	return out
}

// commandCompletions returns every completion candidate, including names and
// their subcommand variants ("copy all", "context reload").
func commandCompletions() []string {
	var out []string
	for _, c := range slashCommands {
		out = append(out, c.names()...)
		for _, n := range c.names() {
			for _, sub := range c.Subcommands {
				out = append(out, n+" "+sub)
			}
		}
	}
	sort.Strings(out)
	return out
}

// helpRow is one rendered line of the command help.
type helpRow struct {
	usage string
	desc  string
}

// commandSections renders the grouped command table without a title or a
// footer, so the help catalog in help.go can embed it.
func commandSections(m *i18n.Messages) string {
	rows := make(map[string][]helpRow, len(slashCommandGroups))
	width := 0
	for _, c := range slashCommands {
		if c.Hidden {
			continue
		}
		row := helpRow{usage: c.usage(), desc: m.CommandDescription(c.Name)}
		rows[c.Group] = append(rows[c.Group], row)
		if n := len(row.usage); n > width {
			width = n
		}
		for _, sub := range c.Subcommands {
			usage := "/" + c.Name + " " + sub
			rows[c.Group] = append(rows[c.Group], helpRow{
				usage: usage,
				desc:  m.CommandDescription(c.Name + " " + sub),
			})
			if n := len(usage); n > width {
				width = n
			}
		}
	}

	var b strings.Builder
	for _, group := range slashCommandGroups {
		groupRows := rows[group]
		if len(groupRows) == 0 {
			continue
		}
		b.WriteString("\n" + m.CommandGroup(group) + "\n")
		for _, row := range groupRows {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, row.usage, row.desc)
		}
	}
	return b.String()
}

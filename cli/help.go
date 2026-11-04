package main

import (
	"fmt"
	"strings"

	"deekseep-coder/internal/i18n"
	"deekseep-coder/internal/mcpx"
	"deekseep-coder/internal/project"
)

// keyHelp is one row of the keyboard shortcut section of the help catalog.
type keyHelp struct {
	keys string
	desc string
}

// keyboardHelp lists the key bindings documented by /help.
func keyboardHelp(m *i18n.Messages) []keyHelp {
	return []keyHelp{
		{keys: "Enter", desc: m.TUIKeySend},
		{keys: "Ctrl+J", desc: m.TUIKeyNewline},
		{keys: "Tab", desc: m.TUIKeyComplete},
		{keys: "Ctrl+Y", desc: m.TUIKeyCopy},
		{keys: "PgUp/PgDn", desc: m.TUIKeyScroll},
		{keys: "Ctrl+C", desc: m.TUIKeyQuit},
	}
}

// helpCatalog renders the whole reference: slash commands, keyboard
// shortcuts, project skills and MCP servers with their tools. Everything is
// read from the live objects, so it documents what this session actually has.
func (a *app) helpCatalog() string {
	var b strings.Builder
	b.WriteString(a.msg.HelpTitle)
	b.WriteString("\n")

	b.WriteString("\n" + a.msg.HelpSectionCommands + "\n")
	b.WriteString(commandSections(a.msg))

	b.WriteString("\n" + a.msg.HelpSectionKeys + "\n")
	for _, k := range keyboardHelp(a.msg) {
		fmt.Fprintf(&b, "  %-12s  %s\n", k.keys, k.desc)
	}

	b.WriteString("\n" + a.msg.HelpSectionSkills + "\n")
	skills := a.projectSkills()
	if len(skills) == 0 {
		b.WriteString("  " + a.msg.HelpNoSkills + "\n")
	}
	for _, s := range skills {
		fmt.Fprintf(&b, "  /%-13s  %s\n", s.Name, descriptionOr(s.Description, a.msg.HelpNoDescription))
	}

	b.WriteString("\n" + a.msg.HelpSectionMCP + "\n")
	servers := a.mcpServers()
	if len(servers) == 0 {
		b.WriteString("  " + a.msg.HelpNoMCP + "\n")
	}
	for _, srv := range servers {
		fmt.Fprintf(&b, "  %s\n", srv.Name)
		if len(srv.Tools()) == 0 {
			b.WriteString("    " + a.msg.HelpNoTools + "\n")
			continue
		}
		for _, t := range srv.Tools() {
			fmt.Fprintf(&b, "    %-16s  %s\n", t.Name, descriptionOr(t.Description, a.msg.HelpNoDescription))
		}
	}

	b.WriteString("\n")
	b.WriteString(a.msg.HelpFooter)
	b.WriteString("\n")
	return b.String()
}

// helpTopic renders the detail card for one command, skill or MCP tool.
func (a *app) helpTopic(topic string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(topic), "/")))
	if name == "" {
		return "", false
	}
	if c, ok := findCommand(name); ok {
		return a.commandCard(c), true
	}
	for _, s := range a.projectSkills() {
		if strings.EqualFold(s.Name, name) {
			return a.skillCard(s), true
		}
	}
	for _, srv := range a.mcpServers() {
		for _, t := range srv.Tools() {
			if strings.EqualFold(t.Name, name) {
				return a.mcpToolCard(srv.Name, t.Name, t.Description), true
			}
		}
	}
	return "", false
}

// helpTopics lists every name /help accepts, for autocompletion.
func (a *app) helpTopics() []string {
	var out []string
	for _, c := range slashCommands {
		if c.Hidden {
			continue
		}
		out = append(out, c.Name)
		for _, sub := range c.Subcommands {
			out = append(out, c.Name+" "+sub)
		}
	}
	for _, s := range a.projectSkills() {
		out = append(out, s.Name)
	}
	for _, srv := range a.mcpServers() {
		for _, t := range srv.Tools() {
			out = append(out, t.Name)
		}
	}
	return out
}

// findCommand resolves a name to a registry entry, accepting the bare name,
// an alias and "name subcommand" forms such as "copy all".
func findCommand(name string) (slashCommand, bool) {
	for _, c := range slashCommands {
		for _, n := range c.names() {
			if n == name {
				return c, true
			}
			for _, sub := range c.Subcommands {
				if name == n+" "+sub {
					return c, true
				}
			}
		}
	}
	return slashCommand{}, false
}

// commandCard renders the documentation of one slash command.
func (a *app) commandCard(c slashCommand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", c.usage(), a.msg.CommandDescription(c.Name))
	fmt.Fprintf(&b, "\n%-9s%s\n", a.msg.HelpTopicUsage+":", c.usage())
	if len(c.Aliases) > 0 {
		fmt.Fprintf(&b, "%-9s%s\n", a.msg.HelpTopicAliases+":", slashList(c.Aliases))
	}
	if detail := a.msg.CommandDetail(c.Name); detail != "" {
		b.WriteString("\n" + detail + "\n")
	}
	if len(c.Subcommands) > 0 {
		b.WriteString("\n" + a.msg.HelpSubcommands + "\n")
		for _, sub := range c.Subcommands {
			fmt.Fprintf(&b, "  /%-14s  %s\n", c.Name+" "+sub, a.msg.CommandDescription(c.Name+" "+sub))
		}
	}
	return b.String()
}

// skillCard renders the documentation of one project skill.
func (a *app) skillCard(s project.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", s.Name, a.msg.HelpSkillTag)
	b.WriteString("\n" + descriptionOr(s.Description, a.msg.HelpNoDescription) + "\n")
	b.WriteString("\n" + a.msg.HelpSkillHowTo("/"+s.Name) + "\n")
	return b.String()
}

// mcpToolCard renders the documentation of one MCP tool.
func (a *app) mcpToolCard(server, name, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", name, a.msg.HelpToolTag)
	fmt.Fprintf(&b, "\n%-9s%s\n", a.msg.HelpTopicServer+":", server)
	b.WriteString("\n" + descriptionOr(description, a.msg.HelpNoDescription) + "\n")
	b.WriteString("\n" + a.msg.HelpToolHowTo(name) + "\n")
	return b.String()
}

// projectSkills returns the skills loaded from the project context.
func (a *app) projectSkills() []project.Skill {
	if a.proj == nil {
		return nil
	}
	return a.proj.Skills
}

// mcpServers returns the connected MCP servers.
func (a *app) mcpServers() []*mcpx.Server {
	if a.mcp == nil {
		return nil
	}
	return a.mcp.ConnectedServers()
}

// slashList renders a list of names with leading slashes.
func slashList(names []string) string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "/"+n)
	}
	return strings.Join(out, ", ")
}

func descriptionOr(desc, fallback string) string {
	if strings.TrimSpace(desc) == "" {
		return fallback
	}
	return strings.TrimSpace(desc)
}

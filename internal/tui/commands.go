package tui

// A Command is one entry in the command palette. Commands are data, not
// behaviour: an entry names an action, and the session decides what running it
// means. That split is why the palette can be listed and tested without a
// terminal, and it is why no entry can quietly start a sandbox.
type Command struct {
	// Title is what the user reads in the list.
	Title string
	// Hint is the key that reaches it directly, or a short qualifier.
	Hint string
	// Group orders the list: entries of the same group sit together.
	Group string
	// Action is what selecting the command asks the session to do.
	Action Action
	// Filter are extra words the fuzzy search should also match.
	Filter []string
}

// ActionKind is the class of a palette action.
type ActionKind int

// The actions a command can raise.
const (
	// ActNone is a label, not an action.
	ActNone ActionKind = iota
	// ActShowView switches the workspace to a view.
	ActShowView
	// ActRun sends the current input line to a fresh sandbox.
	ActRun
	// ActExport opens the export picker.
	ActExport
	// ActExportAll exports everything the session produced.
	ActExportAll
	// ActDiscard asks to delete the session workspace.
	ActDiscard
	// ActExit asks to leave the session.
	ActExit
	// ActRefresh re-reads the platform probe and the monitor.
	ActRefresh
	// ActMotion toggles animation.
	ActMotion
	// ActDismissWarnings clears the notice stack.
	ActDismissWarnings
)

// Action is what a command asks the session to do.
type Action struct {
	Kind ActionKind
	View View
}

// CommandSet is the registry the palette draws from.
type CommandSet struct {
	cmds []Command
}

// Commands returns every command in display order.
func (c CommandSet) Commands() []Command { return c.cmds }

// DefaultCommands is the palette SBT ships with. The order is the display
// order: the things a session does most often come first.
func DefaultCommands() CommandSet {
	return CommandSet{cmds: []Command{
		{Title: "run command", Hint: "enter", Group: "act", Action: Action{Kind: ActRun},
			Filter: []string{"start", "sandbox", "execute"}},
		{Title: "export changes", Hint: "alt+5", Group: "act", Action: Action{Kind: ActExport},
			Filter: []string{"save", "copy", "out"}},
		{Title: "export everything", Hint: "", Group: "act", Action: Action{Kind: ActExportAll},
			Filter: []string{"all", "save"}},
		{Title: "discard workspace", Hint: "", Group: "danger", Action: Action{Kind: ActDiscard},
			Filter: []string{"delete", "drop", "reset"}},
		{Title: "refresh isolation report", Hint: "", Group: "act", Action: Action{Kind: ActRefresh},
			Filter: []string{"probe", "recheck", "detect"}},
		{Title: "terminal", Hint: "alt+1", Group: "view", Action: Action{Kind: ActShowView, View: ViewTerminal},
			Filter: []string{"console", "session"}},
		{Title: "files", Hint: "alt+2", Group: "view", Action: Action{Kind: ActShowView, View: ViewFiles},
			Filter: []string{"workspace", "list"}},
		{Title: "changes", Hint: "alt+3", Group: "view", Action: Action{Kind: ActShowView, View: ViewChanges},
			Filter: []string{"diff", "review"}},
		{Title: "status", Hint: "alt+4", Group: "view", Action: Action{Kind: ActShowView, View: ViewStatus},
			Filter: []string{"security", "isolation", "report"}},
		{Title: "export", Hint: "alt+5", Group: "view", Action: Action{Kind: ActShowView, View: ViewExport},
			Filter: []string{"picker", "files"}},
		{Title: "help", Hint: "alt+6", Group: "view", Action: Action{Kind: ActShowView, View: ViewHelp},
			Filter: []string{"keys", "about"}},
		{Title: "toggle animation", Hint: "", Group: "view", Action: Action{Kind: ActMotion},
			Filter: []string{"motion", "animation", "no-motion"}},
		{Title: "dismiss warnings", Hint: "", Group: "view", Action: Action{Kind: ActDismissWarnings},
			Filter: []string{"clear", "toast", "notice"}},
		{Title: "exit", Hint: "ctrl+d", Group: "danger", Action: Action{Kind: ActExit},
			Filter: []string{"quit", "leave", "bye"}},
	}}
}

// Filter returns the commands that survive a query, in display order.
func (c CommandSet) Filter(query string) []Command {
	out := make([]Command, 0, len(c.cmds))
	for _, cmd := range c.cmds {
		if commandMatches(cmd, query) {
			out = append(out, cmd)
		}
	}
	return out
}

// commandMatches is a case insensitive substring match over the title and the
// filter words, so "ex" finds "export" and "sandbox" both.
func commandMatches(cmd Command, query string) bool {
	if query == "" {
		return true
	}
	if containsFold(cmd.Title, query) {
		return true
	}
	for _, w := range cmd.Filter {
		if containsFold(w, query) {
			return true
		}
	}
	return false
}

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
	// ActStop tears down the running sandbox.
	ActStop
	// ActHighRisk opens the confirmation for the relaxed policy. It is a
	// distinct action from ActSetPolicy because reaching it must always ask
	// first: high risk mode is the one place SBT cannot make the same promise.
	ActHighRisk
	// ActBack closes the current view, overlay or diff.
	ActBack
	// ActToggleTheme swaps between the white and the dark ground.
	ActToggleTheme
	// ActDestroy opens the typed confirmation for the sandbox wipe. It is a
	// distinct action because reaching it must always go through the phrase.
	ActDestroy
)

// Action is what a command asks the session to do.
type Action struct {
	Kind ActionKind
	View View
}

// MenuItem is one row of a menu. Items are data, like commands: a menu never
// performs anything itself, it names an Action and the same code path the
// palette and the key bindings use turns it into a request. That is what keeps
// "file > export" and "ctrl+k export" from ever disagreeing.
type MenuItem struct {
	// Title is the row label.
	Title string
	// Hint is the shortcut that reaches it directly, shown right aligned.
	Hint string
	// Action is what selecting the row raises.
	Action Action
	// Dangerous marks a row whose effect cannot be undone. It is drawn in the
	// danger colour and, when colour is off, keeps its "!" marker.
	Dangerous bool
	// Checked reports the row's toggle state; empty for rows that are not
	// toggles.
	Checked func(s *Snapshot) bool
}

// Menu is one top level menu.
type Menu struct {
	// Title is the word on the menu bar.
	Title string
	// Items are the rows, in display order.
	Items []MenuItem
	// Hint is the key that opens the menu directly.
	Hint string
}

// MenuBar is the ordered set of menus drawn under the top bar.
type MenuBar struct {
	Menus []Menu
}

// DefaultMenuBar is the menu SBT ships with. The order groups the session's
// lifecycle first (what to do with what is here), then how it is viewed, then
// the things that change the trust level - because that is the order a user
// thinks in when they open a menu.
func DefaultMenuBar() MenuBar {
	return MenuBar{Menus: []Menu{
		{Title: "Session", Hint: "alt+m", Items: []MenuItem{
			{Title: "Run command", Hint: "enter", Action: Action{Kind: ActRun}},
			{Title: "Stop sandbox", Hint: "ctrl+.", Action: Action{Kind: ActStop}},
			{Title: "Refresh isolation report", Action: Action{Kind: ActRefresh}},
			{Title: "Toggle animation", Action: Action{Kind: ActMotion},
				Checked: func(s *Snapshot) bool { return motionAllowed() }},
			{Title: "Dismiss notices", Action: Action{Kind: ActDismissWarnings}},
		}},
		{Title: "View", Items: []MenuItem{
			{Title: "Terminal", Hint: "alt+1", Action: Action{Kind: ActShowView, View: ViewTerminal}},
			{Title: "Files", Hint: "alt+2", Action: Action{Kind: ActShowView, View: ViewFiles}},
			{Title: "Changes", Hint: "alt+3", Action: Action{Kind: ActShowView, View: ViewChanges}},
			{Title: "Status", Hint: "alt+4", Action: Action{Kind: ActShowView, View: ViewStatus}},
			{Title: "Export", Hint: "alt+5", Action: Action{Kind: ActShowView, View: ViewExport}},
			{Title: "Help", Hint: "alt+6", Action: Action{Kind: ActShowView, View: ViewHelp}},
			{Title: "Settings", Hint: "alt+7", Action: Action{Kind: ActShowView, View: ViewSettings}},
			{Title: "Permissions", Hint: "alt+8", Action: Action{Kind: ActShowView, View: ViewPermissions}},
			{Title: "Back", Hint: "esc", Action: Action{Kind: ActBack}},
		}},
		{Title: "Security", Items: []MenuItem{
			{Title: "Standard policy", Action: Action{Kind: ActShowView, View: ViewStatus}},
			{Title: "High risk policy…", Action: Action{Kind: ActHighRisk}, Dangerous: true},
			{Title: "Show status view", Action: Action{Kind: ActShowView, View: ViewStatus}},
		}},
		{Title: "Workspace", Items: []MenuItem{
			{Title: "Export changes…", Action: Action{Kind: ActExport}},
			{Title: "Export everything", Action: Action{Kind: ActExportAll}},
			{Title: "Discard workspace…", Action: Action{Kind: ActDiscard}, Dangerous: true},
		}},
		{Title: "Look", Items: []MenuItem{
			{Title: "Light / dark theme", Action: Action{Kind: ActToggleTheme},
				Checked: func(s *Snapshot) bool { return s.Light }},
			{Title: "Toggle animation", Action: Action{Kind: ActMotion},
				Checked: func(s *Snapshot) bool { return motionAllowed() }},
		}},
		{Title: "Help", Items: []MenuItem{
			{Title: "Key bindings", Action: Action{Kind: ActShowView, View: ViewHelp}},
			{Title: "Command palette", Hint: "ctrl+k", Action: Action{Kind: ActShowView, View: ViewHelp}},
			{Title: "Exit", Hint: "ctrl+d", Action: Action{Kind: ActExit}, Dangerous: true},
		}},
	}}
}

// Index returns the position of the titled menu, or -1.
func (m MenuBar) Index(title string) int {
	for i, menu := range m.Menus {
		if menu.Title == title {
			return i
		}
	}
	return -1
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
		{Title: "stop sandbox", Hint: "ctrl+.", Group: "act", Action: Action{Kind: ActStop},
			Filter: []string{"kill", "end", "terminate"}},
		{Title: "export changes", Hint: "alt+5", Group: "act", Action: Action{Kind: ActExport},
			Filter: []string{"save", "copy", "out"}},
		{Title: "export everything", Hint: "", Group: "act", Action: Action{Kind: ActExportAll},
			Filter: []string{"all", "save"}},
		{Title: "discard workspace", Hint: "", Group: "danger", Action: Action{Kind: ActDiscard},
			Filter: []string{"delete", "drop", "reset"}},
		{Title: "refresh isolation report", Hint: "", Group: "act", Action: Action{Kind: ActRefresh},
			Filter: []string{"probe", "recheck", "detect"}},
		{Title: "high risk policy", Hint: "", Group: "danger", Action: Action{Kind: ActHighRisk},
			Filter: []string{"relax", "unsafe", "network"}},
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
		{Title: "settings", Hint: "alt+7", Group: "view", Action: Action{Kind: ActShowView, View: ViewSettings},
			Filter: []string{"preferences", "options", "config", "theme", "animation", "colour"}},
		{Title: "permissions", Hint: "alt+8", Group: "view", Action: Action{Kind: ActShowView, View: ViewPermissions},
			Filter: []string{"capabilities", "rights", "access", "root", "sudo"}},
		{Title: "back", Hint: "esc", Group: "view", Action: Action{Kind: ActBack},
			Filter: []string{"close", "return"}},
		{Title: "toggle animation", Hint: "", Group: "view", Action: Action{Kind: ActMotion},
			Filter: []string{"motion", "animation", "no-motion"}},
		{Title: "light or dark theme", Hint: "", Group: "view", Action: Action{Kind: ActToggleTheme},
			Filter: []string{"light", "dark", "white", "colours"}},
		{Title: "dismiss warnings", Hint: "", Group: "view", Action: Action{Kind: ActDismissWarnings},
			Filter: []string{"clear", "toast", "notice"}},
		{Title: "destroy sandbox", Hint: "", Group: "danger", Action: Action{Kind: ActDestroy},
			Filter: []string{"wipe", "delete", "reset", "critical"}},
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

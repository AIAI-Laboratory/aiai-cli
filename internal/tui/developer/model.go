// Package developer presents Git and workspace services, using asynchronous
// queries and explicit terminal handoff for native Git operations.
package developer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

type Options struct {
	Git                   *gitlocal.Service
	Workspace             *workspace.Service
	Mode, Path            string
	Page                  int
	Repositories, NoColor bool
	Width, Height         int
	Selected              string
}
type (
	Action struct{ Focus *workspace.Project }
	loaded struct {
		token    int
		items    []workspace.Overview
		status   gitlocal.Status
		rows     []string
		commits  []gitlocal.Commit
		branches []gitlocal.Branch
		trees    []gitlocal.Worktree
		activity *gitlocal.Activity
		diff     string
		timeline *workspace.Timeline
		err      error
	}
)

type diffMsg struct {
	token int
	text  string
	err   error
}
type Model struct {
	ctx                                               context.Context
	opts                                              Options
	width, height, cursor, offset, token              int
	items                                             []workspace.Overview
	status                                            gitlocal.Status
	rows                                              []string
	commits                                           []gitlocal.Commit
	branches                                          []gitlocal.Branch
	trees                                             []gitlocal.Worktree
	activity                                          *gitlocal.Activity
	diff                                              string
	timeline                                          *workspace.Timeline
	search, gitInput                                  textinput.Model
	dialog                                            gitDialog
	gitBusy                                           bool
	writeID                                           int
	notice                                            string
	searching, loading, staged, preview, help, detail bool
	backPath                                          string
	err                                               error
	action                                            Action
}

var pages = []string{"Status", "Changes", "History", "Branches", "Worktrees", "Activity"}

func NewModel(ctx context.Context, opts Options) Model {
	if opts.Width == 0 {
		opts.Width = 80
	}
	if opts.Height == 0 {
		opts.Height = 24
	}
	input := textinput.New()
	input.Placeholder = "Search projects or files…"
	input.CharLimit = 100
	input.SetWidth(max(1, opts.Width-8))
	if opts.NoColor {
		input.SetStyles(textinput.Styles{})
	}
	gitInput := textinput.New()
	gitInput.CharLimit = 4096
	gitInput.SetWidth(max(1, min(120, opts.Width-4)-3))
	if opts.NoColor {
		gitInput.SetStyles(textinput.Styles{})
	}
	return Model{ctx: ctx, opts: opts, width: opts.Width, height: opts.Height, search: input, gitInput: gitInput, token: 1, loading: true}
}

func Run(ctx context.Context, in io.Reader, out io.Writer, opts Options) (Action, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if fd, ok := out.(interface{ Fd() uintptr }); ok {
		if w, h, e := term.GetSize(int(fd.Fd())); e == nil {
			opts.Width, opts.Height = w, h
		}
	}
	options := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx)}
	if opts.NoColor {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	result, err := tea.NewProgram(NewModel(ctx, opts), options...).Run()
	if err != nil {
		return Action{}, err
	}
	m := result.(Model)
	return m.action, nil
}
func (m Model) Init() tea.Cmd { return m.load() }
func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		result := loaded{token: m.token, rows: []string{}}
		if m.opts.Mode != "git" {
			items, err := m.opts.Workspace.Board(m.ctx, m.opts.Repositories)
			if err != nil {
				result.err = err
				return result
			}
			result.items = items
			return result
		}
		st, err := m.opts.Git.Status(m.ctx, m.opts.Path)
		result.status, result.err = st, err
		if err != nil {
			return result
		}
		switch m.opts.Page {
		case 0:
			branch := st.Branch
			if st.Detached {
				branch = "detached HEAD"
			}
			if st.Unborn {
				branch += " · no commits"
			}
			result.rows = []string{st.Root, "", branch, "HEAD " + st.Head, fmt.Sprintf("%d staged · %d unstaged · %d untracked · %d conflicts", st.Staged, st.Unstaged, st.Untracked, st.Conflicts)}
			if st.Upstream == "" {
				result.rows = append(result.rows, "No upstream configured")
			} else {
				result.rows = append(result.rows, fmt.Sprintf("%s  +%d / -%d", st.Upstream, st.Ahead, st.Behind))
			}
			if st.LastCommit != nil {
				result.rows = append(result.rows, "", st.LastCommit.Subject, st.LastCommit.Author+" · "+st.LastCommit.Date.Local().Format("2006-01-02 15:04"))
			}
			result.rows = append(result.rows, "", "Local tracking refs only · no network check")
		case 1:
			for _, c := range st.Changes {
				result.rows = append(result.rows, changeLabel(c)+" "+c.Path+" "+c.Original)
			}
		case 2:
			if !st.Unborn {
				var commits []gitlocal.Commit
				commits, result.err = m.opts.Git.History(m.ctx, st.Root, gitlocal.Filter{Limit: 100})
				result.commits = commits
				for _, c := range commits {
					result.rows = append(result.rows, fmt.Sprintf("%.8s  %s  %s — %s", c.Hash, c.Date.Local().Format("2006-01-02 15:04"), c.Subject, c.Author))
				}
			}
		case 3:
			var branches []gitlocal.Branch
			branches, result.err = m.opts.Git.Branches(m.ctx, st.Root)
			result.branches = branches
			for _, b := range branches {
				mark := " "
				if b.Current {
					mark = ">"
				}
				result.rows = append(result.rows, fmt.Sprintf("%s %s · %.8s · %s %s", mark, b.Name, b.Head, b.Upstream, b.Tracking))
			}
		case 4:
			var trees []gitlocal.Worktree
			trees, result.err = m.opts.Git.Worktrees(m.ctx, st.Root)
			result.trees = trees
			for _, w := range trees {
				flags := ""
				if w.Detached {
					flags += " detached"
				}
				if w.Locked != "" {
					flags += " locked: " + w.Locked
				}
				if w.Prunable != "" {
					flags += " prunable: " + w.Prunable
				}
				result.rows = append(result.rows, w.Path+" · "+w.Branch+flags)
			}
		case 5:
			if !st.Unborn {
				var activity gitlocal.Activity
				activity, result.err = m.opts.Git.Activity(m.ctx, st.Root, gitlocal.Filter{Limit: 100})
				result.activity = &activity
				now := time.Now()
				for i := 6; i >= 0; i-- {
					date := now.AddDate(0, 0, -i).Format("2006-01-02")
					count := activity.Days[date]
					result.rows = append(result.rows, fmt.Sprintf("%s  %-20s %d", date, strings.Repeat("█", min(20, count)), count))
				}
				result.rows = append(result.rows, "", "All authors · current reachable history · maximum 100 commits")
				for _, c := range activity.Commits {
					result.rows = append(result.rows, c.Date.Local().Format("01-02 15:04")+" · "+c.Subject+" — "+c.Author)
				}
			}
		}
		return result
	}
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	m.token++
	m.loading = true
	m.err = nil
	return m, m.load()
}

func (m Model) visible() []int {
	result := []int{}
	q := strings.ToLower(m.search.Value())
	if m.opts.Mode == "git" {
		if m.opts.Page == 3 {
			return m.branchIndices()
		}
		for i, row := range m.rows {
			if q == "" || strings.Contains(strings.ToLower(row), q) {
				result = append(result, i)
			}
		}
	} else {
		for i, o := range m.items {
			if q == "" || strings.Contains(strings.ToLower(o.Project.Name+" "+o.Project.Root+" "+strings.Join(o.Project.Tags, " ")), q) {
				result = append(result, i)
			}
		}
	}
	return result
}

func (m Model) selected() *workspace.Overview {
	indices := m.visible()
	if len(indices) == 0 || m.cursor >= len(indices) {
		return nil
	}
	return &m.items[indices[m.cursor]]
}

func (m Model) loadDiff() tea.Cmd {
	indices := m.visible()
	if m.opts.Page != 1 || m.opts.Mode != "git" || len(indices) == 0 {
		return nil
	}
	index := indices[min(m.cursor, len(indices)-1)]
	if index >= len(m.status.Changes) {
		return nil
	}
	c := m.status.Changes[index]
	return func() tea.Msg {
		if c.Untracked {
			return diffMsg{token: m.token, text: "Untracked file · no Git diff. File contents are not opened."}
		}
		diff, err := m.opts.Git.Diff(m.ctx, m.status.Root, c.Path, m.staged)
		return diffMsg{token: m.token, text: diff, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.SetWidth(max(1, m.width-8))
		m.gitInput.SetWidth(max(1, min(120, m.width-4)-3))
	case gitNativeReady:
		return m.runGitCommand(msg)
	case gitWriteDone:
		if msg.id != m.writeID || !m.gitBusy {
			return m, nil
		}
		m.gitBusy = false
		if msg.err != nil {
			if m.dialog.kind != "" {
				m.dialog.err = msg.err.Error()
			} else {
				m.notice = msg.err.Error()
			}
		} else {
			m = m.closeGitDialog()
			m.notice = msg.notice
		}
		return m.refresh()
	case loaded:
		if msg.token != m.token {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.timeline != nil {
			m.timeline = msg.timeline
			m.offset = max(0, len(m.timelineRows(max(1, min(120, m.width-4))))-m.bodyHeight())
			return m, nil
		}
		m.items, m.status, m.rows, m.diff = msg.items, msg.status, msg.rows, msg.diff
		m.commits = msg.commits
		m.branches, m.trees, m.activity = msg.branches, msg.trees, msg.activity
		m.cursor = min(m.cursor, max(0, len(m.visible())-1))
		if m.opts.Selected != "" && m.opts.Mode != "git" {
			for index, o := range m.items {
				if o.Project.ID == m.opts.Selected {
					m.cursor = index
					break
				}
			}
			m.opts.Selected = ""
		}
		if m.opts.Mode == "git" && m.opts.Page == 1 {
			m.diff = "Loading diff…"
			return m, m.loadDiff()
		}
	case diffMsg:
		if msg.token == m.token {
			m.diff = msg.text
			if msg.err != nil {
				m.diff = msg.err.Error()
			}
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.gitBusy {
			return m, nil
		}
		if m.dialog.kind != "" {
			return m.updateGitDialog(msg)
		}
		if m.searching {
			switch key {
			case "esc", "enter":
				m.searching = false
				m.search.Blur()
			case "ctrl+u":
				m.search.SetValue("")
				m.cursor = 0
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.cursor, m.offset = 0, 0
				m.token++
				m.diff = "Loading diff…"
				return m, tea.Batch(cmd, m.loadDiff())
			}
			m.token++
			m.diff = "Loading diff…"
			return m, m.loadDiff()
		}
		if m.help {
			if key == "?" || key == "q" || key == "esc" {
				m.help = false
			}
			return m, nil
		}
		if key == "?" {
			m.help = true
			return m, nil
		}
		if key == "q" || key == "esc" {
			if m.detail {
				m.detail = false
				m.timeline = nil
				m.offset = 0
				m.token++
				m.loading = false
				return m, nil
			}
			if m.backPath != "" {
				m.opts.Mode = ""
				m.opts.Path = m.backPath
				m.backPath = ""
				m.cursor, m.offset = 0, 0
				m.search.SetValue("")
				return m.refresh()
			}
			return m, tea.Quit
		}
		if m.loading {
			return m, nil
		}
		if key == "/" {
			m.detail = false
			m.timeline = nil
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		}
		if key == "r" {
			m.notice = ""
			if m.detail {
				m.detail = false
				m.timeline = nil
			}
			return m.refresh()
		}
		if next, cmd, handled := m.gitShortcut(key); handled {
			return next, cmd
		}
		if m.opts.Mode == "git" && (key == "left" || key == "right" || key == "h" || key == "l") {
			delta := 1
			if key == "left" || key == "h" {
				delta = -1
			}
			m.opts.Page = (m.opts.Page + delta + len(pages)) % len(pages)
			m.cursor, m.offset = 0, 0
			m.preview = false
			m.search.SetValue("")
			return m.refresh()
		}
		if key == "tab" && m.opts.Mode == "git" && m.opts.Page == 1 {
			m.preview = !m.preview
			return m, nil
		}
		if key == "s" && m.opts.Mode == "git" && m.opts.Page == 1 {
			m.staged = !m.staged
			m.offset = 0
			m.token++
			m.diff = "Loading diff…"
			return m, m.loadDiff()
		}
		if key == "up" || key == "k" || key == "down" || key == "j" || key == "pgup" || key == "pgdown" {
			delta := 1
			if key == "up" || key == "k" || key == "pgup" {
				delta = -1
			}
			if key == "pgup" || key == "pgdown" {
				step := m.bodyHeight()
				if m.opts.Mode == "git" && (m.opts.Page == 1 || m.opts.Page == 2) {
					step -= 3
				} else if m.opts.Mode != "git" && !m.detail {
					step = max(1, (step-2)/4)
				}
				delta *= max(1, step)
			}
			if m.detail || m.preview || m.opts.Mode == "git" && m.opts.Page != 1 && m.opts.Page != 3 {
				m.offset = max(0, m.offset+delta)
			} else {
				m.cursor = max(0, min(max(0, len(m.visible())-1), m.cursor+delta))
				m.offset = 0
				m.token++
				m.diff = "Loading diff…"
				return m, m.loadDiff()
			}
			return m, nil
		}
		if m.opts.Mode != "git" {
			o := m.selected()
			if o == nil {
				return m, nil
			}
			switch key {
			case "g":
				if !o.Available || o.Git == nil {
					return m, nil
				}
				m.opts.Selected = o.Project.ID
				m.backPath = o.Project.Root
				m.opts.Path, m.opts.Mode, m.opts.Page = o.Project.Root, "git", 0
				m.cursor, m.offset = 0, 0
				m.detail = false
				m.search.SetValue("")
				return m.refresh()
			case "z":
				if !o.Available || m.opts.Repositories {
					return m, nil
				}
				m.action.Focus = &o.Project
				return m, tea.Quit
			case "enter":
				m.detail = true
				m.offset = 0
				m.loading = true
				project := o.Project
				token := m.token
				return m, func() tea.Msg {
					_ = m.opts.Workspace.Open(m.ctx, project.ID)
					timeline, err := m.opts.Workspace.Timeline(m.ctx, project, gitlocal.Filter{Limit: 100})
					return loaded{token: token, timeline: &timeline, err: err}
				}
			}
		}
	}
	if m.dialog.kind != "" && !m.gitBusy {
		return m.updateGitDialog(msg)
	}
	return m, nil
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return '�'
	}, s)
}

func (m Model) accent(s string) string {
	return style.Paint(s, style.Accent, true, m.opts.NoColor)
}

func (m Model) bodyHeight() int {
	header := 1
	if m.opts.Mode == "git" {
		header++
	}
	if m.searching || m.search.Value() != "" {
		header++
	}
	if m.notice != "" {
		header++
	}
	return max(1, m.height-header-6)
}

func (m Model) View() tea.View {
	width := max(1, min(120, m.width-4))
	if m.width < 40 || m.height < 15 {
		view := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render("AIAI\nResize to at least 40 × 15\nq back"))
		view.AltScreen = true
		return view
	}
	title := "Workspace"
	if m.opts.Repositories {
		title = "Repositories"
	}
	if m.opts.Mode == "git" {
		title = "Git"
	}
	header := m.title("AIAI / " + title)
	if m.opts.Mode == "git" {
		header += m.muted("  "+oneLine(m.status.Name)) + "\n" + m.gitTabs(width)
	}
	if m.searching || m.search.Value() != "" {
		header += "\n" + m.search.View()
	}
	if m.notice != "" {
		header += "\n" + ansi.Truncate(oneLine(m.notice), width, "…")
	}
	height := m.bodyHeight()
	footer := "↑↓ select · enter timeline · g git · z focus · ? help · q back"
	if width < 80 {
		footer = "↑↓ · enter open · ? help · q back"
	}
	body := ""
	switch {
	case m.gitBusy:
		body = "Running Git operation…"
		footer = "ctrl+c cancel"
	case m.dialog.kind != "":
		body, footer = m.gitDialogView(width)
	case m.help:
		body = "Local developer workspace\n\nGit: a actions · c commit · b branches\np push · P pull · f fetch · : Git command\nChanges: Space stage/unstage · A/U all\nBranches: Enter switch · n new\nActions: / find · ↑↓ select · Enter open\nStash · merge/rebase · tags · remotes\nGit uses your terminal for auth/editors.\n←/→ pages · / search · r refresh\nEsc/q back · Ctrl+C cancel"
	case m.loading:
		body = "Loading local data…"
		footer = "q back · ctrl+c cancel"
	case m.err != nil:
		body = clean(m.err.Error()) + "\n\nr retry · q back"
		footer = "r retry · q back"
	default:
		if m.opts.Mode == "git" {
			footer = "a actions · p push · P pull · f fetch · c commit · ←→ pages · q back"
			switch m.opts.Page {
			case 0:
				body = m.statusView(width, height)
			case 1:
				footer = "space stage/unstage · c commit · a actions · tab diff · s staged · q back"
				if m.preview {
					footer = "↑↓ diff · tab files · c commit · a actions · s staged · q back"
				}
				body = m.diffView(width, height)
			case 2:
				body = m.historyView(width, height)
			case 3:
				footer = "↑↓ select · enter switch · n new branch · a actions · q back"
				body = m.branchesView(width, height)
			case 4:
				body = m.worktreesView(width, height)
			case 5:
				body = m.activityView(width, height)
			default:
				body = m.scrollRows(m.rows, width, height)
			}
			if width < 80 {
				footer = "a actions · p push · P pull · f fetch · q back"
				if m.opts.Page == 1 {
					footer = "space stage · c commit · a actions · q back"
					if m.preview {
						footer = "tab files · s staged · a actions · q back"
					}
				}
				if m.opts.Page == 3 {
					footer = "enter switch · n new · a actions · q back"
				}
			}
			if width < 60 {
				footer = "a actions · p/P sync · q back"
				if m.opts.Page == 1 {
					footer = "space stage · a actions · q back"
				}
				if m.opts.Page == 3 {
					footer = "enter switch · a actions · q back"
				}
			}
		} else if m.detail {
			body = m.detailView(width, height)
			footer = "↑↓ scroll · g git · z focus · ? help · q back"
			if width < 60 {
				footer = "↑↓ scroll · ? help · q back"
			}
		} else {
			body = m.boardView(width, height)
		}
	}
	if m.opts.Repositories {
		footer = strings.ReplaceAll(footer, " · z focus", "")
	}
	content := style.Fit(header, width, lipgloss.Height(header)) + "\n\n" + style.Fit(body, width, height) + "\n\n" + m.muted(strings.Repeat("─", width)) + "\n" + ansi.Truncate(style.Keys(footer, m.opts.NoColor), width, "…")
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	view.AltScreen = true
	return view
}

func (m Model) scrollRows(rows []string, width, height int) string {
	filtered := []string{}
	q := strings.ToLower(m.search.Value())
	for _, row := range rows {
		if q == "" || strings.Contains(strings.ToLower(row), q) {
			filtered = append(filtered, clean(row))
		}
	}
	if len(filtered) == 0 {
		return "No entries in this view."
	}
	start := min(m.offset, max(0, len(filtered)-height))
	return strings.Join(filtered[start:min(len(filtered), start+height)], "\n")
}

func (m Model) boardView(width, height int) string {
	indices := m.visible()
	if len(indices) == 0 {
		return "No projects yet.\n\naiai workspace add . --name my-project\naiai git repo add . --name my-repo"
	}
	list := []string{}
	var focus int64
	completed := 0
	for _, o := range m.items {
		focus += o.Focus.Today.FocusSeconds
		completed += o.Focus.Today.Completed
	}
	summary := m.title("Today") + m.muted(fmt.Sprintf("  %s focus · %d completed", focusDuration(focus), completed)) + "\n\n"
	if m.opts.Repositories {
		summary = m.title("Local repositories") + m.muted("  Tracking refs only") + "\n\n"
	}
	slots := max(1, (height-2)/4)
	start := max(0, m.cursor-slots+1)
	for visible := start; visible < min(len(indices), start+slots); visible++ {
		o := m.items[indices[visible]]
		name := "  " + m.title(oneLine(o.Project.Name))
		if visible == m.cursor {
			name = m.accent("> " + clean(o.Project.Name))
		}
		meta := "Non-Git project"
		if !o.Available {
			meta = "Directory unavailable"
		} else if o.Git != nil {
			changes := "clean"
			if len(o.Git.Changes) > 0 {
				changes = fmt.Sprintf("%d changed files", len(o.Git.Changes))
			}
			meta = fmt.Sprintf("%s · %s", o.Git.Branch, changes)
			if o.Git.Ahead > 0 || o.Git.Behind > 0 {
				meta += fmt.Sprintf(" · %d ahead / %d behind", o.Git.Ahead, o.Git.Behind)
			}
			if o.Git.Unborn {
				meta = "No commits · " + meta
			}
			if o.Git.Detached {
				meta = "Detached HEAD · " + meta
			}
		}
		third := "  Focus today  " + focusDuration(o.Focus.Today.FocusSeconds)
		if m.opts.Repositories {
			third = "  " + clean(o.Project.Root)
		}
		list = append(list, name, "  "+m.muted(oneLine(meta)), m.muted(third), "")
	}
	if width < 100 {
		return summary + strings.Join(list, "\n")
	}
	o := m.selected()
	if o == nil {
		return summary + strings.Join(list, "\n")
	}
	details := m.title(oneLine(o.Project.Name)) + "\n" + m.muted(oneLine(o.Project.Root))
	if o.Project.Description != "" {
		details += "\n\n" + oneLine(o.Project.Description)
	}
	if o.Warning != "" {
		details += "\n\n" + m.tone(oneLine(o.Warning), style.Warn)
	}
	if o.Git != nil && o.Git.LastCommit != nil {
		details += "\n\n" + m.title("Latest commit") + "\n" + oneLine(o.Git.LastCommit.Subject) + "\n" + m.muted(o.Git.LastCommit.Date.Local().Format("02 Jan 15:04"))
	}
	if width >= 116 {
		usable := width - 6
		left, mid := usable*30/100, usable*32/100
		right := usable - left - mid
		activity := []string{m.title("Recent activity"), m.muted(fmt.Sprintf("%d commits shown today · all authors", o.TodayCommits)), ""}
		if o.HistoryTruncated {
			activity = append(activity, m.muted("100-commit limit reached"))
		}
		for _, e := range o.Recent {
			activity = append(activity, m.muted(e.Time.Local().Format("02 Jan 15:04")+" · "+eventSource(e.Source)), oneLine(e.Text), "")
		}
		for i := range list {
			list[i] = ansi.Truncate(list[i], left-2, "…")
		}
		detailLines := strings.Split(details, "\n")
		for i := range detailLines {
			detailLines[i] = ansi.Truncate(detailLines[i], mid-2, "…")
		}
		for i := range activity {
			activity[i] = ansi.Truncate(activity[i], right, "…")
		}
		return summary + m.joinColumns([]string{strings.Join(list, "\n"), strings.Join(detailLines, "\n"), strings.Join(activity, "\n")}, []int{left, mid, right})
	}
	half := (width - 3) / 2
	for i := range list {
		list[i] = ansi.Truncate(list[i], half-2, "…")
	}
	return summary + m.joinColumns([]string{strings.Join(list, "\n"), details}, []int{half, width - half - 3})
}

func (m Model) detailView(width, height int) string {
	return m.viewPage(m.timelineRows(width), width, height)
}

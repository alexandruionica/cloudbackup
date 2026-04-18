package restore

import (
	"bytes"
	clientCommon "cloudbackup/client/common"
	clientConfig "cloudbackup/client/config"
	"cloudbackup/httpd"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Browse launches an interactive TUI that fetches backed-up files from
// /report/backup/file/list and lets the user multi-select items to restore. It
// returns the list of absolute source paths selected. If the user aborts, it
// returns an error whose message is "cancelled by user" so the caller can
// distinguish cancellation from a real error.
//
// The browser pages one directory at a time (descend=false) and recurses on
// demand when the user enters a directory. "All files" selection is handled
// outside of Browse via the --all-files flag.
func Browse(config clientConfig.Client, jobName string, jobId string) ([]string, error) {
	if jobId == "" {
		return nil, errors.New("a backup job id is required in order to browse backed up files")
	}
	m := initialModel(config, jobName, jobId)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui error: %w", err)
	}
	fm := finalModel.(*model)
	if fm.cancelled {
		return nil, errors.New("cancelled by user")
	}
	if fm.err != nil {
		return nil, fm.err
	}
	paths := make([]string, 0, len(fm.selected))
	for p := range fm.selected {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// -----------------------------------------------------------------------------
// Bubbletea model
// -----------------------------------------------------------------------------

type entry struct {
	Path   string // absolute path
	Parent string
	IsDir  bool
	Size   uint64
	// IsParent marks a synthetic ".." row that navigates to the parent directory
	// when the user presses Enter on it. It is never itself selectable.
	IsParent bool
}

type pageMsg struct {
	entries []entry
	next    string
	err     error
}

type model struct {
	config  clientConfig.Client
	jobName string
	jobId   string

	// current directory being listed. Empty => "top level" (use all backup roots)
	cwd string
	// navigation stack of parent directories so the user can go "up"
	stack []string

	entries  []entry
	nextTok  string
	loading  bool
	cursor   int
	selected map[string]bool

	width  int
	height int

	status    string
	err       error
	cancelled bool
	done      bool
}

func initialModel(config clientConfig.Client, jobName, jobId string) *model {
	return &model{
		config:   config,
		jobName:  jobName,
		jobId:    jobId,
		selected: make(map[string]bool),
		loading:  true,
	}
}

func (m *model) Init() tea.Cmd {
	return m.fetchCmd("")
}

// resetForCwd clears the entries/next-token/cursor after a cwd change and, when the new
// cwd is not the virtual root, pre-seeds the listing with a synthetic ".." entry so the
// user can go back up using Enter on that row (Midnight Commander style).
func (m *model) resetForCwd() {
	m.entries = nil
	m.nextTok = ""
	m.cursor = 0
	m.loading = true
	m.status = ""
	if m.cwd != "" {
		m.entries = []entry{{Path: "..", IsDir: true, IsParent: true}}
		// Start the cursor on the first real entry so the user isn't parked on "..".
		m.cursor = 1
	}
}

// goUp pops one level off the navigation stack and triggers a fetch for the parent cwd.
// When the stack is empty (we're already at the virtual root) it is a no-op.
func (m *model) goUp() (tea.Model, tea.Cmd) {
	if len(m.stack) == 0 {
		return m, nil
	}
	m.cwd = m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.resetForCwd()
	return m, m.fetchCmd("")
}

// fetchCmd returns a command that queries /report/backup/file/list for the
// current cwd. If nextToken is non-empty, it is used to continue paging the
// current directory.
func (m *model) fetchCmd(nextToken string) tea.Cmd {
	cwd := m.cwd
	return func() tea.Msg {
		entries, next, err := fetchFileList(m.config, m.jobName, m.jobId, cwd, nextToken)
		return pageMsg{entries: entries, next: next, err: err}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case pageMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.entries = append(m.entries, msg.entries...)
		m.nextTok = msg.next
		if m.cursor >= len(m.entries) {
			m.cursor = 0
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "pgup":
			m.cursor -= 10
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "pgdown":
			m.cursor += 10
			if m.cursor >= len(m.entries) {
				m.cursor = len(m.entries) - 1
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.entries) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
		case " ":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := m.entries[m.cursor]
			if e.IsParent {
				return m, nil
			}
			if m.selected[e.Path] {
				delete(m.selected, e.Path)
			} else {
				m.selected[e.Path] = true
			}
		case "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			e := m.entries[m.cursor]
			if e.IsParent {
				return m.goUp()
			}
			if !e.IsDir {
				return m, nil
			}
			m.stack = append(m.stack, m.cwd)
			m.cwd = e.Path
			m.resetForCwd()
			return m, m.fetchCmd("")
		case "backspace", "left", "h", "u":
			return m.goUp()
		case "n":
			if m.nextTok != "" && !m.loading {
				m.loading = true
				return m, m.fetchCmd(m.nextTok)
			}
		case "d":
			m.done = true
			return m, tea.Quit
		case "?":
			if strings.Contains(m.status, "Keys:") {
				m.status = ""
			} else {
				m.status = "Keys: ↑/↓ move, space select, enter descend, backspace up, n next page, d done, q cancel"
			}
		}
	}
	return m, nil
}

// -----------------------------------------------------------------------------
// View
// -----------------------------------------------------------------------------

var (
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleCursor   = lipgloss.NewStyle().Reverse(true)
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleDir      = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	styleHint     = lipgloss.NewStyle().Faint(true)
	styleErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func (m *model) View() string {
	if m.err != nil {
		return styleErr.Render("Error: "+m.err.Error()) + "\n"
	}

	var b strings.Builder
	loc := m.cwd
	if loc == "" {
		loc = "<backup roots>"
	}
	b.WriteString(styleHeader.Render(fmt.Sprintf("Browse: %s (job %s)  —  %s",
		m.jobName, m.jobId, loc)))
	b.WriteString("\n")
	b.WriteString(styleHint.Render(fmt.Sprintf("Selected: %d   ?: help", len(m.selected))))
	b.WriteString("\n\n")

	maxRows := m.height - 7
	if maxRows < 5 {
		maxRows = 5
	}
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.entries) {
		end = len(m.entries)
	}

	if len(m.entries) == 0 && m.loading {
		b.WriteString("Loading…\n")
	} else if len(m.entries) == 0 {
		b.WriteString(styleHint.Render("(empty)") + "\n")
	}

	for i := start; i < end; i++ {
		e := m.entries[i]
		var line string
		if e.IsParent {
			// Synthetic ".." row: no selection marker, constant indent matching "[ ] ".
			line = "    " + styleDir.Render("..")
		} else {
			mark := "[ ]"
			if m.selected[e.Path] {
				mark = "[x]"
			}
			name := e.Path
			if e.IsDir {
				name = styleDir.Render(name + "/")
			} else if m.selected[e.Path] {
				name = styleSelected.Render(name)
			}
			line = fmt.Sprintf("%s %s", mark, name)
		}
		if i == m.cursor {
			line = styleCursor.Render(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	if m.nextTok != "" {
		b.WriteString(styleHint.Render("more results available — press 'n' for next page") + "\n")
	}
	if m.status != "" {
		b.WriteString(styleHint.Render(m.status) + "\n")
	}
	b.WriteString(styleHint.Render("[space] select  [enter] descend  [backspace] up  [n] next  [d] done  [q] cancel"))
	return b.String()
}

// -----------------------------------------------------------------------------
// API call
// -----------------------------------------------------------------------------

type fileListResult struct {
	httpd.HttpStatusReply
	Next   string                                `json:"next"`
	Result []httpd.ReportBackupFileListDbResults `json:"result"`
}

func fetchFileList(cfg clientConfig.Client, jobName, jobId, path, nextToken string) ([]entry, string, error) {
	// When listing at the virtual root (no path selected yet) the server's
	// non-descend query returns nothing because no real row has an empty parent.
	// Use descend=true in that case and filter client-side to just the roots
	// (entries whose parent is not itself one of the returned paths).
	descend := path == ""
	payload := httpd.ReportBackupFileList{
		Name:    jobName,
		JobId:   jobId,
		Path:    path,
		Descend: descend,
		Next:    nextToken,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest("POST", cfg.Address+ApiPrefix+"/report/backup/file/list", bytes.NewBuffer(encoded))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(cfg.Username, cfg.Password)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, "", err
	}
	body, err := clientCommon.ValidateServerResponse(resp)
	if err != nil {
		return nil, "", err
	}
	var decoded fileListResult
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, "", fmt.Errorf("could not decode server response: %w", err)
	}

	// Build the set of all paths returned so we can identify "root" entries
	// (their parent is not among the returned rows) when we're at the virtual root.
	pathSet := make(map[string]struct{}, len(decoded.Result))
	for _, r := range decoded.Result {
		pathSet[r.Path] = struct{}{}
	}

	entries := make([]entry, 0, len(decoded.Result))
	for _, r := range decoded.Result {
		if len(r.Instances) == 0 {
			continue
		}
		inst := r.Instances[0]
		// skip the "self" row that represents the directory the user is listing
		if r.Path == path && path != "" {
			continue
		}
		if descend {
			// Only surface the configured backup roots: rows whose parent is NOT
			// itself one of the returned paths. A normal file like /etc/hosts has
			// parent=/etc which will be in the result set, so it gets filtered out.
			if _, parentInSet := pathSet[r.Parent]; parentInSet {
				continue
			}
		}
		entries = append(entries, entry{
			Path:   r.Path,
			Parent: r.Parent,
			IsDir:  strings.EqualFold(inst.Type, "directory") || strings.EqualFold(inst.Type, "dir"),
			Size:   inst.Size,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, decoded.Next, nil
}

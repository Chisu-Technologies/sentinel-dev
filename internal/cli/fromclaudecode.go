package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// maxClaudeCodeTask bounds the task taken from a Claude Code session. The
// newest requests are kept first: they say where the work stopped.
const maxClaudeCodeTask = 4000

// claudeCodeProjectName is the longest folder name Claude Code gives a
// project before it cuts the name and appends a hash of the whole path
// (https://code.claude.com/docs/en/sessions.md, "Where transcripts are stored").
const claudeCodeProjectName = 200

// errNoClaudeCodeTask says why no task could be taken, with what to do instead.
var errNoClaudeCodeTask = errors.New(`type the task instead: sentinel run "<task>"`)

// claudeCodeTask builds a task from the newest Claude Code session started in
// dir: the person's own requests in it, oldest first. Claude Code says its
// transcript format "changes between versions", so only the user's text is
// read, every line that does not parse is skipped, and a session with nothing
// readable is refused rather than guessed at. Tool output, Claude's answers
// and the injected notes are never read, so they never reach the server.
func claudeCodeTask(env Env, dir string) (task, session string, err error) {
	projects, name, err := claudeCodeProjects(env, dir)
	if err != nil {
		return "", "", err
	}
	folder, err := claudeCodeProjectFolder(projects, name)
	if err != nil {
		return "", "", err
	}
	path, err := newestSession(folder)
	if err != nil {
		return "", "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("could not read the Claude Code session %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	requests, err := userRequests(f)
	if err != nil {
		return "", "", fmt.Errorf("could not read the Claude Code session %s: %w", filepath.Base(path), err)
	}
	if len(requests) == 0 {
		return "", "", fmt.Errorf("the newest Claude Code session in this folder has no request Sentinel can read; %w", errNoClaudeCodeTask)
	}
	return composeClaudeCodeTask(requests), strings.TrimSuffix(filepath.Base(path), ".jsonl"), nil
}

// claudeCodeProjects is the folder Claude Code keeps its projects in and the
// name it gives the one for dir.
func claudeCodeProjects(env Env, dir string) (projects, name string, err error) {
	config := env.getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		if env.HomeDir == "" {
			return "", "", fmt.Errorf("could not find Claude Code's folder; %w", errNoClaudeCodeTask)
		}
		config = filepath.Join(env.HomeDir, ".claude")
	} else if named := env.getenv("CLAUDE_CODE_PROJECT_DIR_NAME"); named != "" {
		// Claude Code honours the name only together with CLAUDE_CONFIG_DIR.
		return filepath.Join(config, "projects"), named, nil
	}
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return "", "", fmt.Errorf("could not tell which folder this is: %w", err)
		}
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return "", "", fmt.Errorf("could not tell which folder this is: %w", err)
	}
	return filepath.Join(config, "projects"), claudeCodeProjectDir(dir), nil
}

// claudeCodeProjectDir is the working directory with every character that is
// not a letter or a digit replaced by "-", as Claude Code names it.
func claudeCodeProjectDir(dir string) string {
	var b strings.Builder
	for _, r := range dir {
		if r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// claudeCodeProjectFolder finds the project's folder. A name too long for
// Claude Code carries a hash whose recipe is not documented, so it is found by
// its first 200 characters, and only when exactly one folder has them.
func claudeCodeProjectFolder(projects, name string) (string, error) {
	missing := fmt.Errorf("no Claude Code session was started in this folder; run this where you started Claude Code, or %w", errNoClaudeCodeTask)
	if len(name) <= claudeCodeProjectName {
		folder := filepath.Join(projects, name)
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			return "", missing
		}
		return folder, nil
	}
	entries, err := os.ReadDir(projects)
	if err != nil {
		return "", missing
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > claudeCodeProjectName && strings.HasPrefix(e.Name(), name[:claudeCodeProjectName]) {
			found = append(found, e.Name())
		}
	}
	switch len(found) {
	case 0:
		return "", missing
	case 1:
		return filepath.Join(projects, found[0]), nil
	default:
		return "", fmt.Errorf("this folder's path is too long to tell its Claude Code sessions from another folder's; %w", errNoClaudeCodeTask)
	}
}

// newestSession is the session file written last.
func newestSession(folder string) (string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return "", fmt.Errorf("could not read Claude Code's sessions: %w", err)
	}
	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if t := info.ModTime().UnixNano(); newest == "" || t > newestTime {
			newest, newestTime = e.Name(), t
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no Claude Code session was started in this folder; %w", errNoClaudeCodeTask)
	}
	return filepath.Join(folder, newest), nil
}

// transcriptEntry is the little of a transcript line that is read.
type transcriptEntry struct {
	Type        string `json:"type"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// userRequests reads the text the person typed, in order. A line is read
// whole however long it is: tool output can make one megabytes long.
func userRequests(r io.Reader) ([]string, error) {
	var requests []string
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if text, ok := userText(line); ok {
			requests = append(requests, text)
		}
		if errors.Is(err, io.EOF) {
			return requests, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func userText(line []byte) (string, bool) {
	var e transcriptEntry
	if json.Unmarshal(line, &e) != nil || e.Type != "user" || e.Message.Role != "user" || e.IsMeta || e.IsSidechain {
		return "", false
	}
	var text string
	if json.Unmarshal(e.Message.Content, &text) != nil {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Message.Content, &parts) != nil {
			return "", false
		}
		var texts []string
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		text = strings.Join(texts, "\n")
	}
	text = strings.TrimSpace(text)
	// Slash commands, their output and the notes Claude Code adds come as
	// tagged text; an interruption is Claude Code's own marker.
	if text == "" || strings.HasPrefix(text, "<") || strings.HasPrefix(text, "[Request interrupted") {
		return "", false
	}
	return text, true
}

// composeClaudeCodeTask puts the newest request first, as the title of the
// pull request, and then every request that fits, oldest first.
func composeClaudeCodeTask(requests []string) string {
	latest := requests[len(requests)-1]
	head, _, _ := strings.Cut(latest, "\n")
	if utf8.RuneCountInString(head) > server.MaxTaskTitle {
		head = string([]rune(head)[:server.MaxTaskTitle-1]) + "…"
	}
	intro := head + "\n\nThis continues a Claude Code session in this folder. What the person asked there, oldest first:\n"
	budget := maxClaudeCodeTask - utf8.RuneCountInString(intro)
	var kept []string
	for i := len(requests) - 1; i >= 0; i-- {
		item := "\n- " + strings.ReplaceAll(requests[i], "\n", "\n  ")
		n := utf8.RuneCountInString(item)
		if n > budget {
			if len(kept) == 0 {
				kept = append(kept, string([]rune(item)[:budget-1])+"…")
			}
			break
		}
		kept = append(kept, item)
		budget -= n
	}
	var b strings.Builder
	b.WriteString(intro)
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
	}
	return b.String()
}

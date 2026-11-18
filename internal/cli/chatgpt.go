package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/chatgpt"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// chatgptHost is what this machine keeps between ChatGPT sign-ins. Neither
// value is a secret: the host id names this installation to OpenAI, and the
// client id is the one OpenAI issued at the first sign-in, which every later
// one must reuse.
type chatgptHost struct {
	HostID   string `json:"ext_agent_host_id"`
	ClientID string `json:"client_id,omitempty"`
}

// maxChatGPTWait is how long the sign-in page may stay open.
const maxChatGPTWait = 10 * time.Minute

// runLoginChatGPT lends this person's ChatGPT plan to their Sentinel: OpenAI's
// own sign-in runs here, where the browser and the loopback callback are, and
// the result goes to the server the CLI is signed in to. The tokens are never
// printed or written to disk by this command.
func runLoginChatGPT(ctx context.Context, env Env) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(env.Stderr, "sentinel: "+format+"\n", a...)
		return ExitFailed
	}
	client, code := newServerClient(env, "login --chatgpt")
	if code >= 0 {
		return code
	}
	host, path, err := loadChatGPTHost(env)
	if err != nil {
		return fail("%v", err)
	}
	if host.HostID == "" {
		if host.HostID, err = chatgpt.NewHostID(); err != nil {
			return fail("could not name this installation for OpenAI: %v", err)
		}
		if err := saveChatGPTHost(path, host); err != nil {
			return fail("could not keep this installation's name in %s: %v", path, err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, maxChatGPTWait)
	defer cancel()
	flow := chatgpt.Flow{
		AuthBase: env.ChatGPTAuthBase,
		AppName:  "Sentinel",
		HostID:   host.HostID,
		ClientID: host.ClientID,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Open: func(page string) error {
			fmt.Fprintf(env.Stdout, "Open this page to sign in with ChatGPT:\n\n  %s\n\nWaiting for the sign-in...\n", page)
			openInBrowser(ctx, env, page)
			return nil
		},
	}
	creds, err := flow.SignIn(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return fail("nobody signed in with ChatGPT in time; run sentinel login --chatgpt again")
		}
		return fail("the ChatGPT sign-in did not finish: %v", err)
	}
	if host.ClientID == "" {
		host.ClientID = creds.ClientID
		if err := saveChatGPTHost(path, host); err != nil {
			return fail("could not keep OpenAI's client id in %s: %v", path, err)
		}
	}
	account := creds.Email
	if account == "" {
		account = creds.Subject
	}
	answer, err := client.PutChatGPTSession(ctx, server.ChatGPTSession{
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		ExpiresIn:    creds.ExpiresIn,
		ClientID:     creds.ClientID,
		Account:      account,
	})
	if err != nil {
		return fail("signed in with ChatGPT, but %s did not keep it: %v", client.Host(), err)
	}
	fmt.Fprintf(env.Stdout, "Signed in with ChatGPT as %s. Sentinel on %s uses your ChatGPT plan; choose its models in Settings > Model access.\n",
		cleanName(answer), client.Host())
	return ExitOK
}

// openInBrowser asks the system to show the page. It is a convenience: the
// page was printed, so a machine without a browser opener still works.
func openInBrowser(ctx context.Context, env Env, page string) {
	if env.Runner == nil {
		return
	}
	var command execx.Command
	switch runtime.GOOS {
	case "windows":
		command = execx.Command{Name: "rundll32", Args: []string{"url.dll,FileProtocolHandler", page}}
	case "darwin":
		command = execx.Command{Name: "open", Args: []string{page}}
	default:
		command = execx.Command{Name: "xdg-open", Args: []string{page}}
	}
	_, _ = env.Runner.Run(ctx, command)
}

func chatgptHostPath(env Env) (string, error) {
	if env.ConfigDir == "" {
		return "", errNoConfigDir
	}
	return filepath.Join(env.ConfigDir, "sentinel", "chatgpt.json"), nil
}

func loadChatGPTHost(env Env) (chatgptHost, string, error) {
	path, err := chatgptHostPath(env)
	if err != nil {
		return chatgptHost{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return chatgptHost{}, path, nil
	}
	if err != nil {
		return chatgptHost{}, "", err
	}
	var host chatgptHost
	if json.Unmarshal(data, &host) != nil {
		return chatgptHost{}, "", fmt.Errorf("%s is not a file this command wrote; remove it and sign in again", path)
	}
	return host, path, nil
}

func saveChatGPTHost(path string, host chatgptHost) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(host, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".chatgpt-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

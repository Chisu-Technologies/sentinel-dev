# Install Sentinel on this computer (for AI agents)

You are an AI agent, such as Claude Code, Codex, the Claude or ChatGPT desktop app, or
Cursor, and the person you work for gave you this repository and asked you to install
Sentinel. Follow these steps in order on their computer. When it is done, Sentinel runs
there, and you can send it the changes you write: it runs the tests in its sandbox,
has a QA and reviewers check them, signs the result and opens a pull request that the
person merges.

## Rules while you install

- Run each command yourself and read its output before the next step.
- Never type a password, a token or an API key, and never ask the person to paste one
  into this conversation. Where a sign-in is needed, the person does it in their own
  browser or terminal, and you wait.
- Never merge a pull request, now or later: merging is the person's decision.
- If a step fails, show the person the error as it was printed and stop there.

## 1. Check what Sentinel needs

```bash
docker info
```

Docker must be installed and running. If this fails, ask the person to install
[Docker Desktop](https://www.docker.com/products/docker-desktop/) (or Docker Engine on
Linux) and start it; it needs administrator rights, which you do not have. Wait, then
run it again.

```bash
gh auth status
```

Sentinel reaches GitHub with the GitHub CLI session. If it is not signed in, ask the
person to run `gh auth login` themselves, then run `gh auth status` again.

## 2. Install the `sentinel` command

On Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.ps1 | iex
```

The installer puts the command in `~/.local/bin/sentinel` (Linux, macOS) or
`%LOCALAPPDATA%\Programs\sentinel\sentinel.exe` (Windows) and adds that folder to the
PATH of new terminals. Your own shell started earlier, so call it by that full path
for the rest of these steps.

## 3. Bring Sentinel up

Ask the person for the email they want as the owner of this installation, then:

```bash
sentinel up --email <their email>
```

The first time it downloads the images, which takes a few minutes. It ends with a
sign-in link: give it to the person. The panel is at http://localhost:8080.

## 4. Sign in the command line

```bash
sentinel login --url http://localhost:8080
```

It prints a code and a page. Ask the person to open the page, check that it shows the
same code and approve it. The command finishes by itself.

## 5. Give Sentinel a model for its QA and reviewers

Sentinel's QA and reviewers need a model of their own. Ask the person which one:

- **Their ChatGPT plan, with no API key:** they run `sentinel login --chatgpt` and sign
  in to ChatGPT in their browser.
- **A model on this computer** (Ollama, LM Studio, vLLM):
  `sentinel models add <name> <address>`, for example
  `sentinel models add ollama http://localhost:11434/v1`.
- **An API key:** the person adds it themselves in the panel, Settings › Model access.

## 6. Connect the agents on this computer

```bash
sentinel agents
```

It adds Sentinel to Claude Code, Codex (also the Codex and ChatGPT desktop apps, which
share its configuration), Gemini CLI, Cursor and the Claude desktop app, keeping a
copy of each file it changes. For Claude Code, the Sentinel plugin also teaches it when
to use Sentinel:

```bash
claude plugin marketplace add Chisu-Technologies/sentinel-dev
```

```bash
claude plugin install sentinel@chisu-sentinel
```

## 7. Check it and hand over

```bash
sentinel doctor
```

```bash
sentinel runs
```

Tell the person what you installed and that agents which were already open, you
included, must be restarted to load Sentinel. From then on they can ask you to
"verify this change with Sentinel", see the runs, or open the panel at
http://localhost:8080.

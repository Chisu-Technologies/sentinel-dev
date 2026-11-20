package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/mcp"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// What sentinel_brain hands the agent is bounded: a large repository has
// thousands of files, and the agent reads this before it starts, in its own
// context.
const (
	brainMaxFiles    = 300
	brainMaxHeadings = 8
	brainMaxDigest   = 32 << 10
)

// mcpClient is the server client of a tool, or the failure that says why
// there is none (no sign-in, no server).
func mcpClient(env Env) (*server.Client, *mcp.Result) {
	var notes bytes.Buffer
	quiet := env
	quiet.Stdout, quiet.Stderr = &notes, &notes
	client, code := newServerClient(quiet, "mcp")
	if code >= 0 {
		return nil, &mcp.Result{Text: strings.TrimSpace(notes.String()), IsError: true}
	}
	return client, nil
}

func mcpJSON(v any) mcp.Result {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	return mcp.Result{Text: string(data)}
}

func mcpRepos(ctx context.Context, env Env) mcp.Result {
	client, failed := mcpClient(env)
	if failed != nil {
		return *failed
	}
	repos, err := client.Repos(ctx)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	type repoLine struct {
		Repo    string `json:"repo"`
		Brain   string `json:"brain"`
		Version *int   `json:"brain_version,omitempty"`
		Digest  string `json:"digest"`
	}
	out := make([]repoLine, 0, len(repos))
	for _, r := range repos {
		out = append(out, repoLine{
			Repo:    client.Clean(r.FullName, 200),
			Brain:   client.Clean(r.ContextStatus, 32),
			Version: r.ContextVersion,
			Digest:  client.Clean(r.DigestStatus, 32),
		})
	}
	return mcpJSON(out)
}

func mcpBrain(ctx context.Context, env Env, repo string) mcp.Result {
	client, failed := mcpClient(env)
	if failed != nil {
		return *failed
	}
	repos, err := client.Repos(ctx)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	var match *server.Repo
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, strings.TrimSpace(repo)) {
			match = &repos[i]
			break
		}
	}
	if match == nil {
		return mcp.Result{Text: "Sentinel has no repository " + client.Clean(repo, 200) + "; sentinel_list_repos lists the ones it has", IsError: true}
	}
	found, ok, err := client.Context(ctx, match.ID)
	if err != nil {
		return mcp.Result{Text: err.Error(), IsError: true}
	}
	if !ok {
		return mcp.Result{Text: "The Brain has not read " + client.Clean(match.FullName, 200) + " yet."}
	}
	var artifact struct {
		Index struct {
			Artifacts []struct {
				Path     string   `json:"path"`
				Kind     string   `json:"kind"`
				Headings []string `json:"headings"`
			} `json:"artifacts"`
		} `json:"index"`
		Digest json.RawMessage `json:"digest"`
	}
	if found.ArtifactJSON != "" {
		if err := json.Unmarshal([]byte(found.ArtifactJSON), &artifact); err != nil {
			return mcp.Result{Text: "the Brain's artifact for this repository is not the JSON that was expected", IsError: true}
		}
	}
	type file struct {
		Path     string   `json:"path"`
		Kind     string   `json:"kind,omitempty"`
		Headings []string `json:"headings,omitempty"`
	}
	out := struct {
		Repo       string          `json:"repo"`
		Status     string          `json:"status"`
		Commit     string          `json:"commit_sha"`
		BuiltAt    *string         `json:"built_at,omitempty"`
		Files      []file          `json:"files"`
		FilesTotal int             `json:"files_total"`
		Digest     json.RawMessage `json:"digest,omitempty"`
		Note       string          `json:"note,omitempty"`
	}{
		Repo:       client.Clean(match.FullName, 200),
		Status:     client.Clean(found.Status, 32),
		Commit:     client.Clean(found.CommitSHA, 64),
		BuiltAt:    found.BuiltAt,
		FilesTotal: len(artifact.Index.Artifacts),
	}
	for _, a := range artifact.Index.Artifacts {
		if len(out.Files) == brainMaxFiles {
			out.Note = "only the first files are listed"
			break
		}
		f := file{Path: client.Clean(a.Path, 300), Kind: client.Clean(a.Kind, 32)}
		for _, h := range a.Headings {
			if len(f.Headings) == brainMaxHeadings {
				break
			}
			f.Headings = append(f.Headings, client.Clean(h, 200))
		}
		out.Files = append(out.Files, f)
	}
	switch digest := bytes.TrimSpace(artifact.Digest); {
	case len(digest) == 0 || string(digest) == "{}" || string(digest) == "null":
	case len(digest) > brainMaxDigest:
		out.Note = strings.TrimPrefix(out.Note+"; the digest is larger than this answer holds: open the Brain in the panel", "; ")
	default:
		out.Digest = digest
	}
	return mcpJSON(out)
}

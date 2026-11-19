package server

import (
	"context"
	"errors"
	"net/http"
)

// Repo is one repository connected to the server.
type Repo struct {
	ID             string `json:"id"`
	FullName       string `json:"full_name"`
	ContextStatus  string `json:"context_status"`
	ContextVersion *int   `json:"context_version"`
	DigestStatus   string `json:"digest_status"`
}

// RepoContext is what the Brain knows of one repository: the last artifact the
// server built from it. ArtifactJSON is the artifact itself, a JSON document,
// only when it was asked for.
type RepoContext struct {
	Repo         string  `json:"repo"`
	Status       string  `json:"status"`
	Version      int     `json:"version"`
	CommitSHA    string  `json:"commit_sha"`
	BuiltAt      *string `json:"built_at"`
	ArtifactJSON string  `json:"artifact_json"`
}

// Repos lists the repositories connected to the server.
func (c *Client) Repos(ctx context.Context) ([]Repo, error) {
	var repos []Repo
	err := c.do(ctx, http.MethodGet, "/api/org/repos", nil, http.StatusOK, &repos)
	return repos, err
}

// Context reads what the Brain knows of a repository, with its artifact.
// found is false when the server has built nothing from it yet.
func (c *Client) Context(ctx context.Context, repoID string) (out RepoContext, found bool, err error) {
	if !ValidRunID(repoID) {
		return RepoContext{}, false, errors.New("not a valid repository id")
	}
	status, data, tooLarge, err := c.fetch(ctx, http.MethodGet, "/api/org/repos/"+repoID+"/context?full=true", nil, "application/json")
	if err != nil {
		return RepoContext{}, false, err
	}
	switch status {
	case http.StatusNotFound:
		return RepoContext{}, false, nil
	case http.StatusOK:
		return out, true, decode(data, tooLarge, &out)
	}
	return RepoContext{}, false, c.apiError(status, data, tooLarge)
}

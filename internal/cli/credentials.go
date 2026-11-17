package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// credentials is what `sentinel login` keeps: one server and its API key. The
// environment wins over it, so SENTINEL_URL and SENTINEL_API_KEY still work
// as before for CI and scripts.
type credentials struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

var errNoConfigDir = errors.New("this user has no configuration folder to keep a sign-in in")

// credentialsPath is <user config dir>/sentinel/credentials.json: on Linux
// ~/.config, on macOS ~/Library/Application Support, on Windows %AppData%.
func credentialsPath(env Env) (string, error) {
	if env.ConfigDir == "" {
		return "", errNoConfigDir
	}
	return filepath.Join(env.ConfigDir, "sentinel", "credentials.json"), nil
}

// loadCredentials returns the saved sign-in, or ok=false when there is none.
func loadCredentials(env Env) (creds credentials, ok bool, err error) {
	path, err := credentialsPath(env)
	if errors.Is(err, errNoConfigDir) {
		return credentials{}, false, nil
	}
	if err != nil {
		return credentials{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return credentials{}, false, nil
	}
	if err != nil {
		return credentials{}, false, err
	}
	if json.Unmarshal(data, &creds) != nil || creds.URL == "" || creds.APIKey == "" {
		return credentials{}, false, fmt.Errorf("%s is not a sign-in this command wrote; remove it and run sentinel login", path)
	}
	return creds, true, nil
}

// saveCredentials writes the sign-in readable by this user only. It is written
// beside its place and renamed, so an interrupted write never leaves half a key.
func saveCredentials(env Env, creds credentials) (string, error) {
	path, err := credentialsPath(env)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}

// removeCredentials deletes the saved sign-in; ok=false when there was none.
func removeCredentials(env Env) (path string, ok bool, err error) {
	path, err = credentialsPath(env)
	if err != nil {
		return "", false, err
	}
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, false, nil
	}
	return path, err == nil, err
}

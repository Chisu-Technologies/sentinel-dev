package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ModelsSetting is where the server reads the endpoints of the models it may
// call besides the providers it knows. The open edition accepts there a model
// served on this machine (Ollama, LM Studio, vLLM); a hosted Sentinel does not.
const ModelsSetting = "SENTINEL_INFERENCE_OPERATOR_ENDPOINTS_JSON"

// HostName is how the containers name this machine. Inside a container
// localhost is the container itself.
const HostName = "host.docker.internal"

// The services that read ModelsSetting; only they restart when it changes.
var modelServices = []string{"api", "broker"}

var modelName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ErrModelName is returned for a name the panel could not show as one choice.
var ErrModelName = errors.New("a model name is 1 to 32 lowercase letters, digits or dashes, starting with a letter or digit")

// ModelURL is the address the containers use for a model served at raw:
// localhost becomes host.docker.internal. Plain HTTP is only for this machine
// or a private address; anything else must be HTTPS, which is also what the
// server checks before it starts.
func ModelURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("%q is not an http:// or https:// address", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q must not carry a user, a query or a fragment", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "0.0.0.0" || host == "::" || isLoopback(host) {
		host = HostName
	}
	if u.Scheme == "http" && host != HostName && !isPrivate(host) {
		return "", fmt.Errorf("%q: plain http only reaches this machine or a private address; use https", raw)
	}
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isPrivate(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsPrivate()
}

// Models are the models set on this machine, by name.
func (i *Install) Models() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(i.Dir, envFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("Sentinel is not installed on this machine")
		}
		return nil, err
	}
	models := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), ModelsSetting+"=")
		if !ok {
			continue
		}
		value = strings.Trim(value, "'")
		if value == "" {
			return models, nil
		}
		if err := json.Unmarshal([]byte(value), &models); err != nil {
			return nil, fmt.Errorf("%s in %s is not a JSON object: %w", ModelsSetting, envFile, err)
		}
	}
	return models, nil
}

// AddModel sets the model name to be served at raw, replacing one of the same
// name, and restarts the services that read it. It returns the address the
// containers use.
func (i *Install) AddModel(ctx context.Context, name, raw string) (string, error) {
	if !modelName.MatchString(name) {
		return "", ErrModelName
	}
	address, err := ModelURL(raw)
	if err != nil {
		return "", err
	}
	models, err := i.Models()
	if err != nil {
		return "", err
	}
	for other, existing := range models {
		if other != name && existing == address {
			return "", fmt.Errorf("%s is already set as %q", address, other)
		}
	}
	models[name] = address
	return address, i.applyModels(ctx, models)
}

// RemoveModel removes the model name and restarts the services that read it.
func (i *Install) RemoveModel(ctx context.Context, name string) error {
	models, err := i.Models()
	if err != nil {
		return err
	}
	if _, ok := models[name]; !ok {
		return fmt.Errorf("no model is set as %q", name)
	}
	delete(models, name)
	return i.applyModels(ctx, models)
}

func (i *Install) applyModels(ctx context.Context, models map[string]string) error {
	if err := i.ready(ctx); err != nil {
		return err
	}
	value := ""
	if len(models) > 0 {
		encoded, err := json.Marshal(models) // map keys come out sorted
		if err != nil {
			return err
		}
		// Single quotes: compose reads the JSON as it is, with no ${...}
		// interpolation. Names and addresses never hold a quote.
		value = "'" + string(encoded) + "'"
	}
	if err := i.setEnv(ModelsSetting, value); err != nil {
		return err
	}
	i.say("Restarting %s", strings.Join(modelServices, " and "))
	args := append([]string{"up", "-d", "--no-deps", "--wait"}, modelServices...)
	_, err := i.run(ctx, "restarting "+strings.Join(modelServices, " and "), i.compose(args...))
	return err
}

// ModelNames are the names of models, sorted.
func ModelNames(models map[string]string) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

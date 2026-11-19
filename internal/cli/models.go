package cli

import (
	"context"
	"flag"
	"fmt"

	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
)

const modelsUsage = "models [--dir <folder>] [list | add <name> <address> | remove <name>]"

// runModels lists, adds and removes the models served on this machine (Ollama,
// LM Studio, vLLM) that the Sentinel here may call.
func runModels(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	dir := fs.String("dir", "", "the folder the installation keeps its files in")
	if code := parse(fs, args, env, modelsUsage); code >= 0 {
		return code
	}
	action, rest := "list", fs.Args()
	if len(rest) > 0 {
		action, rest = rest[0], rest[1:]
	}
	want := map[string]int{"list": 0, "add": 2, "remove": 1}
	n, known := want[action]
	if !known || len(rest) != n {
		fmt.Fprintf(env.Stderr, "usage: sentinel %s\n", modelsUsage)
		return ExitUsage
	}
	install, code := localInstall(env, "models", *dir)
	if code >= 0 {
		return code
	}
	switch action {
	case "add":
		address, err := install.AddModel(ctx, rest[0], rest[1])
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel models: %v\n", err)
			return ExitFailed
		}
		fmt.Fprintf(env.Stdout, "%s is set as %s.\n", rest[0], address)
		fmt.Fprintln(env.Stdout, "In Settings > API keys, add a key to Operator OpenAI-Compatible Endpoint (any value if the model asks for none),")
		fmt.Fprintf(env.Stdout, "choose the endpoint %s and type the model's name as the model serves it.\n", rest[0])
	case "remove":
		if err := install.RemoveModel(ctx, rest[0]); err != nil {
			fmt.Fprintf(env.Stderr, "sentinel models: %v\n", err)
			return ExitFailed
		}
		fmt.Fprintf(env.Stdout, "%s is removed.\n", rest[0])
	default:
		models, err := install.Models()
		if err != nil {
			fmt.Fprintf(env.Stderr, "sentinel models: %v\n", err)
			return ExitFailed
		}
		if len(models) == 0 {
			fmt.Fprintln(env.Stdout, "No model on this machine is set. sentinel models add <name> <address> sets one, for example:")
			fmt.Fprintln(env.Stdout, "  sentinel models add ollama http://localhost:11434/v1")
			return ExitOK
		}
		for _, name := range local.ModelNames(models) {
			fmt.Fprintf(env.Stdout, "%s\t%s\n", name, models[name])
		}
	}
	return ExitOK
}

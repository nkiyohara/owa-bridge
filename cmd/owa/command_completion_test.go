package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"
	"github.com/posener/complete"
)

func TestCompletionScriptsAreRelocatableAndCurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell string
		file  string
		want  string
	}{
		{shell: "bash", file: "owa.bash", want: "complete -o default"},
		{shell: "zsh", file: "_owa", want: "bashcompinit"},
		{shell: "fish", file: "owa.fish", want: "command owa"},
	}
	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := run(context.Background(), []string{"completion", test.shell}, &stdout, &stderr); code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("completion output = %q, want %q", stdout.String(), test.want)
			}
			if strings.Contains(stdout.String(), filepath.Clean(os.Args[0])) {
				t.Fatalf("completion output embeds executable path: %q", stdout.String())
			}
			committedPath := filepath.Join("..", "..", "completions", test.file)
			committed, err := os.ReadFile(committedPath) // #nosec G304 -- fixed repository fixture path.
			if err != nil {
				t.Fatalf("read committed completion: %v", err)
			}
			if string(committed) != stdout.String() {
				t.Fatalf("%s is stale; regenerate it with `owa completion %s`", committedPath, test.shell)
			}
		})
	}
}

func TestCompletionModelPredictsNestedCommandsAndEnums(t *testing.T) {
	t.Parallel()

	parser, err := kong.New(&cli{}, kong.Name("owa"))
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	command, err := kongcompletion.Command(parser)
	if err != nil {
		t.Fatalf("create completion model: %v", err)
	}

	tests := []struct {
		name string
		args complete.Args
		want string
	}{
		{
			name: "nested command",
			args: complete.Args{
				All:           []string{"mcp", "se"},
				Completed:     []string{"mcp"},
				Last:          "se",
				LastCompleted: "mcp",
			},
			want: "serve",
		},
		{
			name: "enum",
			args: complete.Args{
				All:           []string{"completion", "b"},
				Completed:     []string{"completion"},
				Last:          "b",
				LastCompleted: "completion",
			},
			want: "bash",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			predictions := command.Predict(test.args)
			if !containsString(predictions, test.want) {
				t.Fatalf("predictions = %#v, want %q", predictions, test.want)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

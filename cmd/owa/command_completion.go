package main

import (
	"errors"
	"fmt"
)

type completionCommand struct {
	Shell string `arg:"" enum:"bash,zsh,fish" help:"Shell to generate completion for."`
}

var completionScripts = map[string]string{
	"bash": `# bash completion for owa
# The installed owa command resolves itself from PATH for relocatable archives.
complete -o default -o bashdefault -C owa owa
`,
	"zsh": `#compdef owa
# zsh completion for owa through its bash-compatible completion protocol.
autoload -U +X bashcompinit && bashcompinit
complete -o default -o bashdefault -C owa owa
`,
	"fish": `# fish completion for owa
function __owa_complete
    set -lx COMP_LINE (commandline -cp)
    test -z (commandline -ct); and set COMP_LINE "$COMP_LINE "
    command owa
end
complete -f -c owa -a "(__owa_complete)"
`,
}

func (command *completionCommand) Run(app *runtime) error {
	script, exists := completionScripts[command.Shell]
	if !exists {
		return errors.New("unsupported completion shell")
	}
	_, err := fmt.Fprint(app.stdout, script)
	return err
}

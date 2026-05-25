package cmd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

// CompletionCmd prints shell completion scripts.
type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell to generate completions for."`
}

// Run writes a basic shell completion script for the selected shell.
func (c *CompletionCmd) Run(ctx *kong.Context) error {
	k := ctx.Kong
	switch c.Shell {
	case "bash":
		return c.runBash(k)
	case "zsh":
		return c.runZsh(k)
	case "fish":
		return c.runFish(k)
	case "powershell":
		return c.runPowershell(k)
	}
	return nil
}

func (c *CompletionCmd) runBash(k *kong.Kong) error {
	commands := c.allCommands(k)
	fmt.Fprintf(k.Stdout, `_disbug_completion() {
    local cur opts
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts="%s"
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
}
complete -F _disbug_completion disbug
`, strings.Join(commands, " "))
	return nil
}

func (c *CompletionCmd) runZsh(k *kong.Kong) error {
	commands := c.allCommands(k)
	fmt.Fprintf(k.Stdout, `#compdef disbug
_disbug() {
    local -a commands
    commands=(%s)
    _describe 'commands' commands
}
_disbug
`, strings.Join(commands, " "))
	return nil
}

func (c *CompletionCmd) runFish(k *kong.Kong) error {
	commands := c.allCommands(k)
	for _, cmd := range commands {
		fmt.Fprintf(k.Stdout, "complete -c disbug -f -a %s\n", cmd)
	}
	return nil
}

func (c *CompletionCmd) runPowershell(k *kong.Kong) error {
	commands := c.allCommands(k)
	fmt.Fprintf(k.Stdout, `Register-ArgumentCompleter -Native -CommandName disbug -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $commands = @('%s')
    $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, strings.Join(commands, "','"))
	return nil
}

func (c *CompletionCmd) allCommands(k *kong.Kong) []string {
	var cmds []string
	for _, node := range k.Model.Children {
		if !node.Hidden && node.Type == kong.CommandNode {
			cmds = append(cmds, node.Name)
		}
	}
	return cmds
}

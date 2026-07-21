package cmd

import (
	"fmt"
	"strings"
)

// CompletionCmd prints shell completion scripts.
type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell to generate completions for."`
}

// Run writes the requested shell completion script.
func (c CompletionCmd) Run(b bindings) error {
	var script string
	switch c.Shell {
	case "bash":
		script = bashCompletion()
	case "zsh":
		script = zshCompletion()
	case "fish":
		script = fishCompletion()
	case "powershell":
		script = powershellCompletion()
	default:
		return fmt.Errorf("unsupported shell %q", c.Shell)
	}

	_, err := fmt.Fprint(b.Stdout, script)
	return err
}

var completionCommands = []string{
	"sessions",
	"session",
	"pin",
	"pins",
	"search",
	"watch",
	"inspect",
	"login",
	"logout",
	"whoami",
	"doctor",
	"mcp",
	"completion",
	"version",
}

var completionGlobalFlags = []string{
	"--pretty",
	"--profile",
	"--verbose",
	"-v",
	"--version",
	"--help",
	"-h",
}

var completionShells = []string{"bash", "zsh", "fish", "powershell"}

func bashCompletion() string {
	return fmt.Sprintf(`# bash completion for disbug.
_disbug_completion() {
  local cur prev commands global_flags shells
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  commands="%s"
  global_flags="%s"
  shells="%s"

  if [[ "$prev" == "completion" ]]; then
    COMPREPLY=( $(compgen -W "$shells" -- "$cur") )
    return 0
  fi

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "$global_flags" -- "$cur") )
    return 0
  fi

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "$commands $global_flags" -- "$cur") )
    return 0
  fi

  return 0
}

complete -F _disbug_completion disbug
`, strings.Join(completionCommands, " "), strings.Join(completionGlobalFlags, " "), strings.Join(completionShells, " "))
}

func zshCompletion() string {
	return fmt.Sprintf(`#compdef disbug
# zsh completion for disbug.
_disbug_completion() {
  local -a commands global_flags shells
  commands=(%s)
  global_flags=(%s)
  shells=(%s)

  if [[ ${words[2]} == completion && $CURRENT -eq 3 ]]; then
    compadd -- $shells
    return
  fi

  if [[ ${words[CURRENT]} == -* ]]; then
    compadd -- $global_flags
    return
  fi

  if [[ $CURRENT -eq 2 ]]; then
    compadd -- $commands $global_flags
    return
  fi
}

compdef _disbug_completion disbug
`, strings.Join(completionCommands, " "), strings.Join(completionGlobalFlags, " "), strings.Join(completionShells, " "))
}

func fishCompletion() string {
	return fmt.Sprintf(`# fish completion for disbug.
function __disbug_complete
  set -l tokens (commandline -opc)
  if test (count $tokens) -ge 2; and test $tokens[2] = completion
    printf "%%s\n" %s
    return
  end

  printf "%%s\n" %s %s
end

complete -c disbug -f -a "(__disbug_complete)"
complete -c disbug -l pretty -d "Indent JSON output with 2 spaces"
complete -c disbug -l profile -r -d "Configuration profile to use"
complete -c disbug -s v -l verbose -d "Enable verbose logging"
complete -c disbug -l version -d "Show version"
complete -c disbug -s h -l help -d "Show help"
`, fishWords(completionShells), fishWords(completionCommands), fishWords(completionGlobalFlags))
}

func powershellCompletion() string {
	return fmt.Sprintf(`# PowerShell completion for disbug.
Register-ArgumentCompleter -Native -CommandName disbug -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)

  $commands = @(%s)
  $globalFlags = @(%s)
  $shells = @(%s)
  $tokens = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })

  if ($tokens.Count -ge 2 -and $tokens[1] -eq 'completion') {
    $values = $shells
  } elseif ($wordToComplete -like '-*') {
    $values = $globalFlags
  } else {
    $values = $commands + $globalFlags
  }

  $values |
    Where-Object { $_ -like "$wordToComplete*" } |
    ForEach-Object {
      [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, powershellWords(completionCommands), powershellWords(completionGlobalFlags), powershellWords(completionShells))
}

func fishWords(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "\\'")+"'")
	}

	return strings.Join(quoted, " ")
}

func powershellWords(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "''")+"'")
	}

	return strings.Join(quoted, ", ")
}

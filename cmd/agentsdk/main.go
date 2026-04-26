package main

import (
	"fmt"
	"os"

	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := rootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "agentsdk",
		Short:         "Run agentsdk resource bundles",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(cli.NewCommand(cli.CommandConfig{
		Name:        "agentsdk",
		Use:         "run <path> [task]",
		Short:       "Run an agent resource bundle",
		ResourceArg: true,
		AgentFlag:   true,
		Prompt:      "agentsdk> ",
	}))
	return cmd
}

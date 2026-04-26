package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/spf13/cobra"
)

const maxDiscoverDescriptionRunes = 180

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
		Use:         "run [path] [task]",
		Short:       "Run an agent resource bundle",
		ResourceArg: true,
		AgentFlag:   true,
		Prompt:      "agentsdk> ",
	}))
	cmd.AddCommand(discoverCmd())
	return cmd
}

func discoverCmd() *cobra.Command {
	var localOnly bool
	cmd := &cobra.Command{
		Use:           "discover [path]",
		Short:         "Discover agent resources without running them",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			policy := resource.DiscoveryPolicy{
				IncludeGlobalUserResources: true,
				IncludeExternalEcosystems:  true,
				AllowRemote:                true,
			}
			if localOnly {
				policy.IncludeGlobalUserResources = false
				policy.AllowRemote = false
			}
			resolved, err := agentdir.ResolveDirWithOptions(dir, agentdir.ResolveOptions{Policy: policy, LocalOnly: localOnly})
			if err != nil {
				return err
			}
			return printDiscovery(cmd.OutOrStdout(), resolved)
		},
	}
	cmd.Flags().BoolVar(&localOnly, "local", false, "Only inspect the specified workspace/path")
	return cmd
}

type discoveryWriter interface {
	Write([]byte) (int, error)
}

func printDiscovery(out discoveryWriter, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle), app.WithoutBuiltins())
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Sources:")
	if len(resolved.Sources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range resolved.Sources {
		fmt.Fprintf(out, "  %s\n", source)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Agents:")
	agentSpecs := imported.AgentSpecs()
	if len(agentSpecs) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, spec := range agentSpecs {
		id := spec.ResourceID
		if id == "" {
			id = spec.Name
		}
		fmt.Fprintf(out, "  %s  %s  %s\n", spec.Name, displayDescription(spec.Description), id)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	commands := imported.Commands().All()
	if len(commands) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, cmd := range commands {
		spec := cmd.Spec()
		fmt.Fprintf(out, "  /%s  %s\n", spec.Name, displayDescription(spec.Description))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skills:")
	skills := firstSkillContributions(resolved.Bundle.Skills)
	if len(skills) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, skill := range skills {
		fmt.Fprintf(out, "  %s  %s  %s\n", skill.Name, displayDescription(skill.Description), skill.ID)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skill sources:")
	skillSources := imported.SkillSources()
	if len(skillSources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range skillSources {
		fmt.Fprintf(out, "  %s  %s\n", source.ID, source.Label)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Disabled suggestions:")
	hasDisabled := false
	for _, tool := range resolved.Bundle.Tools {
		if tool.Enabled {
			continue
		}
		hasDisabled = true
		fmt.Fprintf(out, "  tool %s  %s\n", tool.ID, tool.Description)
	}
	if !hasDisabled {
		fmt.Fprintln(out, "  none")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Diagnostics:")
	diagnostics := imported.Diagnostics()
	if len(diagnostics) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, diag := range diagnostics {
		fmt.Fprintf(out, "  %s  %s  %s\n", diag.Severity, diag.Source.Label(), diag.Message)
	}
	return nil
}

func firstSkillContributions(skills []resource.SkillContribution) []resource.SkillContribution {
	seen := map[string]bool{}
	out := make([]resource.SkillContribution, 0, len(skills))
	for _, skill := range skills {
		if skill.Name == "" || seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		out = append(out, skill)
	}
	return out
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, `\n`, " ")
	return strings.Join(strings.Fields(s), " ")
}

func displayDescription(s string) string {
	s = oneLine(s)
	if utf8.RuneCountInString(s) <= maxDiscoverDescriptionRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxDiscoverDescriptionRunes-1])) + "..."
}

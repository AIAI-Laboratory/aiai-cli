package command

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/skill"
)

func (s *state) skillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skill",
		Aliases: []string{"skills"},
		Short:   "Manage agent skills from AIAI-Laboratory/aiai-skills",
		Args:    cobra.NoArgs,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if s.deps.Skills == nil {
				return errors.New("skill manager is unavailable")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(s.skillAddCommand(), s.skillListCommand(), s.skillRemoveCommand(), s.skillUpdateCommand())
	return cmd
}

func (s *state) skillAddCommand() *cobra.Command {
	var global, all, force, list bool
	cmd := &cobra.Command{
		Use:     "add [skills...]",
		Aliases: []string{"install", "a"},
		Short:   "Download and install skills from the AIAI skill repository",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, names []string) error {
			if list {
				if len(names) > 0 || all || force || global {
					return fmt.Errorf("%w: --list cannot be combined with names, --all, --force, or --global", skill.ErrValidation)
				}
				result, err := s.deps.Skills.Available(cmd.Context())
				return s.finishSkill(result, err)
			}
			result, err := s.deps.Skills.Install(cmd.Context(), names, skillScope(global), all, force)
			return s.finishSkill(result, err)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "Install to the user-level ~/.agents/skills directory")
	cmd.Flags().BoolVar(&all, "all", false, "Install every available AIAI skill")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing installed skill")
	cmd.Flags().BoolVarP(&list, "list", "l", false, "List skills available from the AIAI repository")
	return cmd
}

func (s *state) skillListCommand() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed AIAI skills",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			result, err := s.deps.Skills.List(skillScope(global))
			return s.finishSkill(result, err)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "List user-level skills from ~/.agents/skills")
	return cmd
}

func (s *state) skillRemoveCommand() *cobra.Command {
	var global, all bool
	cmd := &cobra.Command{
		Use:     "remove [skills...]",
		Aliases: []string{"rm", "r"},
		Short:   "Remove installed AIAI skills",
		Args:    cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, names []string) error {
			result, err := s.deps.Skills.Remove(names, skillScope(global), all)
			return s.finishSkill(result, err)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "Remove from the user-level ~/.agents/skills directory")
	cmd.Flags().BoolVar(&all, "all", false, "Remove every installed skill in the selected scope")
	return cmd
}

func (s *state) skillUpdateCommand() *cobra.Command {
	var global, all bool
	cmd := &cobra.Command{
		Use:     "update [skills...]",
		Aliases: []string{"upgrade"},
		Short:   "Refresh installed skills from the AIAI skill repository",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, names []string) error {
			result, err := s.deps.Skills.Update(cmd.Context(), names, skillScope(global), all)
			return s.finishSkill(result, err)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "Update user-level skills in ~/.agents/skills")
	cmd.Flags().BoolVar(&all, "all", false, "Update every installed skill in the selected scope")
	return cmd
}

func (s *state) finishSkill(result *skill.Result, err error) error {
	if err != nil {
		return err
	}
	s.skill = result
	if !s.json {
		fmt.Fprint(s.human(), output.SkillText(*result))
	}
	return nil
}

func skillScope(global bool) skill.Scope {
	if global {
		return skill.ScopeGlobal
	}
	return skill.ScopeProject
}

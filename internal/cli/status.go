package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"

	"github.com/spf13/cobra"
)

var (
	statusProject string
)

func init() {
	statusCmd.Flags().StringVarP(&statusProject, "project", "p", "", "project name")
	statusCmd.MarkFlagRequired("project")
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show pipeline status for a project",
	Long:  `Display the current pipeline status, phase progress, and any errors for a project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		exists, err := state.ManifestExists(cfg.Pipeline.OutputDir, statusProject)
		if err != nil {
			return fmt.Errorf("checking project: %w", err)
		}
		if !exists {
			return fmt.Errorf("project %q not found in %s", statusProject, cfg.Pipeline.OutputDir)
		}

		m, err := state.LoadManifest(cfg.Pipeline.OutputDir, statusProject)
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Project:  %s\n", m.Project.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Created:  %s\n", m.Project.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(cmd.OutOrStdout(), "Source:   %s (%s)\n", m.Project.Source.Path, m.Project.Source.Type)
		fmt.Fprintf(cmd.OutOrStdout(), "Chapters: %d\n", len(m.Project.Chapters))
		fmt.Fprintf(cmd.OutOrStdout(), "\n")

		statusIcon := map[string]string{
			model.PhaseCompleted:  "✓",
			model.PhaseInProgress: "→",
			model.PhaseFailed:     "✗",
			model.PhasePending:    "·",
			model.PhaseIdle:       "·",
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintf(w, "Pipeline: %s\n", m.Pipeline.Status)
		fmt.Fprintf(w, "Current:  phase %d/6\n", m.Pipeline.CurrentPhase)
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "Phase\tStatus\tDetails\n")
		fmt.Fprintf(w, "-----\t------\t-------\n")
		for _, phase := range model.AllPhases {
			ps, ok := m.Pipeline.Phases[phase]
			if !ok {
				continue
			}
			icon := statusIcon[ps.Status]
			if icon == "" {
				icon = "?"
			}

			details := ""
			if ps.StartedAt != nil {
				details = fmt.Sprintf("started %s", ps.StartedAt.Format(time.RFC3339))
			}
			if ps.CompletedAt != nil {
				details = fmt.Sprintf("completed %s", ps.CompletedAt.Format(time.RFC3339))
			}

			fmt.Fprintf(w, "%s %s\t%s\t%s\n", icon, phase, ps.Status, details)
		}
		w.Flush()

		if len(m.Pipeline.Errors) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nErrors:\n")
			for _, e := range m.Pipeline.Errors {
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", e.Timestamp.Format(time.RFC3339), e.Phase, e.Message)
			}
		}

		return nil
	},
}

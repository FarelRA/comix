package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/FarelRA/comix/internal/state"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Long:  `List all Comix projects in the output directory with their pipeline status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		root := cfg.Pipeline.OutputDir
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No projects found.")
				return nil
			}
			return fmt.Errorf("reading output directory: %w", err)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

		var count int
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(root, entry.Name(), "project.yaml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				continue
			}
			m, err := state.LoadManifest(root, entry.Name())
			if err != nil {
				continue
			}

			phaseInfo := fmt.Sprintf("phase %d/6", m.Pipeline.CurrentPhase)
			if m.Pipeline.Status == "completed" {
				phaseInfo = "done"
			} else if m.Pipeline.Status == "idle" {
				phaseInfo = "not started"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%d chapters\n",
				m.Project.Name,
				m.Pipeline.Status,
				phaseInfo,
				len(m.Project.Chapters))
			count++
		}

		if count == 0 {
			fmt.Println("No projects found.")
			return nil
		}

		w.Flush()
		return nil
	},
}

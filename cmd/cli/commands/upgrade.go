package commands

import (
	"fmt"
	"log"

	"github.com/packalares/packalares/pkg/installer/phases"
	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	var targetVersion string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Packalares to a new version",
		Run: func(cmd *cobra.Command, args []string) {
			if targetVersion == "" {
				log.Fatal("--version is required")
			}
			if err := phases.RunUpgrade(targetVersion); err != nil {
				log.Fatalf("upgrade failed: %v", err)
			}
			fmt.Printf("\nPackalares upgraded to %s.\n", targetVersion)
		},
	}

	cmd.Flags().StringVar(&targetVersion, "version", "", "target version to upgrade to (required)")

	return cmd
}

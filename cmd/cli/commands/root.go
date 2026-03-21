package commands

import (
	"github.com/spf13/cobra"
)

const version = "1.0.0"

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "packalares",
		Short:   "Packalares — self-hosted Olares installer",
		Version: version,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	root.AddCommand(
		newInstallCmd(),
		newUninstallCmd(),
		newPrecheckCmd(),
		newStatusCmd(),
		newUpgradeCmd(),
	)

	return root
}

package commands

import (
	"fmt"
	"log"

	"github.com/packalares/packalares/pkg/installer/phases"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Packalares completely",
		Long:  `Removes all Packalares components: K3s, etcd, containerd, Redis, systemd services, and Kubernetes resources.`,
		Run: func(cmd *cobra.Command, args []string) {
			if !force {
				fmt.Print("This will remove ALL Packalares components. Are you sure? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					fmt.Println("Aborted.")
					return
				}
			}
			if err := phases.RunUninstall(); err != nil {
				log.Fatalf("uninstall failed: %v", err)
			}
			fmt.Println("\nPackalares has been uninstalled.")
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")

	return cmd
}

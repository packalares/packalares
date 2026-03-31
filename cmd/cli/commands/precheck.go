package commands

import (
	"fmt"
	"log"
	"os"

	"github.com/packalares/packalares/pkg/installer/precheck"
	"github.com/spf13/cobra"
)

func newPrecheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "precheck",
		Short: "Check system requirements for Packalares installation",
		Run: func(cmd *cobra.Command, args []string) {
			result := precheck.RunPrecheck()
			precheck.PrintReport(result, os.Stdout)
			if !result.Passed {
				log.Fatal("precheck failed — fix the issues above before installing")
			}
			fmt.Println("\nAll checks passed. System is ready for installation.")
		},
	}

	return cmd
}

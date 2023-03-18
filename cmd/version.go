package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Version of terraform-atlantis-config",
	Long:  "Version of terraform-atlantis-config",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(rootCmd.Use + " " + VERSION)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

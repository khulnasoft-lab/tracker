package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var version string

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"v"},
	Short:   "Print the version number of Tracker",
	Long:    ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Tracker version: %s\n", version)
	},
	DisableFlagsInUseLine: true,
}

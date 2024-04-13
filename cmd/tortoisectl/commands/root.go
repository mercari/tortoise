package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tortoisectl [COMMANDS]",
	Short: "tortoisectl is a CLI for managing Tortoise",
	Long:  `tortoisectl is a CLI for managing Tortoise.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

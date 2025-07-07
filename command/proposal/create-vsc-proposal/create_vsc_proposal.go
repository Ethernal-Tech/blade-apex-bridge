package createvscproposal

import (
	"fmt"
	"os"

	addvalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/add-validator"
	dropvalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/drop-validator"
	removevalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/remove-validator"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create-vsc-proposal",
		Short:             "Top level command to create validator set change proposal.",
		PersistentPreRunE: validateFileFlag,
	}

	cmd.PersistentFlags().String(
		"file",
		"",
		"Path to the file representing the proposal. "+
			"File will be created if missing, otherwise updated.",
	)

	cmd.AddCommand(addvalidator.GetCommand())
	cmd.AddCommand(removevalidator.GetCommand())
	cmd.AddCommand(dropvalidator.GetCommand())

	return cmd
}

func validateFileFlag(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("file")
	if path == "" {
		return fmt.Errorf("path to the file representing the proposal must be specified")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			file.Close()

			return nil
		}

		return fmt.Errorf("could not stat file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("path exists but is a directory")
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("file is not writable: %w", err)
	}

	file.Close()

	return nil
}

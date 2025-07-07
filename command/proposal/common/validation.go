package common

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func ValidateFileFlag(cmd *cobra.Command, _ []string) error {
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

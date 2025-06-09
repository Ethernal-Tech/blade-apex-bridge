package triestorageanalysis

import (
	"github.com/spf13/cobra"
)

var (
	params = &trieStorageAnalysisParams{}
)

type trieStorageAnalysisParams struct {
	DataPath       string
	DBEngine       string
	BlockNumFrom   uint64
	BlockNumTo     uint64
	Accs           []string
	AccStorageOnly bool
	Verbose        bool
}

func GetCommand() *cobra.Command {
	saCMD := TrieStorageAnalysisCMD()

	return saCMD
}

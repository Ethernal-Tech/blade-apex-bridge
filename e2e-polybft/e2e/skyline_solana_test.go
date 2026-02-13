package e2e

import (
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/stretchr/testify/require"
)

// To run solana tests be sure to have necessary tools installed:
// https://solana.com/docs/intro/installation

func Test_SkylineSolana(t *testing.T) {
	solanaChain, err := cardanofw.NewTestSolanaChain(cardanofw.NewTestSolanaChainConfig())
	require.NoError(t, err)

	defer solanaChain.Stop()

	time.Sleep(60 * time.Second)
}

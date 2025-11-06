package cardanofw

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	PotentialFee     = 500_000
	ttlSlotNumberInc = 500
)

func GetGenesisWalletFromCluster(
	dirPath string,
	keyID uint,
) (*wallet.Wallet, error) {
	keyFileName := strings.Join([]string{"utxo", fmt.Sprint(keyID)}, "")

	sKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.skey", keyFileName)))
	if err != nil {
		return nil, err
	}

	sKeyBytes, err := sKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	return wallet.NewWallet(sKeyBytes, nil), nil
}

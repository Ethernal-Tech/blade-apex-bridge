package cardanofw

import (
	"encoding/json"
	"math/big"
	"os"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

func noChanges(mp map[string]interface{}) {}

func getShelleyGenesis(networkType wallet.CardanoNetworkType, initialSupply *big.Int) func(mp map[string]interface{}) {
	switch networkType {
	case wallet.TestNetNetwork:
		return func(mp map[string]interface{}) {
			testPrimeShelleyGenesis(mp, initialSupply)
		}
	case wallet.VectorTestNetNetwork:
		return testVectorShelleyGenesis
	default:
		return nil
	}
}

// Still not in conway era so this be left with noChanges
func getConwayGenesis(networkType wallet.CardanoNetworkType) func(mp map[string]interface{}) {
	switch networkType {
	case wallet.TestNetNetwork:
		return noChanges
	case wallet.VectorTestNetNetwork:
		return noChanges
	default:
		return nil
	}
}

func testPrimeShelleyGenesis(mp map[string]interface{}, initialSupply *big.Int) {
	totalSupply := int(45000000000000000)

	// Calculate remaining supply for reserves and treasury
	remaining := totalSupply - int(initialSupply.Int64()) // ~33,888,888,888,000,000

	// Split remaining between treasury and reserves
	treasury := remaining / 10       // 10% to treasury
	reserves := remaining - treasury // 90% to reserves

	mp["slotLength"] = 0.1
	mp["activeSlotsCoeff"] = 0.1
	mp["securityParam"] = 10
	// slotLength = 0.1 sec * 600 slots in epoch = 60 sec epoch
	mp["epochLength"] = 600
	mp["maxLovelaceSupply"] = totalSupply
	mp["treasury"] = treasury
	mp["reserves"] = reserves
	mp["updateQuorum"] = 2
	prParams := getMapFromInterfaceKey(mp, "protocolParams")
	getMapFromInterfaceKey(prParams, "protocolVersion")["major"] = 7
	prParams["minFeeA"] = 44
	prParams["minFeeB"] = 155381
	prParams["minUTxOValue"] = 1000000
	prParams["decentralisationParam"] = 0.7

	// monetaryExpandRate aka rho - governs the amount of tokens that are returned
	// from reserves to the ecosystem as rewards
	prParams["rho"] = 0.0055
	// treasuryGrowthRate aka tau - proportion of total rewards allocated to
	// treasury each epoch before remaining rewards are distributed to pools.
	prParams["tau"] = 0.000001
	//
	prParams["keyDeposit"] = 2000000
}

func testVectorShelleyGenesis(mp map[string]interface{}) {
	mp["slotLength"] = 1
	mp["activeSlotsCoeff"] = 0.25
	mp["securityParam"] = 216
	mp["epochLength"] = 8640
	mp["maxLovelaceSupply"] = 1000000000000
	mp["updateQuorum"] = 2
	prParams := getMapFromInterfaceKey(mp, "protocolParams")
	getMapFromInterfaceKey(prParams, "protocolVersion")["major"] = 7
	prParams["minFeeA"] = 45
	prParams["minFeeB"] = 156253
	prParams["minUTxOValue"] = 1000000
	prParams["decentralisationParam"] = 0.7
	prParams["rho"] = 0.00001
	prParams["tau"] = 0.000001
}

func updateJSON(content []byte, callback func(mp map[string]interface{})) ([]byte, error) {
	// Parse []byte into a map
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	callback(data)

	return json.MarshalIndent(data, "", "    ") // The second argument is the prefix, and the third is the indentation
}

func UpdateJSONFile(fn1 string, fn2 string, callback func(mp map[string]interface{}), removeOriginal bool) error {
	bytes, err := os.ReadFile(fn1)
	if err != nil {
		return err
	}

	bytes, err = updateJSON(bytes, callback)
	if err != nil {
		return err
	}

	if removeOriginal {
		os.Remove(fn1)
	}

	return os.WriteFile(fn2, bytes, 0600)
}

func getMapFromInterfaceKey(mp map[string]interface{}, key string) map[string]interface{} {
	var prParams map[string]interface{}

	if v, exists := mp[key]; !exists {
		prParams = map[string]interface{}{}
		mp[key] = prParams
	} else {
		prParams, _ = v.(map[string]interface{})
	}

	return prParams
}

func GetMapFromInterfaceKey(mp map[string]interface{}, keys ...string) map[string]interface{} {
	for _, k := range keys {
		mp = getMapFromInterfaceKey(mp, k)
	}

	return mp
}

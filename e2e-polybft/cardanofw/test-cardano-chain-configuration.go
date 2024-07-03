package cardanofw

import (
	"encoding/json"
	"os"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

func noChanges(mp map[string]interface{}) {}

func getShelleyGenesis(networkType wallet.CardanoNetworkType) func(mp map[string]interface{}) {
	switch networkType {
	case wallet.TestNetNetwork:
		return testPrimeShelleyGenesis
	case wallet.VectorTestNetNetwork:
		return testVectorShelleyGenesis
	default:
		return nil
	}
}

func getConwayGenesis(networkType wallet.CardanoNetworkType) func(mp map[string]interface{}) {
	switch networkType {
	case wallet.TestNetNetwork:
		return noChanges
	case wallet.VectorTestNetNetwork:
		return noChanges // TODO:SASA
	default:
		return nil
	}
}

func testPrimeShelleyGenesis(mp map[string]interface{}) {
	mp["slotLength"] = 0.1
	mp["activeSlotsCoeff"] = 0.1
	mp["securityParam"] = 10
	mp["epochLength"] = 500
	mp["maxLovelaceSupply"] = 1000000000000
	mp["updateQuorum"] = 2
	prParams := getMapFromInterfaceKey(mp, "protocolParams")
	getMapFromInterfaceKey(prParams, "protocolVersion")["major"] = 7
	prParams["minFeeA"] = 44
	prParams["minFeeB"] = 155381
	prParams["minUTxOValue"] = 1000000
	prParams["decentralisationParam"] = 0.7
	prParams["rho"] = 0.1
	prParams["tau"] = 0.1
}

// TODO:SASA check generated genesis
func testVectorShelleyGenesis(mp map[string]interface{}) {
	mp["slotLength"] = 1
	mp["activeSlotsCoeff"] = 0.5
	mp["securityParam"] = 2160
	mp["epochLength"] = 500
	mp["maxLovelaceSupply"] = 1000000000000
	mp["updateQuorum"] = 2
	prParams := getMapFromInterfaceKey(mp, "protocolParams")
	getMapFromInterfaceKey(prParams, "protocolVersion")["major"] = 7
	prParams["minFeeA"] = 44
	prParams["minFeeB"] = 155381
	prParams["minUTxOValue"] = 1000000
	prParams["decentralisationParam"] = 0.7
	prParams["rho"] = 0.1
	prParams["tau"] = 0.1
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

func updateJSONFile(fn1 string, fn2 string, callback func(mp map[string]interface{}), removeOriginal bool) error {
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

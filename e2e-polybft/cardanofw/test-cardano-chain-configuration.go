package cardanofw

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

func noChanges(mp map[string]interface{}) {}

func getShelleyGenesis(networkMagic uint) func(mp map[string]interface{}) {
	switch networkMagic {
	case wallet.PrimeTestNetProtocolMagic, wallet.TestNetProtocolMagic:
		return testPrimeShelleyGenesis
	case wallet.VectorTestNetProtocolMagic:
		return testVectorShelleyGenesis
	default:
		return nil
	}
}

// cardano-cli 11 generates pvt-prefixed keys in genesis.conway.json,
// but cardano-node expects the old unprefixed key names.
func fixConwayPoolVotingThresholds(mp map[string]interface{}) {
	// Fix poolVotingThresholds
	if pvt, ok := mp["poolVotingThresholds"].(map[string]interface{}); ok {
		pvtRemap := map[string]string{
			"pvtMotionNoConfidence":    "motionNoConfidence",
			"pvtCommitteeNormal":       "committeeNormal",
			"pvtCommitteeNoConfidence": "committeeNoConfidence",
			"pvtHardForkInitiation":    "hardForkInitiation",
			"pvtPPSecurityGroup":       "ppSecurityGroup",
		}
		fixed := remapKeys(pvt, pvtRemap)

		if _, exists := fixed["ppSecurityGroup"]; !exists {
			fixed["ppSecurityGroup"] = 0.51
		}

		mp["poolVotingThresholds"] = fixed
	}

	// Fix dRepVotingThresholds
	if dvt, ok := mp["dRepVotingThresholds"].(map[string]interface{}); ok {
		dvtRemap := map[string]string{
			"dvtMotionNoConfidence":    "motionNoConfidence",
			"dvtCommitteeNormal":       "committeeNormal",
			"dvtCommitteeNoConfidence": "committeeNoConfidence",
			"dvtUpdateToConstitution":  "updateToConstitution",
			"dvtHardForkInitiation":    "hardForkInitiation",
			"dvtPPNetworkGroup":        "ppNetworkGroup",
			"dvtPPEconomicGroup":       "ppEconomicGroup",
			"dvtPPTechnicalGroup":      "ppTechnicalGroup",
			"dvtPPGovGroup":            "ppGovGroup",
			"dvtTreasuryWithdrawal":    "treasuryWithdrawal",
		}
		mp["dRepVotingThresholds"] = remapKeys(dvt, dvtRemap)
	}

	// Add missing top-level fields required by cardano-node
	if _, exists := mp["minFeeRefScriptCostPerByte"]; !exists {
		mp["minFeeRefScriptCostPerByte"] = 44
	}
}

func remapKeys(m map[string]interface{}, remap map[string]string) map[string]interface{} {
	fixed := make(map[string]interface{}, len(m))

	for k, v := range m {
		if newKey, exists := remap[k]; exists {
			fixed[newKey] = v
		} else {
			fixed[k] = v
		}
	}

	return fixed
}

func getConwayGenesis(networkMagic uint) func(mp map[string]interface{}) {
	switch networkMagic {
	case wallet.TestNetProtocolMagic:
		return fixConwayPoolVotingThresholds
	default:
		return noChanges
	}
}

func testPrimeShelleyGenesis(mp map[string]interface{}) {
	mp["slotLength"] = 0.1
	mp["activeSlotsCoeff"] = 0.1
	mp["securityParam"] = 100
	mp["epochLength"] = 500
	mp["maxLovelaceSupply"] = 1000000000000
	mp["updateQuorum"] = 2
	prParams := getMapFromInterfaceKey(mp, "protocolParams")
	getMapFromInterfaceKey(prParams, "protocolVersion")["major"] = 7
	prParams["minFeeA"] = 47
	prParams["minFeeB"] = 158298
	prParams["minUTxOValue"] = 1000000
	prParams["decentralisationParam"] = 0.7
	prParams["rho"] = 0.1
	prParams["tau"] = 0.1
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

	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()

	if err := dec.Decode(&data); err != nil {
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

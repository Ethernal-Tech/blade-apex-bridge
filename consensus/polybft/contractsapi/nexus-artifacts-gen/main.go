package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	extension = ".sol"
)

func main() {
	_, filename, _, _ := runtime.Caller(0) //nolint: dogsled
	currentPath := path.Dir(filename)
	// TODO:Nexus update path
	proxyscpath := path.Join(currentPath, "../test-contracts")
	nexusscpath := path.Join(currentPath, "../../../../apex-bridge-smartcontracts/artifacts/contracts/")

	str := `// This is auto-generated file. DO NOT EDIT.
package contractsapi

`

	proxyContracts := []struct {
		Path string
		Name string
	}{
		{
			"ERC1967Proxy",
			"ERC1967Proxy",
		},
	}

	// TODO:Nexus list all contracts here Path - artifact.json name, Name outputNameArtifact
	nexusContracts := []struct {
		Path string
		Name string
	}{
		{
			"Claims.sol",
			"ClaimsTest",
		},
	}

	for _, v := range proxyContracts {
		artifactBytes, err := contracts.ReadRawArtifact(proxyscpath, "", getContractName(v.Path))
		if err != nil {
			log.Fatal(err)
		}

		dst := &bytes.Buffer{}
		if err = json.Compact(dst, artifactBytes); err != nil {
			log.Fatal(err)
		}

		str += fmt.Sprintf("var %sArtifact string = `%s`\n", v.Name, dst.String())
	}

	for _, v := range nexusContracts {
		artifactBytes, err := contracts.ReadRawArtifact(nexusscpath, v.Path, getContractName(v.Path))
		if err != nil {
			log.Fatal(err)
		}

		dst := &bytes.Buffer{}
		if err = json.Compact(dst, artifactBytes); err != nil {
			log.Fatal(err)
		}

		str += fmt.Sprintf("var %sArtifact string = `%s`\n", v.Name, dst.String())
	}

	output, err := format.Source([]byte(str))
	if err != nil {
		fmt.Println(str)
		log.Fatal(err)
	}

	if err = common.SaveFileSafe(path.Join(path.Dir(path.Clean(currentPath)),
		"nexus_sc_data.go"), output, 0600); err != nil {
		log.Fatal(err)
	}
}

// getContractName extracts smart contract name from provided path
func getContractName(path string) string {
	pathSegments := strings.Split(path, string([]rune{os.PathSeparator}))
	nameSegment := pathSegments[len(pathSegments)-1]

	return strings.Split(nameSegment, extension)[0]
}

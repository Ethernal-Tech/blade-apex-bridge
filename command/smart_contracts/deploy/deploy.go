package deploy

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/spf13/cobra"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
)

var (
	source               string
	rpcURL               string
	privateKey           string
	selected             []string
	branch               string
	compile              bool
	all                  bool
	verbose              bool
	proxyAdminPrivateKey string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy smart contracts and optionally upgrade OpenZeppelin proxies.",
		Long:  doc,
		RunE:  runCommand,
	}

	cmd.Flags().StringVarP(
		&source,
		"source",
		"s",
		"",
		"Source location for smart contracts (local JSON file, local hardhat project, or remote git repository).",
	)

	_ = cmd.MarkFlagRequired("source")

	cmd.Flags().StringVarP(
		&rpcURL,
		"rpc-url",
		"u",
		"",
		"RPC endpoint URL for blockchain network.",
	)

	_ = cmd.MarkFlagRequired("rpc-url")

	cmd.Flags().StringVarP(
		&privateKey,
		"private-key",
		"k",
		"",
		"Private key for deploying smart contracts (and optionally upgrading the proxy).",
	)

	_ = cmd.MarkFlagRequired("private-key")

	cmd.Flags().StringSliceVar(
		&selected,
		"select",
		nil,
		"Select specific smart contracts to deploy and optionally specify proxy upgrades.",
	)

	_ = cmd.MarkFlagRequired("branch")

	cmd.Flags().StringVarP(
		&branch,
		"branch",
		"b",
		"main",
		"Git branch to be cloned. Defaults to main.",
	)

	cmd.Flags().BoolVarP(
		&compile,
		"compile",
		"c",
		false,
		"If set, compiles Hardhat project. Defaults to false.",
	)

	cmd.Flags().BoolVarP(
		&all,
		"all",
		"a",
		false,
		"If set, all found smart contracts are deployed regardless of --select.",
	)

	cmd.Flags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"If set, git and npm/npx outputs are sent to standard output.",
	)

	cmd.Flags().StringVar(
		&proxyAdminPrivateKey,
		"admin-private-key",
		"",
		"Private key for upgrading proxy smart contracts.",
	)

	return cmd
}

func runCommand(cmd *cobra.Command, _ []string) error {
	fmt.Println("🚀 Starting deployment process...")

	if source == "" {
		return errors.New("missing required flag --source")
	}

	if rpcURL == "" {
		return errors.New("missing required flag --rpc-url")
	}

	if privateKey == "" {
		return errors.New("missing required flag --private-key")
	}

	isRepo := strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
	info, err := os.Stat(source)
	isLocal := err == nil

	if !isRepo && !isLocal {
		return fmt.Errorf("invalid --source: %s is neither a local path nor a valid URL", source)
	}

	if isRepo {
		tmpDir, err := os.MkdirTemp("", "remote_hardhat_repo_")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		if err := gitClone(tmpDir); err != nil {
			return fmt.Errorf("failed to clone remote hardhat repository: %w", err)
		}

		if !isHardhatProject(tmpDir) {
			return fmt.Errorf("not a valid hardhat project, missing hardhat.config.ts in %s", tmpDir)
		}

		if err := hardhatCompile(tmpDir); err != nil {
			return fmt.Errorf("failed to compile hardhat project: %w", err)
		}

		return deployFromHardhat(tmpDir)
	}

	if isLocal && strings.HasSuffix(source, ".json") {
		return deployFromJSON()
	}

	if isLocal && info.IsDir() {
		if !isHardhatProject(source) {
			return fmt.Errorf("not a valid hardhat project, missing hardhat.config.ts in %s", source)
		}

		if compile {
			if err := hardhatCompile(source); err != nil {
				return fmt.Errorf("failed to compile project: %w", err)
			}
		}

		return deployFromHardhat(source)
	}

	return fmt.Errorf("unsupported source format: %s", source)
}

func gitClone(dest string) error {
	fmt.Println("📦 Cloning repository...", source)
	cmd := exec.Command(
		"git",
		"clone",
		"--depth=1",
		"--branch",
		branch,
		source,
		dest)

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

func isHardhatProject(path string) bool {
	configPath := filepath.Join(path, "hardhat.config.ts")
	if _, err := os.Stat(configPath); err != nil {
		return false
	}

	return true
}

func hardhatCompile(dir string) error {
	fmt.Println("🧱 Compiling hardhat project (npm install && npx hardhat compile)...")

	cmds := [][]string{
		{"npm", "install"},
		{"npx", "hardhat", "compile"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) // #nosec
		cmd.Dir = dir

		if verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", strings.Join(args, " "), err)
		}
	}

	return nil
}

func deployFromJSON() error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	var contracts []struct {
		Name     string `json:"contractName"`
		Bytecode string `json:"bytecode"`
	}

	if err := json.Unmarshal(data, &contracts); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	if len(contracts) == 0 {
		return errors.New("no contracts found in JSON")
	}

	for _, contract := range contracts {
		if _, err := deploySmartContract(contract.Name, contract.Bytecode); err != nil {
			return fmt.Errorf("failed to deploy %s: %w", contract.Name, err)
		}
	}

	return nil
}

func deployFromHardhat(dir string) error {
	artifactsPath, err := resolveArtifactsPath(dir)
	if err != nil {
		return fmt.Errorf("failed to deploy: %w", err)
	}

	type pp struct {
		proxy string
		path  string
	}

	var contracts []pp

	if err := filepath.WalkDir(artifactsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() &&
			!strings.Contains(path, "build-info") &&
			!strings.HasSuffix(d.Name(), "dbg.json") {
			if len(selected) == 0 {
				contracts = append(contracts, pp{"", path})

				return nil
			}

			sel, proxy := isSelected(path, artifactsPath, selected)

			if all || sel {
				contracts = append(contracts, pp{proxy, path})
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to deploy: %w", err)
	}

	for _, contract := range contracts {
		data, err := os.ReadFile(contract.path)
		if err != nil {
			return err
		}

		var obj struct {
			ContractName string `json:"contractName"`
			Bytecode     string `json:"bytecode"`
		}

		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("failed to deploy: %w", err)
		}

		addr, err := deploySmartContract(obj.ContractName+".sol", obj.Bytecode)
		if err != nil {
			return fmt.Errorf("failed to deploy %s: %w", obj.ContractName, err)
		}

		if contract.proxy != "" {
			if err := upgradeContract(contract.proxy, addr); err != nil {
				return fmt.Errorf("failed to upgrade %s: %w", contract.proxy, err)
			}
		}
	}

	return nil
}

func isSelected(path string, artifactsPath string, selected []string) (bool, string) {
	rel, err := filepath.Rel(artifactsPath, path)

	if err != nil {
		return false, ""
	}

	rel = filepath.ToSlash(rel)
	proxy := ""

	for _, s := range selected {
		if strings.Contains(s, ":") {
			splited := strings.Split(s, ":")
			proxy = splited[0]
			s = splited[1]
		}

		s = strings.TrimPrefix(s, "./")
		s = strings.TrimSuffix(s, ".sol")
		s = filepath.ToSlash(s)

		contractName := filepath.Base(s)
		expected := s + ".sol/" + contractName + ".json"

		if rel == expected {
			return true, proxy
		}
	}

	return false, ""
}

func deploySmartContract(name, rawBytecode string) (string, error) {
	fmt.Printf("📤 Deploying %s...\n", name)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(rpcURL))
	if err != nil {
		return "", err
	}

	deployer, err := bridgeHelper.GetPrivateKeyForCommand(privateKey)
	if err != nil {
		return "", err
	}

	bytecode, err := hex.DecodeString(strings.TrimPrefix(rawBytecode, "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid bytecode: %w", err)
	}

	txn := types.NewTx(types.NewLegacyTx(types.WithTo(nil), types.WithInput(bytecode)))

	receipt, err := relayer.SendTransaction(txn, deployer)
	if err != nil {
		return "", err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return "", errors.New("deployment transaction failed")
	}

	fmt.Println("✅ Contract deployed at:", receipt.ContractAddress)

	return receipt.ContractAddress.String(), nil
}

var knownProxies = map[string]types.Address{"SM": contracts.StakeManagerContract}

func upgradeContract(proxyAddr, newImplAddr string) error {
	fmt.Printf("🔧 Upgrading %s proxy to %s...\n", proxyAddr, newImplAddr)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(rpcURL))
	if err != nil {
		return err
	}

	var deployer crypto.Key
	if proxyAdminPrivateKey != "" {
		deployer, err = bridgeHelper.DecodePrivateKey(strings.TrimPrefix(proxyAdminPrivateKey, "0x"))
	} else {
		deployer, err = bridgeHelper.GetPrivateKeyForCommand(privateKey)
	}

	if err != nil {
		return err
	}

	method := abi.MustNewMethod("function upgradeTo(address newImplementation)")

	newImpl := types.StringToAddress(newImplAddr)

	input, err := method.Encode([]interface{}{newImpl})
	if err != nil {
		return fmt.Errorf("failed to encode ABI input: %w", err)
	}

	var addr types.Address
	if a, ok := knownProxies[proxyAddr]; ok {
		addr = a
	} else {
		addr = types.StringToAddress(proxyAddr)
	}

	txn := types.NewTx(types.NewLegacyTx(
		types.WithTo(&addr),
		types.WithInput(input),
	))

	receipt, err := relayer.SendTransaction(txn, deployer)
	if err != nil {
		return err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return errors.New("upgrade transaction failed")
	}

	fmt.Printf("✅ Proxy %s upgraded to: %s\n", proxyAddr, newImplAddr)

	return nil
}

func resolveArtifactsPath(dir string) (string, error) {
	configPath := filepath.Join(dir, "hardhat.config.ts")
	data, err := os.ReadFile(configPath)

	if err != nil {
		return filepath.Join(dir, "artifacts/contracts"), nil
	}

	re := regexp.MustCompile(`artifacts:\s*["']([^"']+)["']`)
	m := re.FindSubmatch(data)

	if len(m) > 1 {
		return filepath.Join(dir, string(m[1])), nil
	}

	return filepath.Join(dir, "artifacts/contracts"), nil
}

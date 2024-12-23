# CLI configuration

Configuration parameters are crucial for setting up and operating a Blade-powered chain. You can configure these parameters using the server commands. Before running these commands, it is essential to generate keys using the blade secrets command.

For information on the available CLI commands and their configuration flags and descriptions refer to the sections below.\


## Backup

### Description

Create blockchain backup file by fetching blockchain data from the running node.\
Usage: `./blade backup [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade backup --out backup-file
```

\


## Genesis

### Description

Generates the genesis configuration file with the passed in parameters.\
Usage: `./blade genesis [flags]`\
Usage: `./blade genesis [command]`\
Available commands:\


* predeploy

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--validators`: Validators defined by the user (format: `<P2P multi address>:<public ECDSA address>:<public BLS key>`). If this flag is set, the entire multi address must be specified. If not set, validators configuration will be read from `--validators-path`.
* `--validators-path`: Root path containing polybft validators' secrets. If `--validators` flag is not specified, validators' configuration will be read from this path.
* `--validators-prefix`: Folder prefix names for polybft validators' secrets. If `--validators` flag is set, this prefix will be used for folder names.

</details>

### Example

```bash
./blade genesis --reward-wallet 0xDEADBEEF --premine 0x0000000000000000000000000000000000000000 --proxy-contracts-admin 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed --blade-admin 0x61324166B0202DB1E7502924326262274Fa4358F --validators /ip4/127.0.0.1/tcp/1478/p2p/16Uiu2HAmMYyzK7c649Tnn6XdqFLP7fpPB2QWdck1Ee9vj5a7Nhg8:0x61324166B0202DB1E7502924326262274Fa4358F:06d8d9e6af67c28e85ac400b72c2e635e83234f8a380865e050a206554049a222c4792120d84977a6ca669df56ff3a1cf1cfeccddb650e7aacff4ed6c1d4e37b055858209f80117b3c0a6e7a28e456d4caf2270f430f9df2ba37221f23e9bbd313c9ef488e1849cc5c40d18284d019dde5ed86770309b9c24b70ceff6167a6ca
```



## genesis predeploy

### Description

Specifies the contract to be predeployed on chain start.\
Usage: `./blade genesis predeploy [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--artifacts-name` and `--artifacts-path`: These flags are mutually >exclusive. Use either `--artifacts-name` for built-in contracts or `--artifacts-path` for externaly defined contracts.

</details>

### Example

```bash
./blade genesis predeploy --artifacts-name RootERC20 --deployer-address 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed
```



## Mint-erc20

### Description

Mints ERC20 tokens to specified addresses.\
Usage: `./blade mint-erc20 [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade mint-erc20 --addresses 0x85da99c8a7c2c95964c8efd687e95e632fc533d6 0x26F3f1f3F1d75c6d5d5146d1e44cec8831d0283A --amounts 1 2 --erc20-token 0x37e2e1f3F1d75c6d5d6336d1e44cec8831d0272a --private-key hex_encoded_private_key
```



## Monitor

### Description

Starts logging block add / remove events on the blockchain.\
Usage: `./blade monitor [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade monitor
```



## Peers

### Description

Top level command for interacting with the network peers. Only accepts subcommands.\
Usage: `./blade peers [command]`\
Available commands:\


* add
* list
* status



## peers add

### Description

Adds new peers to the peer list, using the peer's libp2p address.\
Usage: `./blade peers add [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade peers add --addr /ip4/192.168.200.201/tcp/1478/p2p/16Uiu2HAmGEMQmFqe2U4ag35BWiXniZ6orJVgaxdtSyFwXhFqT4Ko
```



## peers list

### Description

Returns the list of connected peers, including the local node.\
Usage: `./blade peers list [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade peers list
```



## peers status

### Description

Returns status of the specified peer, using the libp2p peer node ID.\
Usage: `./blade peers status [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade peers status --peer-id 16Uiu2HAmGEMQmFqe2U4ag35BWiXniZ6orJVgaxdtSyFwXhFqT4Ko
```



## Regenesis

### Description

Copies trie db for specific block to a separate folder.\
Usage: `./blade regenesis [flags]`\
Usage: `./blade regenesis [command]`\
Available commands:\


* getroot
* history

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade regenesis --source-path <dir containing old chain trie> --stateRoot <state root which will be copied into target trie> --target-path <directory containing new trie>
```



## regenesis getroot

### Description

Returns blockchain state root.\
Usage: `./blade regenesis getroot [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade regenesis getroot --rpc http://localhost:10002
```



## regenesis history

### Description

Run history test (compare chain and trie db state roots).\
Usage: `./blade regenesis history [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade regenesis history --chaindb <chaindb path> --triedb <triedb path>
```



## Secrets

### Description

Top level SecretsManager command for interacting with secrets functionality. Only accepts subcommands.\
Usage: `./blade secrets [command]`\
Available commands:\


* generate
* init
* output



## secrets generate

### Description

Initializes the secrets manager configuration in the provided directory.\
Usage: `./blade secrets generate [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade secrets generate --name blade-test --type alibaba-ssm --server-url oos.eu-central-1.aliyuncs.com --extra 'region=eu-central-1,ssm-parameter-path=/devnet'
```



## secrets init

### Description

Initializes private keys for Blade (Validator + Networking) to the specified Secrets Manager.\
Usage: `./blade secrets init [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.
* `--num` and `--config`: These flags are mutually exclusive. Set `--num` to define number of secrets to be created (only for local FS) or use `--config` to provide the SecretsManager config file path.

</details>

### Example

```bash
./blade secrets init --data-dir data --insecure
```



## secrets output

### Description

Outputs validator key address and public network key from the provided Secrets Manager.\
Usage: `./blade secrets output [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade secrets output --data-dir data
```



## Server

### Description

Default command starting the Blade client, by bootstrapping all modules together.\
Usage: `./blade server [flags]`\
Usage: `./blade server [command]`\
Available commands:\


* export

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade server --data-dir data
```



## server export

### Description

Export default-config.yaml file with default parameters that can be used to run the server.\
Usage: `./blade server export [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade server export
```



## Status

### Description

Returns status of the Blade client.\
Usage: `./blade status [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade status
```



## TxPool

### Description

Top level command for interacting with the transaction pool. Only accepts subcommands.\
Usage: `./blade txpool [command]`\
Available commands:\


* status
* subscribe



## txpool status

### Description

Returns the number of transactions in the transaction pool.\
Usage: `./blade txpool status [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade txpool status
```



## txpool subscribe

### Description

Logs specific TxPool events.\
Usage: `./blade txpool subscribe [flags]`

<details>

<summary>Flags ↓</summary>



</details>

### Example

```bash
./blade txpool subscribe --added --demoted  --dropped --enqueued --promoted --pruned-enqueued --pruned-promoted
```



## Validator

### Description

Validator command for interacting with validators. Only accepts subcommands.\
Usage: `./blade validator [command]`\
Available commands:\


* info
* register-validator
* stake
* unstake
* whitelist-validators
* withdraw
* withdraw-rewards



## validator info

### Description

Gets validator info.\
Usage: `./blade validator info [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator info --data-dir data
```



## validator register-validator

### Description

Registers a whitelisted validator to supernet manager on rootchain.\
Usage: `./blade validator register-validator [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator register-validator --data-dir data
```



## validator stake

### Description

Stakes the amount sent to validator.\
Usage: `./blade validator stake [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator stake --amount 10 --data-dir data
```



## validator unstake

### Description

Unstakes the amount sent for validator or undelegates amount from validator.\
Usage: `./blade validator unstake [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator unstake --amount 10 --data-dir data
```



## validator whitelist-validators

### Description

Whitelist new validators.\
Usage: `./blade validator whitelist-validators [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator whitelist-validators --addresses 0x85da99c8a7c2c95964c8efd687e95e632fc533d6 --data-dir data --private-key <private key>
```



## validator withdraw

### Description

Withdraws validator's withdrawable stake.\
Usage: `./blade validator withdraw [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator withdraw --data-dir data
```



## validator withdraw-rewards

### Description

Withdraws validator pending rewards on child chain.\
Usage: `./blade validator withdraw-rewards [flags]`

<details>

<summary>Flags ↓</summary>

**Info**\
Mutually Exclusive Parameters

* `--config` and `--data-dir`: These flags are mutually >exclusive. Use either `--config` to specify the path to the SecretsManager config file or `--data-dir` to set the directory for the Blade data if the local FS is used.

</details>

### Example

```bash
./blade validator withdraw-rewards --data-dir data
```



## Version

### Description

Returns current Blade version.\
Usage: `./blade version`

### Example

```bash
./blade version
```

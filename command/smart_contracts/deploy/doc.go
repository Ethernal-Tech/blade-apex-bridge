package deploy

var doc string = `This command is used to deploy smart contracts (and potentially upgrade OpenZeppelin proxies with
them). Several flags are required for the command to function correctly. The first is --source. It
defines the source (location) where smart contracts will be searched for deployment. If a local path
is specified - either relative or absolute - then smart contracts are searched locally. If the path
points to a .json file (for example: "../my/local/path.json"), it must be structured as follows:

[
    {
        "contractName": "<smart-contract-name>",
        "bytecode": "<smart-contract-bytecode>"  
    },
    {
        "contractName": "<smart-contract-name>",
        "bytecode": "<smart-contract-bytecode>"
    },
    ...
]

The bytecode may or may not be prefixed with "0x". All specified smart contracts will be deployed
(the --select flag has no effect in this case).

If the path points to a Hardhat project (for example: "../my/local/hardhat/project"; the path must
point to the ROOT of the Hardhat project - the directory containing the hardhat.config.ts configuration
file), then smart contracts are taken from the directory defined by the "paths.artifacts" field of the
HardhatUserConfig in the hardhat.config.ts config file. If not previously defined, smart contracts are
taken from the default Hardhat artifacts location ("artifacts/contracts"). Smart contracts to be deployed
can be selectively chosen using the --select flag (see the description of that flag for details). Otherwise,
all found smart contracts are deployed.

The --source flag also accepts a path to a remote git repository (for example: 
"https://github.com/my/remote/hardhat/project"). The specified repository must be a Hardhat project 
(with hardhat.config.ts in the root). Since the repository is cloned locally, the git CLI must be installed
in PATH. After cloning, "npm install && npx hardhat compile" is executed (this is done by default because
node_modules is usually ignored). Of course, for this to work, npm/npx must also be installed in PATH.
Since this now becomes a local Hardhat project, everything described in the previous paragraph applies
in this case as well. Note: SSH is currently not supported, only https (http).

The next required flag is --private-key. It defines the private key that will be used for deploying
smart contracts and potentially upgrading OpenZeppelin proxies if the --admin-private-key flag is
not specified. The private key may or may not be specified with a leading "0x". Alternatively, the flag
can be provided in the "<path-to-config-file>:<secrets-manager-key>" format to read the key from a secrets
manager.

The last required flag is --rpc-url. Example: "http://something:5757".

All flags described below are optional.

Filtering which smart contracts will be deployed is performed using the optional --select flag. Additionally,
this flag can be used to specify which OpenZeppelin proxy should be upgraded with which of the deployed
smart contracts. The --select flag only has an effect when the --source flag points to a Hardhat project,
not when deploying from local .json files (the first case described for the --source flag). There are three
formats:
    1. "<smart-contract-path>"
    2. "<proxy-address>-><smart-contract-path>"
    (Note: "-><smart-contract-path>" without a proxy address is equivalent to the first format.)
    3. "<proxy-address>:<function-signature>:<arg1>,<arg2>,...-><smart-contract-path>"

<smart-contract-path> represents the relative path to the desired smart contract from the root of the Hardhat
project (for example: "contracts/blade/staking/StakeManager.sol" or "./contracts/blade/staking/StakeManager.sol").
<proxy-address> represents the address of the OpenZeppelin Transparent Proxy that will be upgraded with the
smart contract on the right side of "->". The upgrade is performed by calling the "upgradeTo" function on the
proxy smart contract (note: newer versions - >= v5.0.0 - of OpenZeppelin for Transparent Proxies do not support
"upgradeTo"). Proxy addresses may or may not be prefixed with "0x". Also, instead of concrete addresses, aliases
can be used. Currently, only "SM" is available, which is equivalent to "0x10022". If you want to call a function
on the new implementation immediately after the upgrade, you can use the third format which also includes a
<function-signature> and the corresponding arguments (<arg1>,<arg2>,...). In this case, the upgradeToAndCall
function of the OpenZeppelin Transparent Proxy is called. The number of argument types specified in the signature
must match the number of provided arguments. Currently, array and tuple types in arguments are not supported.
Arguments are separated by commas. For example, if we have 16 smart contracts, the following command deploys only
the selected 3 while additionally upgrading the SM proxy smart contract with "contracts/dir2/Random2.sol" and
calling its "sync" method with the provided arguments (after the second ":")

blade sc deploy --source "<path-to-hardhat-project>" --private-key ... --rpc-url ... \
--select contracts/dir1/Random1.sol \
--select SM:function sync(uint,address[]): \
25,[0x0000000000000000000000000000000000000011,0x0000000000000000000000000000000000000011]->contracts/dir2/Random2.sol \
--select :contracts/Random3.sol

The next optional flag is --branch. It allows defining which branch will be cloned in the case of a remote
Hardhat repository. The default is "main". When dealing with a Hardhat project, there is sometimes a need to
compile before "searching" for smart contracts to deploy. For instance, in the third described case for the
--source flag. The flag essentially represents calling "npm install && npx hardhat compile". This flag only
has an effect when a path to a local Hardhat project is specified (the second case for the --source flag).

When cloning a remote hardhat repository, a temporary destination directory is used by default. Once the
command finishes, this directory is removed. If you wish to keep the cloned project (for example, to inspect
the compiled artifacts or to run additional commands), the --dir flag can be used to override this behavior
and specify a persistent destination directory. In this case, the directory will not be deleted after the
command completes.

The --all flag allows deploying all found smart contracts regardless of filtering with the --select flag.
This flag should be used when you want to deploy all smart contracts but additionally upgrade an OpenZeppelin
proxy. For example, if we have 16 smart contracts, the following command will deploy all 16 while additionally
upgrading the SM proxy smart contract to "contracts/dir2/Random2.sol":

blade sc deploy --source "<path-to-hardhat-project>" --private-key ... --rpc-url ... \
--select contracts/dir1/Random1.sol \
--select SM:contracts/dir2/Random2.sol \
--select :contracts/Random3.sol \
--all

With the --verbose flag, output for git and npm/npx commands can be enabled.

By default, the private key passed via the --private-key flag is used for upgrading OpenZeppelin proxies.
This can be overridden with the admin-private-key flag. The (admin) private key may or may not be specified with
a leading "0x". Alternatively, the flag can be provided in the "<path-to-config-file>:<secrets-manager-key>"
format to read the (admin) key from a secrets manager.

NOTE: Due to the nature of the "sc deploy" command, it is NOT transaction-like, meaning it is NOT "all or nothing".
If the command returns an exit code other than 0, undefined behavior is possible.`

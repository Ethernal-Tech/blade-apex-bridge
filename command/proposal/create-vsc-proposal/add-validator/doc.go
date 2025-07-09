package addvalidator

var doc string = `This command is used to create a validator set change proposal—or modify an existing one—by adding
a validator to the group of validators being proposed for inclusion in the validator set. The resulting
proposal is intended to be submitted for governance voting.

Several flags are required for the command to function correctly. The first mandatory flag is "file",
which specifies the proposal file. If the file already exists, it will be updated; otherwise, a new
file will be created. Only one file flag may be provided per command execution.

The next required flag is "address", which specifies the Ethereum-style address of the validator to
be added (e.g., 0x742D35CC6634C0532925A3B844BC454E4438F44E). Only one address flag may be provided
per command execution. 

Another required flag is "cardano-like-chain", which defines the validator's keys for a specific
Cardano-like chain. The expected format is:

chain_name:multisig_verification:fee_verification:multisig_stake_verification:fee_stake_verification

Supported chain names are cardano, prime, and vector. Each key must be a 256-bit value, represented
as a 64-character hex string (e.g., f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a).
The string must not start with the 0x prefix. This flag may be used multiple times to define keys for
multiple chains, but each chain may only be specified once.

Optionally, the "evm-like-chain" flag can be used to define keys for EVM-compatible chains. The format is:

chain_name:bls_key

Currently, only the nexus chain is supported. The BLS key must be provided as a hex string without the
0x prefix. As with Cardano-like chains, this flag can be used multiple times for different chains, but
each chain may only be specified once.

If the validator already exists in the proposal, its information will be updated with the newly provided values.
If the validator is currently listed in proposal as one to be removed from the validator set, it will be
removed from that removal list and added to the inclusion list instead.

All validators added to a proposal must define the exact same set of chains. For example, if the first
validator specifies keys for both prime and nexus, every subsequent validator must also define both prime
and nexus. A validator that defines only prime, or defines vector and nexus, will be rejected. This requirement
does not apply when adding the first validator, as there is no existing validator to compare against.`

package removevalidator

var doc string = `This command is used to create a validator set change proposal—or modify an existing one—by adding
a validator to the group of validators being proposed for removal from the validator set. The resulting
proposal is intended to be submitted for governance voting.

The command accepts two flags, both of which are required.

The "file" flag specifies the proposal file. If the file already exists, it will be updated; otherwise, a new
file will be created. Only one file flag may be provided per command execution.

The "address" flag specifies the Ethereum-style address of the validator to be removed
(e.g., 0x742D35CC6634C0532925A3B844BC454E4438F44E). Only one address flag may be provided per command execution.

If the validator already exists in the proposal, no changes will be made. If the validator is currently
listed in the proposal as one to be added to the validator set, it will be removed from that inclusion
list and added to the removal list instead.`

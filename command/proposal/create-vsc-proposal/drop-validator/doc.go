package dropvalidator

var doc string = `This command is used to modify a validator set change proposal by removing a validator from it.
The validator may be part of the group of validators proposed for addition to the validator set, or the group
proposed for removal. This command allows you to selectively remove the validator from one or both of those groups,
depending on the flags provided.

The command accepts two required flags, along with two optional ones for group selection.

The "file" flag specifies the proposal file. If the file already exists, it will be updated; otherwise, a new (empty)
one will be created. Only one file flag may be provided per command execution.

The address flag specifies the Ethereum-style address of the validator to be removed from the proposal
(e.g., 0x742D35CC6634C0532925A3B844BC454E4438F44E). Only one address flag may be provided per command execution.

The "added" flag can be used to restrict the operation to validators who are part of the group proposed for addition
to the validator set. If this flag is set, the validator will only be removed if it appears in that group.

Similarly, the "removed" flag restricts the operation to validators who are part of the group proposed for removal
from the validator set. If this flag is set, the validator will only be removed if it appears in that group.

If neither "added" nor "removed" flag is specified, the command will search both groups. If the validator is found
in either one, it will be removed from the proposal.

This command is used to correct mistakes in a proposal before it is finalized and submitted for governance voting.`

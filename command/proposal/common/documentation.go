package common

var SubmitRelatedFlagsDoc = `

If the --submit flag is set, the proposal will not only be created but also immediately submitted to
the governance system. Enabling this flag requires several additional flags to be provided.

The first one is --private-key, which specifies the private key used for signing the transaction. The
private key may or may not be specified with a leading "0x". Alternatively, the flag can be provided
in the "<path-to-config-file>:<secrets-manager-key>" format to read the key from a secrets manager.

The next one is --description, which provides a description of the proposal. Example: "some-description".

The last required flag is --rpc-url. Example: "http://something:5757".`

package execute

const (
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	proposalIDFlag     = "proposal-id"
)

type executeParams struct {
	privateKey     string
	jsonRPCAddress string
	proposalID     string
}

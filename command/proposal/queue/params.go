package queue

const (
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	proposalIDFlag     = "proposal-id"
)

type queueParams struct {
	privateKey     string
	jsonRPCAddress string
	proposalID     string
}

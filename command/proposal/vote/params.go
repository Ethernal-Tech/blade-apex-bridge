package vote

const (
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	proposalIDFlag     = "proposal-id"
	againstFlag        = "against"
)

type voteParams struct {
	privateKey     string
	jsonRPCAddress string
	proposalID     string
	against        bool
}

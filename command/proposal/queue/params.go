package queue

const (
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	inputFlag          = "input"
	descriptionFlag    = "description"
)

type queueParams struct {
	privateKey     string
	jsonRPCAddress string
	input          string
	description    string
}

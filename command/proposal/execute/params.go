package execute

const (
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	inputFlag          = "input"
	descriptionFlag    = "description"
)

type executeParams struct {
	privateKey     string
	jsonRPCAddress string
	input          string
	description    string
}

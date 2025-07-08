package submit

const (
	filePathFlag       = "path"
	privateKeyFlag     = "private-key"
	jsonRPCAddressFlag = "json-rpc"
	descriptionFlag    = "description"
)

type submitParams struct {
	filePath       string
	privateKey     string
	jsonRPCAddress string
	description    string
}

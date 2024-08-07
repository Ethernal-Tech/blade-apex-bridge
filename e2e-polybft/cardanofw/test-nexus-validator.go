package cardanofw

import (
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
)

type TestNexusValidator struct {
	ID     int
	Server *framework.TestServer
}

func NewTestNexusValidator(
	server *framework.TestServer,
	id int,
) *TestNexusValidator {
	return &TestNexusValidator{
		Server: server,
		ID:     id,
	}
}

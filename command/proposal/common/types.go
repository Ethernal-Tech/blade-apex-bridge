package common

type ChainID uint8

const (
	Prime   ChainID = 0x1
	Vector          = 0x2
	Nexus           = 0x3
	Cardano         = 0x4
	Blade           = 0xFF
)

var ChainIDMap = map[string]ChainID{
	"prime":   Prime,
	"vector":  Vector,
	"nexus":   Nexus,
	"cardano": Cardano,
	"blade":   Blade,
}

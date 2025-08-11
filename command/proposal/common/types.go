package common

type ChainID uint8

const (
	Prime ChainID = iota + 1
	Vector
	Nexus
	Cardano
	Blade ChainID = 0xFF
)

var ChainIDMap = map[string]ChainID{
	"prime":   Prime,
	"vector":  Vector,
	"nexus":   Nexus,
	"cardano": Cardano,
	"blade":   Blade,
}

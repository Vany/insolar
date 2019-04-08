package platformpolicy

import (
	"github.com/insolar/insolar/insolar"
	"github.com/insolar/insolar/platformpolicy/commoncrypto"
	"github.com/insolar/insolar/platformpolicy/customcrypto/secp256k1"
)

const (
	COMMON    AlgorithmType = "COMMON"
	SECP256K1 AlgorithmType = "SECP256K1"
)

type AlgorithmType string

//NewKeyProcessor(platformpolicy.COMMON)

func NewKeyProcessor(algorithmType AlgorithmType) insolar.KeyProcessor {
	switch algorithmType {
	case SECP256K1:
		return secp256k1.NewKeyProcessor()
	default:
		return commoncrypto.NewKeyProcessor()
	}
}

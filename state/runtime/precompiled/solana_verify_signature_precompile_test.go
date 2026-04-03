package precompiled

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo/abi"
	solanainfra "github.com/Ethernal-Tech/solana-infrastructure/wallet"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func Test_solanaVerifySignaturePrecompile_ValidSignature(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	pubKey, err := solanainfra.PublicKeyFromAddress("FcQWrEJbYj1b7JpNsdzdkrBpa76kabyJukQiXEZ4CDWj")
	require.NoError(t, err)
	sig, err := solana.SignatureFromBase58("3qesGADEM85rH2Y5yVF4RGJxQAVj2H2gZCgtr6cx1gN56tzkpZX3qxKXxqHo2D83WRbzShCNr7RgU7aqSFV5TUiT")
	require.NoError(t, err)
	message, err := hex.DecodeString("02000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002000408d915888ad147329e11e256a48bb4429b3542c912ff8494bf59d6c1589a108694895af4c86f9e9c985af84f878cb664edff97a526e47c56f377dca9a6c15da80ce92817cf5a18dcb1a1c032c3565a10f7bc0d151b646b47311ab7ae14f28393c609b34233d1046cf0114a0abf145321691d18f9bc065004f277bd676756611eba06ddf6e1d765a193d9cbe146ceeb79ac1cb485ed5f5b37913a8cf5857eff00a900000000000000000000000000000000000000000000000000000000000000008c97258f4e2489f1bb3d1029148e0d830b5a1399daff1084048e7bd8dbe9f859ae9259c9db1cce7cc1a666cdf749a70c0814c9cb914a2cad83f4010038960d5a433f81b5bde429e96aecad6e90aeb80e9f249d1b0d8fb643cf6294ab00eea94801070601020304050661491a777538a8d162010000007d5f64e3209be98183f4febbfdfee386f03c340a2f8a5156cbfe52a6fa455b0e0040420f000000000001000000069b8857feab8184fb687f634618c035dac439dc1aeb3b5598a0f000000000010100000000000000")
	require.NoError(t, err)

	require.NoError(t, err)

	value, err := prec.run(
		encodeSolanaVerifySignature(t, message, sig[:], pubKey),
		types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolTrue, value)
}

func Test_solanaVerifySignaturePrecompile_ValidSignatureMultipleMessages(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubKey := privKey.PublicKey()

	messages := [][]byte{
		[]byte("message one"),
		[]byte("message two"),
		[]byte("a]much*longer+message-with_special!chars@123"),
		{0x00, 0x01, 0x02, 0xFF},
	}

	for _, msg := range messages {
		sig, err := privKey.Sign(msg)
		require.NoError(t, err)

		value, err := prec.run(
			encodeSolanaVerifySignature(t, msg, sig[:], pubKey),
			types.ZeroAddress, nil)
		require.NoError(t, err)
		require.Equal(t, abiBoolTrue, value)
	}
}

func Test_solanaVerifySignaturePrecompile_InvalidSignature_WrongKey(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	otherPrivKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	message := []byte("hello solana bridge")

	sig, err := privKey.Sign(message)
	require.NoError(t, err)

	// verify with wrong public key
	value, err := prec.run(
		encodeSolanaVerifySignature(t, message, sig[:], otherPrivKey.PublicKey()),
		types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolFalse, value)
}

func Test_solanaVerifySignaturePrecompile_InvalidSignature_WrongMessage(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubKey := privKey.PublicKey()

	sig, err := privKey.Sign([]byte("original message"))
	require.NoError(t, err)

	// verify against a different message
	value, err := prec.run(
		encodeSolanaVerifySignature(t, []byte("tampered message"), sig[:], pubKey),
		types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolFalse, value)
}

func Test_solanaVerifySignaturePrecompile_InvalidSignature_CorruptedSig(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubKey := privKey.PublicKey()
	message := []byte("hello solana bridge")

	sig, err := privKey.Sign(message)
	require.NoError(t, err)

	corruptedSig := make([]byte, len(sig))
	copy(corruptedSig, sig[:])
	corruptedSig[0] ^= 0xFF

	value, err := prec.run(
		encodeSolanaVerifySignature(t, message, corruptedSig, pubKey),
		types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolFalse, value)
}

func Test_solanaVerifySignaturePrecompile_InvalidInputs(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	// garbage input
	_, err := prec.run(
		[]byte{1, 2, 3},
		types.ZeroAddress, nil)
	require.Error(t, err)

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubKey := privKey.PublicKey()
	message := []byte("hello solana bridge")

	// wrong signature length (63 bytes instead of 64)
	_, err = prec.run(
		encodeSolanaVerifySignature(t, message, make([]byte, 63), pubKey),
		types.ZeroAddress, nil)
	require.Error(t, err)

	// wrong signature length (65 bytes instead of 64)
	_, err = prec.run(
		encodeSolanaVerifySignature(t, message, make([]byte, 65), pubKey),
		types.ZeroAddress, nil)
	require.Error(t, err)

	// empty signature
	_, err = prec.run(
		encodeSolanaVerifySignature(t, message, []byte{}, pubKey),
		types.ZeroAddress, nil)
	require.Error(t, err)

	// truncated ABI input
	sig, err := privKey.Sign(message)
	require.NoError(t, err)

	fullInput := encodeSolanaVerifySignature(t, message, sig[:], pubKey)

	_, err = prec.run(fullInput[1:], types.ZeroAddress, nil)
	require.Error(t, err)
}

func encodeSolanaVerifySignature(t *testing.T,
	message []byte, signature []byte, verifyingKey solana.PublicKey,
) []byte {
	t.Helper()

	encoded, err := abi.Encode([]interface{}{
		message,
		signature,
		verifyingKey,
	},
		solanaVerifySignaturePrecompileInputABIType)
	require.NoError(t, err)

	return encoded
}

package precompiled

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func Test_solanaVerifySignaturePrecompile_ValidSignature(t *testing.T) {
	prec := &solanaVerifySignaturePrecompile{}

	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubKey := privKey.PublicKey()
	message := []byte("hello solana bridge")

	sig, err := privKey.Sign(message)
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

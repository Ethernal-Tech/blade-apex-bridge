package solanafw

import "os"

func tryResolveFromEnv(env, name string) string {
	if bin := os.Getenv(env); bin != "" {
		return bin
	}
	// fallback
	return name
}

func ResolveSolanaTestValidatorBinary() string {
	return tryResolveFromEnv("SOLANA_TEST_VALIDATOR_BINARY", "solana-test-validator")
}

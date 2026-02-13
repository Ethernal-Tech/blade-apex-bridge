package solanafw

import "os"

func tryResolveFromEnv(env, name string) string {
	if bin := os.Getenv(env); bin != "" {
		return bin
	}
	// fallback
	return name
}

func ResolveSurfPoolBinary() string {
	return tryResolveFromEnv("SURF_POOL_BINARY", "surfpool")
}

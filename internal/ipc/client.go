package ipc

func validateClientConfig(config ClientConfig) error {
	return validatePipeName(config.PipeName)
}

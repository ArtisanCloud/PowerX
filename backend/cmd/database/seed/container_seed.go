package seed

import "os"

func shouldSeedDevelopmentAPIKeys() bool {
	return os.Getenv("POWERX_MODE") != "docker"
}

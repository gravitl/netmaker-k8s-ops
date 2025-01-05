package conf

import "os"

func InClusterCfg() bool {
	return os.Getenv("IN_CLUSTER") == "true"
}

package workspace

import "os"

func mountRoot(root string) (bool, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	return isMountRoot(root, data), nil
}

package workspace

// Volume roots are detected by device number in validateRoot on macOS.
func mountRoot(_ string) (bool, error) { return false, nil }

//go:build !windows

package procswitch

func getForegroundProcessName() (string, error) {
	return "", nil
}

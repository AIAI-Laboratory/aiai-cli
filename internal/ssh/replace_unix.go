//go:build !windows

package ssh

import "os"

func replaceStorageFile(source, target string) error {
	return os.Rename(source, target)
}

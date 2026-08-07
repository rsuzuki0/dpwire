// Package atomicfile provides small, owner-private persistence primitives.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteNew writes a new file and refuses to replace an existing path.
func WriteNew(path string, data []byte, permission os.FileMode) (err error) {
	if permission.Perm()&0o077 != 0 {
		return errors.New("atomicfile: permission must be owner-only")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permission.Perm())
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if err := writeAll(file, data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

// Replace atomically replaces one file through a same-directory temporary.
func Replace(path string, data []byte, permission os.FileMode) (err error) {
	if permission.Perm()&0o077 != 0 {
		return errors.New("atomicfile: permission must be owner-only")
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".digitalpaper-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(permission.Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := writeAll(temporary, data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("atomicfile: replace: %w", err)
	}
	return nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return errors.New("atomicfile: zero-length write")
		}
		data = data[written:]
	}
	return nil
}

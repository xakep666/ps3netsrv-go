package fs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RelaxedSystemRoot just adds provided prefix to the file paths before making a system call.
type RelaxedSystemRoot struct {
	path string
}

func NewRelaxedSystemRoot(path string) *RelaxedSystemRoot {
	return &RelaxedSystemRoot{path: path}
}

func (r *RelaxedSystemRoot) realPath(name string) (path string, err error) {
	// taken from https://github.com/spf13/afero/blob/master/basepath.go

	if err := validateBasePathName(name); err != nil {
		return name, err
	}

	bpath := filepath.Clean(r.path)
	path = filepath.Clean(filepath.Join(bpath, name))
	if !strings.HasPrefix(path, bpath) {
		return name, os.ErrNotExist
	}

	return path, nil
}

func validateBasePathName(name string) error {
	if runtime.GOOS != "windows" {
		// Not much to do here;
		// the virtual file paths all look absolute on *nix.
		return nil
	}

	// On Windows a common mistake would be to provide an absolute OS path
	// We could strip out the base part, but that would not be very portable.
	if filepath.IsAbs(name) {
		return os.ErrNotExist
	}

	return nil
}

func (r *RelaxedSystemRoot) Open(path string) (*os.File, error) {
	realPath, err := r.realPath(path)
	if err != nil {
		return nil, err
	}

	return os.Open(realPath)
}

func (r *RelaxedSystemRoot) Create(path string) (*os.File, error) {
	realPath, err := r.realPath(path)
	if err != nil {
		return nil, err
	}

	return os.Create(realPath)
}

func (r *RelaxedSystemRoot) Stat(path string) (fs.FileInfo, error) {
	realPath, err := r.realPath(path)
	if err != nil {
		return nil, err
	}

	return os.Stat(realPath)
}

func (r *RelaxedSystemRoot) Remove(path string) error {
	realPath, err := r.realPath(path)
	if err != nil {
		return err
	}

	return os.Remove(realPath)
}

func (r *RelaxedSystemRoot) Mkdir(path string, mode os.FileMode) error {
	realPath, err := r.realPath(path)
	if err != nil {
		return err
	}

	return os.Mkdir(realPath, mode)
}

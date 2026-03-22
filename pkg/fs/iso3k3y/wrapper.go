package iso3k3y

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const isoExt = ".iso"

type keyedFile struct {
	handler.File
	key []byte
}

func (kf *keyedFile) EncryptionKey() []byte {
	return kf.key
}

type KeyExtractionFileWrapper struct{}

func (KeyExtractionFileWrapper) WrapFile(fsys pkgfs.SystemRoot, f handler.File) (handler.File, error) {
	// 3k3y makes sense only for iso images
	if !strings.EqualFold(filepath.Ext(f.Name()), isoExt) {
		return f, nil
	}

	// .. and only if it's a file
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		return f, nil
	}

	key, err := Test3k3yImage(f)
	switch {
	case errors.Is(err, nil):
		if len(key) == 0 {
			// unencrypted 3k3y
			slog.Debug("unencrypted 3k3y image detected", slog.String("name", f.Name()))
			return NewISO3k3y(f)
		}

		slog.Debug("encrypted 3k3y image with key detected", slog.String("name", f.Name()))
		return &keyedFile{
			File: f,
			key:  key,
		}, nil
	case errors.Is(err, ErrNot3k3y):
		return f, nil
	default:
		return nil, fmt.Errorf("test 3k3y: %w", err)
	}
}

func (KeyExtractionFileWrapper) Name() string {
	return "3k3y_key_extractor"
}

type FileWrapper struct{}

func (FileWrapper) WrapFile(fsys pkgfs.SystemRoot, f handler.File) (handler.File, error) {
	// if it's already a 3k3y iso, just pass
	if _, is3k3y := f.(*ISO3k3y); is3k3y {
		return f, nil
	}

	// 3k3y makes sense only for iso images
	ext := filepath.Ext(f.Name())
	if strings.ToLower(ext) != isoExt {
		return f, nil
	}

	// .. and only if it's a file
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		return f, nil
	}

	_, err = Test3k3yImage(f)
	switch {
	case errors.Is(err, nil):
		return NewISO3k3y(f)
	case errors.Is(err, ErrNot3k3y):
		return f, nil
	default:
		return nil, fmt.Errorf("test 3k3y: %w", err)
	}
}

func (FileWrapper) Name() string {
	return "3k3y_wrapper"
}

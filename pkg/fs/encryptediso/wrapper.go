package encryptediso

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type FileWrapper struct{}

type keyedFile interface {
	handler.File
	EncryptionKey() []byte
}

func (FileWrapper) WrapFile(ctx context.Context, fsys *pkgfs.FS, f handler.File) (handler.File, error) {
	if kf, ok := handler.FileAsType[keyedFile](f); ok {
		slog.Debug("received encrypted key-contained iso", slog.String("name", f.Name()))
		return NewEncryptedISO(f, kf.EncryptionKey(), false)
	}

	key, err := tryGetRedumpKey(fsys.SystemRoot(), f.Name())
	switch {
	case errors.Is(err, nil):
		slog.DebugContext(ctx, "found key file for encrypted iso", slog.String("name", f.Name()))
		return NewEncryptedISO(f, key, false)
	case errors.Is(err, fs.ErrNotExist):
		return f, nil
	default:
		return nil, fmt.Errorf("read key: %w", err)
	}
}

func (FileWrapper) Name() string {
	return "redump_encrypted_iso"
}

// tryGetRedumpKey attempts to find encryption key for .iso image.
func tryGetRedumpKey(fsys pkgfs.SystemRoot, requestedPath string) ([]byte, error) {
	// encryption makes sense only for .iso or .ISO file inside ps3ISO or PS3ISO directory
	ext := filepath.Ext(requestedPath)
	if strings.EqualFold(ext, isoExt) {
		return nil, fs.ErrNotExist
	}

	pathElems := strings.Split(requestedPath, string(filepath.Separator))
	ps3IsoIdx := slices.IndexFunc(pathElems, func(s string) bool {
		return strings.EqualFold(s, ps3isoDir)
	})
	if ps3IsoIdx < 0 {
		return nil, fs.ErrNotExist
	}

	// try .dkey file first
	keyFile, err := fsys.Open(strings.TrimSuffix(requestedPath, ext) + dkeyExt)
	if err == nil {
		defer keyFile.Close()
		return ReadKeyFile(keyFile)
	}

	// try .dkey in REDKEY directory (instead of PS3ISO)
	pathElems[ps3IsoIdx] = redkeyDir
	pathElems[len(pathElems)-1] = strings.TrimSuffix(pathElems[len(pathElems)-1], ext) + dkeyExt
	keyFile, err = fsys.Open(filepath.Join(pathElems...))
	if err == nil {
		defer keyFile.Close()
		return ReadKeyFile(keyFile)
	}

	return nil, err
}

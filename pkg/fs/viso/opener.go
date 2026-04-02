package viso

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type Opener struct{}

type fileType int

const (
	genericFile fileType = iota
	virtualISOFile
	virtualPS3ISOFile
)

const (
	virtualISOMask    = "***DVD***" + string(filepath.Separator)
	virtualPS3ISOMask = "***PS3***" + string(filepath.Separator)
)

func translatePath(path string) (string, fileType) {
	switch {
	case strings.HasPrefix(path, virtualISOMask):
		return strings.TrimPrefix(path, virtualISOMask), virtualISOFile
	case strings.HasPrefix(path, virtualPS3ISOMask):
		return strings.TrimPrefix(path, virtualPS3ISOMask), virtualPS3ISOFile
	default:
		return path, genericFile // avoid os path separator at the beginning
	}
}

func (Opener) Open(ctx context.Context, fsys *pkgfs.FS, path string) (handler.File, error) {
	path, typ := translatePath(path)
	if typ == genericFile {
		return nil, pkgfs.ErrTryNext
	}

	slog.InfoContext(ctx, "Engaging Virtual ISO", slog.String("path", path), slog.Bool("ps3_mode", typ == virtualPS3ISOFile))
	return NewVirtualISO(ctx, fsys, path, typ == virtualPS3ISOFile)
}

func (Opener) Stat(ctx context.Context, fsys *pkgfs.FS, path string) (fs.FileInfo, error) {
	// special handling doesn't necessary here
	return nil, pkgfs.ErrTryNext
}

func (Opener) Name() string {
	return "viso"
}

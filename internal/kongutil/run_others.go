//go:build !windows

package kongutil

import (
	"github.com/alecthomas/kong"
)

func Run(app *kong.Kong, run RunFn) {
	commonRun(app, run)
}

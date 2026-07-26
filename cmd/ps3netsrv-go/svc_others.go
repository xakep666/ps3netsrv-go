//go:build !windows

package main

type svcApp struct{}

func (*svcApp) Signature() string {
	return `cmd:"" hidden:""` // hide for non-windows platforms
}

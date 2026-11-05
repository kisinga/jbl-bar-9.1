//go:build windows || darwin

package usbsetup

import (
	"context"
	"runtime"
)

func formatPlatform(ctx context.Context, d Drive, p *Package, config string) error {
	return nativeFormat(ctx, d, p, config, runtime.GOOS, Run, Discover)
}

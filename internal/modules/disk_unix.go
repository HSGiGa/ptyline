//go:build linux || darwin

package modules

import (
	"context"
	"syscall"
)

type unixDiskProvider struct {
	cwd func() string
}

func newDiskProvider(cwd func() string) sampler[DiskSample] {
	return unixDiskProvider{cwd: cwd}
}

func (p unixDiskProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p unixDiskProvider) Sample(ctx context.Context) (DiskSample, error) {
	select {
	case <-ctx.Done():
		return DiskSample{}, ctx.Err()
	default:
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(diskPath(p.cwd), &stat); err != nil {
		return DiskSample{}, err
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bavail * blockSize
	return diskSample(total, free), nil
}

//go:build darwin

package modules

/*
#include <mach/mach_host.h>
#include <mach/mach_init.h>
#include <sys/sysctl.h>
#include <stdint.h>

// memTotal reads hw.memsize (total physical bytes) via sysctlbyname so the Go
// side does not have to assemble the MIB. Returns 0 on failure.
static uint64_t memTotal() {
	uint64_t total = 0;
	size_t len = sizeof(total);
	if (sysctlbyname("hw.memsize", &total, &len, NULL, 0) != 0) {
		return 0;
	}
	return total;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"unsafe"
)

// darwinMemoryProvider reports host memory using the mach VM statistics and
// hw.memsize, mirroring linuxMemoryProvider's role. The host port is fetched
// once and reused.
type darwinMemoryProvider struct {
	host C.host_t
}

func newMemoryProvider() sampler[MemorySample] {
	return darwinMemoryProvider{host: C.mach_host_self()}
}

func (p darwinMemoryProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p darwinMemoryProvider) Sample(ctx context.Context) (MemorySample, error) {
	select {
	case <-ctx.Done():
		return MemorySample{}, ctx.Err()
	default:
	}

	total := uint64(C.memTotal())
	if total == 0 {
		return MemorySample{}, errors.New("memory: hw.memsize unavailable")
	}

	var pageSize C.vm_size_t
	if ret := C.host_page_size(p.host, &pageSize); ret != C.KERN_SUCCESS {
		return MemorySample{}, fmt.Errorf("host_page_size: mach error %d", int(ret))
	}

	var vm C.vm_statistics64_data_t
	count := C.mach_msg_type_number_t(C.HOST_VM_INFO64_COUNT)
	ret := C.host_statistics64(
		p.host,
		C.HOST_VM_INFO64,
		C.host_info64_t(unsafe.Pointer(&vm)),
		&count,
	)
	if ret != C.KERN_SUCCESS {
		return MemorySample{}, fmt.Errorf("host_statistics64(HOST_VM_INFO64): mach error %d", int(ret))
	}

	ps := uint64(pageSize)
	// "Memory Used" as Activity Monitor reports it: resident app/wired memory plus
	// the compressor's footprint. Free/inactive/purgeable pages are reclaimable, so
	// they count as available, not used.
	used := (uint64(vm.active_count) + uint64(vm.wire_count) + uint64(vm.compressor_page_count)) * ps
	if used > total {
		used = total
	}
	available := total - used
	return MemorySample{
		Total:     total,
		Available: available,
		Used:      used,
		Percent:   100 * float64(used) / float64(total),
	}, nil
}

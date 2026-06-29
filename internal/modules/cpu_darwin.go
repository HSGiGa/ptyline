//go:build darwin

package modules

/*
#include <mach/mach_host.h>
#include <mach/mach_init.h>
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

// newCPUProvider wires the shared delta-based cpuProvider to a mach tick source.
// The host port is fetched once and reused; mach_host_self returns the (cached)
// host name port, so re-fetching it per sample would needlessly churn port refs.
func newCPUProvider() sampler[CPUSample] {
	host := C.mach_host_self()
	return &cpuProvider{read: readMachCPU(host)}
}

// readMachCPU returns a cpuTimes reader backed by host_statistics
// (HOST_CPU_LOAD_INFO), which reports cumulative CPU ticks since boot — the same
// monotonic counters the Linux path derives from /proc/stat, so the shared
// cpuProvider delta logic applies unchanged.
func readMachCPU(host C.host_t) func(context.Context) (cpuTimes, error) {
	return func(ctx context.Context) (cpuTimes, error) {
		select {
		case <-ctx.Done():
			return cpuTimes{}, ctx.Err()
		default:
		}

		var info C.host_cpu_load_info_data_t
		count := C.mach_msg_type_number_t(C.HOST_CPU_LOAD_INFO_COUNT)
		ret := C.host_statistics(
			host,
			C.HOST_CPU_LOAD_INFO,
			C.host_info_t(unsafe.Pointer(&info)),
			&count,
		)
		if ret != C.KERN_SUCCESS {
			return cpuTimes{}, fmt.Errorf("host_statistics(HOST_CPU_LOAD_INFO): mach error %d", int(ret))
		}

		user := uint64(info.cpu_ticks[C.CPU_STATE_USER])
		system := uint64(info.cpu_ticks[C.CPU_STATE_SYSTEM])
		idle := uint64(info.cpu_ticks[C.CPU_STATE_IDLE])
		nice := uint64(info.cpu_ticks[C.CPU_STATE_NICE])
		return cpuTimes{Idle: idle, Total: user + system + idle + nice}, nil
	}
}

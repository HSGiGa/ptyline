package modules

import "testing"

func TestDiskSample(t *testing.T) {
	got := diskSample(100, 25)
	if got.Total != 100 || got.Free != 25 || got.Used != 75 || got.Percent != 75 {
		t.Fatalf("diskSample() = %+v", got)
	}
}

func TestDiskSampleClampsFree(t *testing.T) {
	got := diskSample(100, 120)
	if got.Free != 100 || got.Used != 0 || got.Percent != 0 {
		t.Fatalf("diskSample() = %+v", got)
	}
}

func TestFormatDisk(t *testing.T) {
	gb := uint64(1024 * 1024 * 1024)
	sample := DiskSample{Total: 10 * gb, Free: 3 * gb, Used: 7 * gb, Percent: 70}
	got := formatDisk(sample, "disk {percent}% {free_gb}/{total_gb}G")
	want := "disk 70% 3/10G"
	if got != want {
		t.Fatalf("formatDisk() = %q, want %q", got, want)
	}
}

func TestDiskPath(t *testing.T) {
	if got := diskPath(func() string { return " /tmp " }); got != "/tmp" {
		t.Fatalf("diskPath() = %q, want /tmp", got)
	}
	if got := diskPath(func() string { return "" }); got != "/" {
		t.Fatalf("diskPath() = %q, want /", got)
	}
	if got := diskPath(nil); got != "/" {
		t.Fatalf("diskPath(nil) = %q, want /", got)
	}
}

//go:build !darwin

package util

func getProcessMemoryBytes(pid int) (uint64, error) {
	// Feature change: only macOS has the Activity Monitor footprint metric that
	// motivated this Glance item. Other platforms keep using the existing RSS or
	// working-set approximation so the dev diagnostic stays portable.
	return getProcessRSSBytes(pid)
}

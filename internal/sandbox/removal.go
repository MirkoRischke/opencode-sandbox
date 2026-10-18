package sandbox

// RemovalObserver is called after each successful deletion. SizeBytes is the
// runtime-reported snapshot size, not reclaimed space; it is nil for VMs or
// snapshots with unknown size. Observers must not initiate snapshot operations
// because snapshot deletion holds the repository lock.
type RemovalObserver func(name string, sizeBytes *uint64)

func notifyRemoval(observers []RemovalObserver, name string, sizeBytes *uint64) {
	for _, observer := range observers {
		observer(name, sizeBytes)
	}
}

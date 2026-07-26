package indexer

// Bundled SCIDs are no longer used. App discovery now relies on the
// on-disk app-cache.json (written after each successful FastSync) and
// the HyperGnomon indexer's live class-bucket scan.
// Keeping this exported but empty avoids import-side effects in callers
// that still reference it (e.g. `len(BundledTelaSCIDs)` in sync.go).
var BundledTelaSCIDs = []string{}

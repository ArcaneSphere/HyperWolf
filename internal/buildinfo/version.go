// Package buildinfo holds the single source of truth for the
// HyperWolf application version. Both the router (update check,
// config endpoint) and main reference this to avoid drift.
package buildinfo

const Version = "0.12.0"
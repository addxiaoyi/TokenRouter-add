package plugin

import "context"

// data.go defines the persistent storage contract the manager offers plugin
// processes at start time. Each installation owns one directory that survives
// package replacement and ordinary uninstallation, so business files stay put
// while executables and disposable process state rotate around them.

// DataStorage manages plugin-owned persistent directories across package
// versions and host restarts.
type DataStorage interface {
	// Directory returns the stable directory for id, creating it before
	// returning when the plugin has never run. The returned path is absolute,
	// belongs only to that plugin, and stays valid across restarts until
	// Remove deletes it.
	Directory(context.Context, string) (string, error)

	// Remove deletes one owner's directory after callers drain all plugin
	// work. A missing directory succeeds, so cleanup interrupted by a crash
	// can simply run again.
	Remove(context.Context, string) error
}

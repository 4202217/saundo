//go:build !windows

package discovery

// neighborHosts has no neighbor table to read off Windows.
func neighborHosts() ([]string, error) { return nil, nil }

package util

import "strings"

// HostIDFromIP converts an IP address into a safe host_id used as the StateFun object ID.
// Example: 192.168.157.11 -> 192_168_157_11
func HostIDFromIP(ip string) string {
	return strings.ReplaceAll(strings.TrimSpace(ip), ".", "_")
}

// IPFromHostID converts a host_id back into an IP address.
// Example: 192_168_157_11 -> 192.168.157.11
func IPFromHostID(hostID string) string {
	return strings.ReplaceAll(strings.TrimSpace(hostID), "_", ".")
}

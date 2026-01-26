package util

import "strings"

func GetSafeName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), ".", "_")
}

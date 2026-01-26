package util

import "strings"

func GetSafeName(name string) string {
	if strings.HasPrefix(name, prefix) {
		name = demoIp
	}
	return strings.ReplaceAll(strings.TrimSpace(name), ".", "_")
}

var VmDemoSlice = []string{
	"frisky-dragon",
	"shy-lynx",
	"frozen-leopard",
	"polar-bear",
	"sticky-snake-1",
	"sticky-snake-2",
	"sticky-snake-3",
	"sticky-snake-4",
}

const demoIp = "2.54.24.203"
const prefix = "176.114"

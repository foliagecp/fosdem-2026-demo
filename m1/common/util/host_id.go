package util

import "strings"

func GetSafeName(name string) string {
	if strings.HasPrefix(name, Prefix) {
		name = DemoIp
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

const DemoIp = "2.54.24.203"
const Prefix = "176.114"

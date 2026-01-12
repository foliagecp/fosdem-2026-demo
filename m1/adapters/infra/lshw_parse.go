package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
)

// walkLshw runs fn for each node in the lshw JSON tree.
func walkLshw(node easyjson.JSON, fn func(easyjson.JSON)) {
	if node.IsArray() {
		if arr, ok := node.AsArray(); ok {
			for _, it := range arr {
				walkLshw(easyjson.NewJSON(it), fn)
			}
		}
		return
	}
	if !node.IsObject() {
		return
	}

	fn(node)

	if arr, ok := node.GetByPath("children").AsArray(); ok {
		for _, it := range arr {
			walkLshw(easyjson.NewJSON(it), fn)
		}
	}
}

func findFirstNode(root easyjson.JSON, pred func(easyjson.JSON) bool) easyjson.JSON {
	found := easyjson.NewJSON(nil)
	walkLshw(root, func(n easyjson.JSON) {
		if found.IsObject() {
			return
		}
		if pred(n) {
			found = n
		}
	})
	return found
}

func findAllNodes(root easyjson.JSON, pred func(easyjson.JSON) bool) []easyjson.JSON {
	res := []easyjson.JSON{}
	walkLshw(root, func(n easyjson.JSON) {
		if pred(n) {
			res = append(res, n)
		}
	})
	return res
}

// firstStringByKey finds the first occurrence of a given key on any node.
func firstStringByKey(root easyjson.JSON, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	found := ""
	walkLshw(root, func(n easyjson.JSON) {
		if found != "" {
			return
		}
		v := strings.TrimSpace(n.GetByPath(key).AsStringDefault(""))
		if v != "" {
			found = v
		}
	})
	// Also check root itself.
	if found == "" {
		found = strings.TrimSpace(root.GetByPath(key).AsStringDefault(""))
	}
	return found
}

func firstIP(root easyjson.JSON) string {
	found := ""
	walkLshw(root, func(n easyjson.JSON) {
		if found != "" {
			return
		}
		ip := strings.TrimSpace(n.GetByPath("configuration.ip").AsStringDefault(""))
		if ip != "" {
			// Sometimes it's a space separated list.
			fields := strings.Fields(ip)
			if len(fields) > 0 {
				found = fields[0]
			}
		}
	})
	return found
}

func cpuHasVirt(cpu easyjson.JSON) (vmx bool, svm bool) {
	// lshw can provide capabilities as an object or array; we normalize to strings.
	if cpu.IsObject() {
		if obj, ok := cpu.GetByPath("capabilities").AsObject(); ok {
			for k := range obj {
				kk := strings.ToLower(strings.TrimSpace(k))
				if kk == "vmx" {
					vmx = true
				}
				if kk == "svm" {
					svm = true
				}
			}
		}
		if arr, ok := cpu.GetByPath("capabilities").AsArray(); ok {
			for _, it := range arr {
				s := strings.ToLower(strings.TrimSpace(easyjson.NewJSON(it).AsStringDefault("")))
				if s == "vmx" {
					vmx = true
				}
				if s == "svm" {
					svm = true
				}
			}
		}
	}
	return
}

package main

import (
	"fmt"
	"strings"
)

func getHypervisorUUID(id string) string {
	return fmt.Sprintf("%s__kvm", strings.TrimSpace(id))
}

func getVirtualMachineUUID(id string) string {
	return fmt.Sprintf("%s__vm", strings.TrimSpace(id))
}

func getSerialNumberUUID(id string) string {
	return fmt.Sprintf("%s__sn", strings.TrimSpace(id))
}

func getCpuUUID(id string) string {
	return fmt.Sprintf("%s__cpu", strings.TrimSpace(id))
}

func getBiosUUID(id string) string {
	return fmt.Sprintf("%s__bios", strings.TrimSpace(id))
}

func getDiskUUID(id string, idx int) string {
	return fmt.Sprintf("%s__disk_%d", strings.TrimSpace(id), idx)
}

func getRamUUID(id string, idx int) string {
	return fmt.Sprintf("%s__ram_%d", strings.TrimSpace(id), idx)
}

func getNicUUID(id string, idx int) string {
	return fmt.Sprintf("%s__nic_%d", strings.TrimSpace(id), idx)
}

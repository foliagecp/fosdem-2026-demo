package common

const (
	ModelM1 = "m1"
	ModelM2 = "m2"
	ModelM3 = "m3"
	ModelM4 = "m4"
)

const (
	M1RootObject = "infra"
	M2RootObject = "k8s_infrastructure"
	M3RootObject = "arch_model"
	M4RootObject = "datacenter"
)

const AllObjectsQuery = ".*[l:type('__object')]"

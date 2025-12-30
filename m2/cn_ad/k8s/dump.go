package main

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"
)

func NewFakeK8sClientFromDump(dumpFile string) (*fake.Clientset, error) {
	data, err := os.ReadFile(dumpFile)
	if err != nil {
		return nil, err
	}

	var dumpMap map[string]interface{}
	if err := yaml.Unmarshal(data, &dumpMap); err != nil {
		return nil, err
	}

	items, ok := dumpMap["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("dump file must be a List with 'items' field")
	}

	var objs []runtime.Object
	decoder := scheme.Codecs.UniversalDeserializer()

	for i, item := range items {
		itemBytes, _ := yaml.Marshal(item)

		obj, _, err := decoder.Decode(itemBytes, nil, nil)
		if err != nil {
			fmt.Printf("Warning: skipping item %d due to error: %v\n", i, err)
			continue
		}

		if _, ok := obj.(runtime.Object); ok {
			objs = append(objs, obj)
		}
	}

	if len(objs) == 0 {
		return nil, fmt.Errorf("no valid kubernetes objects found in dump")
	}

	return fake.NewSimpleClientset(objs...), nil
}

//go:build headless || (!headless && novulkan)

package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestVoodooWrapper_MethodSetParity(t *testing.T) {
	var _ VoodooBackend = (*VulkanBackend)(nil)
	var _ VoodooBackend = (*softwareVoodooBackend)(nil)

	iface := reflect.TypeOf((*VoodooBackend)(nil)).Elem()
	vulkan := reflect.TypeOf((*VulkanBackend)(nil))
	software := reflect.TypeOf((*softwareVoodooBackend)(nil))
	for i := 0; i < iface.NumMethod(); i++ {
		method := iface.Method(i).Name
		if _, ok := vulkan.MethodByName(method); !ok {
			t.Fatalf("VulkanBackend missing VoodooBackend method %s", method)
		}
		if _, ok := software.MethodByName(method); !ok {
			t.Fatalf("softwareVoodooBackend missing VoodooBackend method %s", method)
		}
	}
}

func TestVoodooWrapper_TaggedBackendsKeepOnlyConstructorSelection(t *testing.T) {
	for _, path := range []string{"voodoo_novulkan.go", "voodoo_vulkan_headless.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		if strings.Contains(text, "func (vb *VulkanBackend)") {
			t.Fatalf("%s still contains VulkanBackend forwarding methods; keep forwarding in voodoo_software_wrapper.go", path)
		}
		if !strings.Contains(text, "newSoftwareVoodooBackend()") {
			t.Fatalf("%s does not construct through the shared software wrapper", path)
		}
	}
}

//go:build !headless && novulkan

// voodoo_novulkan.go - Software-only Voodoo backend (no Vulkan dependency)

package main

func init() {
	compiledFeatures = append(compiledFeatures, "voodoo:software")
}

// VulkanBackend wraps VoodooSoftwareBackend when Vulkan is disabled.
// Uses the same type name so the rest of the codebase compiles unchanged.
type VulkanBackend struct {
	softwareVoodooBackend
}

func NewVulkanBackend() (*VulkanBackend, error) {
	return &VulkanBackend{
		softwareVoodooBackend: newSoftwareVoodooBackend(),
	}, nil
}

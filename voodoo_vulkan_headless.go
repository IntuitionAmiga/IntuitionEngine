//go:build headless

package main

func init() {
	compiledFeatures = append(compiledFeatures, "voodoo:headless")
}

// VulkanBackend wraps VoodooSoftwareBackend in headless builds.
// Uses the same type name so the rest of the codebase compiles unchanged.
type VulkanBackend struct {
	softwareVoodooBackend
}

func NewVulkanBackend() (*VulkanBackend, error) {
	return &VulkanBackend{
		softwareVoodooBackend: newSoftwareVoodooBackend(),
	}, nil
}

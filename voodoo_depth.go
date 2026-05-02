package main

func normalizeVoodooDepthForVulkan(z float32, fbzMode uint32) float32 {
	if fbzMode&VOODOO_FBZ_WBUFFER != 0 {
		return clampf(z, 0, 1)
	}
	if z > 1 {
		return clampf(z/65536.0, 0, 1)
	}
	return clampf(z, 0, 1)
}

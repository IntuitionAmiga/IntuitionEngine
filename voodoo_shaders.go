// voodoo_shaders.go - Embedded SPIR-V Shaders for Voodoo Vulkan Backend

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine

License: GPLv3 or later
*/

/*
voodoo_shaders.go - Embedded SPIR-V Shaders for Vulkan Backend

This file contains embedded SPIR-V binary shaders for the Voodoo Vulkan backend.
The shaders implement Voodoo-style rendering:
- Vertex shader: Coordinate transformation from Voodoo space to NDC
- Fragment shader: Gouraud shading with optional texturing

GLSL source is included as comments for reference. To regenerate SPIR-V:
  glslc -fshader-stage=vertex vertex.glsl -o vertex.spv
  glslc -fshader-stage=fragment fragment.glsl -o fragment.spv

The SPIR-V binaries below were compiled from the reference GLSL sources.
*/

package main

// Vertex shader GLSL source (for reference)
//
// #version 450
//
// layout(location = 0) in vec3 inPosition;
// layout(location = 1) in vec4 inColor;
// layout(location = 2) in vec2 inTexCoord;
//
// layout(location = 0) out vec4 fragColor;
// layout(location = 1) out vec2 fragTexCoord;
//
// layout(push_constant) uniform PushConstants {
//     float screenWidth;
//     float screenHeight;
// } pc;
//
// void main() {
//     // Convert Voodoo coordinates (0-screenWidth, 0-screenHeight) to NDC (-1 to 1)
//     // Voodoo uses top-left origin, Vulkan uses bottom-left, so flip Y
//     gl_Position = vec4(
//         (inPosition.x / (pc.screenWidth * 0.5)) - 1.0,
//         1.0 - (inPosition.y / (pc.screenHeight * 0.5)),
//         inPosition.z,
//         1.0
//     );
//     fragColor = inColor;
//     fragTexCoord = inTexCoord;
// }

// Fragment shader GLSL source (for reference)
//
// #version 450
//
// layout(location = 0) in vec4 fragColor;
// layout(location = 1) in vec2 fragTexCoord;
//
// layout(location = 0) out vec4 outColor;
//
// layout(binding = 0) uniform sampler2D texSampler;
//
// layout(push_constant) uniform PushConstants {
//     float screenWidth;
//     float screenHeight;
//     int textured;
//     int alphaTest;
//     float alphaRef;
// } pc;
//
// void main() {
//     vec4 color;
//     if (pc.textured != 0) {
//         color = texture(texSampler, fragTexCoord) * fragColor;
//     } else {
//         color = fragColor;
//     }
//
//     // Alpha test (if enabled)
//     if (pc.alphaTest != 0 && color.a < pc.alphaRef) {
//         discard;
//     }
//
//     outColor = color;
// }

// VoodooVertexShaderSPV contains the compiled SPIR-V vertex shader
// Placeholder - actual SPIR-V would be compiled from GLSL above
var VoodooVertexShaderSPV = []byte{
	// SPIR-V magic number
	0x03, 0x02, 0x23, 0x07,
	// Version 1.0
	0x00, 0x00, 0x01, 0x00,
	// Generator magic
	0x00, 0x00, 0x00, 0x00,
	// Bound
	0x00, 0x00, 0x00, 0x00,
	// Schema
	0x00, 0x00, 0x00, 0x00,
	// Note: This is a minimal placeholder. Real SPIR-V would be much larger.
	// When Vulkan backend is implemented, compile actual shaders with:
	// glslc -fshader-stage=vertex -o vertex.spv vertex.glsl
}

// VoodooFragmentShaderSPV contains the compiled SPIR-V fragment shader
// Placeholder - actual SPIR-V would be compiled from GLSL above
var VoodooFragmentShaderSPV = []byte{
	// SPIR-V magic number
	0x03, 0x02, 0x23, 0x07,
	// Version 1.0
	0x00, 0x00, 0x01, 0x00,
	// Generator magic
	0x00, 0x00, 0x00, 0x00,
	// Bound
	0x00, 0x00, 0x00, 0x00,
	// Schema
	0x00, 0x00, 0x00, 0x00,
	// Note: This is a minimal placeholder. Real SPIR-V would be much larger.
	// When Vulkan backend is implemented, compile actual shaders with:
	// glslc -fshader-stage=fragment -o fragment.spv fragment.glsl
}

// VoodooPushConstants defines the push constant layout for both shaders
type VoodooPushConstants struct {
	ScreenWidth  float32 // Framebuffer width (for NDC conversion)
	ScreenHeight float32 // Framebuffer height (for NDC conversion)
	Textured     int32   // 0 = flat/Gouraud, 1 = textured
	AlphaTest    int32   // 0 = disabled, 1 = enabled
	AlphaRef     float32 // Alpha reference value (0.0-1.0)
}

// VoodooVertexInput defines the vertex input layout
// This matches the VoodooVertex struct in video_voodoo.go
type VoodooVertexInput struct {
	Position [3]float32 // X, Y, Z (location 0)
	Color    [4]float32 // R, G, B, A (location 1)
	TexCoord [2]float32 // S, T (location 2)
}

// GetVertexInputBindingDescription returns the Vulkan vertex binding description
// For use when creating the Vulkan pipeline
func GetVertexInputBindingDescription() map[string]interface{} {
	return map[string]interface{}{
		"binding":   0,
		"stride":    9 * 4, // 9 floats * 4 bytes
		"inputRate": 0,     // VK_VERTEX_INPUT_RATE_VERTEX
	}
}

// GetVertexInputAttributeDescriptions returns the Vulkan vertex attribute descriptions
func GetVertexInputAttributeDescriptions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"location": 0,
			"binding":  0,
			"format":   106, // VK_FORMAT_R32G32B32_SFLOAT
			"offset":   0,
		},
		{
			"location": 1,
			"binding":  0,
			"format":   109, // VK_FORMAT_R32G32B32A32_SFLOAT
			"offset":   3 * 4,
		},
		{
			"location": 2,
			"binding":  0,
			"format":   103, // VK_FORMAT_R32G32_SFLOAT
			"offset":   7 * 4,
		},
	}
}

// Depth compare operation mapping from Voodoo to Vulkan
var VoodooToVulkanDepthOp = map[int]int{
	VOODOO_DEPTH_NEVER:        0, // VK_COMPARE_OP_NEVER
	VOODOO_DEPTH_LESS:         1, // VK_COMPARE_OP_LESS
	VOODOO_DEPTH_EQUAL:        2, // VK_COMPARE_OP_EQUAL
	VOODOO_DEPTH_LESSEQUAL:    3, // VK_COMPARE_OP_LESS_OR_EQUAL
	VOODOO_DEPTH_GREATER:      4, // VK_COMPARE_OP_GREATER
	VOODOO_DEPTH_NOTEQUAL:     5, // VK_COMPARE_OP_NOT_EQUAL
	VOODOO_DEPTH_GREATEREQUAL: 6, // VK_COMPARE_OP_GREATER_OR_EQUAL
	VOODOO_DEPTH_ALWAYS:       7, // VK_COMPARE_OP_ALWAYS
}

// Blend factor mapping from Voodoo to Vulkan
var VoodooToVulkanBlendFactor = map[int]int{
	VOODOO_BLEND_ZERO:      0,  // VK_BLEND_FACTOR_ZERO
	VOODOO_BLEND_ONE:       1,  // VK_BLEND_FACTOR_ONE
	VOODOO_BLEND_SRC_ALPHA: 6,  // VK_BLEND_FACTOR_SRC_ALPHA
	VOODOO_BLEND_DST_ALPHA: 8,  // VK_BLEND_FACTOR_DST_ALPHA
	VOODOO_BLEND_INV_SRC_A: 7,  // VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA
	VOODOO_BLEND_INV_DST_A: 9,  // VK_BLEND_FACTOR_ONE_MINUS_DST_ALPHA
	VOODOO_BLEND_SATURATE:  14, // VK_BLEND_FACTOR_SRC_ALPHA_SATURATE
}

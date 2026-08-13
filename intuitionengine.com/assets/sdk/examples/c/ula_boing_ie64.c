/*
 * ULA BOING BALL - Real-Time IE64 C and ZX Spectrum ULA Demonstration
 * ============================================================================
 *
 * SDK QUICK REFERENCE
 *
 * Target CPU:     IE64, compiled from freestanding C23 by ie64-cproc
 * Video system:  ZX Spectrum ULA, 256 by 192 pixels with 8 by 8 attributes
 * Sound effects: SoundChip FLEX channels 0 and 1 with global reverb
 * Music:         Embedded Standard MIDI File played by the MIDI player
 * Build:         make ula-boing-ie64
 * Live image:    Included in the automated Intuition Engine product demonstration
 *
 * WHAT THIS DEMO DOES
 *
 * A red-and-white chequered ball moves across a perspective floor, rotates
 * around two axes and compresses briefly when it strikes the ground. The IE64
 * programme generates every ULA bitmap and attribute byte. The ULA remains the
 * only video device: 6,144 bitmap bytes and 768 attribute bytes form each frame.
 *
 * The workload is deliberately unlike software written for an original
 * Spectrum. Compiler-generated IE64 floating-point code evaluates the projected
 * sphere, rotates its surface coordinates and classifies the result into ULA
 * cells. The IE64 CPU runs that compiled workload through its native JIT. The
 * programme constructs the ball, floor and shadow in ordinary RAM, then
 * presents the completed frame through ULA hardware.
 *
 * ULA COLOUR RESTRICTION
 *
 * One ULA attribute byte controls an 8 by 8 pixel cell. A cell therefore has
 * one PAPER colour and one INK colour, not a colour per pixel. The renderer
 * first records logical red, white, blue, cyan and black pixels in ordinary
 * RAM. classify_ula_cell() then chooses one legal PAPER/INK pair and emits the
 * corresponding one-bit bitmap. Cells wholly inside the ball use bright red
 * and white. Boundary cells use black with either red or white so the sphere
 * keeps a clean outline instead of showing coloured rectangles.
 *
 * FRAME PIPELINE
 *
 * Each iteration performs five ordered stages:
 *
 *   1. Advance position, velocity, rotation and squash state.
 *   2. Restore the cached ULA frame and the logical pixels in the dirty region.
 *   3. Draw the shadow and projected sphere into the logical colour buffer.
 *   4. Rebuild the affected ULA bitmap and attribute cells in ordinary RAM.
 *   5. Wait for the next VBlank edge, then copy all 6,912 bytes to ULA VRAM.
 *
 * The architecture defines the ULA aperture at 0xFA000-0xFBAFF. IE64 can copy
 * the completed frame through that aperture with 864 aligned 64-bit stores.
 * The paged ULA_DATA interface remains valid, but a full frame would require
 * 6,912 separate byte writes. Drawing off-screen keeps intermediate work out of
 * live ULA VRAM and confines display-memory writes to the final copy.
 *
 * AUDIO PIPELINE
 *
 * FLEX channel 0 supplies the pitched triangle body of each impact. FLEX
 * channel 1 supplies a short noise transient. The SoundChip then applies its
 * global reverb stage. The embedded MIDI file uses the separate MIDI player
 * PTR/LEN/CTRL protocol. Its volume is set to 64 out of 255 so that the impact
 * voices remain prominent. The MIDI data is part of the .ie64 image, so the
 * finished demo has no runtime file dependency.
 *
 * (c) 2024-2026 Zayn Otley - GPLv3 or later
 * ============================================================================
 */
#include <stdint.h>
#include <intuitionengine.h>

/* ========================================================================== */
/* DISPLAY GEOMETRY AND FRAME LAYOUT                                          */
/* ========================================================================== */

#define ULA_WIDTH 256u
#define ULA_HEIGHT 192u
#define ULA_BITMAP_SIZE 6144u
#define ULA_ATTR_SIZE 768u
#define ULA_FRAME_SIZE (ULA_BITMAP_SIZE + ULA_ATTR_SIZE)

/* ========================================================================== */
/* HARDWARE REGISTERS                                                         */
/* ========================================================================== */

/* ULA control registers and the separate 6,912-byte VRAM aperture. */
#define ULA_BORDER IE_REG8(0x000f2000u)
#define ULA_CTRL IE_REG8(0x000f2004u)
#define ULA_ADDR_LO IE_REG8(0x000f200cu)
#define ULA_ADDR_HI IE_REG8(0x000f2010u)
#define ULA_DATA IE_REG8(0x000f2014u)
#define ULA_VRAM_APERTURE ((volatile uint64_t *)(uintptr_t)0x000fa000u)
#define CPU_WAIT_VBLANK IE_REG32(0x000f2580u)
/* SoundChip global controls and the first two 64-byte FLEX channel blocks. */
#define AUDIO_CTRL IE_REG32(IE_AUDIO_CONTROL)
#define REVERB_MIX IE_REG32(0x000f0a50u)
#define REVERB_DECAY IE_REG32(0x000f0a54u)
#define FLEX_CH0 0x000f0a80u
#define FLEX_CH1 0x000f0ac0u
#define FLEX_FREQ 0x00u
#define FLEX_VOL 0x04u
#define FLEX_CTRL 0x08u
#define FLEX_SWEEP 0x10u
#define FLEX_ATK 0x14u
#define FLEX_DEC 0x18u
#define FLEX_SUS 0x1cu
#define FLEX_REL 0x20u
#define FLEX_WAVE 0x24u
#define FLEX_NOISE_MODE 0x2cu
#define FLEX_PHASE 0x30u
#define FLEX_ENABLE_GATE 3u
#define WAVE_TRIANGLE 1u
#define WAVE_NOISE 3u
/* MIDI file-player registers. CTRL value 5 means start and loop. */
#define MIDI_PLAY_PTR IE_REG32(0x000f0ba0u)
#define MIDI_PLAY_LEN IE_REG32(0x000f0ba4u)
#define MIDI_VOLUME IE_REG32(0x000f0bb4u)
#define MIDI_PLAY_CTRL IE_REG32(0x000f0ba8u)
#define MIDI_PLAY_LOOP 0x05u
#define MMIO32(a) IE_REG32(a)

/* ========================================================================== */
/* LOGICAL COLOURS                                                            */
/* ========================================================================== */

/* These values index boing_colours. classify_ula_cell() maps them to ULA ink
 * and paper values after examining all 64 pixels in one attribute cell. */
#define COLOUR_BLACK 0u
#define COLOUR_BLUE 1u
#define COLOUR_RED 2u
#define COLOUR_WHITE 3u
#define COLOUR_CYAN 4u

/* ========================================================================== */
/* DEVELOPMENT AND ANIMATION STATE                                            */
/* ========================================================================== */

#define DIAGNOSTICS_ABI_VERSION 0x00010000u
#define MOTION_RUNNING 0u
#define MOTION_STATIC 1u

struct boing_tuning {
	float gravity;
	float restitution;
	float horizontal_speed;
	float angular_velocity_x;
	float angular_velocity_y;
	float squash_amount;
	float ball_radius;
	float checker_scale;
	float sound_start_hz;
	float sound_end_hz;
	uint32_t squash_ticks;
	uint32_t body_volume;
	uint32_t transient_volume;
	uint32_t sweep;
	uint32_t reverb_mix;
	uint32_t reverb_decay;
};

/* IEScript and IEMon locate this structure through the linker map. Counters
 * describe the programme's render-to-commit order. The floating-point fields
 * also provide deterministic states for visual tests and interactive tuning.
 *
 * The fixed dirty region is sized for the release radius and squash values.
 * Scripts that alter either value must keep the rendered ball inside that
 * region. ball_radius must remain above 0.0. squash_amount must remain at least
 * 0.0 and below 1.0. While a squash is active, squash_ticks must remain non-zero
 * and squash_remaining must not exceed squash_ticks. The maximum impact
 * contribution is 76 for body_volume and 38 for transient_volume, so their
 * respective safe maxima are 179 and 217. checker_scale and sound_end_hz are
 * retained in this diagnostics ABI but are not read by the current renderer or
 * sound trigger. */
struct boing_diagnostics {
	uint32_t abi_version;
	uint32_t frame_started;
	uint32_t render_complete;
	uint32_t frame_committed;
	float ball_x;
	float ball_y;
	float velocity_x;
	float velocity_y;
	float rotation_x;
	float rotation_y;
	uint32_t motion_state;
	uint32_t impact_count;
	float impact_velocity;
	uint32_t ula_bytes_written;
	uint32_t audio_trigger_count;
	uint32_t last_audio_frame;
	uint32_t squash_remaining;
	uint32_t render_request;
	uint32_t renderer_pixels;
	uint32_t boundary_cells;
	struct boing_tuning tuning;
};

/* Keep the symbol global and named because the linker map is the debugger ABI. */
volatile struct boing_diagnostics boing_diagnostics = {
	DIAGNOSTICS_ABI_VERSION, 0, 0, 0,
	70.0f, 57.0f, 0.75f, 0.0f, 0.0f, 0.0f,
	MOTION_RUNNING, 0, 0.0f, 0, 0, 0, 0, 0, 0, 0,
	{
		0.12f, 0.86f, 0.75f, 0.025f, 0.014f, 0.18f,
		42.0f, 8.0f, 205.0f, 78.0f,
		3, 178, 68, 0x000000fbu, 55, 105
	}
};

/* ========================================================================== */
/* WORKING BUFFERS                                                            */
/* ========================================================================== */

/* The union gives the renderer byte access to the Spectrum layout and aligned
 * 64-bit access to the same frame during the final aperture copy. */
union ula_frame_buffer {
	uint64_t words[ULA_FRAME_SIZE / 8u];
	uint8_t bytes[ULA_FRAME_SIZE];
};

/* boing_colours is a full-resolution logical colour image. The background
 * copies hold the complete static scene in logical and ULA-ready forms. */
union ula_frame_buffer boing_frame;
union ula_frame_buffer boing_background_frame;
uint8_t boing_colours[ULA_WIDTH * ULA_HEIGHT];
uint8_t boing_background_colours[ULA_WIDTH * ULA_HEIGHT];
float boing_sqrt_lut[1025];
extern uint32_t boing_music_data_ptr(void);
extern uint32_t boing_music_data_end_ptr(void);

/* ========================================================================== */
/* MATHEMATICAL AND ULA ADDRESS HELPERS                                       */
/* ========================================================================== */

/* Negate x when the comparison x < 0 is true, without depending on the standard
 * mathematics library. Negative zero and NaN pass through unchanged. */
static float absf(float x) { return x < 0.0f ? -x : x; }

/* Keep an angle in [-pi, pi]. With the release increments, each loop normally
 * executes at most once. Normalisation keeps the representation bounded during
 * continuous execution. */
static float wrap_angle(float x)
{
	const float pi = 3.14159265358979323846f;
	const float tau = 6.28318530717958647692f;
	while (x > pi) x -= tau;
	while (x < -pi) x += tau;
	return x;
}

/* Divide the visible hemisphere into eight longitude sectors without atan2().
 * The signs select a quadrant and the larger magnitude selects one of its two
 * octants. Alternating that result with the latitude band forms the pattern. */
static int32_t longitude_octant(float x, float z)
{
	float ax = absf(x);
	float az = absf(z);
	if (z >= 0.0f) {
		if (x >= 0.0f) return ax > az ? 1 : 0;
		return ax > az ? 6 : 7;
	}
	if (x >= 0.0f) return ax > az ? 2 : 3;
	return ax > az ? 5 : 4;
}

/* Translate a bitmap byte coordinate into the Spectrum's interleaved display
 * order. Rows are grouped by character line, pixel line and 64-row third rather
 * than stored as one linear 192-row raster. */
static uint32_t ula_bitmap_offset(uint32_t x_byte, uint32_t y)
{
	return ((y & 0xc0u) << 5) | ((y & 7u) << 8) |
	       ((y & 0x38u) << 2) | x_byte;
}

/* ========================================================================== */
/* STATIC BACKGROUND AND SHADOW                                               */
/* ========================================================================== */

/* Build the logical background once. Cyan markers become bright-blue pixels
 * when the cell classifier maps this two-colour layer to black PAPER and blue
 * INK. Uneven row spacing and converging rays suggest a perspective floor. */
static void clear_scene(void)
{
	uint32_t x, y;
	for (y = 0; y < ULA_HEIGHT; ++y) {
		for (x = 0; x < ULA_WIDTH; ++x) {
			uint8_t colour = COLOUR_BLUE;
			/* Rows spread out below the horizon while nine rays converge on it. */
			if (y >= 130u) {
				uint32_t row = y - 130u;
				int32_t dx = (int32_t)x - 128;
				int32_t ray;
				if ((row == 0u) || (row == 7u) || (row == 16u) ||
				    (row == 28u) || (row == 43u) || (row == 61u))
					colour = COLOUR_CYAN;
				for (ray = -4; ray <= 4; ++ray) {
					int32_t distance = dx * 4 - ray * ((int32_t)row + 4);
					if (distance >= -2 && distance <= 2) colour = COLOUR_CYAN;
				}
			}
			boing_colours[y * ULA_WIDTH + x] = colour;
		}
	}
}

/* Draw a flattened ellipse beneath the ball. Its width and height grow as the
 * ball approaches the floor. The two divisions are hoisted out of the pixel
 * loops; every candidate pixel then uses reciprocal multiplication. */
static void render_shadow(float cx, float floor_y, float radius, float height)
{
	int32_t x, y;
	float proximity = 1.0f - height * (1.0f / 105.0f);
	float rx, ry, inv_rx, inv_ry;
	if (proximity < 0.15f) proximity = 0.15f;
	rx = radius * (0.48f + proximity * 0.38f);
	ry = 2.0f + proximity * 3.0f;
	inv_rx = 1.0f / rx;
	inv_ry = 1.0f / ry;
	for (y = (int32_t)(floor_y - ry); y <= (int32_t)(floor_y + ry); ++y) {
		for (x = (int32_t)(cx - rx); x <= (int32_t)(cx + rx); ++x) {
			float nx, ny;
			if (x < 0 || x >= ULA_WIDTH || y < 0 || y >= ULA_HEIGHT) continue;
			nx = ((float)x - cx) * inv_rx;
			ny = ((float)y - floor_y) * inv_ry;
			if (nx * nx + ny * ny <= 1.0f)
				boing_colours[(uint32_t)y * ULA_WIDTH + (uint32_t)x] = COLOUR_CYAN;
		}
	}
}

/* ========================================================================== */
/* SPHERE RENDERER                                                            */
/* ========================================================================== */

/* Rasterise the orthographically projected hemisphere inside its clipped box.
 *
 * Normalised x and y locate a pixel on the unit disc. The square-root table
 * supplies positive z, producing the front half of a sphere. Two rotations
 * transform that surface point. Its longitude sector and latitude band then
 * select red or white. Squash widens rx, shortens ry and lowers the centre so
 * the ball remains in contact with the floor during impact frames. */
void render_ball(void)
{
	const float floor_y = 174.0f;
	float radius = boing_diagnostics.tuning.ball_radius;
	float squash = 0.0f;
	float rx, ry, inv_rx, inv_ry, cy, sx, cx, sy, cos_y;
	int32_t left, right, top, bottom, x, y;

	if (boing_diagnostics.squash_remaining != 0u) {
		float phase = (float)boing_diagnostics.squash_remaining /
		              (float)boing_diagnostics.tuning.squash_ticks;
		squash = boing_diagnostics.tuning.squash_amount * phase;
	}
	rx = radius * (1.0f + squash);
	ry = radius * (1.0f - squash);
	inv_rx = 1.0f / rx;
	inv_ry = 1.0f / ry;
	cy = boing_diagnostics.ball_y + (radius - ry);

	render_shadow(boing_diagnostics.ball_x, floor_y + 3.0f, radius,
	              floor_y - (boing_diagnostics.ball_y + radius));

	cx = __builtin_ie64_fcos(boing_diagnostics.rotation_x);
	sx = __builtin_ie64_fsin(boing_diagnostics.rotation_x);
	cy = boing_diagnostics.ball_y + (radius - ry);
	sy = __builtin_ie64_fsin(boing_diagnostics.rotation_y);
	cos_y = __builtin_ie64_fcos(boing_diagnostics.rotation_y);

	left = (int32_t)(boing_diagnostics.ball_x - rx - 1.0f);
	right = (int32_t)(boing_diagnostics.ball_x + rx + 1.0f);
	top = (int32_t)(cy - ry - 1.0f);
	bottom = (int32_t)(cy + ry + 1.0f);
	if (left < 0) left = 0;
	if (right >= ULA_WIDTH) right = ULA_WIDTH - 1;
	if (top < 0) top = 0;
	if (bottom >= ULA_HEIGHT) bottom = ULA_HEIGHT - 1;

	for (y = top; y <= bottom; ++y) {
		float ny = ((float)y - cy) * inv_ry;
		for (x = left; x <= right; ++x) {
			float nx = ((float)x - boing_diagnostics.ball_x) * inv_rx;
			float rr = nx * nx + ny * ny;
			float nz, px, pz, py;
			int32_t iu, iv;
			if (rr > 1.0f) continue;
			nz = boing_sqrt_lut[(uint32_t)(rr * 1024.0f)];
			px = nx * cx + nz * sx;
			pz = nz * cx - nx * sx;
			py = ny * cos_y - pz * sy;
			pz = pz * cos_y + ny * sy;
			iu = longitude_octant(px, pz);
			iv = (int32_t)((py + 1.0f) * 2.0f);
			if (iv < 0) iv = 0;
			if (iv > 3) iv = 3;
			boing_colours[(uint32_t)y * ULA_WIDTH + (uint32_t)x] =
				((iu ^ iv) & 1) ? COLOUR_WHITE : COLOUR_RED;
			++boing_diagnostics.renderer_pixels;
		}
	}
}

/* Convert one 8 by 8 logical cell into one legal ULA attribute and eight bitmap
 * bytes. A complete ball cell can retain red and white. A boundary cell keeps
 * the more common ball colour against black and chooses red when the counts are
 * equal. The other ball colour is discarded to prevent attribute clash around
 * the silhouette. Background and shadow cells share a black-and-blue pair. */
void classify_ula_cell(uint32_t cell_x, uint32_t cell_y)
{
	uint32_t counts[5] = {0, 0, 0, 0, 0};
	uint32_t px, py, bit, ball_count;
	uint8_t paper, ink, byte;
	for (py = 0; py < 8u; ++py)
		for (px = 0; px < 8u; ++px)
			++counts[boing_colours[(cell_y * 8u + py) * ULA_WIDTH + cell_x * 8u + px]];

	ball_count = counts[COLOUR_RED] + counts[COLOUR_WHITE];
	if (ball_count == 64u) {
		paper = 2u; ink = 7u;
	} else if (ball_count != 0u) {
		paper = 0u;
		ink = counts[COLOUR_WHITE] > counts[COLOUR_RED] ? 7u : 2u;
		++boing_diagnostics.boundary_cells;
	} else if (counts[COLOUR_BLACK] != 0u) {
		paper = 1u; ink = 0u;
	} else {
		paper = 0u; ink = 1u;
	}
	boing_frame.bytes[ULA_BITMAP_SIZE + cell_y * 32u + cell_x] =
		(uint8_t)(0x40u | (paper << 3) | ink);

	for (py = 0; py < 8u; ++py) {
		byte = 0;
		for (px = 0; px < 8u; ++px) {
			uint8_t c = boing_colours[(cell_y * 8u + py) * ULA_WIDTH + cell_x * 8u + px];
			if (ball_count == 64u) bit = c == COLOUR_WHITE;
			else if (ball_count != 0u) bit = (ink == 7u) ? c == COLOUR_WHITE : c == COLOUR_RED;
			else if (counts[COLOUR_BLACK] != 0u) bit = c == COLOUR_BLACK;
			else bit = c == COLOUR_CYAN;
			if (bit) byte |= (uint8_t)(0x80u >> px);
		}
		boing_frame.bytes[ula_bitmap_offset(cell_x, cell_y * 8u + py)] = byte;
	}
}

/* Restore the cached frame, redraw the moving objects and classify the selected
 * dirty region. The fixed margins cover the ball and shadow at their release
 * settings. Row 23 provides a conservative bound at the bottom edge of the
 * 24-row ULA attribute grid. */
static void render_frame(void)
{
	int32_t left, right, top, bottom, x, y;
	boing_diagnostics.renderer_pixels = 0;
	boing_diagnostics.boundary_cells = 0;
	for (x = 0; x < (int32_t)(ULA_FRAME_SIZE / 8u); ++x)
		boing_frame.words[x] = boing_background_frame.words[x];
	left = ((int32_t)boing_diagnostics.ball_x - 52) >> 3;
	right = ((int32_t)boing_diagnostics.ball_x + 52) >> 3;
	top = ((int32_t)boing_diagnostics.ball_y - 48) >> 3;
	bottom = 23;
	if (left < 0) left = 0;
	if (right > 31) right = 31;
	if (top < 0) top = 0;
	for (y = top * 8; y < (bottom + 1) * 8; ++y)
		for (x = left * 8; x < (right + 1) * 8; ++x)
			boing_colours[(uint32_t)y * ULA_WIDTH + (uint32_t)x] =
				boing_background_colours[(uint32_t)y * ULA_WIDTH + (uint32_t)x];
	render_ball();
	for (y = top; y <= bottom; ++y)
		for (x = left; x <= right; ++x)
			classify_ula_cell((uint32_t)x, (uint32_t)y);
	boing_diagnostics.render_complete = boing_diagnostics.frame_started;
}

/* ========================================================================== */
/* MOTION AND IMPACT SOUND                                                    */
/* ========================================================================== */

/* Retrigger both FLEX voices for one collision. CTRL is cleared before any
 * parameter changes, then PHASE resets both oscillators before enable and gate
 * are asserted together. Impact speed raises the body and transient volumes.
 * Its contribution is capped at 76, which keeps the default body level below
 * the SoundChip maximum of 255. */
void trigger_boing(void)
{
	uint32_t body = FLEX_CH0;
	uint32_t noise = FLEX_CH1;
	uint32_t strength = (uint32_t)(boing_diagnostics.impact_velocity * 7.0f);
	if (strength > 76u) strength = 76u;
	MMIO32(body + FLEX_CTRL) = 0;
	MMIO32(noise + FLEX_CTRL) = 0;
	MMIO32(body + FLEX_FREQ) = (uint32_t)(boing_diagnostics.tuning.sound_start_hz * 256.0f);
	MMIO32(body + FLEX_VOL) = boing_diagnostics.tuning.body_volume + strength;
	MMIO32(body + FLEX_SWEEP) = boing_diagnostics.tuning.sweep;
	MMIO32(body + FLEX_PHASE) = 0;
	MMIO32(noise + FLEX_VOL) = boing_diagnostics.tuning.transient_volume + (strength >> 1);
	MMIO32(noise + FLEX_PHASE) = 0;
	MMIO32(body + FLEX_CTRL) = FLEX_ENABLE_GATE;
	MMIO32(noise + FLEX_CTRL) = FLEX_ENABLE_GATE;
	++boing_diagnostics.audio_trigger_count;
	boing_diagnostics.last_audio_frame = boing_diagnostics.frame_started;
}

/* Clamp the ball to the floor and turn downward velocity into one upward
 * rebound. Only a positive incoming velocity creates an impact. The collision
 * changes it to a negative rebound, so the same contact cannot trigger twice.
 * The minimum rebound preserves a steady demonstration loop once restitution
 * has removed energy from the earlier bounces. */
void resolve_collision(void)
{
	float floor_y = 174.0f - boing_diagnostics.tuning.ball_radius;
	float rebound;
	if (boing_diagnostics.ball_y < floor_y) return;
	boing_diagnostics.ball_y = floor_y;
	if (boing_diagnostics.velocity_y > 0.0f) {
		boing_diagnostics.impact_velocity = boing_diagnostics.velocity_y;
		rebound = boing_diagnostics.velocity_y * boing_diagnostics.tuning.restitution;
		/* Prevent low-energy floor chatter during an unattended loop. */
		if (rebound < 2.8f) rebound = 2.8f;
		boing_diagnostics.velocity_y = -rebound;
		boing_diagnostics.squash_remaining = boing_diagnostics.tuning.squash_ticks;
		++boing_diagnostics.impact_count;
		trigger_boing();
	}
}

/* Advance one fixed animation step. Static mode leaves the animation fields
 * untouched for IEScript captures and acknowledges one pending render request.
 * Running mode applies gravity, reflects the horizontal velocity at the travel
 * limits, advances both rotation angles and finally resolves the collision. */
void advance_physics(void)
{
	if (boing_diagnostics.motion_state == MOTION_STATIC) {
		if (boing_diagnostics.render_request != 0u) --boing_diagnostics.render_request;
		return;
	}
	boing_diagnostics.velocity_y += boing_diagnostics.tuning.gravity;
	boing_diagnostics.ball_y += boing_diagnostics.velocity_y;
	boing_diagnostics.ball_x += boing_diagnostics.velocity_x;
	if (boing_diagnostics.ball_x < 48.0f) {
		boing_diagnostics.ball_x = 48.0f;
		boing_diagnostics.velocity_x = absf(boing_diagnostics.tuning.horizontal_speed);
	} else if (boing_diagnostics.ball_x > 208.0f) {
		boing_diagnostics.ball_x = 208.0f;
		boing_diagnostics.velocity_x = -absf(boing_diagnostics.tuning.horizontal_speed);
	}
	boing_diagnostics.rotation_x = wrap_angle(boing_diagnostics.rotation_x +
		boing_diagnostics.tuning.angular_velocity_x);
	boing_diagnostics.rotation_y = wrap_angle(boing_diagnostics.rotation_y +
		boing_diagnostics.tuning.angular_velocity_y);
	resolve_collision();
	if (boing_diagnostics.squash_remaining != 0u) --boing_diagnostics.squash_remaining;
}

/* ========================================================================== */
/* FRAME COMMIT                                                               */
/* ========================================================================== */

/* Copy the completed ordinary-RAM frame through the canonical ULA aperture.
 * ULA_FRAME_SIZE is divisible by eight, and the union is 64-bit aligned, so the
 * loop performs exactly 864 aligned stores. Diagnostic state is published only
 * after the final store. */
void commit_ula_frame(void)
{
	uint32_t i;
	for (i = 0; i < ULA_FRAME_SIZE / 8u; ++i)
		ULA_VRAM_APERTURE[i] = boing_frame.words[i];
	boing_diagnostics.ula_bytes_written = ULA_FRAME_SIZE;
	boing_diagnostics.frame_committed = boing_diagnostics.render_complete;
}

/* ========================================================================== */
/* AUDIO AND MUSIC INITIALISATION                                             */
/* ========================================================================== */

/* Configure the two SoundChip FLEX voices without starting an envelope. The
 * pitched body uses a triangle waveform and a longer decay. The noise voice
 * uses a brief white-noise envelope for the contact transient. Reverb is a
 * global SoundChip stage and is set before the first collision. */
static void initialise_audio(void)
{
	uint32_t body = FLEX_CH0;
	uint32_t noise = FLEX_CH1;
	AUDIO_CTRL = 1;
	REVERB_MIX = boing_diagnostics.tuning.reverb_mix;
	REVERB_DECAY = boing_diagnostics.tuning.reverb_decay;
	MMIO32(body + FLEX_WAVE) = WAVE_TRIANGLE;
	MMIO32(body + FLEX_ATK) = 1;
	MMIO32(body + FLEX_DEC) = 85;
	MMIO32(body + FLEX_SUS) = 0;
	MMIO32(body + FLEX_REL) = 65;
	MMIO32(noise + FLEX_WAVE) = WAVE_NOISE;
	MMIO32(noise + FLEX_NOISE_MODE) = 0;
	MMIO32(noise + FLEX_FREQ) = 3200u * 256u;
	MMIO32(noise + FLEX_ATK) = 1;
	MMIO32(noise + FLEX_DEC) = 18;
	MMIO32(noise + FLEX_SUS) = 0;
	MMIO32(noise + FLEX_REL) = 8;
}

/* Start the embedded Standard MIDI File through the file-player PTR/LEN/CTRL
 * protocol described by the architecture. The assembly helpers materialise
 * linked 32-bit IE memory addresses. Subtracting start from end keeps the byte
 * count tied to the incbin payload. CTRL value 5 starts playback and enables
 * looping. */
static void start_music(void)
{
	uint32_t start = boing_music_data_ptr();
	MIDI_PLAY_PTR = start;
	MIDI_PLAY_LEN = boing_music_data_end_ptr() - start;
	MIDI_VOLUME = 64u;
	MIDI_PLAY_CTRL = MIDI_PLAY_LOOP;
}

/* ========================================================================== */
/* LOOK-UP TABLE AND BACKGROUND INITIALISATION                                */
/* ========================================================================== */

/* Build sqrt(1-r) for 1,025 evenly spaced squared radial distances across the
 * unit disc. This invokes the IE64 floating-point square-root operation during
 * start-up instead of once for every covered sphere pixel. Entry 1,024 is
 * exactly the silhouette value zero. */
static void initialise_sphere_lut(void)
{
	uint32_t i;
	for (i = 0; i <= 1024u; ++i) {
		float rr = (float)i * (1.0f / 1024.0f);
		boing_sqrt_lut[i] = __builtin_ie64_fsqrt(1.0f - rr);
	}
}

/* Render and classify the complete static scene once. Each moving frame begins by
 * copying all 6,912 bytes of boing_background_frame, then restores logical
 * background pixels only inside the dirty region before drawing the moving
 * ball and shadow. */
static void initialise_background(void)
{
	uint32_t i, x, y;
	clear_scene();
	for (y = 0; y < 24u; ++y)
		for (x = 0; x < 32u; ++x)
			classify_ula_cell(x, y);
	for (i = 0; i < ULA_WIDTH * ULA_HEIGHT; ++i)
		boing_background_colours[i] = boing_colours[i];
	for (i = 0; i < ULA_FRAME_SIZE / 8u; ++i)
		boing_background_frame.words[i] = boing_frame.words[i];
}

/* ========================================================================== */
/* PROGRAMME ENTRY AND MAIN LOOP                                              */
/* ========================================================================== */

/* Enable the ULA and its paged-port auto-increment mode, build all static state,
 * configure audio and start the embedded music. The main loop renders in
 * ordinary RAM, parks at the next VBlank edge through the shared CPU wait
 * service, then commits the completed ULA frame. */
int main(void)
{
	ULA_BORDER = 1;
	ULA_CTRL = 1u | 4u;
	initialise_sphere_lut();
	initialise_background();
	initialise_audio();
	start_music();
	for (;;) {
		++boing_diagnostics.frame_started;
		advance_physics();
		render_frame();
		CPU_WAIT_VBLANK = 1;
		commit_ula_frame();
	}
}

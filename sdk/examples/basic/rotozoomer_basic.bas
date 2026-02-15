1 REM ================================================================
2 REM  ROTOZOOMER DEMO - EhBASIC REFERENCE IMPLEMENTATION
3 REM  Intuition Engine SDK Tutorial
4 REM ================================================================
5 REM  SDK QUICK REFERENCE:
6 REM  Target CPU:    IE64 (via EhBASIC interpreter)
7 REM  Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true color)
8 REM  Audio Engine:  SID (Commodore 64 sound chip)
9 REM  Prerequisites: Build EhBASIC with 'make basic'
10 REM  Run:           ./bin/IntuitionEngine -basic
11 REM                 Then type: LOAD "rotozoomer_basic.bas"
12 REM                 Then type: RUN
13 REM  Porting:       BASIC programs are CPU-agnostic (run on IE64
14 REM                 interpreter). No porting needed.
15 REM
6 REM  This programme demonstrates the Mode7 hardware blitter to
7 REM  perform real-time affine texture mapping — the same technique
8 REM  the SNES used for its famous "Mode 7" rotating/scaling
9 REM  backgrounds in games like F-Zero and Mario Kart.
10 REM
11 REM  A 256x256 checkerboard texture is generated in memory, then
12 REM  every frame the blitter rotates and zooms it across the full
13 REM  640x480 display. SID music plays in the background.
14 REM
15 REM  WHY MODE7 HARDWARE?
16 REM  Software rendering in BASIC would need to calculate UV
17 REM  coordinates for all 307,200 pixels per frame — far too slow
18 REM  for 60fps. The Mode7 blitter does this in hardware: we just
19 REM  supply 6 affine parameters and it handles every pixel.
20 REM
21 REM  MEMORY MAP:
22 REM    0x100000  VRAM front buffer (640x480x4 = 1,228,800 bytes)
23 REM    0x500000  Texture source (256x256x4 = 262,144 bytes)
24 REM    0x600000  Back buffer for Mode7 output (same size as VRAM)
25 REM    0x710000  SID music file (loaded via BLOAD)
26 REM
27 REM  AFFINE TRANSFORMATION:
28 REM    For each output pixel (x,y), the blitter computes:
29 REM      U = u0 + x*dUdX + y*dUdY
30 REM      V = v0 + x*dVdX + y*dVdY
31 REM    where (U,V) are texture coordinates in 16.16 fixed-point.
32 REM    For pure rotation+zoom, the matrix is:
33 REM      dUdX =  CA    dUdY = SA     (cosine/sine scaled by zoom)
34 REM      dVdX = -SA    dVdY = CA
35 REM    and (u0,v0) centres the rotation on the texture.
36 REM
37 REM ================================================================

100 REM ================================================================
101 REM  HARDWARE INITIALISATION
102 REM ================================================================
103 REM  POKE &HF0000,1 — enable the VideoChip (register 0 = enable)
104 REM  POKE &HF0004,0 — select mode 0 (640x480 true colour)
105 REM  Both must be set before any rendering is visible.
110 POKE &HF0000, 1: POKE &HF0004, 0

200 REM ================================================================
201 REM  SID MUSIC PLAYBACK
202 REM ================================================================
203 REM  BLOAD loads a binary file into bus memory at address SA.
204 REM  We place it at 0x710000 — well above VRAM and textures so
205 REM  there is no overlap. The SID player parses the file header
206 REM  and drives the SID chip emulation in the background.
207 REM
208 REM  SID PLAY addr, length — starts playback from memory.
209 REM  3725 bytes is the size of the Yummy_Pizza.sid file.
210 REM  SID STATUS returns 1 if playing, 0 if stopped.
211 REM  POKE8 &HF0E18,15 — sets SID master volume to maximum (0-15).
220 SA=&H710000
230 BLOAD "sdk/examples/assets/Yummy_Pizza.sid", SA
240 SID PLAY SA, 3725
250 PRINT "SID STATUS=";SID STATUS
260 POKE8 &HF0E18, 15

300 REM ================================================================
301 REM  TEXTURE GENERATION (256x256 CHECKERBOARD)
302 REM ================================================================
303 REM  We build a 2x2 checkerboard using four 128x128 BLIT FILL
304 REM  operations. Each pixel is 4 bytes (32-bit ARGB), so one row
305 REM  of the 256-pixel-wide texture is 1024 bytes (the stride).
306 REM
307 REM  TEXTURE LAYOUT:
308 REM    +--- 128px ---+--- 128px ---+
309 REM    |             |             |
310 REM    |   WHITE     |   BLACK     |  top half
311 REM    |  TB+0       |  TB+512     |
312 REM    +-------------+-------------+
313 REM    |             |             |
314 REM    |   BLACK     |   WHITE     |  bottom half
315 REM    |  TB+131072  |  TB+131584  |
316 REM    +-------------+-------------+
317 REM
318 REM  ADDRESS CALCULATIONS:
319 REM    TB+0      = top-left origin
320 REM    TB+512    = 128 pixels * 4 bytes = 512 bytes right
321 REM    TB+131072 = 128 rows * 1024 stride = one half-height down
322 REM    TB+131584 = 131072 + 512 = bottom-right quadrant
323 REM
324 REM  WHY BLIT FILL?
325 REM  Four hardware fills are faster and simpler than nested
326 REM  FOR/NEXT loops POKEing 65,536 pixels individually.
330 TB=&H500000: ST=1024
340 W=&HFFFFFFFF: B=&HFF000000
350 BLIT FILL TB, 128, 128, W, ST
360 BLIT FILL TB+512, 128, 128, B, ST
370 BLIT FILL TB+131072, 128, 128, B, ST
380 BLIT FILL TB+131584, 128, 128, W, ST

400 REM ================================================================
401 REM  CONSTANTS AND ANIMATION STATE
402 REM ================================================================
403 REM  BB = back buffer address — Mode7 renders here, then we copy
404 REM       to VRAM. This double-buffering prevents visible tearing
405 REM       (the display never reads a half-rendered frame).
406 REM  FP = 65536 = 2^16, the fixed-point multiplier. Multiplying
407 REM       a floating-point value by FP and taking INT() converts
408 REM       it to 16.16 fixed-point format for the blitter hardware.
409 REM  A  = rotation angle in radians (starts at 0).
410 REM  SI = zoom oscillation phase in radians (starts at 0).
420 BB=&H600000: FP=65536
430 A=0: SI=0

500 REM ================================================================
501 REM  MAIN LOOP — ONE ITERATION PER FRAME
502 REM ================================================================

600 REM ----------------------------------------------------------------
601 REM  STEP 1: COMPUTE SCALE FACTOR
602 REM ----------------------------------------------------------------
603 REM  SC oscillates between 0.2 and 0.8 via:  SC = 0.5 + sin(SI)*0.3
604 REM  The 0.5 baseline keeps the scale always positive (no flipping).
605 REM  The 0.3 amplitude gives roughly a 4:1 zoom ratio between the
606 REM  closest and furthest points (0.2 → 5x zoom, 0.8 → 1.25x zoom).
607 REM  The clamp at 0.2 is a safety net — sin() should never push SC
608 REM  below 0.2, but floating-point edge cases could cause division
609 REM  by near-zero which would produce enormous deltas.
610 SC=0.5+SIN(SI)*0.3: IF SC<0.2 THEN SC=0.2

700 REM ----------------------------------------------------------------
701 REM  STEP 2: COMPUTE ROTATED+SCALED DIRECTION VECTORS
702 REM ----------------------------------------------------------------
703 REM  CA = cos(A)/SC  and  SA = sin(A)/SC
704 REM  Dividing by SC combines rotation and zoom into one operation.
705 REM  When SC is small (zoomed in), CA and SA become large, making
706 REM  the blitter take bigger steps through the texture — which is
707 REM  exactly how zooming in works in UV space.
710 CA=COS(A)/SC: SA=SIN(A)/SC

800 REM ----------------------------------------------------------------
801 REM  STEP 3: CONVERT TO 16.16 FIXED-POINT FOR THE BLITTER
802 REM ----------------------------------------------------------------
803 REM  The Mode7 blitter works in 16.16 fixed-point: the upper 16
804 REM  bits are the integer pixel coordinate, the lower 16 bits are
805 REM  the fractional part (sub-pixel precision for smooth motion).
806 REM
807 REM  DC (delta-cosine) and DS (delta-sine) are the per-pixel step
808 REM  sizes. These become the Mode7 affine matrix:
809 REM    dUdX =  DC    dUdY =  DS
810 REM    dVdX = -DS    dVdY =  DC
811 REM
812 REM  INT() truncates toward zero, matching the blitter's internal
813 REM  fixed-point truncation (the difference from floor() is at most
814 REM  1 LSB for negative values — sub-pixel, visually imperceptible).
820 DC=INT(CA*FP): DS=INT(SA*FP)

900 REM ----------------------------------------------------------------
901 REM  STEP 4: COMPUTE STARTING UV ORIGIN (CENTRING FORMULA)
902 REM ----------------------------------------------------------------
903 REM  We want the centre of the texture (128,128) to appear at the
904 REM  centre of the screen (320,240). Working backwards from the
905 REM  affine equation:
906 REM    U(x,y) = u0 + x*CA + y*SA
907 REM  At the screen centre (320,240), we want U = 128 (texture centre):
908 REM    128 = u0 + 320*CA + 240*SA
909 REM    u0  = 128 - 320*CA + 240*SA    (note sign: -CA, +SA for U)
910 REM  Similarly for V:
911 REM    v0  = 128 - 320*SA - 240*CA
912 REM
913 REM  CX,CY = texture centre (128,128)
914 REM  HW,HH = half-width, half-height of display (320,240)
920 CX=128: CY=128: HW=320: HH=240
930 SU=INT((CX-HW*CA+HH*SA)*FP)
940 SV=INT((CY-HW*SA-HH*CA)*FP)

1000 REM ----------------------------------------------------------------
1001 REM  STEP 5: TRIGGER MODE7 BLIT
1002 REM ----------------------------------------------------------------
1003 REM  BLIT MODE7 parameters (14 values):
1004 REM    TB     — source texture base address
1005 REM    BB     — destination (back buffer, NOT VRAM)
1006 REM    640    — output width in pixels
1007 REM    480    — output height in pixels
1008 REM    SU     — starting U (16.16 fixed-point)
1009 REM    SV     — starting V (16.16 fixed-point)
1010 REM    DC     — dUdX: U step per pixel moving right (cosine term)
1011 REM    DS     — dVdX: V step per pixel moving right (sine term)
1012 REM    0-DS   — dUdY: U step per pixel moving down (-sine term)
1013 REM    DC     — dVdY: V step per pixel moving down (cosine term)
1014 REM    255    — texture U mask (256-1, for power-of-2 wrapping)
1015 REM    255    — texture V mask (256-1, for power-of-2 wrapping)
1016 REM    ST     — source texture stride in bytes (1024)
1017 REM    2560   — destination stride in bytes (640 pixels * 4 bytes)
1018 REM
1019 REM  WHY "0-DS" INSTEAD OF "-DS"?
1020 REM  EhBASIC's expression parser treats a bare minus as subtraction,
1021 REM  not unary negation, when it appears in an argument list. Writing
1022 REM  "0-DS" explicitly subtracts DS from zero, producing -DS reliably.
1030 BLIT MODE7 TB, BB, 640, 480, SU, SV, DC, DS, 0-DS, DC, 255, 255, ST, 2560

1100 REM ----------------------------------------------------------------
1101 REM  STEP 6: COPY BACK BUFFER TO VRAM (DOUBLE BUFFER FLIP)
1102 REM ----------------------------------------------------------------
1103 REM  The Mode7 blit rendered into the back buffer at 0x600000.
1104 REM  Now we copy the finished frame to VRAM at 0x100000 where the
1105 REM  display controller reads it. This ensures the viewer never
1106 REM  sees a partially-rendered frame.
1107 REM
1108 REM  307200 = 640 * 480 pixels, but each pixel is 4 bytes, so the
1109 REM  actual byte count is 1,228,800. The BLIT MEMCOPY command takes
1110 REM  a count in pixels (not bytes) for 32-bit framebuffer copies.
1111 REM
1112 REM  VSYNC waits for the vertical blanking interval before the next
1113 REM  frame, locking the animation to the display refresh rate (60Hz)
1114 REM  and preventing tearing during the copy.
1120 BLIT MEMCOPY &H600000, &H100000, 307200
1130 VSYNC

1200 REM ----------------------------------------------------------------
1201 REM  STEP 7: ADVANCE ANIMATION AND LOOP
1202 REM ----------------------------------------------------------------
1203 REM  A  += 0.03 radians per frame — rotation speed.
1204 REM  SI += 0.01 radians per frame — zoom oscillation speed.
1205 REM  The 3:1 ratio means the texture completes three full rotations
1206 REM  for every one full zoom cycle, creating non-repeating motion
1207 REM  that keeps the visual interesting.
1208 REM
1209 REM  A wraps at 2*PI (6.28318) to prevent unbounded growth of the
1210 REM  floating-point value, which would eventually lose precision.
1211 REM  SI does not need wrapping because SIN(SI) naturally cycles —
1212 REM  the value just grows slowly, and BASIC's SIN() handles any
1213 REM  input magnitude. Over hours of runtime the precision loss in
1214 REM  SI is negligible for a smooth visual effect.
1220 A=A+0.03: IF A>6.28318 THEN A=A-6.28318
1230 SI=SI+0.01
1240 GOTO 600

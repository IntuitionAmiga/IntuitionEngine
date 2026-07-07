
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 46 - The Rotozoomer In BASIC

The first real demo effect is the rotozoomer. A texture is rotated and
scaled across the screen every frame. On Intuition Engine, BASIC does
not draw every pixel. BASIC computes the affine parameters, then the
VideoChip Mode 7 blitter draws the frame.

The supplied BASIC demo `rotozoomer_basic.bas` uses this pattern:

1. Allocate a front buffer, a back buffer, a texture, and music staging
   memory.
2. Load a texture and start SID playback.
3. Compute angle and zoom each frame.
4. Convert the values to `16.16` fixed point.
5. Run `BLIT MODE7` into the back buffer.
6. Run `BLIT MEMCOPY` to present the finished frame.
7. `VSYNC`, advance the phases, and repeat.

## 46.1 Mode 7 In One Listing

This small version uses a generated checker texture instead of a file.

```basic
10 REM SMALL BASIC ROTOZOOMER
20 FB=MEMALLOC(1228800,4096):BB=MEMALLOC(1228800,4096)
30 TB=MEMALLOC(262144,4096):ST=1024:FP=65536
40 POKE32 &H000F0004,0:POKE32 &H000F0080,0
50 POKE32 &H000F0084,FB:POKE32 &H000F0000,1
60 BLIT FILL TB,128,128,&H003060A0,ST
70 BLIT FILL TB+128*4,128,128,&H00A06030,ST
80 BLIT FILL TB+128*ST,128,128,&H00A06030,ST
90 BLIT FILL TB+128*ST+128*4,128,128,&H003060A0,ST
100 A=0:Z=0
110 SC=0.5+SIN(Z)*0.3:IF SC<0.2 THEN SC=0.2
120 CA=COS(A)/SC:SA=SIN(A)/SC
130 DC=INT(CA*FP):DS=INT(SA*FP)
140 U0=INT((128-320*CA+240*SA)*FP)
150 V0=INT((128-320*SA-240*CA)*FP)
160 BLIT MODE7 TB,BB,640,480,U0,V0,DC,DS,0-DS,DC,255,255,ST,2560
170 BLIT MEMCOPY BB,FB,1228800
180 VSYNC
190 A=A+0.03:IF A>6.28318 THEN A=A-6.28318
200 Z=Z+0.01:IF Z>6.28318 THEN Z=Z-6.28318
210 GOTO 110
```

Expected result: the checker texture rotates and zooms across the
screen. The picture is rendered into `BB` first, then copied to `FB`.

## 46.2 The Fixed-Point Step

Mode 7 uses `16.16` fixed-point values. The upper `16` bits are the
whole texture coordinate. The lower `16` bits are the fractional part.

In BASIC:

```basic
FP=65536
DC=INT(CA*FP)
DS=INT(SA*FP)
```

If `CA` is `1.0`, `DC` becomes `65536`. That means "advance one texture
pixel for each screen pixel". If `CA` is `0.5`, `DC` becomes `32768`.
That means "advance half a texture pixel per screen pixel".

The same rule applies to `U0`, `V0`, `duCol`, `dvCol`, `duRow`, and
`dvRow`.

## 46.3 The Matrix

The blitter uses this mapping:

```text
U = U0 + col * duCol + row * duRow
V = V0 + col * dvCol + row * dvRow
```

For a rotation and zoom:

```text
duCol =  cos(A) / scale
dvCol =  sin(A) / scale
duRow = -sin(A) / scale
dvRow =  cos(A) / scale
```

The BASIC listing writes `0-DS` instead of `-DS` in the argument list.
That is a safe spelling for a negative value in IE64 BASIC expressions.

## 46.4 Double Buffering

The Mode 7 draw goes to `BB`, not directly to the displayed framebuffer:

```basic
160 BLIT MODE7 TB,BB,640,480,U0,V0,DC,DS,0-DS,DC,255,255,ST,2560
170 BLIT MEMCOPY BB,FB,1228800
180 VSYNC
```

The important number is `1228800`: `640 * 480 * 4`. `BLIT MEMCOPY` takes
a byte count, not a pixel count.

This is a simple back-buffer method. More advanced machine-code
versions often present by changing `VIDEO_FB_BASE` at VBlank.

## 46.5 Music Beside Visuals

The full BASIC demo starts SID playback before the frame loop. Once the
player is running, the visual loop does not call it every frame:

```basic
SA=MEMALLOC(4096,4096)
BLOAD "YUMMY.SID",SA
SID PLAY SA,3725
POKE8 &H000F0E18,15
```

The point is architectural. Audio is another card on the same machine.
The CPU starts it, then keeps drawing while the audio engine advances in
parallel.

## 46.6 What To Change

| Change | Effect |
|--------|--------|
| Line `110`, change `0.3` | Zoom depth. |
| Line `190`, change `0.03` | Rotation speed. |
| Line `200`, change `0.01` | Zoom pulse speed. |
| Lines `60` to `90` | Texture colours. |
| Line `170`, copy less than `1228800` | Only part of the frame is presented. |

Chapter 47 drives the same VideoChip registers from IE Script so the
effect can be used as a repeatable laboratory experiment.

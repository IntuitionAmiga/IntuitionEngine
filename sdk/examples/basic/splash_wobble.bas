1 REM ============================================================================
2 REM SPLASH WOBBLE DEMO - RAW RGBA SPLASH PLUS MIDI
3 REM EhBASIC for IntuitionEngine - VideoChip Mode 0 (640x480x32bpp)
4 REM ============================================================================
5 REM Run: bin/IntuitionEngine -basic
6 REM      LOAD "sdk/examples/basic/splash_wobble.bas"
7 REM      RUN
8 REM
9 REM MEMORY MAP:
10 REM   0x100000  Front framebuffer (640x480x4)
11 REM   0x600000  Splash source image (640x92x4)
12 REM   0x900000  Back framebuffer (640x480x4)
13 REM ============================================================================

100 REM HARDWARE SETUP
110 FB=&H100000:SR=&H600000:BB=&H900000
120 ST=2560:SW=640:SH=92:TP=194
130 POKE &HF0000,1:POKE &HF0004,0
140 POKE &HF0080,0

200 REM LOAD SPLASH AND START MIDI
210 BLOAD "sdk/examples/assets/splash_640x92.rgba",SR
220 SOUND PLAY "sdk/examples/assets/music/enjoythesilence.mid"
230 PRINT "MEDIA_TYPE=";PEEK(&HF2310)

300 REM STATIC CENTERED SPLASH
310 BLIT FILL FB,640,480,&H00000000,ST
320 BLIT COPY SR,FB+TP*ST,SW,SH,ST,ST
330 VSYNC

400 REM WOBBLE LOOP
410 T=0
500 BLIT FILL BB,640,480,&H00000000,ST
510 FOR Y=0 TO 91
520 DY=TP+Y
530 X=INT(24*SIN(T+Y*0.12))
540 DX=X:SX=0:CW=640
550 IF DX<0 THEN SX=0-DX:CW=640-SX:DX=0
560 IF DX+CW>640 THEN CW=640-DX
570 IF CW<=0 THEN GOTO 610
580 SA=SR+Y*ST+SX*4
590 DA=BB+DY*ST+DX*4
600 BLIT COPY SA,DA,CW,1,ST,ST
610 NEXT Y
620 BLIT MEMCOPY BB,FB,307200
630 VSYNC
640 T=T+0.08:IF T>6.28318 THEN T=T-6.28318
650 GOTO 500

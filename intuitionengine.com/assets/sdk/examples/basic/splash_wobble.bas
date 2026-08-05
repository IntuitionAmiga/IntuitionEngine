1 REM SPLASH WOBBLE: IE64 BASIC, VideoChip mode 0 and a MIDI-backed raster effect
3 REM guest source and RUN it. It needs splash_640x92.rgba and enjoythesilence.mid.
4 REM Read the setup, file loading, static copy, then the per-scanline loop.
5 REM The front buffer is displayed; the back buffer prevents half-drawn rows from
6 REM becoming visible while the wobble is built.

100 REM Allocate guest RAM and make FB the 640 by 480 RGBA framebuffer.
110 FB=MEMALLOC(1228800,4096):SR=MEMALLOC(235520,4096):BB=MEMALLOC(1228800,4096)
120 ST=2560:SW=640:SH=92:TP=194
130 POKE32 &HF0004,0:POKE32 &HF0080,0:POKE32 &HF0084,FB
140 POKE32 &HF0000,1

200 REM Load disk-backed guest assets before the frame loop uses their addresses.
210 BLOAD "sdk/examples/assets/splash_640x92.rgba",SR
220 SOUND PLAY "sdk/examples/assets/music/enjoythesilence.mid"
230 PRINT "MEDIA_TYPE=";PEEK32(&HF2310)

300 REM Show one centred copy first, which makes asset or stride errors obvious.
310 BLIT FILL FB,640,480,&H00000000,ST
320 BLIT COPY SR,FB+TP*ST,SW,SH,ST,ST
330 VSYNC

400 REM Each frame shifts one source row, clips it to the display, then presents BB.
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
620 BLIT MEMCOPY BB,FB,1228800
630 VSYNC
640 T=T+0.08:IF T>6.28318 THEN T=T-6.28318
650 GOTO 500

1 REM WOBBLE ZOOM: IE64 BASIC, a disk-backed splash and the Mode7 blitter
3 REM RUN it. The example reads splash_640x92.rgba and enjoythesilence.mid.
4 REM It builds a 1024 by 512 power-of-two texture, submits an affine Mode7 blit
5 REM into BB, then copies BB to the displayed framebuffer FB.

100 REM Start the guest MIDI asset independently of the video submission path.
110 SOUND PLAY "sdk/examples/assets/music/enjoythesilence.mid"
120 PRINT "MEDIA_TYPE=";PEEK32(&HF2310)

200 REM Allocate aligned guest buffers and select the 640 by 480 RGBA mode.
210 FB=MEMALLOC(1228800,4096):BB=MEMALLOC(1228800,4096):TX=MEMALLOC(2097152,4096):SR=MEMALLOC(235520,4096)
220 ST=2560:TS=4096:SW=640:SH=92
230 TW=1024:TH=512:OX=192:OY=210
240 FP=65536:HW=320:HH=240:CU=512:CV=256
250 POKE32 &HF0004,0:POKE32 &HF0080,0:POKE32 &HF0084,FB
260 POKE32 &HF0000,1

300 REM BLOAD places the raw RGBA rows in guest RAM; ST remains their byte stride.
310 BLOAD "sdk/examples/assets/splash_640x92.rgba",SR

350 REM T moves the row wobble; A rotates; Z changes the affine scale.
360 T=0:A=0:Z=0

400 REM Rebuild TX every frame so Mode7 samples the current wobble. Its dimensions
401 REM are powers of two, matching the 1023 and 511 coordinate masks below.
410 BLIT FILL TX,TW,TH,&H00000000,TS
420 FOR Y=0 TO 91
430 DY=OY+Y
440 X=OX+INT(24*SIN(T+Y*0.12))
450 DX=X:SX=0:CW=640
460 IF DX<0 THEN SX=0-DX:CW=640-SX:DX=0
470 IF DX+CW>TW THEN CW=TW-DX
480 IF CW<=0 THEN GOTO 520
490 SA=SR+Y*ST+SX*4
500 DA=TX+DY*TS+DX*4
510 BLIT COPY SA,DA,CW,1,ST,TS
520 NEXT Y

600 REM Mode7 consumes 16.16 origin and step vectors. BB isolates the render from FB.
610 SC=1.7+SIN(Z)*0.9
620 CA=COS(A)/SC:SA=SIN(A)/SC
630 DC=INT(CA*FP):DS=INT(SA*FP)
640 U0=INT((CU-HW*CA+HH*SA)*FP)
650 V0=INT((CV-HW*SA-HH*CA)*FP)
660 BLIT MODE7 TX,BB,640,480,U0,V0,DC,DS,0-DS,DC,1023,511,TS,ST

700 REM Present after rendering, then wrap phase values to retain useful precision.
710 BLIT MEMCOPY BB,FB,1228800
720 VSYNC
730 T=T+0.08:IF T>6.28318 THEN T=T-6.28318
740 A=A+0.025:IF A>6.28318 THEN A=A-6.28318
750 Z=Z+0.035:IF Z>6.28318 THEN Z=Z-6.28318
760 GOTO 400

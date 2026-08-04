1 REM VOODOO MEGA DEMO: IE64 BASIC command-stream rendering on the Voodoo device
2 REM Start with `go run . -basic -file-root .`, then LOAD this guest source and RUN.
3 REM It reads Reggae_2.sid. Read allocation and MMIO constants, initial scene setup,
4 REM the per-frame command construction, then the table-building subroutines.
5 REM CommandBuffer stores register-value pairs in guest RAM so the program can submit
6 REM each frame as one Voodoo command stream rather than interleaving scene work and MMIO.
10 REM Allocate tables, scene state, SID data and the Voodoo command stream in guest RAM.
11 SineTable=MEMALLOC(4096,4096):ProjectionTable=MEMALLOC(12288,4096):StarData=MEMALLOC(4096,4096):MessageData=MEMALLOC(4096,4096):SidData=MEMALLOC(8192,4096):CommandBuffer=MEMALLOC(524288,4096)
15 AnimationTables=MEMALLOC(8192,4096):GlyphSpans=MEMALLOC(16384,4096):ProjectionResults=MEMALLOC(2883584,4096)
20 QuarterSine=SineTable+2048:FontMasks=SineTable+3000:FontData=SineTable+3072:TwistXTable=AnimationTables:TwistYTable=AnimationTables+1024:WobbleTable=AnimationTables+2048:RainbowRedTable=AnimationTables+3072:RainbowGreenTable=AnimationTables+4096:RainbowBlueTable=AnimationTables+5120
30 ScreenWidth=640:ScreenHeight=480:ScreenCentreX=320:ScreenCentreY=240:TunnelRadius=150:NearPlane=80:FarPlane=1000:FocalLength=200:TwistAmplitude=120:WorldOffset=600:ProjectionProductBias=360000:ProjectionResultBias=2048
40 ProjectionMaxDepth=2304:ScrollSpeed=3:CharacterWidthPixels=24:PixelSize=4:ShadowOffset=2:ScrollBaseY=340:WobbleAmplitude=50:MaxVisibleCharacters=24:MessageLength=217
50 VoodooEnable=&HF8004:VoodooVideoDimensions=&HF8214:VoodooFbzMode=&HF8110:VoodooClipLeftRight=&HF8118:VoodooClipTopBottom=&HF811C:VoodooColourPath=&HF8104:VoodooClearColour=&HF81D8:VoodooFastFill=&HF8124:VoodooSwapBuffer=&HF8128
60 VertexAX=&HF8008:VertexAY=&HF800C:VertexBX=&HF8010:VertexBY=&HF8014:VertexCX=&HF8018:VertexCY=&HF801C:StartRed=&HF8020:StartGreen=&HF8024:StartBlue=&HF8028:StartDepth=&HF802C:StartAlpha=&HF8030:TriangleCommand=&HF8080:ColourSelect=&HF8088
65 CommandPointerRegister=&HF833C:CommandCountRegister=&HF8340:CommandSubmitRegister=&HF8344
70 REM Configure the Voodoo target before submitting clear, triangle or swap commands.
71 POKE32 VoodooEnable,1:POKE32 VoodooVideoDimensions,&H028001E0:POKE32 VoodooFbzMode,&H0770:POKE32 VoodooClipLeftRight,640:POKE32 VoodooClipTopBottom,480:POKE32 VoodooColourPath,0
75 BLOAD "sdk/examples/assets/music/Reggae_2.sid",SidData:POKE32 &HF0E20,SidData:POKE32 &HF0E24,4790:POKE32 &HF0E28,5
80 POKE32 VoodooClearColour,&HFF040410:POKE32 VoodooFastFill,0:GOSUB 3000:POKE32 VoodooSwapBuffer,1
85 FrameCounter=0:ScrollCharacter=0:ScrollPixelOffset=0:BaseWobblePhase=0:WobbleRemainder=0:RandomSeed=54321
90 FOR DataIndex=0 TO 63:READ DataValue:POKE8 QuarterSine+DataIndex,DataValue:NEXT
100 FOR DataIndex=0 TO 4:READ DataValue:POKE8 FontMasks+DataIndex,DataValue:NEXT
110 FOR DataIndex=0 TO 447:READ DataValue:POKE8 FontData+DataIndex,DataValue:NEXT
120 FOR DataIndex=0 TO 216:READ DataValue:POKE8 MessageData+DataIndex,DataValue:NEXT
130 GOSUB 6000:GOSUB 6100:GOSUB 6600:GOSUB 6400:GOSUB 6500:GOSUB 6200
140 REM The asset is staged before table construction; the following label begins frames.
200 REM Start a new frame: clear the back buffer, queue the background, then reset count.
201 POKE32 VoodooClearColour,&HFF040410:POKE32 VoodooFastFill,0:GOSUB 3000:CommandCount=0
210 AnimationPhase=FrameCounter AND 255:TunnelOffsetX=PEEK32(TwistXTable+AnimationPhase*4):TunnelOffsetY=PEEK32(TwistYTable+AnimationPhase*4)
230 FOR StarIndex=0 TO 255
240 StarPointer=StarData+StarIndex*16:StarAngle=PEEK32(StarPointer):StarRadius=PEEK32(StarPointer+4):StarDepth=PEEK32(StarPointer+8):StarSpeed=PEEK32(StarPointer+12)
250 StarDepth=StarDepth-StarSpeed:IF StarDepth>NearPlane THEN 310
260 GOSUB 5800:StarAngle=RandomValue AND 255
270 GOSUB 5800:StarRadius=(((RandomValue AND 255)*TunnelRadius) >> 8)+15
280 GOSUB 5800:StarDepth=(RandomValue AND 511)+FarPlane
290 GOSUB 5800:StarSpeed=(RandomValue AND 7)+2
310 POKE32 StarPointer,StarAngle:POKE32 StarPointer+4,StarRadius:POKE32 StarPointer+8,StarDepth:POKE32 StarPointer+12,StarSpeed
320 StarCosine=PEEK32(SineTable+((StarAngle+64) AND 255)*4):StarSine=PEEK32(SineTable+StarAngle*4)
340 IF StarDepth<10 THEN 520
330 StarLocalX=((StarCosine*StarRadius) >> 6)+WorldOffset+TunnelOffsetX+128:StarLocalY=((StarSine*StarRadius) >> 6)+WorldOffset+TunnelOffsetY+128
350 ProjectionScale=PEEK32(ProjectionTable+StarDepth*4):ProjectionProductX=StarLocalX*ProjectionScale:ProjectionProductY=StarLocalY*ProjectionScale
360 ProjectionOffsetBase=((127*StarRadius) >> 6)+WorldOffset+128:ProjectionOffset=(ProjectionOffsetBase*ProjectionScale) >> 8:ScreenX=(ProjectionProductX >> 8)-ProjectionOffset+ScreenCentreX:ScreenY=(ProjectionProductY >> 8)-ProjectionOffset+ScreenCentreY
370 IF ScreenX<0 THEN 520
371 IF ScreenX>=ScreenWidth THEN 520
372 IF ScreenY<0 THEN 520
373 IF ScreenY>=ScreenHeight THEN 520
380 IF StarDepth<150 THEN 382
381 GOTO 390
382 TriangleSize=64:ColourRed=&H1000:ColourGreen=&H1000:ColourBlue=&H1000:GOTO 440
390 IF StarDepth<240 THEN 392
391 GOTO 400
392 TriangleSize=48:ColourRed=&H1000:ColourGreen=&H0B00:ColourBlue=&H0400:GOTO 440
400 IF StarDepth<380 THEN 402
401 GOTO 410
402 TriangleSize=32:ColourRed=&H0800:ColourGreen=&H1000:ColourBlue=&H1000:GOTO 440
410 IF StarDepth<620 THEN 412
411 GOTO 420
412 TriangleSize=24:ColourRed=&H0C00:ColourGreen=&H0500:ColourBlue=&H1000:GOTO 440
420 IF StarDepth<900 THEN 422
421 GOTO 430
422 TriangleSize=16:ColourRed=&H0400:ColourGreen=&H0800:ColourBlue=&H1000:GOTO 440
430 TriangleSize=10:ColourRed=&H0300:ColourGreen=&H0500:ColourBlue=&H0A00
440 REM Queue colour and geometry writes; the device consumes them at line 530.
450 FixedX=ScreenX*16:FixedY=ScreenY*16
460 GOSUB 5600
520 NEXT StarIndex
530 REM Finish scrolltext, submit the contiguous command stream, then request a swap.
531 GOSUB 4000:POKE32 CommandPointerRegister,CommandBuffer:POKE32 CommandCountRegister,CommandCount:POKE32 CommandSubmitRegister,2
540 POKE32 VoodooSwapBuffer,1
550 FrameCounter=FrameCounter+1:ScrollPixelOffset=ScrollPixelOffset+ScrollSpeed:WobbleRemainder=WobbleRemainder+1-ScrollSpeed:IF ScrollPixelOffset<32 THEN 555
551 ScrollPixelOffset=ScrollPixelOffset-32:ScrollCharacter=ScrollCharacter+1:WobbleRemainder=WobbleRemainder+32:IF ScrollCharacter>=MessageLength THEN ScrollCharacter=0
555 GOSUB 6700
560 GOTO 200
3000 POKE32 ColourSelect,0:POKE32 StartRed,&H0200:POKE32 StartGreen,&H0200:POKE32 StartBlue,&H0800:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3010 POKE32 ColourSelect,1:POKE32 StartRed,&H0800:POKE32 StartGreen,&H0200:POKE32 StartBlue,&H1000:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3020 POKE32 ColourSelect,2:POKE32 StartRed,0:POKE32 StartGreen,&H0800:POKE32 StartBlue,&H1000:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3030 POKE32 VertexAX,0:POKE32 VertexAY,0:POKE32 VertexBX,10240:POKE32 VertexBY,0:POKE32 VertexCX,0:POKE32 VertexCY,7680:POKE32 TriangleCommand,0
3040 POKE32 ColourSelect,0:POKE32 StartRed,&H0800:POKE32 StartGreen,&H0200:POKE32 StartBlue,&H1000:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3050 POKE32 ColourSelect,1:POKE32 StartRed,&H0200:POKE32 StartGreen,0:POKE32 StartBlue,&H0800:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3060 POKE32 ColourSelect,2:POKE32 StartRed,0:POKE32 StartGreen,&H0800:POKE32 StartBlue,&H1000:POKE32 StartAlpha,&H1000:POKE32 StartDepth,&HF000
3070 POKE32 VertexAX,10240:POKE32 VertexAY,0:POKE32 VertexBX,10240:POKE32 VertexBY,7680:POKE32 VertexCX,0:POKE32 VertexCY,7680:POKE32 TriangleCommand,0:RETURN
4000 ShadowPass=1:GOSUB 4100:ShadowPass=0:GOSUB 4100:RETURN
4100 IF ShadowPass=1 THEN StartDepthValue=&H2400
4101 IF ShadowPass=0 THEN StartDepthValue=&H2000
4102 GOSUB 5690
4120 FOR CharacterIndex=0 TO 23
4130 MessageIndex=ScrollCharacter+CharacterIndex:IF MessageIndex>=MessageLength THEN MessageIndex=MessageIndex-MessageLength
4140 CharacterCode=PEEK(MessageData+MessageIndex):IF CharacterCode=0 THEN 4560
4150 CharacterBaseX=CharacterIndex*CharacterWidthPixels-ScrollPixelOffset:WobblePhase=(BaseWobblePhase+CharacterIndex*3) AND 255:CharacterBaseY=PEEK32(WobbleTable+WobblePhase*4)
4170 FontIndex=CharacterCode-32:IF FontIndex<0 THEN 4550
4171 IF FontIndex>=64 THEN 4550
4190 IF ShadowPass=0 THEN 4220
4200 TextRed=&H0100:TextGreen=&H0100:TextBlue=&H0180:GOSUB 5680:GOTO 4260
4220 ColourPhase=((CharacterIndex*32+FrameCounter)*4) AND 255:TextRed=PEEK32(RainbowRedTable+ColourPhase*4):TextGreen=PEEK32(RainbowGreenTable+ColourPhase*4):TextBlue=PEEK32(RainbowBlueTable+ColourPhase*4):GOSUB 5680
4260 FOR FontRowIndex=0 TO 6:SpanRecord=GlyphSpans+(FontIndex*7+FontRowIndex)*28:SpanCount=PEEK32(SpanRecord)
4270 FOR SpanIndex=0 TO 2:IF SpanIndex>=SpanCount THEN 4540
4280 SpanStart=PEEK32(SpanRecord+4+SpanIndex*8):SpanWidth=PEEK32(SpanRecord+8+SpanIndex*8)
4330 RectangleX=CharacterBaseX+SpanStart*PixelSize:RectangleY=CharacterBaseY+FontRowIndex*PixelSize:RectangleWidth=SpanWidth*PixelSize
4340 IF ShadowPass=1 THEN 4342
4341 GOTO 4350
4342 RectangleX=RectangleX+ShadowOffset:RectangleY=RectangleY+ShadowOffset
4350 IF RectangleX<0 THEN 4540
4351 IF RectangleX>=ScreenWidth THEN 4540
4352 IF RectangleY<0 THEN 4540
4353 IF RectangleY>=ScreenHeight THEN 4540
4360 FixedRectangleX=RectangleX*16:FixedRectangleY=RectangleY*16:FixedRectangleWidth=RectangleWidth*16
4370 GOSUB 5650
4540 NEXT SpanIndex
4541 NEXT FontRowIndex
4550 NEXT CharacterIndex
4560 RETURN
5600 IF CommandCount>65524 THEN RETURN
5601 CommandWritePointer=CommandBuffer+CommandCount*8
5602 POKE32 CommandWritePointer,StartRed:POKE32 CommandWritePointer+4,ColourRed:POKE32 CommandWritePointer+8,StartGreen:POKE32 CommandWritePointer+12,ColourGreen:POKE32 CommandWritePointer+16,StartBlue:POKE32 CommandWritePointer+20,ColourBlue:POKE32 CommandWritePointer+24,StartAlpha:POKE32 CommandWritePointer+28,&H1000:POKE32 CommandWritePointer+32,StartDepth:POKE32 CommandWritePointer+36,&H8000
5610 POKE32 CommandWritePointer+40,VertexAX:POKE32 CommandWritePointer+44,FixedX:POKE32 CommandWritePointer+48,VertexAY:POKE32 CommandWritePointer+52,FixedY-TriangleSize:POKE32 CommandWritePointer+56,VertexBX:POKE32 CommandWritePointer+60,FixedX-TriangleSize:POKE32 CommandWritePointer+64,VertexBY:POKE32 CommandWritePointer+68,FixedY+TriangleSize
5611 POKE32 CommandWritePointer+72,VertexCX:POKE32 CommandWritePointer+76,FixedX+TriangleSize:POKE32 CommandWritePointer+80,VertexCY:POKE32 CommandWritePointer+84,FixedY+TriangleSize:POKE32 CommandWritePointer+88,TriangleCommand:POKE32 CommandWritePointer+92,0:CommandCount=CommandCount+12:RETURN
5650 IF CommandCount>65522 THEN RETURN
5651 CommandWritePointer=CommandBuffer+CommandCount*8
5652 POKE32 CommandWritePointer,VertexAX:POKE32 CommandWritePointer+4,FixedRectangleX:POKE32 CommandWritePointer+8,VertexAY:POKE32 CommandWritePointer+12,FixedRectangleY:POKE32 CommandWritePointer+16,VertexBX:POKE32 CommandWritePointer+20,FixedRectangleX+FixedRectangleWidth:POKE32 CommandWritePointer+24,VertexBY:POKE32 CommandWritePointer+28,FixedRectangleY
5653 POKE32 CommandWritePointer+32,VertexCX:POKE32 CommandWritePointer+36,FixedRectangleX:POKE32 CommandWritePointer+40,VertexCY:POKE32 CommandWritePointer+44,FixedRectangleY+64:POKE32 CommandWritePointer+48,TriangleCommand:POKE32 CommandWritePointer+52,0
5660 POKE32 CommandWritePointer+56,VertexAX:POKE32 CommandWritePointer+60,FixedRectangleX+FixedRectangleWidth:POKE32 CommandWritePointer+64,VertexAY:POKE32 CommandWritePointer+68,FixedRectangleY:POKE32 CommandWritePointer+72,VertexBX:POKE32 CommandWritePointer+76,FixedRectangleX+FixedRectangleWidth:POKE32 CommandWritePointer+80,VertexBY:POKE32 CommandWritePointer+84,FixedRectangleY+64
5661 POKE32 CommandWritePointer+88,VertexCX:POKE32 CommandWritePointer+92,FixedRectangleX:POKE32 CommandWritePointer+96,VertexCY:POKE32 CommandWritePointer+100,FixedRectangleY+64:POKE32 CommandWritePointer+104,TriangleCommand:POKE32 CommandWritePointer+108,0:CommandCount=CommandCount+14:RETURN
5680 IF CommandCount>65533 THEN RETURN
5681 CommandWritePointer=CommandBuffer+CommandCount*8:POKE32 CommandWritePointer,StartRed:POKE32 CommandWritePointer+4,TextRed:POKE32 CommandWritePointer+8,StartGreen:POKE32 CommandWritePointer+12,TextGreen:POKE32 CommandWritePointer+16,StartBlue:POKE32 CommandWritePointer+20,TextBlue:CommandCount=CommandCount+3:RETURN
5690 IF CommandCount>65534 THEN RETURN
5691 CommandWritePointer=CommandBuffer+CommandCount*8:POKE32 CommandWritePointer,StartAlpha:POKE32 CommandWritePointer+4,&H1000:POKE32 CommandWritePointer+8,StartDepth:POKE32 CommandWritePointer+12,StartDepthValue:CommandCount=CommandCount+2:RETURN
5800 RandomSeed=(RandomSeed*1664525+1013904223) AND &HFFFFFFFF
5810 RandomValue=RandomSeed:RETURN
5900 WorkingValue=WorkingValue AND 255:WorkingValue=PEEK32(SineTable+WorkingValue*4):RETURN
6000 FOR DataIndex=0 TO 255:Quadrant=INT(DataIndex/64):QuarterIndex=DataIndex AND 63:IF (Quadrant AND 1)<>0 THEN QuarterIndex=63-QuarterIndex
6010 DataValue=PEEK(QuarterSine+QuarterIndex):IF (Quadrant AND 2)=0 THEN 6012
6011 DataValue=127-DataValue:GOTO 6020
6012 DataValue=DataValue+127
6020 POKE32 SineTable+DataIndex*4,DataValue
6021 NEXT
6022 RETURN
6100 FOR DataIndex=0 TO ProjectionMaxDepth:IF DataIndex=0 THEN 6102
6101 DataValue=INT((FocalLength*256)/DataIndex):GOTO 6110
6102 DataValue=0
6110 POKE32 ProjectionTable+DataIndex*4,DataValue
6111 NEXT
6112 RETURN
6200 FOR DataIndex=0 TO 255:StarPointer=StarData+DataIndex*16
6210 GOSUB 5800:POKE32 StarPointer,RandomValue AND 255
6220 GOSUB 5800:POKE32 StarPointer+4,(((RandomValue AND 255)*TunnelRadius) >> 8)+15
6230 GOSUB 5800:POKE32 StarPointer+8,(RandomValue AND 2047)+NearPlane
6240 GOSUB 5800:POKE32 StarPointer+12,(RandomValue AND 7)+2
6250 NEXT
6260 RETURN
6400 FOR DataIndex=0 TO 255
6410 WorkingValue=DataIndex:GOSUB 5900:WorkingProduct=WorkingValue*TwistAmplitude*4:TwistValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias:WorkingProduct=WorkingValue*WobbleAmplitude*2:WobbleValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias:WorkingProduct=WorkingValue*16:RainbowValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias
6411 POKE32 TwistXTable+DataIndex*4,TwistValue-238:POKE32 WobbleTable+DataIndex*4,WobbleValue+ScrollBaseY-20:POKE32 RainbowRedTable+DataIndex*4,RainbowValue*256+&H0400
6420 WorkingProduct=DataIndex*3*128:WorkingValue=(PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias+64) AND 255:GOSUB 5900:WorkingProduct=WorkingValue*TwistAmplitude*4:TwistValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias:POKE32 TwistYTable+DataIndex*4,TwistValue-238
6430 WorkingValue=(DataIndex+85) AND 255:GOSUB 5900:WorkingProduct=WorkingValue*16:RainbowValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias:POKE32 RainbowGreenTable+DataIndex*4,RainbowValue*256+&H0400
6431 WorkingValue=(DataIndex+170) AND 255:GOSUB 5900:WorkingProduct=WorkingValue*16:RainbowValue=PEEK32(ProjectionResults+(ProjectionProductBias+WorkingProduct)*4)-ProjectionResultBias:POKE32 RainbowBlueTable+DataIndex*4,RainbowValue*256+&H0400
6440 NEXT
6450 RETURN
6500 FOR FontIndex=0 TO 63:FontPointer=FontData+FontIndex*7
6510 FOR FontRowIndex=0 TO 6:FontRowBits=PEEK(FontPointer+FontRowIndex):SpanRecord=GlyphSpans+(FontIndex*7+FontRowIndex)*28:SpanCount=0:FontColumn=0
6520 IF FontColumn>=5 THEN 6570
6530 FontMask=PEEK(FontMasks+FontColumn):IF (FontRowBits AND FontMask)=0 THEN FontColumn=FontColumn+1:GOTO 6520
6540 SpanStart=FontColumn:SpanWidth=0
6550 IF FontColumn>=5 THEN 6560
6551 FontMask=PEEK(FontMasks+FontColumn):IF (FontRowBits AND FontMask)=0 THEN 6560
6552 FontColumn=FontColumn+1:SpanWidth=SpanWidth+1:GOTO 6550
6560 POKE32 SpanRecord+4+SpanCount*8,SpanStart:POKE32 SpanRecord+8+SpanCount*8,SpanWidth:SpanCount=SpanCount+1:GOTO 6520
6570 POKE32 SpanRecord,SpanCount:NEXT FontRowIndex
6580 NEXT FontIndex
6590 RETURN
6600 ProjectionQuotient=0:ProjectionRemainder=0
6610 FOR ProjectionProduct=0 TO 360000
6620 POKE32 ProjectionResults+(ProjectionProductBias+ProjectionProduct)*4,ProjectionResultBias+ProjectionQuotient
6630 IF ProjectionProduct=0 THEN 6650
6640 NegativeProjection=0-ProjectionQuotient:IF ProjectionRemainder<>0 THEN NegativeProjection=NegativeProjection-1
6641 POKE32 ProjectionResults+(ProjectionProductBias-ProjectionProduct)*4,ProjectionResultBias+NegativeProjection
6650 ProjectionRemainder=ProjectionRemainder+1:IF ProjectionRemainder<256 THEN 6670
6660 ProjectionRemainder=0:ProjectionQuotient=ProjectionQuotient+1
6670 NEXT ProjectionProduct
6680 RETURN
6700 IF WobbleRemainder>=0 THEN 6720
6710 WobbleRemainder=WobbleRemainder+8:BaseWobblePhase=(BaseWobblePhase-1) AND 255:GOTO 6700
6720 IF WobbleRemainder<8 THEN RETURN
6730 WobbleRemainder=WobbleRemainder-8:BaseWobblePhase=(BaseWobblePhase+1) AND 255:GOTO 6720
8000 DATA 0,3,6,10,13,16,19,22,25,28,31,34,37,40,43,46
8010 DATA 49,51,54,57,60,62,65,68,70,73,75,78,80,82,85,87
8020 DATA 89,91,94,96,98,100,102,103,105,107,108,110,112,113,114,116
8030 DATA 117,118,119,120,121,122,123,124,124,125,125,126,126,127,127,127
8040 DATA 16,8,4,2,1
8050 DATA 0,0,0,0,0,0,0,4,4,4,4,0,4,0,10,10
8060 DATA 0,0,0,0,0,10,31,10,10,31,10,0,4,15,20,14
8070 DATA 5,30,4,24,25,2,4,8,19,3,8,20,20,8,21,18
8080 DATA 13,4,4,0,0,0,0,0,2,4,8,8,8,4,2,8
8090 DATA 4,2,2,2,4,8,0,4,21,14,21,4,0,0,4,4
8100 DATA 31,4,4,0,0,0,0,0,4,4,8,0,0,0,31,0
8110 DATA 0,0,0,0,0,0,0,4,0,1,2,2,4,8,8,16
8120 DATA 14,17,19,21,25,17,14,4,12,4,4,4,4,14,14,17
8130 DATA 1,6,8,16,31,14,17,1,6,1,17,14,2,6,10,18
8140 DATA 31,2,2,31,16,30,1,1,17,14,6,8,16,30,17,17
8150 DATA 14,31,1,2,4,8,8,8,14,17,17,14,17,17,14,14
8160 DATA 17,17,15,1,2,12,0,0,4,0,4,0,0,0,0,4
8170 DATA 0,4,4,8,2,4,8,16,8,4,2,0,0,31,0,31
8180 DATA 0,0,8,4,2,1,2,4,8,14,17,1,2,4,0,4
8190 DATA 14,17,23,21,23,16,14,14,17,17,31,17,17,17,30,17
8200 DATA 17,30,17,17,30,14,17,16,16,16,17,14,30,17,17,17
8210 DATA 17,17,30,31,16,16,30,16,16,31,31,16,16,30,16,16
8220 DATA 16,14,17,16,23,17,17,14,17,17,17,31,17,17,17,14
8230 DATA 4,4,4,4,4,14,1,1,1,1,17,17,14,17,18,20
8240 DATA 24,20,18,17,16,16,16,16,16,16,31,17,27,21,21,17
8250 DATA 17,17,17,25,21,19,17,17,17,14,17,17,17,17,17,14
8260 DATA 30,17,17,30,16,16,16,14,17,17,17,21,18,13,30,17
8270 DATA 17,30,20,18,17,14,17,16,14,1,17,14,31,4,4,4
8280 DATA 4,4,4,17,17,17,17,17,17,14,17,17,17,17,10,10
8290 DATA 4,17,17,17,21,21,27,17,17,17,10,4,10,17,17,17
8300 DATA 17,10,4,4,4,4,31,1,2,4,8,16,31,14,8,8
8310 DATA 8,8,8,14,16,8,8,4,2,2,1,14,2,2,2,2
8320 DATA 2,14,4,10,17,0,0,0,0,0,0,0,0,0,0,31
8330 DATA 32,32,32,32,32,73,78,84,85,73,84,73,79,78,32,69
8340 DATA 78,71,73,78,69,32,32,32,32,32,51,68,70,88,32,86
8350 DATA 79,79,68,79,79,32,84,87,73,83,84,73,78,71,32,83
8360 DATA 84,65,82,70,73,69,76,68,32,84,85,78,78,69,76,32
8370 DATA 32,32,32,32,67,79,68,69,58,32,73,69,51,50,32,82
8380 DATA 73,83,67,32,65,83,77,32,66,89,32,73,78,84,85,73
8390 DATA 84,73,79,78,32,32,32,32,32,32,77,85,83,73,67,58
8400 DATA 32,82,69,71,71,65,69,32,50,32,66,89,32,68,74,73
8410 DATA 78,78,32,40,54,53,48,50,32,43,32,83,73,68,41,32
8420 DATA 32,32,32,32,71,82,69,69,84,73,78,71,83,32,84,79
8430 DATA 32,65,76,76,32,68,69,77,79,83,67,69,78,69,82,83
8440 DATA 46,46,46,32,32,32,32,32,86,73,83,73,84,32,73,78
8450 DATA 84,85,73,84,73,79,78,83,85,66,83,89,78,84,72,46
8460 DATA 67,79,77,32,32,32,32,32,32

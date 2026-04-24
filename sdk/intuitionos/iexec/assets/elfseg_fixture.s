prog_elfseg:
    dc.b    0x7F, 0x45, 0x4C, 0x46, 0x02, 0x01, 0x01, 0x00
    ds.b    8
    dc.w    0x0002
    dc.w    0x4945
    dc.l    1
    dc.q    0x0000000000601000
    dc.q    0x0000000000000040
    dc.q    0
    dc.l    0
    dc.w    64
    dc.w    56
    dc.w    2
    dc.w    0
    dc.w    0
    dc.w    0

    ; Program header 0: RX segment
    dc.l    1
    dc.l    5
    dc.q    0x0000000000001000
    dc.q    0x0000000000601000
    dc.q    0x0000000000601000
    dc.q    4
    dc.q    0x0000000000001000
    dc.q    0x0000000000001000

    ; Program header 1: RW segment
    dc.l    1
    dc.l    6
    dc.q    0x0000000000002000
    dc.q    0x0000000000602000
    dc.q    0x0000000000602000
    dc.q    4
    dc.q    0x0000000000001000
    dc.q    0x0000000000001000

    ; Pad from 0xB0 to 0x1000
    ds.b    3920
    dc.b    0x11, 0x22, 0x33, 0x44
    ; Pad from 0x1004 to 0x2000
    ds.b    4092
    dc.b    0x55, 0x66, 0x77, 0x88
prog_elfseg_iosm:
prog_elfseg_end:

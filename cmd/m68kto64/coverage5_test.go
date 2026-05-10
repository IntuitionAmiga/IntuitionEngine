package main

import (
	"strings"
	"testing"
)

// Wide grid sweep — exercise many addressing modes × sizes × mnemonics for
// branch coverage on the per-mnemonic and shadow paths.
func TestGridSweep(t *testing.T) {
	cases := []string{
		// MOVE / MOVEA grid.
		"\tmove.b d0,d1\n",
		"\tmove.w d0,d1\n",
		"\tmove.l d0,d1\n",
		"\tmove.b #1,d1\n",
		"\tmove.w #1,d1\n",
		"\tmove.l #1,d1\n",
		"\tmove.l (a0),(a1)\n",
		"\tmove.l 8(a0),(a1)+\n",
		"\tmove.l -(a0),8(a1)\n",
		"\tmove.b (a0)+,d0\n",
		"\tmove.w -(a0),d0\n",
		"\tmove.l (8,a0,d0.w),d1\n",
		"\tmovea.l #100,a0\n",
		"\tmovea.w (a0),a1\n",
		"\tmovea.l 8(a0),a1\n",
		// ALU grid.
		"\tadd.b d0,d1\n",
		"\tadd.w (a0),d1\n",
		"\tadd.l d0,(a0)\n",
		"\tadd.l #5,d0\n",
		"\tsub.b d0,d1\n",
		"\tsub.w (a0)+,d1\n",
		"\tand.l #-1,d0\n",
		"\tand.w d0,(a0)\n",
		"\tor.b #$F,d0\n",
		"\teor.l d0,d1\n",
		// Unary.
		"\tneg.b d0\n",
		"\tneg.w (a0)\n",
		"\tnot.l d0\n",
		"\tnot.w (a0)\n",
		"\tclr.b d0\n",
		"\tclr.w d0\n",
		"\tclr.l d0\n",
		"\tclr.b (a0)\n",
		// Shifts at all widths.
		"\tlsl.b #2,d0\n",
		"\tlsl.w #2,d0\n",
		"\tlsl.l #2,d0\n",
		"\tlsr.l d0,d1\n",
		"\tasr.w #1,d0\n",
		"\tasl.w #4,d0\n",
		"\trol.l #8,d0\n",
		"\tror.l d0,d1\n",
		// Misc.
		"\tswap d0\n",
		"\text.w d0\n",
		"\text.l d0\n",
		"\textb.l d0\n",
		// Branches (adjacent fuse).
		"\tcmp.b d0,d1\n\tbeq L\nL:\n\trts\n",
		"\tcmpi.b #5,d0\n\tbne L\nL:\n\trts\n",
		"\tcmp.w (a0),d0\n\tblt L\nL:\n\trts\n",
		"\ttst.b d0\n\tbpl L\nL:\n\trts\n",
		"\ttst.b (a0)\n\tbmi L\nL:\n\trts\n",
		// Standalone (shadow path).
		"\ttst.l d0\n\tnop\n\tbgt L\nL:\n\trts\n",
		"\ttst.l d0\n\tnop\n\tbcc L\nL:\n\trts\n",
		"\ttst.l d0\n\tnop\n\tbcs L\nL:\n\trts\n",
		"\ttst.l d0\n\tnop\n\tbvs L\nL:\n\trts\n",
		"\ttst.l d0\n\tnop\n\tbvc L\nL:\n\trts\n",
		// Scc all conditions on Dn.
		"\ttst.l d0\n\tseq d1\n",
		"\ttst.l d0\n\tsne d1\n",
		"\ttst.l d0\n\tsmi d1\n",
		"\ttst.l d0\n\tspl d1\n",
		"\ttst.l d0\n\tsvs d1\n",
		"\ttst.l d0\n\tsvc d1\n",
		"\ttst.l d0\n\tscs d1\n",
		"\ttst.l d0\n\tscc d1\n",
		"\ttst.l d0\n\tsgt d1\n",
		"\ttst.l d0\n\tsle d1\n",
		"\ttst.l d0\n\tshi d1\n",
		"\ttst.l d0\n\tsls d1\n",
		// DBcc all kinds.
		"\ttst.l d0\n\tdbeq d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbcs d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbvs d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbge d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbgt d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdble d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbhi d0,L\nL:\n\trts\n",
		"\ttst.l d0\n\tdbls d0,L\nL:\n\trts\n",
		"\tdbt d0,L\nL:\n\trts\n",
		// Phase B grid.
		"\tabcd -(a0),-(a1)\n",
		"\tsbcd -(a0),-(a1)\n",
		"\tnbcd d0\n",
		"\tpack -(a0),-(a1),#$0\n",
		"\tunpk -(a0),-(a1),#$0\n",
		"\tcas.w d0,d1,(a0)\n",
		"\tbfins d0,(a0){#0:#8}\n",
		"\tbfclr (a0){#4:#4}\n",
		"\tbfset (a0){#0:#1}\n",
		"\tbfchg (a0){#1:#2}\n",
		"\tbftst (a0){#0:#16}\n",
		"\tbfffo (a0){#0:#8},d1\n",
		"\ttrapv\n",
		"\tchk.l d0,d1\n",
		"\tmovec vbr,d0\n",
		// Bitfield with .l index.
		"\tbfextu d0{#0:#8},d1\n",
		"\tbfexts d0{#4:#4},d1\n",
		// Big multi-line program (push through many paths).
		"start:\n\tmove.l #10,d0\n\tmove.l #0,d1\nloop:\n\tadd.l d0,d1\n\tsub.l #1,d0\n\tbne loop\n\trts\n",
	}
	for _, src := range cases {
		c := NewConverter()
		c.noHeader = true
		out, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("%q: errors:\n%s", strings.ReplaceAll(strings.TrimSpace(src), "\n", " | "), out)
		}
	}
}

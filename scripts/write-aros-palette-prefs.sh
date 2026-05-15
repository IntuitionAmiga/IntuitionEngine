#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 OUTFILE" >&2
  exit 2
fi

out="$1"
mkdir -p "$(dirname "$out")"

perl -e '
use strict;
use warnings;

my ($out) = @ARGV;

# Mirrors workbench/prefs/palette/prefs.c for a NUMDRIPENS=12 build.
my @dri_pens = (1, 0, 1, 2, 1, 3, 1, 0, 2, 1, 2, 1);
my @pens = (@dri_pens, (0xffff) x (32 - scalar @dri_pens));

my @colors = (
  [0, 0xaaaa, 0xaaaa, 0xaaaa],
  [1, 0x0000, 0x0000, 0x0000],
  [2, 0xffff, 0xffff, 0xffff],
  [3, 0x6666, 0x8888, 0xbbbb],
  [4, 0xeeee, 0x4444, 0x4444],
  [5, 0x5555, 0xdddd, 0x5555],
  [6, 0x0000, 0x4444, 0xdddd],
  [7, 0xeeee, 0x9999, 0x0000],
);
push @colors, [0xffff, 0, 0, 0] while @colors < 32;

my $palt = pack("N4", 0, 0, 0, 0);
$palt .= pack("n*", @pens);
$palt .= pack("n*", @pens);
for my $color (@colors) {
  $palt .= pack("n4", @$color);
}

die "PALT size mismatch" unless length($palt) == 400;

my $prhd = pack("C C C4", 0, 0, 0, 0, 0, 0);
my $body = "PREF" . "PRHD" . pack("N", length($prhd)) . $prhd .
           "PALT" . pack("N", length($palt)) . $palt;
my $form = "FORM" . pack("N", length($body)) . $body;

open my $fh, ">:raw", $out or die "open $out: $!";
print {$fh} $form or die "write $out: $!";
close $fh or die "close $out: $!";
' "$out"

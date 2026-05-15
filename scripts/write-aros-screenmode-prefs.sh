#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 5 ]; then
    echo "usage: $0 OUTFILE [WIDTH HEIGHT DEPTH DISPLAY_ID]" >&2
    exit 2
fi

out="$1"
width="${2:-1920}"
height="${3:-1080}"
depth="${4:-32}"
display_id="${5:-0xffffffff}"

mkdir -p "$(dirname "$out")"

perl -e '
use strict;
use warnings;

my ($out, $width, $height, $depth, $display_id) = @ARGV;
for my $pair (["width", $width, 1, 65535], ["height", $height, 1, 65535], ["depth", $depth, 1, 65535]) {
    my ($name, $value, $min, $max) = @$pair;
    die "$name out of range\n" unless defined($value) && $value =~ /^\d+$/ && $value >= $min && $value <= $max;
}

my $mode = oct($display_id);
die "display_id out of range\n" unless $mode >= 0 && $mode <= 0xffffffff;

my $prhd = pack("CCN", 0, 0, 0);
my $scrm = pack("NNNNNnnnn", 0, 0, 0, 0, $mode, $width, $height, $depth, 0);
my $body = "PREF" . "PRHD" . pack("N", length($prhd)) . $prhd .
           "SCRM" . pack("N", length($scrm)) . $scrm;
my $file = "FORM" . pack("N", length($body)) . $body;

open my $fh, ">:raw", $out or die "open $out: $!\n";
print {$fh} $file;
close $fh or die "close $out: $!\n";
' "$out" "$width" "$height" "$depth" "$display_id"

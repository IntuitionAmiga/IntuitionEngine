---
title: "Lessons For Your Own IE Ports"
sources:
  - ../mk64-ie/IE-PORT-NOTES.md
  - ../mk64-ie/src/platform/platform.h
  - ../mk64-ie/ie/ie_memory_layout.h
  - ../mk64-ie/ie/ie_gfx_voodoo.c
  - ../mk64-ie/ie/ie_platform_audio.c
  - ../mk64-ie/ie/coproc/ie_coproc.c
  - ../mk64-ie/ie/coproc/tnl_proto.h
  - ../mk64-ie/ie/ie_gfx_svc_client.c
  - ../mk64-ie/ie/ie_audio_svc_client.c
  - ../mk64-ie/ie/coproc/gfx_svc_main.c
  - ../mk64-ie/ie/coproc/audio_svc_main.c
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 65 - Lessons For Your Own IE Ports

This part has used one large port as a case study. The point is not to
copy its tree. The point is to learn how to make Intuition Engine the
target machine.

## 65.1 Choose The Main CPU Deliberately

Pick the CPU that fits the programme's control flow, data layout, and
available code. In the case study, M68K is the game CPU. That does not
make M68K the right answer for every port.

IE64 is the natural choice for new native systems code. M68K may suit
portable C with `68020` and FPU assumptions. 6502 and Z80 are best when
the job is small, authentic, or tightly bound to their apertures. x86 is
available when its instruction model is the useful contract.

## 65.2 Keep Contracts Small

A good platform contract says what the game needs, not how the machine
does it. Time, input, save, asset, graphics, and audio contracts are
enough to keep the game core clean.

Avoid contracts that expose every register. That only moves the device
manual into the wrong file.

## 65.3 Use IE Hardware At The Right Level

Use Voodoo for triangles. Use VideoChip for 2D, blits, copper, and Mode
7. Use the audio engines for voices and mixing. Use the file device for
records and packed data. Use coprocessors for coarse shared-memory
work.

The port should look like it belongs to IE when it reaches the machine
boundary.

## 65.4 Build Pipelines With Ownership

A worker is useful when it owns a complete stage. In the case study the
main M68K owns game state, one M68K worker owns graphics translation,
IE64 owns batched vertex mathematics, another M68K worker owns sequence
processing, and Voodoo owns rasterisation.

Each boundary has one producer, one consumer, stable shared buffers, and
a bounded amount of work in flight. That is why the four processors can
overlap without sharing every mutable structure.

Keep a local path for an optional service. It gives startup a controlled
failure mode and gives testing a reference implementation of the same
contract.

## 65.5 Measure Before Rewriting

Before replacing a subsystem, count what it costs. A slow frame might be
triangle submission, texture streaming, audio writes, file activity,
matrix work, or something else entirely.

Use counters and frame groups to decide. Then change one contract at a
time and measure again.

## 65.6 Keep The Legal Boundary Clean

Do not mix generated commercial data into the guide, the source
contract, or the reader path. A clean port can still be a serious IE
case study when it documents architecture, protocol, and engineering
decisions rather than copied assets.

## 65.7 The General IE Lesson

Porting to IE is not preservation for its own sake. It is translation.
Keep the behaviour that matters, choose the IE devices and processors
that express it well, give every pipeline stage one clear owner, measure
the result, and leave the next programmer a clear machine contract.

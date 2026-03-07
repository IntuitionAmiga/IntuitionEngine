// m68k_musashi_bridge.c — Unity build of Musashi 68020 core for test oracle use.
// Build tag: musashi && m68k_test (via Go wrapper)

// Unity build: include Musashi sources directly.
// Order: softfloat first (float routines), then m68kops (opcode handlers),
// then m68kcpu (core, which #includes m68kfpu.c and m68kmmu.h).
#include "softfloat/softfloat.c"
#include "m68kops.c"
#include "m68kcpu.c"

#include <string.h>

#define MUSASHI_MEM_SIZE (16 * 1024 * 1024) // 16MB

static unsigned char musashi_mem[MUSASHI_MEM_SIZE];

// Musashi memory callbacks — big-endian (native 68020 byte order)
unsigned int m68k_read_memory_8(unsigned int addr) {
	if (addr >= MUSASHI_MEM_SIZE) return 0;
	return musashi_mem[addr];
}

unsigned int m68k_read_memory_16(unsigned int addr) {
	if (addr + 1 >= MUSASHI_MEM_SIZE) return 0;
	return (musashi_mem[addr] << 8) | musashi_mem[addr + 1];
}

unsigned int m68k_read_memory_32(unsigned int addr) {
	if (addr + 3 >= MUSASHI_MEM_SIZE) return 0;
	return ((unsigned int)musashi_mem[addr] << 24) |
	       ((unsigned int)musashi_mem[addr+1] << 16) |
	       ((unsigned int)musashi_mem[addr+2] << 8) |
	       (unsigned int)musashi_mem[addr+3];
}

void m68k_write_memory_8(unsigned int addr, unsigned int val) {
	if (addr >= MUSASHI_MEM_SIZE) return;
	musashi_mem[addr] = val & 0xFF;
}

void m68k_write_memory_16(unsigned int addr, unsigned int val) {
	if (addr + 1 >= MUSASHI_MEM_SIZE) return;
	musashi_mem[addr]     = (val >> 8) & 0xFF;
	musashi_mem[addr + 1] = val & 0xFF;
}

void m68k_write_memory_32(unsigned int addr, unsigned int val) {
	if (addr + 3 >= MUSASHI_MEM_SIZE) return;
	musashi_mem[addr]     = (val >> 24) & 0xFF;
	musashi_mem[addr + 1] = (val >> 16) & 0xFF;
	musashi_mem[addr + 2] = (val >> 8) & 0xFF;
	musashi_mem[addr + 3] = val & 0xFF;
}

// Wrapper functions for Go CGO bridge
void musashi_init(void) {
	m68k_init();
	m68k_set_cpu_type(M68K_CPU_TYPE_68030);
}

void musashi_reset(void) {
	m68k_pulse_reset();
}

int musashi_execute(int cycles) {
	return m68k_execute(cycles);
}

void musashi_set_reg(int reg, unsigned int val) {
	m68k_set_reg((m68k_register_t)reg, val);
}

unsigned int musashi_get_reg(int reg) {
	return m68k_get_reg(NULL, (m68k_register_t)reg);
}

void musashi_write_byte(unsigned int addr, unsigned char val) {
	if (addr < MUSASHI_MEM_SIZE) musashi_mem[addr] = val;
}

unsigned char musashi_read_byte(unsigned int addr) {
	if (addr >= MUSASHI_MEM_SIZE) return 0;
	return musashi_mem[addr];
}

unsigned int musashi_read_32(unsigned int addr) {
	return m68k_read_memory_32(addr);
}

void musashi_clear_mem(void) {
	memset(musashi_mem, 0, MUSASHI_MEM_SIZE);
}

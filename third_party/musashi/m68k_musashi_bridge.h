#ifndef M68K_MUSASHI_BRIDGE_H
#define M68K_MUSASHI_BRIDGE_H

void musashi_init(void);
void musashi_reset(void);
int musashi_execute(int cycles);
void musashi_set_reg(int reg, unsigned int val);
unsigned int musashi_get_reg(int reg);
void musashi_write_byte(unsigned int addr, unsigned char val);
unsigned char musashi_read_byte(unsigned int addr);
unsigned int musashi_read_32(unsigned int addr);
void musashi_clear_mem(void);

#endif /* M68K_MUSASHI_BRIDGE_H */

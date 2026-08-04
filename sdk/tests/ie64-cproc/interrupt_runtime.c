#include <ie64.h>

#define RESULT ((volatile uint64_t *)0x80000)
#define PAGE_SIZE 0x1000UL
#define PTE_PRESENT 0x01UL
#define PTE_READ 0x02UL
#define PTE_WRITE 0x04UL
#define PTE_EXECUTE 0x08UL

static unsigned char level0_storage[PAGE_SIZE + 128 * sizeof(uint64_t)];
static unsigned char level1_storage[PAGE_SIZE + 512 * sizeof(uint64_t)];
static unsigned char level2_storage[PAGE_SIZE + 512 * sizeof(uint64_t)];
static unsigned char level3_storage[PAGE_SIZE + 512 * sizeof(uint64_t)];
static unsigned char level4_storage[PAGE_SIZE + 512 * sizeof(uint64_t)];
static unsigned char level5_storage[PAGE_SIZE + 512 * sizeof(uint64_t)];
static uint64_t interrupt_stack[128];
static uint64_t *level0;
static uint64_t *level1;
static uint64_t *level2;
static uint64_t *level3;
static uint64_t *level4;
static uint64_t *level5;

static uint64_t *
page_align(void *storage)
{
	uintptr_t address = (uintptr_t)storage;

	address = (address + PAGE_SIZE - 1) & ~(uintptr_t)(PAGE_SIZE - 1);
	return (uint64_t *)address;
}

static uint64_t
table_entry(const void *table)
{
	return (uint64_t)(uintptr_t)table | PTE_PRESENT;
}

static void
install_identity_mapping(void)
{
	unsigned long page;

	level0 = page_align(level0_storage);
	level1 = page_align(level1_storage);
	level2 = page_align(level2_storage);
	level3 = page_align(level3_storage);
	level4 = page_align(level4_storage);
	level5 = page_align(level5_storage);
	level0[0] = table_entry(level1);
	level1[0] = table_entry(level2);
	level2[0] = table_entry(level3);
	level3[0] = table_entry(level4);
	level4[0] = table_entry(level5);
	for (page = 0; page < 512; ++page)
		level5[page] = page * PAGE_SIZE | PTE_PRESENT | PTE_READ | PTE_WRITE | PTE_EXECUTE;
}

void
interrupt_handler(void)
{
	*RESULT = 0x494e545250415353UL;
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 0);
	__builtin_ie64_eret();
}

int
main(void)
{
	*RESULT = 0x494e54524641494cUL;
	ie64_disable_interrupts();
	install_identity_mapping();
	__builtin_ie64_mtcr(IE64_CR_TRAP_VEC, (uint64_t)(uintptr_t)interrupt_handler);
	__builtin_ie64_mtcr(IE64_CR_PTBR, (uint64_t)(uintptr_t)level0);
	__builtin_ie64_mtcr(IE64_CR_KSP, (uint64_t)(uintptr_t)(interrupt_stack + 128));
	__builtin_ie64_mtcr(IE64_CR_INTR_VEC, (uint64_t)(uintptr_t)interrupt_handler);
	__builtin_ie64_tlbflush();
	__builtin_ie64_mtcr(IE64_CR_MMU_CTRL, 1);
	__builtin_ie64_mtcr(IE64_CR_TIMER_PERIOD, 1);
	__builtin_ie64_mtcr(IE64_CR_TIMER_COUNT, 1);
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 1);
	ie64_enable_interrupts();
	while (*RESULT != 0x494e545250415353UL)
		ie64_nop();
	__builtin_ie64_halt();
}

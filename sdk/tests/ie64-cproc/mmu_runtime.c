#include <ie64.h>
#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)
#define PT_BASE 0x83000UL
#define PAGE_SIZE 0x1000UL
#define PTE_FLAGS 0x1fUL

int
main(void)
{
	unsigned int level, page;
	volatile uint64_t *table;
	volatile uint64_t *mapped = (volatile uint64_t *)0x81000;

	/* Low virtual addresses use index zero until the final level. */
	for (level = 0; level < 5; ++level) {
		table = (volatile uint64_t *)(PT_BASE + level * PAGE_SIZE);
		table[0] = PT_BASE + (level + 1) * PAGE_SIZE | 1;
	}
	table = (volatile uint64_t *)(PT_BASE + 5 * PAGE_SIZE);
	for (page = 0; page < 0x9f; ++page)
		table[page] = (uint64_t)page * PAGE_SIZE | PTE_FLAGS;

	__builtin_ie64_mtcr(IE64_CR_PTBR, PT_BASE);
	__builtin_ie64_mtcr(IE64_CR_MMU_CTRL, 1);
	*mapped = 0x1122334455667788UL;
	if (*mapped != 0x1122334455667788UL) {
		*RESULT = 1;
		__builtin_ie64_halt();
	}
	__builtin_ie64_tlbinval((uintptr_t)mapped);
	if (*mapped != 0x1122334455667788UL) {
		*RESULT = 2;
		__builtin_ie64_halt();
	}
	__builtin_ie64_tlbflush();
	if (*mapped != 0x1122334455667788UL) {
		*RESULT = 3;
		__builtin_ie64_halt();
	}
	__builtin_ie64_mtcr(IE64_CR_MMU_CTRL, 0);
	*RESULT = 0x4d4d555041535345UL;
	return 0;
}

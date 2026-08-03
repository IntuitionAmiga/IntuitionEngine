#ifndef IE64_PLATFORM_H
#define IE64_PLATFORM_H

#include <stddef.h>
#include <stdint.h>

enum {
	IE64_OPEN_READ = 0x01,
	IE64_OPEN_WRITE = 0x02,
	IE64_OPEN_CREATE = 0x04,
	IE64_OPEN_TRUNCATE = 0x08,
	IE64_OPEN_APPEND = 0x10,
	IE64_OPEN_EXCLUSIVE = 0x20
};

enum {
	IE64_SEEK_SET = 0,
	IE64_SEEK_CUR = 1,
	IE64_SEEK_END = 2
};

int __ie64_console_read(void *buffer, size_t size, size_t *done);
int __ie64_console_write(const void *buffer, size_t size, size_t *done);
int __ie64_file_open(const char *path, unsigned int flags, uint64_t *handle);
int __ie64_file_read(uint64_t handle, void *buffer, size_t size, size_t *done);
int __ie64_file_write(uint64_t handle, const void *buffer, size_t size,
			      size_t *done);
int __ie64_file_seek(uint64_t handle, int64_t offset, int whence,
			     int64_t *position);
int __ie64_file_close(uint64_t handle);
int __ie64_file_remove(const char *path);
int __ie64_file_rename(const char *old_path, const char *new_path);
int __ie64_file_tmp(uint64_t *handle);
int __ie64_monotonic_ticks(uint64_t *ticks);
_Noreturn void __ie64_terminate(int status);

#endif

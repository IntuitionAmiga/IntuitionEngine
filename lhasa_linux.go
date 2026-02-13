//go:build linux && !headless

// lhasa_linux.go - LHA decompression using system liblhasa (Linux only).

package main

/*
#cgo pkg-config: liblhasa
#include <stdlib.h>
#include <lhasa.h>

static int lha_decompress_file(const char* path, unsigned char** out, size_t* out_len) {
	LHAInputStream* stream = lha_input_stream_from((char*)path);
	if (stream == NULL) {
		return 0;
	}

	LHAReader* reader = lha_reader_new(stream);
	if (reader == NULL) {
		lha_input_stream_free(stream);
		return 0;
	}

	LHAFileHeader* header = lha_reader_next_file(reader);
	if (header == NULL) {
		lha_reader_free(reader);
		lha_input_stream_free(stream);
		return 0;
	}

	if (header->length == 0) {
		lha_reader_free(reader);
		lha_input_stream_free(stream);
		return 0;
	}

	size_t length = (size_t) header->length;
	unsigned char* buffer = (unsigned char*) malloc(length);
	if (buffer == NULL) {
		lha_reader_free(reader);
		lha_input_stream_free(stream);
		return 0;
	}

	size_t total = 0;
	while (total < length) {
		size_t n = lha_reader_read(reader, buffer + total, length - total);
		if (n == 0) {
			break;
		}
		total += n;
	}

	lha_reader_free(reader);
	lha_input_stream_free(stream);

	*out = buffer;
	*out_len = total;
	return total > 0;
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

func DecompressLHAFile(path string) ([]byte, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var out *C.uchar
	var outLen C.size_t
	ok := C.lha_decompress_file(cPath, &out, &outLen)
	if ok == 0 || out == nil || outLen == 0 {
		return nil, fmt.Errorf("lha decompression failed")
	}
	defer C.free(unsafe.Pointer(out))

	data := C.GoBytes(unsafe.Pointer(out), C.int(outLen))
	return data, nil
}

// DecompressLHAData decompresses in-memory LHA data by writing to a temp file.
func DecompressLHAData(data []byte) ([]byte, error) {
	tmp, err := os.CreateTemp("", "lha-*.bin")
	if err != nil {
		return nil, fmt.Errorf("lha temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("lha temp write: %w", err)
	}
	tmp.Close()
	return DecompressLHAFile(tmpPath)
}

//go:build !headless

// audio_backend_alsa.go - ALSA audio output implementation

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

/*
#cgo LDFLAGS: -lasound
#cgo CFLAGS: -Ofast -march=native -mtune=native -flto
#include <alsa/asoundlib.h>
#include <stdlib.h>

static snd_pcm_t* openPCM(const char* device, int* err) {
    snd_pcm_t* handle;
    *err = snd_pcm_open(&handle, device, SND_PCM_STREAM_PLAYBACK, 0);
    return handle;
}

static int setupPCM(snd_pcm_t* handle, unsigned int rate) {
    snd_pcm_hw_params_t* params;
    int err;

    snd_pcm_hw_params_alloca(&params);
    err = snd_pcm_hw_params_any(handle, params);
    if (err < 0) return err;

    err = snd_pcm_hw_params_set_access(handle, params, SND_PCM_ACCESS_RW_INTERLEAVED);
    if (err < 0) return err;

    err = snd_pcm_hw_params_set_format(handle, params, SND_PCM_FORMAT_FLOAT);
    if (err < 0) return err;

    err = snd_pcm_hw_params_set_channels(handle, params, 1);
    if (err < 0) return err;

    err = snd_pcm_hw_params_set_rate(handle, params, rate, 0);
    if (err < 0) return err;

    err = snd_pcm_hw_params(handle, params);
    if (err < 0) return err;

    return snd_pcm_prepare(handle);
}

static int writePCM(snd_pcm_t* handle, float* buffer, int frames) {
    return snd_pcm_writei(handle, buffer, frames);
}

static void closePCM(snd_pcm_t* handle) {
    if (handle != NULL) {
        snd_pcm_drain(handle);
        snd_pcm_close(handle);
    }
}
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

type ALSAPlayer struct {
	handle  *C.snd_pcm_t
	started bool
	playing bool
	mutex   sync.Mutex
	samples []float32
}

func NewALSAPlayer() (*ALSAPlayer, error) {
	var err C.int
	handle := C.openPCM(C.CString("default"), &err)
	if err < 0 {
		return nil, fmt.Errorf("failed to open PCM device: %s", C.GoString(C.snd_strerror(err)))
	}

	if err = C.setupPCM(handle, C.uint(SAMPLE_RATE)); err < 0 {
		C.closePCM(handle)
		return nil, fmt.Errorf("failed to setup PCM: %s", C.GoString(C.snd_strerror(err)))
	}

	return &ALSAPlayer{
		handle:  handle,
		playing: false,
		started: false,
		samples: make([]float32, 4410),
	}, nil
}

func (ap *ALSAPlayer) SetupPlayer() {}

func (ap *ALSAPlayer) IsStarted() bool {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()
	return ap.started
}

func (ap *ALSAPlayer) Write(samples []float32) error {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if !ap.playing {
		return nil
	}

	copy(ap.samples, samples)
	frames := C.writePCM(ap.handle, (*C.float)(unsafe.Pointer(&ap.samples[0])), C.int(len(samples)))
	if frames < 0 {
		if frames == -C.EPIPE {
			C.snd_pcm_prepare(ap.handle)
			frames = C.writePCM(ap.handle, (*C.float)(unsafe.Pointer(&ap.samples[0])), C.int(len(samples)))
		}
		if frames < 0 {
			return fmt.Errorf("write failed: %s", C.GoString(C.snd_strerror(C.int(frames))))
		}
	}
	return nil
}

func (ap *ALSAPlayer) Start() {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if !ap.started {
		ap.started = true
		ap.playing = true
	}
}

func (ap *ALSAPlayer) Stop() {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if ap.playing {
		ap.playing = false
		ap.started = false
	}
}

func (ap *ALSAPlayer) Close() {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if ap.handle != nil {
		ap.playing = false
		ap.started = false
		C.closePCM(ap.handle)
		ap.handle = nil
	}
}

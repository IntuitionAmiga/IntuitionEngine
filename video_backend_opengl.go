// video_backend_opengl.go - OpenGL video backend for IntuitionEngine

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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

/*
#cgo windows LDFLAGS: -lopengl32
#cgo linux LDFLAGS: -lGL -lX11
#cgo darwin LDFLAGS: -framework OpenGL
#cgo CFLAGS: -Ofast -march=native -mtune=native -flto

#include <stdlib.h>
#include <time.h>

#ifdef __APPLE__
   #include <OpenGL/gl.h>
#else
   #include <GL/gl.h>
   #include <GL/glx.h>
   #include <X11/Xlib.h>
#endif

// OpenGL context management
static Display* display;
static Window window;
static GLXContext context;
static int initialized = 0;

static int initOpenGL(int width, int height) {
    display = XOpenDisplay(NULL);
    if (!display) {
        return -1;
    }

    // Choose appropriate visual
    static int visual_attribs[] = {
        GLX_RGBA,
        GLX_DEPTH_SIZE, 24,
        GLX_DOUBLEBUFFER,
        None
    };

    int screen = DefaultScreen(display);
    XVisualInfo* vi = glXChooseVisual(display, screen, visual_attribs);
    if (!vi) {
        XCloseDisplay(display);
        return -2;
    }

    // Create colormap
    Colormap cmap = XCreateColormap(display,
                                  RootWindow(display, vi->screen),
                                  vi->visual,
                                  AllocNone);

    // Create window
    XSetWindowAttributes swa;
    swa.colormap = cmap;
    swa.border_pixel = 0;
    swa.event_mask = StructureNotifyMask | ExposureMask;

    window = XCreateWindow(display,
                         RootWindow(display, vi->screen),
                         0, 0,                    // x, y position
                         width, height,           // width, height
                         0,                       // border width
                         vi->depth,              // depth
                         InputOutput,            // class
                         vi->visual,             // visual
                         CWBorderPixel|CWColormap|CWEventMask,
                         &swa);

    // Set window properties
    XStoreName(display, window, "IntuitionEngine Display");
    XMapWindow(display, window);

    // Create OpenGL context
    context = glXCreateContext(display, vi, NULL, GL_TRUE);
    if (!context) {
        XDestroyWindow(display, window);
        XCloseDisplay(display);
        return -3;
    }

    glXMakeCurrent(display, window, context);

    // Initialize OpenGL state
    glViewport(0, 0, width, height);
    glMatrixMode(GL_PROJECTION);
    glLoadIdentity();
    glOrtho(0, width, height, 0, -1, 1);
    glMatrixMode(GL_MODELVIEW);
    glLoadIdentity();

    glEnable(GL_TEXTURE_2D);
    glDisable(GL_DEPTH_TEST);
    glClearColor(0.0f, 0.0f, 0.0f, 1.0f);

    initialized = 1;
    return 0;
}

static void updateTexture(GLuint texID, unsigned char* pixels,
                         int width, int height) {
    if (!initialized) return;

    glBindTexture(GL_TEXTURE_2D, texID);
    glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA,
                 width, height, 0,
                 GL_RGBA, GL_UNSIGNED_BYTE, pixels);
}

static void renderFrame(GLuint texID, int width, int height) {
    if (!initialized) return;

    glClear(GL_COLOR_BUFFER_BIT);
    glLoadIdentity();

    glBindTexture(GL_TEXTURE_2D, texID);
    glBegin(GL_QUADS);
    glTexCoord2f(0.0f, 0.0f); glVertex2f(0.0f, 0.0f);
    glTexCoord2f(1.0f, 0.0f); glVertex2f(width, 0.0f);
    glTexCoord2f(1.0f, 1.0f); glVertex2f(width, height);
    glTexCoord2f(0.0f, 1.0f); glVertex2f(0.0f, height);
    glEnd();

    glXSwapBuffers(display, window);
}

static void cleanupOpenGL() {
    if (!initialized) return;

    glXMakeCurrent(display, None, NULL);
    glXDestroyContext(display, context);
    XDestroyWindow(display, window);
    XCloseDisplay(display);
    initialized = 0;
}

static int waitForVSync() {
    if (!initialized) return -1;

    // Use GLX_SGI_video_sync if available
    typedef int (*GLXGETSYNCVALUESPROC)(Display*, GLXDrawable, int64_t*, int64_t*, int64_t*);
    GLXGETSYNCVALUESPROC glXGetSyncValuesOML =
        (GLXGETSYNCVALUESPROC)glXGetProcAddress((const GLubyte*)"glXGetSyncValuesOML");

    if (glXGetSyncValuesOML) {
        int64_t ust, msc, sbc;
        glXGetSyncValuesOML(display, window, &ust, &msc, &sbc);
        return 0;
    }

    // Fallback: sleep for approximate vsync period
    struct timespec ts = {0, 16666667}; // ~60Hz
    nanosleep(&ts, NULL);
    return 0;
}
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

type OpenGLOutput struct {
	mutex         sync.RWMutex
	started       bool
	config        DisplayConfig
	textureID     uint32
	buffer        []byte
	frameCount    uint64
	vsyncChan     chan struct{}
	renderTicker  *time.Ticker
	done          chan struct{}
	dirtyRegions  map[int]DirtyRegion
	frameSnapshot FrameSnapshot
}

func NewOpenGLOutput() (VideoOutput, error) {
	output := &OpenGLOutput{
		config: DisplayConfig{
			Width:       640,
			Height:      480,
			Scale:       1,
			RefreshRate: 60,
			PixelFormat: PixelFormatRGBA,
			VSync:       true,
		},
		vsyncChan:    make(chan struct{}),
		done:         make(chan struct{}),
		dirtyRegions: make(map[int]DirtyRegion),
	}

	// Initialize OpenGL
	result := C.initOpenGL(C.int(output.config.Width),
		C.int(output.config.Height))
	if result != 0 {
		return nil, fmt.Errorf("failed to initialize OpenGL: %d", result)
	}

	// Create texture for frame buffer
	var texID C.GLuint
	C.glGenTextures(1, &texID)
	output.textureID = uint32(texID)

	C.glBindTexture(C.GL_TEXTURE_2D, texID)
	C.glTexParameteri(C.GL_TEXTURE_2D, C.GL_TEXTURE_MIN_FILTER, C.GL_NEAREST)
	C.glTexParameteri(C.GL_TEXTURE_2D, C.GL_TEXTURE_MAG_FILTER, C.GL_NEAREST)

	// Allocate frame buffer
	output.buffer = make([]byte, output.config.Width*output.config.Height*4)

	return output, nil
}

func (gl *OpenGLOutput) Start() error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if !gl.started {
		gl.renderTicker = time.NewTicker(time.Second /
			time.Duration(gl.config.RefreshRate))
		go gl.renderLoop()
		gl.started = true
	}
	return nil
}

func (gl *OpenGLOutput) Stop() error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if gl.started {
		gl.renderTicker.Stop()
		close(gl.done)
		gl.started = false
	}
	return nil
}

func (gl *OpenGLOutput) Close() error {
	gl.Stop()
	C.cleanupOpenGL()
	return nil
}

func (gl *OpenGLOutput) Clear(color uint32) error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	r := byte(color)
	g := byte(color >> 8)
	b := byte(color >> 16)
	a := byte(color >> 24)

	for i := 0; i < len(gl.buffer); i += 4 {
		gl.buffer[i] = r
		gl.buffer[i+1] = g
		gl.buffer[i+2] = b
		gl.buffer[i+3] = a
	}

	// Mark entire screen as dirty
	gl.dirtyRegions[0] = DirtyRegion{
		x: 0, y: 0,
		width:  gl.config.Width,
		height: gl.config.Height,
	}

	return nil
}

func (gl *OpenGLOutput) UpdateFrame(data []byte) error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if len(data) != len(gl.buffer) {
		return fmt.Errorf("invalid buffer size: got %d, expected %d",
			len(data), len(gl.buffer))
	}

	copy(gl.buffer, data)

	// Mark entire screen as dirty
	gl.dirtyRegions[0] = DirtyRegion{
		x: 0, y: 0,
		width:  gl.config.Width,
		height: gl.config.Height,
	}

	return nil
}

func (gl *OpenGLOutput) UpdateRegion(x, y, width, height int, pixels []byte) error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if x < 0 || y < 0 || x+width > gl.config.Width || y+height > gl.config.Height {
		return fmt.Errorf("region out of bounds: x=%d y=%d w=%d h=%d",
			x, y, width, height)
	}

	// Update the specified region
	for dy := 0; dy < height; dy++ {
		dstOffset := ((y+dy)*gl.config.Width + x) * 4
		srcOffset := dy * width * 4
		copy(gl.buffer[dstOffset:], pixels[srcOffset:srcOffset+width*4])
	}

	// Mark region as dirty
	regionKey := y*gl.config.Width + x
	gl.dirtyRegions[regionKey] = DirtyRegion{
		x: x, y: y,
		width:  width,
		height: height,
	}

	return nil
}

func (gl *OpenGLOutput) SetDisplayConfig(config DisplayConfig) error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if config.Width <= 0 || config.Height <= 0 || config.Scale <= 0 {
		return fmt.Errorf("invalid config dimensions")
	}

	// Recreate window if size changed
	if config.Width != gl.config.Width || config.Height != gl.config.Height {
		C.cleanupOpenGL()
		result := C.initOpenGL(C.int(config.Width), C.int(config.Height))
		if result != 0 {
			return fmt.Errorf("failed to reinitialize OpenGL: %d", result)
		}

		gl.buffer = make([]byte, config.Width*config.Height*4)
	}

	// Update refresh rate if changed
	if config.RefreshRate != gl.config.RefreshRate && gl.renderTicker != nil {
		gl.renderTicker.Stop()
		gl.renderTicker = time.NewTicker(time.Second /
			time.Duration(config.RefreshRate))
	}

	gl.config = config
	return nil
}

func (gl *OpenGLOutput) GetDisplayConfig() DisplayConfig {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()
	return gl.config
}

func (gl *OpenGLOutput) WaitForVSync() error {
	if !gl.config.VSync {
		return nil
	}

	C.waitForVSync()
	return nil
}

func (gl *OpenGLOutput) GetFrameCount() uint64 {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()
	return gl.frameCount
}

func (gl *OpenGLOutput) GetRefreshRate() int {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()
	return gl.config.RefreshRate
}

func (gl *OpenGLOutput) GetSnapshot() (FrameSnapshot, error) {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()

	snapshot := FrameSnapshot{
		Buffer:    make([]byte, len(gl.buffer)),
		Width:     gl.config.Width,
		Height:    gl.config.Height,
		Format:    gl.config.PixelFormat,
		Timestamp: time.Now(),
	}
	copy(snapshot.Buffer, gl.buffer)
	return snapshot, nil
}

func (gl *OpenGLOutput) IsStarted() bool {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()
	return gl.started
}

func (gl *OpenGLOutput) SupportsPalette() bool {
	return false
}

func (gl *OpenGLOutput) SupportsTextures() bool {
	return true
}

func (gl *OpenGLOutput) SupportsSprites() bool {
	return false
}

func (gl *OpenGLOutput) renderLoop() {
	for {
		select {
		case <-gl.done:
			return

		case <-gl.renderTicker.C:
			gl.mutex.Lock()

			// Update texture if there are dirty regions
			if len(gl.dirtyRegions) > 0 {
				C.updateTexture(
					C.GLuint(gl.textureID),
					(*C.uchar)(unsafe.Pointer(&gl.buffer[0])),
					C.int(gl.config.Width),
					C.int(gl.config.Height),
				)
				gl.dirtyRegions = make(map[int]DirtyRegion)
			}

			// Render frame
			C.renderFrame(
				C.GLuint(gl.textureID),
				C.int(gl.config.Width),
				C.int(gl.config.Height),
			)

			gl.frameCount++
			gl.mutex.Unlock()

			// Signal vsync
			select {
			case gl.vsyncChan <- struct{}{}:
			default:
			}
		}
	}
}

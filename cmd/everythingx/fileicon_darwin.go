//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework ImageIO -framework CoreFoundation

#import <AppKit/AppKit.h>
#import <ImageIO/ImageIO.h>
#include <stdlib.h>
#include <string.h>

static unsigned char* fileIconPNG(const char* path, int size, int* outLen) {
    @autoreleasepool {
        NSString *nsPath = [NSString stringWithUTF8String:path];
        NSImage *img = [[NSWorkspace sharedWorkspace] iconForFile:nsPath];
        if (!img) { *outLen = 0; return NULL; }

        NSRect rect = NSMakeRect(0, 0, size, size);
        CGImageRef cgImg = [img CGImageForProposedRect:&rect context:nil hints:nil];
        if (!cgImg) { *outLen = 0; return NULL; }

        CFMutableDataRef data = CFDataCreateMutable(kCFAllocatorDefault, 0);
        CGImageDestinationRef dest = CGImageDestinationCreateWithData(
            data, CFSTR("public.png"), 1, NULL);
        if (!dest) { CFRelease(data); *outLen = 0; return NULL; }

        CGImageDestinationAddImage(dest, cgImg, NULL);
        if (!CGImageDestinationFinalize(dest)) {
            CFRelease(dest); CFRelease(data);
            *outLen = 0; return NULL;
        }
        CFRelease(dest);

        *outLen = (int)CFDataGetLength(data);
        unsigned char *bytes = (unsigned char*)malloc(*outLen);
        memcpy(bytes, (const void*)CFDataGetBytePtr(data), *outLen);
        CFRelease(data);
        return bytes;
    }
}

static void freeBytes(unsigned char* p) { free(p); }
*/
import "C"
import "unsafe"

func getFileIconPNG(path string, size int) []byte {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var outLen C.int
	data := C.fileIconPNG(cpath, C.int(size), &outLen)
	if data == nil || outLen == 0 {
		return nil
	}
	result := C.GoBytes(unsafe.Pointer(data), outLen)
	C.freeBytes(data)
	return result
}

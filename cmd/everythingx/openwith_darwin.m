//go:build darwin

#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>
#include "openwith_darwin.h"

extern void goChooseAppDone(uintptr_t handle, char *appPath);

static NSString *pathString(const char *path) {
    return [[NSFileManager defaultManager]
        stringWithFileSystemRepresentation:path
                                    length:strlen(path)];
}

static char *copyString(NSString *s) {
    const char *utf8 = s ? [s UTF8String] : "";
    return strdup(utf8 ? utf8 : "");
}

static char *copyPath(NSURL *url) {
    const char *fs = [url fileSystemRepresentation];
    return strdup(fs ? fs : "");
}

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// appURLsForURL lists every app LaunchServices would offer for url, the same
// list as Finder's Open With.
static NSArray<NSURL *> *appURLsForURL(NSURL *url) {
    if (@available(macOS 12.0, *)) {
        return [[NSWorkspace sharedWorkspace] URLsForApplicationsToOpenURL:url];
    }
    CFArrayRef apps = LSCopyApplicationURLsForURL((CFURLRef)url, kLSRolesAll);
    return apps ? [(NSArray *)apps autorelease] : @[];
}

#pragma clang diagnostic pop

// exAppsForPath returns the apps that can open path, with the default marked.
// It returns NULL with *count 0 when there are none.
EXApp *exAppsForPath(const char *cpath, int *count) {
    *count = 0;
    @autoreleasepool {
        NSString *path = pathString(cpath);
        if (path == nil) {
            return NULL;
        }
        NSURL *url = [NSURL fileURLWithPath:path];
        NSWorkspace *ws = [NSWorkspace sharedWorkspace];
        NSURL *def = [[ws URLForApplicationToOpenURL:url] URLByStandardizingPath];

        NSMutableArray<NSURL *> *urls = [NSMutableArray array];
        NSMutableSet<NSString *> *seen = [NSMutableSet set];
        if (def != nil) {
            [urls addObject:def];
            [seen addObject:def.path];
        }
        for (NSURL *u in appURLsForURL(url)) {
            NSURL *std = [u URLByStandardizingPath];
            if (std.path != nil && ![seen containsObject:std.path]) {
                [urls addObject:std];
                [seen addObject:std.path];
            }
        }
        if (urls.count == 0) {
            return NULL;
        }

        EXApp *apps = calloc(urls.count, sizeof(EXApp));
        if (apps == NULL) {
            return NULL;
        }
        NSFileManager *fm = [NSFileManager defaultManager];
        int n = 0;
        for (NSURL *u in urls) {
            NSString *name = [fm displayNameAtPath:u.path];
            if ([name.pathExtension caseInsensitiveCompare:@"app"] == NSOrderedSame) {
                name = [name stringByDeletingPathExtension];
            }
            NSBundle *bundle = [NSBundle bundleWithURL:u];
            NSString *version = [bundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
            if (![version isKindOfClass:[NSString class]]) {
                version = nil;
            }
            apps[n].path = copyPath(u);
            apps[n].name = copyString(name);
            apps[n].version = copyString(version);
            apps[n].isDefault = (def != nil && [u isEqual:def]) ? 1 : 0;
            n++;
        }
        *count = n;
        return apps;
    }
}

void exFreeApps(EXApp *apps, int count) {
    for (int i = 0; i < count; i++) {
        free(apps[i].path);
        free(apps[i].name);
        free(apps[i].version);
    }
    free(apps);
}

// imagePNG renders icon at px by px and returns it as malloc'd PNG data, or
// NULL. The caller holds an autorelease pool.
static void *imagePNG(NSImage *icon, int px, int *length) {
    *length = 0;
    if (icon == nil) {
        return NULL;
    }
    NSBitmapImageRep *rep = [[[NSBitmapImageRep alloc]
        initWithBitmapDataPlanes:NULL
                      pixelsWide:px
                      pixelsHigh:px
                   bitsPerSample:8
                 samplesPerPixel:4
                        hasAlpha:YES
                        isPlanar:NO
                  colorSpaceName:NSCalibratedRGBColorSpace
                     bytesPerRow:0
                    bitsPerPixel:0] autorelease];
    if (rep == nil) {
        return NULL;
    }
    NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
    [NSGraphicsContext saveGraphicsState];
    [NSGraphicsContext setCurrentContext:ctx];
    ctx.imageInterpolation = NSImageInterpolationHigh;
    [icon drawInRect:NSMakeRect(0, 0, px, px)
            fromRect:NSZeroRect
           operation:NSCompositingOperationCopy
            fraction:1.0];
    [ctx flushGraphics];
    [NSGraphicsContext restoreGraphicsState];

    NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
    if (png.length == 0) {
        return NULL;
    }
    void *buf = malloc(png.length);
    if (buf == NULL) {
        return NULL;
    }
    memcpy(buf, png.bytes, png.length);
    *length = (int)png.length;
    return buf;
}

// exAppIconPNG renders the Finder icon of appPath at px by px and returns it as
// malloc'd PNG data, or NULL.
void *exAppIconPNG(const char *cpath, int px, int *length) {
    *length = 0;
    @autoreleasepool {
        NSString *path = pathString(cpath);
        if (path == nil) {
            return NULL;
        }
        return imagePNG([[NSWorkspace sharedWorkspace] iconForFile:path], px, length);
    }
}

// exTypeIconPNG renders the Finder icon for a kind of item rather than one
// file: kind is EXKindFolder, EXKindSymlink, or EXKindFile, whose type comes
// from ext (without the dot; empty for none). It returns malloc'd PNG data, or
// NULL.
void *exTypeIconPNG(const char *cext, int kind, int px, int *length) {
    *length = 0;
    @autoreleasepool {
        UTType *type = nil;
        switch (kind) {
        case EXKindFolder:
            type = UTTypeFolder;
            break;
        case EXKindSymlink:
            type = UTTypeSymbolicLink;
            break;
        default:
            if (cext != NULL && cext[0] != '\0') {
                type = [UTType typeWithFilenameExtension:[NSString stringWithUTF8String:cext]];
            }
        }
        if (type == nil) {
            type = UTTypeData;
        }
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForContentType:type];
        return imagePNG(icon, px, length);
    }
}

// exChooseApp shows an Open panel in /Applications for picking an app, like
// Finder's Open With > Other…, and reports the choice, or NULL when cancelled,
// to goChooseAppDone. The panel runs modeless on the main queue, which GLFW's
// event loop services, so the window keeps drawing while it is up.
void exChooseApp(uintptr_t handle) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSOpenPanel *panel = [[NSOpenPanel openPanel] retain];
        panel.canChooseFiles = YES;
        panel.canChooseDirectories = NO;
        panel.allowsMultipleSelection = NO;
        panel.treatsFilePackagesAsDirectories = NO;
        panel.directoryURL = [NSURL fileURLWithPath:@"/Applications" isDirectory:YES];
        panel.message = @"Choose an application to open the document.";
        panel.prompt = @"Open";
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        panel.allowedFileTypes = @[ @"app" ];
#pragma clang diagnostic pop
        [NSApp activateIgnoringOtherApps:YES];
        [panel beginWithCompletionHandler:^(NSModalResponse result) {
            char *chosen = NULL;
            if (result == NSModalResponseOK && panel.URLs.count > 0) {
                chosen = copyPath(panel.URLs.firstObject);
            }
            goChooseAppDone(handle, chosen);
            free(chosen);
            [panel release];
        }];
    });
}

#ifndef EVERYTHINGX_OPENWITH_DARWIN_H
#define EVERYTHINGX_OPENWITH_DARWIN_H

#include <stdint.h>

// EXApp is one application LaunchServices offers for a file. Every string is
// malloc'd UTF-8 (path in file-system representation) and freed by
// exFreeApps.
typedef struct {
    char *path;
    char *name;
    char *version;
    int isDefault;
} EXApp;

EXApp *exAppsForPath(const char *path, int *count);
void exFreeApps(EXApp *apps, int count);
void *exAppIconPNG(const char *appPath, int px, int *length);

// Kinds of item for exTypeIconPNG.
enum { EXKindFile = 0, EXKindFolder = 1, EXKindSymlink = 2 };
void *exTypeIconPNG(const char *ext, int kind, int px, int *length);
void exChooseApp(uintptr_t handle);

#endif

#import <Cocoa/Cocoa.h>
#if __has_include(<UniformTypeIdentifiers/UniformTypeIdentifiers.h>)
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#endif

static NSImage *GetWorkspaceIconForExtension(NSString *extension) {
    NSWorkspace *workspace = [NSWorkspace sharedWorkspace];

    if (@available(macOS 11.0, *)) {
#if __has_include(<UniformTypeIdentifiers/UniformTypeIdentifiers.h>)
        if ([extension length] > 0) {
            UTType *contentType = [UTType typeWithFilenameExtension:extension];
            if (contentType != nil) {
                return [workspace iconForContentType:contentType];
            }
        }

        return [workspace iconForContentType:UTTypeData];
#endif
    }

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
    return [workspace iconForFileType:extension];
#pragma clang diagnostic pop
}

const unsigned char *GetFileIconBytes(const char *pathC, size_t *length) {
    @autoreleasepool {
        if (pathC == NULL) return NULL;
        NSString *path = [NSString stringWithUTF8String:pathC];
        if ([path length] == 0) return NULL;
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:path];
        if (!icon) return NULL;

        CGImageRef cgRef = [icon CGImageForProposedRect:NULL context:nil hints:nil];
        if (!cgRef) return NULL;
        NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:cgRef];
        [rep setSize:[icon size]];
        NSData *pngData = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (!pngData) return NULL;

        *length = [pngData length];
        unsigned char *bytes = (unsigned char *)malloc(*length);
        memcpy(bytes, [pngData bytes], *length);
        return bytes;
    }
}

const unsigned char *GetFileTypeIconBytes(const char *extC, size_t *length) {
    @autoreleasepool {
        if (extC == NULL) return NULL;
        NSString *ext = [NSString stringWithUTF8String:extC];
        if ([ext hasPrefix:@"."]) {
            ext = [ext substringFromIndex:1];
        }
        NSImage *icon = GetWorkspaceIconForExtension(ext);
        if (!icon) return NULL;

        CGImageRef cgRef = [icon CGImageForProposedRect:NULL context:nil hints:nil];
        if (!cgRef) return NULL;
        NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:cgRef];
        [rep setSize:[icon size]];
        NSData *pngData = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (!pngData) return NULL;

        *length = [pngData length];
        unsigned char *bytes = (unsigned char *)malloc(*length);
        memcpy(bytes, [pngData bytes], *length);
        return bytes;
    }
}

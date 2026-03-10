#import <ApplicationServices/ApplicationServices.h>
#import <Cocoa/Cocoa.h>
#import <Foundation/Foundation.h>
#import <dlfcn.h>
#import <stdlib.h>
#import <string.h>

typedef struct {
  unsigned char *data;
  int len;
  char *err;
} CapturePNGResult;

static char *copy_error_message(const char *message) {
  if (message == NULL) {
    return NULL;
  }

  size_t len = strlen(message) + 1;
  char *copy = (char *)malloc(len);
  if (copy == NULL) {
    return NULL;
  }

  memcpy(copy, message, len);
  return copy;
}

typedef CGImageRef (*CGDisplayCreateImageFunc)(CGDirectDisplayID displayID);

static CGImageRef create_display_image(CGDirectDisplayID displayID) {
  static CGDisplayCreateImageFunc fn = NULL;
  static dispatch_once_t onceToken;
  dispatch_once(&onceToken, ^{
    fn = (CGDisplayCreateImageFunc)dlsym(RTLD_DEFAULT, "CGDisplayCreateImage");
  });

  if (fn == NULL) {
    return NULL;
  }

  return fn(displayID);
}

CapturePNGResult captureDisplayPNG(unsigned int displayID) {
  @autoreleasepool {
    CapturePNGResult result = {0};

    CGImageRef image = create_display_image((CGDirectDisplayID)displayID);
    if (image == NULL) {
      result.err = copy_error_message(
          "failed to capture display image (screen recording permission may be missing)");
      return result;
    }

    NSBitmapImageRep *bitmap =
        [[NSBitmapImageRep alloc] initWithCGImage:image];
    CGImageRelease(image);
    if (bitmap == nil) {
      result.err = copy_error_message("failed to create bitmap representation");
      return result;
    }

    NSData *pngData = [bitmap representationUsingType:NSBitmapImageFileTypePNG
                                           properties:@{}];
    if (pngData == nil || [pngData length] == 0) {
      result.err = copy_error_message("failed to encode capture as PNG");
      return result;
    }

    result.len = (int)[pngData length];
    result.data = (unsigned char *)malloc((size_t)result.len);
    if (result.data == NULL) {
      result.len = 0;
      result.err = copy_error_message("failed to allocate PNG buffer");
      return result;
    }

    memcpy(result.data, [pngData bytes], (size_t)result.len);
    return result;
  }
}

void releaseCapturePNGResult(CapturePNGResult result) {
  if (result.data != NULL) {
    free(result.data);
  }
  if (result.err != NULL) {
    free(result.err);
  }
}

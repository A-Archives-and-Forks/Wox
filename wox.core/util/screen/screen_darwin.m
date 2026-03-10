#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

typedef struct {
  int width;
  int height;
  int x;
  int y;
} ScreenInfo;

typedef struct {
  unsigned int id;
  int x;
  int y;
  int width;
  int height;
  int workX;
  int workY;
  int workWidth;
  int workHeight;
  int pixelX;
  int pixelY;
  int pixelWidth;
  int pixelHeight;
  int pixelWorkX;
  int pixelWorkY;
  int pixelWorkWidth;
  int pixelWorkHeight;
  double scale;
  int primary;
} ScreenDisplayInfo;

ScreenInfo getMouseScreenSize() {
  NSPoint mouseLoc = [NSEvent mouseLocation];
  NSArray *screens = [NSScreen screens];

  for (NSScreen *screen in screens) {
    NSRect frame = [screen frame];
    if (NSMouseInRect(mouseLoc, frame, NO)) {
      // IMPORTANT: Use visibleFrame instead of frame to exclude menu bar and
      // dock areas This ensures window positioning calculations use only the
      // available workspace area
      NSRect visibleFrame = [screen visibleFrame];

      // Convert from AppKit's bottom-left origin to top-left origin coordinate
      // system
      //
      // Why this conversion is needed:
      // - AppKit uses bottom-left origin with Y-axis pointing up
      // - Go backend expects top-left origin with Y-axis pointing down
      // (standard for most UI frameworks)
      // - We need to return the Y offset from the physical screen top to the
      // visible area top
      //
      // Calculation:
      // - frame.size.height = total screen height (e.g., 1080 pixels)
      // - visibleFrame.size.height = available height excluding menu bar (e.g.,
      // 1055 pixels)
      // - topY = frame.size.height - visibleFrame.size.height = menu bar height
      // (e.g., 25 pixels)
      //
      // This topY value tells Go backend: "the usable area starts 25 pixels
      // from the screen top"
      int topY = frame.size.height - visibleFrame.size.height;

      return (ScreenInfo){.width = visibleFrame.size.width,
                          .height = visibleFrame.size.height,
                          .x = visibleFrame.origin.x,
                          .y = topY};
    }
  }
  return (ScreenInfo){.width = 0, .height = 0, .x = 0, .y = 0};
}

ScreenInfo getPrimaryScreenSize() {
  NSScreen *primaryScreen = [NSScreen mainScreen];
  NSRect frame = [primaryScreen frame];
  // Use visibleFrame to exclude menu bar and dock areas
  NSRect visibleFrame = [primaryScreen visibleFrame];
  // Convert from AppKit's bottom-left origin to top-left origin
  // topY = distance from physical screen top to visible area top (menu bar
  // height)
  int topY = frame.size.height - visibleFrame.size.height;
  return (ScreenInfo){.width = visibleFrame.size.width,
                      .height = visibleFrame.size.height,
                      .x = visibleFrame.origin.x,
                      .y = topY};
}

ScreenInfo getActiveScreenSize() {
  NSWindow *keyWindow = [[NSApplication sharedApplication] keyWindow];
  if (!keyWindow) {
    // Fallback to primary screen if no active window
    return getPrimaryScreenSize();
  }

  NSScreen *activeScreen = [keyWindow screen];
  if (!activeScreen) {
    // Fallback to primary screen if window's screen not found
    return getPrimaryScreenSize();
  }

  NSRect frame = [activeScreen frame];
  // Use visibleFrame to exclude menu bar and dock areas
  NSRect visibleFrame = [activeScreen visibleFrame];
  // Convert from AppKit's bottom-left origin to top-left origin
  // topY = distance from physical screen top to visible area top (menu bar
  // height)
  int topY = frame.size.height - visibleFrame.size.height;
  return (ScreenInfo){.width = visibleFrame.size.width,
                      .height = visibleFrame.size.height,
                      .x = visibleFrame.origin.x,
                      .y = topY};
}

int listDisplays(ScreenDisplayInfo *displays, int maxCount) {
  NSArray<NSScreen *> *screens = [NSScreen screens];
  NSInteger count = [screens count];
  if (count <= 0 || maxCount <= 0) {
    return 0;
  }

  if (count > maxCount) {
    count = maxCount;
  }

  CGFloat minX = 0;
  CGFloat minPixelX = 0;
  CGFloat minPixelY = 0;
  CGFloat maxY = 0;
  BOOL initialized = NO;

  for (NSScreen *screen in screens) {
    NSRect frame = [screen frame];
    NSNumber *screenNumber = screen.deviceDescription[@"NSScreenNumber"];
    CGDirectDisplayID displayID = (CGDirectDisplayID)[screenNumber unsignedIntValue];
    CGRect pixelFrame = CGDisplayBounds(displayID);

    if (!initialized) {
      minX = frame.origin.x;
      maxY = NSMaxY(frame);
      minPixelX = pixelFrame.origin.x;
      minPixelY = pixelFrame.origin.y;
      initialized = YES;
    } else {
      if (frame.origin.x < minX) {
        minX = frame.origin.x;
      }
      if (NSMaxY(frame) > maxY) {
        maxY = NSMaxY(frame);
      }
      if (pixelFrame.origin.x < minPixelX) {
        minPixelX = pixelFrame.origin.x;
      }
      if (pixelFrame.origin.y < minPixelY) {
        minPixelY = pixelFrame.origin.y;
      }
    }
  }

  for (NSInteger i = 0; i < count; i++) {
    NSScreen *screen = screens[i];
    NSRect frame = [screen frame];
    NSRect visibleFrame = [screen visibleFrame];
    CGFloat scale = [screen backingScaleFactor];

    NSNumber *screenNumber = screen.deviceDescription[@"NSScreenNumber"];
    CGDirectDisplayID displayID = (CGDirectDisplayID)[screenNumber unsignedIntValue];
    CGRect pixelFrame = CGDisplayBounds(displayID);

    int logicalX = (int)llround(frame.origin.x - minX);
    int logicalY = (int)llround(maxY - NSMaxY(frame));
    int logicalWidth = (int)llround(frame.size.width);
    int logicalHeight = (int)llround(frame.size.height);

    int logicalWorkX = (int)llround(visibleFrame.origin.x - minX);
    int logicalWorkY = (int)llround(maxY - NSMaxY(visibleFrame));
    int logicalWorkWidth = (int)llround(visibleFrame.size.width);
    int logicalWorkHeight = (int)llround(visibleFrame.size.height);

    int pixelX = (int)llround(pixelFrame.origin.x - minPixelX);
    int pixelY = (int)llround(pixelFrame.origin.y - minPixelY);
    int pixelWidth = (int)llround(pixelFrame.size.width);
    int pixelHeight = (int)llround(pixelFrame.size.height);

    int pixelWorkX = (int)llround((visibleFrame.origin.x - minX) * scale);
    int pixelWorkY = (int)llround((maxY - NSMaxY(visibleFrame)) * scale);
    int pixelWorkWidth = (int)llround(visibleFrame.size.width * scale);
    int pixelWorkHeight = (int)llround(visibleFrame.size.height * scale);

    displays[i] = (ScreenDisplayInfo){
        .id = displayID,
        .x = logicalX,
        .y = logicalY,
        .width = logicalWidth,
        .height = logicalHeight,
        .workX = logicalWorkX,
        .workY = logicalWorkY,
        .workWidth = logicalWorkWidth,
        .workHeight = logicalWorkHeight,
        .pixelX = pixelX,
        .pixelY = pixelY,
        .pixelWidth = pixelWidth,
        .pixelHeight = pixelHeight,
        .pixelWorkX = pixelWorkX,
        .pixelWorkY = pixelWorkY,
        .pixelWorkWidth = pixelWorkWidth,
        .pixelWorkHeight = pixelWorkHeight,
        .scale = scale,
        .primary = [screen isEqual:[NSScreen mainScreen]] ? 1 : 0,
    };
  }

  return (int)count;
}

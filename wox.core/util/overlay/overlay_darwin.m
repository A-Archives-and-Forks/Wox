#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreVideo/CoreVideo.h>
#import <ApplicationServices/ApplicationServices.h>
#include <math.h>
#include <stdlib.h>

// -----------------------------------------------------------------------------
// Options Struct (Must match CGO / Go definition)
// -----------------------------------------------------------------------------
typedef struct {
    char* name;
    char* title;
    char* message;
    unsigned char* iconData;
    int iconLen;
    bool transparent;
    bool hitTestIconOnly;
    float iconX;
    float iconY;
    float iconWidth;
    float iconHeight;
    bool closable;
    int stickyWindowPid; // 0 = Screen, >0 = Window
    int anchor;          // 0-8: TL,TC,TR, LC,C,RC, BL,BC,BR
    int autoCloseSeconds;
    bool movable;
    float offsetX;
    float offsetY;
    float width;         // 0 = auto
    float height;        // 0 = auto
    float fontSize;      // 0 = system default, unit: pt
    float iconSize;      // 0 = default (16), unit: pt
    char* tooltip;
    unsigned char* tooltipIconData;
    int tooltipIconLen;
    float tooltipIconSize; // 0 = default (16), unit: pt
} OverlayOptions;

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------
static const CGFloat kDefaultWindowWidth = 400;
static const CGFloat kDefaultIconSize = 16;
static const CGFloat kCloseSize = 20;
static const CGFloat kTooltipIconGap = 8;
static const CGFloat kTooltipGap = 6;
static const CGFloat kTooltipPadding = 8;
static const CGFloat kTooltipMaxWidth = 400;
static const CGFloat kTooltipFontSize = 12;
static const CGFloat kStickyPredictiveCorrectionThreshold = 48;

extern void overlayClickCallbackCGO(char* name);
extern void overlayDebugLogCallbackCGO(char* message);

static void OverlayDebugLog(NSString *message) {
    if (!message) return;
    char *raw = strdup([message UTF8String]);
    if (!raw) return;
    overlayDebugLogCallbackCGO(raw);
    free(raw);
}

// -----------------------------------------------------------------------------
// Overlay Window
// -----------------------------------------------------------------------------
@class OverlayTooltipWindow;
@interface OverlayWindow : NSPanel
@property(nonatomic, strong) NSString *name; // Store the ID
@property(nonatomic, strong) NSTimer *closeTimer;
@property(nonatomic, strong) NSImageView *iconView;
@property(nonatomic, strong) NSImageView *tooltipIconView;
@property(nonatomic, strong) NSTextField *messageLabel;
// Simplified text view for now, or use full NSTextView from notifier if needed for multiline.
// Plan said "use NotificationWindow's robust text logic". So I should use NSTextView.
@property(nonatomic, strong) NSTextView *messageView;
@property(nonatomic, strong) NSButton *closeButton;
@property(nonatomic, strong) NSVisualEffectView *backgroundView;
@property(nonatomic, assign) int stickyPid;
@end

@interface OverlayWindow ()
@property(nonatomic, strong) NSTrackingArea *trackingArea;
@property(nonatomic, strong) NSTrackingArea *tooltipTrackingArea;
@property(nonatomic, assign) BOOL isMouseInside;
@property(nonatomic, assign) BOOL isAutoClosePending;
@property(nonatomic, assign) NSPoint initialLocation;
@property(nonatomic, assign) BOOL isMovable;
@property(nonatomic, assign) BOOL isDragging;
@property(nonatomic, assign) NSPoint initialWindowOrigin;
// AXObserver for tracking window movement
@property(nonatomic, assign) AXObserverRef axObserver;
@property(nonatomic, assign) AXUIElementRef trackedWindow;
@property(nonatomic, assign) pid_t trackedPid;
@property(nonatomic, assign) OverlayOptions currentOpts;
// Target window number for z-order management
@property(nonatomic, assign) CGWindowID stickyWindowNumber;
@property(nonatomic, strong) OverlayTooltipWindow *tooltipWindow;
@property(nonatomic, copy) NSString *tooltipText;
@property(nonatomic, assign) NSRect tooltipIconRect;
@property(nonatomic, assign) BOOL transparentMode;
@property(nonatomic, assign) BOOL hitTestIconOnly;
@property(nonatomic, assign) NSRect iconHitRect;
@property(nonatomic, assign) unsigned long long stickyMoveEventCount;
@property(nonatomic, assign) CFTimeInterval lastStickyMoveEventTime;
@property(nonatomic, assign) unsigned long long layoutUpdateCount;
@property(nonatomic, assign) CFTimeInterval lastLayoutUpdateTime;
@property(nonatomic, strong) NSTimer *stickyLiveFollowTimer;
@property(nonatomic, assign) unsigned long long stickyLiveFollowPollCount;
@property(nonatomic, assign) BOOL hasStickyPredictiveAnchor;
@property(nonatomic, assign) CGRect stickyPredictiveAnchorTargetRect;
@property(nonatomic, assign) NSPoint stickyPredictiveAnchorMouse;
@end

static NSMutableDictionary<NSString*, OverlayWindow*> *gOverlayWindows = nil;

// -----------------------------------------------------------------------------
// Helper Classes
// -----------------------------------------------------------------------------
@interface HandCursorButton : NSButton
@end

@implementation HandCursorButton
- (void)updateTrackingAreas {
  [super updateTrackingAreas];
  for (NSTrackingArea *area in self.trackingAreas) {
    [self removeTrackingArea:area];
  }
  NSTrackingArea *area = [[NSTrackingArea alloc] initWithRect:self.bounds
                                                      options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect | NSTrackingCursorUpdate
                                                        owner:self
                                                     userInfo:nil];
  [self addTrackingArea:area];
}

- (void)mouseEntered:(NSEvent *)event {
  [[NSCursor pointingHandCursor] set];
}

- (void)mouseExited:(NSEvent *)event {
  [[NSCursor arrowCursor] set];
}

- (void)cursorUpdate:(NSEvent *)event {
  [[NSCursor pointingHandCursor] set];
}
@end

// -----------------------------------------------------------------------------
// Passthrough TextView - lets mouse events pass through to window for dragging
// -----------------------------------------------------------------------------
@interface PassthroughTextView : NSTextView
@end

@implementation PassthroughTextView
- (NSView *)hitTest:(NSPoint)point {
    return nil; // Let mouse events pass through to window
}
@end

// -----------------------------------------------------------------------------
// Passthrough ImageView - lets mouse events pass through to window for dragging
// -----------------------------------------------------------------------------
@interface PassthroughImageView : NSImageView
@end

@implementation PassthroughImageView
- (NSView *)hitTest:(NSPoint)point {
    return nil; // Let mouse events pass through to window
}
@end

// -----------------------------------------------------------------------------
// Passthrough VisualEffectView - lets mouse events pass through to window
// -----------------------------------------------------------------------------
@interface PassthroughVisualEffectView : NSVisualEffectView
@end

@implementation PassthroughVisualEffectView
- (NSView *)hitTest:(NSPoint)point {
    return nil; // Let mouse events pass through to window
}
@end

// -----------------------------------------------------------------------------
// Draggable Content View - accepts first mouse to enable immediate dragging
// -----------------------------------------------------------------------------
@interface DraggableContentView : NSView
@end

@implementation DraggableContentView
- (BOOL)acceptsFirstMouse:(NSEvent *)event {
    return YES; // Accept click even when window is not key
}

- (NSView *)hitTest:(NSPoint)point {
    OverlayWindow *overlay = [self.window isKindOfClass:[OverlayWindow class]] ? (OverlayWindow *)self.window : nil;
    if (overlay && overlay.transparentMode && overlay.hitTestIconOnly && !NSPointInRect(point, overlay.iconHitRect)) {
        return nil;
    }
    return [super hitTest:point];
}
@end

// -----------------------------------------------------------------------------
// Tooltip Window
// -----------------------------------------------------------------------------
@interface OverlayTooltipWindow : NSPanel
@property(nonatomic, strong) NSVisualEffectView *backgroundView;
@property(nonatomic, strong) NSTextField *textLabel;
- (void)showWithText:(NSString *)text relativeToRect:(NSRect)iconRect inWindow:(NSWindow *)owner;
- (void)hideTooltip;
@end

@implementation OverlayTooltipWindow

- (instancetype)init {
    self = [super initWithContentRect:NSMakeRect(0, 0, 100, 40)
                             styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                               backing:NSBackingStoreBuffered
                                 defer:NO];
    if (self) {
        [self setOpaque:NO];
        [self setHasShadow:YES];
        [self setBackgroundColor:[NSColor clearColor]];
        [self setLevel:NSFloatingWindowLevel];
        [self setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorTransient];
        [self setIgnoresMouseEvents:YES];

        NSView *content = [[NSView alloc] initWithFrame:self.contentView.bounds];
        [self setContentView:content];
        content.wantsLayer = YES;
        content.layer.cornerRadius = 6.0;
        content.layer.masksToBounds = YES;

        NSVisualEffectView *bg = [[NSVisualEffectView alloc] initWithFrame:content.bounds];
        bg.material = NSVisualEffectMaterialHUDWindow;
        bg.state = NSVisualEffectStateActive;
        bg.blendingMode = NSVisualEffectBlendingModeBehindWindow;
        if (@available(macOS 10.14, *)) {
            bg.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        }
        [content addSubview:bg positioned:NSWindowBelow relativeTo:nil];
        self.backgroundView = bg;

        NSTextField *label = [[NSTextField alloc] initWithFrame:NSZeroRect];
        label.editable = NO;
        label.selectable = NO;
        label.drawsBackground = NO;
        label.bezeled = NO;
        label.font = [NSFont systemFontOfSize:kTooltipFontSize];
        label.textColor = [NSColor whiteColor];
        label.alignment = NSTextAlignmentLeft;
        label.lineBreakMode = NSLineBreakByWordWrapping;
        label.usesSingleLineMode = NO;
        if ([label.cell respondsToSelector:@selector(setWraps:)]) {
            label.cell.wraps = YES;
        }
        if ([label.cell respondsToSelector:@selector(setScrollable:)]) {
            label.cell.scrollable = NO;
        }
        if ([label respondsToSelector:@selector(setMaximumNumberOfLines:)]) {
            label.maximumNumberOfLines = 0;
        }
        [content addSubview:label];
        self.textLabel = label;
    }
    return self;
}

- (BOOL)canBecomeKeyWindow {
    return NO;
}

- (void)showWithText:(NSString *)text relativeToRect:(NSRect)iconRect inWindow:(NSWindow *)owner {
    if (!owner) return;
    if (!text) text = @"";

    self.textLabel.stringValue = text;
    NSFont *font = self.textLabel.font ?: [NSFont systemFontOfSize:kTooltipFontSize];
    NSDictionary *attrs = @{NSFontAttributeName: font};

    NSRect textRect = [text boundingRectWithSize:NSMakeSize(kTooltipMaxWidth, CGFLOAT_MAX)
                                         options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingUsesFontLeading
                                      attributes:attrs];
    CGFloat textW = ceil(textRect.size.width);
    CGFloat textH = ceil(textRect.size.height);
    if (textW < 1) textW = 1;
    if (textH < 1) textH = 1;

    CGFloat width = textW + kTooltipPadding * 2;
    CGFloat height = textH + kTooltipPadding * 2;

    self.textLabel.frame = NSMakeRect(kTooltipPadding, kTooltipPadding, textW, textH);
    self.backgroundView.frame = ((NSView *)self.contentView).bounds;

    NSRect iconScreen = [owner convertRectToScreen:iconRect];
    NSPoint iconCenter = NSMakePoint(NSMidX(iconScreen), NSMidY(iconScreen));

    NSScreen *targetScreen = owner.screen ?: [NSScreen mainScreen];
    for (NSScreen *screen in [NSScreen screens]) {
        if (NSPointInRect(iconCenter, screen.frame)) {
            targetScreen = screen;
            break;
        }
    }
    NSRect workArea = targetScreen.visibleFrame;

    CGFloat x = iconScreen.origin.x + (iconScreen.size.width - width) / 2;
    CGFloat y = iconScreen.origin.y + iconScreen.size.height + kTooltipGap;

    if (y + height > NSMaxY(workArea)) {
        y = iconScreen.origin.y - height - kTooltipGap;
    }
    if (x + width > NSMaxX(workArea)) {
        x = NSMaxX(workArea) - width;
    }
    if (x < workArea.origin.x) {
        x = workArea.origin.x;
    }
    if (y < workArea.origin.y) {
        y = workArea.origin.y;
    }

    [self setFrame:NSMakeRect(x, y, width, height) display:YES];
    self.backgroundView.frame = ((NSView *)self.contentView).bounds;
    [self orderFront:nil];
}

- (void)hideTooltip {
    [self orderOut:nil];
}

@end

@implementation OverlayWindow

- (instancetype)initWithContentRect:(NSRect)contentRect styleMask:(NSWindowStyleMask)style backing:(NSBackingStoreType)backingStoreType defer:(BOOL)flag {
    self = [super initWithContentRect:contentRect styleMask:style backing:backingStoreType defer:flag];
    if (self) {
        [self setBackgroundColor:[NSColor clearColor]];
        // ... (Keep existing setup)
        [self setOpaque:NO];
        [self setHasShadow:YES];
        [self setLevel:NSFloatingWindowLevel];
        [self setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorTransient];
        // Allow first click to trigger mouseDown instead of just activating window
        [self setBecomesKeyOnlyIfNeeded:NO];

        // Set custom content view that accepts first mouse
        DraggableContentView *contentView = [[DraggableContentView alloc] initWithFrame:contentRect];
        [self setContentView:contentView];
        
        // Background - use PassthroughVisualEffectView for drag support
        PassthroughVisualEffectView *bg = [[PassthroughVisualEffectView alloc] initWithFrame:contentView.bounds];
        bg.material = NSVisualEffectMaterialHUDWindow;
        bg.state = NSVisualEffectStateActive;
        bg.blendingMode = NSVisualEffectBlendingModeBehindWindow;
        if (@available(macOS 10.14, *)) {
            bg.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        }
        [self.contentView addSubview:bg positioned:NSWindowBelow relativeTo:nil];
        self.backgroundView = bg;

        // Icon - use PassthroughImageView for drag support
        self.iconView = [[PassthroughImageView alloc] initWithFrame:NSMakeRect(12, 0, kDefaultIconSize, kDefaultIconSize)];
        self.iconView.imageScaling = NSImageScaleProportionallyUpOrDown;
        self.iconView.hidden = YES;
        [self.contentView addSubview:self.iconView];

        // Tooltip Icon - use PassthroughImageView for drag support
        self.tooltipIconView = [[PassthroughImageView alloc] initWithFrame:NSMakeRect(0, 0, kDefaultIconSize, kDefaultIconSize)];
        self.tooltipIconView.imageScaling = NSImageScaleProportionallyUpOrDown;
        self.tooltipIconView.hidden = YES;
        [self.contentView addSubview:self.tooltipIconView];

        // Message (TextView for multiline) - use PassthroughTextView for drag support
        self.messageView = [[PassthroughTextView alloc] initWithFrame:NSZeroRect];
        self.messageView.editable = NO;
        self.messageView.selectable = NO;
        self.messageView.drawsBackground = NO;
        if (@available(macOS 10.14, *)) {
            self.messageView.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        }
        [self.contentView addSubview:self.messageView];

        // Close Button (HandCursorButton)
        self.closeButton = [[HandCursorButton alloc] initWithFrame:NSMakeRect(0, 0, kCloseSize, kCloseSize)];
        [self.closeButton setBezelStyle:NSBezelStyleRegularSquare];
        [self.closeButton setButtonType:NSButtonTypeMomentaryLight];
        [self.closeButton setTitle:@"×"];
        [self.closeButton setFont:[NSFont systemFontOfSize:16 weight:NSFontWeightBold]];
        [self.closeButton setTarget:self];
        [self.closeButton setAction:@selector(onClose)];
        [self.closeButton setHidden:NO];
        [self.closeButton setBordered:NO];
        [self.closeButton setWantsLayer:YES];
        self.closeButton.layer.backgroundColor = [NSColor colorWithWhite:1.0 alpha:0.3].CGColor;
        self.closeButton.layer.cornerRadius = kCloseSize / 2;
        
        NSMutableAttributedString *attributedTitle = [[NSMutableAttributedString alloc] initWithString:@"×"];
        [attributedTitle addAttribute:NSForegroundColorAttributeName value:[NSColor whiteColor] range:NSMakeRange(0, attributedTitle.length)];
        [self.closeButton setAttributedTitle:attributedTitle];
        
        [self.contentView addSubview:self.closeButton];

        // Tracking Area setup
        [self setupTrackingArea];
    }
    return self;
}

- (void)mouseDown:(NSEvent *)event {
    [self hideTooltipWindow];
    self.initialLocation = [NSEvent mouseLocation];
    self.initialWindowOrigin = self.frame.origin;

    if (self.isMovable) {
        self.isDragging = YES;
    }
}

- (void)mouseDragged:(NSEvent *)event {
    if (!self.isDragging) return;
    
    NSPoint currentLocation = [NSEvent mouseLocation];
    CGFloat dx = currentLocation.x - self.initialLocation.x;
    CGFloat dy = currentLocation.y - self.initialLocation.y;
    
    NSPoint newOrigin = NSMakePoint(self.initialWindowOrigin.x + dx,
                                    self.initialWindowOrigin.y + dy);
    [self setFrameOrigin:newOrigin];
}

- (void)mouseUp:(NSEvent *)event {
    self.isDragging = NO;
    
    NSPoint currentLocation = [NSEvent mouseLocation];
    CGFloat dx = currentLocation.x - self.initialLocation.x;
    CGFloat dy = currentLocation.y - self.initialLocation.y;
    
    // If movement is small, treat as click
    if (dx*dx + dy*dy < 25.0) {
        [self onClick];
    }
    
    // If auto-close passed while dragging, and we are not inside (or maybe we should just re-check pending state)
    // Actually, if we release drag, and pending is YES, we should check if we are currently inside.
    // But `isMouseInside` state is maintained by Enter/Exit events.
    if (self.isAutoClosePending && !self.isMouseInside) {
        [self onClose];
    }
}

- (void)setupTrackingArea {
    if (self.trackingArea) {
        [self.contentView removeTrackingArea:self.trackingArea];
    }
    
    NSTrackingAreaOptions options = NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect;
    self.trackingArea = [[NSTrackingArea alloc] initWithRect:NSZeroRect // Ignored by InVisibleRect
                                                     options:options
                                                       owner:self
                                                    userInfo:nil];
    [self.contentView addTrackingArea:self.trackingArea];
}

- (void)updateTooltipTrackingAreaWithRect:(NSRect)rect enabled:(BOOL)enabled {
    if (self.tooltipTrackingArea) {
        [self.contentView removeTrackingArea:self.tooltipTrackingArea];
        self.tooltipTrackingArea = nil;
    }

    if (!enabled) return;

    NSTrackingAreaOptions options = NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways;
    NSDictionary *info = @{@"type": @"tooltip"};
    self.tooltipTrackingArea = [[NSTrackingArea alloc] initWithRect:rect
                                                            options:options
                                                              owner:self
                                                           userInfo:info];
    [self.contentView addTrackingArea:self.tooltipTrackingArea];
}

- (void)ensureTooltipWindow {
    if (!self.tooltipWindow) {
        self.tooltipWindow = [[OverlayTooltipWindow alloc] init];
    }
}

- (void)showTooltipWindow {
    if (!self.tooltipText || self.tooltipText.length == 0) return;
    if (NSIsEmptyRect(self.tooltipIconRect)) return;
    [self ensureTooltipWindow];
    [self.tooltipWindow showWithText:self.tooltipText relativeToRect:self.tooltipIconRect inWindow:self];
}

- (void)hideTooltipWindow {
    if (self.tooltipWindow) {
        [self.tooltipWindow hideTooltip];
    }
}

- (void)mouseEntered:(NSEvent *)event {
    if (event.trackingArea == self.tooltipTrackingArea) {
        self.isMouseInside = YES;
        [self showTooltipWindow];
        return;
    }
    self.isMouseInside = YES;
}

- (void)mouseExited:(NSEvent *)event {
    if (event.trackingArea == self.tooltipTrackingArea) {
        [self hideTooltipWindow];
        return;
    }
    self.isMouseInside = NO;
    // Don't auto-close while dragging
    if (self.isAutoClosePending && !self.isDragging) {
        [self onClose];
    }
}

// ... (Timer methods remain same) ...

- (void)startAutoCloseTimer:(NSTimeInterval)seconds {
    [self stopAutoCloseTimer];
    if (seconds > 0) {
        self.closeTimer = [NSTimer scheduledTimerWithTimeInterval:seconds
                                                           target:self
                                                         selector:@selector(onAutoCloseTimerFired:)
                                                         userInfo:nil
                                                          repeats:NO];
    }
}

- (void)stopAutoCloseTimer {
    if (self.closeTimer) {
        [self.closeTimer invalidate];
        self.closeTimer = nil;
    }
    self.isAutoClosePending = NO;
}

- (void)onAutoCloseTimerFired:(NSTimer*)timer {
    if (self.isMouseInside || self.isDragging) {
        self.isAutoClosePending = YES;
    } else {
        [self onClose];
    }
}

- (void)setCornerRadius:(CGFloat)radius {
    self.contentView.wantsLayer = YES;
    self.contentView.layer.cornerRadius = radius;
    self.contentView.layer.masksToBounds = YES;
}

- (void)onClose {
    [self stopAutoCloseTimer];
    [self stopTrackingWindow];
    [self hideTooltipWindow];
    [self close];
    if (gOverlayWindows && self.name) {
        [gOverlayWindows removeObjectForKey:self.name];
    }
}

- (void)stopTrackingWindow {
    [self stopStickyLiveFollowTimerWithReason:@"tracking-stopped"];
    self.hasStickyPredictiveAnchor = NO;

    if (self.axObserver) {
        CFRunLoopRemoveSource(CFRunLoopGetMain(), 
                              AXObserverGetRunLoopSource(self.axObserver), 
                              kCFRunLoopDefaultMode);
        CFRelease(self.axObserver);
        self.axObserver = NULL;
    }
    if (self.trackedWindow) {
        CFRelease(self.trackedWindow);
        self.trackedWindow = NULL;
    }
    self.trackedPid = 0;
}

// Get the focused window number for a given PID
- (CGWindowID)getWindowNumberForPid:(pid_t)pid {
    CGWindowID result = 0;
    
    // Get all windows
    CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    if (!windowList) return 0;
    
    // Find the frontmost window for this PID
    for (CFIndex i = 0; i < CFArrayGetCount(windowList); i++) {
        NSDictionary *windowInfo = (NSDictionary *)CFArrayGetValueAtIndex(windowList, i);
        NSNumber *windowPid = windowInfo[(id)kCGWindowOwnerPID];
        NSNumber *windowNumber = windowInfo[(id)kCGWindowNumber];
        NSNumber *windowLayer = windowInfo[(id)kCGWindowLayer];
        
        // Only consider normal layer windows (layer 0)
        if ([windowPid intValue] == pid && [windowLayer intValue] == 0) {
            result = [windowNumber unsignedIntValue];
            break; // First one found is typically the frontmost
        }
    }
    
    CFRelease(windowList);
    return result;
}

- (BOOL)getWindowFrameForPid:(pid_t)pid outRect:(CGRect *)outRect {
    if (pid <= 0 || !outRect) return NO;

    CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    if (!windowList) return NO;

    BOOL found = NO;
    for (CFIndex i = 0; i < CFArrayGetCount(windowList); i++) {
        NSDictionary *windowInfo = (NSDictionary *)CFArrayGetValueAtIndex(windowList, i);
        NSNumber *windowPid = windowInfo[(id)kCGWindowOwnerPID];
        NSNumber *windowLayer = windowInfo[(id)kCGWindowLayer];
        NSNumber *windowAlpha = windowInfo[(id)kCGWindowAlpha];
        NSDictionary *boundsDict = windowInfo[(id)kCGWindowBounds];

        if ([windowPid intValue] != pid || [windowLayer intValue] != 0 || !boundsDict) {
            continue;
        }
        if (windowAlpha && [windowAlpha doubleValue] <= 0.01) {
            continue;
        }

        CGRect cgBounds = CGRectZero;
        if (!CGRectMakeWithDictionaryRepresentation((CFDictionaryRef)boundsDict, &cgBounds)) {
            continue;
        }
        if (cgBounds.size.width <= 1 || cgBounds.size.height <= 1) {
            continue;
        }

        // Bug fix: AX focused-window lookup can fail for debug builds or stale
        // focus state. Sticky overlays must stay attached to their target window;
        // falling back to the whole screen can place them offscreen. CGWindowList
        // gives the native target rect without requiring AX, while preserving the
        // existing AX observer path when accessibility is available.
        CGFloat mainScreenH = [NSScreen mainScreen].frame.size.height;
        CGFloat cocoaY = mainScreenH - cgBounds.origin.y - cgBounds.size.height;
        *outRect = CGRectMake(cgBounds.origin.x, cocoaY, cgBounds.size.width, cgBounds.size.height);
        found = YES;
        break;
    }

    CFRelease(windowList);
    return found;
}

// Order overlay window relative to sticky window
- (void)orderRelativeToStickyWindow {
    if (self.stickyWindowNumber > 0) {
        // Use normal window level and order above the target window
        [self setLevel:NSNormalWindowLevel];
        [self orderWindow:NSWindowAbove relativeTo:self.stickyWindowNumber];
    } else {
        // Fallback to floating level
        [self setLevel:NSFloatingWindowLevel];
        [self orderFront:nil];
    }
}

- (void)onClick {
    if (self.name) {
       overlayClickCallbackCGO((char*)[self.name UTF8String]);
    }
}

- (BOOL)canBecomeKeyWindow {
    return YES; // Allow interaction
}

- (void)updateLayoutWithOptions:(OverlayOptions)opts {
    CFTimeInterval layoutStart = CACurrentMediaTime();
    double layoutIntervalMs = -1;
    if (self.lastLayoutUpdateTime > 0) {
        layoutIntervalMs = (layoutStart - self.lastLayoutUpdateTime) * 1000.0;
    }
    self.lastLayoutUpdateTime = layoutStart;
    self.layoutUpdateCount++;

    // 0. Reset State
    self.isMovable = opts.movable;
    self.isDragging = NO;
    [self stopAutoCloseTimer];

    // 1. Content Update
    [self hideTooltipWindow];
    NSString *msg = opts.message ? [NSString stringWithUTF8String:opts.message] : @"";
    NSImage *icon = nil;
    if (opts.iconData && opts.iconLen > 0) {
        NSData *data = [NSData dataWithBytes:opts.iconData length:opts.iconLen];
        icon = [[NSImage alloc] initWithData:data];
    }

    self.iconView.image = icon;
    self.iconView.hidden = (icon == nil);
    
    self.closeButton.hidden = !opts.closable;

    NSString *tooltip = opts.tooltip ? [NSString stringWithUTF8String:opts.tooltip] : @"";
    self.tooltipText = tooltip;

    NSImage *tooltipIcon = nil;
    if (tooltip.length > 0) {
        if (opts.tooltipIconData && opts.tooltipIconLen > 0) {
            NSData *tipData = [NSData dataWithBytes:opts.tooltipIconData length:opts.tooltipIconLen];
            tooltipIcon = [[NSImage alloc] initWithData:tipData];
        } else {
            tooltipIcon = [NSImage imageNamed:NSImageNameInfo];
        }
    }

    self.tooltipIconView.image = tooltipIcon;
    self.tooltipIconView.hidden = (tooltip.length == 0 || tooltipIcon == nil);

    // 2. Measure & Layout
    CGFloat windowWidth = (opts.width > 0) ? opts.width : kDefaultWindowWidth;
    CGFloat windowHeight = 0;
    self.transparentMode = opts.transparent;
    self.hitTestIconOnly = opts.hitTestIconOnly;
    self.backgroundView.hidden = opts.transparent;
    [self setHasShadow:!opts.transparent];

    if (opts.transparent) {
        // Transparent overlays are generic drawing surfaces. The default HUD layout
        // centers content inside a blurred notification bubble, while surface mode
        // lets callers place their own content inside a clear native window.
        CGFloat sourceIconWidth = icon ? icon.size.width : kDefaultIconSize;
        CGFloat sourceIconHeight = icon ? icon.size.height : kDefaultIconSize;
        CGFloat fallbackIconSize = (opts.iconSize > 0) ? opts.iconSize : MAX(sourceIconWidth, sourceIconHeight);
        CGFloat iconWidth = (opts.iconWidth > 0) ? opts.iconWidth : fallbackIconSize;
        CGFloat iconHeight = (opts.iconHeight > 0) ? opts.iconHeight : fallbackIconSize;
        windowWidth = (opts.width > 0) ? opts.width : iconWidth;
        windowHeight = (opts.height > 0) ? opts.height : iconHeight;

        self.messageView.hidden = YES;
        self.closeButton.hidden = YES;
        self.tooltipIconView.hidden = YES;
        self.tooltipIconRect = NSZeroRect;
        [self updateTooltipTrackingAreaWithRect:NSZeroRect enabled:NO];

        CGFloat iconX = opts.iconX;
        CGFloat iconY = windowHeight - opts.iconY - iconHeight;
        self.iconView.frame = NSMakeRect(iconX, iconY, iconWidth, iconHeight);
        self.iconHitRect = self.iconView.hidden ? NSZeroRect : self.iconView.frame;
    } else {
        self.messageView.hidden = NO;
        self.iconHitRect = NSZeroRect;

        // Paddings
        CGFloat padLeft = 12;
        CGFloat padRight = 12;
        CGFloat padTop = 10;
        CGFloat padBottom = 10;
        
        CGFloat iconSize = (opts.iconSize > 0) ? opts.iconSize : kDefaultIconSize;
        CGFloat fontSize = (opts.fontSize > 0) ? opts.fontSize : [NSFont systemFontSize];
        CGFloat tooltipIconSize = (opts.tooltipIconSize > 0) ? opts.tooltipIconSize : kDefaultIconSize;
        CGFloat tooltipIconGap = self.tooltipIconView.hidden ? 0 : kTooltipIconGap;

        if (!self.iconView.hidden) padLeft += iconSize + 8;
        if (!self.closeButton.hidden) padRight += kCloseSize + 4;
        if (!self.tooltipIconView.hidden) padRight += tooltipIconSize + tooltipIconGap;

        CGFloat contentWidth = windowWidth - padLeft - padRight;
        
        // Setup TextView string
        NSDictionary *attrs = @{
            NSFontAttributeName: [NSFont systemFontOfSize:fontSize],
            NSForegroundColorAttributeName: [NSColor whiteColor]
        };
        NSAttributedString *attrStr = [[NSAttributedString alloc] initWithString:msg attributes:attrs];
        [self.messageView.textStorage setAttributedString:attrStr];
        
        // Measure Height
        NSSize textSize = [self.messageView.layoutManager usedRectForTextContainer:self.messageView.textContainer].size; 
        NSTextContainer *tc = self.messageView.textContainer;
        tc.containerSize = NSMakeSize(contentWidth, CGFLOAT_MAX);
        tc.widthTracksTextView = NO;
        [self.messageView.layoutManager ensureLayoutForTextContainer:tc];
        textSize = [self.messageView.layoutManager usedRectForTextContainer:tc].size;

        CGFloat textHeight = textSize.height;
        windowHeight = (opts.height > 0) ? opts.height : (textHeight + padTop + padBottom);
        if (windowHeight < 40) windowHeight = 40; // Min height

        // Update Frames
        CGFloat currentY = (windowHeight - textHeight) / 2; // Center Vertically
        if (currentY < padTop) currentY = padTop;

        self.messageView.frame = NSMakeRect(padLeft, currentY, contentWidth, textHeight);
        
        if (!self.iconView.hidden) {
            self.iconView.frame = NSMakeRect(12, (windowHeight - iconSize)/2, iconSize, iconSize);
        }
        if (!self.tooltipIconView.hidden) {
            CGFloat textRight = padLeft + contentWidth;
            CGFloat ty = (windowHeight - tooltipIconSize) / 2;
            if (ty < padTop) ty = padTop;
            self.tooltipIconView.frame = NSMakeRect(textRight + tooltipIconGap, ty, tooltipIconSize, tooltipIconSize);
            self.tooltipIconRect = [self.contentView convertRect:self.tooltipIconView.frame toView:nil];
            [self updateTooltipTrackingAreaWithRect:self.tooltipIconView.frame enabled:YES];
        } else {
            self.tooltipIconRect = NSZeroRect;
            [self updateTooltipTrackingAreaWithRect:NSZeroRect enabled:NO];
        }
        if (!self.closeButton.hidden) {
            self.closeButton.frame = NSMakeRect(windowWidth - kCloseSize - 6, (windowHeight - kCloseSize)/2, kCloseSize, kCloseSize);
        }
    }

    // 3. Position Calculation (Anchor)
    CGRect targetRect;
    NSString *targetSource = @"screen";
    BOOL preserveLiveFollowFrame = opts.stickyWindowPid > 0 && self.stickyLiveFollowTimer != nil;
    NSPoint liveFollowOrigin = self.frame.origin;
    
    if (preserveLiveFollowFrame) {
        // Bug fix: content refreshes can arrive while a sticky overlay is being
        // live-followed. Re-anchoring from AX here can use stale geometry and pull
        // the overlay behind the dragged window, so preserve the live-followed
        // origin and let the poller own position updates during the active drag.
        self.stickyWindowNumber = [self getWindowNumberForPid:(pid_t)opts.stickyWindowPid];
        targetRect = CGRectMake(liveFollowOrigin.x, liveFollowOrigin.y, windowWidth, windowHeight);
        targetSource = @"live-follow-preserve";
    } else if (opts.stickyWindowPid > 0) {
        pid_t pid = (pid_t)opts.stickyWindowPid;
        targetSource = @"none";
        
        // Get window number for z-order management
        self.stickyWindowNumber = [self getWindowNumberForPid:pid];

        BOOL targetFound = NO;
        AXUIElementRef app = AXUIElementCreateApplication(pid);
        AXUIElementRef frontWindow = NULL;
        AXError err = app ? AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute, (CFTypeRef *)&frontWindow) : kAXErrorFailure;
        if (err == kAXErrorSuccess && frontWindow) {
            CFTypeRef posVal = NULL, sizeVal = NULL;
            CGPoint pos; CGSize size;
            AXError posErr = AXUIElementCopyAttributeValue(frontWindow, kAXPositionAttribute, &posVal);
            AXError sizeErr = AXUIElementCopyAttributeValue(frontWindow, kAXSizeAttribute, &sizeVal);
            if (posErr == kAXErrorSuccess && sizeErr == kAXErrorSuccess && posVal && sizeVal) {
                AXValueGetValue(posVal, kAXValueCGPointType, &pos);
                AXValueGetValue(sizeVal, kAXValueCGSizeType, &size);

                // Find the screen containing the window center
                NSPoint windowCenter = NSMakePoint(pos.x + size.width / 2, pos.y + size.height / 2);
                NSScreen *targetScreen = nil;
                for (NSScreen *screen in [NSScreen screens]) {
                    // Convert screen frame from Cocoa to CG coordinates for comparison
                    NSRect screenFrame = screen.frame;
                    CGFloat mainScreenH = [NSScreen mainScreen].frame.size.height;
                    CGRect cgScreenFrame = CGRectMake(screenFrame.origin.x, 
                                                       mainScreenH - screenFrame.origin.y - screenFrame.size.height, 
                                                       screenFrame.size.width, 
                                                       screenFrame.size.height);
                    if (CGRectContainsPoint(cgScreenFrame, windowCenter)) {
                        targetScreen = screen;
                        break;
                    }
                }
                if (!targetScreen) targetScreen = [NSScreen mainScreen];

                // Convert CG coordinates to Cocoa coordinates using main screen height
                CGFloat mainScreenH = [NSScreen mainScreen].frame.size.height;
                CGFloat cocoaY = mainScreenH - pos.y - size.height;
                targetRect = CGRectMake(pos.x, cocoaY, size.width, size.height);
                targetFound = YES;
                targetSource = @"initial-ax";
            }
            if (posVal) CFRelease(posVal);
            if (sizeVal) CFRelease(sizeVal);
            CFRelease(frontWindow);
        }
        if (app) CFRelease(app);
        if (!targetFound) {
            if ([self getWindowFrameForPid:pid outRect:&targetRect]) {
                targetSource = @"initial-cg-window-list";
            } else {
                targetRect = [NSScreen mainScreen].visibleFrame;
                targetSource = @"initial-screen-fallback";
            }
        }
    } else {
        self.stickyWindowNumber = 0;
        targetRect = [NSScreen mainScreen].frame;
        targetRect = [NSScreen mainScreen].visibleFrame;
    }

    CGFloat ax = targetRect.origin.x;
    CGFloat ay = targetRect.origin.y;
    CGFloat aw = targetRect.size.width;
    CGFloat ah = targetRect.size.height;

    CGFloat px, py; 
    int col = opts.anchor % 3; 
    if (col == 0) px = ax;
    else if (col == 1) px = ax + aw / 2;
    else px = ax + aw;

    int row = opts.anchor / 3; 
    if (row == 0) py = ay + ah; 
    else if (row == 1) py = ay + ah / 2; 
    else py = ay; 

    CGFloat ox = 0;
    CGFloat oy = 0;
    CGFloat ow = windowWidth;
    CGFloat oh = windowHeight;

    if (col == 0) ox = 0;           
    else if (col == 1) ox = -ow/2;  
    else ox = -ow;                  

    if (row == 0) oy = -oh;         
    else if (row == 1) oy = -oh/2;  
    else oy = 0;                    

    CGFloat finalX = px + ox + opts.offsetX;
    CGFloat finalY = py + oy + opts.offsetY;
    if (preserveLiveFollowFrame) {
        finalX = liveFollowOrigin.x;
        finalY = liveFollowOrigin.y;
    }
    if (opts.stickyWindowPid > 0 && !preserveLiveFollowFrame && CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft)) {
        // Optimization: layout refresh can detect mouse-down before the first AX
        // move notification. Seeding the predictive anchor here gives the live
        // poller a usable baseline instead of waiting for the low-frequency AX
        // movement event.
        [self refreshStickyPredictiveAnchorWithTargetRect:targetRect source:targetSource debug:NO];
    }

    [self setFrame:NSMakeRect(finalX, finalY, windowWidth, windowHeight) display:YES];
    self.backgroundView.frame = self.contentView.bounds;
    [self setCornerRadius:opts.transparent ? 0 : 10.0];
    
    // 4. Auto Close (Timer)
    [self startAutoCloseTimer:(NSTimeInterval)opts.autoCloseSeconds];
    
    // 5. Store options and setup window tracking
    self.currentOpts = opts;
    if (opts.stickyWindowPid > 0) {
        [self startTrackingWindowWithPid:opts.stickyWindowPid];
        // Optimization: animation/content refreshes can observe the mouse-down
        // state before the first coalesced AX move notification arrives. Starting
        // live follow from this generic refresh path reduces the initial sticky
        // lag without requiring callers to know when native dragging begins.
        [self startStickyLiveFollowTimerIfNeeded];
    } else {
        [self stopTrackingWindow];
    }

    BOOL shouldLogLayout = opts.stickyWindowPid > 0 && (self.layoutUpdateCount <= 5 || self.layoutUpdateCount % 10 == 0 || layoutIntervalMs > 150.0);
    if (shouldLogLayout) {
        // Diagnostics: ShowOverlay can be called independently from sticky move
        // notifications. Sampling this path separates animation/layout refresh
        // cost from native window-follow cost when investigating drag lag.
        double elapsedMs = (CACurrentMediaTime() - layoutStart) * 1000.0;
        OverlayDebugLog([NSString stringWithFormat:@"sticky-layout name=%@ pid=%d count=%llu intervalMs=%.2f elapsedMs=%.2f source=%@ frame=(%.1f,%.1f %.1fx%.1f) transparent=%@ icon=(%.1f,%.1f %.1fx%.1f)",
                         self.name ?: @"", opts.stickyWindowPid, self.layoutUpdateCount, layoutIntervalMs, elapsedMs, targetSource,
                         self.frame.origin.x, self.frame.origin.y, self.frame.size.width, self.frame.size.height,
                         opts.transparent ? @"true" : @"false", opts.iconX, opts.iconY, opts.iconWidth, opts.iconHeight]);
    }
}

// AXObserver callback - called when tracked window moves or resizes
static void axObserverCallback(AXObserverRef observer, AXUIElementRef element, CFStringRef notification, void *refcon) {
    OverlayWindow *win = (__bridge OverlayWindow *)refcon;
    [win handleTrackedWindowMoved];
}

- (void)handleTrackedWindowMoved {
    CFTimeInterval eventStart = CACurrentMediaTime();
    double eventIntervalMs = -1;
    if (self.lastStickyMoveEventTime > 0) {
        eventIntervalMs = (eventStart - self.lastStickyMoveEventTime) * 1000.0;
    }
    self.lastStickyMoveEventTime = eventStart;
    self.stickyMoveEventCount++;
    BOOL shouldLog = self.stickyMoveEventCount <= 5 || self.stickyMoveEventCount % 10 == 0 || eventIntervalMs > 50.0;

    // Sticky overlays are a generic base capability. The earlier implementation
    // hid overlays until the user released the target window, which made attached
    // surfaces flicker and lag. Always live-follow here so every module gets the
    // same stable window attachment behavior.
    BOOL updated = [self updatePositionFromTrackedWindowWithDebug:shouldLog eventIntervalMs:eventIntervalMs];
    [self startStickyLiveFollowTimerIfNeeded];
    [self orderRelativeToStickyWindow];
    self.alphaValue = 1.0;
    [self hideTooltipWindow];

    if (shouldLog) {
        double totalMs = (CACurrentMediaTime() - eventStart) * 1000.0;
        OverlayDebugLog([NSString stringWithFormat:@"sticky-move event name=%@ pid=%d count=%llu intervalMs=%.2f updated=%@ totalMs=%.2f frame=(%.1f,%.1f %.1fx%.1f)",
                         self.name ?: @"", self.currentOpts.stickyWindowPid, self.stickyMoveEventCount, eventIntervalMs,
                         updated ? @"true" : @"false", totalMs, self.frame.origin.x, self.frame.origin.y, self.frame.size.width, self.frame.size.height]);
    }
}

- (void)startStickyLiveFollowTimerIfNeeded {
    if (self.stickyLiveFollowTimer || self.currentOpts.stickyWindowPid <= 0) {
        return;
    }
    if (!CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft)) {
        return;
    }

    self.stickyLiveFollowPollCount = 0;
    // Bug fix: AX moved notifications are coalesced by macOS and can arrive only
    // every 90-120ms while the target window is dragged. Polling during the active
    // drag keeps sticky overlays attached at frame cadence without changing the
    // generic overlay API or adding module-specific follow modes.
    self.stickyLiveFollowTimer = [NSTimer timerWithTimeInterval:(1.0 / 60.0)
                                                         target:self
                                                       selector:@selector(handleStickyLiveFollowTimer:)
                                                       userInfo:nil
                                                        repeats:YES];
    [[NSRunLoop mainRunLoop] addTimer:self.stickyLiveFollowTimer forMode:NSRunLoopCommonModes];
    OverlayDebugLog([NSString stringWithFormat:@"sticky-live-poll started name=%@ pid=%d", self.name ?: @"", self.currentOpts.stickyWindowPid]);
}

- (void)stopStickyLiveFollowTimerWithReason:(NSString *)reason {
    if (!self.stickyLiveFollowTimer) {
        return;
    }
    [self.stickyLiveFollowTimer invalidate];
    self.stickyLiveFollowTimer = nil;
    self.hasStickyPredictiveAnchor = NO;
    OverlayDebugLog([NSString stringWithFormat:@"sticky-live-poll stopped name=%@ pid=%d reason=%@ count=%llu",
                     self.name ?: @"", self.currentOpts.stickyWindowPid, reason ?: @"unknown", self.stickyLiveFollowPollCount]);
}

- (void)refreshStickyPredictiveAnchorWithTargetRect:(CGRect)targetRect source:(NSString *)source debug:(BOOL)debug {
    if (!CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft)) {
        return;
    }
    // Predictive follow uses true sticky samples as anchors and then applies
    // mouse deltas between those samples. This keeps the overlay moving at timer
    // cadence even when AX/CG window geometry updates arrive at only ~10Hz.
    self.stickyPredictiveAnchorTargetRect = targetRect;
    self.stickyPredictiveAnchorMouse = [NSEvent mouseLocation];
    self.hasStickyPredictiveAnchor = YES;

    if (debug) {
        OverlayDebugLog([NSString stringWithFormat:@"sticky-predictive anchor name=%@ pid=%d source=%@ target=(%.1f,%.1f %.1fx%.1f) mouse=(%.1f,%.1f)",
                         self.name ?: @"", self.currentOpts.stickyWindowPid, source ?: @"unknown",
                         targetRect.origin.x, targetRect.origin.y, targetRect.size.width, targetRect.size.height,
                         self.stickyPredictiveAnchorMouse.x, self.stickyPredictiveAnchorMouse.y]);
    }
}

- (BOOL)getStickyPredictiveTargetRect:(CGRect *)outRect {
    if (!self.hasStickyPredictiveAnchor || !outRect) {
        return NO;
    }
    NSPoint mouse = [NSEvent mouseLocation];
    CGFloat dx = mouse.x - self.stickyPredictiveAnchorMouse.x;
    CGFloat dy = mouse.y - self.stickyPredictiveAnchorMouse.y;
    *outRect = CGRectOffset(self.stickyPredictiveAnchorTargetRect, dx, dy);
    return YES;
}

- (void)handleStickyLiveFollowTimer:(NSTimer *)timer {
    if (!CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft)) {
        [self stopStickyLiveFollowTimerWithReason:@"mouse-up"];
        return;
    }

    self.stickyLiveFollowPollCount++;
    BOOL shouldLog = self.stickyLiveFollowPollCount <= 5 || self.stickyLiveFollowPollCount % 15 == 0;
    BOOL updated = [self updatePositionFromTrackedWindowWithDebug:shouldLog eventIntervalMs:-1 preferCGWindowList:YES];
    [self orderRelativeToStickyWindow];
    self.alphaValue = 1.0;

    if (shouldLog) {
        OverlayDebugLog([NSString stringWithFormat:@"sticky-live-poll tick name=%@ pid=%d count=%llu updated=%@ frame=(%.1f,%.1f %.1fx%.1f)",
                         self.name ?: @"", self.currentOpts.stickyWindowPid, self.stickyLiveFollowPollCount,
                         updated ? @"true" : @"false", self.frame.origin.x, self.frame.origin.y, self.frame.size.width, self.frame.size.height]);
    }
}

- (void)startTrackingWindowWithPid:(pid_t)pid {
    if (pid > 0 && self.trackedPid == pid && self.axObserver && self.trackedWindow) {
        // Optimization: reused overlays can refresh their content frequently.
        // Reusing the existing AX observer avoids tearing down native tracking on
        // every update, which keeps live-follow reliable for all overlay modules.
        return;
    }

    // Stop any existing tracking first
    [self stopTrackingWindow];
    self.stickyMoveEventCount = 0;
    self.lastStickyMoveEventTime = 0;
    
    // Create AXUIElement for the application
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    if (!app) {
        OverlayDebugLog([NSString stringWithFormat:@"sticky-track failed name=%@ pid=%d reason=create-application", self.name ?: @"", pid]);
        return;
    }
    
    // Get the focused window
    AXUIElementRef frontWindow = NULL;
    AXError err = AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute, (CFTypeRef *)&frontWindow);
    if (err != kAXErrorSuccess || !frontWindow) {
        OverlayDebugLog([NSString stringWithFormat:@"sticky-track failed name=%@ pid=%d reason=focused-window axErr=%d", self.name ?: @"", pid, err]);
        CFRelease(app);
        return;
    }
    
    // Store the tracked window
    self.trackedWindow = frontWindow;
    self.trackedPid = pid;
    
    // Create AXObserver
    AXObserverRef observer = NULL;
    err = AXObserverCreate(pid, axObserverCallback, &observer);
    if (err != kAXErrorSuccess || !observer) {
        OverlayDebugLog([NSString stringWithFormat:@"sticky-track failed name=%@ pid=%d reason=observer axErr=%d", self.name ?: @"", pid, err]);
        CFRelease(app);
        CFRelease(frontWindow);
        self.trackedWindow = NULL;
        self.trackedPid = 0;
        return;
    }
    
    self.axObserver = observer;
    
    // Add notifications for window movement and resize
    AXObserverAddNotification(observer, frontWindow, kAXMovedNotification, (__bridge void *)self);
    AXObserverAddNotification(observer, frontWindow, kAXResizedNotification, (__bridge void *)self);
    
    // Add observer to run loop
    CFRunLoopAddSource(CFRunLoopGetMain(), AXObserverGetRunLoopSource(observer), kCFRunLoopDefaultMode);
    
    OverlayDebugLog([NSString stringWithFormat:@"sticky-track started name=%@ pid=%d windowNumber=%u", self.name ?: @"", pid, self.stickyWindowNumber]);

    CFRelease(app);
}

- (BOOL)updatePositionFromTrackedWindow {
    return [self updatePositionFromTrackedWindowWithDebug:NO eventIntervalMs:-1];
}

- (BOOL)updatePositionFromTrackedWindowWithDebug:(BOOL)debug eventIntervalMs:(double)eventIntervalMs {
    return [self updatePositionFromTrackedWindowWithDebug:debug eventIntervalMs:eventIntervalMs preferCGWindowList:NO];
}

- (BOOL)updatePositionFromTrackedWindowWithDebug:(BOOL)debug eventIntervalMs:(double)eventIntervalMs preferCGWindowList:(BOOL)preferCGWindowList {
    if (self.currentOpts.stickyWindowPid <= 0) return NO;

    CFTimeInterval start = CACurrentMediaTime();

    CGRect targetRect;
    BOOL targetFound = NO;
    NSString *source = @"none";
    CFTypeRef posVal = NULL, sizeVal = NULL;
    CGPoint pos; CGSize size;
    BOOL preserveSmallPredictiveCorrection = self.stickyLiveFollowTimer != nil &&
                                             self.hasStickyPredictiveAnchor &&
                                             !preferCGWindowList &&
                                             CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft);
    NSPoint predictedOriginBeforeRealSample = self.frame.origin;

    if (preferCGWindowList && [self getStickyPredictiveTargetRect:&targetRect]) {
        targetFound = YES;
        source = @"predictive-mouse";
    }

    if (!targetFound && preferCGWindowList) {
        // Bug fix: during live dragging, AX position attributes can stay stale
        // between coalesced move notifications. CGWindowList is queried first for
        // the polling path because it reflects compositor window bounds without
        // waiting for the next AX notification.
        if ([self getWindowFrameForPid:(pid_t)self.currentOpts.stickyWindowPid outRect:&targetRect]) {
            targetFound = YES;
            source = @"cg-window-list-live";
        }
    }

    if (!targetFound && self.trackedWindow) {
        AXError posErr = AXUIElementCopyAttributeValue(self.trackedWindow, kAXPositionAttribute, &posVal);
        AXError sizeErr = AXUIElementCopyAttributeValue(self.trackedWindow, kAXSizeAttribute, &sizeVal);
        if (posErr == kAXErrorSuccess && sizeErr == kAXErrorSuccess && posVal && sizeVal) {
            AXValueGetValue(posVal, kAXValueCGPointType, &pos);
            AXValueGetValue(sizeVal, kAXValueCGSizeType, &size);

            // Bug fix: use the tracked AX window instead of asking for the current
            // focused window on every move. During drag, focus can lag behind the
            // window geometry, while the observer element is the window that moved.
            CGFloat mainScreenH = [NSScreen mainScreen].frame.size.height;
            CGFloat cocoaY = mainScreenH - pos.y - size.height;
            targetRect = CGRectMake(pos.x, cocoaY, size.width, size.height);
            targetFound = YES;
            source = @"tracked-ax";
        }
        if (posVal) CFRelease(posVal);
        if (sizeVal) CFRelease(sizeVal);
    }

    if (!targetFound) {
        if ([self getWindowFrameForPid:(pid_t)self.currentOpts.stickyWindowPid outRect:&targetRect]) {
            targetFound = YES;
            source = @"cg-window-list";
        } else {
            if (debug) {
                double elapsedMs = (CACurrentMediaTime() - start) * 1000.0;
                OverlayDebugLog([NSString stringWithFormat:@"sticky-position failed name=%@ pid=%d intervalMs=%.2f elapsedMs=%.2f",
                                 self.name ?: @"", self.currentOpts.stickyWindowPid, eventIntervalMs, elapsedMs]);
            }
            return NO;
        }
    }

    if (targetFound && !preferCGWindowList) {
        [self refreshStickyPredictiveAnchorWithTargetRect:targetRect source:source debug:debug];
    }

    // Calculate new position based on anchor
    OverlayOptions opts = self.currentOpts;
    CGFloat ax = targetRect.origin.x;
    CGFloat ay = targetRect.origin.y;
    CGFloat aw = targetRect.size.width;
    CGFloat ah = targetRect.size.height;
    
    CGFloat px, py;
    int col = opts.anchor % 3;
    if (col == 0) px = ax;
    else if (col == 1) px = ax + aw / 2;
    else px = ax + aw;
    
    int row = opts.anchor / 3;
    if (row == 0) py = ay + ah;
    else if (row == 1) py = ay + ah / 2;
    else py = ay;
    
    CGFloat ow = self.frame.size.width;
    CGFloat oh = self.frame.size.height;
    CGFloat ox = 0, oy = 0;
    
    if (col == 0) ox = 0;
    else if (col == 1) ox = -ow/2;
    else ox = -ow;
    
    if (row == 0) oy = -oh;
    else if (row == 1) oy = -oh/2;
    else oy = 0;
    
    CGFloat finalX = px + ox + opts.offsetX;
    CGFloat finalY = py + oy + opts.offsetY;

    if (preserveSmallPredictiveCorrection) {
        CGFloat correctionX = finalX - predictedOriginBeforeRealSample.x;
        CGFloat correctionY = finalY - predictedOriginBeforeRealSample.y;
        if (fabs(correctionX) <= kStickyPredictiveCorrectionThreshold &&
            fabs(correctionY) <= kStickyPredictiveCorrectionThreshold) {
            // Optimization: low-frequency AX samples are still used to refresh the
            // predictive anchor, but small real-sample corrections should not pull
            // the overlay back to an older geometry point during an active drag.
            // Preserving the timer-driven frame removes the visible snap while the
            // threshold still allows large corrections for window snapping, screen
            // edge changes, or other cases where prediction has genuinely drifted.
            finalX = predictedOriginBeforeRealSample.x;
            finalY = predictedOriginBeforeRealSample.y;
            source = [source stringByAppendingString:@"-preserve"];
        }
    }
    
    [self setFrameOrigin:NSMakePoint(finalX, finalY)];
    if (debug) {
        double elapsedMs = (CACurrentMediaTime() - start) * 1000.0;
        OverlayDebugLog([NSString stringWithFormat:@"sticky-position name=%@ pid=%d source=%@ intervalMs=%.2f elapsedMs=%.2f target=(%.1f,%.1f %.1fx%.1f) final=(%.1f,%.1f) overlaySize=(%.1fx%.1f)",
                         self.name ?: @"", self.currentOpts.stickyWindowPid, source, eventIntervalMs, elapsedMs,
                         targetRect.origin.x, targetRect.origin.y, targetRect.size.width, targetRect.size.height,
                         finalX, finalY, self.frame.size.width, self.frame.size.height]);
    }
    return YES;
}

@end

// -----------------------------------------------------------------------------
// C Exported Functions
// -----------------------------------------------------------------------------

void ShowOverlay(OverlayOptions opts) {
    @autoreleasepool {
        if (!gOverlayWindows) {
            gOverlayWindows = [[NSMutableDictionary alloc] init];
        }

        NSString *key = [NSString stringWithUTF8String:opts.name];
        OverlayWindow *win = [gOverlayWindows objectForKey:key];
        
        if (!win) {
            // Create new
            NSRect frame = NSZeroRect; // Will be set in updateLayout
            win = [[OverlayWindow alloc] initWithContentRect:frame 
                                                   styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel 
                                                     backing:NSBackingStoreBuffered 
                                                       defer:NO];
            win.name = key;
            [gOverlayWindows setObject:win forKey:key];
        }

        [win updateLayoutWithOptions:opts];
        [win orderRelativeToStickyWindow];
        win.alphaValue = 1.0;
    }
}

void CloseOverlay(char* name) {
    @autoreleasepool {
        if (!gOverlayWindows) return;
        NSString *key = [NSString stringWithUTF8String:name];
        OverlayWindow *win = [gOverlayWindows objectForKey:key];
        if (win) {
            // Don't close if user is dragging the overlay
            if (win.isDragging) return;
            [win stopTrackingWindow];
            [win hideTooltipWindow];
            [win close];
            [gOverlayWindows removeObjectForKey:key];
        }
    }
}

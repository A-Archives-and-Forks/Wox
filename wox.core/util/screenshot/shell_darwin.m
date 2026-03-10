#import <Cocoa/Cocoa.h>
#import <Foundation/Foundation.h>

typedef struct {
  double selectionX;
  double selectionY;
  double selectionWidth;
  double selectionHeight;
  double toolbarAnchorX;
  double toolbarAnchorY;
  double propertiesAnchorX;
  double propertiesAnchorY;
  double strokeWidth;
  double fontSize;
  int state;
  int showToolbar;
  int showProperties;
  int activeHandle;
  int cursor;
  int activeTool;
  int strokeColorR;
  int strokeColorG;
  int strokeColorB;
  int strokeColorA;
  int textColorR;
  int textColorG;
  int textColorB;
  int textColorA;
  int canUndo;
  int canRedo;
  int allowConfirm;
} ScreenshotShellViewModel;

void screenshotShellMouseDownCGO(char *sessionID, double x, double y, int button);
void screenshotShellMouseMoveCGO(char *sessionID, double x, double y);
void screenshotShellMouseUpCGO(char *sessionID, double x, double y, int button);
void screenshotShellKeyDownCGO(char *sessionID, char *key);
void screenshotShellToolSelectedCGO(char *sessionID, int tool);
void screenshotShellToolbarActionCGO(char *sessionID, int action);
void screenshotShellPropertyFloatChangedCGO(char *sessionID, char *name, double value);
void screenshotShellPropertyColorChangedCGO(char *sessionID, char *name, int r, int g, int b, int a);
void screenshotShellClosedCGO(char *sessionID);

@interface WoxScreenshotOverlayView : NSView
@property(nonatomic, copy) NSString *sessionID;
@property(nonatomic, assign) NSRect selectionRect;
@property(nonatomic, assign) BOOL hasSelection;
@property(nonatomic, assign) CGFloat minX;
@property(nonatomic, assign) CGFloat maxY;
@property(nonatomic, assign) CGFloat logicalX;
@property(nonatomic, assign) CGFloat logicalY;
@property(nonatomic, assign) NSInteger activeHandle;
@property(nonatomic, assign) NSInteger cursorCode;
@end

@interface WoxScreenshotToolbarView : NSView
@property(nonatomic, copy) NSString *sessionID;
@property(nonatomic, strong) NSMutableDictionary<NSNumber *, NSButton *> *toolButtons;
@property(nonatomic, strong) NSButton *undoButton;
@property(nonatomic, strong) NSButton *redoButton;
@property(nonatomic, strong) NSButton *cancelButton;
@property(nonatomic, strong) NSButton *confirmButton;
- (void)applyViewModel:(ScreenshotShellViewModel)viewModel;
@end

@interface WoxScreenshotPropertiesView : NSView
@property(nonatomic, copy) NSString *sessionID;
@property(nonatomic, strong) NSTextField *titleLabel;
@property(nonatomic, strong) NSColorWell *colorWell;
@property(nonatomic, strong) NSTextField *metricLabel;
@property(nonatomic, strong) NSSlider *metricSlider;
@property(nonatomic, strong) NSTextField *metricValueLabel;
@property(nonatomic, assign) NSInteger activeTool;
- (void)applyViewModel:(ScreenshotShellViewModel)viewModel;
@end

@interface WoxScreenshotOverlayWindow : NSWindow
@end

@interface WoxScreenshotChromeWindow : NSPanel
@end

@implementation WoxScreenshotOverlayWindow

- (BOOL)canBecomeKeyWindow {
  return YES;
}

- (BOOL)canBecomeMainWindow {
  return YES;
}

@end

@implementation WoxScreenshotChromeWindow

- (BOOL)canBecomeKeyWindow {
  return YES;
}

- (BOOL)canBecomeMainWindow {
  return YES;
}

@end

@implementation WoxScreenshotOverlayView

- (BOOL)acceptsFirstResponder {
  return YES;
}

- (BOOL)isFlipped {
  return NO;
}

- (NSPoint)normalizedPointForEvent:(NSEvent *)event {
  NSPoint pointInWindow = [event locationInWindow];
  NSPoint screenPoint = NSMakePoint(self.window.frame.origin.x + pointInWindow.x,
                                    self.window.frame.origin.y + pointInWindow.y);
  CGFloat normalizedX = screenPoint.x - self.minX;
  CGFloat normalizedY = self.maxY - screenPoint.y;
  return NSMakePoint(normalizedX, normalizedY);
}

- (void)mouseDown:(NSEvent *)event {
  NSPoint point = [self normalizedPointForEvent:event];
  screenshotShellMouseDownCGO((char *)[self.sessionID UTF8String], point.x,
                              point.y, 1);
}

- (void)mouseDragged:(NSEvent *)event {
  NSPoint point = [self normalizedPointForEvent:event];
  screenshotShellMouseMoveCGO((char *)[self.sessionID UTF8String], point.x,
                              point.y);
}

- (void)mouseMoved:(NSEvent *)event {
  NSPoint point = [self normalizedPointForEvent:event];
  screenshotShellMouseMoveCGO((char *)[self.sessionID UTF8String], point.x,
                              point.y);
}

- (void)resetCursorRects {
  [self discardCursorRects];
  [self addCursorRect:self.bounds cursor:[self cursorForCode]];
}

- (void)mouseUp:(NSEvent *)event {
  NSPoint point = [self normalizedPointForEvent:event];
  screenshotShellMouseUpCGO((char *)[self.sessionID UTF8String], point.x,
                            point.y, 1);
}

- (void)keyDown:(NSEvent *)event {
  if ([event keyCode] == 53) {
    screenshotShellKeyDownCGO((char *)[self.sessionID UTF8String], "Escape");
    return;
  }
  if ([event keyCode] == 36) {
    screenshotShellKeyDownCGO((char *)[self.sessionID UTF8String], "Return");
    return;
  }

  NSString *characters = [event charactersIgnoringModifiers];
  if (characters != nil && [characters length] > 0) {
    screenshotShellKeyDownCGO((char *)[self.sessionID UTF8String],
                              (char *)[characters UTF8String]);
  }
}

- (void)drawRect:(NSRect)dirtyRect {
  [super drawRect:dirtyRect];

  [[NSColor colorWithCalibratedWhite:0 alpha:0.45] setFill];
  NSRectFill(self.bounds);

  if (!self.hasSelection || NSIsEmptyRect(self.selectionRect)) {
    return;
  }

  NSBezierPath *overlayPath = [NSBezierPath bezierPathWithRect:self.bounds];
  NSBezierPath *selectionPath = [NSBezierPath bezierPathWithRect:self.selectionRect];
  [overlayPath appendBezierPath:selectionPath];
  [overlayPath setWindingRule:NSWindingRuleEvenOdd];
  [[NSColor clearColor] setFill];
  [overlayPath fill];

  [[NSColor colorWithCalibratedRed:0 green:1 blue:0.63 alpha:1] setStroke];
  [selectionPath setLineWidth:2.0];
  [selectionPath stroke];

  [self drawSelectionHandles];

  NSString *label = [NSString stringWithFormat:@"%.0f x %.0f",
                                               self.selectionRect.size.width,
                                               self.selectionRect.size.height];
  NSDictionary *attributes = @{
    NSForegroundColorAttributeName : [NSColor whiteColor],
    NSFontAttributeName : [NSFont boldSystemFontOfSize:18]
  };
  [label drawAtPoint:NSMakePoint(NSMinX(self.selectionRect),
                                 NSMaxY(self.selectionRect) + 8)
      withAttributes:attributes];
}

- (void)drawSelectionHandles {
  NSArray<NSValue *> *points = @[
    [NSValue valueWithPoint:NSMakePoint(NSMinX(self.selectionRect), NSMinY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMidX(self.selectionRect), NSMinY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMaxX(self.selectionRect), NSMinY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMinX(self.selectionRect), NSMidY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMaxX(self.selectionRect), NSMidY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMinX(self.selectionRect), NSMaxY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMidX(self.selectionRect), NSMaxY(self.selectionRect))],
    [NSValue valueWithPoint:NSMakePoint(NSMaxX(self.selectionRect), NSMaxY(self.selectionRect))]
  ];
  NSArray<NSNumber *> *handles = @[@7, @2, @6, @5, @4, @9, @3, @8];

  for (NSUInteger i = 0; i < [points count]; i++) {
    NSPoint point = [points[i] pointValue];
    NSInteger handle = [handles[i] integerValue];
    BOOL isActive = self.activeHandle == handle;
    NSRect handleRect = NSMakeRect(point.x - 4, point.y - 4, 8, 8);
    NSBezierPath *path = [NSBezierPath bezierPathWithRoundedRect:handleRect xRadius:2 yRadius:2];

    if (isActive) {
      [[NSColor colorWithCalibratedRed:0 green:1 blue:0.63 alpha:1] setFill];
    } else {
      [[NSColor whiteColor] setFill];
    }
    [path fill];

    [[NSColor colorWithCalibratedRed:0 green:0.36 blue:0.24 alpha:1] setStroke];
    [path setLineWidth:1.0];
    [path stroke];
  }
}

- (NSCursor *)cursorForCode {
  switch (self.cursorCode) {
  case 1:
    return [NSCursor crosshairCursor];
  case 2:
    return [NSCursor openHandCursor];
  case 3:
    return [NSCursor resizeUpDownCursor];
  case 4:
    return [NSCursor resizeLeftRightCursor];
  case 5:
  case 6:
    return [NSCursor crosshairCursor];
  case 7:
    return [NSCursor IBeamCursor];
  default:
    return [NSCursor arrowCursor];
  }
}

@end

@implementation WoxScreenshotToolbarView

- (instancetype)initWithFrame:(NSRect)frameRect {
  self = [super initWithFrame:frameRect];
  if (self == nil) {
    return nil;
  }

  self.wantsLayer = YES;
  self.layer.cornerRadius = 12.0;
  self.layer.backgroundColor = [[NSColor colorWithCalibratedRed:0.09 green:0.11 blue:0.19 alpha:0.94] CGColor];
  self.layer.borderWidth = 1.0;
  self.layer.borderColor = [[NSColor colorWithCalibratedRed:0.21 green:0.26 blue:0.40 alpha:0.65] CGColor];
  self.layer.shadowColor = [[NSColor colorWithCalibratedWhite:0 alpha:0.35] CGColor];
  self.layer.shadowOpacity = 1.0;
  self.layer.shadowRadius = 16.0;
  self.layer.shadowOffset = NSMakeSize(0, -2);

  self.toolButtons = [[NSMutableDictionary alloc] init];

  NSArray<NSDictionary *> *items = @[
    @{@"title" : @"Select", @"tag" : @1},
    @{@"title" : @"Rect", @"tag" : @2},
    @{@"title" : @"Arrow", @"tag" : @3},
    @{@"title" : @"Pen", @"tag" : @4},
    @{@"title" : @"Text", @"tag" : @5}
  ];

  CGFloat x = 12;
  CGFloat y = 10;
  CGFloat buttonHeight = 28;
  CGFloat toolWidth = 58;

  for (NSDictionary *item in items) {
    NSButton *button = [NSButton buttonWithTitle:item[@"title"] target:self action:@selector(toolButtonPressed:)];
    button.frame = NSMakeRect(x, y, toolWidth, buttonHeight);
    button.bordered = NO;
    button.wantsLayer = YES;
    button.layer.cornerRadius = 8.0;
    button.layer.backgroundColor = [[NSColor clearColor] CGColor];
    button.tag = [item[@"tag"] integerValue];
    button.font = [NSFont systemFontOfSize:12 weight:NSFontWeightMedium];
    [self applyButtonStyle:button
                titleColor:[NSColor colorWithCalibratedWhite:1 alpha:0.90]
           backgroundColor:[NSColor clearColor]];
    [self addSubview:button];
    self.toolButtons[@(button.tag)] = button;
    x += toolWidth + 8;
  }

  x += 10;
  self.undoButton = [self toolbarButtonWithTitle:@"Undo" tag:1 x:x y:y];
  x += 56;
  self.redoButton = [self toolbarButtonWithTitle:@"Redo" tag:2 x:x y:y];
  x += 56;
  self.cancelButton = [self toolbarButtonWithTitle:@"Cancel" tag:3 x:x y:y];
  x += 64;
  self.confirmButton = [self toolbarButtonWithTitle:@"Confirm" tag:4 x:x y:y];

  return self;
}

- (NSButton *)toolbarButtonWithTitle:(NSString *)title tag:(NSInteger)tag x:(CGFloat)x y:(CGFloat)y {
  NSButton *button = [NSButton buttonWithTitle:title target:self action:@selector(toolbarActionPressed:)];
  button.frame = NSMakeRect(x, y, 52, 28);
  if ([title isEqualToString:@"Cancel"]) {
    button.frame = NSMakeRect(x, y, 60, 28);
  }
  if ([title isEqualToString:@"Confirm"]) {
    button.frame = NSMakeRect(x, y, 72, 28);
  }
  button.bordered = NO;
  button.wantsLayer = YES;
  button.layer.cornerRadius = 8.0;
  button.layer.backgroundColor = [[NSColor clearColor] CGColor];
  button.tag = tag;
  button.font = [NSFont systemFontOfSize:12 weight:NSFontWeightMedium];
  [self applyButtonStyle:button
              titleColor:[NSColor colorWithCalibratedWhite:1 alpha:0.90]
         backgroundColor:[NSColor clearColor]];
  [self addSubview:button];
  return button;
}

- (void)applyButtonStyle:(NSButton *)button titleColor:(NSColor *)titleColor backgroundColor:(NSColor *)backgroundColor {
  NSDictionary *attributes = @{
    NSForegroundColorAttributeName : titleColor,
    NSFontAttributeName : button.font ?: [NSFont systemFontOfSize:12 weight:NSFontWeightMedium]
  };
  button.attributedTitle = [[NSAttributedString alloc] initWithString:button.title attributes:attributes];
  button.layer.backgroundColor = backgroundColor.CGColor;
}

- (void)toolButtonPressed:(NSButton *)sender {
  screenshotShellToolSelectedCGO((char *)[self.sessionID UTF8String], (int)sender.tag);
}

- (void)toolbarActionPressed:(NSButton *)sender {
  screenshotShellToolbarActionCGO((char *)[self.sessionID UTF8String], (int)sender.tag);
}

- (void)applyViewModel:(ScreenshotShellViewModel)viewModel {
  NSColor *accentColor = [NSColor colorWithCalibratedRed:0.18 green:0.95 blue:0.67 alpha:1];
  NSColor *accentBackground = [NSColor colorWithCalibratedRed:0.12 green:0.31 blue:0.26 alpha:0.55];
  NSColor *defaultTextColor = [NSColor colorWithCalibratedWhite:1 alpha:0.88];
  NSColor *disabledTextColor = [NSColor colorWithCalibratedWhite:1 alpha:0.32];

  for (NSNumber *key in self.toolButtons) {
    NSButton *button = self.toolButtons[key];
    BOOL isActive = [key integerValue] == viewModel.activeTool;
    [self applyButtonStyle:button
                titleColor:isActive ? accentColor : defaultTextColor
           backgroundColor:isActive ? accentBackground : [NSColor clearColor]];
  }

  self.undoButton.enabled = viewModel.canUndo != 0;
  self.redoButton.enabled = viewModel.canRedo != 0;
  self.confirmButton.enabled = viewModel.allowConfirm != 0;

  [self applyButtonStyle:self.undoButton
              titleColor:self.undoButton.enabled ? defaultTextColor : disabledTextColor
         backgroundColor:[NSColor clearColor]];
  [self applyButtonStyle:self.redoButton
              titleColor:self.redoButton.enabled ? defaultTextColor : disabledTextColor
         backgroundColor:[NSColor clearColor]];
  [self applyButtonStyle:self.cancelButton
              titleColor:[NSColor colorWithCalibratedRed:1 green:0.44 blue:0.48 alpha:0.96]
         backgroundColor:[NSColor clearColor]];
  [self applyButtonStyle:self.confirmButton
              titleColor:self.confirmButton.enabled ? accentColor : disabledTextColor
         backgroundColor:self.confirmButton.enabled ? accentBackground : [NSColor clearColor]];
}

@end

@implementation WoxScreenshotPropertiesView

- (instancetype)initWithFrame:(NSRect)frameRect {
  self = [super initWithFrame:frameRect];
  if (self == nil) {
    return nil;
  }

  self.wantsLayer = YES;
  self.layer.cornerRadius = 12.0;
  self.layer.backgroundColor = [[NSColor colorWithCalibratedWhite:0.08 alpha:0.92] CGColor];
  self.layer.borderWidth = 1.0;
  self.layer.borderColor = [[NSColor colorWithCalibratedWhite:1 alpha:0.08] CGColor];

  self.titleLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(14, 88, 180, 18)];
  self.titleLabel.editable = NO;
  self.titleLabel.selectable = NO;
  self.titleLabel.drawsBackground = NO;
  self.titleLabel.bezeled = NO;
  self.titleLabel.font = [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold];
  self.titleLabel.textColor = [NSColor whiteColor];
  [self addSubview:self.titleLabel];

  NSTextField *colorLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(14, 56, 44, 18)];
  colorLabel.stringValue = @"Color";
  colorLabel.editable = NO;
  colorLabel.selectable = NO;
  colorLabel.drawsBackground = NO;
  colorLabel.bezeled = NO;
  colorLabel.font = [NSFont systemFontOfSize:12];
  colorLabel.textColor = [NSColor colorWithCalibratedWhite:0.9 alpha:1];
  [self addSubview:colorLabel];

  self.colorWell = [[NSColorWell alloc] initWithFrame:NSMakeRect(68, 50, 56, 28)];
  self.colorWell.target = self;
  self.colorWell.action = @selector(colorChanged:);
  [self addSubview:self.colorWell];

  self.metricLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(14, 24, 54, 18)];
  self.metricLabel.editable = NO;
  self.metricLabel.selectable = NO;
  self.metricLabel.drawsBackground = NO;
  self.metricLabel.bezeled = NO;
  self.metricLabel.font = [NSFont systemFontOfSize:12];
  self.metricLabel.textColor = [NSColor colorWithCalibratedWhite:0.9 alpha:1];
  [self addSubview:self.metricLabel];

  self.metricSlider = [[NSSlider alloc] initWithFrame:NSMakeRect(68, 20, 110, 24)];
  self.metricSlider.target = self;
  self.metricSlider.action = @selector(metricChanged:);
  [self addSubview:self.metricSlider];

  self.metricValueLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(182, 24, 40, 18)];
  self.metricValueLabel.editable = NO;
  self.metricValueLabel.selectable = NO;
  self.metricValueLabel.drawsBackground = NO;
  self.metricValueLabel.bezeled = NO;
  self.metricValueLabel.font = [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightMedium];
  self.metricValueLabel.alignment = NSTextAlignmentRight;
  self.metricValueLabel.textColor = [NSColor whiteColor];
  [self addSubview:self.metricValueLabel];

  return self;
}

- (void)colorChanged:(NSColorWell *)sender {
  NSColor *color = [[sender color] colorUsingColorSpace:[NSColorSpace deviceRGBColorSpace]];
  if (color == nil) {
    return;
  }

  const char *propertyName = self.activeTool == 5 ? "text_color" : "stroke_color";
  screenshotShellPropertyColorChangedCGO((char *)[self.sessionID UTF8String], (char *)propertyName,
                                         (int)lrint(color.redComponent * 255.0),
                                         (int)lrint(color.greenComponent * 255.0),
                                         (int)lrint(color.blueComponent * 255.0),
                                         (int)lrint(color.alphaComponent * 255.0));
}

- (void)metricChanged:(NSSlider *)sender {
  const char *propertyName = self.activeTool == 5 ? "font_size" : "stroke_width";
  screenshotShellPropertyFloatChangedCGO((char *)[self.sessionID UTF8String], (char *)propertyName, sender.doubleValue);
}

- (void)applyViewModel:(ScreenshotShellViewModel)viewModel {
  self.activeTool = viewModel.activeTool;

  BOOL isTextTool = viewModel.activeTool == 5;
  self.titleLabel.stringValue = isTextTool ? @"Text" : @"Style";
  self.metricLabel.stringValue = isTextTool ? @"Font" : @"Width";

  if (isTextTool) {
    self.colorWell.color = [NSColor colorWithCalibratedRed:viewModel.textColorR / 255.0
                                                     green:viewModel.textColorG / 255.0
                                                      blue:viewModel.textColorB / 255.0
                                                     alpha:viewModel.textColorA / 255.0];
    self.metricSlider.minValue = 10.0;
    self.metricSlider.maxValue = 72.0;
    self.metricSlider.doubleValue = MAX(10.0, viewModel.fontSize);
    self.metricValueLabel.stringValue = [NSString stringWithFormat:@"%.0f", self.metricSlider.doubleValue];
  } else {
    self.colorWell.color = [NSColor colorWithCalibratedRed:viewModel.strokeColorR / 255.0
                                                     green:viewModel.strokeColorG / 255.0
                                                      blue:viewModel.strokeColorB / 255.0
                                                     alpha:viewModel.strokeColorA / 255.0];
    self.metricSlider.minValue = 1.0;
    self.metricSlider.maxValue = 24.0;
    self.metricSlider.doubleValue = MAX(1.0, viewModel.strokeWidth);
    self.metricValueLabel.stringValue = [NSString stringWithFormat:@"%.0f", self.metricSlider.doubleValue];
  }
}

@end

@interface WoxScreenshotWindowDelegate : NSObject <NSWindowDelegate>
@property(nonatomic, copy) NSString *sessionID;
@end

@implementation WoxScreenshotWindowDelegate

- (void)windowWillClose:(NSNotification *)notification {
  screenshotShellClosedCGO((char *)[self.sessionID UTF8String]);
}

@end

@interface WoxScreenshotShellController : NSObject
@property(nonatomic, strong) NSMutableArray<NSWindow *> *windows;
@property(nonatomic, strong) NSMutableArray<WoxScreenshotOverlayView *> *overlayViews;
@property(nonatomic, strong) NSMutableArray<WoxScreenshotWindowDelegate *> *delegates;
@property(nonatomic, strong) WoxScreenshotChromeWindow *toolbarWindow;
@property(nonatomic, strong) WoxScreenshotToolbarView *toolbarView;
@property(nonatomic, strong) WoxScreenshotChromeWindow *propertiesWindow;
@property(nonatomic, strong) WoxScreenshotPropertiesView *propertiesView;
@property(nonatomic, copy) NSString *sessionID;
@property(nonatomic, assign) CGFloat minX;
@property(nonatomic, assign) CGFloat maxY;
@property(nonatomic, assign) NSRect virtualFrame;
@property(nonatomic, strong) id keyMonitor;
@end

@implementation WoxScreenshotShellController
@end

static NSMutableDictionary<NSString *, WoxScreenshotShellController *> *controllers;

static void ensureControllers(void) {
  static dispatch_once_t onceToken;
  dispatch_once(&onceToken, ^{
    controllers = [[NSMutableDictionary alloc] init];
  });
}

static NSRect computeVirtualFrame(CGFloat *minX, CGFloat *maxY) {
  NSArray<NSScreen *> *screens = [NSScreen screens];
  CGFloat localMinX = 0;
  CGFloat localMinY = 0;
  CGFloat localMaxX = 0;
  CGFloat localMaxY = 0;
  BOOL initialized = NO;

  for (NSScreen *screen in screens) {
    NSRect frame = [screen frame];
    if (!initialized) {
      localMinX = NSMinX(frame);
      localMinY = NSMinY(frame);
      localMaxX = NSMaxX(frame);
      localMaxY = NSMaxY(frame);
      initialized = YES;
      continue;
    }

    localMinX = MIN(localMinX, NSMinX(frame));
    localMinY = MIN(localMinY, NSMinY(frame));
    localMaxX = MAX(localMaxX, NSMaxX(frame));
    localMaxY = MAX(localMaxY, NSMaxY(frame));
  }

  if (minX != NULL) {
    *minX = localMinX;
  }
  if (maxY != NULL) {
    *maxY = localMaxY;
  }

  return NSMakeRect(localMinX, localMinY, localMaxX - localMinX,
                    localMaxY - localMinY);
}

static NSRect logicalFrameForScreen(NSScreen *screen, CGFloat minX, CGFloat maxY) {
  NSRect frame = [screen frame];
  return NSMakeRect(frame.origin.x - minX, maxY - NSMaxY(frame), frame.size.width,
                    frame.size.height);
}

static NSRect intersectionForLogicalRects(NSRect a, NSRect b) {
  CGFloat left = MAX(NSMinX(a), NSMinX(b));
  CGFloat top = MAX(NSMinY(a), NSMinY(b));
  CGFloat right = MIN(NSMaxX(a), NSMaxX(b));
  CGFloat bottom = MIN(NSMaxY(a), NSMaxY(b));

  if (right <= left || bottom <= top) {
    return NSZeroRect;
  }

  return NSMakeRect(left, top, right - left, bottom - top);
}

static NSRect selectionRectFromViewModel(ScreenshotShellViewModel viewModel, WoxScreenshotOverlayView *view) {
  if (viewModel.selectionWidth <= 0 || viewModel.selectionHeight <= 0) {
    return NSZeroRect;
  }

  NSRect selectionLogicalRect = NSMakeRect(viewModel.selectionX, viewModel.selectionY,
                                           viewModel.selectionWidth, viewModel.selectionHeight);
  NSRect screenLogicalRect = NSMakeRect(view.logicalX, view.logicalY, view.bounds.size.width,
                                        view.bounds.size.height);
  NSRect intersection = intersectionForLogicalRects(selectionLogicalRect, screenLogicalRect);
  if (NSIsEmptyRect(intersection)) {
    return NSZeroRect;
  }

  CGFloat localX = intersection.origin.x - view.logicalX;
  CGFloat localTopY = intersection.origin.y - view.logicalY;
  CGFloat localY = view.bounds.size.height - localTopY - intersection.size.height;
  return NSMakeRect(localX, localY, intersection.size.width, intersection.size.height);
}

static WoxScreenshotChromeWindow *createChromeWindow(NSRect frame, NSView *contentView) {
  WoxScreenshotChromeWindow *window =
      [[WoxScreenshotChromeWindow alloc] initWithContentRect:frame
                                                   styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                                                     backing:NSBackingStoreBuffered
                                                       defer:NO];
  [window setOpaque:NO];
  [window setBackgroundColor:[NSColor clearColor]];
  [window setLevel:NSScreenSaverWindowLevel + 1];
  [window setReleasedWhenClosed:NO];
  [window setHidesOnDeactivate:NO];
  [window setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
                                NSWindowCollectionBehaviorFullScreenAuxiliary |
                                NSWindowCollectionBehaviorStationary];
  [window setContentView:contentView];
  return window;
}

static NSRect frameForToolbarWindow(WoxScreenshotShellController *controller, ScreenshotShellViewModel viewModel, NSSize size) {
  CGFloat topLeftX = controller.minX + viewModel.toolbarAnchorX;
  CGFloat topLeftY = controller.maxY - viewModel.toolbarAnchorY;
  CGFloat originX = topLeftX - size.width / 2.0;
  CGFloat originY = topLeftY - size.height;

  CGFloat minX = NSMinX(controller.virtualFrame);
  CGFloat maxX = NSMaxX(controller.virtualFrame) - size.width;
  CGFloat minY = NSMinY(controller.virtualFrame);
  CGFloat maxY = NSMaxY(controller.virtualFrame) - size.height;

  if (maxX < minX) {
    maxX = minX;
  }
  if (maxY < minY) {
    maxY = minY;
  }

  originX = MIN(MAX(originX, minX), maxX);
  originY = MIN(MAX(originY, minY), maxY);
  return NSMakeRect(originX, originY, size.width, size.height);
}

static NSRect frameForPropertiesWindow(WoxScreenshotShellController *controller, ScreenshotShellViewModel viewModel, NSSize size) {
  CGFloat topLeftX = controller.minX + viewModel.propertiesAnchorX;
  CGFloat topLeftY = controller.maxY - viewModel.propertiesAnchorY;
  CGFloat originX = topLeftX;
  CGFloat originY = topLeftY - size.height;

  CGFloat minX = NSMinX(controller.virtualFrame);
  CGFloat maxX = NSMaxX(controller.virtualFrame) - size.width;
  CGFloat minY = NSMinY(controller.virtualFrame);
  CGFloat maxY = NSMaxY(controller.virtualFrame) - size.height;

  if (maxX < minX) {
    maxX = minX;
  }
  if (maxY < minY) {
    maxY = minY;
  }

  originX = MIN(MAX(originX, minX), maxX);
  originY = MIN(MAX(originY, minY), maxY);
  return NSMakeRect(originX, originY, size.width, size.height);
}

void CreateScreenshotShell(const char *sessionIDCString) {
  ensureControllers();

  NSString *sessionID = [NSString stringWithUTF8String:sessionIDCString];
  if (sessionID == nil || controllers[sessionID] != nil) {
    return;
  }

  CGFloat minX = 0;
  CGFloat maxY = 0;
  NSRect virtualFrame = computeVirtualFrame(&minX, &maxY);

  WoxScreenshotShellController *controller =
      [[WoxScreenshotShellController alloc] init];
  controller.windows = [[NSMutableArray alloc] init];
  controller.overlayViews = [[NSMutableArray alloc] init];
  controller.delegates = [[NSMutableArray alloc] init];
  controller.sessionID = sessionID;
  controller.minX = minX;
  controller.maxY = maxY;
  controller.virtualFrame = virtualFrame;

  controllers[sessionID] = controller;

  NSArray<NSScreen *> *screens = [NSScreen screens];
  for (NSUInteger i = 0; i < [screens count]; i++) {
    NSScreen *screen = screens[i];
    NSRect frame = [screen frame];

    WoxScreenshotOverlayWindow *window =
        [[WoxScreenshotOverlayWindow alloc] initWithContentRect:frame
                                                      styleMask:NSWindowStyleMaskBorderless
                                                        backing:NSBackingStoreBuffered
                                                          defer:NO];
    [window setOpaque:NO];
    [window setBackgroundColor:[NSColor clearColor]];
    [window setLevel:NSScreenSaverWindowLevel];
    [window setIgnoresMouseEvents:NO];
    [window setReleasedWhenClosed:NO];
    [window setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
                                  NSWindowCollectionBehaviorFullScreenAuxiliary |
                                  NSWindowCollectionBehaviorStationary];
    [window setHidesOnDeactivate:NO];
    [window setAcceptsMouseMovedEvents:YES];

    WoxScreenshotOverlayView *view =
        [[WoxScreenshotOverlayView alloc] initWithFrame:NSMakeRect(0, 0, frame.size.width,
                                                                   frame.size.height)];
    view.sessionID = sessionID;
    view.minX = minX;
    view.maxY = maxY;

    NSRect logicalFrame = logicalFrameForScreen(screen, minX, maxY);
    view.logicalX = logicalFrame.origin.x;
    view.logicalY = logicalFrame.origin.y;
    [window setContentView:view];

    WoxScreenshotWindowDelegate *delegate =
        [[WoxScreenshotWindowDelegate alloc] init];
    delegate.sessionID = sessionID;
    [window setDelegate:delegate];

    [controller.windows addObject:window];
    [controller.overlayViews addObject:view];
    [controller.delegates addObject:delegate];

    if (i == 0) {
      [window makeKeyAndOrderFront:nil];
      [window makeFirstResponder:view];
    } else {
      [window orderFrontRegardless];
    }
  }

  controller.toolbarView = [[WoxScreenshotToolbarView alloc] initWithFrame:NSMakeRect(0, 0, 520, 48)];
  controller.toolbarView.sessionID = sessionID;
  controller.toolbarWindow = createChromeWindow(NSMakeRect(minX, maxY - 48, 520, 48), controller.toolbarView);
  [controller.toolbarWindow orderOut:nil];

  controller.propertiesView = [[WoxScreenshotPropertiesView alloc] initWithFrame:NSMakeRect(0, 0, 228, 116)];
  controller.propertiesView.sessionID = sessionID;
  controller.propertiesWindow = createChromeWindow(NSMakeRect(minX, maxY - 116, 228, 116), controller.propertiesView);
  [controller.propertiesWindow orderOut:nil];

  controller.keyMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
                                                                handler:^NSEvent *(NSEvent *event) {
    if ([event keyCode] == 53) {
      screenshotShellKeyDownCGO((char *)[sessionID UTF8String], "Escape");
      return nil;
    }
    if ([event keyCode] == 36) {
      screenshotShellKeyDownCGO((char *)[sessionID UTF8String], "Return");
      return nil;
    }
    return event;
  }];

  [NSApp activateIgnoringOtherApps:YES];
}

void UpdateScreenshotShell(const char *sessionIDCString,
                           ScreenshotShellViewModel viewModel) {
  ensureControllers();
  NSString *sessionID = [NSString stringWithUTF8String:sessionIDCString];
  WoxScreenshotShellController *controller = controllers[sessionID];
  if (controller == nil) {
    return;
  }

  for (WoxScreenshotOverlayView *view in controller.overlayViews) {
    view.selectionRect = selectionRectFromViewModel(viewModel, view);
    view.hasSelection = !NSIsEmptyRect(view.selectionRect);
    view.activeHandle = viewModel.activeHandle;
    view.cursorCode = viewModel.cursor;
    [view.window invalidateCursorRectsForView:view];
    [[view cursorForCode] set];
    [view setNeedsDisplay:YES];
  }

  [controller.toolbarView applyViewModel:viewModel];
  if (viewModel.showToolbar != 0) {
    NSRect frame = frameForToolbarWindow(controller, viewModel, controller.toolbarWindow.frame.size);
    [controller.toolbarWindow setFrame:frame display:YES];
    [controller.toolbarWindow orderFrontRegardless];
  } else {
    [controller.toolbarWindow orderOut:nil];
  }

  [controller.propertiesView applyViewModel:viewModel];
  if (viewModel.showProperties != 0) {
    NSRect frame = frameForPropertiesWindow(controller, viewModel, controller.propertiesWindow.frame.size);
    [controller.propertiesWindow setFrame:frame display:YES];
    [controller.propertiesWindow orderFrontRegardless];
  } else {
    [controller.propertiesWindow orderOut:nil];
  }
}

void CloseScreenshotShell(const char *sessionIDCString) {
  ensureControllers();
  NSString *sessionID = [NSString stringWithUTF8String:sessionIDCString];
  WoxScreenshotShellController *controller = controllers[sessionID];
  if (controller == nil) {
    return;
  }

  [controllers removeObjectForKey:sessionID];

  if (controller.keyMonitor != nil) {
    [NSEvent removeMonitor:controller.keyMonitor];
    controller.keyMonitor = nil;
  }

  for (NSWindow *window in controller.windows) {
    [window orderOut:nil];
    [window close];
  }

  [controller.toolbarWindow orderOut:nil];
  [controller.toolbarWindow close];
  [controller.propertiesWindow orderOut:nil];
  [controller.propertiesWindow close];
}

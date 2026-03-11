#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

extern void keyboardHotkeyTriggeredCGO(int id);
extern int keyboardHookEventCGO(int eventKind, unsigned int keyCode, unsigned int modifiers, unsigned int character);

static EventHandlerRef gHotkeyHandler = NULL;
static NSMutableDictionary<NSNumber *, NSValue *> *gHotkeyRefs = nil;
static id gRawKeyboardMonitor = nil;

static char *copyErrorMessage(const char *message) {
    if (!message) {
        return NULL;
    }
    size_t len = strlen(message) + 1;
    char *copy = malloc(len);
    if (!copy) {
        return NULL;
    }
    memcpy(copy, message, len);
    return copy;
}

static UInt32 toCarbonModifiers(unsigned int modifiers) {
    UInt32 carbon = 0;
    if (modifiers & 1) {
        carbon |= controlKey;
    }
    if (modifiers & 2) {
        carbon |= shiftKey;
    }
    if (modifiers & 4) {
        carbon |= optionKey;
    }
    if (modifiers & 8) {
        carbon |= cmdKey;
    }
    return carbon;
}

static unsigned int currentModifierMask(NSEventModifierFlags flags) {
    NSEventModifierFlags masked = flags & NSEventModifierFlagDeviceIndependentFlagsMask;
    unsigned int modifiers = 0;
    if (masked & NSEventModifierFlagControl) {
        modifiers |= 1;
    }
    if (masked & NSEventModifierFlagShift) {
        modifiers |= 2;
    }
    if (masked & NSEventModifierFlagOption) {
        modifiers |= 4;
    }
    if (masked & NSEventModifierFlagCommand) {
        modifiers |= 8;
    }
    return modifiers;
}

static BOOL isModifierKeyCode(unsigned short keyCode) {
    switch (keyCode) {
        case 54:
        case 55:
        case 56:
        case 58:
        case 59:
        case 60:
        case 61:
        case 62:
            return YES;
        default:
            return NO;
    }
}

static BOOL modifierKeyPressed(unsigned short keyCode, NSEventModifierFlags flags) {
    NSEventModifierFlags masked = flags & NSEventModifierFlagDeviceIndependentFlagsMask;
    switch (keyCode) {
        case 54:
        case 55:
            return (masked & NSEventModifierFlagCommand) != 0;
        case 56:
        case 60:
            return (masked & NSEventModifierFlagShift) != 0;
        case 58:
        case 61:
            return (masked & NSEventModifierFlagOption) != 0;
        case 59:
        case 62:
            return (masked & NSEventModifierFlagControl) != 0;
        default:
            return NO;
    }
}

static unsigned int currentCharacterCode(NSEvent *event) {
    if (!event) {
        return 0;
    }

    // Use the character produced by the active keyboard layout so raw-key
    // consumers such as Explorer type-to-search see the same text as the user.
    NSString *chars = event.charactersIgnoringModifiers;
    if (!chars || chars.length == 0) {
        return 0;
    }

    unichar ch = [chars characterAtIndex:0];
    if (ch > 0x7F || !isalnum((int)ch)) {
        return 0;
    }

    return (unsigned int)ch;
}

static OSStatus hotkeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    EventHotKeyID hotkeyID;
    GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof(hotkeyID), NULL, &hotkeyID);
    keyboardHotkeyTriggeredCGO((int)hotkeyID.id);
    return noErr;
}

int woxDarwinEnsureKeyboardReady(char **errorOut) {
    @autoreleasepool {
        if (!gHotkeyRefs) {
            gHotkeyRefs = [[NSMutableDictionary alloc] init];
        }

        if (!gHotkeyHandler) {
            EventTypeSpec eventType;
            eventType.eventClass = kEventClassKeyboard;
            eventType.eventKind = kEventHotKeyPressed;
            OSStatus status = InstallApplicationEventHandler(&hotkeyHandler, 1, &eventType, NULL, &gHotkeyHandler);
            if (status != noErr) {
                if (errorOut) {
                    *errorOut = copyErrorMessage("failed to install macOS hotkey handler");
                }
                return 0;
            }
        }

        return 1;
    }
}

int woxDarwinRegisterHotkey(int id, unsigned int modifiers, unsigned int keyCode, char **errorOut) {
    @autoreleasepool {
        EventHotKeyRef hotkeyRef = NULL;
        EventHotKeyID hotkeyID;
        hotkeyID.signature = 'WOXK';
        hotkeyID.id = (UInt32)id;

        OSStatus status = RegisterEventHotKey((UInt32)keyCode, toCarbonModifiers(modifiers), hotkeyID, GetApplicationEventTarget(), 0, &hotkeyRef);
        if (status != noErr || hotkeyRef == NULL) {
            if (errorOut) {
                *errorOut = copyErrorMessage("failed to register macOS hotkey");
            }
            return 0;
        }

        if (!gHotkeyRefs) {
            gHotkeyRefs = [[NSMutableDictionary alloc] init];
        }
        gHotkeyRefs[@(id)] = [NSValue valueWithPointer:hotkeyRef];
        return 1;
    }
}

int woxDarwinUnregisterHotkey(int id, char **errorOut) {
    @autoreleasepool {
        NSValue *value = gHotkeyRefs[@(id)];
        if (!value) {
            return 1;
        }

        EventHotKeyRef hotkeyRef = (EventHotKeyRef)[value pointerValue];
        OSStatus status = UnregisterEventHotKey(hotkeyRef);
        [gHotkeyRefs removeObjectForKey:@(id)];
        if (status != noErr) {
            if (errorOut) {
                *errorOut = copyErrorMessage("failed to unregister macOS hotkey");
            }
            return 0;
        }
        return 1;
    }
}

int woxDarwinSetRawKeyboardHookEnabled(int enabled, char **errorOut) {
    @autoreleasepool {
        if (enabled) {
            if (!gRawKeyboardMonitor) {
                NSEventMask mask = NSEventMaskKeyDown | NSEventMaskKeyUp | NSEventMaskFlagsChanged;
                gRawKeyboardMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:mask handler:^(NSEvent *event) {
                    if (!event) {
                        return;
                    }

                    unsigned int modifiers = currentModifierMask(event.modifierFlags);
                    unsigned short keyCode = event.keyCode;

                    if (event.type == NSEventTypeFlagsChanged) {
                        if (!isModifierKeyCode(keyCode)) {
                            return;
                        }
                        int eventKind = modifierKeyPressed(keyCode, event.modifierFlags) ? 0 : 1;
                        keyboardHookEventCGO(eventKind, keyCode, modifiers, 0);
                        return;
                    }

                    if (event.type == NSEventTypeKeyDown) {
                        keyboardHookEventCGO(0, keyCode, modifiers, currentCharacterCode(event));
                        return;
                    }

                    if (event.type == NSEventTypeKeyUp) {
                        keyboardHookEventCGO(1, keyCode, modifiers, currentCharacterCode(event));
                    }
                }];
            }
            return 1;
        }

        if (gRawKeyboardMonitor) {
            [NSEvent removeMonitor:gRawKeyboardMonitor];
            gRawKeyboardMonitor = nil;
        }
        return 1;
    }
}

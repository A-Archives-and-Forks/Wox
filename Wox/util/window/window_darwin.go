package window

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa
#include <stdlib.h>

int getActiveWindowIcon(unsigned char **iconData);
char* getActiveWindowName();
int getActiveWindowPid();
*/
import "C"
import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"unsafe"
)

func GetActiveWindowIcon() (image.Image, error) {
	var iconData *C.uchar
	length := C.getActiveWindowIcon(&iconData)
	if length == 0 {
		return nil, errors.New("failed to get active window icon")
	}
	defer C.free(unsafe.Pointer(iconData))

	data := C.GoBytes(unsafe.Pointer(iconData), length)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return img, nil
}

func GetActiveWindowName() string {
	name := C.getActiveWindowName()
	return C.GoString(name)
}

func GetActiveWindowPid() int {
	pid := C.getActiveWindowPid()
	return int(pid)
}

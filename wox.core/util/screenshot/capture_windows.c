#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    unsigned char* data;
    int len;
    int width;
    int height;
    char* err;
} CaptureRawResult;

static char* copy_error_message(const char* message) {
    if (message == NULL) {
        return NULL;
    }

    size_t len = strlen(message) + 1;
    char* copy = (char*)malloc(len);
    if (copy == NULL) {
        return NULL;
    }

    memcpy(copy, message, len);
    return copy;
}

CaptureRawResult captureRectBGRA(int x, int y, int width, int height) {
    CaptureRawResult result;
    ZeroMemory(&result, sizeof(result));

    if (width <= 0 || height <= 0) {
        result.err = copy_error_message("invalid capture size");
        return result;
    }

    HDC screenDC = GetDC(NULL);
    if (screenDC == NULL) {
        result.err = copy_error_message("failed to get screen device context");
        return result;
    }

    HDC memoryDC = CreateCompatibleDC(screenDC);
    if (memoryDC == NULL) {
        ReleaseDC(NULL, screenDC);
        result.err = copy_error_message("failed to create compatible device context");
        return result;
    }

    BITMAPINFO bmi;
    ZeroMemory(&bmi, sizeof(bmi));
    bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bmi.bmiHeader.biWidth = width;
    bmi.bmiHeader.biHeight = -height;
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB;

    void* bits = NULL;
    HBITMAP bitmap = CreateDIBSection(screenDC, &bmi, DIB_RGB_COLORS, &bits, NULL, 0);
    if (bitmap == NULL || bits == NULL) {
        DeleteDC(memoryDC);
        ReleaseDC(NULL, screenDC);
        result.err = copy_error_message("failed to create DIB section");
        return result;
    }

    HGDIOBJ oldBitmap = SelectObject(memoryDC, bitmap);
    DWORD rasterOp = SRCCOPY;
#ifdef CAPTUREBLT
    rasterOp |= CAPTUREBLT;
#endif
    if (!BitBlt(memoryDC, 0, 0, width, height, screenDC, x, y, rasterOp)) {
        SelectObject(memoryDC, oldBitmap);
        DeleteObject(bitmap);
        DeleteDC(memoryDC);
        ReleaseDC(NULL, screenDC);
        result.err = copy_error_message("BitBlt failed");
        return result;
    }

    result.len = width * height * 4;
    result.width = width;
    result.height = height;
    result.data = (unsigned char*)malloc((size_t)result.len);
    if (result.data == NULL) {
        SelectObject(memoryDC, oldBitmap);
        DeleteObject(bitmap);
        DeleteDC(memoryDC);
        ReleaseDC(NULL, screenDC);
        result.err = copy_error_message("failed to allocate capture buffer");
        result.len = 0;
        result.width = 0;
        result.height = 0;
        return result;
    }

    memcpy(result.data, bits, (size_t)result.len);

    SelectObject(memoryDC, oldBitmap);
    DeleteObject(bitmap);
    DeleteDC(memoryDC);
    ReleaseDC(NULL, screenDC);
    return result;
}

void releaseCaptureRawResult(CaptureRawResult result) {
    if (result.data != NULL) {
        free(result.data);
    }
    if (result.err != NULL) {
        free(result.err);
    }
}

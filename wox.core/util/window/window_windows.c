#define NTDDI_VERSION NTDDI_VISTA
#define _WIN32_WINNT _WIN32_WINNT_VISTA
#include <windows.h>
#include <psapi.h>
#include <shellapi.h>
#include <stdlib.h>
#include <string.h>

char* getIconData(HICON hIcon, unsigned char **iconData, int *iconSize, int *width, int *height) {
    ICONINFO iconinfo;
    if (!GetIconInfo(hIcon, &iconinfo)) {
        return "Failed to get icon info";
    }

    BITMAP bm;
    if (!GetObject(iconinfo.hbmColor, sizeof(BITMAP), &bm)) {
        return "Failed to retrieve bitmap info";
    }

    *width = bm.bmWidth;
    *height = bm.bmHeight;

    HDC hdc = GetDC(NULL);
    if (!hdc) {
        return "Failed to get device context";
    }

    HDC hdcMem = CreateCompatibleDC(hdc);
    if (!hdcMem) {
        ReleaseDC(NULL, hdc);
        return "Failed to create memory device context";
    }

    HBITMAP hbmp = CreateCompatibleBitmap(hdc, *width, *height);
    if (!hbmp) {
        DeleteDC(hdcMem);
        ReleaseDC(NULL, hdc);
        return "Failed to create bitmap";
    }

    SelectObject(hdcMem, hbmp);
    DrawIconEx(hdcMem, 0, 0, hIcon, *width, *height, 0, NULL, DI_NORMAL);

    BITMAPINFOHEADER bi = {sizeof(BITMAPINFOHEADER), *width, -*height, 1, 32, BI_RGB};
    *iconSize = (*width) * (*height) * 4;
    *iconData = (unsigned char*)malloc(*iconSize);
    if (!*iconData) {
        DeleteObject(hbmp);
        DeleteDC(hdcMem);
        ReleaseDC(NULL, hdc);
        return "Failed to allocate memory for icon data";
    }

    if (!GetDIBits(hdcMem, hbmp, 0, *height, *iconData, (BITMAPINFO*)&bi, DIB_RGB_COLORS)) {
        free(*iconData);
        DeleteObject(hbmp);
        DeleteDC(hdcMem);
        ReleaseDC(NULL, hdc);
        return "Failed to retrieve bits from bitmap";
    }

    DeleteObject(iconinfo.hbmColor);
    DeleteObject(iconinfo.hbmMask);
    DeleteObject(hbmp);
    DeleteDC(hdcMem);
    ReleaseDC(NULL, hdc);
    DestroyIcon(hIcon);

    return NULL;
}

char* getActiveWindowIcon(unsigned char **iconData, int *iconSize, int *width, int *height) {
    HWND hwnd = GetForegroundWindow();
    if (!hwnd) {
        return "Unable to get active window handle";
    }

    DWORD processId;
    GetWindowThreadProcessId(hwnd, &processId);

    HANDLE hProcess = OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, FALSE, processId);
    if (!hProcess) {
        return "Unable to open process";
    }

    WCHAR exePath[MAX_PATH];
    DWORD exePathLen = MAX_PATH;
    if (0 == QueryFullProcessImageNameW(hProcess, 0, exePath, &exePathLen)) {
        CloseHandle(hProcess);
        return "Unable to retrieve executable path";
    }

    char exePathA[MAX_PATH];
    WideCharToMultiByte(CP_ACP, 0, exePath, -1, exePathA, MAX_PATH, NULL, NULL);

    HICON hIcon;
    ExtractIconExA(exePathA, 0, &hIcon, NULL, 1);
    if (!hIcon) {
        CloseHandle(hProcess);
        return "Failed to extract icon from executable";
    }

    char* result = getIconData(hIcon, iconData, iconSize, width, height);
    CloseHandle(hProcess);
    return result;
}

char* getActiveWindowName() {
    HWND hwnd = GetForegroundWindow();
    if (!hwnd) {
        char* result = (char*)malloc(1);
        result[0] = '\0';
        return result;
    }

    WCHAR windowTitle[1024];
    if (0 == GetWindowTextW(hwnd, windowTitle, 1024)) {
        char* result = (char*)malloc(1);
        result[0] = '\0';
        return result;
    }

    int len = WideCharToMultiByte(CP_UTF8, 0, windowTitle, -1, NULL, 0, NULL, NULL);
    char* windowTitleA = (char*)malloc(len);
    WideCharToMultiByte(CP_UTF8, 0, windowTitle, -1, windowTitleA, len, NULL, NULL);

    return windowTitleA;
}

int getActiveWindowPid() {
    HWND hwnd = GetForegroundWindow();
    if (!hwnd) {
        return 0;
    }

    DWORD processId;
    GetWindowThreadProcessId(hwnd, &processId);
    return processId;
}
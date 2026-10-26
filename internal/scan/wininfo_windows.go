//go:build windows

// Package scan 的 Windows 专有部分：读取 exe 版本信息、提取 exe 图标。
package scan

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modVersion  = syscall.NewLazyDLL("version.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")

	procGetFileVersionInfoSizeW = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modVersion.NewProc("VerQueryValueW")

	procPrivateExtractIconsW = modUser32.NewProc("PrivateExtractIconsW")
	procGetIconInfo          = modUser32.NewProc("GetIconInfo")
	procDestroyIcon          = modUser32.NewProc("DestroyIcon")

	procGetObjectW = modGdi32.NewProc("GetObjectW")
	procGetDIBits  = modGdi32.NewProc("GetDIBits")
	procDeleteObj  = modGdi32.NewProc("DeleteObject")
)

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// ExeVersionInfo 读取 exe 的版本信息字符串（ProductName / FileDescription 等）。
func ExeVersionInfo(path string) map[string]string {
	out := map[string]string{}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return out
	}

	var handle uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&handle)))
	if size == 0 || size > 16<<20 {
		return out
	}

	buf := make([]byte, size)
	ok, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(p)), 0, uintptr(size), uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return out
	}

	query := func(sub string) string {
		key, err := syscall.UTF16PtrFromString(sub)
		if err != nil {
			return ""
		}
		var valPtr unsafe.Pointer
		var valLen uint32
		r, _, _ := procVerQueryValueW.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(key)),
			uintptr(unsafe.Pointer(&valPtr)),
			uintptr(unsafe.Pointer(&valLen)))
		if r == 0 || valLen == 0 || valPtr == nil {
			return ""
		}
		s := syscall.UTF16ToString(unsafe.Slice((*uint16)(valPtr), valLen))
		return strings.TrimSpace(strings.Trim(s, "\x00"))
	}

	// 先取翻译表，确定语言与代码页
	var transPtr unsafe.Pointer
	var transLen uint32
	transKey, _ := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	r, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(transKey)),
		uintptr(unsafe.Pointer(&transPtr)),
		uintptr(unsafe.Pointer(&transLen)))

	var langs [][2]uint16
	if r != 0 && transPtr != nil && transLen >= 4 {
		n := int(transLen) / 4
		if n > 8 {
			n = 8
		}
		words := unsafe.Slice((*uint16)(transPtr), n*2)
		for i := 0; i < n; i++ {
			langs = append(langs, [2]uint16{words[i*2], words[i*2+1]})
		}
	}
	langs = append(langs, [2]uint16{0x0409, 0x04B0}, [2]uint16{0x0411, 0x04B0}, [2]uint16{0x0804, 0x04B0})

	for _, field := range []string{"ProductName", "FileDescription", "CompanyName", "LegalCopyright"} {
		for _, l := range langs {
			sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, l[0], l[1], field)
			if v := query(sub); v != "" {
				out[field] = v
				break
			}
		}
	}
	return out
}

// BestTitle 综合目录名与 exe 版本信息，给出一个尽量好看的标题。
func BestTitle(folder, exe, fallback string) string {
	if exe == "" {
		return fallback
	}
	info := ExeVersionInfo(exe)
	for _, field := range []string{"ProductName", "FileDescription"} {
		cand := strings.TrimSpace(info[field])
		if cand == "" {
			continue
		}
		// 过滤运行库/系统组件描述
		low := strings.ToLower(cand)
		if strings.Contains(low, "microsoft") || strings.Contains(low, "visual c++") ||
			strings.Contains(low, "runtime") || strings.Contains(low, "redistributable") ||
			strings.Contains(low, ".net framework") || strings.Contains(low, "windows") {
			continue
		}
		if len([]rune(cand)) < 2 {
			continue
		}
		return cand
	}
	return fallback
}

// ExeIcon 提取 exe 的主图标（优先大尺寸），失败返回 nil。
func ExeIcon(path string) image.Image {
	for _, size := range []int{256, 128, 96, 64, 48, 32} {
		if img := extractIcon(path, size); img != nil {
			return img
		}
	}
	return nil
}

func extractIcon(path string, size int) image.Image {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	var hIcon uintptr
	var iconID uint32
	ret, _, _ := procPrivateExtractIconsW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(size), uintptr(size),
		uintptr(unsafe.Pointer(&hIcon)),
		uintptr(unsafe.Pointer(&iconID)),
		1, 0)
	if ret == 0 || hIcon == 0 {
		return nil
	}
	defer procDestroyIcon.Call(hIcon)
	return hIconToImage(hIcon)
}

func hIconToImage(hIcon uintptr) image.Image {
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(hIcon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil
	}
	if ii.hbmColor != 0 {
		defer procDeleteObj.Call(ii.hbmColor)
	}
	if ii.hbmMask != 0 {
		defer procDeleteObj.Call(ii.hbmMask)
	}
	if ii.hbmColor == 0 {
		return nil
	}

	var bm bitmap
	if r, _, _ := procGetObjectW.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil
	}
	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
		return nil
	}

	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, hdc)

	bi := bitmapInfoHeader{
		biSize:      uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:     int32(w),
		biHeight:    -int32(h), // 负数表示自上而下
		biPlanes:    1,
		biBitCount:  32,
		biCompression: 0, // BI_RGB
	}
	pixels := make([]byte, w*h*4)
	r, _, _ := procGetDIBits.Call(
		hdc, ii.hbmColor, 0, uintptr(h),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bi)),
		0) // DIB_RGB_COLORS
	if r == 0 {
		return nil
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hasAlpha := false
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			hasAlpha = true
			break
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			b, g, rr, a := pixels[o], pixels[o+1], pixels[o+2], pixels[o+3]
			if !hasAlpha {
				a = 255
			}
			img.SetRGBA(x, y, color.RGBA{R: rr, G: g, B: b, A: a})
		}
	}
	return img
}

var (
	procGetDC     = modUser32.NewProc("GetDC")
	procReleaseDC = modUser32.NewProc("ReleaseDC")
)

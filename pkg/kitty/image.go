package kitty

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"pod/pkg/config"
	"pod/pkg/util"
)

func GenerateGenericCover(title string) image.Image {
	const imgDim = 256
	dst := InitGenericCoverBackground(imgDim)
	lines := WrapTitleLines(title)

	const lineH = 14
	srcBuf, maxLineW, rawH := RenderTextBuffer(lines, lineH)

	targetW := float64(imgDim) * 0.80
	targetH := float64(imgDim) * 0.80

	scaleX := targetW / float64(maxLineW)
	scaleY := targetH / float64(rawH)
	scale := scaleX
	if scale*float64(rawH) > targetH {
		scale = scaleY
	}

	scaledW := int(float64(maxLineW) * scale)
	scaledH := int(float64(rawH) * scale)
	if scaledW <= 0 {
		scaledW = 1
	}
	if scaledH <= 0 {
		scaledH = 1
	}

	dstX := (imgDim - scaledW) / 2
	dstY := (imgDim - scaledH) / 2
	dstRect := image.Rect(dstX, dstY, dstX+scaledW, dstY+scaledH)

	draw.ApproxBiLinear.Scale(dst, dstRect, srcBuf, srcBuf.Bounds(), draw.Over, nil)
	return dst
}

func InitGenericCoverBackground(imgDim int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, imgDim, imgDim))
	bg := color.RGBA{R: 248, G: 249, B: 250, A: 255}
	border := color.RGBA{R: 220, G: 224, B: 230, A: 255}
	accent := color.RGBA{R: 70, G: 130, B: 180, A: 255}

	for y := 0; y < imgDim; y++ {
		for x := 0; x < imgDim; x++ {
			if x == 0 || x == imgDim-1 || y == 0 || y == imgDim-1 {
				dst.Set(x, y, border)
			} else if y < 4 {
				dst.Set(x, y, accent)
			} else {
				dst.Set(x, y, bg)
			}
		}
	}
	return dst
}

func WrapTitleLines(title string) []string {
	words := strings.Fields(title)
	var lines []string
	var cur string
	maxCharsPerLine := 14
	if len(title) > 28 {
		maxCharsPerLine = 18
	}
	for _, w := range words {
		if cur == "" {
			cur = w
		} else if len(cur)+1+len(w) <= maxCharsPerLine {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
			if len(lines) >= 4 {
				break
			}
		}
	}
	if cur != "" && len(lines) < 4 {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		lines = []string{"Podcast"}
	}
	return lines
}

func RenderTextBuffer(lines []string, lineH int) (*image.RGBA, int, int) {
	rawH := len(lines) * lineH
	maxLineW := 0
	for _, l := range lines {
		w := len(l) * 7
		if w > maxLineW {
			maxLineW = w
		}
	}
	if maxLineW <= 0 {
		maxLineW = 7
	}
	if rawH <= 0 {
		rawH = 13
	}

	srcBuf := image.NewRGBA(image.Rect(0, 0, maxLineW, rawH))
	d := &font.Drawer{
		Dst:  srcBuf,
		Src:  image.NewUniform(color.RGBA{R: 30, G: 35, B: 42, A: 255}),
		Face: basicfont.Face7x13,
	}

	for i, l := range lines {
		lineW := len(l) * 7
		startX := (maxLineW - lineW) / 2
		d.Dot = fixed.P(startX, i*lineH+11)
		d.DrawString(l)
	}
	return srcBuf, maxLineW, rawH
}

func ScaleImageThumbnail(img image.Image, targetW, targetH int) image.Image {
	if targetW <= 0 {
		targetW = 256
	}
	if targetH <= 0 {
		targetH = 256
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= 0 || h <= 0 {
		return img
	}

	dstRect := image.Rect(0, 0, targetW, targetH)
	dstImg := image.NewRGBA(dstRect)
	draw.ApproxBiLinear.Scale(dstImg, dstRect, img, bounds, draw.Over, nil)
	return dstImg
}

func GetOrCacheCoverPNG(filePath string) ([]byte, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s:%d:%d", filePath, fi.ModTime().UnixNano(), fi.Size())

	coverPngMemoryCacheMu.Lock()
	if cached, ok := coverPngMemoryCache[key]; ok {
		coverPngMemoryCacheMu.Unlock()
		return cached, nil
	}
	coverPngMemoryCacheMu.Unlock()

	cDir := PodcastCacheDirForImage(filePath)
	diskPngPath := filepath.Join(cDir, "cover.png")

	if diskFi, err := os.Stat(diskPngPath); err == nil && diskFi.Size() > 0 {
		if !diskFi.ModTime().Before(fi.ModTime()) {
			if diskData, err := os.ReadFile(diskPngPath); err == nil && len(diskData) >= 8 && string(diskData[:8]) == "\x89PNG\r\n\x1a\n" {
				coverPngMemoryCacheMu.Lock()
				coverPngMemoryCache[key] = diskData
				coverPngMemoryCacheMu.Unlock()
				return diskData, nil
			}
		}
	}

	return readAndScaleImagePNG(filePath, diskPngPath, key)
}

func readAndScaleImagePNG(filePath, diskPngPath, cacheKey string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
			coverPngMemoryCacheMu.Lock()
			coverPngMemoryCache[cacheKey] = data
			coverPngMemoryCacheMu.Unlock()
			return data, nil
		}
		return nil, err
	}

	scaled := ScaleImageThumbnail(img, 256, 256)
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return nil, err
	}
	pngBytes := buf.Bytes()

	coverPngMemoryCacheMu.Lock()
	coverPngMemoryCache[cacheKey] = pngBytes
	coverPngMemoryCacheMu.Unlock()

	_ = os.WriteFile(diskPngPath, pngBytes, 0644)
	return pngBytes, nil
}

func PodcastCacheDirForImage(imagePath string) string {
	dir := filepath.Dir(imagePath)
	base := config.CacheBaseDir()
	if strings.HasPrefix(filepath.Clean(dir), filepath.Clean(base)) {
		return dir
	}
	return config.CacheDirForPodcast(dir)
}

func FindCoverImage(podcastDir string) string {
	podcastCoverPathCacheMu.Lock()
	if cached, ok := podcastCoverPathCache[podcastDir]; ok {
		podcastCoverPathCacheMu.Unlock()
		return cached
	}
	podcastCoverPathCacheMu.Unlock()

	res := FindCoverImageUncached(podcastDir)

	podcastCoverPathCacheMu.Lock()
	podcastCoverPathCache[podcastDir] = res
	podcastCoverPathCacheMu.Unlock()
	return res
}

func FindCoverImageUncached(podcastDir string) string {
	candidates := []string{
		"cover.jpg", "cover.jpeg", "cover.png", "cover.webp",
		"folder.jpg", "folder.jpeg", "folder.png",
		"podcast.jpg", "podcast.jpeg", "podcast.png",
		"album.jpg", "album.jpeg", "album.png",
	}

	for _, cand := range candidates {
		p := filepath.Join(podcastDir, cand)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return p
		}
	}

	cDir := config.CacheDirForPodcast(podcastDir)
	for _, cand := range candidates {
		p := filepath.Join(cDir, cand)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return p
		}
	}

	matches, err := filepath.Glob(filepath.Join(podcastDir, "*.jpg"))
	if err == nil && len(matches) > 0 {
		return matches[0]
	}
	matchesPng, err := filepath.Glob(filepath.Join(podcastDir, "*.png"))
	if err == nil && len(matchesPng) > 0 {
		return matchesPng[0]
	}

	genPath := filepath.Join(cDir, "cover.png")
	if fi, err := os.Stat(genPath); err == nil && !fi.IsDir() && fi.Size() > 0 {
		return genPath
	}

	title := util.DisplayName(filepath.Base(podcastDir))
	if title == "" || title == "." || title == "/" {
		title = "Podcast"
	}
	img := GenerateGenericCover(title)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err == nil {
		_ = os.WriteFile(genPath, buf.Bytes(), 0644)
		return genPath
	}

	return ""
}

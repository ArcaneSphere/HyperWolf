package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os/exec"
	"runtime"

	"github.com/gogpu/systray"
)

var (
	iconBytes    []byte
	dashboardURL string
	onStartNode  func()
	onStopNode   func()
	onQuit       func()
	isConnected  bool
	t            *systray.SystemTray
	updateCh     chan struct{}
)

func SetIcon(png []byte) {
	iconBytes = png
}

func defaultIcon() []byte {
	r := image.Rect(0, 0, 32, 32)
	img := image.NewRGBA(r)
	c := color.RGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
	c2 := color.RGBA{R: 0x88, G: 0xbb, B: 0xff, A: 0xff}
	inner := image.Rect(8, 8, 24, 24)
	draw.Draw(img, inner, &image.Uniform{c2}, image.Point{}, draw.Over)
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func makeStatusIcon(connected bool) []byte {
	src := iconBytes
	if src == nil {
		src = defaultIcon()
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	nrgba := image.NewNRGBA(img.Bounds())
	draw.Draw(nrgba, nrgba.Bounds(), img, img.Bounds().Min, draw.Src)

	b := nrgba.Bounds()
	cx, cy := b.Max.X-5, b.Max.Y-5
	radius := 3

	var led color.RGBA
	if connected {
		led = color.RGBA{R: 0x00, G: 0xcc, B: 0x00, A: 0xff}
	} else {
		led = color.RGBA{R: 0xcc, G: 0x00, B: 0x00, A: 0xff}
	}

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				x, y := cx+dx, cy+dy
				if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
					nrgba.Set(x, y, led)
				}
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, nrgba)
	return buf.Bytes()
}

func Run(dashboardAddr string, startNode, stopNode, quit func(), openBrowserOnStart bool) {
	dashboardURL = "http://" + dashboardAddr
	onStartNode = startNode
	onStopNode = stopNode
	onQuit = quit
	updateCh = make(chan struct{}, 1)

	t = systray.New()
	t.SetIcon(makeStatusIcon(false))
	t.SetTooltip("HyperWolf - DERO TELA Browser")
	t.OnClick(func() {
		openBrowser(dashboardURL + "/tray-popup.html")
	})
	rebuildMenu()
	t.Show()

	if openBrowserOnStart {
		openBrowser(dashboardURL)
	}

	go func() {
		for range updateCh {
			t.SetIcon(makeStatusIcon(isConnected))
			rebuildMenu()
		}
	}()

	if err := t.Run(); err != nil {
		log.Printf("Tray run error: %v", err)
	}
}

func rebuildMenu() {
	menu := systray.NewMenu()
	menu.Add("Open Dashboard", func() {
		openBrowser(dashboardURL)
	})
	menu.AddSeparator()
	if isConnected {
		menu.Add("Stop Node", func() {
			isConnected = false
			rebuildMenu()
			if onStopNode != nil {
				onStopNode()
			}
		})
	} else {
		menu.Add("Start Node", func() {
			isConnected = true
			rebuildMenu()
			if onStartNode != nil {
				onStartNode()
			}
		})
	}
	menu.AddSeparator()
	menu.Add("Quit", func() {
		if onQuit != nil {
			onQuit()
		}
	})
	t.SetMenu(menu)
}

func SetConnected(connected bool) {
	isConnected = connected
	select {
	case updateCh <- struct{}{}:
	default:
	}
}

func Stop() {
	if t != nil {
		t.Remove()
	}
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("open browser: %v", err)
	}
}

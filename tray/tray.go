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
	"sync"

	"github.com/gogpu/systray"
)

type trayState struct {
	mu           sync.RWMutex
	iconBytes    []byte
	dashboardURL string
	onStartNode  func()
	onStopNode   func()
	onQuit       func()
	isConnected  bool
	t            *systray.SystemTray
	updateCh     chan struct{}
	defaultIcon  []byte
}

var state = &trayState{
	updateCh: make(chan struct{}, 1),
}

// t is the active system tray instance; guarded by state.mu in callers.
var t *systray.SystemTray

func SetIcon(png []byte) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.iconBytes = png
}

func defaultIcon() []byte {
	state.mu.RLock()
	if state.defaultIcon != nil {
		icon := state.defaultIcon
		state.mu.RUnlock()
		return icon
	}
	state.mu.RUnlock()

	// Create the icon
	r := image.Rect(0, 0, 32, 32)
	img := image.NewRGBA(r)
	c := color.RGBA{R: 0x44, G: 0x88, B: 0xff, A: 0xff}
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
	c2 := color.RGBA{R: 0x88, G: 0xbb, B: 0xff, A: 0xff}
	inner := image.Rect(8, 8, 24, 24)
	draw.Draw(img, inner, &image.Uniform{c2}, image.Point{}, draw.Over)
	var buf bytes.Buffer
	png.Encode(&buf, img)
	icon := buf.Bytes()

	// Cache it
	state.mu.Lock()
	state.defaultIcon = icon
	state.mu.Unlock()
	return icon
}

func makeStatusIcon() []byte {
	state.mu.RLock()
	src := state.iconBytes
	if src == nil {
		src = defaultIcon()
	}
	isConn := state.isConnected
	state.mu.RUnlock()

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
	if isConn {
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
	state.mu.Lock()
	state.dashboardURL = "http://" + dashboardAddr
	state.onStartNode = startNode
	state.onStopNode = stopNode
	state.onQuit = quit
	state.mu.Unlock()

	t = systray.New()
	t.SetIcon(makeStatusIcon())
	t.SetTooltip("HyperWolf - DERO TELA Browser")
	t.OnClick(func() {
		state.mu.RLock()
		url := state.dashboardURL + "/tray-popup.html"
		state.mu.RUnlock()
		openBrowser(url)
	})
	rebuildMenu()
	t.Show()

	state.mu.RLock()
	obs := openBrowserOnStart
	state.mu.RUnlock()
	if obs {
		state.mu.RLock()
		url := state.dashboardURL
		state.mu.RUnlock()
		openBrowser(url)
	}

	go func() {
		for range state.updateCh {
			t.SetIcon(makeStatusIcon())
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
		state.mu.RLock()
		url := state.dashboardURL
		state.mu.RUnlock()
		openBrowser(url)
	})
	menu.AddSeparator()
	state.mu.RLock()
	conn := state.isConnected
	startNode := state.onStartNode
	stopNode := state.onStopNode
	state.mu.RUnlock()

	if conn {
		menu.Add("Stop Node", func() {
			setConnected(false)
			rebuildMenu()
			if stopNode != nil {
				stopNode()
			}
		})
	} else {
		menu.Add("Start Node", func() {
			setConnected(true)
			rebuildMenu()
			if startNode != nil {
				startNode()
			}
		})
	}
	menu.AddSeparator()
	menu.Add("Quit", func() {
		state.mu.RLock()
		q := state.onQuit
		state.mu.RUnlock()
		if q != nil {
			q()
		}
	})
	t.SetMenu(menu)
}

// setConnected records the connection state and triggers an icon + menu
// refresh through the updateCh consumer. This is the single path every state
// change must go through so the tray icon and menu always stay in sync.
func setConnected(conn bool) {
	state.mu.Lock()
	state.isConnected = conn
	state.mu.Unlock()
	select {
	case state.updateCh <- struct{}{}:
	default:
	}
}

func SetConnected(connected bool) {
	setConnected(connected)
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

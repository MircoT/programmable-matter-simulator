package pkg

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"github.com/mircot/programmable-matter-simulator/assets"
)

const (
	ScreenWidth    = 800
	ScreenHeight   = 600
	StatusBarDelay = 60
	DefaultDPI     = 96
)

var (
	mplusNormalFont    font.Face
	mplusHelpMenuFont  font.Face
	mplusStatusBarFont font.Face
)

type statusBarMsg struct {
	msg   string
	delay int
}

type Renderer struct {
	hexSize  int
	w        int
	h        int
	half_w   int
	half_h   int
	max_dist int
	mx       int
	my       int
	c_row    int
	c_column int
	// offset_x     int
	// offset_y     int
	engine         Engine
	stateAssets    []*ebiten.Image
	keys           []ebiten.Key
	ticker         *time.Ticker
	engineTick     chan int
	round          int
	guiDebug       bool
	helpDialog     bool
	statusBarMsgs  []statusBarMsg
	statusBarDelay int
	statusBarMsg   string
}

func (r *Renderer) drawCircle(screen *ebiten.Image, x, y, radius int, clr color.RGBA, fill bool) {
	radius64 := float64(radius)
	minAngle := math.Acos(1 - 1/radius64)

	for angle := float64(0); angle <= 360; angle += minAngle {
		xDelta := radius64 * math.Cos(angle)
		yDelta := radius64 * math.Sin(angle)

		x1 := int(math.Round(float64(x) + xDelta))
		y1 := int(math.Round(float64(y) + yDelta))

		if fill {
			if y1 < y {
				for y2 := y1; y2 <= y; y2++ {
					screen.Set(x1, y2, clr)
				}
			} else {
				for y2 := y1; y2 > y; y2-- {
					screen.Set(x1, y2, clr)
				}
			}
		}

		screen.Set(x1, y1, clr)
	}
}

func (r *Renderer) drawParticles(screen *ebiten.Image) {
	for row, columns := range r.engine.grid {
		cur_h := row * r.half_h
		w_quarter := r.half_w / 2.0

		for column, particle := range columns {
			cur_w := column * r.half_w
			if row%2 == 0 {
				cur_w += w_quarter
			}

			if particle.state != VOID {
				op := &ebiten.DrawImageOptions{}
				scaleFactor := float64(r.hexSize) / 128.
				center := (128. * scaleFactor) / 2.
				// op.GeoM.Scale(4, 4)
				op.GeoM.Scale(2.0, 2.0)
				op.GeoM.Scale(scaleFactor, scaleFactor)
				op.GeoM.Translate(float64(cur_w)-center, float64(cur_h)-center)
				// By default, nearest filter is used.
				if particle.moveFailed {
					r.drawCircle(screen, cur_w, cur_h, 16, color.RGBA{242, 0, 0, 10}, true)
				}

				if particle.iState == AWAKE {
					screen.DrawImage(r.stateAssets[len(r.stateAssets)-1], op)
				}

				screen.DrawImage(r.stateAssets[particle.GetStateN()-1], op)
			}
		}
	}
}

func (r *Renderer) drawHelp(screen *ebiten.Image) {
	ebitenutil.DrawRect(screen, 48, 48, ScreenWidth-(42*2), ScreenHeight-(42*2), color.RGBA{21, 21, 21, 196})
	ebitenutil.DrawRect(screen, 42, 42, ScreenWidth-(42*2), ScreenHeight-(42*2), color.RGBA{96, 96, 96, 196})

	text.Draw(screen, "------[Help dialog]------", mplusHelpMenuFont, ScreenWidth/2-196, ScreenHeight/2-196, color.White)

	text.Draw(screen, "[Keys] -> Action", mplusHelpMenuFont, 55, ScreenHeight/3, color.White)
	text.Draw(screen, " - [H] -> Show/Hide this dialog", mplusHelpMenuFont, 55, ScreenHeight/3+48, color.White)
	text.Draw(screen, " - [R] -> Reload all engine", mplusHelpMenuFont, 55, ScreenHeight/3+48+42, color.White)
	text.Draw(screen, " - [L] -> Reload scripts", mplusHelpMenuFont, 55, ScreenHeight/3+48+42+42, color.White)
	text.Draw(screen, " - [Space] -> Start/Stop simulation", mplusHelpMenuFont, 55, ScreenHeight/3+48+42+42+42, color.White)
	text.Draw(screen, " - [F] -> Enter/Exit fullscreen", mplusHelpMenuFont, 55, ScreenHeight/3+48+42+42+42+42, color.White)
	text.Draw(screen, " - [0..9] -> Select a particle script", mplusHelpMenuFont, 55, ScreenHeight/3+48+42+42+42+42+42, color.White)
}

func (r *Renderer) drawStatusBar(screen *ebiten.Image) {
	ebitenutil.DrawRect(screen, 0, ScreenHeight-28, ScreenWidth, ScreenHeight, color.RGBA{21, 21, 21, 196})
	ebitenutil.DrawRect(screen, 0, ScreenHeight-24, ScreenWidth, ScreenHeight, color.RGBA{96, 96, 96, 196})

	if r.statusBarMsg != "" {
		text.Draw(screen, r.statusBarMsg, mplusStatusBarFont, 6, ScreenHeight-6, color.White)
	}

	if len(r.statusBarMsgs) > 0 && r.statusBarMsg == "" {
		curMsg := r.statusBarMsgs[0]
		r.statusBarMsg = curMsg.msg
		r.statusBarDelay = curMsg.delay
		r.statusBarMsgs = r.statusBarMsgs[1:]
	}

	if r.statusBarDelay > 0 {
		r.statusBarDelay -= 1
	} else {
		r.statusBarMsg = ""
		r.statusBarDelay = StatusBarDelay
	}
}

func (r *Renderer) drawGrid(screen *ebiten.Image) {
	for row, columns := range r.engine.grid {
		cur_h := row * r.half_h
		next_h := cur_h + r.half_h
		w_quarter := r.half_w / 2.0

		ebitenutil.DrawLine(screen,
			0., float64(cur_h), ScreenWidth, float64(cur_h),
			color.RGBA{142, 142, 142, 255},
		)

		for column := range columns {
			cur_w := column * r.half_w
			if row%2 == 0 {
				cur_w += w_quarter
			}

			ebitenutil.DrawLine(screen,
				float64(cur_w), float64(cur_h), float64(cur_w-w_quarter), float64(next_h),
				color.RGBA{142, 142, 142, 255})
			ebitenutil.DrawLine(screen,
				float64(cur_w), float64(cur_h), float64(cur_w+w_quarter), float64(next_h),
				color.RGBA{142, 142, 142, 255})

			if r.guiDebug {
				msg := fmt.Sprintf("(%d,%d)", row, column)
				text.Draw(screen, msg, mplusNormalFont, cur_w, cur_h, color.Black)
			}
		}
	}
}

func (r *Renderer) InitImages() error {
	// images [c, l, r, ul, ur, ll, lr, r, l, lr, ll, ur, ul, o ,a]
	r.stateAssets = make([]*ebiten.Image, 0)

	img, err := png.Decode(bytes.NewReader(assets.Contracted))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedL))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedR))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedUL))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedUR))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedLL))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ExpandedLR))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	//        [x  1  2  3   4   5   6   2  1  6   5   4   3   x  x]
	// images [c, l, r, ul, ur, ll, lr, r, l, lr, ll, ur, ul, o ,a]
	r.stateAssets = append(r.stateAssets, r.stateAssets[2])
	r.stateAssets = append(r.stateAssets, r.stateAssets[1])
	r.stateAssets = append(r.stateAssets, r.stateAssets[6])
	r.stateAssets = append(r.stateAssets, r.stateAssets[5])
	r.stateAssets = append(r.stateAssets, r.stateAssets[4])
	r.stateAssets = append(r.stateAssets, r.stateAssets[3])

	img, err = png.Decode(bytes.NewReader(assets.Obstacle))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	img, err = png.Decode(bytes.NewReader(assets.ContractedAwake))
	if err != nil {
		panic(err)
	}

	r.stateAssets = append(r.stateAssets, ebiten.NewImageFromImage(img))

	return nil
}

func (r *Renderer) Init() error {
	tt, err := opentype.Parse(fonts.MPlus1pRegular_ttf)
	if err != nil {
		return err
	}

	mplusNormalFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    12,
		DPI:     DefaultDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}

	mplusStatusBarFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    14,
		DPI:     DefaultDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}

	mplusHelpMenuFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    24,
		DPI:     DefaultDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}

	r.ticker = time.NewTicker(250 * time.Millisecond)
	r.engineTick = make(chan int)
	r.round = 0

	err = r.InitImages()
	if err != nil {
		panic(err)
	}

	err = r.engine.LoadScripts()
	if err != nil {
		panic(err)
	}

	hexSize, initialState, err := r.engine.InitialState()
	if err != nil {
		panic(err)
	}

	r.hexSize = hexSize

	// Ref: https://www.redblobgames.com/grids/hexagons/#distances
	r.w = 2 * r.hexSize
	r.h = int(math.Sqrt(3) * float64(r.hexSize))

	r.half_w = int(r.w) / 2
	r.half_h = int(r.h) / 2

	numRows := int(ScreenHeight/r.half_h) + 1
	numCols := int(ScreenWidth/r.half_w) + 1

	if err := r.engine.Init(numRows, numCols); err != nil {
		panic(err)
	}

	if err := r.engine.InitGrid(initialState); err != nil {
		panic(err)
	}

	r.max_dist = r.hexSize / 2

	r.statusBarMsgs = make([]statusBarMsg, 0)
	r.statusBarDelay = StatusBarDelay
	r.statusBarMsg = ""

	go r.engine.Update(&r.engineTick)

	return nil
}

func (r *Renderer) drawCursor(screen *ebiten.Image) {
	// draw cursor
	ebitenutil.DrawRect(screen, float64(r.mx)-8, float64(r.my)-8, 16, 16, color.RGBA{0, 0, 0, 96})
	r.drawCircle(screen, r.mx, r.my, r.max_dist, color.RGBA{0, 0, 0, 96}, false)
	r.drawCircle(screen, r.mx, r.my, r.hexSize, color.RGBA{0, 0, 0, 96}, false)
}

func (r *Renderer) drawNeighbors(screen *ebiten.Image) {
	// L
	curRow := r.c_row * r.half_h
	curCol := r.c_column - 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 242, 0, 10}, true)

	// R
	curCol = r.c_column + 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 242, 0, 10}, true)

	// UL
	curRow = r.c_row - 1
	curCol = r.c_column
	if curRow%2 == 0 {
		curCol -= 1
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{242, 0, 0, 10}, true)

	// UR
	curRow = r.c_row - 1
	curCol = r.c_column + 1
	if curRow%2 == 0 {
		curCol -= 1
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 0, 242, 10}, true)

	// LL
	curRow = r.c_row + 1
	curCol = r.c_column
	if curRow%2 == 0 {
		curCol -= 1
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{242, 0, 0, 96}, true)

	// LR
	curRow = r.c_row + 1
	curCol = r.c_column + 1
	if curRow%2 == 0 {
		curCol -= 1
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 0, 242, 96}, true)

	// 2L
	curRow = r.c_row * r.half_h
	curCol = r.c_column - 2
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 242, 0, 10}, true)

	// 2R
	curCol = r.c_column + 2
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 242, 0, 10}, true)

	// U2L
	curRow = r.c_row - 2
	curCol = r.c_column - 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{242, 0, 0, 10}, true)

	// U2R
	curRow = r.c_row - 2
	curCol = r.c_column + 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 0, 242, 10}, true)

	// L2L
	curRow = r.c_row + 2
	curCol = r.c_column - 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{242, 0, 0, 96}, true)

	// L2R
	curRow = r.c_row + 2
	curCol = r.c_column + 1
	if curRow%2 == 0 {
		curCol = curCol*r.half_w + r.half_w/2
	} else {
		curCol = curCol * r.half_w
	}
	curRow *= r.half_h
	r.drawCircle(screen, curCol, curRow, 16, color.RGBA{0, 0, 242, 96}, true)

}

func (r *Renderer) Update() error {
	select {
	case <-r.ticker.C:
		select {
		case round := <-r.engineTick:
			// fmt.Println("Engine Updated")
			r.round = round
		default:
			// fmt.Println("Engine NOT Updated")
		}
	default:
	}

	r.keys = inpututil.AppendPressedKeys(r.keys[:0])

	for _, p := range r.keys {
		switch p.String() {
		case "Space":
			if inpututil.IsKeyJustPressed(p) {
				if r.engine.IsRunning() {
					r.engine.Stop()
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{"Simulation stop!", 21})
				} else {
					r.engine.Start()
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{"Simulation start!", 21})
				}
			}
		case "L":
			if inpututil.IsKeyJustPressed(p) {
				if err := r.engine.LoadScripts(); err != nil {
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{fmt.Sprintf("%s", err), 120})
				} else {
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{"Scripts reloaded...", StatusBarDelay})
				}
			}
		case "R":
			if inpututil.IsKeyJustPressed(p) {
				if err := r.Init(); err != nil {
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{fmt.Sprintf("%s", err), 120})
				} else {
					r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{"Engine reloaded...", StatusBarDelay})
				}
			}
		case "D":
			if inpututil.IsKeyJustPressed(p) {
				r.guiDebug = !r.guiDebug
			}
		case "H":
			if inpututil.IsKeyJustPressed(p) {
				r.helpDialog = !r.helpDialog
			}
		case "F":
			if inpututil.IsKeyJustPressed(p) {
				ebiten.SetFullscreen(!ebiten.IsFullscreen())
			}
		case "Digit0", "Digit1", "Digit2", "Digit3", "Digit4", "Digit5", "Digit6", "Digit7", "Digit8", "Digit9":
			if inpututil.IsKeyJustPressed(p) {
				digit := strings.ReplaceAll(p.String(), "Digit", "")
				if idx, err := strconv.ParseInt(digit, 10, 0); err == nil {
					if scriptName, err := r.engine.SelectScript(int(idx)); err != nil {
						r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{fmt.Sprintf("%s", err), 120})
					} else {
						r.statusBarMsgs = append(r.statusBarMsgs, statusBarMsg{fmt.Sprintf("Selected -> %s", scriptName), 60})
					}
				} else {
					panic(err)
				}
			}
		}
	}

	mx, my := ebiten.CursorPosition()

	max_row := my / int(r.half_h)
	max_column := mx / int(r.half_w)

	for row := max_row - 1; row < max_row+2; row++ {
		cur_h := row * r.half_h
		w_quarter := r.half_w / 2

		for column := max_column - 1; column < max_column+2; column++ {
			cur_w := column * r.half_w
			if row%2 == 0 {
				cur_w += w_quarter
			}
			diff_x := cur_w - mx
			diff_y := cur_h - my

			if diff_x*diff_x+diff_y*diff_y <= r.max_dist*r.max_dist {
				r.mx = cur_w
				r.my = cur_h
				r.c_row = row
				r.c_column = column

				return nil
			}
		}
	}

	return nil
}

func (r *Renderer) Draw(screen *ebiten.Image) {
	screen.Fill(color.White)
	r.drawGrid(screen)

	// drawEbitenText(screen)
	// drawEbitenLogo(screen, 20, 90)
	// drawArc(screen, r.counter)
	// drawWave(screen, r.counter)
	if r.guiDebug {
		r.drawCursor(screen)
		r.drawNeighbors(screen)
	}
	r.drawParticles(screen)

	for _, p := range r.keys {
		switch p.String() {
		case "Space":
			if inpututil.IsKeyJustPressed(p) {
				screen.Fill(color.Black)
			}
		case "L":
			if inpututil.IsKeyJustPressed(p) {
				screen.Fill(color.Black)
			}
		case "R":
			if inpututil.IsKeyJustPressed(p) {
				screen.Fill(color.Black)
			}
		}
	}

	if r.guiDebug {
		ebitenutil.DrawRect(screen, 0, 0, 96, 72, color.RGBA{96, 96, 96, 196})

		ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f\nFPS: %0.2f\nCursor: (%d,%d)\nRound: %d",
			ebiten.CurrentTPS(), ebiten.CurrentFPS(), r.c_row, r.c_column, r.round))
	}

	r.drawStatusBar(screen)

	if r.helpDialog {
		r.drawHelp(screen)
	}

}

func (r *Renderer) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenWidth, ScreenHeight
}

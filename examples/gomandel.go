package main

import (
	"sdl"
	"gl"
	"unsafe"
	"flag"
	"runtime"
)

var iterations *int = flag.Int("i", 1024, "number of iterations")

func drawQuad(x, y, w, h int, u, v, u2, v2 float) {
	gl.Begin(gl.QUADS)

	gl.TexCoord2f(gl.GLfloat(u), gl.GLfloat(v))
	gl.Vertex2i(gl.GLint(x), gl.GLint(y))

	gl.TexCoord2f(gl.GLfloat(u2), gl.GLfloat(v))
	gl.Vertex2i(gl.GLint(x+w), gl.GLint(y))

	gl.TexCoord2f(gl.GLfloat(u2), gl.GLfloat(v2))
	gl.Vertex2i(gl.GLint(x+w), gl.GLint(y+h))

	gl.TexCoord2f(gl.GLfloat(u), gl.GLfloat(v2))
	gl.Vertex2i(gl.GLint(x), gl.GLint(y+h))

	gl.End()
}

func uploadTexture_RGBA32(w, h int, data []byte) gl.GLuint {
	var id gl.GLuint

	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.GLsizei(w), gl.GLsizei(h), 0, gl.RGBA,
		gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))

	if gl.GetError() != gl.NO_ERROR {
		gl.DeleteTextures(1, &id)
		panic("Failed to load a texture")
		return 0
	}
	return id
}

type Color struct {
	R, G, B, A byte
}

type ColorRange struct {
	Start, End Color
	Range      float
}

var (
	DarkYellow   = Color{0xEE, 0xEE, 0x9E, 0xFF}
	DarkGreen    = Color{0x44, 0x88, 0x44, 0xFF}
	PaleGreyBlue = Color{0x49, 0x93, 0xDD, 0xFF}
	Cyan         = Color{0x00, 0xFF, 0xFF, 0xFF}
	Red          = Color{0xFF, 0x00, 0x00, 0xFF}
	White        = Color{0xFF, 0xFF, 0xFF, 0xFF}
	Black        = Color{0x00, 0x00, 0x00, 0xFF}
)

var colorScale = [...]ColorRange{
	ColorRange{DarkYellow, DarkGreen, 0.25},
	ColorRange{DarkGreen, Cyan, 0.25},
	ColorRange{Cyan, Red, 0.25},
	ColorRange{Red, White, 0.125},
	ColorRange{White, PaleGreyBlue, 0.125},
}

var palette []Color

func interpolateColor(c1, c2 Color, f float) Color {
	var c Color
	c.R = byte(float(c1.R)*f + float(c2.R)*(1.0-f))
	c.G = byte(float(c1.G)*f + float(c2.G)*(1.0-f))
	c.B = byte(float(c1.B)*f + float(c2.B)*(1.0-f))
	c.A = byte(float(c1.A)*f + float(c2.A)*(1.0-f))
	return c
}

func buildPalette() {
	palette = make([]Color, *iterations+1)
	p := 0
	for _, r := range colorScale {
		n := int(r.Range*float(*iterations) + 0.5)
		for i := 0; i < n && p < *iterations; i++ {
			c := interpolateColor(r.Start, r.End, float(i)/float(n))
			palette[p] = c
			p++
		}
	}
	palette[*iterations] = Black
}

func mandelbrotAt(c complex128) Color {
	var z complex128 = cmplx(0, 0)
	for i := 0; i < *iterations; i++ {
		z = z*z + c
		if real(z)*real(z)+imag(z)*imag(z) > 4 {
			return palette[i]
		}
	}
	return palette[*iterations]
}

type Rect struct {
	X, Y float64
	W, H float64
}

func mandelbrot(w, h int, what Rect, discard <-chan bool, progress chan int) <-chan []byte {
	result := make(chan []byte)
	go func() {
		data := make([]byte, w*h*4)
		stepx := what.W / float64(w)
		stepy := what.H / float64(h)

		for y := 0; y < h; y++ {
			i := float64(y)*stepy + what.Y

			for x := 0; x < w; x++ {
				r := float64(x)*stepx + what.X
				c := cmplx(r, i)

				offset := y*w*4 + x*4
				color := mandelbrotAt(c)
				data[offset+0] = color.R
				data[offset+1] = color.G
				data[offset+2] = color.B
				data[offset+3] = color.A
			}
			_, ok := <-discard
			if ok {
				return
			}

			// discard value if we have something
			_, ok = <-progress
			if len(progress) == 0 {
				percents := int(float(y) / float(h-1) * 100)
				if percents < 0 {
					percents = 0
				} else if percents > 100 {
					percents = 100
				}
				progress <- percents
			}
		}
		result <- data
	}()

	return result
}

type Point struct {
	X, Y int
}

func MinInt(i1, i2 int) int {
	if i1 < i2 {
		return i1
	}
	return i2
}

func MaxInt(i1, i2 int) int {
	if i1 > i2 {
		return i1
	}
	return i2
}

func minMaxPoints(p1, p2 Point) (min, max Point) {
	min.X = MinInt(p1.X, p2.X)
	min.Y = MinInt(p1.Y, p2.Y)
	max.X = MaxInt(p1.X, p2.X)
	max.Y = MaxInt(p1.Y, p2.Y)
	return
}

const BarSize = 8

func drawProgress(w, h, percents, pending int) {
	switch pending {
	case None:
		gl.Color3ub(200, 0, 0)
		drawQuad(0, h-BarSize, w, BarSize, 0, 0, 1, 1)
		gl.Color3ub(255, 255, 255)
		return
	case Small:
		gl.Color3ub(200, 200, 0)
	case Big:
		gl.Color3ub(200, 200, 0)
		drawQuad(0, h-BarSize, w, BarSize, 0, 0, 1, 1)
		gl.Color3ub(200, 0, 0)
	}
	step := float(w) / 100
	drawQuad(0, h-BarSize, int(float(percents)*step), BarSize, 0, 0, 1, 1)
	gl.Color3ub(255, 255, 255)
}

func drawSelection(p1, p2 Point) {
	min, max := minMaxPoints(p1, p2)

	gl.Color3ub(255, 0, 0)
	gl.Begin(gl.LINES)
	gl.Vertex2i(gl.GLint(min.X), gl.GLint(min.Y))
	gl.Vertex2i(gl.GLint(max.X), gl.GLint(min.Y))

	gl.Vertex2i(gl.GLint(min.X), gl.GLint(min.Y))
	gl.Vertex2i(gl.GLint(min.X), gl.GLint(max.Y))

	gl.Vertex2i(gl.GLint(max.X), gl.GLint(max.Y))
	gl.Vertex2i(gl.GLint(max.X), gl.GLint(min.Y))

	gl.Vertex2i(gl.GLint(max.X), gl.GLint(max.Y))
	gl.Vertex2i(gl.GLint(min.X), gl.GLint(max.Y))
	gl.End()
	gl.Color3ub(255, 255, 255)
}

func rectFromSelection(p1, p2 Point, scrw, scrh int, cur Rect) Rect {
	min, max := minMaxPoints(p1, p2)

	// we need to keep aspect ratio here, assuming 1:1 TODO: don't assume!
	cw, ch := max.X-min.X, max.Y-min.Y
	if cw < ch {
		dif := (ch - cw) / 2
		min.X -= dif
		max.X += dif
	} else if ch < cw {
		dif := (cw - ch) / 2
		min.Y -= dif
		max.Y += dif
	}

	stepx := cur.W / float64(scrw)
	stepy := cur.H / float64(scrh)

	var r Rect
	r.X = float64(min.X)*stepx + cur.X
	r.Y = float64(min.Y)*stepy + cur.Y
	r.W = float64(max.X-min.X) * stepx
	r.H = float64(max.Y-min.Y) * stepy
	return r
}

type TexCoords struct {
	TX, TY, TX2, TY2 float
}

func texCoordsFromSelection(p1, p2 Point, w, h int, tcold TexCoords) (tc TexCoords) {
	min, max := minMaxPoints(p1, p2)
	cw, ch := max.X-min.X, max.Y-min.Y
	if cw < ch {
		dif := (ch - cw) / 2
		min.X -= dif
		max.X += dif
	} else if ch < cw {
		dif := (cw - ch) / 2
		min.Y -= dif
		max.Y += dif
	}

	modx := tcold.TX2 - tcold.TX
	mody := tcold.TY2 - tcold.TY

	stepx := (1 / float(w)) * modx
	stepy := (1 / float(h)) * mody

	tc.TX = tcold.TX + float(min.X)*stepx
	tc.TX2 = tcold.TX + float(max.X)*stepx
	tc.TY = tcold.TY + float(min.Y)*stepy
	tc.TY2 = tcold.TY + float(max.Y)*stepy
	return
}

//-------------------------------------------------------------------------
// MandelbrotRequest
//
// Mandelbrot drawing request abstraction, controls drawing goroutines
// and builds textures
//-------------------------------------------------------------------------

const None = 0
const Small = 1
const Big = 2

const SmallSize = 256

type MandelbrotRequest struct {
	Texture   gl.GLuint
	Discarder chan bool
	Result    <-chan []byte
	Progress  chan int

	W, H int
	What Rect

	Pending       int
	PercentsReady int
}

func ReuploadTexture(tex *gl.GLuint, w, h int, data []byte) {
	if *tex > 0 {
		gl.DeleteTextures(1, tex)
	}
	*tex = uploadTexture_RGBA32(w, h, data)
}

func (self *MandelbrotRequest) MakeRequest(w, h int, what Rect) {
	self.Drop()
	self.W = w
	self.H = h
	self.What = what
	self.Discarder = make(chan bool)
	self.Progress = make(chan int, 250)

	// request small first
	self.Pending = Small
	self.Result = mandelbrot(SmallSize, SmallSize, what, self.Discarder, self.Progress)
}

func (self *MandelbrotRequest) makeBigRequest() {
	self.Pending = Big
	self.Result = mandelbrot(self.W, self.H, self.What, self.Discarder, self.Progress)
}

func (self *MandelbrotRequest) Drop() {
	if self.Pending > 0 {
		self.Discarder <- true
		self.Pending = None
	}
}

func (self *MandelbrotRequest) Update(tex *gl.GLuint, tc *TexCoords) int {
	// if request is pending check status
	progress := -1
	if self.Pending > 0 {
		if p, ok := <-self.Progress; ok {
			progress = p
		}

		// if something is finished
		if data, ok := <-self.Result; ok {
			switch self.Pending {
			case Small:
				// this was a small image
				ReuploadTexture(tex, SmallSize, SmallSize, data)
				self.makeBigRequest()
			case Big:
				ReuploadTexture(tex, self.W, self.H, data)
				self.Pending = None
			default:
				panic("unreachable")
			}
			*tc = TexCoords{0, 0, 1, 1}
			progress = 0
		}
	}
	return progress
}

func (self *MandelbrotRequest) WaitFor(pending int, tex *gl.GLuint, tc *TexCoords) {
	*tc = TexCoords{0, 0, 1, 1}
	switch pending {
	case Small:
		data := <-self.Result
		ReuploadTexture(tex, SmallSize, SmallSize, data)
		self.makeBigRequest()
	case Big:
		self.Drop()
		self.makeBigRequest()
		data := <-self.Result
		ReuploadTexture(tex, self.W, self.H, data)
		self.Pending = None
	}
}

//-------------------------------------------------------------------------
// main()
//-------------------------------------------------------------------------

func main() {
	runtime.LockOSThread()
	flag.Parse()
	buildPalette()
	sdl.Init(sdl.INIT_VIDEO)
	defer sdl.Quit()

	sdl.GL_SetAttribute(sdl.GL_SWAP_CONTROL, 1)

	if sdl.SetVideoMode(512, 512, 32, sdl.OPENGL) == nil {
		panic("sdl error")
	}

	sdl.WM_SetCaption("Gomandel", "Gomandel")

	if gl.Init() != 0 {
		panic("glew error")
	}

	gl.Enable(gl.TEXTURE_2D)
	gl.Viewport(0, 0, 512, 512)
	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, 512, 512, 0, -1, 1)

	gl.ClearColor(0, 0, 0, 0)


	//-----------------------------------------------------------------------------
	var dndDragging bool = false
	var dndStart Point
	var dndEnd Point
	var tex gl.GLuint
	var tc TexCoords
	var lastProgress int
	initialRect := Rect{-1.5, -1.5, 3, 3}
	rect := initialRect

	rc := new(MandelbrotRequest)
	rc.MakeRequest(512, 512, rect)
	rc.WaitFor(Small, &tex, &tc)

	running := true
	e := new(sdl.Event)
	for running {
		for e.Poll() {
			switch e.Type {
			case sdl.QUIT:
				running = false
			case sdl.MOUSEBUTTONDOWN:
				dndDragging = true
				sdl.GetMouseState(&dndStart.X, &dndStart.Y)
				dndEnd = dndStart
			case sdl.MOUSEBUTTONUP:
				dndDragging = false
				sdl.GetMouseState(&dndEnd.X, &dndEnd.Y)
				if e.MouseButton().Button == 3 {
					rect = initialRect
				} else {
					rect = rectFromSelection(dndStart, dndEnd, 512, 512, rect)
					tc = texCoordsFromSelection(dndStart, dndEnd, 512, 512, tc)
				}

				// make request
				rc.MakeRequest(512, 512, rect)
			case sdl.MOUSEMOTION:
				if dndDragging {
					sdl.GetMouseState(&dndEnd.X, &dndEnd.Y)
				}
			}
		}

		// if we're waiting for a result, check if it's ready
		p := rc.Update(&tex, &tc)
		if p != -1 {
			lastProgress = p
		}

		gl.Clear(gl.COLOR_BUFFER_BIT)
		gl.BindTexture(gl.TEXTURE_2D, tex)
		drawQuad(0, 0, 512, 512, tc.TX, tc.TY, tc.TX2, tc.TY2)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		if dndDragging {
			drawSelection(dndStart, dndEnd)
		}
		drawProgress(512, 512, lastProgress, rc.Pending)
		sdl.GL_SwapBuffers()
	}
}

package audio

import (
	"github.com/g3n/engine/audio"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/text"
	"github.com/g3n/engine/texture"
	"github.com/g3n/g3nd/app"
	"github.com/g3n/g3nd/demos"

	"time"
)

func init() {
	demos.Map["audio.position"] = &AudioPosition{}
}

type AudioPosition struct {
	ps1 *PlayerSphere
	ps2 *PlayerSphere
	ps3 *PlayerSphere
	ps4 *PlayerSphere
	ps5 *PlayerSphere
	ps6 *PlayerSphere
}

func (t *AudioPosition) Initialize(a *app.App) {

	// Show axis helper
	ah := graphic.NewAxisHelper(1.0)
	a.Scene().Add(ah)

	// Show grid helper
	grid := graphic.NewGridHelper(100, 1, &math32.Color{0.4, 0.4, 0.4})
	a.Scene().Add(grid)

	// Sets camera position
	a.Camera().GetCamera().SetPosition(0, 4, 12)
	a.Camera().GetCamera().LookAt(&math32.Vector3{0,0,0})

	// Creates listener and adds it to the current camera
	listener := audio.NewListener()
	a.Camera().GetCamera().Add(listener)

	// Creates player sphere
	t.ps1 = NewPlayerSphere(a, "Vivaldi1.wav", &math32.Color{1, 0, 0})
	t.ps1.SetPosition(0, 0, 0)
	t.ps1.speed = 1.00
	t.ps1.player.SetRolloffFactor(1)
	a.Scene().Add(t.ps1)

	t.ps2 = NewPlayerSphere(a, "Bach1.ogg", &math32.Color{0, 1, 0})
	t.ps2.SetPosition(0, 1, 0)
	t.ps2.speed = 0.90
	a.Scene().Add(t.ps2)

	t.ps3 = NewPlayerSphere(a, "bomb1.wav", &math32.Color{0, 0, 1})
	t.ps3.SetPosition(0, 2, 0)
	t.ps3.speed = 0.80
	a.Scene().Add(t.ps3)

	t.ps4 = NewPlayerSphere(a, "bomb2.ogg", &math32.Color{0, 1, 1})
	t.ps4.SetPosition(0, 3, 0)
	t.ps4.speed = 0.70
	a.Scene().Add(t.ps4)

	t.ps5 = NewPlayerSphere(a, "tone_440hz.wav", &math32.Color{1, 1, 0})
	t.ps5.SetPosition(0, 4, 0)
	t.ps5.speed = 0.60
	a.Scene().Add(t.ps5)

	t.ps6 = NewPlayerSphere(a, "tone_1khz.wav", &math32.Color{1, 0, 1})
	t.ps6.SetPosition(0, 5, 0)
	t.ps6.speed = 0.50
	a.Scene().Add(t.ps6)

	// Add controls
	if a.ControlFolder() == nil {
		return
	}
	g := a.ControlFolder().AddGroup("Play sources")
	cb1 := g.AddCheckBox("Vivaldi1").SetValue(true)
	cb1.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps1.Toggle()
	})
	cb2 := g.AddCheckBox("Bach1").SetValue(true)
	cb2.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps2.Toggle()
	})
	cb3 := g.AddCheckBox("bomb1").SetValue(true)
	cb3.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps3.Toggle()
	})
	cb4 := g.AddCheckBox("bomb2").SetValue(true)
	cb4.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps4.Toggle()
	})
	cb5 := g.AddCheckBox("tone_440hz").SetValue(true)
	cb5.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps5.Toggle()
	})
	cb6 := g.AddCheckBox("tone_1khz").SetValue(true)
	cb6.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		t.ps6.Toggle()
	})
}

func (t *AudioPosition) Render(a *app.App) {

	t.ps1.Update(a)
	t.ps2.Update(a)
	t.ps3.Update(a)
	t.ps4.Update(a)
	t.ps5.Update(a)
	t.ps6.Update(a)
}

type PlayerSphere struct {
	graphic.Mesh
	player *audio.Player
	start  time.Time
	label  *graphic.Sprite
	speed  float32
}

func NewPlayerSphere(a *app.App, filename string, color *math32.Color) *PlayerSphere {

	ps := new(PlayerSphere)

	// Creates audio source
	player, err := audio.NewPlayer(a.DirData() + "/audio/" + filename)
	if err != nil {
		a.Log().Fatal("error:%s", err)
	}
	ps.player = player

	// Creates sphere
	geom := geometry.NewSphere(0.2, 32, 32, 0, math32.Pi*2, 0, math32.Pi)
	mat := material.NewStandard(color)
	mat.SetUseLights(material.UseLightNone)
	mat.SetEmissiveColor(color)
	ps.Mesh.Init(geom, mat)
	ps.SetVisible(true)

	// Adds plane with file name
	font := gui.StyleDefault().Font
	font.SetPointSize(32)
	width, height := font.MeasureText(filename)
	canvas := text.NewCanvas(width, height, &math32.Color4{0, 0, 0, 0})
	aspect := float32(width) / float32(height)
	canvas.DrawText(0, 0, filename, font)
	tex := texture.NewTexture2DFromRGBA(canvas.RGBA)
	plane_mat := material.NewStandard(math32.NewColor("black"))
	plane_mat.SetTransparent(true)
	plane_mat.AddTexture(tex)
	ps.label = graphic.NewSprite(0.5*aspect, 0.5, plane_mat)
	ps.label.SetPosition(0, 0.4, 0)
	ps.Add(ps.label)

	// Set up player and adds it to the sphere
	ps.player.SetLooping(true)
	ps.player.Play()
	ps.start = time.Now()
	ps.speed = 1.0
	ps.Add(ps.player)
	return ps
}

func (ps *PlayerSphere) Toggle() {

	if ps.Visible() {
		ps.player.Pause()
		ps.SetVisible(false)
	} else {
		ps.player.Play()
		ps.SetVisible(true)
	}
}

func (ss *PlayerSphere) Update(a *app.App) {

	delta := time.Now().Sub(ss.start).Seconds()
	x := 8 * math32.Cos(float32(delta)*ss.speed)
	z := 8 * math32.Sin(float32(delta)*ss.speed)
	ss.SetPosition(x, ss.Position().Y, z)
}

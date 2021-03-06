package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/g3n/engine/audio/al"
	"github.com/g3n/engine/camera/control"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/util/application"
	"github.com/g3n/engine/util/logger"
	"github.com/g3n/engine/util/stats"
	"github.com/g3n/engine/window"
	"github.com/kardianos/osext"
)

// App contains the application state
type App struct {
	*application.Application                    // Embedded standard application object
	log                      *logger.Logger     // Application logger
	currentDemo              IDemo              // current test object
	dirData                  string             // full path of data directory
	labelFPS                 *gui.Label         // header FPS label
	treeTests                *gui.Tree          // tree with test names
	stats                    *stats.Stats       // statistics object
	statsTable               *stats.StatsTable  // statistics table panel
	control                  *gui.ControlFolder // Pointer to gui control panel
	ambLight                 *light.Ambient     // Scene default ambient light
	finalizers               []func()           // List of demo finalizers functions
}

// IDemo is the interface that must be satisfied for all demo objects
type IDemo interface {
	Initialize(*App) // Called once to initialize the demo
	Render(*App)     // Called at each frame for animations
}

// Command line options
// The standard application object may add other command line options
var (
	oNogui       = flag.Bool("nogui", false, "Do not show the GUI, only the specified demo")
	oHideFPS     = flag.Bool("hidefps", false, "Do now show calculated FPS in the GUI")
	oUpdateFPS   = flag.Uint("updatefps", 1000, "Time interval in milliseconds to update the FPS in the GUI")
	oLogs        = flag.String("logs", "", "Set log levels for packages. Ex: gui:debug,gls:info")
	oStats       = flag.Bool("stats", false, "Shows statistics control panel in the GUI")
	oRenderStats = flag.Bool("renderstats", false, "Shows gui renderer statistics in the console")
)

const (
	progName = "G3N Demo"
	execName = "g3nd"
	vmajor   = 0
	vminor   = 5
)

// Create creates the G3ND application using the specified map of demos
func Create(demoMap map[string]IDemo) *App {

	// Sets the application usage
	flag.Usage = usage

	// Creates standard application object
	a, err := application.Create(application.Options{
		Title:       progName,
		Width:       1000,
		Height:      600,
		Fullscreen:  false,
		LogPrefix:   "G3ND",
		LogLevel:    logger.DEBUG,
		TargetFPS:   60,
		EnableFlags: true,
	})
	if err != nil {
		panic(err)
	}
	app := new(App)
	app.Application = a
	app.log = app.Log()
	app.log.Info("%s v%d.%d starting", progName, vmajor, vminor)
	app.stats = stats.NewStats(app.Gl())

	// Apply log levels to engine package loggers
	if *oLogs != "" {
		logs := strings.Split(*oLogs, ",")
		for i := 0; i < len(logs); i++ {
			parts := strings.Split(logs[i], ":")
			if len(parts) != 2 {
				app.log.Error("Invalid logs level string")
				continue
			}
			pack := strings.ToUpper(parts[0])
			level := strings.ToUpper(parts[1])
			path := "G3N/" + pack
			packlog := logger.Find(path)
			if packlog == nil {
				app.log.Error("No logger for package:%s", pack)
				continue
			}
			err := packlog.SetLevelByName(level)
			if err != nil {
				app.log.Error("%s", err)
			}
			app.log.Info("Set log level:%s for package:%s", level, pack)
		}
	}

	// Check for data directory and aborts if not found
	app.dirData = app.checkDirData("data")
	app.log.Info("Using data directory:%s", app.dirData)

	// Open default audio device
	err = app.OpenDefaultAudioDevice()
	if err != nil {
		app.log.Error("%v", err)
	}

	// Builds user interface
	if *oNogui == false {
		app.buildGui(demoMap)
	}

	// Setup scene
	app.setupScene()

	// If name of test supplied in the command line
	// sets it as the current test and initialize it.
	if len(flag.Args()) > 0 {
		tname := flag.Args()[0]
		for name, test := range demoMap {
			if name == tname {
				app.currentDemo = test
				app.currentDemo.Initialize(app)
				break
			}
		}
		if app.currentDemo == nil {
			app.log.Error("Invalid demo name")
			usage()
			return nil
		}
	}

	// Subscribe to before render events to call current test Render method
	app.Subscribe(application.OnBeforeRender, func(evname string, ev interface{}) {
		if app.currentDemo != nil {
			app.currentDemo.Render(app)
		}
	})

	// Subscribe to after render events to update the FPS
	app.Subscribe(application.OnAfterRender, func(evname string, ev interface{}) {
		// Update statistics
		if app.stats.Update(time.Second) {
			if app.statsTable != nil {
				app.statsTable.Update(app.stats)
			}
		}
		// Update render stats
		if *oRenderStats {
			stats := app.Renderer().Stats()
			if stats.Panels > 0 {
				app.log.Debug("render stats:%+v", stats)
			}
		}
		// Update FPS
		app.updateFPS()
	})
	return app
}

// GuiPanel returns the current gui panel for demos to add elements to.
func (app *App) GuiPanel() *gui.Panel {

	if *oNogui {
		return &app.Gui().Panel
	}
	return app.Panel3D().GetPanel()
}

// DirData returns the base directory for data
func (app *App) DirData() string {

	return app.dirData
}

// ControlFolder returns the application control folder
func (app *App) ControlFolder() *gui.ControlFolder {

	return app.control
}

// AmbLight returns the default scene ambient light
func (app *App) AmbLight() *light.Ambient {

	return app.ambLight
}

// AddFinalizer adds a function which will be executed before another demo is started
func (app *App) AddFinalizer(f func()) {

	app.finalizers = append(app.finalizers, f)
}

// UpdateFPS updates the fps value in the window title or header label
func (app *App) updateFPS() {

	if *oHideFPS {
		return
	}

	// Get the FPS and potential FPS from the frameRater
	fps, pfps, ok := app.FrameRater().FPS(time.Duration(*oUpdateFPS) * time.Millisecond)
	if !ok {
		return
	}

	// Shows the values in the window title or header label
	msg := fmt.Sprintf("%3.1f / %3.1f", fps, pfps)
	if *oNogui {
		app.Window().SetTitle(msg)
	} else {
		app.labelFPS.SetText(msg)
	}
}

// setupScene resets the current scene for executing a new (or first) test
func (app *App) setupScene() {

	// Execute demo finalizers functions and clear finalizers list
	for i := 0; i < len(app.finalizers); i++ {
		app.finalizers[i]()
	}
	app.finalizers = app.finalizers[0:0]

	// Cancel next events and clear all window subscriptions
	app.Window().CancelDispatch()
	app.Window().ClearSubscriptions()
	app.GuiPanel().ClearSubscriptions()

	// Reset current cursor and clear all custom cursors
	app.Window().Manager().DisposeAllCursors()
	app.Window().SetStandardCursor(window.ArrowCursor)

	// Dispose of all test scene children
	app.Scene().DisposeChildren(true)
	if app.Panel3D() != nil {
		app.Panel3D().GetPanel().DisposeChildren(true)
	}

	// Sets default background color
	app.Gl().ClearColor(0.6, 0.6, 0.6, 1.0)

	// Reset renderer z-sorting flag
	app.Renderer().SetObjectSorting(true)

	// Adds ambient light to the test scene
	app.ambLight = light.NewAmbient(&math32.Color{1.0, 1.0, 1.0}, 0.5)
	app.Scene().Add(app.ambLight)

	// Sets perspective camera position
	width, height := app.Window().Size()
	aspect := float32(width) / float32(height)
	app.CameraPersp().SetPosition(0, 0, 5)
	app.CameraPersp().LookAt(&math32.Vector3{0, 0, 0})
	app.CameraPersp().SetAspect(aspect)

	// Sets orthographic camera
	app.CameraOrtho().SetPosition(0, 0, 3)
	app.CameraOrtho().LookAt(&math32.Vector3{0, 0, 0})
	app.CameraOrtho().SetZoom(1.0)

	// Default camera is perspective
	app.SetCamera(app.CameraPersp())
	// Adds camera to scene (important for audio demos)
	app.Scene().Add(app.Camera().GetCamera())

	// Subscribe to window key events
	app.Window().Subscribe(window.OnKeyDown, func(evname string, ev interface{}) {
		kev := ev.(*window.KeyEvent)
		// ESC terminates program
		if kev.Keycode == window.KeyEscape {
			app.Quit()
			return
		}
		// Alt F11 toggles full screen
		if kev.Keycode == window.KeyF11 && kev.Mods == window.ModAlt {
			app.Window().SetFullScreen(!app.Window().FullScreen())
			return
		}
		// Ctr-Alt-S prints statistics in the console
		if kev.Keycode == window.KeyS && kev.Mods == window.ModControl|window.ModAlt {
			app.logStats()
		}
	})

	// Subscribe to window resize events
	app.Window().Subscribe(window.OnWindowSize, func(evname string, ev interface{}) {
		app.OnWindowResize()
	})

	// Because all windows events were cleared
	// We need to inform the gui root panel to subscribe again.
	app.Gui().SubscribeWin()

	// Recreates the orbit camera control
	// It is important to do this after the root panel subscription
	// to avoid GUI events being propagated to the orbit control.
	app.SetOrbit(control.NewOrbitControl(app.Camera(), app.Window()))

	// If audio active, resets global listener parameters
	al.Listener3f(al.Position, 0, 0, 0)
	al.Listener3f(al.Velocity, 0, 0, 0)
	al.Listenerfv(al.Orientation, []float32{0, 0, -1, 0, 1, 0})

	// If no gui control folder, nothing more to do
	if app.control == nil {
		return
	}

	// Remove all controls and adds default ones
	app.control.Clear()

	// Adds camera selection
	cb := app.control.AddCheckBox("Perspective camera")
	cb.SetValue(true)
	cb.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		if cb.Value() {
			app.SetCamera(app.CameraPersp())
		} else {
			app.SetCamera(app.CameraOrtho())
		}
		app.OnWindowResize()
		// Recreates orbit camera control
		app.Orbit().Dispose()
		app.SetOrbit(control.NewOrbitControl(app.Camera(), app.Window()))
	})

	// Adds ambient light slider
	s1 := app.control.AddSlider("Ambient light:", 2.0, app.ambLight.Intensity())
	s1.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		app.ambLight.SetIntensity(s1.Value())
	})
}

// buildGui builds the tester GUI
func (app *App) buildGui(demoMap map[string]IDemo) {

	// Create dock layout for the tester root panel
	dl := gui.NewDockLayout()
	app.Gui().SetLayout(dl)

	// Add transparent panel at the center to contain demos
	center := gui.NewPanel(0, 0)
	center.SetRenderable(false)
	center.SetColor4(&gui.StyleDefault().Scroller.BgColor)
	center.SetLayoutParams(&gui.DockLayoutParams{Edge: gui.DockCenter})
	app.Gui().Add(center)
	app.SetPanel3D(center)

	// Adds header after the gui central panel to ensure that the control folder
	// stays over the gui panel when opened.
	headerColor := math32.Color4{13.0/256.0, 41.0/256.0, 62.0/256.0, 1}
	lightTextColor := math32.Color4{0.8, 0.8, 0.8, 1}
	header := gui.NewPanel(600, 40)
	header.SetBorders(0, 0, 1, 0)
	header.SetPaddings(4, 4, 4, 4)
	header.SetColor4(&headerColor)
	header.SetLayoutParams(&gui.DockLayoutParams{Edge: gui.DockTop})

	// Horizontal box layout for the header
	hbox := gui.NewHBoxLayout()
	header.SetLayout(hbox)
	app.Gui().Add(header)

	// Add an optional image to header
	logo, err := gui.NewImage(app.dirData + "/images/g3n_logo_32.png")
	if err == nil {
		logo.SetContentAspectWidth(32)
		header.Add(logo)
	}

	// Header title
	const fontSize = 20
	title := gui.NewLabel(" ")
	title.SetFontSize(fontSize)
	title.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignCenter})
	title.SetText(fmt.Sprintf("%s v%d.%d", progName, vmajor, vminor))
	title.SetColor4(&lightTextColor)
	header.Add(title)
	// FPS
	if !*oHideFPS {
		l1 := gui.NewLabel(" ")
		l1.SetFontSize(fontSize)
		l1.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignCenter})
		l1.SetText("  FPS: ")
		l1.SetColor4(&lightTextColor)
		header.Add(l1)
		// FPS value
		app.labelFPS = gui.NewLabel(" ")
		app.labelFPS.SetFontSize(fontSize)
		app.labelFPS.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignCenter})
		app.labelFPS.SetColor4(&lightTextColor)
		header.Add(app.labelFPS)
	}

	// New styles for control folder
	styles := gui.StyleDefault().ControlFolder
	styles.Folder.Normal.BgColor = headerColor
	styles.Folder.Over.BgColor = headerColor
	styles.Folder.Normal.FgColor = lightTextColor
	styles.Folder.Over.FgColor = lightTextColor

	// Adds statistics table control folder if requested
	if *oStats {
		// Adds spacer to right justify the control folder in the header
		spacer := gui.NewPanel(0, 0)
		spacer.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignBottom, Expand: 1.2})
		header.Add(spacer)

		// Creates control folder for statistics table
		statsControlFolder := gui.NewControlFolder("Stats", 100)
		statsControlFolder.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignBottom})
		statsControlFolder.SetStyles(&styles)
		header.Add(statsControlFolder)

		// Adds stats table in the control folder
		app.statsTable = stats.NewStatsTable(220, 200, app.Gl())
		statsControlFolder.AddPanel(app.statsTable)
	}

	// Adds spacer to right justify the control folder in the header
	spacer := gui.NewPanel(0, 0)
	spacer.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignBottom, Expand: 1})
	header.Add(spacer)

	// Adds control folder in the header
	app.control = gui.NewControlFolder("Controls", 100)
	app.control.SetLayoutParams(&gui.HBoxLayoutParams{AlignV: gui.AlignBottom})
	app.control.SetStyles(&styles)
	header.Add(app.control)

	// Test list
	app.treeTests = gui.NewTree(175, 0)
	app.treeTests.SetLayoutParams(&gui.DockLayoutParams{Edge: gui.DockLeft})
	// Sort test names
	tnames := []string{}
	nodes := make(map[string]*gui.TreeNode)
	for name := range demoMap {
		tnames = append(tnames, name)
	}
	sort.Strings(tnames)
	// Add items to the list
	for _, name := range tnames {
		parts := strings.Split(name, ".")
		if len(parts) > 1 {
			category := parts[0]
			node := nodes[category]
			if node == nil {
				node = app.treeTests.AddNode(category)
				nodes[category] = node
			}
			labelText := strings.Join(parts[1:], ".")
			item := gui.NewLabel(labelText)
			item.SetUserData(demoMap[name])
			node.Add(item)
		} else {
			item := gui.NewLabel(name)
			item.SetUserData(demoMap[name])
			app.treeTests.Add(item)
		}
	}
	app.treeTests.Subscribe(gui.OnChange, func(evname string, ev interface{}) {
		sel := app.treeTests.Selected()
		if sel == nil {
			return
		}
		label, ok := sel.(*gui.Label)
		if ok {
			app.setupScene()
			test := label.GetNode().UserData().(IDemo)
			test.Initialize(app)
			app.currentDemo = test
		}
	})
	app.Gui().Add(app.treeTests)
}

// logStats generate log with current statistics
func (app *App) logStats() {

	const statsFormat = `
         Shaders: %d
            Vaos: %d
         Buffers: %d
        Textures: %d
  Uniforms/frame: %d
Draw calls/frame: %d
 CGO calls/frame: %d
`
	app.log.Info(statsFormat,
		app.stats.Glstats.Shaders,
		app.stats.Glstats.Vaos,
		app.stats.Glstats.Buffers,
		app.stats.Glstats.Textures,
		app.stats.Unisets,
		app.stats.Drawcalls,
		app.stats.Cgocalls,
	)
}

// checkDirData try to find and return the complete data directory path.
// Aborts if not found
func (app *App) checkDirData(dirDataName string) string {

	// Checks first if data directory is in the current directory
	if _, err := os.Stat(dirDataName); err == nil {
		dirData, err := filepath.Abs(dirDataName)
		if err != nil {
			panic(err)
		}
		return dirData
	}

	// Get the executable path
	execPath, err := osext.Executable()
	if err != nil {
		panic(err)
	}

	// Checks if data directory is in the executable directory
	execDir := filepath.Dir(execPath)
	path := filepath.Join(execDir, dirDataName)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Assumes the executable is in $GOPATH/bin
	goPath := filepath.Dir(execDir)
	path = filepath.Join(goPath, "src", "github.com", "g3n", "g3nd", dirDataName)
	// Checks data path
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// If the data directory hasn't been found, manually scan the $GOPATH directories
	rawPaths := os.Getenv("GOPATH")
	paths := strings.Split(rawPaths, ":")
	for _, j := range paths {
		// Checks data path
		path = filepath.Join(j, "src", "github.com", "g3n", "g3nd", dirDataName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Shows error message and aborts
	app.log.Fatal("Data directory NOT FOUND")
	return ""
}

// usage shows the application usage
func usage() {

	fmt.Fprintf(os.Stderr, "%s v%d.%d\n", progName, vmajor, vminor)
	fmt.Fprintf(os.Stderr, "usage: %s [options] [<test>] \n", execName)
	flag.PrintDefaults()
	os.Exit(2)
}

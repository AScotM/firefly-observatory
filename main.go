package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Config struct {
	Agents    int     `json:"agents"`
	FPS       int     `json:"fps"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Seed      int64   `json:"seed"`
	Palette   string  `json:"palette"`
	Mode      string  `json:"mode"`
	Coupling  float64 `json:"coupling"`
	Radius    float64 `json:"radius"`
	Speed     float64 `json:"speed"`
	Noise     float64 `json:"noise"`
	Trails    float64 `json:"trails"`
	Telemetry bool    `json:"telemetry"`
	Duration  float64 `json:"duration"`
	Output    string  `json:"output"`
	Frames    int     `json:"frames"`
	Density   float64 `json:"density"`
	Border    bool    `json:"border"`
	Stats     bool    `json:"stats"`
	ASCII     bool    `json:"ascii"`
	Record    string  `json:"record"`
	Snapshot  string  `json:"snapshot"`
	Restore   string  `json:"restore"`
	Headless  bool    `json:"headless"`
	Quiet     bool    `json:"quiet"`
	Wind      float64 `json:"wind"`
	Gravity   float64 `json:"gravity"`
	Bloom     float64 `json:"bloom"`
}
type Agent struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	VX        float64 `json:"vx"`
	VY        float64 `json:"vy"`
	Phase     float64 `json:"phase"`
	Frequency float64 `json:"frequency"`
	Energy    float64 `json:"energy"`
	Size      float64 `json:"size"`
	Family    int     `json:"family"`
	Age       float64 `json:"age"`
}
type Snapshot struct {
	Version int     `json:"version"`
	Config  Config  `json:"config"`
	Agents  []Agent `json:"agents"`
	Time    float64 `json:"time"`
	Frame   int     `json:"frame"`
	Created string  `json:"created"`
}
type Color struct{ R, G, B float64 }
type Pixel struct{ R, G, B float64 }
type Field struct {
	W, H int
	Data []Pixel
}
type Telemetry struct {
	CPU, Memory, IO           float64
	LastCPU, LastIdle, LastIO uint64
	Ready                     bool
}
type World struct {
	C          Config
	Agents     []Agent
	RNG        *rand.Rand
	T          float64
	Frame      int
	Telemetry  Telemetry
	Order      float64
	MeanEnergy float64
	Grid       map[int][]int
	GridSize   float64
}
type Recorder struct {
	File    *os.File
	Writer  *bufio.Writer
	Encoder *json.Encoder
}
type Event struct {
	Type   string  `json:"type"`
	Frame  int     `json:"frame"`
	Time   float64 `json:"time"`
	Order  float64 `json:"order"`
	Energy float64 `json:"energy"`
	Agents int     `json:"agents"`
	CPU    float64 `json:"cpu,omitempty"`
	Memory float64 `json:"memory,omitempty"`
	IO     float64 `json:"io,omitempty"`
}

var palettes = map[string][]Color{
	"aurora": {{40, 190, 190}, {70, 255, 160}, {80, 125, 255}, {255, 195, 110}},
	"ember":  {{255, 110, 45}, {255, 195, 60}, {255, 80, 100}, {255, 235, 160}},
	"moon":   {{80, 140, 225}, {150, 195, 255}, {190, 165, 255}, {215, 235, 255}},
	"forest": {{40, 145, 80}, {100, 220, 100}, {180, 245, 100}, {255, 215, 100}},
	"ocean":  {{25, 120, 195}, {45, 220, 230}, {120, 175, 255}, {170, 250, 245}},
	"violet": {{130, 65, 225}, {205, 105, 245}, {255, 145, 220}, {115, 175, 255}},
	"mono":   {{120, 120, 120}, {170, 170, 170}, {215, 215, 215}, {255, 255, 255}},
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func wrap(v, max float64) float64 {
	if max <= 0 {
		return 0
	}
	v = math.Mod(v, max)
	if v < 0 {
		v += max
	}
	return v
}
func torusDelta(a, b, size float64) float64 {
	d := b - a
	if d > size/2 {
		d -= size
	}
	if d < -size/2 {
		d += size
	}
	return d
}
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
func colorMix(a, b Color, t float64) Color {
	return Color{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t)}
}
func defaultConfig() Config {
	return Config{Agents: 420, FPS: 24, Width: 0, Height: 0, Seed: 0, Palette: "aurora", Mode: "flock", Coupling: 1.4, Radius: 6.5, Speed: 1, Noise: 0.22, Trails: 0.83, Duration: 0, Density: 1, Border: false, Stats: true, Wind: 0.08, Gravity: 0.04, Bloom: 0.85}
}
func parse() (string, Config, error) {
	command := "run"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}
	c := defaultConfig()
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.IntVar(&c.Agents, "agents", c.Agents, "number of luminous agents")
	fs.IntVar(&c.FPS, "fps", c.FPS, "frames per second")
	fs.IntVar(&c.Width, "width", c.Width, "canvas width (0 = terminal)")
	fs.IntVar(&c.Height, "height", c.Height, "canvas height (0 = terminal)")
	fs.Int64Var(&c.Seed, "seed", c.Seed, "reproducible random seed (0 = random)")
	fs.StringVar(&c.Palette, "palette", c.Palette, "aurora, ember, moon, forest, ocean, violet, mono")
	fs.StringVar(&c.Mode, "mode", c.Mode, "flock, drift, orbit, breathe, swarm, rain")
	fs.Float64Var(&c.Coupling, "coupling", c.Coupling, "oscillator coupling strength")
	fs.Float64Var(&c.Radius, "radius", c.Radius, "neighborhood interaction radius")
	fs.Float64Var(&c.Speed, "speed", c.Speed, "simulation speed multiplier")
	fs.Float64Var(&c.Noise, "noise", c.Noise, "stochastic motion strength")
	fs.Float64Var(&c.Trails, "trails", c.Trails, "persistence from 0 to 0.99")
	fs.BoolVar(&c.Telemetry, "telemetry", c.Telemetry, "modulate with Linux CPU, memory and disk activity")
	fs.Float64Var(&c.Duration, "duration", c.Duration, "stop after seconds (0 = unlimited)")
	fs.StringVar(&c.Output, "output", c.Output, "output path for render or report")
	fs.IntVar(&c.Frames, "frames", c.Frames, "frame limit (0 = unlimited)")
	fs.Float64Var(&c.Density, "density", c.Density, "brightness multiplier")
	fs.BoolVar(&c.Border, "border", c.Border, "draw decorative frame")
	fs.BoolVar(&c.Stats, "stats", c.Stats, "show status line")
	fs.BoolVar(&c.ASCII, "ascii", c.ASCII, "ASCII brightness glyphs instead of truecolor")
	fs.StringVar(&c.Record, "record", c.Record, "write JSONL telemetry timeline")
	fs.StringVar(&c.Snapshot, "snapshot", c.Snapshot, "write final world snapshot")
	fs.StringVar(&c.Restore, "restore", c.Restore, "restore world snapshot")
	fs.BoolVar(&c.Headless, "headless", c.Headless, "simulate without terminal animation")
	fs.BoolVar(&c.Quiet, "quiet", c.Quiet, "suppress progress output")
	fs.Float64Var(&c.Wind, "wind", c.Wind, "horizontal wind strength")
	fs.Float64Var(&c.Gravity, "gravity", c.Gravity, "attraction to canvas center")
	fs.Float64Var(&c.Bloom, "bloom", c.Bloom, "glow radius multiplier")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return "", c, err
	}
	if fs.NArg() != 0 {
		return "", c, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if c.Agents < 1 || c.Agents > 30000 {
		return "", c, errors.New("agents must be between 1 and 30000")
	}
	if c.FPS < 1 || c.FPS > 120 {
		return "", c, errors.New("fps must be between 1 and 120")
	}
	if c.Radius <= 0 || c.Speed <= 0 || c.Coupling < 0 || c.Trails < 0 || c.Trails >= 1 || c.Density <= 0 || c.Bloom < 0 {
		return "", c, errors.New("invalid physical parameter")
	}
	if _, ok := palettes[c.Palette]; !ok {
		return "", c, errors.New("unknown palette")
	}
	if !strings.Contains("|flock|drift|orbit|breathe|swarm|rain|", "|"+c.Mode+"|") {
		return "", c, errors.New("unknown mode")
	}
	return command, c, nil
}
func dimensions() (int, int) {
	w, h := 90, 30
	if s := os.Getenv("COLUMNS"); s != "" {
		if n, e := strconv.Atoi(s); e == nil && n > 0 {
			w = n
		}
	}
	if s := os.Getenv("LINES"); s != "" {
		if n, e := strconv.Atoi(s); e == nil && n > 0 {
			h = n
		}
	}
	if f, e := os.Open("/dev/tty"); e == nil {
		defer f.Close()
		var ws struct{ Row, Col, X, Y uint16 }
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
		if errno == 0 && ws.Col > 0 && ws.Row > 0 {
			w = int(ws.Col)
			h = int(ws.Row)
		}
	}
	return w, h
}
func newWorld(c Config) *World {
	if c.Seed == 0 {
		c.Seed = time.Now().UnixNano()
	}
	w := &World{C: c, RNG: rand.New(rand.NewSource(c.Seed)), Grid: make(map[int][]int)}
	w.Agents = make([]Agent, c.Agents)
	for i := range w.Agents {
		w.Agents[i] = Agent{X: w.RNG.Float64() * float64(c.Width), Y: w.RNG.Float64() * float64(c.Height), VX: (w.RNG.Float64() - .5) * 2, VY: (w.RNG.Float64() - .5) * 2, Phase: w.RNG.Float64() * 2 * math.Pi, Frequency: 0.65 + w.RNG.Float64()*1.3, Energy: 0.5 + w.RNG.Float64()*.5, Size: 0.6 + w.RNG.Float64()*.8, Family: w.RNG.Intn(4)}
	}
	return w
}
func (w *World) buildGrid() {
	w.Grid = make(map[int][]int, len(w.Agents)/3)
	w.GridSize = math.Max(1, w.C.Radius)
	cols := int(math.Ceil(float64(w.C.Width) / w.GridSize))
	for i, a := range w.Agents {
		x := int(a.X / w.GridSize)
		y := int(a.Y / w.GridSize)
		w.Grid[y*cols+x] = append(w.Grid[y*cols+x], i)
	}
}
func (w *World) neighbors(i int, fn func(int, float64, float64, float64)) {
	a := w.Agents[i]
	cell := w.GridSize
	cols := int(math.Ceil(float64(w.C.Width) / cell))
	rows := int(math.Ceil(float64(w.C.Height) / cell))
	cx := int(a.X / cell)
	cy := int(a.Y / cell)
	seen := make(map[int]bool, 16)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x := (cx + dx + cols) % cols
			y := (cy + dy + rows) % rows
			for _, j := range w.Grid[y*cols+x] {
				if i == j || seen[j] {
					continue
				}
				seen[j] = true
				b := w.Agents[j]
				xx := torusDelta(a.X, b.X, float64(w.C.Width))
				yy := torusDelta(a.Y, b.Y, float64(w.C.Height))
				d := math.Hypot(xx, yy)
				if d < w.C.Radius {
					fn(j, xx, yy, d)
				}
			}
		}
	}
}
func (w *World) step(dt float64) {
	w.buildGrid()
	next := make([]Agent, len(w.Agents))
	sx, sy, energy := 0., 0., 0.
	c := w.C
	cx := float64(c.Width) / 2
	cy := float64(c.Height) / 2
	for i, a := range w.Agents {
		count := 0.
		sinSum := 0.
		alignX, alignY := 0., 0.
		repelX, repelY := 0., 0.
		w.neighbors(i, func(j int, dx, dy, d float64) {
			b := w.Agents[j]
			sinSum += math.Sin(b.Phase - a.Phase)
			alignX += b.VX
			alignY += b.VY
			if d > 0.001 {
				f := 1 / (d*d + 0.2)
				repelX -= dx * f
				repelY -= dy * f
			}
			count++
		})
		coupling := c.Coupling
		if c.Telemetry {
			coupling *= 1 + .35*w.Telemetry.CPU
		}
		a.Phase = wrap(a.Phase+dt*(a.Frequency+coupling*sinSum/math.Max(1, count)), 2*math.Pi)
		nx := w.RNG.NormFloat64() * c.Noise
		ny := w.RNG.NormFloat64() * c.Noise
		switch c.Mode {
		case "flock":
			if count > 0 {
				nx += 0.3*(alignX/count-a.VX) + 0.12*repelX
				ny += 0.3*(alignY/count-a.VY) + 0.12*repelY
			}
		case "orbit":
			dx := torusDelta(a.X, cx, float64(c.Width))
			dy := torusDelta(a.Y, cy, float64(c.Height))
			r := math.Max(1, math.Hypot(dx, dy))
			nx += -dy/r*.7 + dx/r*.08
			ny += dx/r*.7 + dy/r*.08
		case "breathe":
			nx += math.Sin(w.T*.7+a.Phase) * .3
			ny += math.Cos(w.T*.7+a.Phase) * .3
		case "swarm":
			nx += repelX*.35 + math.Sin(w.T*.17+float64(a.Family))*.3
			ny += repelY*.35 + math.Cos(w.T*.13+float64(a.Family))*.3
		case "rain":
			ny += .6
			nx += math.Sin(w.T+a.Y*.05) * .15
		case "drift":
			nx += math.Sin(w.T*.2+a.Y*.03) * .08
			ny += math.Cos(w.T*.2+a.X*.03) * .08
		}
		if c.Mode != "rain" {
			nx += torusDelta(a.X, cx, float64(c.Width)) / math.Max(1, float64(c.Width)) * c.Gravity
			ny += torusDelta(a.Y, cy, float64(c.Height)) / math.Max(1, float64(c.Height)) * c.Gravity
		}
		nx += c.Wind * .3
		a.VX = clamp(a.VX*.985+nx*dt, -2.8, 2.8)
		a.VY = clamp(a.VY*.985+ny*dt, -2.8, 2.8)
		a.X = wrap(a.X+a.VX*dt*2.2, float64(c.Width))
		a.Y = wrap(a.Y+a.VY*dt*1.1, float64(c.Height))
		a.Age += dt
		a.Energy = clamp(.55+.45*math.Sin(a.Phase), 0, 1)
		next[i] = a
		sx += math.Cos(a.Phase)
		sy += math.Sin(a.Phase)
		energy += a.Energy
	}
	w.Agents = next
	w.T += dt
	w.Frame++
	w.Order = math.Hypot(sx, sy) / float64(len(next))
	w.MeanEnergy = energy / float64(len(next))
}
func newField(w, h int) *Field { return &Field{W: w, H: h, Data: make([]Pixel, w*h)} }
func (f *Field) fade(k float64) {
	for i := range f.Data {
		f.Data[i].R *= k
		f.Data[i].G *= k
		f.Data[i].B *= k
	}
}
func (f *Field) glow(x, y, r float64, c Color, strength float64) {
	if r < .3 {
		r = .3
	}
	span := int(math.Ceil(r * 2.7))
	ix, iy := int(x), int(y)
	for dy := -span; dy <= span; dy++ {
		py := iy + dy
		if py < 0 || py >= f.H {
			continue
		}
		for dx := -span; dx <= span; dx++ {
			px := ix + dx
			if px < 0 || px >= f.W {
				continue
			}
			d2 := math.Pow(float64(px)-x, 2) + math.Pow((float64(py)-y)*1.7, 2)
			v := strength * math.Exp(-d2/(2*r*r))
			p := &f.Data[py*f.W+px]
			p.R = clamp(p.R+c.R*v, 0, 255)
			p.G = clamp(p.G+c.G*v, 0, 255)
			p.B = clamp(p.B+c.B*v, 0, 255)
		}
	}
}
func (w *World) paint(f *Field) {
	f.fade(w.C.Trails)
	colors := palettes[w.C.Palette]
	for _, a := range w.Agents {
		base := colors[a.Family%len(colors)]
		pulse := math.Pow(a.Energy, 3)
		v := (.06 + pulse*.8) * w.C.Density
		f.glow(a.X, a.Y, (.65+a.Size*.4)*w.C.Bloom, base, v)
		if pulse > .72 {
			f.glow(a.X, a.Y, .45, Color{255, 255, 235}, (pulse-.7)*1.5)
		}
	}
}
func ansi(f *Field, c Config, w *World) []byte {
	var b bytes.Buffer
	b.Grow(f.W * f.H * 25)
	b.WriteString("\x1b[H")
	chars := " .,:;irsXA253hMHGS#9B&@"
	last := ""
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			p := f.Data[y*f.W+x]
			lum := clamp((p.R*.2126+p.G*.7152+p.B*.0722)/255, 0, 1)
			if c.ASCII {
				idx := int(lum * float64(len(chars)-1))
				b.WriteByte(chars[idx])
				continue
			}
			glyph := " "
			if lum > .045 {
				glyph = "·"
			}
			if lum > .15 {
				glyph = "•"
			}
			if lum > .43 {
				glyph = "✦"
			}
			if lum > .76 {
				glyph = "✶"
			}
			r, g, bl := int(p.R), int(p.G), int(p.B)
			code := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bl)
			if code != last {
				b.WriteString(code)
				last = code
			}
			b.WriteString(glyph)
		}
		b.WriteString("\x1b[0m\x1b[K")
		last = ""
		if y < f.H-1 {
			b.WriteByte('\n')
		}
	}
	if c.Stats {
		fmt.Fprintf(&b, "\n\x1b[2m firefly observatory | %s | %d agents | t %.1fs | coherence %.3f | glow %.3f | seed %d\x1b[0m\x1b[K", c.Mode, len(w.Agents), w.T, w.Order, w.MeanEnergy, c.Seed)
		if c.Telemetry {
			fmt.Fprintf(&b, "\n\x1b[2m CPU %.1f%% | memory %.1f%% | disk %.1f MB/s\x1b[0m\x1b[K", w.Telemetry.CPU*100, w.Telemetry.Memory*100, w.Telemetry.IO)
		}
	}
	return b.Bytes()
}
func readCPU() (uint64, uint64) {
	b, e := os.ReadFile("/proc/stat")
	if e != nil {
		return 0, 0
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	v := strings.Fields(line)
	var total, idle uint64
	for i, s := range v[1:] {
		n, _ := strconv.ParseUint(s, 10, 64)
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle
}
func readMemory() float64 {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return 0
	}
	var total, available float64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		if f[0] == "MemTotal:" {
			total = v
		}
		if f[0] == "MemAvailable:" {
			available = v
		}
	}
	if total == 0 {
		return 0
	}
	return clamp(1-available/total, 0, 1)
}
func readIO() uint64 {
	b, e := os.ReadFile("/proc/diskstats")
	if e != nil {
		return 0
	}
	var sum uint64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		name := f[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
			continue
		}
		r, _ := strconv.ParseUint(f[5], 10, 64)
		wr, _ := strconv.ParseUint(f[9], 10, 64)
		sum += (r + wr) * 512
	}
	return sum
}
func (w *World) pollTelemetry(dt float64) {
	if !w.C.Telemetry {
		return
	}
	total, idle := readCPU()
	io := readIO()
	t := &w.Telemetry
	if t.Ready && total > t.LastCPU && io >= t.LastIO {
		busy := 1 - float64(idle-t.LastIdle)/float64(total-t.LastCPU)
		t.CPU = clamp(lerp(t.CPU, busy, .3), 0, 1)
		t.IO = lerp(t.IO, float64(io-t.LastIO)/math.Max(.001, dt)/1e6, .3)
	}
	t.Memory = readMemory()
	t.LastCPU = total
	t.LastIdle = idle
	t.LastIO = io
	t.Ready = true
}
func saveSnapshot(path string, w *World) error {
	if path == "" {
		return nil
	}
	s := Snapshot{Version: 1, Config: w.C, Agents: w.Agents, Time: w.T, Frame: w.Frame, Created: time.Now().UTC().Format(time.RFC3339)}
	return writeJSON(path, s)
}
func writeJSON(path string, v interface{}) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func loadSnapshot(path string) (Snapshot, error) {
	var s Snapshot
	b, e := os.ReadFile(path)
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	if e != nil {
		return s, e
	}
	if s.Version != 1 || len(s.Agents) == 0 {
		return s, errors.New("unsupported or empty snapshot")
	}
	return s, nil
}
func newRecorder(path string) (*Recorder, error) {
	if path == "" {
		return nil, nil
	}
	f, e := os.Create(path)
	if e != nil {
		return nil, e
	}
	r := &Recorder{File: f, Writer: bufio.NewWriter(f)}
	r.Encoder = json.NewEncoder(r.Writer)
	return r, nil
}
func (r *Recorder) add(w *World) error {
	if r == nil {
		return nil
	}
	return r.Encoder.Encode(Event{Type: "frame", Frame: w.Frame, Time: w.T, Order: w.Order, Energy: w.MeanEnergy, Agents: len(w.Agents), CPU: w.Telemetry.CPU, Memory: w.Telemetry.Memory, IO: w.Telemetry.IO})
}
func (r *Recorder) close() error {
	if r == nil {
		return nil
	}
	e := r.Writer.Flush()
	e2 := r.File.Close()
	if e != nil {
		return e
	}
	return e2
}
func exportPPM(path string, f *Field) error {
	file, e := os.Create(path)
	if e != nil {
		return e
	}
	defer file.Close()
	if _, e = fmt.Fprintf(file, "P6\n%d %d\n255\n", f.W, f.H); e != nil {
		return e
	}
	b := make([]byte, f.W*f.H*3)
	for i, p := range f.Data {
		b[i*3] = byte(clamp(p.R, 0, 255))
		b[i*3+1] = byte(clamp(p.G, 0, 255))
		b[i*3+2] = byte(clamp(p.B, 0, 255))
	}
	_, e = file.Write(b)
	return e
}
func exportText(path string, f *Field) error {
	var b strings.Builder
	chars := " .,:;irsXA253hMHGS#9B&@"
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			p := f.Data[y*f.W+x]
			lum := clamp((p.R*.2126+p.G*.7152+p.B*.0722)/255, 0, 1)
			b.WriteByte(chars[int(lum*float64(len(chars)-1))])
		}
		b.WriteByte('\n')
	}
	if path == "" {
		fmt.Print(b.String())
		return nil
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
func printPalettes() {
	names := make([]string, 0, len(palettes))
	for k := range palettes {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
}
func printModes() { fmt.Println("flock\ndrift\norbit\nbreathe\nswarm\nrain") }
func info() {
	fmt.Printf("Firefly Observatory 1.0.0\nGo %s | %s/%s\nStandard library only | GPL-3.0-only\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
func run(command string, c Config) error {
	if command == "palettes" {
		printPalettes()
		return nil
	}
	if command == "modes" {
		printModes()
		return nil
	}
	if command == "version" {
		info()
		return nil
	}
	if command != "run" && command != "render" && command != "inspect" {
		return fmt.Errorf("unknown command %q", command)
	}
	tw, th := dimensions()
	if c.Width == 0 {
		c.Width = tw
	}
	if c.Height == 0 {
		c.Height = th - 2
	}
	if c.Telemetry && c.Stats {
		c.Height--
	}
	c.Width = int(clamp(float64(c.Width), 12, 500))
	c.Height = int(clamp(float64(c.Height), 6, 200))
	if c.Seed == 0 {
		c.Seed = time.Now().UnixNano()
	}
	if command == "render" {
		if c.Frames == 0 {
			c.Frames = 240
		}
		c.Headless = true
	}
	if command == "inspect" {
		c.Headless = true
		if c.Frames == 0 {
			c.Frames = 600
		}
	}
	w := newWorld(c)
	if c.Restore != "" {
		s, e := loadSnapshot(c.Restore)
		if e != nil {
			return e
		}
		w.Agents = s.Agents
		w.T = s.Time
		w.Frame = s.Frame
		for i := range w.Agents {
			w.Agents[i].X = wrap(w.Agents[i].X, float64(c.Width))
			w.Agents[i].Y = wrap(w.Agents[i].Y, float64(c.Height))
		}
	}
	field := newField(c.Width, c.Height)
	rec, e := newRecorder(c.Record)
	if e != nil {
		return e
	}
	defer rec.close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "run" && !c.Headless {
		fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J")
		defer fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
	}
	dt := c.Speed / float64(c.FPS)
	interval := time.Second / time.Duration(c.FPS)
	next := time.Now()
	lastPoll := -1.
	initialFrame := w.Frame
	for {
		select {
		case <-ctx.Done():
			goto finished
		default:
		}
		if c.Frames > 0 && w.Frame-initialFrame >= c.Frames {
			break
		}
		if c.Duration > 0 && w.T >= c.Duration {
			break
		}
		if c.Telemetry && (lastPoll < 0 || w.T-lastPoll >= 1) {
			w.pollTelemetry(1)
			lastPoll = w.T
		}
		w.step(dt)
		w.paint(field)
		if e := rec.add(w); e != nil {
			return e
		}
		if command == "run" && !c.Headless {
			if _, e := os.Stdout.Write(ansi(field, c, w)); e != nil {
				return e
			}
			next = next.Add(interval)
			if delay := time.Until(next); delay > 0 {
				time.Sleep(delay)
			} else if delay < -interval*3 {
				next = time.Now()
			}
		} else if command == "run" && c.Frames == 0 && c.Duration == 0 {
			return errors.New("headless run requires --frames or --duration")
		}
	}
finished:
	if e := saveSnapshot(c.Snapshot, w); e != nil {
		return e
	}
	if command == "render" {
		out := c.Output
		if out == "" {
			out = "fireflies.ppm"
		}
		if strings.HasSuffix(strings.ToLower(out), ".txt") {
			return exportText(out, field)
		}
		if !strings.HasSuffix(strings.ToLower(out), ".ppm") {
			return errors.New("render output must end in .ppm or .txt")
		}
		if e := exportPPM(out, field); e != nil {
			return e
		}
		if !c.Quiet {
			fmt.Printf("rendered %s (%dx%d, %d frames)\n", out, field.W, field.H, w.Frame-initialFrame)
		}
		return nil
	}
	if command == "inspect" {
		report := map[string]interface{}{"version": 1, "seed": c.Seed, "frames": w.Frame - initialFrame, "time": w.T, "coherence": w.Order, "mean_energy": w.MeanEnergy, "agents": len(w.Agents), "palette": c.Palette, "mode": c.Mode, "telemetry": w.Telemetry}
		if c.Output != "" {
			return writeJSON(c.Output, report)
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
	} else if c.Headless && !c.Quiet {
		fmt.Printf("frames=%d time=%.2f coherence=%.4f mean_energy=%.4f agents=%d seed=%d\n", w.Frame-initialFrame, w.T, w.Order, w.MeanEnergy, len(w.Agents), c.Seed)
	}
	return nil
}
func main() {
	command, c, e := parse()
	if e == nil {
		e = run(command, c)
	}
	if e != nil {
		if !errors.Is(e, io.EOF) {
			fmt.Fprintln(os.Stderr, "firefly:", e)
		}
		os.Exit(1)
	}
}

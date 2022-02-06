package pkg

import (
	"fmt"
	"io/ioutil"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

type State int
type InnerState int

const (
	VOID State = iota
	CONTRACTED
	EXPANDEDL  // LEFT
	EXPANDEDR  // RIGHT
	EXPANDEDUL // UPPER LEFT
	EXPANDEDUR // UPPER RIGHT
	EXPANDEDLL // LOWER LEFT
	EXPANDEDLR // LOWER RIGHT
)

const (
	SLEEP InnerState = iota
	AWAKE
)

type Particle struct {
	state  State
	iState InnerState
	round  int // the minimum of all contracted particle rounds is the current round
	deg    int
	n1     []string
	n2     []string
}

func (p *Particle) Init() *Particle {
	p.n1 = make([]string, 6)
	p.n2 = make([]string, 6)

	return p
}

func (p *Particle) SetNeighbors(n1 []string, n2 []string) error {
	n := copy(p.n1, n1)
	n += copy(p.n2, n2)

	if n != 12 {
		return fmt.Errorf("error on copy neighbors")
	}

	return nil
}

func (p *Particle) GetNeighbors() ([]string, []string) {
	return p.n1, p.n2
}

func (p *Particle) SetDeg(n int) error {
	if n < 0 || n > 6 {
		return fmt.Errorf("%d is not a valid degree number", n)
	}

	p.deg = n

	return nil
}

func (p *Particle) GetDeg() int {
	return p.deg
}

func (p *Particle) SetStateN(n int) error {
	switch n {
	case 0:
		p.state = VOID
	case 1:
		p.state = CONTRACTED
	case 2:
		p.state = EXPANDEDL
	case 3:
		p.state = EXPANDEDR
	case 4:
		p.state = EXPANDEDUL
	case 5:
		p.state = EXPANDEDUR
	case 6:
		p.state = EXPANDEDLL
	case 7:
		p.state = EXPANDEDLR
	default:
		return fmt.Errorf("%d is not a valid state number", n)
	}

	return nil
}

func (p *Particle) SetStateS(s string) error {
	switch s {
	case "VOID":
		p.state = VOID
	case "CONTRACTED":
		p.state = CONTRACTED
	case "EXPANDEDL":
		p.state = EXPANDEDL
	case "EXPANDEDR":
		p.state = EXPANDEDR
	case "EXPANDEDUL":
		p.state = EXPANDEDUL
	case "EXPANDEDUR":
		p.state = EXPANDEDUR
	case "EXPANDEDLL":
		p.state = EXPANDEDLL
	case "EXPANDEDLR":
		p.state = EXPANDEDLR
	default:
		return fmt.Errorf("'%s' is not a valid state number", s)
	}

	return nil
}

func (p *Particle) GetIStateN() int {
	switch p.iState {
	case SLEEP:
		return 0
	case AWAKE:
		return 1
	}

	return -1
}

func (p *Particle) GetStateN() int {
	switch p.state {
	case VOID:
		return 0
	case CONTRACTED:
		return 1
	case EXPANDEDL:
		return 2
	case EXPANDEDR:
		return 3
	case EXPANDEDUL:
		return 4
	case EXPANDEDUR:
		return 5
	case EXPANDEDLL:
		return 6
	case EXPANDEDLR:
		return 7
	}

	return -1
}

func (p *Particle) GetStateS() string {
	switch p.state {
	case VOID:
		return "VOID"
	case CONTRACTED:
		return "CONTRACTED"
	case EXPANDEDL:
		return "EXPANDEDL"
	case EXPANDEDR:
		return "EXPANDEDR"
	case EXPANDEDUL:
		return "EXPANDEDUL"
	case EXPANDEDUR:
		return "EXPANDEDUR"
	case EXPANDEDLL:
		return "EXPANDEDLL"
	case EXPANDEDLR:
		return "EXPANDEDLR"
	}

	return "UNKNOWN"
}

func (p *Particle) Round() int {
	return p.round
}

// Awake a particle. Returns true if particle changes the inner state
func (p *Particle) Awake() bool {
	if p.iState == AWAKE {
		return false
	}

	p.iState = AWAKE
	p.round += 1

	return true
}

// Sleep a particle. Returns true if particle changes the inner state
func (p *Particle) Sleep() bool {
	if p.iState == SLEEP {
		return false
	}

	p.iState = SLEEP

	return true
}

type Engine struct {
	initScript      *tengo.Compiled
	schedulerScript *tengo.Script
	particleScript  *tengo.Script
	running         bool
}

func (e *Engine) Init() error {
	if err := e.LoadScripts(); err != nil {
		return err
	}

	return nil
}

func (e *Engine) IsRunning() bool {
	return e.running
}

func (e *Engine) Start() error {
	e.running = true

	return nil
}

func (e *Engine) Stop() error {
	e.running = false

	return nil
}

func (e *Engine) LoadScripts() error {
	// Load tengo modules
	modules := stdlib.GetModuleMap(stdlib.AllModuleNames()...)

	fData, err := ioutil.ReadFile("scripts/init.tengo")
	if err != nil {
		return err
	}

	initScript := tengo.NewScript(fData)

	e.initScript, err = initScript.Compile()
	if err != nil {
		return err
	}

	fData, err = ioutil.ReadFile("scripts/scheduler.tengo")
	if err != nil {
		return err
	}

	e.schedulerScript = tengo.NewScript(fData)
	e.schedulerScript.SetImports(modules) // Add tengo stdlib

	fData, err = ioutil.ReadFile("scripts/particle.tengo")
	if err != nil {
		return err
	}

	e.particleScript = tengo.NewScript(fData)
	e.particleScript.SetImports(modules) // Add tengo stdlib

	// fmt.Println("scripts are loaded")

	return nil
}

func (e *Engine) InitialState() (int, map[string]interface{}, error) {
	if err := e.initScript.Run(); err != nil {
		return -1, nil, err
	}

	initState := e.initScript.Get("init_state")
	hex_size := e.initScript.Get("hex_size")

	return hex_size.Int(), initState.Map(), nil
}

func (e *Engine) Scheduler(particles []interface{}, states []interface{}) ([]interface{}, error) {
	err := e.schedulerScript.Add("particles", particles)
	if err != nil {
		return nil, err
	}

	err = e.schedulerScript.Add("states", states)
	if err != nil {
		return nil, err
	}

	schdulerScriptCompiled, err := e.schedulerScript.Compile()
	if err != nil {
		return nil, err
	}

	err = schdulerScriptCompiled.Run()
	if err != nil {
		return nil, err
	}

	activeParticles := schdulerScriptCompiled.Get("active_particles")

	return activeParticles.Array(), nil
}

func (e *Engine) Particle(p *Particle, neighbors1 []string, neighbors2 []string, neighbors1Deg []int) (string, error) {
	// inputs: state, l, r, ul, ur, ll, lr
	err := e.particleScript.Add("state", p.GetStateS())
	if err != nil {
		return "", err
	}

	for i, s := range []string{"l", "r", "ul", "ur", "ll", "lr"} {
		err := e.particleScript.Add(s, neighbors1[i])
		if err != nil {
			return "", err
		}
	}

	for i, s := range []string{"l2", "r2", "u2l", "u2r", "l2l", "l2r"} {
		err := e.particleScript.Add(s, neighbors2[i])
		if err != nil {
			return "", err
		}
	}

	for i, s := range []string{"dl", "dr", "dul", "dur", "dll", "dlr"} {
		err := e.particleScript.Add(s, neighbors1Deg[i])
		if err != nil {
			return "", err
		}
	}

	particleScriptCompiled, err := e.particleScript.Compile()
	if err != nil {
		return "", err
	}

	err = particleScriptCompiled.Run()
	if err != nil {
		return "", err
	}

	nextState := particleScriptCompiled.Get("next_state")

	return nextState.String(), nil
}

package pkg

import (
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

type Engine struct {
	grid            [][]*Particle
	edges           map[int]map[int]bool
	initScript      *tengo.Compiled
	schedulerScript *tengo.Script
	particleScript  *tengo.Script
	running         bool
}

func (e *Engine) Init(numRows, numCols int) error {
	e.grid = make([][]*Particle, numRows)
	e.edges = make(map[int]map[int]bool)

	for i := range e.grid {
		e.grid[i] = make([]*Particle, numCols)
	}

	for row, columns := range e.grid {
		for column := range columns {
			newParticle := Particle{}
			newParticle.Init()
			e.grid[row][column] = &newParticle
		}
	}

	return nil
}

func (e *Engine) InitGrid(initialState map[string]interface{}) error {
	for key, val := range initialState {
		parts := strings.Split(key, ",")

		x, err := strconv.ParseInt(parts[0], 10, 0)
		if err != nil {
			panic(err)
		}

		y, err := strconv.ParseInt(parts[1], 10, 0)
		if err != nil {
			panic(err)
		}

		err = e.grid[x][y].SetStateN(int(val.(int64)))
		if err != nil {
			panic(err)
		}
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

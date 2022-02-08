package pkg

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

type Phases int

const (
	SCHEDULER Phases = iota
	UPDATE
)

type Engine struct {
	phase           Phases
	schedulerRes    []interface{}
	grid            [][]*Particle
	edges           map[int]map[int]bool
	initScript      *tengo.Compiled
	schedulerScript *tengo.Script
	particleScript  *tengo.Script
	running         bool
}

func (e *Engine) Init(numRows, numCols int) error {
	e.phase = SCHEDULER

	e.schedulerRes = make([]interface{}, 0)
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

func (e *Engine) Update(eTick *chan int) {
	if e.running {
		// fmt.Println("UPDATE ENGINE")

		switch e.phase {
		case SCHEDULER:
			particles := make([]interface{}, 0)
			states := make([]interface{}, 0)

			for row, columns := range e.grid {
				for column, particle := range columns {
					if curState := particle.GetStateN(); curState != 0 {
						particles = append(particles, fmt.Sprintf("%d,%d", row, column))
						states = append(states, particle.GetStateS())
					}
				}
			}

			res, err := e.Scheduler(particles, states)
			if err != nil {
				panic(err)
			}
			// fmt.Printf("Scheduler awakes: %s\n", res)

			for i := range res {
				j := rand.Intn(i + 1)
				res[i], res[j] = res[j], res[i]
			}

			e.schedulerRes = make([]interface{}, len(res))
			copy(e.schedulerRes, res)

			for _, p := range res {
				parsed, ok := p.(string)
				if !ok {
					panic(ok)
				}

				splitted := strings.Split(parsed, ",")

				row, err := strconv.ParseInt(splitted[0], 10, 0)
				if err != nil {
					panic(err)
				}

				column, err := strconv.ParseInt(splitted[1], 10, 0)
				if err != nil {
					panic(err)
				}

				curParticle := e.grid[row][column]
				curParticle.Awake()
			}

			e.phase = UPDATE

		case UPDATE:
			for _, p := range e.schedulerRes {
				parsed, ok := p.(string)
				if !ok {
					panic(ok)
				}

				splitted := strings.Split(parsed, ",")

				row, err := strconv.ParseInt(splitted[0], 10, 0)
				if err != nil {
					panic(err)
				}

				column, err := strconv.ParseInt(splitted[1], 10, 0)
				if err != nil {
					panic(err)
				}

				curParticle := e.grid[row][column]

				if curParticle.GetIStateN() == 1 {
					e.updateNeighbors()

					neighbors1, neighbors2 := curParticle.GetNeighbors()
					neighbors1Deg := e.getN1Degs(int(row), int(column))

					// inputs: state, [l, r, ul, ur, ll, lr], [2l, 2r, u2l, u2r, l2l, l2r]
					nextState, err := e.Particle(curParticle, neighbors1, neighbors2, neighbors1Deg)
					if err != nil {
						panic(err)
					}

					if err := e.grid[row][column].SetStateS(nextState); err != nil {
						panic(err)
					}
					e.grid[row][column].Sleep()

					// fmt.Println(curState, nextState)

					// if nextState == "CONTRACTED" {
					// 	switch curState {
					// 	case 2: // EXPANDEDL
					// 		break
					// 	case 3: // EXPANDEDR
					// 		curP := e.grid[row][column]
					// 		e.grid[row][column] = e.grid[row][column+1]
					// 		e.grid[row][column+1] = curP
					// 	case 4: // EXPANDEDUL
					// 		break
					// 	case 5: // EXPANDEDUR
					// 		break
					// 	case 6: // EXPANDEDLL
					// 		break
					// 	case 7: // EXPANDEDLR
					// 		break
					// 	}
					// }
				}
			}

			e.phase = SCHEDULER
		}
	}

	if eTick != nil {
		*eTick <- e.getRound()
	}
}

func (e *Engine) getN1Degs(row, column int) (neighbors1Deg []int) {
	neighbors1Deg = make([]int, 0)
	// L
	curRow := row
	curCol := column - 1
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	// R
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	// UL
	curRow = row - 1
	curCol = column
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	// UR
	curRow = row - 1
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	// LL
	curRow = row + 1
	curCol = column
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	// LR
	curRow = row + 1
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.grid[curRow][curCol].GetDeg())

	return neighbors1Deg
}

func (e *Engine) updateNeighbors() {
	for row, columns := range e.grid {
		for column, particle := range columns {
			if particle.GetStateN() > 0 {
				neighbors1, neighbors2 := e.getNeighbors(row, column)
				if err := particle.SetNeighbors(neighbors1, neighbors2); err != nil {
					panic(err)
				}

				deg := 0
				if err := particle.SetDeg(deg); err != nil {
					panic(err)
				}

				for _, neighbor := range neighbors1 {
					if neighbor == "CONTRACTED" {
						deg += 1
					}
				}

				if err := particle.SetDeg(deg); err != nil {
					panic(err)
				}
			}
		}
	}
}

// getRound: returns the current simulation round.
// Tip: the minimum of all contracted particle rounds is the current round.
func (e *Engine) getRound() int {
	min := math.MaxInt

	for _, columns := range e.grid {
		for _, particle := range columns {
			if particle.GetStateN() == 1 {
				if round := particle.Round(); round < min {
					min = round
				}
			}
		}
	}

	return min
}

func (e *Engine) getNeighbors(row, column int) (neighbors1 []string, neighbors2 []string) {
	neighbors1 = make([]string, 0)
	neighbors2 = make([]string, 0)

	// L
	curRow := row
	curCol := column - 1
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// R
	curCol = column + 1
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// UL
	curRow = row - 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// UR
	curRow = row - 1
	curCol = column + 1
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// LL
	curRow = row + 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// LR
	curRow = row + 1
	curCol = column + 1
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].GetStateS())

	// 2L
	curRow = row
	curCol = column - 2
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	// 2R
	curCol = column + 2
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	// U2L
	curRow = row - 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	// U2R
	curRow = row - 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	// L2L
	curRow = row + 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	// L2R
	curRow = row + 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].GetStateS())

	return neighbors1, neighbors2
}

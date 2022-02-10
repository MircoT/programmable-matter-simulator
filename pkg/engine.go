package pkg

import (
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"
)

type Phases int

const (
	SCHEDULER Phases = iota
	LOOK
	COMPUTE
	MOVE
)

type Scheduler int

const (
	SYNC Scheduler = iota
	ASYNC
)

type asyncResult struct {
	row, column int
}

type Engine struct {
	phase                  Phases
	schedulerType          Scheduler
	schedulerRes           []interface{}
	grid                   [][]*Particle
	edges                  map[int]map[int]bool
	initScript             *tengo.Compiled
	schedulerScript        *tengo.Script
	particleScript         []*tengo.Script
	particleScriptNames    []string
	particleScriptSelected int
	running                bool
	asyncLoopRunning       bool
	asyncResults           chan asyncResult
	asyncInitPhase         int
	asyncLookPhase         int
	asyncComputePhase      int
	asyncMovePhase         int
	asyncMu                sync.RWMutex
	asyncGridAwoken        [][]bool
}

func (e *Engine) Init(numRows, numCols int) error {
	e.phase = SCHEDULER

	// Trigger the scheduler to read the scheduler type and update it
	if _, err := e.Scheduler([]interface{}{}, []interface{}{}); err != nil {
		return err
	}

	e.schedulerRes = make([]interface{}, 0)
	e.grid = make([][]*Particle, numRows)
	e.asyncGridAwoken = make([][]bool, numRows)
	e.edges = make(map[int]map[int]bool)
	e.asyncResults = make(chan asyncResult, numRows*numCols)
	e.asyncInitPhase = 1000    // max time to wait = 1000 milliseconds
	e.asyncLookPhase = 1000    // max time to wait = 1000 milliseconds
	e.asyncComputePhase = 1000 // max time to wait = 1000 milliseconds
	e.asyncMovePhase = 1000    // max time to wait = 1000 milliseconds

	for i := range e.grid {
		e.grid[i] = make([]*Particle, numCols)
		e.asyncGridAwoken[i] = make([]bool, numCols)
	}

	for row, columns := range e.grid {
		for column := range columns {
			newParticle := Particle{}
			newParticle.Init()
			e.grid[row][column] = &newParticle
		}
	}

	if !e.asyncLoopRunning {
		go e.asyncUpdate()
		e.asyncLoopRunning = true
	}

	return nil
}

func (e *Engine) SetSyncSheduler() {
	e.schedulerType = SYNC
}

func (e *Engine) SetAsyncSheduler() {
	e.schedulerType = ASYNC
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

		e.grid[x][y].iState = SLEEP
		e.grid[x][y].round = 0
	}

	return nil
}

func (e *Engine) IsRunning() bool {
	return e.running
}

func (e *Engine) Start() {
	e.running = true
}

func (e *Engine) Stop() {
	e.running = false
}

func (e *Engine) LoadScripts() error {
	e.particleScript = make([]*tengo.Script, 0)
	e.particleScriptNames = make([]string, 0)

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

	files, err := ioutil.ReadDir("scripts/")
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "particle") && strings.HasSuffix(file.Name(), ".tengo") {
			fData, err := ioutil.ReadFile(path.Join("scripts", file.Name()))
			if err != nil {
				return err
			}

			curScript := tengo.NewScript(fData)
			curScript.SetImports(modules) // Add tengo stdlib

			e.particleScript = append(e.particleScript, curScript)
			e.particleScriptNames = append(e.particleScriptNames, file.Name())
		}
	}

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
	schedulerType := schdulerScriptCompiled.Get("scheduler_type").String()

	switch strings.ToLower(schedulerType) {
	case "async":
		e.SetAsyncSheduler()
	case "sync":
		e.SetSyncSheduler()
	}

	return activeParticles.Array(), nil
}

func (e *Engine) SelectScript(i int) (string, error) {
	if i > len(e.particleScript) || i < 0 {
		return "", fmt.Errorf("No script found at that index %d", i)
	}

	e.particleScriptSelected = ((i - 1) + len(e.particleScript)) % len(e.particleScript)

	return e.particleScriptNames[e.particleScriptSelected], nil
}

func (e *Engine) Particle(p *Particle, neighbors1 []string, neighbors2 []string, neighbors1Deg []int) (string, error) {
	e.asyncMu.Lock()
	defer e.asyncMu.Unlock()

	p.moveFailed = false

	curScript := e.particleScript[e.particleScriptSelected]
	// inputs: state, l, r, ul, ur, ll, lr
	err := curScript.Add("state", p.GetStateS(nil))
	if err != nil {
		return "", err
	}

	for i, s := range []string{"l", "r", "ul", "ur", "ll", "lr"} {
		err := curScript.Add(s, neighbors1[i])
		if err != nil {
			return "", err
		}
	}

	for i, s := range []string{"l2", "r2", "u2l", "u2r", "l2l", "l2r"} {
		err := curScript.Add(s, neighbors2[i])
		if err != nil {
			return "", err
		}
	}

	for i, s := range []string{"dl", "dr", "dul", "dur", "dll", "dlr"} {
		err := curScript.Add(s, neighbors1Deg[i])
		if err != nil {
			return "", err
		}
	}

	particleScriptCompiled, err := curScript.Compile()
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

func (e *Engine) randDuration(max, perc int) time.Duration {
	var val int

	if perc < 0 || perc > 100 {
		randN := rand.Intn(101)
		val = (max * randN) / 100
	} else {
		val = (max * perc) / 100
	}

	return time.Duration(val * int(time.Millisecond))
}

func (e *Engine) asyncTask(row, column int) {
	curParticle := e.grid[row][column]

	fmt.Printf("[%d,%d]->iSTATE:%d\n", row, column, curParticle.iState)

	fmt.Printf("[%d,%d]->INIT\n", row, column)
	time.Sleep(e.randDuration(e.asyncInitPhase, -1))

	e.asyncMu.Lock()
	fmt.Printf("[%d,%d]->AWOKEN: %t\n", row, column, e.asyncGridAwoken[row][column])
	if !e.asyncGridAwoken[row][column] {
		e.asyncGridAwoken[row][column] = true
		e.asyncMu.Unlock()
	} else {
		e.asyncMu.Unlock()

		return
	}

	curParticle.Awake()

	fmt.Printf("[%d,%d]->LOOK\n", row, column)

	e.updateNeighbors(row, column)

	time.Sleep(e.randDuration(e.asyncLookPhase, -1))

	fmt.Printf("[%d,%d]->COMPUTE\n", row, column)

	neighbors1, neighbors2 := curParticle.GetNeighborsString()

	// inputs: state, [l, r, ul, ur, ll, lr], [2l, 2r, u2l, u2r, l2l, l2r], [lDeg, rDeg, ulDeg, urDeg, llDeg, lrDeg]
	nextStateS, err := e.Particle(curParticle, neighbors1, neighbors2, curParticle.n1Deg)
	if err != nil {
		panic(err)
	}

	time.Sleep(e.randDuration(e.asyncComputePhase, -1))

	fmt.Printf("[%d,%d]->MOVE\n", row, column)

	switch nextStateS {
	case "VOID":
		curParticle.state = VOID
	case "CONTRACTED":
		curParticle.state = CONTRACTED
	case "EXPANDL":
		curParticle.state = EXPANDL
	case "EXPANDR":
		curParticle.state = EXPANDR
	case "EXPANDUL":
		curParticle.state = EXPANDUL
	case "EXPANDUR":
		curParticle.state = EXPANDUR
	case "EXPANDLL":
		curParticle.state = EXPANDLL
	case "EXPANDLR":
		curParticle.state = EXPANDLR
	case "MOVEL":
		curParticle.nextState = MOVEL
	case "MOVER":
		curParticle.nextState = MOVER
	case "MOVEUL":
		curParticle.nextState = MOVEUL
	case "MOVEUR":
		curParticle.nextState = MOVEUR
	case "MOVELL":
		curParticle.nextState = MOVELL
	case "MOVELR":
		curParticle.nextState = MOVELR
	default:
		panic(fmt.Errorf("'%s' is not a valid state string", nextStateS))
	}

	time.Sleep(e.randDuration(e.asyncMovePhase, -1))

	e.asyncResults <- asyncResult{row, column}

}

func (e *Engine) asyncUpdate() {
	for {
		select {
		case result := <-e.asyncResults:
			fmt.Printf("SLEEP [%d,%d]\n", result.row, result.column)

			curParticle := e.grid[result.row][result.column]

			if curParticle.state != VOID && curParticle.state != CONTRACTED {
				switch curParticle.nextState {
				case MOVEL:
					newCol := result.column - 1
					fmt.Printf("MOVE LEFT -> %d\n", e.grid[result.row][newCol].state)
					if e.grid[result.row][newCol].state == VOID {
						e.grid[result.row][newCol], e.grid[result.row][result.column] = e.grid[result.row][result.column], e.grid[result.row][newCol]
					} else {
						curParticle.moveFailed = true
					}

					curParticle.state = CONTRACTED
				case MOVER:
					newCol := result.column + 1
					fmt.Printf("MOVE RIGHT -> %d\n", e.grid[result.row][newCol].state)
					if e.grid[result.row][newCol].state == VOID {
						e.grid[result.row][newCol], e.grid[result.row][result.column] = e.grid[result.row][result.column], e.grid[result.row][newCol]
					} else {
						curParticle.moveFailed = true
					}

					curParticle.state = CONTRACTED
				}

				curParticle.nextState = VOID
			}

			curParticle.Sleep()

			e.asyncGridAwoken[result.row][result.column] = false

		default:
		}
	}
}

func (e *Engine) Update(eTick *chan int) {
	for {
		fmt.Println(e.running)
		if e.running {
			fmt.Println("UPDATE ENGINE")
			if e.schedulerType == SYNC {
				switch e.phase {
				case SCHEDULER:
					particles := make([]interface{}, 0)
					states := make([]interface{}, 0)

					for row, columns := range e.grid {
						for column, particle := range columns {
							if particle.state != VOID && particle.state != OBSTACLE {
								particles = append(particles, fmt.Sprintf("%d,%d", row, column))
								states = append(states, particle.GetStateS(nil))
							}
						}
					}

					fmt.Println("SYNC SCHEDULER")
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
						if awoken := curParticle.Awake(); !awoken {
							panic("cannot awake particle...")
						}
					}

					e.phase = LOOK

				case LOOK:
					e.updateNeighbors(-1, -1)

					e.phase = COMPUTE

				case COMPUTE:
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

						if curParticle.iState == AWAKE {
							neighbors1, neighbors2 := curParticle.GetNeighborsString()

							// inputs: state, [l, r, ul, ur, ll, lr], [2l, 2r, u2l, u2r, l2l, l2r], [lDeg, rDeg, ulDeg, urDeg, llDeg, lrDeg]
							nextState, err := e.Particle(curParticle, neighbors1, neighbors2, curParticle.n1Deg)
							if err != nil {
								panic(err)
							}

							switch nextState {
							case "VOID":
								curParticle.nextState = VOID
							case "CONTRACTED":
								curParticle.nextState = CONTRACTED
							case "EXPANDL":
								curParticle.nextState = EXPANDL
							case "EXPANDR":
								curParticle.nextState = EXPANDR
							case "EXPANDUL":
								curParticle.nextState = EXPANDUL
							case "EXPANDUR":
								curParticle.nextState = EXPANDUR
							case "EXPANDLL":
								curParticle.nextState = EXPANDLL
							case "EXPANDLR":
								curParticle.nextState = EXPANDLR
							default:
								panic(fmt.Errorf("'%s' is not a valid state string", nextState))
							}
						}
					}

					e.phase = MOVE

				case MOVE:
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

						if curParticle.iState == AWAKE {
							curParticle.state = curParticle.nextState
							curParticle.nextState = VOID

							curParticle.Sleep()
						}
					}

					e.phase = SCHEDULER
				}
			} else if e.schedulerType == ASYNC {
				particles := make([]interface{}, 0)
				states := make([]interface{}, 0)

				for row, columns := range e.grid {
					for column, particle := range columns {
						if particle.state != VOID && particle.state != OBSTACLE {
							particles = append(particles, fmt.Sprintf("%d,%d", row, column))
							states = append(states, particle.GetStateS(nil))
						}
					}
				}

				fmt.Println("ASYNC SCHEDULER")
				res, err := e.Scheduler(particles, states)
				if err != nil {
					panic(err)
				}
				fmt.Printf("Scheduler awakes: %s\n", res)

				for i := range res {
					j := rand.Intn(i + 1)
					res[i], res[j] = res[j], res[i]
				}

				e.schedulerRes = make([]interface{}, len(res))
				copy(e.schedulerRes, res)

				fmt.Println(e.schedulerRes)

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

					fmt.Printf("LAUNCH [%d,%d]\n", row, column)
					go e.asyncTask(int(row), int(column))
				}
			}
		}

		if eTick != nil {
			*eTick <- e.getRound()
		}
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

func (e *Engine) updateNeighbors(iRow, iCol int) {
	if iRow == -1 && iCol == -1 {
		// Update neighbors
		for row, columns := range e.grid {
			for column, particle := range columns {
				if particle.state != VOID && particle.state != OBSTACLE {
					neighbors1, neighbors2 := e.getNeighbors(row, column)
					if err := particle.SetNeighbors(neighbors1, neighbors2); err != nil {
						panic(err)
					}

					deg := 0
					if err := particle.SetDeg(deg); err != nil {
						panic(err)
					}

					for _, neighbor := range neighbors1 {
						if neighbor == CONTRACTED {
							deg += 1
						}
					}

					if err := particle.SetDeg(deg); err != nil {
						panic(err)
					}
				}
			}
		}
		// Update neighbors' deg
		for row, columns := range e.grid {
			for column, particle := range columns {
				if particle.state != VOID && particle.state != OBSTACLE {
					neighbors1Deg := e.getN1Degs(int(row), int(column))
					if err := particle.SetNeighborsDeg(neighbors1Deg); err != nil {
						panic(err)
					}
				}
			}
		}
	} else {
		e.asyncMu.RLock()
		defer e.asyncMu.RUnlock()

		particle := e.grid[iRow][iCol]
		neighbors1, neighbors2 := e.getNeighbors(iRow, iCol)
		if err := particle.SetNeighbors(neighbors1, neighbors2); err != nil {
			panic(err)
		}

		deg := 0
		if err := particle.SetDeg(deg); err != nil {
			panic(err)
		}

		for _, neighbor := range neighbors1 {
			if neighbor == CONTRACTED {
				deg += 1
			}
		}

		if err := particle.SetDeg(deg); err != nil {
			panic(err)
		}
	}
}

// getRound: returns the current simulation round.
// Tip: the minimum of all particle rounds is the current round.
func (e *Engine) getRound() int {
	e.asyncMu.RLock()
	defer e.asyncMu.RUnlock()

	min := math.MaxInt

	for _, columns := range e.grid {
		for _, particle := range columns {
			if particle.state != VOID && particle.state != OBSTACLE {
				if round := particle.Round(); round < min {
					min = round
				}
			}
		}
	}

	return min
}

func (e *Engine) getNeighbors(row, column int) (neighbors1 []State, neighbors2 []State) {
	e.asyncMu.RLock()
	defer e.asyncMu.RUnlock()

	neighbors1 = make([]State, 0)
	neighbors2 = make([]State, 0)

	// L
	curRow := row
	curCol := column - 1
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// R
	curCol = column + 1
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// UL
	curRow = row - 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// UR
	curRow = row - 1
	curCol = column + 1
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// LL
	curRow = row + 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// LR
	curRow = row + 1
	curCol = column + 1
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.grid[curRow][curCol].state)

	// 2L
	curRow = row
	curCol = column - 2
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	// 2R
	curCol = column + 2
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	// U2L
	curRow = row - 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	// U2R
	curRow = row - 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	// L2L
	curRow = row + 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	// L2R
	curRow = row + 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.grid[curRow][curCol].state)

	return neighbors1, neighbors2
}

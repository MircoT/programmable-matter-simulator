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
	phase                          Phases
	schedulerType                  Scheduler
	schedulerEventDriven           bool
	schedulerEventDrivenWithBlocks bool
	schedulerRes                   []interface{}
	grid                           [][]*Particle
	edges                          map[int]map[int]bool
	initScript                     *tengo.Compiled
	schedulerScript                *tengo.Script
	particleScript                 []*tengo.Script
	particleScriptNames            []string
	particleScriptSelected         int
	running                        bool
	asyncLoopRunning               bool
	asyncResults                   chan asyncResult
	asyncInitPhase                 int
	asyncLookPhase                 int
	asyncComputePhase              int
	asyncMovePhase                 int
	asyncMu                        sync.RWMutex
	asyncGridAwoken                [][]bool
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
		e.asyncResults = make(chan asyncResult, numRows*numCols)
		go e.asyncUpdateController()
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

func (e *Engine) Bootstrap(initialState map[string]interface{}, ppWakeup int, ppLook int, ppCompute int, ppMove int) error {
	e.asyncInitPhase = ppWakeup
	e.asyncLookPhase = ppLook
	e.asyncMovePhase = ppCompute
	e.asyncComputePhase = ppMove

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
		e.grid[x][y].moveFailed = false
		e.grid[x][y].nextState = VOID
		e.grid[x][y].round = 0
	}

	return nil
}

func (e *Engine) IsRunning() bool {
	e.asyncMu.RLock()
	defer e.asyncMu.RUnlock()

	return e.running
}

func (e *Engine) Start() {
	e.asyncMu.Lock()
	defer e.asyncMu.Unlock()

	e.running = true
}

func (e *Engine) Stop() {
	e.asyncMu.Lock()
	defer e.asyncMu.Unlock()

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

func (e *Engine) InitialState() (int, map[string]interface{}, int, int, int, int, error) {
	if err := e.initScript.Run(); err != nil {
		return -1, nil, -1, -1, -1, -1, err
	}

	initState := e.initScript.Get("init_state")
	hex_size := e.initScript.Get("hex_size")
	pp_wakeup := e.initScript.Get("particle_phase_wakeup")
	pp_look := e.initScript.Get("particle_phase_look")
	pp_compute := e.initScript.Get("particle_phase_compute")
	pp_move := e.initScript.Get("particle_phase_move")

	return hex_size.Int(), initState.Map(), pp_wakeup.Int(), pp_look.Int(), pp_compute.Int(), pp_move.Int(), nil
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
	e.schedulerEventDriven = schdulerScriptCompiled.Get("scheduler_event_driven").Bool()
	e.schedulerEventDrivenWithBlocks = schdulerScriptCompiled.Get("scheduler_event_driven_with_blocks").Bool()

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

	e.asyncMu.Unlock()

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

	fmt.Printf("[%d,%d]->AWOKEN: %t\n", row, column, e.asyncGridAwoken[row][column])
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

func (e *Engine) asyncUpdateController() {
	for result := range e.asyncResults {
		fmt.Println("----- GET RESULT -----")
		curParticle := e.grid[result.row][result.column]

		if curParticle.state != VOID && curParticle.state != OBSTACLE {
			newRow := result.row
			newCol := result.column

			switch curParticle.nextState {
			case EXPANDL, MOVEL:
				newCol -= 1
				fmt.Printf("MOVE LEFT -> %d\n", e.grid[newRow][newCol].state)

			case EXPANDR, MOVER:
				newCol += 1
				fmt.Printf("MOVE RIGHT -> %d\n", e.grid[newRow][newCol].state)

			case EXPANDUL, MOVEUL:
				newRow -= 1
				if newRow%2 == 0 {
					newCol -= 1
				}
				fmt.Printf("MOVE UPPER LEFT -> %d\n", e.grid[newRow][newCol].state)

			case EXPANDUR, MOVEUR:
				newRow -= 1
				if newRow%2 != 0 {
					newCol += 1
				}
				fmt.Printf("MOVE UPPER RIGHT -> %d\n", e.grid[newRow][newCol].state)

			case EXPANDLL, MOVELL:
				newRow += 1
				if newRow%2 == 0 {
					newCol -= 1
				}
				fmt.Printf("MOVE LOWER LEFT -> %d\n", e.grid[newRow][newCol].state)

			case EXPANDLR, MOVELR:
				newRow += 1
				if newRow%2 != 0 {
					newCol += 1
				}
				fmt.Printf("MOVE LOWER RIGHT -> %d\n", e.grid[newRow][newCol].state)
			}

			switch curParticle.nextState {
			case MOVEL, MOVER, MOVEUL, MOVEUR, MOVELL, MOVELR:
				if e.grid[newRow][newCol].state == VOID {
					e.grid[newRow][newCol], e.grid[result.row][result.column] = e.grid[result.row][result.column], e.grid[newRow][newCol]
					curParticle.state = CONTRACTED
				} else {
					curParticle.moveFailed = true
				}

				curParticle.nextState = VOID
			case EXPANDL, EXPANDR, EXPANDUL, EXPANDUR, EXPANDLL, EXPANDLR:
				if curParticle.nextState != curParticle.state {
					if e.grid[newRow][newCol].state != VOID && e.grid[newRow][newCol].state != CONTRACTED {
						curParticle.moveFailed = true
						curParticle.state = CONTRACTED
						curParticle.nextState = VOID
					} else {
						curParticle.state = curParticle.nextState
						curParticle.nextState = VOID
					}
				} else {
					curParticle.nextState = VOID
				}
			default:
				curParticle.state = curParticle.nextState
				curParticle.nextState = VOID
			}
		}

		fmt.Printf("SLEEP [%d,%d]\n", result.row, result.column)
		curParticle.Sleep()

		e.asyncGridAwoken[result.row][result.column] = false
	}
	fmt.Println("----- EXITED -----")
}

func (e *Engine) syncUpdate() {
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
				newRow := row
				newCol := column

				switch curParticle.nextState {
				case EXPANDL, MOVEL:
					newCol -= 1
				case EXPANDR, MOVER:
					newCol += 1
				case EXPANDUL, MOVEUL:
					newRow -= 1
					if newRow%2 == 0 {
						newCol -= 1
					}
				case EXPANDUR, MOVEUR:
					newRow -= 1
					if newRow%2 != 0 {
						newCol += 1
					}
				case EXPANDLL, MOVELL:
					newRow += 1
					if newRow%2 == 0 {
						newCol -= 1
					}
				case EXPANDLR, MOVELR:
					newRow += 1
					if newRow%2 != 0 {
						newCol += 1
					}
				}

				fmt.Printf("NEXT STATE: %d\n", curParticle.nextState)

				switch curParticle.nextState {
				case MOVEL, MOVER, MOVEUL, MOVEUR, MOVELL, MOVELR:
					fmt.Printf("MOVE: %d to -> %d\n", curParticle.nextState, e.grid[newRow][newCol].state)
					if e.grid[newRow][newCol].state == VOID {
						e.grid[newRow][newCol], e.grid[row][column] = e.grid[row][column], e.grid[newRow][newCol]
						curParticle.state = CONTRACTED
					} else {
						curParticle.moveFailed = true
					}

					curParticle.nextState = VOID
				case EXPANDL, EXPANDR, EXPANDUL, EXPANDUR, EXPANDLL, EXPANDLR:
					if curParticle.nextState != curParticle.state {
						if e.grid[newRow][newCol].state != VOID && e.grid[newRow][newCol].state != CONTRACTED {
							curParticle.moveFailed = true
							curParticle.state = CONTRACTED
							curParticle.nextState = VOID
						} else {
							curParticle.state = curParticle.nextState
							curParticle.nextState = VOID
						}
					} else {
						curParticle.nextState = VOID
					}
				default:
					curParticle.state = curParticle.nextState
					curParticle.nextState = VOID
				}

				curParticle.Sleep()
			}
		}

		e.phase = SCHEDULER
	}
}

func (e *Engine) asyncUpdate() {
	particles := make([]interface{}, 0)
	states := make([]interface{}, 0)
	eventDrivenParticles := make([]interface{}, 0)

	for row, columns := range e.grid {
		for column, particle := range columns {
			if e.schedulerEventDriven {
				if particle.state == CONTRACTED {
					neighbors1, _ := e.getNeighbors(row, column)

					// fmt.Printf("particle (%d,%d): %#\n", row, column, neighbors1)

					for _, neighbor := range neighbors1 {
						if (neighbor != VOID && neighbor != OBSTACLE) || (e.schedulerEventDrivenWithBlocks && neighbor != VOID) {
							eventDrivenParticles = append(eventDrivenParticles, fmt.Sprintf("%d,%d", row, column))

							break
						}
					}

					// Update deg to calculate isolated particles
					particle.deg = 0
				} else {
					if particle.state != VOID && particle.state != OBSTACLE {
						particles = append(particles, fmt.Sprintf("%d,%d", row, column))
						states = append(states, particle.GetStateS(nil))
					}
				}
			} else {
				if particle.state != VOID && particle.state != OBSTACLE {
					particles = append(particles, fmt.Sprintf("%d,%d", row, column))
					states = append(states, particle.GetStateS(nil))
				}
			}
		}
	}

	fmt.Println("ASYNC SCHEDULER")

	res, err := e.Scheduler(particles, states)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Event driven %#\n", eventDrivenParticles)

	if e.schedulerEventDriven {
		res = append(res, eventDrivenParticles...)
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

		e.asyncMu.Lock()
		if !e.asyncGridAwoken[row][column] {
			e.asyncGridAwoken[row][column] = true
			go e.asyncTask(int(row), int(column))
		}
		e.asyncMu.Unlock()
	}
}

func (e *Engine) Update(eTick *chan int) {
	fmt.Printf("UPDATE ENGINE %t\n", e.running)

	if !e.running {
		return
	}

	switch e.schedulerType {
	case SYNC:
		e.syncUpdate()
	case ASYNC:
		e.asyncUpdate()
	}

	if eTick != nil {
		*eTick <- e.getRound()
	}
}

func (e *Engine) getSafeN1Degs(row, col int) int {
	if row < 1 || col < 1 || row > len(e.grid)-1 || col > len(e.grid[0])-1 {
		return 6
	}

	return e.grid[row][col].GetDeg()
}

func (e *Engine) getN1Degs(row, column int) (neighbors1Deg []int) {
	neighbors1Deg = make([]int, 0)
	// L
	curRow := row
	curCol := column - 1
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

	// R
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

	// UL
	curRow = row - 1
	curCol = column
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

	// UR
	curRow = row - 1
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

	// LL
	curRow = row + 1
	curCol = column
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

	// LR
	curRow = row + 1
	curCol = column + 1
	neighbors1Deg = append(neighbors1Deg, e.getSafeN1Degs(curRow, curCol))

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
						if neighbor != VOID && neighbor != OBSTACLE {
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
			if neighbor != VOID && neighbor != OBSTACLE {
				deg += 1
			}
		}

		if err := particle.SetDeg(deg); err != nil {
			panic(err)
		}

		neighbors1Deg := e.getN1Degs(int(iRow), int(iCol))
		if err := particle.SetNeighborsDeg(neighbors1Deg); err != nil {
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

func (e *Engine) getSafeState(row, col int) State {
	if row < 1 || col < 1 || row > len(e.grid)-1 || col > len(e.grid[0])-1 {
		return OBSTACLE
	}

	return e.grid[row][col].state
}

func (e *Engine) getNeighbors(row, column int) (neighbors1 []State, neighbors2 []State) {
	e.asyncMu.RLock()
	defer e.asyncMu.RUnlock()

	neighbors1 = make([]State, 0)
	neighbors2 = make([]State, 0)

	// L
	curRow := row
	curCol := column - 1
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// R
	curCol = column + 1
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// UL
	curRow = row - 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// UR
	curRow = row - 1
	curCol = column
	if curRow%2 != 0 {
		curCol += 1
	}
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// LL
	curRow = row + 1
	curCol = column
	if curRow%2 == 0 {
		curCol -= 1
	}
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// LR
	curRow = row + 1
	curCol = column
	if curRow%2 != 0 {
		curCol += 1
	}
	neighbors1 = append(neighbors1, e.getSafeState(curRow, curCol))

	// 2L
	curRow = row
	curCol = column - 2
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	// 2R
	curCol = column + 2
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	// U2L
	curRow = row - 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	// U2R
	curRow = row - 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	// L2L
	curRow = row + 2
	curCol = column - 1
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	// L2R
	curRow = row + 2
	curCol = column + 1
	neighbors2 = append(neighbors2, e.getSafeState(curRow, curCol))

	return neighbors1, neighbors2
}

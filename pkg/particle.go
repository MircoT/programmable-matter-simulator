package pkg

import "fmt"

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
	MOVEDL     // LEFT
	MOVEDR     // RIGHT
	MOVEDUL    // UPPER LEFT
	MOVEDUR    // UPPER RIGHT
	MOVEDLL    // LOWER LEFT
	MOVEDLR    // LOWER RIGHT
)

const (
	SLEEP InnerState = iota
	AWAKE
)

type Particle struct {
	state     State
	nextState State
	iState    InnerState
	round     int // the minimum of all contracted particle rounds is the current round
	deg       int
	n1        []State
	n2        []State
	n1Deg     []int
}

func (p *Particle) Init() *Particle {
	p.n1 = make([]State, 6)
	p.n2 = make([]State, 6)
	p.n1Deg = make([]int, 6)

	return p
}

func (p *Particle) SetNeighbors(n1, n2 []State) error {
	n := copy(p.n1, n1)
	n += copy(p.n2, n2)

	if n != 12 {
		return fmt.Errorf("error on copy neighbors")
	}

	return nil
}

func (p *Particle) SetNeighborsDeg(n1Deg []int) error {
	if n := copy(p.n1Deg, n1Deg); n != 6 {
		return fmt.Errorf("error on copy neighbors deg")
	}

	return nil
}

func (p *Particle) GetNeighborsString() ([]string, []string) {
	n1 := make([]string, 6)
	n2 := make([]string, 6)

	for i, n := range p.n1 {
		pState := n
		curState := p.GetStateS(&pState)
		n1[i] = curState
	}

	for i, n := range p.n2 {
		pState := n
		curState := p.GetStateS(&pState)
		n2[i] = curState
	}

	return n1, n2
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

func (p *Particle) GetStateS(state *State) string {
	var curState State
	if state == nil {
		curState = p.state
	} else {
		curState = *state
	}
	switch curState {
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

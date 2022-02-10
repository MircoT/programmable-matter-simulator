package pkg

import "fmt"

type State int
type InnerState int

const (
	VOID State = iota
	CONTRACTED
	EXPANDL  // LEFT
	EXPANDR  // RIGHT
	EXPANDUL // UPPER LEFT
	EXPANDUR // UPPER RIGHT
	EXPANDLL // LOWER LEFT
	EXPANDLR // LOWER RIGHT
	MOVEL    // LEFT
	MOVER    // RIGHT
	MOVEUL   // UPPER LEFT
	MOVEUR   // UPPER RIGHT
	MOVELL   // LOWER LEFT
	MOVELR   // LOWER RIGHT
	OBSTACLE
)

const (
	SLEEP InnerState = iota
	AWAKE
)

type Particle struct {
	state      State
	nextState  State
	iState     InnerState
	round      int // the minimum of all contracted particle rounds is the current round
	deg        int
	n1         []State
	n2         []State
	n1Deg      []int
	moveFailed bool
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
		p.state = EXPANDL
	case 3:
		p.state = EXPANDR
	case 4:
		p.state = EXPANDUL
	case 5:
		p.state = EXPANDUR
	case 6:
		p.state = EXPANDLL
	case 7:
		p.state = EXPANDLR
	case 8:
		p.state = MOVEL
	case 9:
		p.state = MOVER
	case 10:
		p.state = MOVEUL
	case 11:
		p.state = MOVEUR
	case 12:
		p.state = MOVELL
	case 13:
		p.state = MOVELR
	case 14:
		p.state = OBSTACLE
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
	case EXPANDL:
		return 2
	case EXPANDR:
		return 3
	case EXPANDUL:
		return 4
	case EXPANDUR:
		return 5
	case EXPANDLL:
		return 6
	case EXPANDLR:
		return 7
	case MOVEL:
		return 8
	case MOVER:
		return 9
	case MOVEUL:
		return 10
	case MOVEUR:
		return 11
	case MOVELL:
		return 12
	case MOVELR:
		return 13
	case OBSTACLE:
		return 14
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
	case "EXPANDL":
		p.state = EXPANDL
	case "EXPANDR":
		p.state = EXPANDR
	case "EXPANDUL":
		p.state = EXPANDUL
	case "EXPANDUR":
		p.state = EXPANDUR
	case "EXPANDLL":
		p.state = EXPANDLL
	case "EXPANDLR":
		p.state = EXPANDLR
	case "MOVEL":
		p.state = MOVEL
	case "MOVER":
		p.state = MOVER
	case "MOVEUL":
		p.state = MOVEUL
	case "MOVEUR":
		p.state = MOVEUR
	case "MOVELL":
		p.state = MOVELL
	case "MOVELR":
		p.state = MOVELR
	case "OBSTACLE":
		p.state = OBSTACLE
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
	case EXPANDL:
		return "EXPANDL"
	case EXPANDR:
		return "EXPANDR"
	case EXPANDUL:
		return "EXPANDUL"
	case EXPANDUR:
		return "EXPANDUR"
	case EXPANDLL:
		return "EXPANDLL"
	case EXPANDLR:
		return "EXPANDLR"
	case MOVEL:
		return "MOVEL"
	case MOVER:
		return "MOVER"
	case MOVEUL:
		return "MOVEUL"
	case MOVEUR:
		return "MOVEUR"
	case MOVELL:
		return "MOVELL"
	case MOVELR:
		return "MOVELR"
	case OBSTACLE:
		return "OBSTACLE"
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

package tictactoe

import (
	"github.com/sa-ha/quint-mbt-go/itf"
	"github.com/sa-ha/quint-mbt-go/mbt"
)

// Adapter mirrors TicTacToeDriver and GameState in quint-connect's
// connect/examples/tictactoe/mbt.rs.
type Adapter struct{ game Game }

// NewAdapter is the factory to pass to mbt.Run or mbt.ReplayFiles.
func NewAdapter() mbt.Adapter { return &Adapter{} }

// Actions maps the actions of spec/tictactoe.qnt. MoveX reads whichever of
// the corner and coordinate picks the step made, and moves to the center when
// it made neither (the TakeCenter branch has no nondet).
func (a *Adapter) Actions() map[string]mbt.Action {
	return map[string]mbt.Action{
		"init": func(mbt.Picks) error { a.game = Game{}; return nil },
		"MoveX": func(p mbt.Picks) error {
			v, ok := p.Get("corner")
			if !ok {
				v, ok = p.Get("coordinate")
			}
			if !ok {
				a.game.MoveTo(Position{1, 1}, X)
				return nil
			}
			pos, err := toPosition(v)
			if err == nil {
				a.game.MoveTo(pos, X)
			}
			return err
		},
		"MoveO": func(p mbt.Picks) error {
			v, err := p.Value("coordinate")
			if err != nil {
				return err
			}
			pos, err := toPosition(v)
			if err == nil {
				a.game.MoveTo(pos, O)
			}
			return err
		},
		"stuttered": func(mbt.Picks) error { return nil },
	}
}

func toPosition(v itf.Value) (Position, error) {
	elems, err := v.Elems()
	if err != nil {
		return Position{}, err
	}
	var xy [2]int
	for n := range xy {
		if n < len(elems) {
			xy[n], err = elems[n].AsInt()
		}
		if err != nil {
			return Position{}, err
		}
	}
	return Position{xy[0] - 1, xy[1] - 1}, nil
}

func player(p Player) itf.Value {
	if p == X {
		return itf.Variant("X", itf.Tuple())
	}
	return itf.Variant("O", itf.Tuple())
}

// State projects the game onto the spec's board and nextTurn. The nesting
// follows GameState::from_driver: board[y][x] holds Board[x-1][y-1].
func (a *Adapter) State() (map[string]any, error) {
	board := map[int]map[int]itf.Value{}
	for i, col := range a.game.Board {
		for j, cell := range col {
			x, y := i+1, j+1
			if board[y] == nil {
				board[y] = map[int]itf.Value{}
			}
			sq := itf.Variant("Empty", itf.Tuple())
			if cell.Occupied {
				sq = itf.Variant("Occupied", player(cell.Player))
			}
			board[y][x] = sq
		}
	}
	return map[string]any{"board": board, "nextTurn": player(a.game.NextTurn)}, nil
}

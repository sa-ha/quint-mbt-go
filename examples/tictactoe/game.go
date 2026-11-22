// Package tictactoe is a Go port of the implementation in quint-connect's
// tictactoe example (connect/examples/tictactoe/game.rs at v0.1.2), kept
// behaviorally identical so that both drivers can be compared on the same
// traces.
package tictactoe

// Player is X or O. The zero value is X, who moves first.
type Player int

// The two players.
const (
	X Player = iota
	O
)

// Position is a zero-based board coordinate.
type Position struct{ X, Y int }

// Cell is one board square.
type Cell struct {
	Occupied bool
	Player   Player
}

// Game is the board, indexed as Board[y][x], and whose turn it is.
type Game struct {
	Board    [3][3]Cell
	NextTurn Player
}

// MoveTo places player at pos. Like the Rust original, it panics when the
// player moves out of turn or onto an occupied cell.
func (g *Game) MoveTo(pos Position, player Player) {
	if g.NextTurn != player {
		panic("player out of turn")
	}
	cell := &g.Board[pos.Y][pos.X]
	if cell.Occupied {
		panic("moving to occupied cell")
	}
	*cell = Cell{Occupied: true, Player: player}
	if player == X {
		g.NextTurn = O
	} else {
		g.NextTurn = X
	}
}

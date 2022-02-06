package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/mircot/programmable-matter-simulator/pkg"
)

func main() {
	r := &pkg.Renderer{}
	if err := r.Init(); err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowSize(pkg.ScreenWidth, pkg.ScreenHeight)
	ebiten.SetWindowTitle("Programmable Matter Simulator - Demo")

	if err := ebiten.RunGame(r); err != nil {
		log.Fatal(err)
	}
}

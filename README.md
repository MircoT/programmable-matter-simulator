# programmable-matter-simulator

A programmable matter simulator written using [Go](https://go.dev/) and [Ebiten](https://ebiten.org/) scriptable using [Tengo language](https://github.com/d5/tengo).

<p align="center">
    <img src="https://github.com/MircoT/programmable-matter-simulator/raw/main/screenshot0.png" width="auto" height="320" />
    <img src="https://github.com/MircoT/programmable-matter-simulator/raw/main/screenshot1.png" width="auto" height="320" />
</p>

## :rocket: Quick start

Simply run the main script:

```bash
go run main.go
```

Edit the scripts in `scripts` folder to modify the behavior of the simulation.

### :question: Help

If you press `H` you will see the following help menu:

<p align="center">
    <img src="https://github.com/MircoT/programmable-matter-simulator/raw/main/help_menu.png" width="auto" height="320" />
</p>

As you can see, you can select different script for the particles and start, pause
or restart the simulation. Of course, if you edit the scripts, you have to reload
the environment pressing `R`.

## :warning: Known bugs :warning:

- A particle that reach the border will crash the engine (not supported at the moment, maybe a toroidal map could help)

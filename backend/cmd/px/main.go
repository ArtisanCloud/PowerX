package main

import "github.com/ArtisanCloud/PowerX/cmd/px/commands"

var version = "dev"

func main() {
	commands.Execute(version)
}

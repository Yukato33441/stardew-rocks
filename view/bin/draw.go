package main

import (
	"log"
	"os"

	"github.com/nictuku/stardew-rocks/parser"
	"github.com/nictuku/stardew-rocks/view"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Expected `%v <xml save file> <output.png>`", os.Args[0])
	}
	farm := parser.LoadFarmMap()

	log.Printf("Processing %v", os.Args[1])
	sg, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile(os.Args[2], os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}

	gameSave, err := parser.ParseSaveGame(sg)
	if err != nil {
		log.Fatal(err)
	}
	view.WriteImage(farm, gameSave, f)
}

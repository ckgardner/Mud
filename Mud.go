package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

var allCommands = make(map[string]func(string))

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	socialCommands()
	directionalCommands()
	if err := commandLoop(); err != nil {
		log.Fatalf("%v", err)
	}

}

func commandLoop() error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		doCommand(line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("in main command loop: %v", err)
	}

	return nil
}

func addCommand(cmd string, f func(string)) {
	for i := range cmd {
		if i == 0 {
			continue
		}
		prefix := cmd[:i]
		allCommands[prefix] = f
	}
	allCommands[cmd] = f
}

func socialCommands() {
	addCommand("smile", cmdSmile)
	addCommand("laugh", cmdLaugh)
}

func directionalCommands(){
	addCommand("look", cmdLook)
	addCommand("north", cmdNorth)
	addCommand("south", cmdSouth)
	addCommand("east", cmdEast)
	addCommand("west", cmdWest)
}

func doCommand(cmd string) error {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return nil
	}

	if f, exists := allCommands[strings.ToLower(words[0])]; exists {
		f(cmd)
	} else {
		fmt.Printf("Huh?\n")
	}
	return nil
}

//Social Commands
func cmdSmile(s string) {
	fmt.Printf("You smile happily.\n")
}
func cmdLaugh(s string) {
	fmt.Printf("You laugh out loud.\n")
}

//Directional Commands
func cmdLook(s string) {
	fmt.Printf("You look around.\n")
}
func cmdNorth(s string) {
	fmt.Printf("You move North.\n")
}
func cmdSouth(s string) {
	fmt.Printf("You move South.\n")
}
func cmdEast(s string) {
	fmt.Printf("You move East.\n")
}
func cmdWest(s string) {
	fmt.Printf("You move West.\n")
}
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"context"
)

var allCommands = make(map[string]func(string))

func main() {
	//create the database function
	db, err := sql.Open("sqlite3", "./world.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	//end of database creation
	//open database to do something or handle error
	var allZones map[int]*Zone
	tx, err := db.Begin()
	handleError(err)
	allZones = readAllZones(tx)
	if err != nil {
		tx.Rollback()
		return
	}
	err = tx.Commit()
	//readAllRooms(tx)
	tx2, err := db.Begin()
	handleError(err)
	allRooms := readAllRooms(tx2, allZones)
	if err != nil {
		tx.Rollback()
		return
	}
	err = tx2.Commit()

	tx3, err := db.Begin()
	handleError(err)
	allRooms = readAllExits(tx3, allRooms)
	if err != nil {
		tx.Rollback()
		return
	}
	err = tx3.Commit()
	for i := range allRooms[32].Exits{
		fmt.Printf(allRooms[32].Exits[i].Description)
	}
	directionsLookUp()

	//run commands
	socialCommands()
	directionalCommands()
	if err := commandLoop(); err != nil {
		log.Fatalf("%v", err)
	}
}

func handleError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
//scans through all commands and says what to do with it 
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
//adds a new command
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
	addCommand("look", cmdLook)
	addCommand("recall", cmdRecall)
}

func directionalCommands(){
	addCommand("north", cmdNorth)
	addCommand("south", cmdSouth)
	addCommand("east", cmdEast)
	addCommand("west", cmdWest)
	addCommand("up", cmdUp)
	addCommand("down", cmdDown)
}
//actually performs the command from the command loop
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
func cmdLook(s string) {
	fmt.Printf("You look around.\n")
}
func cmdRecall(s string) {
	fmt.Printf("You have been taken back to the start.\n")
}

//Directional Commands
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
func cmdUp(s string) {
	fmt.Printf("You move Up.\n")
}
func cmdDown(s string) {
	fmt.Printf("You move Down.\n")
}

//Directions Look-up
func directionsLookUp()(directions map[string]string){
	dir := map[string]string{
		"0": "north",
		"1": "east",
		"2": "west",
		"3": "south",
		"4": "up",
		"5": "down",
		"north": "0",
		"east": "1",
		"west": "2",
		"south": "3",
		"up": "4",
		"down": "5",
	}
	return dir
}

// Zone: This is a Zone//
type Zone struct {
    ID    int 
    Name  string
    Rooms []*Room
}
//Room: This is a room
type Room struct {
    ID          int 
    Zone        *Zone
    Name        string
    Description string
    Exits       [6]Exit
}
//Exit: This is an exit
type Exit struct {
    To          *Room
    Description string
}
//Player: player object
type Player struct {
	location 	*Room
}

var (
	ctx context.Context
	db  *sql.DB
)

func readRoom(){
	rows, err := db.QueryContext(ctx, "select id, zone_id, name, description from rooms where id = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		
		var id int
		var zoneID int
		var name string
		var description string
		err = rows.Scan(&id, &zoneID, &name, &description)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(id, zoneID, name, description)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
}

func readAllZones( tx *sql.Tx) (map[int]*Zone){
	allZones := make(map[int]*Zone)
	rows, err := tx.Query("select id, name FROM zones ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		
		var id int
		var name string

		err = rows.Scan(&id, &name)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(name)
		rooms := make([]*Room, 0)
		var zone *Zone
		zone = new(Zone)
		zone.ID = id
		zone.Name = name
		zone.Rooms = rooms
		allZones[id] = zone
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	return allZones
}

func readAllRooms(tx *sql.Tx, zones map[int]*Zone) []*Room{
	var rooms []*Room
	rows, err := tx.Query("select id, zone_id, name, description FROM rooms ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		
		var id int
		var zone_id int
		var name string
		var description string

		err = rows.Scan(&id, &zone_id, &name, &description)
		if err != nil {
			log.Fatal(err)
		}
		z := zones[zone_id]
		fmt.Println(name)
		var exits [6]Exit
		room := Room{id, z, name, description, exits}
		rooms = append(rooms, &room)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	return rooms
}

func readAllExits(tx *sql.Tx, rooms []*Room ) []*Room{
	rows, err := tx.Query("select from_room_id, to_room_id, direction, description FROM exits ORDER BY from_room_id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	dir := [6]string{"n", "e", "w", "s", "u", "d"}
	for rows.Next() {
		
		var from_room_id, to_room_id int
		var direction, description string

		err = rows.Scan(&from_room_id, &to_room_id, &direction, &description)
		if err != nil {
			log.Fatal(err)
		}
		exit := Exit{nil, description}
		for i := range rooms{
			if rooms[i].ID == to_room_id {
				exit.To = rooms[i]
				break
			}
		}
		var directionIndex int
		for i := range dir{
			if dir[i] == direction{
				directionIndex = i
			}
		}
		for i := range rooms{
			if rooms[i].ID == from_room_id {
				rooms[i].Exits[directionIndex] = exit
				break
			}
		}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	}
	return rooms
}
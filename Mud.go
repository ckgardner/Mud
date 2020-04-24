package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"net"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/pbkdf2"

	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// Zone is a Zone//
type Zone struct {
	ID    int
	Name  string
	Rooms []*Room
}

//Room is a room
type Room struct {
	ID          int
	Zone        *Zone
	Name        string
	Description string
	Exits       [6]Exit
}

//Exit is an exit
type Exit struct {
	To          *Room
	Description string
}

//Player is a player object
type Player struct {
	Name		string
	Location 	*Room
	Home		*Room
	Outputs		chan OutputEvent
}
// InputEvent struct
type InputEvent struct {
	Player  *Player
	Command []string
	Close   bool
	Login   bool
}
// OutputEvent struct
type OutputEvent struct {
	Text  string
	Close bool
}

var allCommands = make(map[string]func(*Player, []string))
var db  *sql.DB
var players = make(map[string]*Player)
var dirLookup = make(map[string]int)
var dirLookupInt = make(map[int]string)

func checkForUser(tx *sql.Tx, username string) (bool, error) {
	var name string
	dbuser := tx.QueryRow("SELECT name FROM players WHERE name=?", username)
	switch err := dbuser.Scan(&name); err {
	case sql.ErrNoRows:
		fmt.Printf("User %v does not exist\n", username)
		return false, tx.Commit()
	case nil:
		fmt.Printf("User %v found!\n", username)
		return true, tx.Commit()
	default:
		fmt.Printf("Error checking for user.\n")
		tx.Rollback()
		return false, err
	}
}

func loginUser(tx *sql.Tx, password string, username string) (bool, error) {
	dbplayer, err := tx.Query("SELECT salt,hash FROM players WHERE name=?", username)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	defer dbplayer.Close()
	var salt64, hash64 string
	for dbplayer.Next() {

		if err := dbplayer.Scan(&salt64, &hash64); err != nil {
			tx.Rollback()
			return false, err
		}
	}
	salt, err := base64.StdEncoding.DecodeString(salt64)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	hash, err := base64.StdEncoding.DecodeString(hash64)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	inputhash := pbkdf2.Key(
		[]byte(password),
		salt,
		64*1024,
		32,
		sha256.New)
	if subtle.ConstantTimeCompare(hash, inputhash) != 1 {
		return false, tx.Commit()
	}
	return true, tx.Commit()
}

func createUser(tx *sql.Tx, password string, username string) error {
	salt := make([]byte, 32)
	_, err := crand.Read(salt)
	if err != nil {
		log.Print("Error creating salt: ", err)
	}
	salt64 := base64.StdEncoding.EncodeToString(salt)
	hash := pbkdf2.Key(
		[]byte(password),
		salt,
		64*1024,
		32,
		sha256.New)
	hash64 := base64.StdEncoding.EncodeToString(hash)

	tx.Exec("INSERT INTO players (name,salt,hash) VALUES (?,?,?)", username, salt64, hash64)
	return tx.Commit()
}

func getRoom(tx *sql.Tx, rooms []*Room, ID int) (*Room, error) {
	dbroom, err := tx.Query("SELECT id, zone_id, name, description FROM rooms WHERE id=?", ID)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	defer dbroom.Close()
	var room *Room
	for dbroom.Next() {
		var id, zoneID int
		var name, description string
		if err := dbroom.Scan(&id, &zoneID, &name, &description); err != nil {
			tx.Rollback()
			return nil, err
		}
		for i := range rooms {
			if rooms[i].ID == id {
				room = rooms[i]
			}
		}
	}
	if err := dbroom.Err(); err != nil {
		tx.Rollback()
		return nil, err
	}
	return room, tx.Commit()
}

func readAllZones(zones map[int]Zone, tx1 *sql.Tx) (map[int]Zone, error) {
	dbzones, err := tx1.Query("SELECT id, name FROM zones ORDER BY id")
	if err != nil {
		tx1.Rollback()
		return nil, fmt.Errorf("reading a room from the database: %v", err)
	}

	defer dbzones.Close()
	for dbzones.Next() {
		var id int
		var name string
		if err := dbzones.Scan(&id, &name); err != nil {
			tx1.Rollback()
			return nil, err
		}
		rooms := make([]*Room, 0)
		z := Zone{id, name, rooms}
		zones[id] = z
	}
	if err := dbzones.Err(); err != nil {
		tx1.Rollback()
		return nil, err
	}
	return zones, tx1.Commit()
}

func readAllRooms(tx *sql.Tx, zones map[int]Zone) ([]*Room, error) {
	dbrooms, err := tx.Query("SELECT id, zone_id, name, description FROM rooms ORDER BY id")
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	defer dbrooms.Close()
	var rooms []*Room
	for dbrooms.Next() {
		var id, zoneID int
		var name, description string
		if err := dbrooms.Scan(&id, &zoneID, &name, &description); err != nil {
			tx.Rollback()
			return nil, err
		}
		z := zones[zoneID]
		zone := &z
		var exits [6]Exit
		room := Room{id, zone, name, description, exits}
		rooms = append(rooms, &room)
	}
	if err := dbrooms.Err(); err != nil {
		tx.Rollback()
		return nil, err
	}
	return rooms, tx.Commit()
}

func readAllExits(tx *sql.Tx, rooms []*Room) ([]*Room, error) {
	dbexits, err := tx.Query("SELECT from_room_id, to_room_id, direction, description FROM exits ORDER BY from_room_id")
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	defer dbexits.Close()
	dirs := [6]string{"n", "e", "w", "s", "u", "d"}
	for dbexits.Next() {
		var fromRoomID, toRoomID int
		var direction, description string
		if err := dbexits.Scan(&fromRoomID, &toRoomID, &direction, &description); err != nil {
			tx.Rollback()
			return nil, err
		}
		exit := Exit{nil, description}
		for i := range rooms {
			if rooms[i].ID == toRoomID {
				exit.To = rooms[i]
			}
		}
		var dirIndex int
		for i := range dirs {
			if dirs[i] == direction {
				dirIndex = i
			}
		}
		for i := range rooms {
			if rooms[i].ID == fromRoomID {
				rooms[i].Exits[dirIndex] = exit
			}
		}

	}
	if err := dbexits.Err(); err != nil {
		tx.Rollback()
		return nil, err
	}
	return rooms, tx.Commit()
}

func main() {
	//create the database function
	path := "world.db"
	options :=
    	"?" + "_busy_timeout=10000" +
			"&" + "_case_sensitive_like=OFF" +
			"&" + "_foreign_keys=ON" +
			"&" + "_journal_mode=WAL" +
			"&" + "_locking_mode=NORMAL" +
			"&" + "mode=rw" +
			"&" + "_synchronous=NORMAL"
	db, err := sql.Open("sqlite3", path+options)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	//end of database creation
	//open database to do something or handle error
	fmt.Println("Mud is running")
	tx1, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	//Read in the zones
	var zones = make(map[int]Zone)
	zones, err = readAllZones(zones, tx1)
	if err != nil {
		tx1.Rollback()
		log.Fatal(err)
	}

	//readAllRooms(tx)
	tx2, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	//Read in the rooms
	rooms := make([]*Room, 0)
	rooms, err = readAllRooms(tx2, zones)
	if err != nil {
		tx2.Rollback()
		log.Fatal(err)
	}

	tx3, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	//Read in all the exits
	rooms, err = readAllExits(tx3, rooms)
	if err != nil {
		tx3.Rollback()
		fmt.Printf("Error reading in exits")
		log.Fatal(err)
	}

	tx4, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	startRoom, err := getRoom(tx4, rooms, 3001)
	if err != nil {
		tx4.Rollback()
		log.Fatalf("TX 4 Error: %v", err)
	}
	fmt.Println("All transactions processed")
	directionsLookUp()

	//run commands
	socialCommands()
	directionalCommands()

	// Go Routine for connections
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Listen error: %v", err)
	}

	inputs := make(chan InputEvent)

	go func() {
		log.Println("Go routine 1 running")
		for {
			conn, err := ln.Accept()
			log.Println("accepting connection...")
			if err != nil {
				log.Fatalf("Accept error: %v", err)
				continue
			}
			log.Printf("Incoming connection from %s", conn.RemoteAddr())
			fmt.Fprintf(conn, "Welcome to MUD!\n")
			scanner1 := bufio.NewScanner(conn)
			fmt.Fprint(conn, "Username: ")
			scanner1.Scan()
			text := scanner1.Text()
			username := text
			
			// Check if user exists
			tx, err := db.Begin()
			if err != nil{
				log.Fatalf("Error beginning trnx: %v", err)
			}
			userexists, err := checkForUser(tx, username)
			if err != nil {
				tx.Rollback()
				log.Printf("error checking if user exists: %v", err)
			}
			if userexists{
				for{
					tx2, err := db.Begin()
					if err != nil{
						log.Fatalf("error in main tx #2: %v", err)
					}
					fmt.Fprintf(conn, "Password: ")
					scanner1.Scan()
					text = scanner1.Text()
					password := text
					correctPW, err := loginUser(tx2, password, username)
					if err != nil{
						log.Fatalf("Error logging in the user: %v", err)
					}
					if correctPW == false{
						fmt.Fprint(conn, "Password Incorrect, try again.\n")
					}else{
						break
					}
				}
			}else{
				tx3, err := db.Begin()
				if err != nil{
					log.Fatalf("error in 3rd TX: %v", err)
				}
				fmt.Fprintf(conn, "Creating new user: %s\n", username)
				fmt.Fprintf(conn, "Password: ")
				scanner1.Scan()
				text = scanner1.Text()
				password := text
				err2 := createUser(tx3, password, username)
				if err2 != nil{
					log.Fatalf("Error creating user: %v", err)
				}
			}

			outputs := make(chan OutputEvent)
			player := &Player{username, startRoom, startRoom, outputs}
			input := InputEvent{player, nil, false, true}
			inputs <- input
			go handlePlayerConnection(db, conn, inputs, player)
		}
		// ...
	}()

	for{
		input := <- inputs
		if input.Login == true{
			if player, ok := players[input.Player.Name]; ok{
				if player.Outputs != nil{
					close(player.Outputs)
					player.Outputs = nil
				}
			}
			players[input.Player.Name] = input.Player
		}else{
			if input.Close == true{
				close(input.Player.Outputs)
				input.Player.Outputs = nil
				delete(players, input.Player.Name)
			}else{
				if input.Player != nil{
					doCommand(input.Player, input.Command)
				}
			}
		}
	}
}

//scans through all commands and says what to do with it
func doCommand(player *Player, commands []string) {
	command := commands[0]
	value, ok := allCommands[command]
	if ok {
		player.Printf("\n")
		value(player, commands[1:])
	} else {
		player.Printf("[%s] Command not recognized\n", command)
	}
}
func addCommand(command string, action func(*Player, []string)) {
	allCommands[command] = action
}

func socialCommands() {
	addCommand("smile", cmdSmile)
	addCommand("laugh", cmdLaugh)
	addCommand("look", cmdLook)
	addCommand("gossip", cmdGossip)
	addCommand("say", cmdSay)
	addCommand("tell", cmdTell)
	addCommand("shout", cmdShout)
}
func directionalCommands() {
	addCommand("look north", cmdLookNorth)
	addCommand("look south", cmdLookSouth)
	addCommand("look east", cmdLookEast)
	addCommand("look west", cmdLookEast)
	addCommand("look up", cmdLookUp)
	addCommand("look down", cmdLookDown)
	addCommand("north", cmdNorth)
	addCommand("south", cmdSouth)
	addCommand("east", cmdEast)
	addCommand("west", cmdWest)
	addCommand("up", cmdUp)
	addCommand("down", cmdDown)
	addCommand("n", cmdNorth)
	addCommand("s", cmdSouth)
	addCommand("e", cmdEast)
	addCommand("w", cmdWest)
	addCommand("u", cmdUp)
	addCommand("d", cmdDown)
	addCommand("recall", cmdRecall)
}

//Social Commands
func cmdSmile(player *Player, cmd []string) {
	for _, otherplayer := range players{
		if otherplayer.Location == player.Location{
			otherplayer.Printf("%v: [Smiles] \n", player.Name)
		}
	}
	fmt.Printf("You smile to the entire room\n")
}
func cmdLaugh(player *Player, cmd []string) {
	for _, otherplayer := range players{
		if otherplayer.Location.Zone.ID == player.Location.Zone.ID{
			otherplayer.Printf("%v [LOL] at %v \n", player.Name);
		}
	}
	fmt.Printf("You laugh out loud.\n")
}
func cmdLook(player *Player, cmd []string) {
	room := player.Location
	if len(cmd) == 0{
		player.Printf("Current room: %s\n\n", room.Name)
		player.Printf("Room Description: %s", room.Description)
		player.Printf("\nPossible Exits:\n")
		for i := 0; i <= 5; i++{
			if room.Exits[i].Description != ""{
				player.Printf(" %s", dirLookupInt[i])
				player.Printf("\n")
			}
		}
		player.Printf("Players in room:\n")
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location{
				player.Printf(" %v\n", otherplayer.Name)
			}
		}
		player.Printf("\n")
	}else{
		dir := cmd[0]
		dirN := dirLookup[dir]
		if room.Exits[dirN].Description != ""{
			player.Printf("%s\n", room.Exits[dirN].Description)
		}else{
			player.Printf("There is no path that way.\n")
		}
	}
}
func cmdGossip(player *Player, cmd []string){
	if len(cmd) == 0{
		player.Printf("You have to type a message to send to the player\n")
	}
	message := strings.Join(cmd, " ")
	for _, otherplayer := range players {
		otherplayer.Printf("%v:[Gossip] %v \n", player.Name, message)
	}
}
func cmdSay(player *Player, cmd []string){
	if len(cmd) == 0{
		player.Printf("You must type a message to say to the room\n")
	}
	message := strings.Join(cmd, " ")
	for _, otherplayer := range players{
		if otherplayer.Location == player.Location{
			otherplayer.Printf("%v:[Say] %v \n", player.Name, message)
		}
	}
}
func cmdTell(player *Player, cmd []string){
	if len(cmd) < 2{
		player.Printf("You must type a name and a player to use tell command")
	}else{
		if otherplayer, ok := players[cmd[0]];ok{
			message := strings.Join(cmd[1:], " ")
			otherplayer.Printf("%v:[Tell] %v \n", player.Name, message)
		}else{
			player.Printf("Player '%s' not found", cmd[0])
		}
	}
}
func cmdShout(player *Player, cmd []string){
	if len(cmd) == 0{
		player.Printf("Must type a message to shout")
	}
	message := strings.Join(cmd, " ")
	for _, otherplayer := range players{
		if otherplayer.Location.Zone.ID == player.Location.Zone.ID{
			otherplayer.Printf("%v: [Shout] %v \n", player.Name, message)
		}
	}
}

//Look a direction commands
func cmdLookNorth(player *Player, cmd []string) {
	fmt.Println("North Room Description: " + player.Location.Exits[0].Description + "\n")
}
func cmdLookSouth(player *Player, cmd []string) {
	fmt.Println("South Room Description: " + player.Location.Exits[1].Description + "\n")
}
func cmdLookEast(player *Player, cmd []string) {
	fmt.Println("East Room Description: " + player.Location.Exits[2].Description + "\n")
}
func cmdLookWest(player *Player, cmd []string) {
	fmt.Println("West Room Description: " + player.Location.Exits[3].Description + "\n")
}
func cmdLookUp(player *Player, cmd []string) {
	fmt.Println("Up Room Description: " + player.Location.Exits[4].Description + "\n")
}
func cmdLookDown(player *Player, cmd []string) {
	fmt.Println("Down Room Description: " + player.Location.Exits[5].Description + "\n")
}

//Directional Commands
func cmdNorth(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[0].Description != ""{
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v went North\n", player.Name)
			}
		}
		player.Location = room.Exits[0].To
		player.Printf("%s\n", player.Location.Description)
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v came from the South\n", player.Name)
			}
		}
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdEast(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[1].Description != ""{
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v went East\n", player.Name)
			}
		}
		player.Location = room.Exits[1].To
		player.Printf("%s\n", player.Location.Description)
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v came from the West\n", player.Name)
			}
		}
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdWest(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[2].Description != ""{
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v went West\n", player.Name)
			}
		}
		player.Location = room.Exits[2].To
		player.Printf("%s\n", player.Location.Description)
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v came from the East\n", player.Name)
			}
		}
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdSouth(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[3].Description != ""{
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v went South\n", player.Name)
			}
		}
		player.Location = room.Exits[3].To
		player.Printf("%s\n", player.Location.Description)
		for _, otherplayer := range players{
			if otherplayer.Location == player.Location && otherplayer != player{
				otherplayer.Printf("%v came from the North\n", player.Name)
			}
		}
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdUp(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[4].Description != ""{
		player.Location = room.Exits[4].To
		player.Printf("%s\n", player.Location.Description)
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdDown(player *Player, cmd []string) {
	room := player.Location
	if room.Exits[5].Description != ""{
		player.Location = room.Exits[5].To
		player.Printf("%s\n", player.Location.Description)
	}else{
		player.Printf("You can't go that way\n")
	}
}
func cmdRecall(player *Player, cmd []string){
	for _, otherplayer := range players{
		if otherplayer.Location == player.Location && otherplayer != player{
			otherplayer.Printf("%v recalled\n", player.Name)
		}
	}
	player.Location = player.Home
	player.Printf("%s\n", player.Location.Description)
}

func printRoomDescription(player *Player){
	fmt.Printf(player.Location.Name)
    fmt.Printf("\n")
    fmt.Printf(player.Location.Description)
    availableExits := "[ Exits:"
    if player.Location.Exits[0].To != nil {
        availableExits += " n "
    }
    if player.Location.Exits[1].To != nil {
        availableExits += " e "
    }
    if player.Location.Exits[2].To != nil {
        availableExits += " w "
    }
    if player.Location.Exits[3].To != nil {
        availableExits += " s "
    }
    if player.Location.Exits[4].To != nil {
        availableExits += " u "
    }
    if player.Location.Exits[5].To != nil {
        availableExits += " d "
    }
    availableExits += "]"
    fmt.Printf(availableExits)
    fmt.Printf("\n")
}

//Directions Look-up
func directionsLookUp() {
	dirLookup["north"]=0
	dirLookup["n"]=0
	dirLookupInt[0]="north"

	dirLookup["east"]=1
	dirLookup["e"]=1
	dirLookupInt[1]="east"

	dirLookup["west"]=2
	dirLookup["w"]=2
	dirLookupInt[2]="west"

	dirLookup["south"]=3
	dirLookup["s"]=3
	dirLookupInt[3]="south"

	dirLookup["up"]=4
	dirLookup["u"]=4
	dirLookupInt[4]="up"

	dirLookup["down"]=5
	dirLookup["d"]=5
	dirLookupInt[5]="down"

}

// Handle Player connection
func handlePlayerConnection(db *sql.DB, conn net.Conn, inputs chan<-InputEvent, player *Player){
	fmt.Fprintf(conn, "Welcome %s! ", player.Name)
	scanner := bufio.NewScanner(conn)
	fmt.Fprintln(conn, "Enter a command or type 'quit' to quit")
	go func(){
		log.Println("Go routine running")
		for{
			output, more := <-player.Outputs
			if more{
				fmt.Fprintf(conn, output.Text)
			}else{
				log.Print("Output channel closed for the user: ", player.Name)
				conn.Close()
				return
			}
		}
	}()
	for scanner.Scan(){
		log.Println("scanner running")
		input := scanner.Text()
		commands := strings.Fields(input)
		if len(commands) == 0{
			fmt.Fprintln(conn, "Please enter a command")
		}else if commands[0] == "quit" || commands[0] == "Quit"{
			input := InputEvent{player, commands, true, false}
			inputs <- input
		}else{
			input := InputEvent{player, commands, false, false}
			inputs <- input
		}

		if err := scanner.Err(); err != nil{
			fmt.Fprintln(os.Stderr, "Reading std input: ", err)
		}
	}
}

// Printf to pass errors
func (p *Player) Printf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	p.Outputs <- OutputEvent{Text: msg}
}
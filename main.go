package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"go.bug.st/serial"
)

//-------------------------------------------------------------------------
func main() {
	agent := SSE()
	fmt.Println("GPS-Track-Recorder")
	fmt.Printf("Operating System : %s\n", runtime.GOOS)

	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		log.Fatal("No serial ports found!")
	}

	// Print the list of detected ports
	for _, port := range ports {
		fmt.Printf("Found port: %v\n", port)
	}

	// Open the first serial port detected at 9600bps N81
	mode := &serial.Mode{
		BaudRate: 4800,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(ports[0], mode)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			line := ""
			buff := make([]byte, 1)
			maxlata := 0
			minlata := 999999999
			maxlona := 0
			minlona := 999999999
			maxlatb := 0
			minlatb := 999999999
			maxlonb := 0
			minlonb := 999999999
			on := true
			for on != false {
				line = ""
				for {
					n, err := port.Read(buff)
					if err != nil {
						log.Fatal(err)
					}
					if n == 0 {
						fmt.Println("\nEOF")
						break
					}
					line = line + string(buff[:n])
					if strings.Contains(string(buff[:n]), "\n") {
						break
					}

				}

				id, latitude, longitude, ns, ew, speed, degree := getGPSPosition(line)
				latdif := 0
				londif := 0
				if len(id) > 0 {
					if len(latitude) > 0 {
						l := strings.Split(latitude, ".")
						la, _ := strconv.Atoi(l[0])
						lb, _ := strconv.Atoi(l[1])
						if la > maxlata {
							maxlata = la
						}
						if lb > maxlatb {
							maxlatb = lb
						}
						if la < minlata {
							minlata = la
						}
						if lb < minlatb {
							minlatb = lb
						}
						latdif = maxlata - minlata
						latdif = maxlatb - minlatb
						avg := maxlatb - latdif/2
						off := lb - avg
						fmt.Printf("Latitude maxb %d  minb %d  avg %d off %d\n", maxlatb, minlatb, avg, off)
					}
					if len(longitude) > 0 {
						l := strings.Split(longitude, ".")
						la, _ := strconv.Atoi(l[0])
						lb, _ := strconv.Atoi(l[1])
						if la > maxlona {
							maxlona = la
						}
						if lb > maxlonb {
							maxlonb = lb
						}
						if la < minlona {
							minlona = la
						}
						if lb < minlonb {
							minlonb = lb
						}
						londif = maxlona - minlona
						londif = maxlonb - minlonb
						avg := maxlonb - londif/2
						off := lb - avg
						fmt.Printf("Logitude maxb %d  minb %d avg %d off %d\n", maxlonb, minlonb, avg, off)

					}

					event := fmt.Sprintf("%s  latitude=%s  %s %d  longitude=%s %s %d knots=%s degrees=%s\n", id, latitude, ns, latdif, longitude, ew, londif, speed, degree)
					fmt.Println(event)
					agent.Notifier <- []byte(event)
				}
			}
		}
	}()

	Openbrowser("http://localhost:8080")
	log.Fatal("HTTP server error: ", http.ListenAndServe("localhost:8080", agent))

}

func getGPSPosition(sentence string) (string, string, string, string, string, string, string) {
	data := strings.Split(sentence, ",")
	id := ""
	latitude := ""
	longitude := ""
	ns := ""
	ew := ""
	speed := ""
	degree := ""

	switch {
	case string(data[0]) == "$GPGGA":
		// id = data[0]
		latitude = data[2]
		ns = data[3]
		longitude = data[4]
		ew = data[5]

	case string(data[0]) == "$GPGLL":
		// id = data[0]
		latitude = data[1]
		ns = data[2]
		longitude = data[3]
		ew = data[4]

	case string(data[0]) == "$GPVTG":
		//id = data[0]
		degree = data[1]

	case string(data[0]) == "$GPRMC":
		id = data[0]
		latitude = data[3]
		ns = data[4]
		longitude = data[5]
		ew = data[6]
		speed = data[7]
		degree = data[8]

	case string(data[0]) == "$GPGSA":
	//	id = data[0]

	case string(data[0]) == "$GPGSV":
		//id = data[0]

	case string(data[0]) == "$GPTXT":
	//	id = data[0]

	default:
		fmt.Println("-- %s", data[0])

	}

	return id, latitude, longitude, ns, ew, speed, degree
}

// Openbrowser : Opens default web browser to specified url
func Openbrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "linux":
		cmd = "chromium-browser"
		args = []string{""}

	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

type Agent struct {
	Notifier    chan []byte
	newuser     chan chan []byte
	closinguser chan chan []byte
	user        map[chan []byte]bool
}

func SSE() (agent *Agent) {
	agent = &Agent{
		Notifier:    make(chan []byte, 1),
		newuser:     make(chan chan []byte),
		closinguser: make(chan chan []byte),
		user:        make(map[chan []byte]bool),
	}
	go agent.listen()
	return
}

func (agent *Agent) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Error ", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	mChan := make(chan []byte)
	agent.newuser <- mChan
	defer func() {
		agent.closinguser <- mChan
	}()
	notify := req.Context().Done()
	go func() {
		<-notify
		agent.closinguser <- mChan
	}()
	for {
		fmt.Fprintf(rw, "%s", <-mChan)
		flusher.Flush()
	}

}

func (agent *Agent) listen() {
	for {
		select {
		case s := <-agent.newuser:
			agent.user[s] = true
		case s := <-agent.closinguser:
			delete(agent.user, s)
		case event := <-agent.Notifier:
			for userMChan, _ := range agent.user {
				userMChan <- event
			}
		}
	}

}

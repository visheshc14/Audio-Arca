package main

import (
	"encoding/binary"
	"fmt"
	"github.com/gordonklaus/portaudio"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
)

const sampleRate = 44100

type user struct {
	name        string
	chatWriter  http.ResponseWriter
	audioWriter http.ResponseWriter
	idsSent     []int
}

var users map[string]user
var usersWriteLock sync.Mutex

type messageAndId struct {
	id  int
	msg string
}

var id int = 0
var messages []messageAndId

func main() {
	portaudio.Initialize()
	defer portaudio.Terminate()
	users = map[string]user{}
	var bufLen string
	if len(os.Args) < 2 {
		fmt.Println("How many seconds long should the buffer be (values lower than 0.03 result in low audio quality delay will be at least 2*length):")
		fmt.Scanln(&bufLen)
	} else {
		bufLen = os.Args[1]
	}
	var seconds, _ = strconv.ParseFloat(bufLen, 64)
	buffer := make([]float32, int64(sampleRate*seconds))
	stream, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(buffer), func(in []float32) {
		for i := range buffer {
			buffer[i] = in[i]
		}
	})
	chk(err)
	chk(stream.Start())
	defer stream.Close()

	http.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("expected http.ResponseWriter to be an http.Flusher")
		}
		userData, exists := users[r.RemoteAddr]
		usersWriteLock.Lock()
		if exists {
			userData.audioWriter = w
			users[r.RemoteAddr] = userData
		} else {
			users[r.RemoteAddr] = user{r.RemoteAddr, nil, w, make([]int, 0)}
		}
		usersWriteLock.Unlock()
		w.Header().Set("Connection", "Keep-Alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "audio/wave")
		for true {
			binary.Write(w, binary.BigEndian, &buffer)
			flusher.Flush() // Trigger "chunked" encoding and send a chunk...
			return
		}
	})
	http.HandleFunc("/bufsize", func(writer http.ResponseWriter, request *http.Request) {
		binary.Write(writer, binary.BigEndian, seconds)
	})
	http.HandleFunc("/chatin", func(writer http.ResponseWriter, request *http.Request) {
		userData, exists := users[request.RemoteAddr]
		usersWriteLock.Lock()
		if exists {
			userData.audioWriter = writer
			users[request.RemoteAddr] = userData
		} else {
			users[request.RemoteAddr] = user{request.RemoteAddr, nil, nil, make([]int, 0)}
		}
		usersWriteLock.Unlock()
		request.ParseForm()
		sendmsg(request.Form["message"][0], users[request.RemoteAddr])
		defer request.Body.Close()
	})
	http.HandleFunc("/chatout", func(writer http.ResponseWriter, request *http.Request) {
		userData, exists := users[request.RemoteAddr]
		usersWriteLock.Lock()
		if exists {
			userData.chatWriter = writer
			users[request.RemoteAddr] = userData
		} else {
			users[request.RemoteAddr] = user{request.RemoteAddr, writer, nil, make([]int, 0)}
		}
		usersWriteLock.Unlock()
		user := users[request.RemoteAddr]
		for _, msg := range messages {
			if contains(msg.id, user.idsSent) {
				continue
			}
			binary.Write(writer, binary.BigEndian, msg.msg)
			//fmt.Println("Message sent")
			user.idsSent = append(users[request.RemoteAddr].idsSent, msg.id)
		}
		usersWriteLock.Lock()
		users[request.RemoteAddr] = user
		usersWriteLock.Unlock()
	})
	http.HandleFunc("/setname", func(writer http.ResponseWriter, request *http.Request) {
		request.ParseForm()
		userData, exists := users[request.RemoteAddr]
		usersWriteLock.Lock()
		if exists {
			userData.name = request.Form["name"][0]
			users[request.RemoteAddr] = userData
		} else {
			users[request.RemoteAddr] = user{request.Form["name"][0], nil, nil, make([]int, 0)}
		}
		usersWriteLock.Unlock()
	})
	fmt.Println("Server Created")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func sendmsg(msg string, sourceUser user) {
	msg = sourceUser.name + ": " + msg
	fmt.Println(msg)
	//for _, destUser := range users {
	//	if destUser == sourceUser {continue}
	//	binary.Write(destUser.chatWriter, binary.BigEndian, msg)
	//}
	messages = append(messages, messageAndId{id, msg})
	id += 1
}

func contains(id int, ids []int) bool {
	var doesContain = false
	for idCheck := range ids {
		doesContain = doesContain || id == idCheck
	}
	return doesContain
}

func chk(err error) {
	if err != nil {
		panic(err)
	}
}

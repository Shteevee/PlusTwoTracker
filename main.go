package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gempir/go-twitch-irc/v3"
	"github.com/gorilla/websocket"
)

const (
	streamer   string        = "Northernlion"
	windowSize time.Duration = 10
)

var addr = flag.String("addr", "localhost:8080", "http service address")

var upgrader = websocket.Upgrader{}

type StatWindow struct {
	Time      int64
	PlusTwos  uint32
	MinusTwos uint32
}

func findConnIndex(connStats []chan StatWindow, conn chan StatWindow) int {
	for i, c := range connStats {
		if c == conn {
			return i
		}
	}
	return -1
}

func getStat(stat *uint32) uint32 {
	return atomic.LoadUint32(stat)
}

func resetStat(stat *uint32) {
	atomic.StoreUint32(stat, 0)
}

func incrementStat(stat *uint32) {
	atomic.AddUint32(stat, 1)
}

func handleInterrupt(c chan os.Signal) {
	<-c
	fmt.Println("Closing...")
	os.Exit(0)
}

func pushStatsWindow(
	connStats *[]chan StatWindow,
	plusTwos *uint32,
	minusTwos *uint32,
) {
	for range time.Tick(time.Second * windowSize) {
		statWindow := StatWindow{
			Time:      time.Now().Unix(),
			PlusTwos:  getStat(plusTwos),
			MinusTwos: getStat(minusTwos),
		}
		log.Println(statWindow)
		for _, stats := range *connStats {
			stats <- statWindow
		}
		resetStat(plusTwos)
		resetStat(minusTwos)
	}
}

func handleWebsocket(
	w http.ResponseWriter,
	r *http.Request,
	statWindows chan StatWindow,
) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	for statWindow := range statWindows {
		json, _ := json.Marshal(statWindow)
		err := c.WriteMessage(websocket.TextMessage, json)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func createTwitchClient(plusTwos *uint32, minusTwos *uint32) *twitch.Client {
	client := twitch.NewAnonymousClient()
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		if strings.Contains(message.Message, "+2") {
			incrementStat(plusTwos)
		}
		if strings.Contains(message.Message, "-2") {
			incrementStat(minusTwos)
		}
	})
	client.OnConnect(func() {
		fmt.Println("Collecting plus twos...")
	})
	client.Join(streamer)

	return client
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go handleInterrupt(c)

	var plusTwos uint32 = 0
	var minusTwos uint32 = 0
	connStats := make([]chan StatWindow, 0)
	connStatsLock := sync.Mutex{}

	http.HandleFunc(
		"/tracker",
		func(w http.ResponseWriter, r *http.Request) {
			statWindows := make(chan StatWindow)
			connStatsLock.Lock()
			connStats = append(connStats, statWindows)
			connStatsLock.Unlock()
			defer func() {
				connStatsLock.Lock()
				i := findConnIndex(connStats, statWindows)
				if i != -1 {
					connStats = connStats[:i+copy(connStats[i:], connStats[i+1:])]
				}
				connStatsLock.Unlock()
			}()
			handleWebsocket(w, r, statWindows)
		},
	)

	go http.ListenAndServe(*addr, nil)
	go pushStatsWindow(&connStats, &plusTwos, &minusTwos)

	client := createTwitchClient(&plusTwos, &minusTwos)
	clientErr := client.Connect()
	if clientErr != nil {
		panic(clientErr)
	}
}

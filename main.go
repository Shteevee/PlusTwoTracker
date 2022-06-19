package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gempir/go-twitch-irc/v3"
)

const (
	streamer   string        = "Northernlion"
	windowSize time.Duration = 10
)

type StatTracker struct {
	mu    sync.Mutex
	count uint32
}

func (st *StatTracker) incrementCount() {
	st.mu.Lock()
	st.count++
	st.mu.Unlock()
}

func handleInterrupt(c chan os.Signal, f *os.File) {
	<-c
	fmt.Println("Closing...")
	f.Close()
	os.Exit(0)
}

func writeCounts(f *os.File, plus2Count uint32, minus2Count uint32) {
	line := fmt.Sprintf("%d,%d,%d\n", time.Now().Unix(), plus2Count, minus2Count)
	if _, err := f.Write([]byte(line)); err != nil {
		log.Fatal(err)
	}
}

func handleMessageWindow(f *os.File, plusTwos chan int, minusTwos chan int) {
	plusTwoTracker := StatTracker{mu: sync.Mutex{}, count: 0}
	minusTwoTracker := StatTracker{mu: sync.Mutex{}, count: 0}
	go func() {
		for range plusTwos {
			plusTwoTracker.incrementCount()
		}
	}()
	go func() {
		for range minusTwos {
			minusTwoTracker.incrementCount()
		}
	}()
	for range time.Tick(time.Second * windowSize) {
		plusTwoTracker.mu.Lock()
		minusTwoTracker.mu.Lock()
		writeCounts(f, plusTwoTracker.count, minusTwoTracker.count)
		plusTwoTracker.count = 0
		minusTwoTracker.count = 0
		plusTwoTracker.mu.Unlock()
		minusTwoTracker.mu.Unlock()
	}
}

func createTwitchClient(plusTwos chan int, minusTwos chan int) *twitch.Client {
	client := twitch.NewAnonymousClient()
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		if strings.Contains(message.Message, "+2") {
			plusTwos <- 1
		} else if strings.Contains(message.Message, "-2") {
			minusTwos <- 1
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

	f, fileErr := os.OpenFile("plus2.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr != nil {
		log.Fatal(fileErr)
	}
	go handleInterrupt(c, f)

	plusTwos := make(chan int)
	minusTwos := make(chan int)
	go handleMessageWindow(f, plusTwos, minusTwos)

	client := createTwitchClient(plusTwos, minusTwos)
	clientErr := client.Connect()
	if clientErr != nil {
		panic(clientErr)
	}
}

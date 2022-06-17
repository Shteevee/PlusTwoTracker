package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gempir/go-twitch-irc/v3"
)

func handleInterrupt(c chan os.Signal, f *os.File) {
	<-c
	fmt.Println("Closing...")
	f.Close()
	os.Exit(0)
}

func writeCounts(f *os.File, plus2Count int, minus2Count int) {
	line := fmt.Sprintf("%d,%d,%d\n", time.Now().Unix(), plus2Count, minus2Count)
	if _, err := f.Write([]byte(line)); err != nil {
		log.Fatal(err)
	}
}

func handleMessageWindow(f *os.File, plusTwos chan int, minusTwos chan int) {
	plusTwoCount := 0
	minusTwoCount := 0
	go func() {
		for range plusTwos {
			plusTwoCount++
		}
	}()
	go func() {
		for range minusTwos {
			minusTwoCount++
		}
	}()
	for range time.Tick(time.Second * 10) {
		writeCounts(f, plusTwoCount, minusTwoCount)
		plusTwoCount = 0
		minusTwoCount = 0
	}
}

func createTwitchClient(messageCallback func(message twitch.PrivateMessage)) *twitch.Client {
	client := twitch.NewAnonymousClient()
	client.OnPrivateMessage(messageCallback)
	client.OnConnect(func() {
		fmt.Println("Collecting plus twos...")
	})
	client.Join("Northernlion")

	return client
}

func messageCallback(plusTwos chan int, minusTwos chan int) func(message twitch.PrivateMessage) {
	return func(message twitch.PrivateMessage) {
		if strings.Contains(message.Message, "+2") {
			plusTwos <- 1
		} else if strings.Contains(message.Message, "-2") {
			minusTwos <- 1
		}
	}
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

	client := createTwitchClient(messageCallback(plusTwos, minusTwos))
	clientErr := client.Connect()
	if clientErr != nil {
		panic(clientErr)
	}
}

package main

import (
	"errors"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"os"
	"runtime"
	"strings"
	"time"
)

var searchTerms = [...]string{"Solana"}

var generatedCount = 0
var numThreads = 16
var startTime = time.Now()
var shouldStopThreads = false
var logFile = fmt.Sprintf("solana_%d.log", startTime.Unix())
var remainingSearches = []string{}

func remove(s []string, index int) ([]string, error) {
	if index >= len(s) {
		return nil, errors.New("Out of Range Error")
	}
	return append(s[:index], s[index+1:]...), nil
}

func generateWallet(f *os.File) {
	for {
		if shouldStopThreads {
			return
		}
		newWallet := solana.NewWallet()
		for i := 0; i < len(remainingSearches); i++ {
			currentLookup := remainingSearches[i]
			if strings.HasPrefix(newWallet.PublicKey().String(), currentLookup) && !shouldStopThreads {
				firstCharAfterSearchTerm := strings.Split(newWallet.PublicKey().String(), currentLookup)[1][0:1]
				if firstCharAfterSearchTerm == strings.ToUpper(firstCharAfterSearchTerm) {
					fmt.Printf("Success! Wallet found: %s\n", newWallet.PublicKey())
					fmt.Printf("Secret Key: %v\n", newWallet.PrivateKey)
					fmt.Printf("Attempts required: %d, Time elapsed: %s\n\n", generatedCount+1, time.Since(startTime))

					if _, err := f.WriteString(fmt.Sprintf("%s | %v | Took: %s\n", newWallet.PublicKey(), newWallet.PrivateKey, time.Since(startTime))); err != nil {
						panic(err)
					}

					leftOver, err := remove(remainingSearches, i)
					remainingSearches = leftOver
					if err != nil {
						shouldStopThreads = true
					}
				}
			}
		}
		generatedCount++
		if generatedCount%1000000 == 0 {
			fmt.Printf("Status: %d wallets generated in %s\n", generatedCount, time.Since(startTime))
		}
	}
}

func main() {
	// Get the current GOMAXPROCS value
	currentValue := runtime.GOMAXPROCS(0)
	fmt.Printf("Current GOMAXPROCS value: %d\n", currentValue)

	// Set GOMAXPROCS to utilize all available CPU cores
	maxCores := runtime.NumCPU()
	fmt.Printf("maxCores value: %d\n", maxCores)
	newValue := maxCores - 2

	runtime.GOMAXPROCS(newValue)
	fmt.Printf("Updated GOMAXPROCS value: %d\n", runtime.GOMAXPROCS(0))

	for i := 0; i < len(searchTerms); i++ {
		remainingSearches = append(remainingSearches, searchTerms[i])
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err = f.WriteString(fmt.Sprintf("\n%s\n", startTime)); err != nil {
		panic(err)
	}

	fmt.Printf("Target prefixes:\n")
	fmt.Println(remainingSearches)
	fmt.Printf("Starting...\n\n")

	for i := 0; i < numThreads; i++ {
		go generateWallet(f)
	}

	fmt.Scanln()

	if _, err = f.WriteString(fmt.Sprintf("----------------- %s -------------------\n\n", time.Since(startTime))); err != nil {
		panic(err)
	}
}

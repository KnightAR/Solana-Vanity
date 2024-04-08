package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"go-hep.org/x/hep/csvutil"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var searchTerms = [...]string{}

var generatedCount = 0
var numThreads = 14
var startTime = time.Now()
var shouldStopThreads = false
var remainingSearches = []string{}
var remainingSearchesLOWER = []string{}
var searchTermsFile = "searchTerms.txt"

var logFile = fmt.Sprintf("logs/solana_%d.log", startTime.Unix())
var resultsFile = fmt.Sprintf("results/%d.csv", startTime.Unix())
var resultsNonExactFile = fmt.Sprintf("results/%d_nonexact.csv", startTime.Unix())

var logLock = &sync.Mutex{}
var resultsLock = &sync.Mutex{}
var nonExactResultsLock = &sync.Mutex{}

var singleLogInstance *os.File

func getLogInstance() *os.File {
	if singleLogInstance == nil {
		logLock.Lock()
		defer logLock.Unlock()
		if singleLogInstance == nil {
			var err error = nil
			singleLogInstance, err = os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if err != nil {
				panic(err)
			}
		}
	}

	return singleLogInstance
}

var singleResultsInstance *csvutil.Table

func getResultsInstance() *csvutil.Table {
	if singleResultsInstance == nil {
		resultsLock.Lock()
		defer resultsLock.Unlock()
		if singleResultsInstance == nil {
			var err error = nil
			singleResultsInstance, err = csvutil.Create(resultsFile)
			if err != nil {
				log.Fatalf("could not create %s: %v\n", resultsFile, err)
			}

			err = singleResultsInstance.WriteHeader("term,publickey,privatekey,attempts,time\n")
			if err != nil {
				log.Fatalf("error writing header: %v\n", err)
			}
		}
	}

	return singleResultsInstance
}

var singleNoExactResultsInstance *csvutil.Table

func getNonExactResultsInstance() *csvutil.Table {
	if singleNoExactResultsInstance == nil {
		nonExactResultsLock.Lock()
		defer nonExactResultsLock.Unlock()
		if singleNoExactResultsInstance == nil {
			var err error = nil
			singleNoExactResultsInstance, err = csvutil.Create(resultsNonExactFile)
			if err != nil {
				log.Fatalf("could not create %s: %v\n", resultsNonExactFile, err)
			}

			err = singleNoExactResultsInstance.WriteHeader("term,publickey,privatekey,attempts,time\n")
			if err != nil {
				log.Fatalf("error writing header: %v\n", err)
			}
		}
	}

	return singleNoExactResultsInstance
}

// Read a whole file into the memory and store it as array of lines
func readLines(path string) (lines []string, err error) {
	var (
		file   *os.File
		part   []byte
		prefix bool
	)
	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buffer := bytes.NewBuffer(make([]byte, 0))
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			lines = append(lines, buffer.String())
			buffer.Reset()
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func remove(s []string, index int) ([]string, error) {
	if index >= len(s) {
		return nil, errors.New("Out of Range Error")
	}
	return append(s[:index], s[index+1:]...), nil
}

func clean(s []byte) string {
	j := 0
	for _, b := range s {
		if ('a' <= b && b <= 'z') ||
			('A' <= b && b <= 'Z') ||
			('0' <= b && b <= '9') {
			s[j] = b
			j++
		}
	}
	return string(s[:j])
}

func generateWallet(logFile *os.File) {
	tbl := getResultsInstance()
	tblNonExactResults := getNonExactResultsInstance()
	p := message.NewPrinter(language.English)
	for {
		if shouldStopThreads {
			fmt.Printf("Stopping threads ... %d wallets generated in %s\n", generatedCount, time.Since(startTime))
			return
		}
		newWallet := solana.NewWallet()
		publicKey := newWallet.PublicKey()
		publicKeyString := publicKey.String()
		publicKeyStringLOWER := strings.ToLower(publicKey.String())
		for i := 0; i < len(remainingSearches); i++ {
			currentLookup := remainingSearches[i]
			currentLookupLOWER := remainingSearchesLOWER[i]
			isLowerMatch := strings.HasPrefix(publicKeyStringLOWER, currentLookupLOWER)
			isExactMatch := strings.HasPrefix(publicKeyString, currentLookup)
			if (isLowerMatch || isExactMatch) && !shouldStopThreads {
				privateKeyString := newWallet.PrivateKey.String()
				since := time.Since(startTime)
				foundTerm := publicKeyString[0:len(currentLookup)]
				if isExactMatch {
					fmt.Printf("Exact Match: Success! Wallet found: %s\n", publicKey)
				} else {
					fmt.Printf("Non-Exact Match: Success! Wallet found: %s\n", publicKey)
				}
				fmt.Printf("Secret Key: %v\n", privateKeyString)
				p.Printf("Attempts required: %d, Time elapsed: %s\n", generatedCount+1, since)

				if isExactMatch {
					if _, err := logFile.WriteString(p.Sprintf("Exact Match Found: %s | %s | %v | Attempts: %d | Took: %s\n", foundTerm, publicKeyString, privateKeyString, generatedCount+1, since)); err != nil {
						panic(err)
					}

					err := tbl.WriteRow(foundTerm, publicKeyString, privateKeyString, generatedCount+1, since)
					if err != nil {
						log.Fatalf("error writing row %d: %v\n", i, err)
					}
				} else {
					if _, err := logFile.WriteString(p.Sprintf("Non-Exact Match Found: %s | %s | %v | Attempts: %d | Took: %s\n", foundTerm, publicKeyString, privateKeyString, generatedCount+1, since)); err != nil {
						panic(err)
					}
					err := tblNonExactResults.WriteRow(foundTerm, publicKeyString, privateKeyString, generatedCount+1, since)
					if err != nil {
						log.Fatalf("error writing row %d: %v\n", i, err)
					}
				}

				if isExactMatch {
					//firstCharAfterSearchTerm := strings.Split(publicKeyString, currentLookup)[1][0:1]
					//if firstCharAfterSearchTerm == strings.ToUpper(firstCharAfterSearchTerm) {
					/*					fmt.Printf("Success! Wallet found: %s\n", publicKey)
										fmt.Printf("Secret Key: %v\n", newWallet.PrivateKey)
										p.Printf("Attempts required: %d, Time elapsed: %s\n\n", generatedCount+1, time.Since(startTime))

										if _, err := f.WriteString(p.Sprintf("Found: %s | %s | %v | Attempts: %d | Took: %s\n", currentLookup, publicKeyString, newWallet.PrivateKey, generatedCount+1, time.Since(startTime))); err != nil {
											panic(err)
										}

										var (
											pk = fmt.Sprintf("%s", newWallet.PrivateKey)
											ts = time.Since(startTime)
										)
										err := tbl.WriteRow(currentLookup, publicKeyString, pk, generatedCount+1, ts)
										if err != nil {
											log.Fatalf("error writing row %d: %v\n", i, err)
										}
										tbl.Writer.Flush()*/

					p.Printf("---> Found: %s REMOVING FROM SEARCH TERMS <---- \n", currentLookup)

					leftOver, err := remove(remainingSearches, i)
					remainingSearches = leftOver
					if err != nil {
						shouldStopThreads = true
					}

					leftOverLOWER, err := remove(remainingSearchesLOWER, i)
					remainingSearchesLOWER = leftOverLOWER
					if err != nil {
						shouldStopThreads = true
					}

					if len(remainingSearches) == 0 {
						shouldStopThreads = true
					}

					//}
				}

				p.Printf("\n")
			}
		}
		generatedCount++
		if generatedCount%1000000 == 0 {
			p.Printf("Status: %d wallets generated in %s | Prefixes Remaining: %d\n", generatedCount, time.Since(startTime), len(remainingSearches))
			tbl.Writer.Flush()
			tblNonExactResults.Writer.Flush()
		}

		/*if generatedCount%10000000 == 0 {
			if len(remainingSearches) > 20 {
				p.Printf("Searching for %d prefixes\n", len(remainingSearches))
			} else {
				fmt.Printf("Target prefixes:\n")
				fmt.Println(remainingSearches)
			}
		}*/
	}
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	//return !os.IsNotExist(err)
	return !errors.Is(err, os.ErrNotExist)
}

func makeBase58Comp(input string) string {
	input = strings.ReplaceAll(input, "0", "o")
	input = strings.ReplaceAll(input, "O", "o")
	input = strings.ReplaceAll(input, "I", "i")
	input = strings.ReplaceAll(input, "l", "L")
	return input
}

func main() {

	// Get the current GOMAXPROCS value
	//currentValue := runtime.GOMAXPROCS(0)
	//fmt.Printf("Current GOMAXPROCS value: %d\n", currentValue)

	// Set GOMAXPROCS to utilize all available CPU cores
	maxCores := runtime.NumCPU()
	//fmt.Printf("maxCores value: %d\n", maxCores)
	newValue := maxCores - 2

	runtime.GOMAXPROCS(newValue)
	//fmt.Printf("Updated GOMAXPROCS value: %d\n", runtime.GOMAXPROCS(0))

	p := message.NewPrinter(language.English)

	var f *os.File = getLogInstance()
	defer f.Close()

	if len(searchTerms) > 0 {
		for i := 0; i < len(searchTerms); i++ {
			byteLine := []byte(searchTerms[i])
			newLine := makeBase58Comp(clean(byteLine))
			remainingSearches = append(remainingSearches, newLine)
			remainingSearchesLOWER = append(remainingSearchesLOWER, strings.ToLower(newLine))
		}
	}

	isFileExist := checkFileExists(searchTermsFile)

	if isFileExist {
		lines, err := readLines(searchTermsFile)
		if err != nil {
			fmt.Printf("Error reading from file %s: %s\n\n", searchTermsFile, err)
			return
		}
		for _, line := range lines {
			byteLine := []byte(line)
			newLine := makeBase58Comp(clean(byteLine))
			remainingSearches = append(remainingSearches, newLine)
			remainingSearchesLOWER = append(remainingSearchesLOWER, strings.ToLower(newLine))

		}
	}

	entries, err := os.ReadDir("./searches")
	if err != nil {
		log.Fatal(err)
	}

	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			//fmt.Println(e.Name())

			lines, err := readLines("./searches/" + e.Name())
			if err != nil {
				fmt.Printf("Error reading from file %s: %s\n\n", "./searches/"+e.Name(), err)
				return
			}
			for _, line := range lines {
				byteLine := []byte(line)
				newLine := makeBase58Comp(clean(byteLine))
				remainingSearches = append(remainingSearches, newLine)
				remainingSearchesLOWER = append(remainingSearchesLOWER, strings.ToLower(newLine))

				/*				if !strings.HasPrefix(newLine, strings.ToLower(newLine)) {
								remainingSearches = append(remainingSearches, strings.ToLower(newLine))
							}*/
			}
		}
	}

	if len(remainingSearches) == 0 {
		panic("No search terms detected, Please add some to searchTerms.txt.")
	}

	if _, err := f.WriteString(p.Sprintf("\n%s\n", startTime)); err != nil {
		panic(err)
	}

	fmt.Printf("Target prefixes:\n")
	if len(remainingSearches) > 20 {
		p.Printf("Searching for %d prefixes\n", len(remainingSearches))
	} else {
		fmt.Println(remainingSearches)
	}
	fmt.Printf("Starting...\n\n")

	tblResults := getResultsInstance()
	defer tblResults.Close()

	tblNonExactResults := getNonExactResultsInstance()
	defer tblNonExactResults.Close()

	for i := 0; i < numThreads; i++ {
		go generateWallet(f)
	}

	fmt.Scanln()

	if _, err := f.WriteString(p.Sprintf("%d wallets generated in %s\n", generatedCount, time.Since(startTime))); err != nil {
		panic(err)
	}

	if _, err := f.WriteString(p.Sprintf("----------------- %s -------------------\n\n", time.Since(startTime))); err != nil {
		panic(err)
	}
}

func init() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		// Run Cleanup

		tbl := getResultsInstance()
		tbl.Writer.Flush()
		tbl.Close()

		tblNoneExact := getNonExactResultsInstance()
		tblNoneExact.Writer.Flush()
		tblNoneExact.Close()

		p := message.NewPrinter(language.English)
		f := getLogInstance()
		if _, err := f.WriteString("<--- Detected Ctrl+C ------->\n"); err != nil {
			panic(err)
		}

		if _, err := f.WriteString(p.Sprintf("%d wallets generated in %s\n", generatedCount, time.Since(startTime))); err != nil {
			panic(err)
		}

		if _, err := f.WriteString(p.Sprintf("----------------- %s -------------------\n\n", time.Since(startTime))); err != nil {
			panic(err)
		}

		os.Exit(1)
	}()
}

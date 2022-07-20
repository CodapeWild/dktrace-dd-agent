package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

var (
	path = "/v0.4/traces"
)

type Config struct {
	DDTraceFile  string `json:"ddtrace_file"`
	TotalClients int    `json:"total_clients"`
	RequestTimes int    `json:"request_times"`
	DKAddress    string `json:"dk_address"`
}

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	log.SetOutput(os.Stdout)

	buf, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatalln(err.Error())
	}

	config := &Config{}
	if err = json.Unmarshal(buf, config); err != nil {
		log.Fatalln(err.Error())
	}

	if buf, err = os.ReadFile(config.DDTraceFile); err != nil {
		log.Fatalln(err.Error())
	}

	var path = fmt.Sprintf("http://%s%s", config.DKAddress, path)

	wg := sync.WaitGroup{}
	wg.Add(config.TotalClients)
	for i := 0; i < config.TotalClients; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < config.RequestTimes; j++ {
				req, err := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(buf))
				if err != nil {
					log.Println(err.Error())
					continue
				}
				req.Header.Add("Content-Type", "application/msgpack")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Println(err.Error())
					continue
				}
				resp.Body.Close()

				log.Println(resp.Status)
			}
		}()
	}
	wg.Wait()
}

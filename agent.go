package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
)

var (
	globalCloser = make(chan struct{})
	ddv4         = "/v0.4/traces"
)

func handleDDTraceData(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalln(err.Error())
	}
	if len(buf) <= 1 {
		return
	}

	go sendDDTraceTask(cfg.Sender, buf, "http://"+cfg.DkAgent+ddv4)
}

func startAgent() {
	log.Printf("### start ddtrace agent %s", agentAddress)

	http.HandleFunc(ddv4, handleDDTraceData)
	if err := http.ListenAndServe(agentAddress, nil); err != nil {
		log.Fatalln(err.Error())
	}
}

func sendDDTraceTask(sender *sender, buf []byte, urlstr string) {
	if sender.Threads <= 0 || sender.SendCount <= 0 {
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(sender.Threads)
	for i := 0; i < sender.Threads; i++ {
		go func(buf []byte) {
			defer wg.Done()

			for j := 0; j < sender.SendCount; j++ {
				req, err := http.NewRequest(http.MethodPut, urlstr, bytes.NewBuffer(buf))
				if err != nil {
					log.Println(err.Error())
					continue
				}
				req.Header.Set("Content-Type", "application/msgpack")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Println(err.Error())
					continue
				}
				resp.Body.Close()
				log.Println(resp.Status)
			}
		}(buf)
	}
	wg.Wait()

	close(globalCloser)
}

package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/CodapeWild/dktrace-dd-agent/pb"
)

func startAgent() {
	log.Printf("### start DDTrace agent %s\n", agentAddress)

	svr := getTimeoutServer(agentAddress, http.HandlerFunc(handleDDTraceData))
	if err := svr.ListenAndServe(); err != nil {
		log.Fatalln(err.Error())
	}
}

func getTimeoutServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Millisecond,
		ReadTimeout:       time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       10 * time.Second,
	}
}

func handleDDTraceData(resp http.ResponseWriter, req *http.Request) {
	log.Println("### DDTrace original headers:")
	for k, v := range req.Header {
		log.Printf("%s: %v\n", k, v)
	}

	resp.WriteHeader(http.StatusOK)

	if req.Header.Get("X-Datadog-Trace-Count") == "0" {
		return
	}

	if cfg.Sender.Threads <= 0 || cfg.Sender.SendCount <= 0 {
		close(globalCloser)

		return
	}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalln(err.Error())
	}

	go sendDDTraceTask(cfg.Sender, buf, "http://"+cfg.DkAgent+ddv4, req.Header)
}

func sendDDTraceTask(sender *sender, buf []byte, endpoint string, headers http.Header) {
	ddtraces := pb.Traces{}
	if _, err := ddtraces.UnmarshalMsg(buf); err != nil {
		log.Fatalln(err.Error())
	}

	wg := sync.WaitGroup{}
	wg.Add(sender.Threads)
	for i := 0; i < sender.Threads; i++ {
		go func(ddtraces pb.Traces, ti int) {
			defer wg.Done()

			for j := 0; j < sender.SendCount; j++ {
				modifyIDs(ddtraces, uint64(ti), uint64(j))
				buf, err := ddtraces.MarshalMsg(nil)
				if err != nil {
					log.Fatalln(err.Error())
				}

				req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(buf))
				if err != nil {
					log.Println(err.Error())
					continue
				}
				req.Header = headers

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Println(err.Error())
					continue
				}
				log.Println(resp.Status)
				resp.Body.Close()
			}
		}(ddtraces, i+1)
	}
	wg.Wait()

	close(globalCloser)
}

func modifyIDs(ddtraces pb.Traces, ti, cj uint64) {
	if len(ddtraces) == 0 {
		return
	}

	for i := range ddtraces {
		if len(ddtraces[i]) == 0 {
			continue
		}
		var tid = ddtraces[i][0].TraceID*ti + cj
		for j := range ddtraces[i] {
			ddtraces[i][j].TraceID = tid
			if ddtraces[i][j].ParentID != 0 {
				ddtraces[i][j].ParentID = ddtraces[i][j].ParentID*ti + cj
			}
			ddtraces[i][j].SpanID = ddtraces[i][j].SpanID*ti + cj
		}
	}
}

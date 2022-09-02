package main

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
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

	rand.Seed(time.Now().UnixNano())

	wg := sync.WaitGroup{}
	wg.Add(sender.Threads)
	for i := 0; i < sender.Threads; i++ {
		dupi := duplicate(ddtraces)

		go func(ddtraces pb.Traces, ti int) {
			defer wg.Done()

			for j := 0; j < sender.SendCount; j++ {
				modifyIDs(ddtraces)
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
		}(dupi, i+1)
	}
	wg.Wait()

	close(globalCloser)
}

func duplicate(ddtraces pb.Traces) pb.Traces {
	dupi := make(pb.Traces, len(ddtraces))
	for i := range ddtraces {
		dupi[i] = make(pb.Trace, len(ddtraces[i]))
		for j := range ddtraces[i] {
			dupi[i][j] = &pb.Span{
				Service:    ddtraces[i][j].Service,
				Name:       ddtraces[i][j].Name,
				Resource:   ddtraces[i][j].Resource,
				TraceID:    ddtraces[i][j].TraceID,
				SpanID:     ddtraces[i][j].SpanID,
				ParentID:   ddtraces[i][j].ParentID,
				Start:      ddtraces[i][j].Start,
				Duration:   ddtraces[i][j].Duration,
				Error:      ddtraces[i][j].Error,
				Meta:       ddtraces[i][j].Meta,
				Metrics:    ddtraces[i][j].Metrics,
				Type:       ddtraces[i][j].Type,
				MetaStruct: ddtraces[i][j].MetaStruct,
			}
		}
	}

	return dupi
}

func modifyIDs(ddtraces pb.Traces) {
	if len(ddtraces) == 0 {
		log.Println("### empty ddtraces")
		return
	}

	var tid = uint64(idflk.NextInt64Id())
	for i := range ddtraces {
		if len(ddtraces[i]) == 0 {
			log.Println("### empty ddtrace")
			continue
		}

		for j := range ddtraces[i] {
			ddtraces[i][j].TraceID = tid
		}
	}
}

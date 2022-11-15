package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func startAgent() {
	log.Printf("### start DataDog APM agent %s\n", agentAddress)

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
	log.Println("### DataDog APM original headers:")
	for k, v := range req.Header {
		log.Printf("%s: %v\n", k, v)
	}

	resp.WriteHeader(http.StatusOK)

	if req.Header.Get("X-Datadog-Trace-Count") == "0" {
		return
	}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalln(err.Error())
	}
	if len(buf) == 0 {
		log.Println("empty traces")

		return
	}

	go sendDDTraceTask(cfg.Sender, buf, "http://"+cfg.DkAgent+path, req.Header)
}

func sendDDTraceTask(sender *sender, buf []byte, endpoint string, headers http.Header) {
	ddtraces := pb.Traces{}
	if _, err := ddtraces.UnmarshalMsg(buf); err != nil {
		log.Fatalln(err.Error())
	}

	wg := sync.WaitGroup{}
	wg.Add(sender.Threads)
	for i := 0; i < sender.Threads; i++ {
		dupi := shallowCopyDDTrace(ddtraces)

		go func(ddtraces pb.Traces) {
			defer wg.Done()

			for j := 0; j < sender.SendCount; j++ {
				modifyTraceID(ddtraces)
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
		}(dupi)
	}
	wg.Wait()

	close(globalCloser)
}

func shallowCopyDDTrace(src pb.Traces) pb.Traces {
	dest := make(pb.Traces, len(src))
	for i := range src {
		dest[i] = make(pb.Trace, len(src[i]))
		for j := range src[i] {
			dest[i][j] = &pb.Span{
				Service:    src[i][j].Service,
				Name:       src[i][j].Name,
				Resource:   src[i][j].Resource,
				TraceID:    src[i][j].TraceID,
				SpanID:     src[i][j].SpanID,
				ParentID:   src[i][j].ParentID,
				Start:      src[i][j].Start,
				Duration:   src[i][j].Duration,
				Error:      src[i][j].Error,
				Meta:       src[i][j].Meta,
				Metrics:    src[i][j].Metrics,
				Type:       src[i][j].Type,
				MetaStruct: src[i][j].MetaStruct,
			}
		}
	}

	return dest
}

func modifyTraceID(src pb.Traces) {
	if len(src) == 0 {
		log.Println("### empty ddtraces")

		return
	}

	var tid = uint64(idflk.NextInt64Id())
	for i := range src {
		if len(src[i]) == 0 {
			log.Println("### empty ddtrace")
			continue
		}

		for j := range src[i] {
			src[i][j].TraceID = tid
		}
	}
}

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace"
	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/tracer"
)

var (
	cfg          *config
	agentAddress = "127.0.0.1:"
)

type sender struct {
	Threads      int `json:"threads"`
	SendCount    int `json:"send_count"`
	SendInterval int `json:"send_interval"`
}

type config struct {
	DkAgent    string  `json:"dk_agent"`
	Sender     *sender `json:"sender"`
	Service    string  `json:"service"`
	DumpSize   int     `json:"dump_size"`
	RandomDump bool    `json:"random_dump"`
	Trace      []*span `json:"trace"`
}

type tag struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type span struct {
	Resource  string  `json:"resource"`
	Operation string  `json:"operation"`
	SpanType  string  `json:"span_type"`
	Duration  int64   `json:"duration"`
	Error     string  `json:"error"`
	Tags      []tag   `json:"tags"`
	Children  []*span `json:"children"`
	dumpSize  int64
}

func main() {
	go startAgent()

	tracer.Start(tracer.WithAgentAddr(agentAddress), tracer.WithService(cfg.Service), tracer.WithDebugMode(true), tracer.WithLogStartup(true))
	defer tracer.Stop()

	spanCount := countSpans(cfg.Trace, 0)
	log.Printf("### span count: %d\n", spanCount)
	log.Printf("### random dump: %v", cfg.RandomDump)
	if cfg.RandomDump {
		log.Printf("### dump size: 0kb~%dkb", cfg.DumpSize)
	} else {
		log.Printf("### dump size: %dkb", cfg.DumpSize)
	}

	var fillup int64
	if cfg.DumpSize > 0 {
		fillup = int64(cfg.DumpSize / spanCount)
		fillup <<= 10
		setPerDumpSize(cfg.Trace, fillup, cfg.RandomDump)
	}

	root, children := startRootSpan(cfg.Trace)
	orchestrator(tracer.ContextWithSpan(context.Background(), root), children)

	<-globalCloser

	tracer.Flush()
}

func countSpans(trace []*span, c int) int {
	c += len(trace)
	for i := range trace {
		if len(trace[i].Children) != 0 {
			c = countSpans(trace[i].Children, c)
		}
	}

	return c
}

func setPerDumpSize(trace []*span, fillup int64, isRandom bool) {
	for i := range trace {
		if isRandom {
			trace[i].dumpSize = rand.Int63n(fillup)
		} else {
			trace[i].dumpSize = fillup
		}
		if len(trace[i].Children) != 0 {
			setPerDumpSize(trace[i].Children, fillup, isRandom)
		}
	}
}

func startRootSpan(trace []*span) (root ddtrace.Span, children []*span) {
	var (
		d   int64
		err error
	)
	if len(trace) == 1 {
		root = tracer.StartSpan(trace[0].Operation)
		root.SetTag(ResourceName, trace[0].Resource)
		root.SetTag(SpanType, trace[0].SpanType)
		for _, tag := range trace[0].Tags {
			root.SetTag(tag.Key, tag.Value)
		}
		d = trace[0].Duration * int64(time.Millisecond)
		children = trace[0].Children
		if len(trace[0].Error) != 0 {
			err = errors.New(trace[0].Error)
		}
	} else {
		root = tracer.StartSpan("start_root_span")
		d = int64(time.Duration(60+rand.Intn(300)) * time.Millisecond)
		children = trace
	}

	time.Sleep(time.Duration(d) / 2)
	go func(root ddtrace.Span, d int64, err error) {
		time.Sleep(time.Duration(d) / 2)
		root.Finish(tracer.WithError(err))
	}(root, d, err)

	return
}

func orchestrator(ctx context.Context, children []*span) {
	if len(children) == 1 {
		ctx = startSpanFromContext(ctx, children[0])
		if len(children[0].Children) != 0 {
			orchestrator(ctx, children[0].Children)
		}
	} else {
		wg := sync.WaitGroup{}
		wg.Add(len(children))
		for k := range children {
			go func(ctx context.Context, span *span) {
				defer wg.Done()

				ctx = startSpanFromContext(ctx, span)
				if len(span.Children) != 0 {
					orchestrator(ctx, span.Children)
				}
			}(ctx, children[k])
		}
		wg.Wait()
	}
}

func getRandomHexString(n int64) string {
	buf := make([]byte, n)
	rand.Read(buf)

	return hex.EncodeToString(buf)
}

func startSpanFromContext(ctx context.Context, span *span) context.Context {
	var err error
	if len(span.Error) != 0 {
		err = errors.New(span.Error)
	}
	ddspan, ctx := tracer.StartSpanFromContext(ctx, span.Operation)
	defer ddspan.Finish(tracer.WithError(err))

	ddspan.SetTag(ResourceName, span.Resource)
	ddspan.SetTag(SpanType, span.SpanType)
	for _, tag := range span.Tags {
		ddspan.SetTag(tag.Key, tag.Value)
	}

	if span.dumpSize != 0 {
		ddspan.SetTag(DumpData, getRandomHexString(span.dumpSize))
	}

	time.Sleep(time.Duration(span.Duration * int64(time.Millisecond)))

	return ctx
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	data, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatalln(err.Error())
	}

	cfg = &config{}
	if err = json.Unmarshal(data, cfg); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}

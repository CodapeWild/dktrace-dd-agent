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

	"github.com/CodapeWild/devtools/idflaker"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	cfg          *config
	idflk        *idflaker.IDFlaker
	globalCloser = make(chan struct{})
	agentAddress = "127.0.0.1:"
	ddv4         = "/v0.4/traces"
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
	Resource  string        `json:"resource"`
	Operation string        `json:"operation"`
	SpanType  string        `json:"span_type"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error"`
	Tags      []tag         `json:"tags"`
	Children  []*span       `json:"children"`
	dumpSize  int64
}

func (sp *span) startSpanFromContext(ctx context.Context) (ddtrace.Span, context.Context) {
	var err error
	if len(sp.Error) != 0 {
		err = errors.New(sp.Error)
	}

	var ddspan ddtrace.Span
	ddspan, ctx = tracer.StartSpanFromContext(ctx, sp.Operation)
	defer ddspan.Finish(tracer.WithError(err))

	ddspan.SetTag(ResourceName, sp.Resource)
	ddspan.SetTag(SpanType, sp.SpanType)
	for _, tag := range sp.Tags {
		ddspan.SetTag(tag.Key, tag.Value)
	}

	if sp.dumpSize != 0 {
		buf := make([]byte, sp.dumpSize)
		rand.Read(buf)

		ddspan.SetTag(DumpData, hex.EncodeToString(buf))
	}

	time.Sleep(sp.Duration * time.Millisecond)

	return ddspan, ctx
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
	if cfg.RandomDump && cfg.DumpSize > 0 {
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
		d   time.Duration
		err error
	)
	if len(trace) == 1 {
		root = tracer.StartSpan(trace[0].Operation)
		root.SetTag(ResourceName, trace[0].Resource)
		root.SetTag(SpanType, trace[0].SpanType)
		for _, tag := range trace[0].Tags {
			root.SetTag(tag.Key, tag.Value)
		}
		d = trace[0].Duration * time.Millisecond
		children = trace[0].Children
		if len(trace[0].Error) != 0 {
			err = errors.New(trace[0].Error)
		}
	} else {
		root = tracer.StartSpan("startRootSpan")
		root.SetTag(SpanType, "web")
		d = time.Duration(60+rand.Intn(300)) * time.Millisecond
		children = trace
	}

	time.Sleep(d / 2)
	go func(root ddtrace.Span, d time.Duration, err error) {
		time.Sleep(d / 2)
		root.Finish(tracer.WithError(err))
	}(root, d, err)

	return
}

func orchestrator(ctx context.Context, children []*span) {
	if len(children) == 1 {
		_, ctx = children[0].startSpanFromContext(ctx)
		if len(children[0].Children) != 0 {
			orchestrator(ctx, children[0].Children)
		}
	} else {
		wg := sync.WaitGroup{}
		wg.Add(len(children))
		for k := range children {
			go func(ctx context.Context, span *span) {
				defer wg.Done()

				_, ctx = span.startSpanFromContext(ctx)
				if len(span.Children) != 0 {
					orchestrator(ctx, span.Children)
				}
			}(ctx, children[k])
		}
		wg.Wait()
	}
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
	if len(cfg.Trace) == 0 {
		log.Fatalln("empty trace")
	}

	if idflk, err = idflaker.NewIdFlaker(66); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}

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
	"time"

	"github.com/CodapeWild/devtools/idflaker"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	cfg          *config
	idflk        *idflaker.IDFlaker
	globalCloser chan struct{}
	agentAddress = "127.0.0.1:"
	path         = "/v0.4/traces"
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
	var ddspan ddtrace.Span
	ddspan, ctx = tracer.StartSpanFromContext(ctx, sp.Operation)

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

	total := int64(sp.Duration * time.Millisecond)
	d := rand.Int63n(total)
	time.Sleep(time.Duration(d))
	go func() {
		time.Sleep(time.Duration(total - d))
		if len(sp.Error) != 0 {
			ddspan.Finish(tracer.WithError(errors.New(sp.Error)))
		} else {
			ddspan.Finish()
		}
	}()

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
		if cfg.DumpSize <= 0 {
			cfg.DumpSize = rand.Intn(924) + 100
		}
		log.Printf("### dump size: 0kb~%dkb", cfg.DumpSize)
	} else {
		log.Printf("### dump size: %dkb", cfg.DumpSize)
	}

	if cfg.RandomDump || cfg.DumpSize > 0 {
		setPerDumpSize(cfg.Trace, int64(cfg.DumpSize/spanCount)<<10, cfg.RandomDump)
	}

	root, children := startRootSpan(cfg.Trace)
	orchestrator(tracer.ContextWithSpan(context.Background(), root), children)
	time.Sleep(3 * time.Second)
	tracer.Flush()

	<-globalCloser
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
	if isRandom {
		for i := range trace {
			trace[i].dumpSize = rand.Int63n(fillup)
			if len(trace[i].Children) != 0 {
				setPerDumpSize(trace[i].Children, fillup, isRandom)
			}
		}
	} else {
		for i := range trace {
			trace[i].dumpSize = fillup
			if len(trace[i].Children) != 0 {
				setPerDumpSize(trace[i].Children, fillup, isRandom)
			}
		}
	}
}

func startRootSpan(trace []*span) (root ddtrace.Span, children []*span) {
	var sp *span
	if len(trace) == 1 {
		sp = trace[0]
		children = trace[0].Children
	} else {
		sp = &span{
			Operation: "startRootSpan",
			SpanType:  "web",
			Duration:  time.Duration(60 + rand.Intn(300)),
		}
		children = trace
	}
	root, _ = sp.startSpanFromContext(context.Background())

	return
}

func orchestrator(ctx context.Context, children []*span) {
	if len(children) == 1 {
		_, ctx = children[0].startSpanFromContext(ctx)
		if len(children[0].Children) != 0 {
			orchestrator(ctx, children[0].Children)
		}
	} else {
		for k := range children {
			go func(ctx context.Context, span *span) {
				_, ctx = span.startSpanFromContext(ctx)
				if len(span.Children) != 0 {
					orchestrator(ctx, span.Children)
				}
			}(ctx, children[k])
		}
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
	if cfg.Sender == nil || cfg.Sender.Threads <= 0 || cfg.Sender.SendCount <= 0 {
		log.Fatalln("sender not configured properly")
	}
	if len(cfg.Trace) == 0 {
		log.Fatalln("empty trace")
	}

	globalCloser = make(chan struct{})

	if idflk, err = idflaker.NewIdFlaker(66); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}

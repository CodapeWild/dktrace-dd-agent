package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace"
	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/tracer"
)

var cfg *config

type sender struct {
	Threads      int `json:"threads"`
	SendCount    int `json:"send_count"`
	SendInterval int `json:"send_interval"`
}

type config struct {
	DkAgent string  `json:"dk_agent"`
	Sender  *sender `json:"sender"`
	Service string  `json:"service"`
	Trace   []*span `json:"trace"`
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
}

func main() {
	go startAgent()

	tracer.Start(tracer.WithAgentAddr(agentAddress), tracer.WithService(cfg.Service), tracer.WithDebugMode(true), tracer.WithLogStartup(true))
	defer tracer.Stop()

	root, children := startRootSpan(cfg.Trace)
	orchestrator(tracer.ContextWithSpan(context.Background(), root), children)

	<-globalCloser
}

func startRootSpan(trace []*span) (root ddtrace.Span, children []*span) {
	var (
		d   int64
		err error
	)
	if len(trace) == 1 {
		root = tracer.StartSpan(trace[0].Operation)
		root.SetTag(ext.ResourceName, trace[0].Resource)
		root.SetTag(ext.SpanType, trace[0].SpanType)
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
		d = int64(10 * time.Millisecond)
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

func startSpanFromContext(ctx context.Context, span *span) context.Context {
	ddspan, ctx := tracer.StartSpanFromContext(ctx, span.Operation)

	ddspan.SetTag(ext.ResourceName, span.Resource)
	ddspan.SetTag(ext.SpanType, span.SpanType)
	for _, tag := range span.Tags {
		ddspan.SetTag(tag.Key, tag.Value)
	}

	var err error
	if len(span.Error) != 0 {
		err = errors.New(span.Error)
	}

	time.Sleep(time.Duration(span.Duration * int64(time.Millisecond)))

	ddspan.Finish(tracer.WithError(err))

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
}
